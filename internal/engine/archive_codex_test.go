package engine_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"skillet/internal/engine"
)

// Codex config.toml shape ([[skills.config]] path/name/enabled) encodes the
// conventions verified locally in docs/research/skill-mechanisms.md.

func TestArchiveUninstallCodexSkillRemovesStaleConfigEntry(t *testing.T) {
	f := newFixture(t)
	skillFolder := writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "codex-skill"), "codex-skill", "Codex description", "")
	skillMD := filepath.Join(skillFolder, "SKILL.md")

	configBefore := "[profile]\nname = \"default\"\n\n[[skills.config]]\npath = " +
		strconv.Quote(skillMD) + "\nenabled = false\n\n[[skills.config]]\nname = \"other-skill\"\nenabled = false\n"
	writeFile(t, filepath.Join(f.roots.CodexHome, "config.toml"), configBefore)

	e := engine.New(f.roots)
	entry, err := e.Uninstall(skillFolder)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if entry.Source != engine.SourceCodex || entry.Kind != engine.KindSkill {
		t.Fatalf("unexpected archive entry: %#v", entry)
	}
	if _, err := os.Lstat(skillFolder); !os.IsNotExist(err) {
		t.Fatalf("original location still exists or unexpected error: %v", err)
	}
	if len(entry.RemovedConfigEntries) != 1 {
		t.Fatalf("removed config entries = %#v, want 1 entry", entry.RemovedConfigEntries)
	}

	got, err := os.ReadFile(filepath.Join(f.roots.CodexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config after uninstall: %v", err)
	}
	// The blank line that separated the two blocks belonged to neither one
	// (see skillsConfigBlockSpans) and survives the removal untouched,
	// producing two consecutive blank lines here rather than one — that
	// separator is what lets a second Codex skill's entry in the same file
	// be restored independently of this one, in any order.
	want := "[profile]\nname = \"default\"\n\n\n[[skills.config]]\nname = \"other-skill\"\nenabled = false\n"
	if string(got) != want {
		t.Fatalf("config after uninstall = %q, want %q", string(got), want)
	}

	if _, ok := findSkill(e.Inventory(), engine.SourceCodex, "codex-skill"); ok {
		t.Fatalf("archived codex skill still appears in inventory")
	}
}

func TestArchiveRestoreCodexSkillReinstatesConfigEntryByteIdentical(t *testing.T) {
	f := newFixture(t)
	skillFolder := writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "codex-skill"), "codex-skill", "Codex description", "")
	skillMD := filepath.Join(skillFolder, "SKILL.md")

	configBefore := "[profile]\nname = \"default\"\n\n[[skills.config]]\npath = " +
		strconv.Quote(skillMD) + "\nenabled = false\n\n[[skills.config]]\nname = \"other-skill\"\nenabled = false\n"
	writeFile(t, filepath.Join(f.roots.CodexHome, "config.toml"), configBefore)

	before := snapshotTree(t, f.root)
	e := engine.New(f.roots)

	entry, err := e.Uninstall(skillFolder)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if err := e.Restore(entry.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}

	after := snapshotTree(t, f.root)
	assertSnapshotsEqual(t, before, after)
}

func TestArchiveUninstallCodexSkillByNameMatch(t *testing.T) {
	f := newFixture(t)
	skillFolder := writeSkill(t, filepath.Join(f.roots.AgentsHome, "skills", "named-skill"), "named-skill", "Named skill", "")

	configBefore := "[[skills.config]]\nname = \"named-skill\"\nenabled = false\n"
	writeFile(t, filepath.Join(f.roots.CodexHome, "config.toml"), configBefore)

	e := engine.New(f.roots)
	entry, err := e.Uninstall(skillFolder)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if len(entry.RemovedConfigEntries) != 1 {
		t.Fatalf("removed config entries = %#v, want 1 entry", entry.RemovedConfigEntries)
	}

	got, err := os.ReadFile(filepath.Join(f.roots.CodexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config after uninstall: %v", err)
	}
	if string(got) != "" {
		t.Fatalf("config after uninstall = %q, want empty", string(got))
	}

	if err := e.Restore(entry.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, err = os.ReadFile(filepath.Join(f.roots.CodexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config after restore: %v", err)
	}
	if string(got) != configBefore {
		t.Fatalf("config after restore = %q, want %q", string(got), configBefore)
	}
}

func TestArchiveUninstallCodexSkillNoConfigEntryLeavesConfigUntouched(t *testing.T) {
	f := newFixture(t)
	skillFolder := writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "plain-skill"), "plain-skill", "Plain skill", "")
	configBefore := "[[skills.config]]\nname = \"unrelated-skill\"\nenabled = false\n"
	writeFile(t, filepath.Join(f.roots.CodexHome, "config.toml"), configBefore)

	e := engine.New(f.roots)
	entry, err := e.Uninstall(skillFolder)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if len(entry.RemovedConfigEntries) != 0 {
		t.Fatalf("removed config entries = %#v, want none", entry.RemovedConfigEntries)
	}

	got, err := os.ReadFile(filepath.Join(f.roots.CodexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(got) != configBefore {
		t.Fatalf("config = %q, want untouched %q", string(got), configBefore)
	}
}

func TestArchiveUninstallCodexPrompt(t *testing.T) {
	f := newFixture(t)
	promptPath := filepath.Join(f.roots.CodexHome, "prompts", "ideas.md")
	writePrompt(t, promptPath, "Ideas prompt")

	e := engine.New(f.roots)
	entry, err := e.Uninstall(promptPath)
	if err != nil {
		t.Fatalf("uninstall prompt: %v", err)
	}
	if entry.Name != "ideas" || entry.Source != engine.SourceCodex || entry.Kind != engine.KindPrompt {
		t.Fatalf("unexpected archive entry: %#v", entry)
	}
	if _, err := os.Lstat(promptPath); !os.IsNotExist(err) {
		t.Fatalf("original prompt still exists or unexpected error: %v", err)
	}
	if _, ok := findSkill(e.Inventory(), engine.SourceCodex, "ideas"); ok {
		t.Fatalf("archived prompt still appears in inventory")
	}

	if err := e.Restore(entry.ID); err != nil {
		t.Fatalf("restore prompt: %v", err)
	}
	if _, ok := findSkill(e.Inventory(), engine.SourceCodex, "ideas"); !ok {
		t.Fatalf("restored prompt missing from inventory")
	}
}

// Two Codex skills sharing one config.toml, archived and then restored out
// of order: restoring the first while the second is still archived must not
// corrupt config.toml using stale positions from before the second skill's
// entry was also removed. Each Restore leaves config.toml byte-identical to
// what it held immediately after that skill's own Uninstall (mid-sequence),
// and the final Restore reconstructs the pre-uninstall original exactly.
func TestArchiveTwoCodexSkillsSharedConfigRestoreOutOfOrder(t *testing.T) {
	f := newFixture(t)
	skillA := writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "skill-a"), "skill-a", "Skill A", "")
	skillB := writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "skill-b"), "skill-b", "Skill B", "")

	original := "[profile]\nname = \"default\"\n\n[[skills.config]]\nname = \"skill-a\"\nenabled = false\n\n[[skills.config]]\nname = \"skill-b\"\nenabled = false\n"
	writeFile(t, filepath.Join(f.roots.CodexHome, "config.toml"), original)

	e := engine.New(f.roots)

	entryA, err := e.Uninstall(skillA)
	if err != nil {
		t.Fatalf("uninstall A: %v", err)
	}
	afterA, err := os.ReadFile(filepath.Join(f.roots.CodexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config after uninstall A: %v", err)
	}
	// The blank line separating A and B belongs to neither block (see
	// skillsConfigBlockSpans) and survives untouched, so it now sits next to
	// the blank line that originally preceded A.
	wantAfterA := "[profile]\nname = \"default\"\n\n\n[[skills.config]]\nname = \"skill-b\"\nenabled = false\n"
	if string(afterA) != wantAfterA {
		t.Fatalf("config after uninstall A = %q, want %q", string(afterA), wantAfterA)
	}

	entryB, err := e.Uninstall(skillB)
	if err != nil {
		t.Fatalf("uninstall B: %v", err)
	}
	afterB, err := os.ReadFile(filepath.Join(f.roots.CodexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config after uninstall B: %v", err)
	}
	wantAfterB := "[profile]\nname = \"default\"\n\n\n"
	if string(afterB) != wantAfterB {
		t.Fatalf("config after uninstall B = %q, want %q", string(afterB), wantAfterB)
	}

	// Restore A first, while B is still archived. A's block reappears
	// followed by the blank line that used to separate A and B — that
	// blank line was never part of either skill's removed block, so it's
	// still sitting in the file waiting for B.
	if err := e.Restore(entryA.ID); err != nil {
		t.Fatalf("restore A: %v", err)
	}
	afterRestoreA, err := os.ReadFile(filepath.Join(f.roots.CodexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config after restore A: %v", err)
	}
	wantAfterRestoreA := "[profile]\nname = \"default\"\n\n[[skills.config]]\nname = \"skill-a\"\nenabled = false\n\n"
	if string(afterRestoreA) != wantAfterRestoreA {
		t.Fatalf("config after restore A = %q, want %q", string(afterRestoreA), wantAfterRestoreA)
	}

	// Now restore B; the file must reconstruct the pre-uninstall original exactly.
	if err := e.Restore(entryB.ID); err != nil {
		t.Fatalf("restore B: %v", err)
	}
	final, err := os.ReadFile(filepath.Join(f.roots.CodexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config after restore B: %v", err)
	}
	if string(final) != original {
		t.Fatalf("final config = %q, want original %q", string(final), original)
	}
}

func TestArchivePurgeCodexSkillDoesNotRestoreConfig(t *testing.T) {
	f := newFixture(t)
	skillFolder := writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "purge-skill"), "purge-skill", "Purge me", "")
	skillMD := filepath.Join(skillFolder, "SKILL.md")
	configBefore := "[[skills.config]]\npath = " + strconv.Quote(skillMD) + "\nenabled = false\n"
	writeFile(t, filepath.Join(f.roots.CodexHome, "config.toml"), configBefore)

	e := engine.New(f.roots)
	entry, err := e.Uninstall(skillFolder)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if err := e.Purge(entry.ID); err != nil {
		t.Fatalf("purge: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(f.roots.CodexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config after purge: %v", err)
	}
	if string(got) != "" {
		t.Fatalf("config after purge = %q, want empty (entry stays removed)", string(got))
	}
	entries, err := e.ListArchive()
	if err != nil {
		t.Fatalf("list archive: %v", err)
	}
	if archiveContains(entries, entry.ID) {
		t.Fatalf("purged archive entry still listed: %#v", entries)
	}
}
