package plugin

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"fluentlauncher/internal/plugin/protocol"
	"fluentlauncher/locales"
)

const (
	maxPackageEntries = 512
	maxPackageBytes   = 100 << 20
	maxPackageFile    = 30 << 20
)

// Problem is one plugin package the launcher could not load. The settings
// page lists these one by one: a single line of joined warnings hid every
// broken package but the last one.
type Problem struct {
	// Directory is the package directory name under the plugin root.
	Directory string
	// Message says what is wrong in terms the user can act on.
	Message string
}

// Repository installs and enumerates plugin packages under the user's
// plugin directory.
type Repository struct {
	dir       string
	mu        sync.Mutex
	installed []protocol.Registration
	problems  []Problem
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

// Problems returns the packages that could not be loaded, sorted by directory.
func (r *Repository) Problems() []Problem {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Problem(nil), r.problems...)
}

func (r *Repository) Refresh() error {
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return errors.New(locales.PluginDirCreateFailed(err))
	}
	entries, err := os.ReadDir(r.dir)
	if errors.Is(err, os.ErrNotExist) {
		r.mu.Lock()
		r.installed = nil
		r.problems = nil
		r.mu.Unlock()
		return nil
	}
	if err != nil {
		return errors.New(locales.PluginDirReadFailed(err))
	}
	installed := make([]protocol.Registration, 0)
	problems := make([]Problem, 0)
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			// A leftover staging directory from an interrupted install holds a
			// valid plugin.json, and must not be listed as an installed plugin.
			continue
		}
		registration, err := protocol.ReadManifest(filepath.Join(r.dir, entry.Name()))
		if err != nil {
			problems = append(problems, Problem{Directory: entry.Name(), Message: err.Error()})
			continue
		}
		if registration.Manifest.Executable == "" {
			problems = append(problems, Problem{
				Directory: entry.Name(),
				Message:   locales.PluginLegacyPackage(),
			})
			continue
		}
		if !strings.EqualFold(registration.Manifest.ID, entry.Name()) {
			// The plugin still runs from its own directory, and uninstalling
			// goes through the registration, so it stays usable; the user is
			// told because the folder no longer matches the package.
			problems = append(problems, Problem{
				Directory: entry.Name(),
				Message:   locales.PluginDirIDMismatch(registration.Manifest.ID),
			})
		}
		installed = append(installed, registration)
	}
	sort.Slice(installed, func(i, j int) bool { return installed[i].Manifest.ID < installed[j].Manifest.ID })
	sort.Slice(problems, func(i, j int) bool { return problems[i].Directory < problems[j].Directory })
	r.mu.Lock()
	r.installed = installed
	r.problems = problems
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
		return locales.PluginUpdateInstall(c.Package.Name, c.Package.Version)
	}
	switch compareVersion(c.Package.Version, c.Installed.Version) {
	case 1:
		return locales.PluginUpdateUpgrade(c.Installed.Version, c.Package.Version)
	case 0:
		return locales.PluginUpdateReinstall(c.Installed.Version)
	default:
		return locales.PluginUpdateDowngrade(c.Package.Version, c.Installed.Version)
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
		return protocol.Registration{}, errors.New(locales.PluginAlreadyInstalled(manifest.ID))
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
		return protocol.Registration{}, errors.New(locales.PluginPackageIDMismatch())
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

// Uninstall removes the package directory of one plugin. The directory is
// taken from the registration, not rebuilt from the id: a package whose folder
// name and manifest id differ still uninstalls, which is what the settings page
// promises when it shows the plugin.
func (r *Repository) Uninstall(id string) error {
	if !protocol.ValidID(id) {
		return errors.New(locales.PluginInvalidID())
	}
	directory := filepath.Join(r.dir, id)
	for _, registration := range r.Installed() {
		if strings.EqualFold(registration.Manifest.ID, id) {
			directory = registration.Directory
			break
		}
	}
	if !inside(r.dir, directory) {
		return errors.New(locales.PluginDirOutsideRoot())
	}
	if err := os.RemoveAll(directory); err != nil {
		return errors.New(locales.PluginDirDeleteFailed(filepath.Base(directory), err))
	}
	return r.Refresh()
}

func readPackageManifest(source string) (protocol.Manifest, error) {
	info, err := os.Stat(source)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return protocol.Manifest{}, errors.New(locales.PluginPackageNotFound(source))
		}
		return protocol.Manifest{}, errors.New(locales.PluginPackageReadFailed(err))
	}
	if info.IsDir() {
		// ReadManifest checks the entry too, so a directory without the
		// executable it names is refused here instead of after the copy.
		registration, err := protocol.ReadManifest(source)
		if err != nil {
			return protocol.Manifest{}, err
		}
		return registration.Manifest, nil
	}
	archive, err := zip.OpenReader(source)
	if err != nil {
		return protocol.Manifest{}, errors.New(locales.PluginPackageNotZip(err))
	}
	defer archive.Close()
	if len(archive.File) < 1 || len(archive.File) > maxPackageEntries {
		return protocol.Manifest{}, errors.New(locales.PluginPackageEntryCount(len(archive.File)))
	}
	for _, file := range archive.File {
		if file.Name != "plugin.json" {
			continue
		}
		if file.UncompressedSize64 > 1<<20 {
			return protocol.Manifest{}, errors.New(locales.PluginManifestTooLarge())
		}
		reader, err := file.Open()
		if err != nil {
			return protocol.Manifest{}, errors.New(locales.PluginManifestOpenFailed(err))
		}
		var manifest protocol.Manifest
		err = json.NewDecoder(reader).Decode(&manifest)
		reader.Close()
		if err != nil {
			return protocol.Manifest{}, errors.New(locales.PluginManifestInvalidJSON(err))
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
			return protocol.Manifest{}, errors.New(locales.PluginEntryNotInPackage(manifest.Entry()))
		}
		return manifest, nil
	}
	return protocol.Manifest{}, errors.New(locales.PluginPackageMissingManifest())
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
		return errors.New(locales.PluginPackageTooManyEntries())
	}
	var total int64
	for _, file := range archive.File {
		if file.Mode()&os.ModeSymlink != 0 {
			return errors.New(locales.PluginPackageSymlink())
		}
		name := filepath.Clean(file.Name)
		if filepath.IsAbs(name) || strings.Contains(name, "..") || strings.Contains(name, ":") {
			return errors.New(locales.PluginPackageInvalidPath(file.Name))
		}
		target := filepath.Join(destination, name)
		if !inside(destination, target) {
			return errors.New(locales.PluginPackagePathOutside())
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if file.UncompressedSize64 > maxPackageFile {
			return errors.New(locales.PluginPackageFileTooLarge())
		}
		total += int64(file.UncompressedSize64)
		if total > maxPackageBytes {
			return errors.New(locales.PluginPackageSizeLimit())
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
