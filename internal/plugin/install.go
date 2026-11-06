package plugin

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/mistquay/fluentlauncher/internal/plugin/protocol"
)

const (
	maxPackageEntries = 512
	maxPackageBytes   = 100 << 20
	maxPackageFile    = 30 << 20
)

// Repository installs and enumerates plugin packages under the user's
// plugin directory.
type Repository struct {
	dir       string
	mu        sync.Mutex
	installed []protocol.Registration
	warning   string
}

func NewRepository(dir string) *Repository {
	return &Repository{dir: dir}
}

func (r *Repository) Dir() string { return r.dir }

func (r *Repository) Installed() []protocol.Registration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]protocol.Registration(nil), r.installed...)
}

func (r *Repository) Warning() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.warning
}

func (r *Repository) Refresh() error {
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(r.dir)
	if errors.Is(err, os.ErrNotExist) {
		r.mu.Lock()
		r.installed = nil
		r.mu.Unlock()
		return nil
	}
	if err != nil {
		return err
	}
	installed := make([]protocol.Registration, 0)
	var warning string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		registration, err := protocol.ReadManifest(filepath.Join(r.dir, entry.Name()))
		if err != nil {
			warning = "插件包无效 " + entry.Name() + "：" + err.Error()
			continue
		}
		if registration.Manifest.Executable == "" {
			warning = "跳过旧版 C# 插件 " + entry.Name() + "；请安装可执行插件包"
			continue
		}
		installed = append(installed, registration)
	}
	sort.Slice(installed, func(i, j int) bool { return installed[i].Manifest.ID < installed[j].Manifest.ID })
	r.mu.Lock()
	r.installed = installed
	r.warning = warning
	r.mu.Unlock()
	return nil
}

type UpdateCheck struct {
	Package   protocol.Manifest
	Installed *protocol.Manifest
}

func (c UpdateCheck) IsUpgrade() bool { return c.Installed != nil }

func (c UpdateCheck) IsNewer() bool {
	return c.Installed == nil || compareVersion(c.Package.Version, c.Installed.Version) > 0
}

func (c UpdateCheck) Describe() string {
	if c.Installed == nil {
		return fmt.Sprintf("将安装 %s %s。", c.Package.Name, c.Package.Version)
	}
	switch compareVersion(c.Package.Version, c.Installed.Version) {
	case 1:
		return fmt.Sprintf("将从 %s 升级到 %s。", c.Installed.Version, c.Package.Version)
	case 0:
		return fmt.Sprintf("包内版本与已安装的 %s 相同，将重新安装。", c.Installed.Version)
	default:
		return fmt.Sprintf("包内版本 %s 低于已安装的 %s，继续将降级。", c.Package.Version, c.Installed.Version)
	}
}

func (r *Repository) CheckUpdate(source string) (UpdateCheck, error) {
	manifest, err := readPackageManifest(source)
	if err != nil {
		return UpdateCheck{}, err
	}
	var installed *protocol.Manifest
	for _, registration := range r.Installed() {
		if registration.Manifest.ID == manifest.ID {
			copy := registration.Manifest
			installed = &copy
			break
		}
	}
	return UpdateCheck{Package: manifest, Installed: installed}, nil
}

// Install extracts a zip or copies a directory into the plugin repository.
// It rejects path traversal, absolute paths, symlinks and oversized entries.
func (r *Repository) Install(source string, upgrade bool) (protocol.Registration, error) {
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return protocol.Registration{}, err
	}
	manifest, err := readPackageManifest(source)
	if err != nil {
		return protocol.Registration{}, err
	}
	target := filepath.Join(r.dir, manifest.ID)
	if _, err := os.Stat(target); err == nil && !upgrade {
		return protocol.Registration{}, fmt.Errorf("插件 %s 已安装；请先卸载或使用升级", manifest.ID)
	}
	staging, err := os.MkdirTemp(r.dir, ".staging-*")
	if err != nil {
		return protocol.Registration{}, err
	}
	defer os.RemoveAll(staging)
	if err := extractPackage(source, staging); err != nil {
		return protocol.Registration{}, err
	}
	registration, err := protocol.ReadManifest(staging)
	if err != nil {
		return protocol.Registration{}, err
	}
	if registration.Manifest.ID != manifest.ID {
		return protocol.Registration{}, fmt.Errorf("包内标识不一致")
	}
	backup := target + ".old"
	_ = os.RemoveAll(backup)
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, backup); err != nil {
			return protocol.Registration{}, err
		}
	}
	if err := os.Rename(staging, target); err != nil {
		if _, statErr := os.Stat(backup); statErr == nil {
			_ = os.Rename(backup, target)
		}
		return protocol.Registration{}, err
	}
	_ = os.RemoveAll(backup)
	if err := r.Refresh(); err != nil {
		return protocol.Registration{}, err
	}
	return protocol.ReadManifest(target)
}

func (r *Repository) Uninstall(id string) error {
	if !safeID(id) {
		return fmt.Errorf("插件标识无效")
	}
	target := filepath.Join(r.dir, id)
	if !inside(r.dir, target) {
		return fmt.Errorf("插件路径超出目录")
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return r.Refresh()
}

func readPackageManifest(source string) (protocol.Manifest, error) {
	info, err := os.Stat(source)
	if err != nil {
		return protocol.Manifest{}, err
	}
	if info.IsDir() {
		data, err := os.ReadFile(filepath.Join(source, "plugin.json"))
		if err != nil {
			return protocol.Manifest{}, err
		}
		var manifest protocol.Manifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return protocol.Manifest{}, err
		}
		if err := manifest.Validate(); err != nil {
			return protocol.Manifest{}, err
		}
		return manifest, nil
	}
	archive, err := zip.OpenReader(source)
	if err != nil {
		return protocol.Manifest{}, err
	}
	defer archive.Close()
	if len(archive.File) < 1 || len(archive.File) > maxPackageEntries {
		return protocol.Manifest{}, fmt.Errorf("插件包条目数量无效")
	}
	for _, file := range archive.File {
		if file.Name != "plugin.json" {
			continue
		}
		if file.UncompressedSize64 > 1<<20 {
			return protocol.Manifest{}, fmt.Errorf("插件清单过大")
		}
		reader, err := file.Open()
		if err != nil {
			return protocol.Manifest{}, err
		}
		var manifest protocol.Manifest
		err = json.NewDecoder(reader).Decode(&manifest)
		reader.Close()
		if err != nil {
			return protocol.Manifest{}, err
		}
		if err := manifest.Validate(); err != nil {
			return protocol.Manifest{}, err
		}
		found := false
		for _, entry := range archive.File {
			if entry.Name == manifest.Entry() {
				found = true
				break
			}
		}
		if !found {
			return protocol.Manifest{}, fmt.Errorf("插件入口不存在")
		}
		return manifest, nil
	}
	return protocol.Manifest{}, fmt.Errorf("插件包缺少 plugin.json")
}

func extractPackage(source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDirectory(source, destination)
	}
	archive, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer archive.Close()
	if len(archive.File) > maxPackageEntries {
		return fmt.Errorf("插件包条目过多")
	}
	var total int64
	for _, file := range archive.File {
		if file.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("插件包不能包含符号链接")
		}
		name := filepath.Clean(file.Name)
		if filepath.IsAbs(name) || strings.Contains(name, "..") || strings.Contains(name, ":") {
			return fmt.Errorf("插件包路径无效：%s", file.Name)
		}
		target := filepath.Join(destination, name)
		if !inside(destination, target) {
			return fmt.Errorf("插件包路径超出目录")
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if file.UncompressedSize64 > maxPackageFile {
			return fmt.Errorf("插件包单个文件过大")
		}
		total += int64(file.UncompressedSize64)
		if total > maxPackageBytes {
			return fmt.Errorf("插件包解压后超过大小上限")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		input, err := file.Open()
		if err != nil {
			return err
		}
		output, err := os.Create(target)
		if err != nil {
			input.Close()
			return err
		}
		_, copyErr := io.Copy(output, io.LimitReader(input, maxPackageFile+1))
		input.Close()
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func copyDirectory(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.Create(target)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func inside(root, path string) bool {
	root = strings.ToLower(filepath.Clean(root))
	path = strings.ToLower(filepath.Clean(path))
	return path == root || strings.HasPrefix(path, root+string(os.PathSeparator))
}

func safeID(id string) bool {
	if id == "" || len(id) > 100 || id == "." || id == ".." {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func compareVersion(a, b string) int {
	left := strings.Split(a, ".")
	right := strings.Split(b, ".")
	for i := 0; i < max(len(left), len(right)); i++ {
		var x, y int
		if i < len(left) {
			x, _ = strconv.Atoi(left[i])
		}
		if i < len(right) {
			y, _ = strconv.Atoi(right[i])
		}
		if x != y {
			if x > y {
				return 1
			}
			return -1
		}
	}
	return 0
}
