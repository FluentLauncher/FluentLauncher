// Repack MyGo's signed update as a single EXE. Runtime DLLs are embedded in
// that EXE; official plugin packages are distributed independently.
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	dir := flag.String("dir", "build/release/windows-amd64", "MyGo build directory")
	flag.Parse()
	if err := repack(*dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func repack(dir string) error {
	manifestPath := filepath.Join(dir, "update-windows-amd64.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	var manifest struct {
		Version   string `json:"version"`
		Notes     string `json:"notes,omitempty"`
		Date      string `json:"date,omitempty"`
		URL       string `json:"url"`
		Size      int64  `json:"size"`
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	private, err := base64.StdEncoding.DecodeString(strings.TrimSpace(os.Getenv("MYGO_UPDATER_PRIVATE_KEY")))
	if err != nil || len(private) != ed25519.PrivateKeySize {
		return fmt.Errorf("invalid update private key")
	}
	public, err := base64.StdEncoding.DecodeString(strings.TrimSpace(os.Getenv("MYGO_UPDATER_PUBLIC_KEY")))
	if err != nil || !ed25519.PrivateKey(private).Public().(ed25519.PublicKey).Equal(ed25519.PublicKey(public)) {
		return fmt.Errorf("update signing keys do not match")
	}
	archive := filepath.Join(dir, "fluent-launcher-"+manifest.Version+"-windows-amd64.tar.gz")
	if err := writeArchive(archive, filepath.Join(dir, "FluentLauncher.exe")); err != nil {
		return err
	}
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	sum := sha256.New()
	manifest.Size, err = io.Copy(sum, f)
	f.Close()
	if err != nil {
		return err
	}
	manifest.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(private, sum.Sum(nil)))
	data, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(manifestPath, append(data, '\n'), 0o644)
}

func writeArchive(path, executable string) error {
	src, err := os.Open(executable)
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	dst, err := os.Create(path)
	if err != nil {
		return err
	}
	defer dst.Close()
	gz := gzip.NewWriter(dst)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "FluentLauncher.exe", Mode: 0o755, Size: info.Size()}); err != nil {
		return err
	}
	if _, err := io.Copy(tw, src); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return dst.Close()
}
