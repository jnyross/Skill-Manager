package engine_test

import (
	"os"
	"path/filepath"
	"testing"

	"skillet/internal/engine"
)

func TestArchiveUninstallPersonalSkill(t *testing.T) {
	f := newFixture(t)
	location := writeSkill(t, filepath.Join(f.roots.ClaudeHome, "skills", "to-archive"), "to-archive", "Archive me", "")
	e := engine.New(f.roots)

	entry, err := e.Uninstall(location)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if entry.Name != "to-archive" || entry.Source != engine.SourcePersonal || entry.OriginalLocation != location {
		t.Fatalf("unexpected archive entry: %#v", entry)
	}
	if _, err := os.Lstat(location); !os.IsNotExist(err) {
		t.Fatalf("original location still exists or unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.roots.DataDir, "archive", entry.ID, "to-archive")); err != nil {
		t.Fatalf("archived folder missing: %v", err)
	}

	if _, ok := findSkill(e.Inventory(), engine.SourcePersonal, "to-archive"); ok {
		t.Fatalf("archived skill still appears in inventory")
	}
	entries, err := e.ListArchive()
	if err != nil {
		t.Fatalf("list archive: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("archive length = %d, want 1: %#v", len(entries), entries)
	}
	if entries[0].ID != entry.ID || entries[0].OriginalLocation != location {
		t.Fatalf("unexpected listed archive entry: %#v", entries[0])
	}
}

func TestArchiveSymlinkUninstallAndRestore(t *testing.T) {
	f := newFixture(t)
	target := writeSkill(t, filepath.Join(f.root, "shared-skills", "linked"), "linked", "Linked description", "")
	link := filepath.Join(f.roots.ClaudeHome, "skills", "linked")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	e := engine.New(f.roots)

	entry, err := e.Uninstall(link)
	if err != nil {
		t.Fatalf("uninstall symlink: %v", err)
	}
	if !entry.IsSymlink || entry.SymlinkTarget != target {
		t.Fatalf("unexpected symlink provenance: %#v", entry)
	}
	if err := e.Restore(entry.ID); err != nil {
		t.Fatalf("restore symlink: %v", err)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("restored symlink missing: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("restored entry is not a symlink: %s", info.Mode())
	}
	restoredTarget, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("read restored symlink: %v", err)
	}
	if restoredTarget != target {
		t.Fatalf("restored target = %q, want %q", restoredTarget, target)
	}
}

func TestArchiveRestoreRoundTripPreservesFixtureTree(t *testing.T) {
	f := newFixture(t)
	location := writeSkill(t, filepath.Join(f.roots.ClaudeHome, "skills", "roundtrip"), "roundtrip", "Roundtrip description", "")
	writeFile(t, filepath.Join(location, "references", "notes.md"), "notes\n")
	before := snapshotTree(t, f.root)
	e := engine.New(f.roots)

	entry, err := e.Uninstall(location)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if err := e.Restore(entry.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}
	after := snapshotTree(t, f.root)
	assertSnapshotsEqual(t, before, after)
}

func TestArchivePurgeRemovesArchiveEntry(t *testing.T) {
	f := newFixture(t)
	location := writeSkill(t, filepath.Join(f.roots.ClaudeHome, "skills", "purge-me"), "purge-me", "Purge me", "")
	e := engine.New(f.roots)
	entry, err := e.Uninstall(location)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	if err := e.Purge(entry.ID); err != nil {
		t.Fatalf("purge: %v", err)
	}
	entries, err := e.ListArchive()
	if err != nil {
		t.Fatalf("list archive: %v", err)
	}
	if archiveContains(entries, entry.ID) {
		t.Fatalf("purged archive entry still listed: %#v", entries)
	}
	if _, err := os.Stat(filepath.Join(f.roots.DataDir, "archive", entry.ID)); !os.IsNotExist(err) {
		t.Fatalf("purged archive dir still exists or unexpected error: %v", err)
	}
}

func TestReadOnlyMethodsLeaveFixtureTreeUnchanged(t *testing.T) {
	f := newFixture(t)
	writeSkill(t, filepath.Join(f.roots.ClaudeHome, "skills", "read-only"), "read-only", "Read only", "")
	writeSkill(t, filepath.Join(f.roots.AgentsHome, "skills", "codex-read-only"), "codex-read-only", "Codex read only", "")
	before := snapshotTree(t, f.root)
	e := engine.New(f.roots)

	_ = e.Inventory()
	if _, err := e.ListArchive(); err != nil {
		t.Fatalf("list archive: %v", err)
	}

	after := snapshotTree(t, f.root)
	assertSnapshotsEqual(t, before, after)
}
