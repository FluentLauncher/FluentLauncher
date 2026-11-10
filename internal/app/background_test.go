package app

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackgroundResizeCacheAndReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "background.png")
	write := func(w, h int) {
		t.Helper()
		var data bytes.Buffer
		if err := png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, w, h))); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(400, 200)
	request := backgroundRequest{path: path, maxPixels: 100}
	loaded, err := readBackground(context.Background(), request, backgroundCache{})
	if err != nil {
		t.Fatal(err)
	}
	if w, h := loaded.bitmap.Size(); w != 100 || h != 50 {
		t.Fatalf("size %dx%d", w, h)
	}
	reused, err := readBackground(context.Background(), request, loaded)
	if err != nil || reused.bitmap != loaded.bitmap {
		t.Fatal("unchanged background decoded again")
	}
	write(100, 200)
	now := time.Now().Add(time.Second)
	if err := os.Chtimes(path, now, now); err != nil {
		t.Fatal(err)
	}
	replaced, err := readBackground(context.Background(), request, loaded)
	if err != nil {
		t.Fatal(err)
	}
	if replaced.bitmap == loaded.bitmap {
		t.Fatal("changed image reused stale pixels")
	}
	if w, h := replaced.bitmap.Size(); w != 50 || h != 100 {
		t.Fatalf("replaced size %dx%d", w, h)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readBackground(ctx, request, replaced); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestBackgroundPreservesJPEGOrientation(t *testing.T) {
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 20, 10)), nil); err != nil {
		t.Fatal(err)
	}
	// EXIF orientation 6 rotates the camera's landscape pixels to portrait.
	exif := []byte("Exif\x00\x00II\x2a\x00\x08\x00\x00\x00\x01\x00\x12\x01\x03\x00\x01\x00\x00\x00\x06\x00\x00\x00\x00\x00\x00\x00")
	data := append([]byte{0xff, 0xd8, 0xff, 0xe1, 0, byte(len(exif) + 2)}, exif...)
	data = append(data, encoded.Bytes()[2:]...)
	path := filepath.Join(t.TempDir(), "camera.jpg")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := readBackground(context.Background(), backgroundRequest{path, 10}, backgroundCache{})
	if err != nil {
		t.Fatal(err)
	}
	if w, h := loaded.bitmap.Size(); w != 10 || h != 20 {
		t.Fatalf("EXIF orientation lost: %dx%d", w, h)
	}
}
