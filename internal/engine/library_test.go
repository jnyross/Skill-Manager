package engine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

func TestListLibraryEmptyWhenMissing(t *testing.T) {
	f := newFixture(t)
	e := engine.New(f.roots)

	entries, err := e.ListLibrary()
	if err != nil {
		t.Fatalf("ListLibrary: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("ListLibrary len = %d, want 0", len(entries))
	}
}

func TestAddListRemoveLibraryLocalPath(t *testing.T) {
	f := newFixture(t)
	location := writeSkill(t, filepath.Join(f.roots.ClaudeHome, "skills", "my-skill"), "my-skill", "desc", "")
	// Snapshot skill tree before Library bookkeeping — remove must not touch it.
	before, err := os.ReadFile(filepath.Join(location, "SKILL.md"))
	if err != nil {
		t.Fatalf("read skill: %v", err)
	}

	e := engine.New(f.roots)
	entry, err := e.AddLibraryEntry(engine.LibraryEntry{
		Name: "my-skill",
		Kind: engine.KindSkill,
		Tool: engine.ToolClaudeCode,
		Source: engine.LibrarySource{
			Kind:      engine.LibrarySourceLocalPath,
			LocalPath: location,
		},
	})
	if err != nil {
		t.Fatalf("AddLibraryEntry: %v", err)
	}
	if entry.ID == "" {
		t.Fatal("expected assigned ID")
	}
	if entry.AddedAt.IsZero() {
		t.Fatal("expected Assigned AddedAt")
	}
	if entry.Source.Kind != engine.LibrarySourceLocalPath || entry.Source.LocalPath != location {
		t.Fatalf("unexpected source: %#v", entry.Source)
	}

	listed, err := e.ListLibrary()
	if err != nil {
		t.Fatalf("ListLibrary: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("ListLibrary len = %d, want 1", len(listed))
	}
	if listed[0].ID != entry.ID || listed[0].Name != "my-skill" {
		t.Fatalf("listed entry: %#v", listed[0])
	}
	recordPath := filepath.Join(f.roots.DataDir, "library", entry.ID+".json")
	if _, err := os.Stat(recordPath); err != nil {
		t.Fatalf("expected one-JSON-per-id record at %s: %v", recordPath, err)
	}

	// Second local-path entry (Codex) for multi-entry list order (AddedAt ascending).
	codexLoc := writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "codex-skill"), "codex-skill", "c", "")
	second, err := e.AddLibraryEntry(engine.LibraryEntry{
		Name: "codex-skill",
		Kind: engine.KindSkill,
		Tool: engine.ToolCodex,
		Source: engine.LibrarySource{
			Kind:      engine.LibrarySourceLocalPath,
			LocalPath: codexLoc,
		},
	})
	if err != nil {
		t.Fatalf("AddLibraryEntry second: %v", err)
	}
	listed, err = e.ListLibrary()
	if err != nil {
		t.Fatalf("ListLibrary: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("ListLibrary len = %d, want 2", len(listed))
	}
	if listed[0].ID != entry.ID || listed[1].ID != second.ID {
		t.Fatalf("want AddedAt ascending order, got %#v", listed)
	}

	if err := e.RemoveLibraryEntry(entry.ID); err != nil {
		t.Fatalf("RemoveLibraryEntry: %v", err)
	}
	listed, err = e.ListLibrary()
	if err != nil {
		t.Fatalf("ListLibrary after remove: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != second.ID {
		t.Fatalf("after remove: %#v", listed)
	}

	after, err := os.ReadFile(filepath.Join(location, "SKILL.md"))
	if err != nil {
		t.Fatalf("skill missing after Library remove: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("RemoveLibraryEntry mutated installed skill content")
	}
	if _, err := os.Stat(location); err != nil {
		t.Fatalf("skill directory missing after Library remove: %v", err)
	}
}

func TestAddLibraryEntryRequiresLocalPath(t *testing.T) {
	f := newFixture(t)
	e := engine.New(f.roots)

	_, err := e.AddLibraryEntry(engine.LibraryEntry{
		Name: "broken",
		Kind: engine.KindSkill,
		Source: engine.LibrarySource{
			Kind: engine.LibrarySourceLocalPath,
		},
	})
	if err == nil {
		t.Fatal("expected error for empty local path")
	}
}

func TestRemoveLibraryEntryMissing(t *testing.T) {
	f := newFixture(t)
	e := engine.New(f.roots)
	if err := e.RemoveLibraryEntry("no-such-id"); err == nil {
		t.Fatal("expected error removing missing entry")
	}
}

func TestFindLibraryEntryByLocalPath(t *testing.T) {
	f := newFixture(t)
	location := writeSkill(t, filepath.Join(f.roots.ClaudeHome, "skills", "find-me"), "find-me", "d", "")
	e := engine.New(f.roots)

	if _, ok := e.FindLibraryEntryByLocalPath(location); ok {
		t.Fatal("expected no match before add")
	}

	entry, err := e.AddLibraryEntry(engine.LibraryEntry{
		Name: "find-me",
		Kind: engine.KindSkill,
		Tool: engine.ToolClaudeCode,
		Source: engine.LibrarySource{
			Kind:      engine.LibrarySourceLocalPath,
			LocalPath: location,
		},
	})
	if err != nil {
		t.Fatalf("AddLibraryEntry: %v", err)
	}

	found, ok := e.FindLibraryEntryByLocalPath(location)
	if !ok || found.ID != entry.ID {
		t.Fatalf("FindLibraryEntryByLocalPath = (%#v, %t)", found, ok)
	}
	if _, ok := e.FindLibraryEntryByLocalPath(location + "-other"); ok {
		t.Fatal("expected no match for other path")
	}
}

func TestRemoveLibraryEntryRejectsMalformedIDs(t *testing.T) {
	f := newFixture(t)
	e := engine.New(f.roots)

	malformed := []string{"", " ", ".", "..", "../../foo", "foo/bar", `foo\bar`}
	for _, id := range malformed {
		t.Run(label(id), func(t *testing.T) {
			if err := e.RemoveLibraryEntry(id); err == nil {
				t.Fatalf("RemoveLibraryEntry(%q) succeeded, want error", id)
			}
		})
	}

	outside := filepath.Join(f.root, "foo.json")
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("malformed id created file outside data dir: %s", outside)
	}
}
