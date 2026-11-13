//go:build windows

package plugin

import (
	"context"
	"time"
)

// SettingsChanged never starts an idle plugin or refreshes its idle lifetime.
// It uses the peer directly: a config update may originate inside an execute
// RPC whose operation gate is already held.
func (m *Manager) SettingsChanged(id string) {
	m.mu.Lock()
	session := m.sessions[id]
	m.mu.Unlock()
	if session == nil || !session.client.settingsEvents.Load() {
		return
	}
	peer := session.client.peer.Load()
	if peer == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = peer.Call(ctx, "settingsChanged", map[string]any{}, 0)
	}()
}
