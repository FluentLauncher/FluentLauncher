package app

import (
	"bytes"
	"context"
	"image"
	"log"
	"math"
	"os"
	"time"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"

	"fluentlauncher/internal/core/settings"
)

type backgroundRequest struct {
	path      string
	maxPixels int
}

type backgroundCache struct {
	request  backgroundRequest
	modified time.Time
	size     int64
	bitmap   *ui.Bitmap
}

// Only the UI thread changes the loader. Workers receive value snapshots;
// generations keep a late decode from replacing a newer image or theme.
type backgroundLoader struct {
	request    backgroundRequest
	generation uint64
	cancel     context.CancelFunc
	cache      backgroundCache
}

func (a *App) loadBackground(theme settings.ThemeSettings) {
	request := backgroundRequest{}
	if theme.Background == settings.BackgroundImage && theme.ImagePath != "" {
		// The same picture serves the panel and settings. Bound the decoded
		// pixels to their displayed size, including high-DPI screens.
		side := max(a.settings.Width, a.settings.Height, 1180, 840)
		request = backgroundRequest{theme.ImagePath, int(math.Ceil(float64(side) * max(a.scaleFactor, 1)))}
	}
	loader := &a.backgroundImage
	if request == loader.request {
		return
	}
	loader.request = request
	loader.generation++
	if loader.cancel != nil {
		loader.cancel()
		loader.cancel = nil
	}
	a.background = nil
	if request.path == "" || a.panel == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	loader.cancel = cancel
	generation, cached := loader.generation, loader.cache
	go func() {
		defer cancel()
		loaded, err := readBackground(ctx, request, cached)
		if ctx.Err() != nil {
			return
		}
		a.panel.Update(func() {
			if a.quitting || generation != a.backgroundImage.generation {
				return
			}
			a.backgroundImage.cancel = nil
			if err != nil {
				log.Printf("background: %v", err)
				return
			}
			a.backgroundImage.cache = loaded
			a.background = loaded.bitmap
			if a.settingsWin != nil {
				a.settingsWin.Invalidate()
			}
			if a.workspaceWin != nil {
				a.workspaceWin.Invalidate()
			}
		})
	}()
}

func readBackground(ctx context.Context, request backgroundRequest, cached backgroundCache) (backgroundCache, error) {
	if err := ctx.Err(); err != nil {
		return backgroundCache{}, err
	}
	info, err := os.Stat(request.path)
	if err != nil {
		return backgroundCache{}, err
	}
	if cached.bitmap != nil && cached.request == request && cached.size == info.Size() && cached.modified.Equal(info.ModTime()) {
		return cached, nil
	}
	data, err := os.ReadFile(request.path)
	if err != nil {
		return backgroundCache{}, err
	}
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return backgroundCache{}, err
	}
	if err := ctx.Err(); err != nil {
		return backgroundCache{}, err
	}
	// MyGo applies JPEG EXIF orientation. Keep that decoder for photos;
	// resizing their raw pixels here would silently rotate camera images.
	if format == "jpeg" {
		bitmap, err := ui.DecodeBitmap(data)
		return backgroundCache{request, info.ModTime(), info.Size(), bitmap}, err
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return backgroundCache{}, err
	}
	bounds := src.Bounds()
	if side := max(bounds.Dx(), bounds.Dy()); side > request.maxPixels && request.maxPixels > 0 {
		ratio := float64(request.maxPixels) / float64(side)
		resized := image.NewNRGBA(image.Rect(0, 0, max(1, int(float64(bounds.Dx())*ratio)), max(1, int(float64(bounds.Dy())*ratio))))
		draw.ApproxBiLinear.Scale(resized, resized.Bounds(), src, bounds, draw.Src, nil)
		src = resized
	}
	if err := ctx.Err(); err != nil {
		return backgroundCache{}, err
	}
	return backgroundCache{request, info.ModTime(), info.Size(), ui.NewBitmap(src)}, nil
}
