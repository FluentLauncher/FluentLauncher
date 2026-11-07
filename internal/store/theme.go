package store

import (
	"archive/zip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"fluentlauncher/internal/core/settings"
	"fluentlauncher/locales"
)

const maxThemeImageBytes = 30 << 20

var themeImageExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".bmp": true, ".webp": true}

// ThemeStore exports and imports the zip format used by the C# launcher:
// theme.json plus at most one static image.
type ThemeStore struct {
	dir string
}

func NewThemeStore(dir string) *ThemeStore {
	return &ThemeStore{dir: dir}
}

func (s *ThemeStore) Export(theme settings.ThemeSettings, destination string) error {
	if err := settings.ValidateTheme(theme); err != nil {
		return err
	}
	packaged := theme
	if theme.Background == settings.BackgroundImage {
		ext := strings.ToLower(filepath.Ext(theme.ImagePath))
		if !themeImageExt[ext] {
			return errors.New(locales.StoreThemeImageFormatUnsupported())
		}
		info, err := os.Stat(theme.ImagePath)
		if err != nil || info.Size() > maxThemeImageBytes {
			return errors.New(locales.StoreThemeImageInvalidOrTooLarge())
		}
		packaged.ImagePath = "background" + ext
	}
	file, err := os.Create(destination)
	if err != nil {
		return err
	}
	archive := zip.NewWriter(file)
	entry, err := archive.Create("theme.json")
	if err != nil {
		file.Close()
		return err
	}
	if err := json.NewEncoder(entry).Encode(packaged); err != nil {
		file.Close()
		return err
	}
	if theme.Background == settings.BackgroundImage {
		input, err := os.Open(theme.ImagePath)
		if err != nil {
			file.Close()
			return err
		}
		defer input.Close()
		image, err := archive.Create(packaged.ImagePath)
		if err != nil {
			file.Close()
			return err
		}
		if _, err := io.CopyN(image, input, maxThemeImageBytes+1); err != nil && !errors.Is(err, io.EOF) {
			file.Close()
			return err
		}
	}
	if err := archive.Close(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func (s *ThemeStore) Import(source string) (settings.ThemeSettings, error) {
	archive, err := zip.OpenReader(source)
	if err != nil {
		return settings.ThemeSettings{}, err
	}
	defer archive.Close()
	if len(archive.File) < 1 || len(archive.File) > 2 {
		return settings.ThemeSettings{}, errors.New(locales.StoreThemePackageEntries())
	}
	var theme settings.ThemeSettings
	theme.ImagePath = ""
	var imageEntry *zip.File
	for _, entry := range archive.File {
		if entry.Name == "theme.json" {
			if entry.UncompressedSize64 > 65536 {
				return settings.ThemeSettings{}, errors.New(locales.StoreThemeJSONTooLarge())
			}
			reader, err := entry.Open()
			if err != nil {
				return settings.ThemeSettings{}, err
			}
			err = json.NewDecoder(reader).Decode(&theme)
			reader.Close()
			if err != nil {
				return settings.ThemeSettings{}, err
			}
			continue
		}
		imageEntry = entry
	}
	if theme.Background != settings.BackgroundImage {
		if len(archive.File) != 1 {
			return settings.ThemeSettings{}, errors.New(locales.StoreThemeNonImageExtraFiles())
		}
		if err := settings.ValidateTheme(theme); err != nil {
			return settings.ThemeSettings{}, err
		}
		return theme, nil
	}
	name := theme.ImagePath
	if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) ||
		!themeImageExt[strings.ToLower(filepath.Ext(name))] {
		return settings.ThemeSettings{}, errors.New(locales.StoreThemeImageNameInvalid())
	}
	if imageEntry == nil || imageEntry.Name != name {
		return settings.ThemeSettings{}, errors.New(locales.StoreThemeImageMissing())
	}
	if imageEntry.UncompressedSize64 > maxThemeImageBytes {
		return settings.ThemeSettings{}, errors.New(locales.StoreThemeImageTooLarge())
	}
	directory := filepath.Join(s.dir, randomName())
	destination := filepath.Join(directory, "background"+strings.ToLower(filepath.Ext(name)))
	theme.ImagePath = destination
	if err := settings.ValidateTheme(theme); err != nil {
		return settings.ThemeSettings{}, err
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return settings.ThemeSettings{}, err
	}
	input, err := imageEntry.Open()
	if err != nil {
		os.RemoveAll(directory)
		return settings.ThemeSettings{}, err
	}
	defer input.Close()
	output, err := os.Create(destination)
	if err != nil {
		os.RemoveAll(directory)
		return settings.ThemeSettings{}, err
	}
	written, copyErr := io.Copy(output, io.LimitReader(input, maxThemeImageBytes+1))
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil {
		os.RemoveAll(directory)
		if copyErr != nil {
			return settings.ThemeSettings{}, copyErr
		}
		return settings.ThemeSettings{}, closeErr
	}
	if written > maxThemeImageBytes {
		os.RemoveAll(directory)
		return settings.ThemeSettings{}, errors.New(locales.StoreThemeImageTooLarge())
	}
	return theme, nil
}

func randomName() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "theme"
	}
	return hex.EncodeToString(b[:8])
}
