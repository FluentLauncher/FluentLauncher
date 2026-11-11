//go:build windows

package plugin

import (
	"context"
	"time"

	"fluentlauncher/internal/plugin/protocol"
)

// SetTheme records the palette without starting a process or blocking the UI.
func (c *Client) SetTheme(theme protocol.Theme) {
	c.themeMu.Lock()
	c.theme = &theme
	c.themeMu.Unlock()
}

func (c *Client) syncThemeLocked(ctx context.Context) error {
	if c.cmd == nil || !containsFold(c.capabilities, "theme") {
		return nil
	}
	c.themeMu.Lock()
	theme := c.theme
	c.themeMu.Unlock()
	if theme == nil || (c.sentTheme != nil && *c.sentTheme == *theme) {
		return nil
	}
	if _, err := c.requestLocked(ctx, "setTheme", reqExecute, *theme, time.Second); err != nil {
		return err
	}
	c.sentTheme = theme
	return nil
}

func (c *Client) syncTheme(ctx context.Context) {
	if err := c.lock(ctx); err != nil {
		return
	}
	defer func() { <-c.opGate }()
	if err := c.syncThemeLocked(ctx); err != nil {
		c.fail(err, ctx)
	}
}

func (m *Manager) SetTheme(theme protocol.Theme) {
	m.mu.Lock()
	if m.theme != nil && *m.theme == theme {
		m.mu.Unlock()
		return
	}
	m.theme = &theme
	for _, sess := range m.sessions {
		sess.client.SetTheme(theme)
		go func(client *Client) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			client.syncTheme(ctx)
		}(sess.client)
	}
	m.mu.Unlock()
}
