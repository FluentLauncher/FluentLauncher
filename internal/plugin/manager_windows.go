//go:build windows

package plugin

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/search"
	"fluentlauncher/internal/core/settings"
	"fluentlauncher/internal/plugin/protocol"
)

// maxConsecutiveFailures is how many failed queries a plugin gets before the
// manager pauses it, so one broken package cannot slow every search down.
const maxConsecutiveFailures = 3

type session struct {
	client   *Client
	busy     int
	failures int
	// dropped counts results the launcher refused because they broke the
	// protocol, and dropNote keeps the first reason, which is what a plugin
	// author needs to see when a plugin "returns nothing".
	dropped  int
	dropNote string
	idle     *time.Timer
}

// runtimeState is what survives a plugin process: the last numbers it
// reported, so the settings page can still say how slow it was after it has
// been recycled.
type runtimeState struct {
	stats    ClientStats
	failures int
	dropped  int
	dropNote string
}

// Manager owns plugin processes and exposes them as one search source.
type Manager struct {
	mu        sync.Mutex
	installed []protocol.Registration
	options   settings.PluginOptions
	sessions  map[string]*session
	history   map[string]runtimeState
	paused    map[string]bool
	version   int64
	disposed  bool
}

func NewManager() *Manager {
	return &Manager{
		sessions: map[string]*session{},
		history:  map[string]runtimeState{},
		paused:   map[string]bool{},
		options:  settings.DefaultPluginOptions(),
	}
}

// Configure applies new settings. Sessions the settings no longer want are
// closed off this goroutine: the settings page calls Configure while a query
// may still be running, and stopping a process waits for that query, so the
// window would freeze for as long as a slow plugin takes.
func (m *Manager) Configure(installed []protocol.Registration, options settings.PluginOptions) {
	m.mu.Lock()
	m.installed = append([]protocol.Registration(nil), installed...)
	m.options = options
	m.paused = map[string]bool{}
	closing := make([]detached, 0, len(m.sessions))
	for id, session := range m.sessions {
		if !containsFold(options.EnabledIDs, id) || !m.has(id) {
			closing = append(closing, m.detachLocked(id))
			continue
		}
		session.failures = 0
	}
	for id := range m.history {
		if !m.has(id) {
			delete(m.history, id)
		}
	}
	m.mu.Unlock()
	for _, item := range closing {
		go m.closeSession(item, true)
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
		return []model.Result{errorResult(id, registration.Manifest.Name, "插件连续失败已暂停；请在设置的插件页重启它。")}
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
		sess.busy--
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			// A search replaced by the next keystroke is not a plugin failure:
			// counting it would pause a plugin the user is merely typing at.
			m.scheduleIdleLocked(id, sess)
			m.mu.Unlock()
			return nil
		}
		sess.failures++
		failures := sess.failures
		paused := failures >= maxConsecutiveFailures
		if paused {
			m.paused[id] = true
		}
		m.scheduleIdleLocked(id, sess)
		m.mu.Unlock()
		message := err.Error()
		if paused {
			message = fmt.Sprintf("插件连续 %d 次查询失败，已暂停；请在设置中重启它。最近的错误：%s", failures, message)
		}
		return []model.Result{errorResult(id, registration.Manifest.Name, message)}
	}
	sess.failures = 0
	results := make([]model.Result, 0, len(response.Results))
	rank := 0
	dropped := 0
	dropNote := ""
	for _, item := range response.Results {
		if reason, ok := validateResult(item); !ok {
			dropped++
			if dropNote == "" {
				dropNote = "结果被丢弃：" + reason
			}
			continue
		}
		actions := make([]model.Action, 0, len(item.Actions))
		for _, action := range item.Actions {
			if reason, ok := validateAction(action); !ok {
				dropped++
				if dropNote == "" {
					dropNote = "动作被丢弃：" + reason
				}
				continue
			}
			actions = append(actions, model.Action{ID: action.ID, Title: action.Title, Kind: action.Kind, Value: action.Value})
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
			if action.Kind == protocol.KindCopyText {
				entry.CopyText = action.Value
				break
			}
		}
		results = append(results, model.Result{Entry: entry, Score: 13000 - rank})
		rank++
	}
	if dropped > 0 {
		sess.dropped += dropped
		sess.dropNote = dropNote
		log.Printf("plugin %s: dropped %d item(s): %s", id, dropped, dropNote)
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
	if !containsFold(options.EnabledIDs, entry.PluginID) {
		m.mu.Unlock()
		return fmt.Errorf("插件 %s 已停用，请在设置中启用后再执行", entry.PluginID)
	}
	if sess == nil {
		m.mu.Unlock()
		return errors.New("插件进程已回收，请重新搜索后再执行")
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

// Statuses returns one row per installed plugin, in package order, including
// the plugins whose process is not running: those keep the numbers they last
// reported. The settings page draws these rows, so the call takes the
// manager's lock only to copy data and never waits for a plugin.
func (m *Manager) Statuses() []Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Status, 0, len(m.installed))
	for _, registration := range m.installed {
		id := registration.Manifest.ID
		row := Status{
			Registration: registration,
			Enabled:      containsFold(m.options.EnabledIDs, id),
			GlobalQuery:  containsFold(m.options.GlobalQueryIDs, id),
			Paused:       m.paused[id],
		}
		switch {
		case m.sessions[id] != nil:
			session := m.sessions[id]
			row.Failures = session.failures
			row.Dropped = session.dropped
			row.DropNote = session.dropNote
			row.ClientStats = session.client.Stats()
		case m.history[id] != (runtimeState{}):
			state := m.history[id]
			row.Failures = state.failures
			row.Dropped = state.dropped
			row.DropNote = state.dropNote
			row.ClientStats = state.stats
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Registration.Manifest.ID < out[j].Registration.Manifest.ID })
	return out
}

// Restart stops a plugin's process and clears the pause that consecutive
// failures set, so the next search starts it again with a clean slate. The
// settings page offers it on a paused plugin, which is the only way back
// without editing the configuration file.
func (m *Manager) Restart(id string) {
	m.mu.Lock()
	item := m.detachLocked(id)
	delete(m.paused, id)
	delete(m.history, id)
	m.mu.Unlock()
	// No history: a restart is a clean slate, including the page's numbers.
	m.closeSession(item, false)
}

// Stop ends a plugin's process. It waits for a query in flight to finish,
// because the uninstall path deletes the package right after: a process still
// running from that directory would keep the files locked on Windows.
func (m *Manager) Stop(id string) {
	m.mu.Lock()
	item := m.detachLocked(id)
	delete(m.paused, id)
	m.mu.Unlock()
	m.closeSession(item, true)
}

func (m *Manager) Dispose() {
	m.mu.Lock()
	m.disposed = true
	items := make([]detached, 0, len(m.sessions))
	for id := range m.sessions {
		items = append(items, m.detachLocked(id))
	}
	m.mu.Unlock()
	for _, item := range items {
		m.closeSession(item, false)
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
		if m.sessions[id] != session || session.busy != 0 {
			m.mu.Unlock()
			return
		}
		item := m.detachLocked(id)
		m.mu.Unlock()
		m.closeSession(item, true)
	})
}

// detached pairs a session with its plugin id, so it can be closed after the
// manager's lock is released.
type detached struct {
	id      string
	session *session
}

// detachLocked takes a session out of the maps without stopping its process;
// the caller closes it once the lock is free.
func (m *Manager) detachLocked(id string) detached {
	session, ok := m.sessions[id]
	if !ok {
		return detached{}
	}
	delete(m.sessions, id)
	if session.idle != nil {
		session.idle.Stop()
	}
	return detached{id: id, session: session}
}

// closeSession stops one detached process and, when asked, keeps the numbers it
// last reported, so the page can still say how fast a plugin was, and why it
// failed, after the process is gone. It must not be called with the manager's
// lock held: closing waits for a query in flight.
func (m *Manager) closeSession(item detached, record bool) {
	if item.session == nil {
		return
	}
	item.session.client.Close()
	if !record {
		return
	}
	m.mu.Lock()
	m.history[item.id] = runtimeState{
		stats:    item.session.client.Stats(),
		failures: item.session.failures,
		dropped:  item.session.dropped,
		dropNote: item.session.dropNote,
	}
	m.mu.Unlock()
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

// validateResult reports whether a plugin result may be shown, and when it may
// not, which rule it broke: the reason goes to the plugin's diagnostics, where
// it is the only feedback a plugin author gets for a result that silently does
// not appear.
func validateResult(result protocol.Result) (string, bool) {
	switch {
	case result.ID == "":
		return "结果缺少 id", false
	case len(result.ID) > 512:
		return "结果 id 超过 512 个字符", false
	case strings.TrimSpace(result.Title) == "":
		return "结果缺少标题", false
	case len(result.Title) > 1000:
		return "结果标题超过 1000 个字符", false
	case len(result.Subtitle) > 4000:
		return "结果副标题超过 4000 个字符", false
	case len(result.Actions) > 10:
		return "一个结果最多 10 个动作", false
	case len(result.PreviewText) > 16000:
		return "预览文本超过 16000 个字符", false
	}
	return "", true
}

func validateAction(action protocol.Action) (string, bool) {
	switch {
	case action.ID == "":
		return "动作缺少 id", false
	case len(action.ID) > 200:
		return "动作 id 超过 200 个字符", false
	case strings.TrimSpace(action.Title) == "":
		return "动作缺少标题", false
	case len(action.Title) > 200:
		return "动作标题超过 200 个字符", false
	case len(action.Value) > 16000:
		return "动作值超过 16000 个字符", false
	}
	switch action.Kind {
	case protocol.KindCopyText, protocol.KindInvoke:
		return "", true
	case protocol.KindOpenURL:
		if strings.HasPrefix(action.Value, "http://") || strings.HasPrefix(action.Value, "https://") {
			return "", true
		}
		return fmt.Sprintf("动作类型 openUrl 的值必须是 http 或 https 地址，当前为 %q", truncate(action.Value, 80)), false
	case protocol.KindOpenPath:
		if filepath.IsAbs(action.Value) {
			return "", true
		}
		return fmt.Sprintf("动作类型 openPath 的值必须是绝对路径，当前为 %q", truncate(action.Value, 80)), false
	default:
		return fmt.Sprintf("不支持的动作类型 %q", action.Kind), false
	}
}

func truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

func containsFold(items []string, value string) bool {
	for _, item := range items {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}
