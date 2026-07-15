package engine_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/jnyross/Skill-Manager/internal/engine"
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
	if entry.Tool != engine.ToolClaudeCode || entry.OriginRepo != "" {
		t.Fatalf("unexpected archive entry tool/origin: %#v", entry)
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

func TestArchiveRestoreRoundTripProjectSkills(t *testing.T) {
	cases := []struct {
		name          string
		locationParts []string
		configure     func(*engine.Roots, string)
		wantTool      engine.Tool
	}{
		{
			name:          "claude project skill",
			locationParts: []string{".claude", "skills", "project-claude"},
			configure: func(roots *engine.Roots, repo string) {
				roots.ClaudeProjectRoots = []string{repo}
			},
			wantTool: engine.ToolClaudeCode,
		},
		{
			name:          "codex project skill",
			locationParts: []string{".agents", "skills", "project-codex"},
			configure: func(roots *engine.Roots, repo string) {
				roots.ProjectRoots = []string{repo}
			},
			wantTool: engine.ToolCodex,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			repo := filepath.Join(f.root, "repo")
			location := writeSkill(t, filepath.Join(append([]string{repo}, tc.locationParts...)...), filepath.Base(tc.locationParts[len(tc.locationParts)-1]), "Project description", "")
			tc.configure(&f.roots, repo)
			writeFile(t, filepath.Join(location, "references", "notes.md"), "notes\n")
			before := snapshotTree(t, f.root)
			e := engine.New(f.roots)

			entry, err := e.Uninstall(location)
			if err != nil {
				t.Fatalf("uninstall: %v", err)
			}
			if entry.Source != engine.SourceProject || entry.Kind != engine.KindSkill || entry.OriginalLocation != location {
				t.Fatalf("unexpected archive entry: %#v", entry)
			}
			if entry.Tool != tc.wantTool || entry.OriginRepo != repo {
				t.Fatalf("unexpected project archive tool/origin: %#v", entry)
			}
			if _, err := os.Lstat(location); !os.IsNotExist(err) {
				t.Fatalf("original location still exists or unexpected error: %v", err)
			}

			if err := e.Restore(entry.ID); err != nil {
				t.Fatalf("restore: %v", err)
			}
			after := snapshotTree(t, f.root)
			assertSnapshotsEqual(t, before, after)
		})
	}
}

func TestListArchiveFiltersProjectEntriesToCurrentRepo(t *testing.T) {
	f := newFixture(t)
	repoA := filepath.Join(f.root, "repo-a")
	repoB := filepath.Join(f.root, "repo-b")

	allRoots := f.roots
	allRoots.ClaudeProjectRoots = []string{repoA, repoB}
	allRoots.ProjectRoots = []string{repoA, repoB}
	archiver := engine.New(allRoots)

	repoAClaude := writeSkill(t, filepath.Join(repoA, ".claude", "skills", "a-claude"), "a-claude", "A Claude", "")
	repoACodex := writeSkill(t, filepath.Join(repoA, ".agents", "skills", "a-codex"), "a-codex", "A Codex", "")
	repoBClaude := writeSkill(t, filepath.Join(repoB, ".claude", "skills", "b-claude"), "b-claude", "B Claude", "")
	repoBCodex := writeSkill(t, filepath.Join(repoB, ".agents", "skills", "b-codex"), "b-codex", "B Codex", "")
	personal := writeSkill(t, filepath.Join(f.roots.ClaudeHome, "skills", "personal-global"), "personal-global", "Personal", "")
	codex := writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "codex-global"), "codex-global", "Codex", "")

	entryAClaude, err := archiver.Uninstall(repoAClaude)
	if err != nil {
		t.Fatalf("uninstall repo A Claude: %v", err)
	}
	entryACodex, err := archiver.Uninstall(repoACodex)
	if err != nil {
		t.Fatalf("uninstall repo A Codex: %v", err)
	}
	entryBClaude, err := archiver.Uninstall(repoBClaude)
	if err != nil {
		t.Fatalf("uninstall repo B Claude: %v", err)
	}
	entryBCodex, err := archiver.Uninstall(repoBCodex)
	if err != nil {
		t.Fatalf("uninstall repo B Codex: %v", err)
	}
	entryPersonal, err := archiver.Uninstall(personal)
	if err != nil {
		t.Fatalf("uninstall personal: %v", err)
	}
	entryCodex, err := archiver.Uninstall(codex)
	if err != nil {
		t.Fatalf("uninstall codex: %v", err)
	}

	pluginID := "plugin-global"
	writeArchiveProvenance(t, f.roots, engine.ArchiveEntry{
		ID:               pluginID,
		Name:             "plugin-global",
		Source:           engine.SourcePlugin,
		Tool:             engine.ToolClaudeCode,
		Kind:             engine.KindSkill,
		OriginalLocation: filepath.Join(f.root, "plugin", "plugin-global"),
		ArchivedAt:       time.Now().UTC(),
	})

	currentRoots := f.roots
	currentRoots.ClaudeProjectRoots = []string{repoA}
	currentRoots.ProjectRoots = []string{repoA}
	entries, err := engine.New(currentRoots).ListArchive()
	if err != nil {
		t.Fatalf("list archive: %v", err)
	}

	for _, entry := range []engine.ArchiveEntry{entryAClaude, entryACodex, entryPersonal, entryCodex} {
		if !archiveContains(entries, entry.ID) {
			t.Fatalf("expected archive entry %s to be listed: %#v", entry.ID, entries)
		}
	}
	if !archiveContains(entries, pluginID) {
		t.Fatalf("expected plugin archive entry to be listed: %#v", entries)
	}
	for _, entry := range []engine.ArchiveEntry{entryBClaude, entryBCodex} {
		if archiveContains(entries, entry.ID) {
			t.Fatalf("expected archive entry %s to be filtered out: %#v", entry.ID, entries)
		}
	}
}

func TestArchiveUninstallRemovesConfigEntryForSuppressedProjectCodexSkill(t *testing.T) {
	f := newFixture(t)
	folder := writeSkill(t, filepath.Join(f.root, "repo", ".agents", "skills", "project-codex"), "project-codex", "Project Codex description", "")
	f.roots.ProjectRoots = []string{filepath.Join(f.root, "repo")}
	configPath := filepath.Join(f.roots.CodexHome, "config.toml")

	e := engine.New(f.roots)
	skill := engine.Skill{
		Name:     "project-codex",
		Source:   engine.SourceProject,
		Tool:     engine.ToolCodex,
		Kind:     engine.KindSkill,
		Location: folder,
	}
	if err := e.Suppress(skill); err != nil {
		t.Fatalf("suppress: %v", err)
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config.toml missing after suppress: %v", err)
	}

	entry, err := e.Uninstall(folder)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if entry.Source != engine.SourceProject {
		t.Fatalf("unexpected archive entry: %#v", entry)
	}
	if len(entry.RemovedConfigEntries) == 0 {
		t.Fatalf("expected archived entry to record the removed config.toml block, got none")
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config after uninstall: %v", err)
	}
	if string(got) != "" {
		t.Fatalf("config after uninstall = %q, want empty (matching TestArchiveUninstallCodexSkillByNameMatch's existing behavior for a config.toml that becomes empty)", string(got))
	}

	if err := e.Restore(entry.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, err = os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config after restore: %v", err)
	}
	want := "[[skills.config]]\npath = " + strconv.Quote(filepath.Join(folder, "SKILL.md")) + "\nenabled = false\n"
	if string(got) != want {
		t.Fatalf("config after restore = %q, want %q", string(got), want)
	}
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

func writeArchiveProvenance(t *testing.T, roots engine.Roots, entry engine.ArchiveEntry) {
	t.Helper()
	archiveDir := filepath.Join(roots.DataDir, "archive", entry.ID)
	mkdirAll(t, archiveDir)
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		t.Fatalf("marshal archive provenance: %v", err)
	}
	if err := os.WriteFile(filepath.Join(archiveDir, "provenance.json"), append(data, '\n'), 0o600); err != nil {
		t.Fatalf("write archive provenance: %v", err)
	}
}
