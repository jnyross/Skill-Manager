package tui

import (
	"testing"
	"time"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

func TestBuildArchiveItemsPreservesOrderAndEmpty(t *testing.T) {
	if items := buildArchiveItems(nil); len(items) != 0 {
		t.Fatalf("buildArchiveItems(nil) len = %d, want 0", len(items))
	}
	if items := buildArchiveItems([]engine.ArchiveEntry{}); len(items) != 0 {
		t.Fatalf("buildArchiveItems(empty) len = %d, want 0", len(items))
	}

	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	entries := []engine.ArchiveEntry{
		{ID: "new", Name: "newer", Source: engine.SourcePersonal, ArchivedAt: newer},
		{ID: "old", Name: "older", Source: engine.SourceProject, Tool: engine.ToolCodex, ArchivedAt: older},
	}
	items := buildArchiveItems(entries)
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	assertArchive(t, items[0], "newer")
	assertArchive(t, items[1], "older")
}

func assertArchive(t *testing.T, item any, name string) {
	t.Helper()
	entry, ok := item.(archiveItem)
	if !ok {
		t.Fatalf("item = %#v, want archiveItem", item)
	}
	if entry.entry.Name != name {
		t.Fatalf("entry.Name = %q, want %q", entry.entry.Name, name)
	}
}
