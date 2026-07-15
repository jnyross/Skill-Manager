package tui

import (
	"testing"
	"time"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

func TestBuildLibraryItemsPreservesOrderAndEmpty(t *testing.T) {
	if items := buildLibraryItems(nil); len(items) != 0 {
		t.Fatalf("buildLibraryItems(nil) len = %d, want 0", len(items))
	}

	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	entries := []engine.LibraryEntry{
		{ID: "a", Name: "first", Source: engine.LibrarySource{Kind: engine.LibrarySourceLocalPath, LocalPath: "/a"}, AddedAt: older},
		{ID: "b", Name: "second", Source: engine.LibrarySource{Kind: engine.LibrarySourceLocalPath, LocalPath: "/b"}, AddedAt: newer},
	}
	items := buildLibraryItems(entries)
	if len(items) != 2 {
		t.Fatalf("len = %d, want 2", len(items))
	}
	first, ok := items[0].(libraryItem)
	if !ok || first.entry.Name != "first" {
		t.Fatalf("items[0] = %#v", items[0])
	}
	second, ok := items[1].(libraryItem)
	if !ok || second.entry.Name != "second" {
		t.Fatalf("items[1] = %#v", items[1])
	}
}

func TestCanToggleLibraryMembership(t *testing.T) {
	cases := []struct {
		name  string
		skill engine.Skill
		want  bool
	}{
		{"personal skill", engine.Skill{Source: engine.SourcePersonal, Kind: engine.KindSkill}, true},
		{"codex skill", engine.Skill{Source: engine.SourceCodex, Kind: engine.KindSkill}, true},
		{"codex prompt", engine.Skill{Source: engine.SourceCodex, Kind: engine.KindPrompt}, false},
		{"plugin", engine.Skill{Source: engine.SourcePlugin, Kind: engine.KindSkill}, false},
		{"project", engine.Skill{Source: engine.SourceProject, Kind: engine.KindSkill, Tool: engine.ToolClaudeCode}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := canToggleLibraryMembership(tc.skill); got != tc.want {
				t.Fatalf("canToggleLibraryMembership = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestLibrarySourceLocation(t *testing.T) {
	if got := librarySourceLocation(engine.LibrarySource{Kind: engine.LibrarySourceLocalPath, LocalPath: "/tmp/s"}); got != "/tmp/s" {
		t.Fatalf("local path = %q", got)
	}
	if got := librarySourceLocation(engine.LibrarySource{Kind: engine.LibrarySourceMarketplace, Marketplace: "m", PluginName: "p"}); got != "p@m" {
		t.Fatalf("marketplace = %q", got)
	}
}
