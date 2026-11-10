package search

import (
	"context"
	"fmt"
	"testing"

	"fluentlauncher/internal/core/model"
)

func benchmarkEntries(n int) []model.Entry {
	entries := make([]model.Entry, n)
	for i := range entries {
		title := fmt.Sprintf("ProjectReport_%06d.txt", i)
		if i%4 == 0 {
			title = fmt.Sprintf("项目报告_%06d.txt", i)
		}
		path := fmt.Sprintf(`C:\Fixture\Folder%03d\%s`, i%100, title)
		entries[i] = model.Entry{ID: "file:" + path, Title: title, Subtitle: path, LaunchTarget: path, IconPath: path, Kind: model.File, SourceID: "files"}
	}
	return entries
}

func BenchmarkEngine(b *testing.B) {
	for _, n := range []int{1000, 10000, 100000} {
		entries := benchmarkEntries(n)
		e := NewEngine()
		e.ReplaceEntries(entries, nil)
		for _, query := range []string{"project", "项目", "zzzzzz"} {
			b.Run(fmt.Sprintf("Search/%d/%s", n, query), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					e.Search(context.Background(), query, 20)
				}
			})
		}
		b.Run(fmt.Sprintf("Profile/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				e.UpdateProfile(model.Profile{EntryID: entries[0].ID, UseCount: 1})
			}
		})
		b.Run(fmt.Sprintf("Replace/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				e.ReplaceEntries(entries, nil)
			}
		})
	}
}
