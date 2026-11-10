package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestSingleExecutableUpdateSignature(t *testing.T) {
	dir := t.TempDir()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYGO_UPDATER_PRIVATE_KEY", base64.StdEncoding.EncodeToString(private))
	t.Setenv("MYGO_UPDATER_PUBLIC_KEY", base64.StdEncoding.EncodeToString(public))
	write := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("FluentLauncher.exe", []byte("MZ executable"))
	write("mygo-sqlite3.dll", []byte("must not ship beside the EXE"))
	write("update-windows-amd64.json", []byte(`{"version":"1.2.3","url":"https://example.com/archive.tar.gz","notes":"Notes"}`))
	if err := repack(dir); err != nil {
		t.Fatal(err)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(dir, "update-windows-amd64.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Size             int64
		Signature, Notes string
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	archive, err := os.ReadFile(filepath.Join(dir, "fluent-launcher-1.2.3-windows-amd64.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	signature, err := base64.StdEncoding.DecodeString(manifest.Signature)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(archive)
	if !ed25519.Verify(public, digest[:], signature) || manifest.Size != int64(len(archive)) || manifest.Notes != "Notes" {
		t.Fatal("manifest size, signature or notes mismatch")
	}
	f, err := os.Open(filepath.Join(dir, "fluent-launcher-1.2.3-windows-amd64.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	header, err := tr.Next()
	if err != nil || header.Name != "FluentLauncher.exe" {
		t.Fatalf("unexpected archive entry: %+v, %v", header, err)
	}
	if _, err := tr.Next(); err != io.EOF {
		t.Fatal("update contains sidecars")
	}
	archive[0] ^= 1
	digest = sha256.Sum256(archive)
	if ed25519.Verify(public, digest[:], signature) {
		t.Fatal("tampered archive verified")
	}
	otherPublic, _, _ := ed25519.GenerateKey(rand.Reader)
	t.Setenv("MYGO_UPDATER_PUBLIC_KEY", base64.StdEncoding.EncodeToString(otherPublic))
	if err := repack(dir); err == nil {
		t.Fatal("mismatched key accepted")
	}
}
