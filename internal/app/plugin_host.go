package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"

	"fluentlauncher/internal/plugin"
	"fluentlauncher/internal/plugin/hostservices"
	"fluentlauncher/internal/plugin/protocol"
	"fluentlauncher/internal/plugin/rpc"
)

type pluginHost struct {
	app          *App
	registration protocol.Registration
	store        *hostservices.Store
	noticeMu     sync.Mutex
	lastNotice   time.Time
	lastNoticeID string
}

func (a *App) newPluginHost(registration protocol.Registration) plugin.HostProvider {
	return &pluginHost{app: a, registration: registration, store: a.pluginData}
}

func (h *pluginHost) Configuration() (plugin.HostConfiguration, error) {
	data, cache, err := h.store.Directories(h.registration.Manifest.ID)
	if err != nil {
		return plugin.HostConfiguration{}, err
	}
	configuration := plugin.HostConfiguration{APIVersion: 1, HostVersion: appVersion(), Locale: i18n.Locale(), DataDir: data, CacheDir: cache}
	for _, name := range h.registration.Manifest.HostServices {
		if version := hostservices.Version(name); version > 0 {
			configuration.Services = append(configuration.Services, plugin.HostService{Name: name, Version: version})
		}
	}
	for _, required := range h.registration.Manifest.RequiredHostServices {
		if hostservices.Version(required.Name) < required.Version {
			return configuration, serviceError("unsupported", "required host service unavailable: "+required.Name)
		}
	}
	return configuration, nil
}

func serviceError(kind, message string) error {
	return &rpc.Error{Code: -32001, Message: message, Data: map[string]string{"kind": kind}}
}
func invalidServiceParams() error {
	return &rpc.Error{Code: -32602, Message: "invalid host service parameters"}
}

func (h *pluginHost) Call(ctx context.Context, method string, params json.RawMessage, interactive bool) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	name := strings.TrimPrefix(method, "host.")
	if name == method || hostservices.Version(name) == 0 {
		return nil, &rpc.Error{Code: -32601, Message: "unknown host service"}
	}
	declared := false
	for _, service := range h.registration.Manifest.HostServices {
		if service == name {
			declared = true
			break
		}
	}
	if !declared {
		return nil, serviceError("not_declared", "host service was not declared by this plugin: "+name)
	}
	if !interactive && (name == "launcher.show" || name == "launcher.hide" || strings.HasPrefix(name, "dialogs.") || strings.HasPrefix(name, "shell.")) {
		return nil, serviceError("interaction_required", "call this service from an explicit command or action")
	}
	var result any = map[string]any{}
	var err error
	switch name {
	case "context.get":
		return h.Configuration()
	case "settings.get":
		var values map[string]json.RawMessage
		values, err = h.store.Get(h.registration.Manifest)
		result = map[string]any{"values": values}
	case "settings.set":
		var request struct {
			Key   string          `json:"key"`
			Value json.RawMessage `json:"value"`
		}
		if json.Unmarshal(params, &request) != nil || request.Key == "" {
			return nil, invalidServiceParams()
		}
		err = h.store.SetIf(h.registration.Manifest, request.Key, request.Value, func() bool { return ctx.Err() == nil })
		if err == nil {
			err = ctx.Err()
		}
		if err == nil {
			h.app.plugins.SettingsChanged(h.registration.Manifest.ID)
		}
	case "clipboard.readText":
		text := mygo.Clipboard.ReadText()
		if len(text) > 1<<20 {
			return nil, serviceError("too_large", "clipboard text exceeds 1 MiB")
		}
		return map[string]string{"text": text}, nil
	case "clipboard.writeText":
		var request struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(params, &request) != nil || len(request.Text) > 1<<20 {
			return nil, invalidServiceParams()
		}
		mygo.Clipboard.WriteText(request.Text)
	case "launcher.show":
		var request struct {
			Query string `json:"query"`
		}
		if json.Unmarshal(params, &request) != nil || len(request.Query) > 4096 {
			return nil, invalidServiceParams()
		}
		err = h.onUI(ctx, func() error {
			h.app.show()
			h.app.query = request.Query
			h.app.updateResults()
			h.app.input.Focus()
			return nil
		})
	case "launcher.hide":
		err = h.onUI(ctx, func() error { h.app.hidePanel(); return nil })
	case "ui.notify":
		var request struct{ Kind, Title, Message, ReplaceID string }
		if json.Unmarshal(params, &request) != nil || len(request.Title) > 400 || len(request.Message) > 4000 || len(request.ReplaceID) > 100 || request.Title == "" {
			return nil, invalidServiceParams()
		}
		if request.Kind != "" && request.Kind != "info" && request.Kind != "success" && request.Kind != "error" {
			return nil, invalidServiceParams()
		}
		h.noticeMu.Lock()
		if time.Since(h.lastNotice) < time.Second {
			if request.ReplaceID != "" && request.ReplaceID == h.lastNoticeID {
				h.noticeMu.Unlock()
				return map[string]any{}, nil
			}
			h.noticeMu.Unlock()
			return nil, serviceError("rate_limited", "notifications are limited to one per second")
		}
		h.lastNotice = time.Now()
		h.lastNoticeID = request.ReplaceID
		h.noticeMu.Unlock()
		err = h.onUI(ctx, func() error {
			text := h.registration.Manifest.Name + " · " + request.Title
			if request.Message != "" {
				text += " · " + request.Message
			}
			if request.Kind == "error" {
				h.app.notice = text
				h.app.settingsNotice = ""
			} else {
				h.app.status = text
				h.app.settingsNotice = text
			}
			id := ""
			if request.ReplaceID != "" {
				id = "plugin:" + h.registration.Manifest.ID + ":" + request.ReplaceID
			}
			return mygo.NewNotification(mygo.NotificationOptions{ID: id, Group: h.registration.Manifest.ID, Title: h.registration.Manifest.Name + " · " + request.Title, Body: request.Message}).Show()
		})
	case "dialogs.open", "dialogs.save":
		var request struct {
			Title       string            `json:"title"`
			DefaultPath string            `json:"defaultPath"`
			Directory   bool              `json:"directory"`
			Multiple    bool              `json:"multiple"`
			Filters     []mygo.FileFilter `json:"filters"`
		}
		if json.Unmarshal(params, &request) != nil || len(request.Title) > 400 || len(request.DefaultPath) > 4096 || len(request.Filters) > 20 {
			return nil, invalidServiceParams()
		}
		title := h.registration.Manifest.Name + " · " + request.Title
		if name == "dialogs.open" {
			var paths []string
			paths, err = mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: h.app.panel, Title: title, DefaultPath: request.DefaultPath, Directory: request.Directory, Multiple: request.Multiple, Filters: request.Filters})
			result = map[string]any{"paths": paths, "cancelled": len(paths) == 0}
		} else {
			var path string
			path, err = mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: h.app.panel, Title: title, DefaultPath: request.DefaultPath, Filters: request.Filters})
			result = map[string]any{"path": path, "cancelled": path == ""}
		}
	case "shell.open", "shell.reveal":
		var request struct {
			Target string `json:"target"`
		}
		if json.Unmarshal(params, &request) != nil || len(request.Target) > 16000 {
			return nil, invalidServiceParams()
		}
		if filepath.IsAbs(request.Target) {
			if name == "shell.reveal" {
				mygo.Shell.ShowItemInFolder(request.Target)
			} else {
				err = mygo.Shell.OpenPath(request.Target)
			}
		} else {
			address, parseErr := url.Parse(request.Target)
			if name == "shell.reveal" || parseErr != nil || address.Host == "" || (address.Scheme != "https" && address.Scheme != "http") {
				return nil, invalidServiceParams()
			}
			err = mygo.Shell.OpenExternal(request.Target)
		}
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, serviceError("cancelled", "host service was cancelled")
		}
		return nil, serviceError("unavailable", err.Error())
	}
	return result, nil
}

func (h *pluginHost) onUI(ctx context.Context, action func() error) error {
	if h.app.panel == nil {
		return serviceError("unavailable", "launcher window is unavailable")
	}
	done := make(chan error, 1)
	h.app.panel.Update(func() {
		if err := ctx.Err(); err != nil {
			done <- err
			return
		}
		if h.app.quitting {
			done <- context.Canceled
			return
		}
		done <- action()
		if h.app.settingsWin != nil {
			h.app.settingsWin.Invalidate()
		}
	})
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
