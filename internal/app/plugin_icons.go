package app

import (
	"bytes"
	"context"
	"image/png"
	"io"
	"os"
	"time"

	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/plugin"
	"fluentlauncher/internal/plugin/protocol"
)

func (a *App) marketIcon(c *ui.Context, item plugin.MarketItem, size float32) {
	theme := c.Theme()
	key := item.ID
	var registration protocol.Registration
	if item.Installed != nil {
		registration = item.Installed.Registration
		key += ":" + registration.Manifest.Version + ":" + registration.Manifest.Icon
	}
	if item.Official != nil && item.Official.Icon != nil {
		key += ":" + item.Official.Icon.SHA256
	}
	if a.market.icons == nil {
		a.market.icons = map[string]*ui.Bitmap{}
		a.market.iconPending = map[string]bool{}
		a.market.iconSlots = make(chan struct{}, 4)
	}
	bitmap, loaded := a.market.icons[key]
	client := a.official.client
	var asset *plugin.PluginPackage
	if item.Official != nil {
		asset = item.Official.Icon
	}
	if !loaded && !a.market.iconPending[key] && a.panel != nil && (registration.Manifest.Icon != "" || asset != nil && client != nil) {
		a.market.iconPending[key] = true
		parent := a.updates.ctx
		if parent == nil {
			parent = context.Background()
		}
		ctx, cancel := context.WithTimeout(parent, 30*time.Second)
		slots := a.market.iconSlots
		go func() {
			defer cancel()
			var icon *ui.Bitmap
			defer func() {
				a.publishUpdate(func() {
					delete(a.market.iconPending, key)
					if len(a.market.icons) >= 256 {
						clear(a.market.icons)
					}
					a.market.icons[key] = icon
				})
			}()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				return
			}
			var data []byte
			if registration.Manifest.Icon != "" {
				if path, err := protocol.ResourcePath(registration.Directory, registration.Manifest.Icon); err == nil {
					if f, err := os.Open(path); err == nil {
						data, _ = io.ReadAll(io.LimitReader(f, (1<<20)+1))
						_ = f.Close()
					}
				}
			}
			if len(data) == 0 && asset != nil && client != nil {
				data, _ = client.Icon(ctx, *asset)
			}
			if len(data) <= 1<<20 {
				if config, err := png.DecodeConfig(bytes.NewReader(data)); err == nil && config.Width > 0 && config.Height > 0 && config.Width <= 512 && config.Height <= 512 {
					icon, _ = ui.DecodeBitmap(data)
				}
			}
		}()
	}
	ui.Box(c).Size(size, size).Shrink(0).Radius(theme.Radius * 2).Background(theme.Accent.Alpha(0.12)).Center().Children(func() {
		if bitmap != nil {
			ui.Image(c, bitmap).Size(size, size).Radius(theme.Radius * 2)
		} else {
			ui.Icon(c, pageIcons[pagePlugins]).FontSize(size * 0.55).TextColor(theme.Accent)
		}
	})
}
