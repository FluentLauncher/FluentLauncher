// Isolated acceptance process: exercise MyGo's actual GitHub release selection,
// signatures and Windows EXE replacement against a local HTTPS fixture.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/netproxy"

	"fluentlauncher/internal/core/settings"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	dir := os.Args[1]
	data, err := os.ReadFile(filepath.Join(dir, "update-windows-amd64.json"))
	must(err)
	var manifest struct{ Version, URL, Notes string }
	must(json.Unmarshal(data, &manifest))
	if !strings.HasPrefix(manifest.Version, "0.1.1-nightly.") || manifest.Notes == "" {
		panic("acceptance requires a signed 0.1.1-nightly build with notes")
	}
	archiveURL, err := url.Parse(manifest.URL)
	must(err)
	archive, err := os.ReadFile(filepath.Join(dir, filepath.Base(archiveURL.Path)))
	must(err)
	var stable atomic.Value
	stable.Store("0.1.0")
	var corrupt atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stableVersion := stable.Load().(string)
		switch {
		case strings.HasPrefix(r.URL.Path, "/repos/"):
			fmt.Fprintf(w, `[{"tag_name":"v%s","draft":false,"prerelease":true},{"tag_name":"v%s","draft":false,"prerelease":false}]`, manifest.Version, stableVersion)
		case strings.HasSuffix(r.URL.Path, "/update-windows-amd64.json"):
			if strings.Contains(r.URL.Path, "/v"+manifest.Version+"/") {
				_, _ = w.Write(data)
			} else {
				fmt.Fprintf(w, `{"version":%q,"url":%q,"signature":"test"}`, stableVersion, manifest.URL)
			}
		case r.URL.Path == archiveURL.Path:
			payload := append([]byte(nil), archive...)
			if corrupt.Load() {
				payload[len(payload)/2] ^= 1
			}
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL, err := url.Parse(server.URL)
	must(err)
	transport := netproxy.Transport()
	transport.Proxy = nil
	transport.TLSClientConfig = server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	transport.TLSClientConfig.ServerName = "example.com"
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, serverURL.Host)
	}
	check := func(current, channel, want string) *mygo.Update {
		mygo.App.SetVersion(current)
		s := settings.Default()
		s.UpdateChannel = channel
		mygo.Updater.SetPrereleases(s.EffectiveUpdateChannel(current) == settings.UpdateChannelNightly)
		up, err := mygo.Updater.Check(context.Background())
		must(err)
		got := ""
		if up != nil {
			got = up.Version
		}
		if got != want {
			panic(fmt.Sprintf("%s on %s: got %q, want %q", channel, current, got, want))
		}
		return up
	}
	check("0.1.0", settings.UpdateChannelStable, "")
	check("0.1.0", settings.UpdateChannelNightly, manifest.Version)
	check("0.1.1-nightly.20260101.1.1", settings.UpdateChannelNightly, manifest.Version)
	check(manifest.Version, settings.UpdateChannelDefault, "")
	check(manifest.Version, settings.UpdateChannelStable, "")
	stable.Store("0.1.1")
	check(manifest.Version, settings.UpdateChannelStable, "0.1.1")
	check(manifest.Version, settings.UpdateChannelNightly, "0.1.1")
	fmt.Println("PASS: stable excludes nightly; preview advances; stable supersedes nightly; switching back never downgrades")
	stable.Store("0.1.0")
	up := check("0.1.0", settings.UpdateChannelNightly, manifest.Version)
	exe, err := os.Executable()
	must(err)
	before, err := os.ReadFile(exe)
	must(err)
	sentinel := filepath.Join(filepath.Dir(exe), "keep-personal-file.txt")
	must(os.WriteFile(sentinel, []byte("preserve"), 0600))
	corrupt.Store(true)
	if err := up.Install(context.Background(), nil); err == nil {
		panic("tampered nightly was accepted")
	}
	after, err := os.ReadFile(exe)
	must(err)
	if sha256.Sum256(before) != sha256.Sum256(after) {
		panic("failed update changed the executable")
	}
	corrupt.Store(false)
	must(up.Install(context.Background(), nil))
	after, err = os.ReadFile(exe)
	must(err)
	want, err := os.ReadFile(filepath.Join(dir, "FluentLauncher.exe"))
	must(err)
	if sha256.Sum256(after) != sha256.Sum256(want) {
		panic("installed nightly differs from the signed release")
	}
	kept, err := os.ReadFile(sentinel)
	must(err)
	if string(kept) != "preserve" {
		panic("personal file was changed")
	}
	fmt.Println("PASS: tampered nightly rejected; signed nightly replaced the isolated Windows EXE; personal file preserved")
}
