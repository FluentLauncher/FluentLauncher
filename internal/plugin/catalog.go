package plugin

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/netproxy"

	"fluentlauncher/internal/plugin/protocol"
)

// Injected by the release build. The catalogue has its own signing key,
// independent of the launcher's executable updates.
var OfficialRepository = "FluentLauncher/FluentLauncher-plugins"
var OfficialPublicKey string

type Catalog struct {
	SchemaVersion int            `json:"schemaVersion"`
	Plugins       []CatalogEntry `json:"plugins"`
}

type CatalogEntry struct {
	HostServices         []string                      `json:"hostServices,omitempty"`
	ID                   string                        `json:"id"`
	Version              string                        `json:"version"`
	ProtocolVersion      int                           `json:"protocolVersion"`
	Name                 map[string]string             `json:"name"`
	Summary              map[string]string             `json:"summary"`
	Platforms            map[string]PluginPackage      `json:"platforms"`
	Author               string                        `json:"author,omitempty"`
	Description          map[string]string             `json:"description,omitempty"`
	Usage                map[string]string             `json:"usage,omitempty"`
	ReleaseNotes         map[string]string             `json:"releaseNotes,omitempty"`
	Homepage             string                        `json:"homepage,omitempty"`
	Icon                 *PluginPackage                `json:"icon,omitempty"`
	RequiredHostServices []protocol.ServiceRequirement `json:"requiredHostServices,omitempty"`
}

type PluginPackage struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

func (c *Catalog) Localized() []OfficialPlugin {
	plugins := make([]OfficialPlugin, 0, len(c.Plugins))
	for _, entry := range c.Plugins {
		plugins = append(plugins, OfficialPlugin{
			ID: entry.ID, Name: officialText(entry.Name, i18n.Locale()),
			Summary: officialText(entry.Summary, i18n.Locale()), Version: entry.Version,
			Package: entry.Platforms["windows-amd64"],
			Author:  entry.Author, Description: officialText(entry.Description, i18n.Locale()),
			Usage: officialText(entry.Usage, i18n.Locale()), Homepage: entry.Homepage,
			ReleaseNotes: officialText(entry.ReleaseNotes, i18n.Locale()), Icon: entry.Icon,
			RequiredHostServices: entry.RequiredHostServices,
			HostServices:         entry.HostServices,
		})
	}
	return plugins
}

func (p OfficialPlugin) NewerThan(installed string) bool {
	return p.Version != "" && compareVersion(p.Version, installed) > 0
}

type CatalogClient struct {
	Repository string
	PublicKey  string
	CachePath  string
	HTTP       *http.Client
}

func NewCatalogClient(cache string) *CatalogClient {
	return &CatalogClient{Repository: OfficialRepository, PublicKey: OfficialPublicKey,
		CachePath: cache, HTTP: &http.Client{Transport: netproxy.Transport()}}
}

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:\.[0-9]+)?$`)

func (c *CatalogClient) Configured() bool {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(c.PublicKey))
	return err == nil && len(key) == ed25519.PublicKeySize && repositoryPattern.MatchString(c.Repository)
}

type cachedCatalog struct {
	Data      []byte `json:"data"`
	Signature string `json:"signature"`
}

// Cached re-verifies the cached catalogue, including after a key/source change.
// Call from a worker; never read cache files while building the settings view.
func (c *CatalogClient) Cached() (*Catalog, error) {
	data, err := os.ReadFile(c.CachePath)
	if err != nil {
		return nil, err
	}
	if len(data) > 2<<20 {
		return nil, errors.New("plugin catalogue cache is too large")
	}
	var cached cachedCatalog
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, err
	}
	return c.verify(cached.Data, cached.Signature)
}

// Refresh fetches metadata only. Executables are downloaded exclusively by
// Download when the user explicitly chooses an installation or upgrade.
func (c *CatalogClient) Refresh(ctx context.Context) (*Catalog, error) {
	if !c.Configured() {
		return nil, errors.New("official plugin repository signing key is not configured")
	}
	base := "https://github.com/" + c.Repository + "/releases/latest/download/"
	data, err := c.get(ctx, base+"catalog.json", 1<<20)
	if err != nil {
		return nil, err
	}
	signature, err := c.get(ctx, base+"catalog.sig", 1024)
	if err != nil {
		return nil, err
	}
	catalog, err := c.verify(data, string(signature))
	if err != nil {
		return nil, err
	}
	// RawMessage can be re-indented by encoding/json, which would invalidate
	// the signature. Cache an exact byte string in base64 instead of rewriting it.
	cacheData, err := json.Marshal(cachedCatalog{data, string(signature)})
	if err != nil {
		return nil, err
	}
	if err := writeCatalogCache(c.CachePath, cacheData); err != nil {
		return nil, err
	}
	return catalog, nil
}

func (c *CatalogClient) verify(data []byte, signature string) (*Catalog, error) {
	if !c.Configured() {
		return nil, errors.New("official plugin repository signing key is not configured")
	}
	key, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(c.PublicKey))
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(signature))
	digest := sha256.Sum256(data)
	if err != nil || !ed25519.Verify(key, digest[:], sig) {
		return nil, errors.New("official plugin catalogue signature is invalid")
	}
	var catalog Catalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, err
	}
	if catalog.SchemaVersion != 1 || len(catalog.Plugins) > 200 {
		return nil, errors.New("unsupported official plugin catalogue")
	}
	seen := map[string]bool{}
	for _, entry := range catalog.Plugins {
		asset, ok := entry.Platforms["windows-amd64"]
		id := strings.ToLower(entry.ID)
		if !protocol.ValidID(entry.ID) || seen[id] || !versionPattern.MatchString(entry.Version) || entry.ProtocolVersion != protocol.ProtocolVersion || entry.Name["zh-Hans"] == "" || !ok {
			return nil, fmt.Errorf("invalid official plugin entry: %s", entry.ID)
		}
		if err := c.validatePackage(asset); err != nil {
			return nil, err
		}
		if entry.Icon != nil {
			if err := c.validateAsset(*entry.Icon, ".png", 1<<20); err != nil {
				return nil, err
			}
		}
		seen[id] = true
	}
	return &catalog, nil
}

func (c *CatalogClient) validatePackage(asset PluginPackage) error {
	return c.validateAsset(asset, ".zip", maxPackageBytes)
}

func (c *CatalogClient) validateAsset(asset PluginPackage, extension string, maxSize int64) error {
	u, err := url.Parse(asset.URL)
	prefix := "/" + c.Repository + "/releases/download/"
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, prefix) || len(strings.Split(strings.TrimPrefix(u.Path, prefix), "/")) != 2 || !strings.HasSuffix(u.Path, extension) {
		return errors.New("official plugin asset must use a fixed release URL in its repository")
	}
	sum, err := hex.DecodeString(asset.SHA256)
	if err != nil || len(sum) != sha256.Size || asset.Size <= 0 || asset.Size > maxSize {
		return errors.New("invalid official plugin package checksum or size")
	}
	return nil
}

// Download verifies the archive and manifest before touching an installed
// plugin. The caller owns destination and can remove it after installation.
func (c *CatalogClient) Download(ctx context.Context, entry OfficialPlugin, destination string) error {
	return c.DownloadWithProgress(ctx, entry, destination, nil)
}

// DownloadWithProgress streams a bounded archive to disk. The callback runs on
// the caller's worker and is throttled; it never touches UI state directly.
func (c *CatalogClient) DownloadWithProgress(ctx context.Context, entry OfficialPlugin, destination string, progress func(int64, int64)) error {
	if err := c.validatePackage(entry.Package); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, entry.Package.URL, nil)
	if err != nil {
		return err
	}
	response, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("official plugin download: HTTP %d", response.StatusCode)
	}
	f, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	hash := sha256.New()
	reader := io.LimitReader(response.Body, entry.Package.Size+1)
	buffer := make([]byte, 32<<10)
	var downloaded int64
	last := time.Time{}
	for {
		n, readErr := reader.Read(buffer)
		if n > 0 {
			downloaded += int64(n)
			if downloaded > entry.Package.Size {
				return errors.New("official plugin download exceeds its size limit")
			}
			if _, err := f.Write(buffer[:n]); err != nil {
				return err
			}
			_, _ = hash.Write(buffer[:n])
			if progress != nil && (time.Since(last) >= 100*time.Millisecond || downloaded == entry.Package.Size) {
				progress(downloaded, entry.Package.Size)
				last = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	if downloaded != entry.Package.Size || !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), entry.Package.SHA256) {
		return errors.New("official plugin package checksum or size mismatch")
	}
	manifest, err := readPackageManifest(destination)
	if err != nil {
		return err
	}
	if manifest.ID != entry.ID || manifest.Version != entry.Version || manifest.ProtocolVersion != protocol.ProtocolVersion {
		return errors.New("downloaded plugin identity, version or protocol differs from the catalogue")
	}
	if err := ValidateHostRequirements(manifest); err != nil {
		return err
	}
	return nil
}

// Icon returns only content matching signed metadata, including on cache hits.
func (c *CatalogClient) Icon(ctx context.Context, asset PluginPackage) ([]byte, error) {
	if err := c.validateAsset(asset, ".png", 1<<20); err != nil {
		return nil, err
	}
	path := filepath.Join(filepath.Dir(c.CachePath), "plugin-icons", strings.ToLower(asset.SHA256)+".png")
	valid := func(data []byte) bool {
		sum := sha256.Sum256(data)
		return int64(len(data)) == asset.Size && strings.EqualFold(hex.EncodeToString(sum[:]), asset.SHA256)
	}
	if f, err := os.Open(path); err == nil {
		data, readErr := io.ReadAll(io.LimitReader(f, asset.Size+1))
		_ = f.Close()
		if readErr == nil && valid(data) {
			return data, nil
		}
	}
	data, err := c.get(ctx, asset.URL, asset.Size)
	if err != nil {
		return nil, err
	}
	if !valid(data) {
		return nil, errors.New("plugin icon checksum or size mismatch")
	}
	if err := writeCatalogCache(path, data); err != nil {
		return nil, err
	}
	return data, nil
}

func (c *CatalogClient) get(ctx context.Context, address string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	response, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("official plugin download: HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("official plugin download exceeds its size limit")
	}
	return data, nil
}

func writeCatalogCache(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".catalog-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
