package plugin

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type catalogTransport func(*http.Request) (*http.Response, error)

func (f catalogTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestOfficialCatalogMetadataAndOnDemandDownload(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	root := t.TempDir()
	archive := filepath.Join(root, "fixture.zip")
	makePluginZip(t, archive, "echo.exe", false)
	zipBytes, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(zipBytes)
	catalog := Catalog{SchemaVersion: 1, Plugins: []CatalogEntry{{ID: "official.echo", Version: "1.0.0", ProtocolVersion: 1,
		Name: map[string]string{"zh-Hans": "回显示例"}, Summary: map[string]string{"zh-Hans": "示例"},
		Platforms: map[string]PluginPackage{"windows-amd64": {URL: "https://github.com/owner/plugins/releases/download/v1.0.0/echo.zip", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(zipBytes))}}}}}
	data, _ := json.MarshalIndent(catalog, "", "  ")
	digest := sha256.Sum256(data)
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(private, digest[:]))
	packageRequests := 0
	fail, corrupt := false, false
	client := &CatalogClient{Repository: "owner/plugins", PublicKey: base64.StdEncoding.EncodeToString(public), CachePath: filepath.Join(root, "cache.json")}
	client.HTTP = &http.Client{Transport: catalogTransport(func(req *http.Request) (*http.Response, error) {
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
		if fail {
			return nil, errors.New("offline")
		}
		var body []byte
		switch {
		case strings.HasSuffix(req.URL.Path, "catalog.json"):
			body = data
		case strings.HasSuffix(req.URL.Path, "catalog.sig"):
			body = []byte(signature)
		default:
			packageRequests++
			body = append([]byte(nil), zipBytes...)
			if corrupt {
				body[0] ^= 1
			}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
	})}
	verified, err := client.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if packageRequests != 0 {
		t.Fatal("refresh downloaded executable packages")
	}
	cached, err := client.Cached()
	if err != nil || len(cached.Plugins) != 1 {
		t.Fatalf("signed cache: %+v %v", cached, err)
	}
	entry := verified.Localized()[0]
	destination := filepath.Join(root, "download.zip")
	if err := client.Download(context.Background(), entry, destination); err != nil {
		t.Fatal(err)
	}
	if packageRequests != 1 {
		t.Fatal("package was not downloaded on demand")
	}
	repo := NewRepository(filepath.Join(root, "installed"))
	if _, err := repo.Install(destination, false); err != nil {
		t.Fatal(err)
	}
	corrupt = true
	if err := client.Download(context.Background(), entry, destination); err == nil {
		t.Fatal("corrupt package accepted")
	}
	if len(repo.Installed()) != 1 {
		t.Fatal("download failure removed installed plugin")
	}
	corrupt = false
	entry.Version = "2.0.0"
	if err := client.Download(context.Background(), entry, destination); err == nil {
		t.Fatal("manifest version mismatch accepted")
	}
	fail = true
	if _, err := client.Refresh(context.Background()); err == nil {
		t.Fatal("offline refresh succeeded")
	}
	if _, err := client.Cached(); err != nil {
		t.Fatal("failed refresh damaged cached catalogue", err)
	}
	fail = false
	data[0] ^= 1
	if _, err := client.Refresh(context.Background()); err == nil {
		t.Fatal("tampered catalogue accepted")
	}
	data[0] ^= 1
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Refresh(ctx); err == nil {
		t.Fatal("cancelled refresh succeeded")
	}
}

func TestOfficialCatalogRejectsUnsafeEntries(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	client := &CatalogClient{Repository: "owner/plugins", PublicKey: base64.StdEncoding.EncodeToString(public)}
	validAsset := PluginPackage{URL: "https://github.com/owner/plugins/releases/download/v1/echo.zip", SHA256: strings.Repeat("a", 64), Size: 100}
	for _, test := range []struct {
		name   string
		mutate func(*Catalog)
	}{
		{"duplicate", func(c *Catalog) { c.Plugins = append(c.Plugins, c.Plugins[0]) }},
		{"protocol", func(c *Catalog) { c.Plugins[0].ProtocolVersion = 2 }},
		{"id", func(c *Catalog) { c.Plugins[0].ID = "../outside" }},
		{"moving URL", func(c *Catalog) {
			a := validAsset
			a.URL = "https://github.com/owner/plugins/releases/latest/download/echo.zip"
			c.Plugins[0].Platforms["windows-amd64"] = a
		}},
		{"another repo", func(c *Catalog) {
			a := validAsset
			a.URL = "https://github.com/other/plugins/releases/download/v1/echo.zip"
			c.Plugins[0].Platforms["windows-amd64"] = a
		}},
		{"hash", func(c *Catalog) { a := validAsset; a.SHA256 = "bad"; c.Plugins[0].Platforms["windows-amd64"] = a }},
		{"oversized", func(c *Catalog) {
			a := validAsset
			a.Size = maxPackageBytes + 1
			c.Plugins[0].Platforms["windows-amd64"] = a
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := Catalog{SchemaVersion: 1, Plugins: []CatalogEntry{{ID: "official.echo", Version: "1.0.0", ProtocolVersion: 1, Name: map[string]string{"zh-Hans": "Echo"}, Platforms: map[string]PluginPackage{"windows-amd64": validAsset}}}}
			test.mutate(&c)
			data, _ := json.Marshal(c)
			sum := sha256.Sum256(data)
			sig := base64.StdEncoding.EncodeToString(ed25519.Sign(private, sum[:]))
			if _, err := client.verify(data, sig); err == nil {
				t.Fatal("unsafe signed entry accepted")
			}
		})
	}
}
