package plugin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestIconCacheRequiresTheSignedHashOnEveryRead(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 16, 16))); err != nil {
		t.Fatal(err)
	}
	data := encoded.Bytes()
	sum := sha256.Sum256(data)
	asset := PluginPackage{URL: "https://github.com/owner/plugins/releases/download/v1/icon.png", Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}
	requests := 0
	client := &CatalogClient{Repository: "owner/plugins", CachePath: filepath.Join(t.TempDir(), "catalog.json"), HTTP: &http.Client{Transport: catalogTransport(func(*http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})}}
	for range 2 {
		if _, err := client.Icon(context.Background(), asset); err != nil {
			t.Fatal(err)
		}
	}
	if requests != 1 {
		t.Fatal("valid cache was not reused")
	}
	path := filepath.Join(filepath.Dir(client.CachePath), "plugin-icons", asset.SHA256+".png")
	if err := os.WriteFile(path, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Icon(context.Background(), asset); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatal("tampered cache was accepted")
	}
	asset.URL = "https://example.com/icon.png"
	if _, err := client.Icon(context.Background(), asset); err == nil {
		t.Fatal("foreign asset URL accepted")
	}
}
