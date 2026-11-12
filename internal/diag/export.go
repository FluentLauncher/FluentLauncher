package diag

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Export includes only our allowlisted logs and an explicitly constructed
// summary. Raw crash reports and plugin stderr require a separate UI opt-in.
func (r *Recorder) Export(ctx context.Context, destination string, summary any, raw map[string]string, includeCrashes bool) (err error) {
	files, err := r.snapshot(ctx)
	if err != nil {
		return err
	}
	metadata, err := json.MarshalIndent(map[string]any{
		"created_at": time.Now().UTC(), "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
		"health": r.health(), "app": summary, "timings": r.timingSummary(),
	}, "", "  ")
	if err != nil {
		return err
	}
	files["summary.json"] = metadata
	if len(raw) > 0 {
		data, err := json.MarshalIndent(raw, "", "  ")
		if err != nil {
			return err
		}
		files["private/plugin-stderr.json"] = data
	}
	if includeCrashes {
		for _, name := range []string{"crash-current.log", "crash-previous.log"} {
			if f, err := os.Open(filepath.Join(r.dir, name)); err == nil {
				data, readErr := io.ReadAll(io.LimitReader(f, 1<<20))
				f.Close()
				if readErr == nil && len(data) > 0 {
					files["private/"+name] = data
				}
			}
		}
	}
	// Build beside the destination and publish only a complete archive.
	f, err := os.CreateTemp(filepath.Dir(destination), ".fluent-diagnostics-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	z := zip.NewWriter(f)
	for name, data := range files {
		if err = ctx.Err(); err != nil {
			break
		}
		var writer io.Writer
		writer, err = z.Create(name)
		if err == nil {
			_, err = writer.Write(data)
		}
		if err != nil {
			break
		}
	}
	zipErr, closeErr := z.Close(), f.Close()
	if err != nil {
		return err
	}
	if zipErr != nil {
		return zipErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), destination)
}
