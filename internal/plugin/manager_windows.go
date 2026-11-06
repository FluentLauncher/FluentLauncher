//go:build windows

package plugin

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mistquay/fluentlauncher/internal/core/model"
	"github.com/mistquay/fluentlauncher/internal/core/search"
	"github.com/mistquay/fluentlauncher/internal/core/settings"
	"github.com/mistquay/fluentlauncher/internal/plugin/protocol"
)

type session struct {
	client   *Client
	busy     int
	failures int
	idle     *time.Timer
}

// Manager owns plugin processes and exposes them as one search source.
type Manager struct {
	mu        sync.Mutex
	installed []protocol.Registration
	options   settings.PluginOptions
	sessions  map[string]*session
	paused    map[string]bool
	version   int64
	disposed  bool
}

func NewManager() *Manager {
	return &Manager{
		sessions: map[string]*session{},
		paused:   map[string]bool{},
		options:  settings.DefaultPluginOptions(),
	}
}

func (m *Manager) Configure(installed []protocol.Registration, options settings.PluginOptions) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.installed = append([]protocol.Registration(nil), installed...)
	m.options = options
	m.paused = map[string]bool{}
	for id, session := range m.sessions {
		if !containsFold(options.EnabledIDs, id) || !m.has(id) {
			m.removeLocked(id)
			continue
		}
		session.failures = 0
	}
}

func (m *Manager) ID() string { return "plugins" }

func (m *Manager) Supports(q search.Query) bool {
	return (q.Scope == search.ScopeAll || q.Scope == search.ScopePlugins) && strings.TrimSpace(q.Text) != ""
}

func (m *Manager) Search(ctx context.Context, q search.Query, limit int) ([]model.Result, error) {
	m.mu.Lock()
	installed := append([]protocol.Registration(nil), m.installed...)
	options := m.options
	version := m.version + 1
	m.version = version
	m.mu.Unlock()

	type route struct {
		registration protocol.Registration
		text         string
		keyword      string
	}
	routes := make([]route, 0)
	for _, registration := range installed {
		if !containsFold(options.EnabledIDs, registration.Manifest.ID) {
			continue
		}
		if text, keyword, ok := m.route(registration, q, options); ok {
			routes = append(routes, route{registration, text, keyword})
		}
	}
	results := make([]model.Result, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, routeItem := range routes {
		wg.Add(1)
		go func(item route) {
			defer wg.Done()
			batch := m.searchOne(ctx, item.registration, item.text, item.keyword, min(limit, options.MaxResults), version, options)
			mu.Lock()
			results = append(results, batch...)
			mu.Unlock()
		}(routeItem)
	}
	wg.Wait()
	sort.SliceStable(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (m *Manager) route(registration protocol.Registration, q search.Query, options settings.PluginOptions) (string, string, bool) {
	for _, keyword := range registration.Manifest.Keywords {
		if strings.EqualFold(q.Text, keyword) {
			return "", keyword, true
		}
		if len(q.Text) > len(keyword) && strings.EqualFold(q.Text[:len(keyword)], keyword) && q.Text[len(keyword)] == ' ' {
			return strings.TrimSpace(q.Text[len(keyword)+1:]), keyword, true
		}
	}
	if q.Scope == search.ScopeAll && containsFold(options.GlobalQueryIDs, registration.Manifest.ID) {
		return q.Text, "", true
	}
	return "", "", false
}

func (m *Manager) searchOne(ctx context.Context, registration protocol.Registration, text, keyword string,
	limit int, version int64, options settings.PluginOptions) []model.Result {
	id := registration.Manifest.ID
	m.mu.Lock()
	if m.disposed {
		m.mu.Unlock()
		return nil
	}
	if m.paused[id] {
		m.mu.Unlock()
		return []model.Result{errorResult(id, registration.Manifest.Name, "插件连续失败已暂停，请在设置中重新启用。")}
	}
	sess := m.sessions[id]
	if sess == nil {
		if len(m.sessions) >= options.MaxActiveProcesses {
			m.mu.Unlock()
			return []model.Result{errorResult(id, registration.Manifest.Name, "插件活动进程已达上限，请稍后重试。")}
		}
		sess = &session{client: NewClient(registration.Path(), id)}
		m.sessions[id] = sess
	}
	if sess.idle != nil {
		sess.idle.Stop()
		sess.idle = nil
	}
	sess.busy++
	m.mu.Unlock()

	response, err := sess.client.Query(ctx, protocol.Query{
		Text: text, MaxResults: limit, QueryVersion: version, Keyword: keyword,
	}, time.Duration(options.QueryTimeoutMs)*time.Millisecond)
	if err == nil {
		log.Printf("plugin %s: %d results for %q", id, len(response.Results), text)
	} else {
		log.Printf("plugin %s: %v", id, err)
	}

	m.mu.Lock()
	if err != nil {
		sess.failures++
		if sess.failures >= 3 {
			m.paused[id] = true
		}
		sess.busy--
		m.scheduleIdleLocked(id, sess)
		m.mu.Unlock()
		if ctx.Err() != nil {
			return nil
		}
		return []model.Result{errorResult(id, registration.Manifest.Name, err.Error())}
	}
	sess.failures = 0
	results := make([]model.Result, 0, len(response.Results))
	rank := 0
	for _, item := range response.Results {
		if !validResult(item) {
			continue
		}
		actions := make([]model.Action, 0, len(item.Actions))
		for _, action := range item.Actions {
			if validAction(action) {
				actions = append(actions, model.Action{ID: action.ID, Title: action.Title, Kind: action.Kind, Value: action.Value})
			}
		}
		entry := model.Entry{
			ID:           "plugin:" + id + ":" + item.ID,
			Title:        item.Title,
			Subtitle:     item.Subtitle,
			Kind:         model.Plugin,
			SourceID:     "plugins",
			PluginID:     id,
			PluginResult: item.ID,
			Actions:      actions,
			PreviewText:  item.PreviewText,
		}
		for _, action := range actions {
			if action.Kind == "copyText" {
				entry.CopyText = action.Value
				break
			}
		}
		results = append(results, model.Result{Entry: entry, Score: 13000 - rank})
		rank++
	}
	sess.busy--
	m.scheduleIdleLocked(id, sess)
	m.mu.Unlock()
	return results
}

func (m *Manager) Execute(ctx context.Context, entry model.Entry, action model.Action) error {
	m.mu.Lock()
	sess := m.sessions[entry.PluginID]
	options := m.options
	if sess == nil || !containsFold(options.EnabledIDs, entry.PluginID) {
		m.mu.Unlock()
		return fmt.Errorf("插件未启用或进程已回收，请重新查询")
	}
	sess.busy++
	if sess.idle != nil {
		sess.idle.Stop()
		sess.idle = nil
	}
	m.mu.Unlock()
	err := sess.client.Execute(ctx, protocol.ExecuteRequest{ResultID: entry.PluginResult, ActionID: action.ID},
		time.Duration(options.QueryTimeoutMs)*time.Millisecond)
	m.mu.Lock()
	sess.busy--
	m.scheduleIdleLocked(entry.PluginID, sess)
	m.mu.Unlock()
	return err
}

func (m *Manager) Diagnostics() []Diagnostics {
	m.mu.Lock()
	sessions := make([]*session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.mu.Unlock()
	out := make([]Diagnostics, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, session.client.Diagnostics())
	}
	return out
}

func (m *Manager) Stop(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeLocked(id)
	delete(m.paused, id)
}

func (m *Manager) Dispose() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.disposed = true
	for id := range m.sessions {
		m.removeLocked(id)
	}
}

func (m *Manager) scheduleIdleLocked(id string, session *session) {
	if session.busy != 0 || m.disposed {
		return
	}
	seconds := m.options.IdleSeconds
	if seconds <= 0 {
		seconds = 120
	}
	session.idle = time.AfterFunc(time.Duration(seconds)*time.Second, func() {
		m.mu.Lock()
		if m.sessions[id] == session && session.busy == 0 {
			m.removeLocked(id)
		}
		m.mu.Unlock()
	})
}

func (m *Manager) removeLocked(id string) {
	session, ok := m.sessions[id]
	if !ok {
		return
	}
	delete(m.sessions, id)
	if session.idle != nil {
		session.idle.Stop()
	}
	session.client.Close()
}

func (m *Manager) has(id string) bool {
	for _, registration := range m.installed {
		if registration.Manifest.ID == id {
			return true
		}
	}
	return false
}

func errorResult(id, name, message string) model.Result {
	return model.Result{Entry: model.Entry{
		ID:       "plugin-error:" + id,
		Title:    name + " 暂不可用",
		Subtitle: message,
		Kind:     model.Plugin,
		SourceID: "plugins",
	}, Score: -1}
}

func validResult(result protocol.Result) bool {
	return result.ID != "" && len(result.ID) <= 512 && result.Title != "" && len(result.Title) <= 1000 &&
		len(result.Subtitle) <= 4000 && len(result.Actions) <= 10 && len(result.PreviewText) <= 16000
}

func validAction(action protocol.Action) bool {
	if action.ID == "" || len(action.ID) > 200 || action.Title == "" || len(action.Title) > 200 ||
		len(action.Value) > 16000 {
		return false
	}
	switch action.Kind {
	case "copyText", "invoke":
		return true
	case "openUrl":
		return strings.HasPrefix(action.Value, "http://") || strings.HasPrefix(action.Value, "https://")
	case "openPath":
		return filepath.IsAbs(action.Value)
	}
	return false
}

func containsFold(items []string, value string) bool {
	for _, item := range items {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}
