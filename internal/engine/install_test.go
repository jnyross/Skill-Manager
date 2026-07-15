package engine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skillet/internal/engine"
)

func TestInstallLibraryEntryLocalPathToPersonalClaude(t *testing.T) {
	f := newFixture(t)
	src := writeSkill(t, filepath.Join(f.root, "catalog", "demo-skill"), "demo-skill", "Demo description", "version: \"1.0.0\"\n")
	// Extra supporting file so Install copies the whole tree, not just SKILL.md.
	writeFile(t, filepath.Join(src, "notes.txt"), "supporting notes\n")
	srcSnap := snapshotTree(t, src)

	e := engine.New(f.roots)
	entry := engine.LibraryEntry{
		Name: "demo-skill",
		Kind: engine.KindSkill,
		Tool: engine.ToolClaudeCode,
		Source: engine.LibrarySource{
			Kind:      engine.LibrarySourceLocalPath,
			LocalPath: src,
		},
	}

	if err := e.InstallLibraryEntry(entry, engine.InstallTarget{Kind: engine.InstallTargetPersonal}, engine.ActivationAuto); err != nil {
		t.Fatalf("InstallLibraryEntry: %v", err)
	}

	dest := filepath.Join(f.roots.ClaudeHome, "skills", "demo-skill")
	assertSnapshotsEqual(t, srcSnap, snapshotTree(t, dest))

	// Source must remain intact (disconnected copy, not move).
	assertSnapshotsEqual(t, srcSnap, snapshotTree(t, src))

	// Placed skill is ordinary Personal inventory with no Library link.
	skill, ok := findSkill(e.Inventory(), engine.SourcePersonal, "demo-skill")
	if !ok {
		t.Fatal("installed skill not in Personal inventory")
	}
	if skill.Activation != engine.ActivationAuto {
		t.Fatalf("activation = %q, want Auto", skill.Activation)
	}
	if skill.Location != dest {
		t.Fatalf("location = %q, want %q", skill.Location, dest)
	}
}

func TestInstallLibraryEntryLocalPathToProjectClaude(t *testing.T) {
	f := newFixture(t)
	repo := filepath.Join(f.root, "proj")
	mkdirAll(t, repo)
	f.roots.ClaudeProjectRoots = []string{repo}
	f.roots.ProjectRoots = []string{repo}

	src := writeSkill(t, filepath.Join(f.root, "catalog", "proj-skill"), "proj-skill", "Project skill", "")
	writeFile(t, filepath.Join(src, "extra.md"), "# extra\n")
	srcSnap := snapshotTree(t, src)

	e := engine.New(f.roots)
	entry := engine.LibraryEntry{
		Name: "proj-skill",
		Kind: engine.KindSkill,
		Tool: engine.ToolClaudeCode,
		Source: engine.LibrarySource{
			Kind:      engine.LibrarySourceLocalPath,
			LocalPath: src,
		},
	}
	target := engine.InstallTarget{Kind: engine.InstallTargetProject, RepoRoot: repo}

	if err := e.InstallLibraryEntry(entry, target, engine.ActivationAuto); err != nil {
		t.Fatalf("InstallLibraryEntry: %v", err)
	}

	dest := filepath.Join(repo, ".claude", "skills", "proj-skill")
	assertSnapshotsEqual(t, srcSnap, snapshotTree(t, dest))

	skill, ok := findSkill(e.Inventory(), engine.SourceProject, "proj-skill")
	if !ok {
		t.Fatal("installed skill not in Project inventory")
	}
	if skill.Tool != engine.ToolClaudeCode {
		t.Fatalf("tool = %q, want Claude Code", skill.Tool)
	}
}

func TestInstallLibraryEntryLocalPathToPersonalCodex(t *testing.T) {
	f := newFixture(t)
	src := writeSkill(t, filepath.Join(f.root, "catalog", "codex-skill"), "codex-skill", "Codex skill", "")
	srcSnap := snapshotTree(t, src)

	e := engine.New(f.roots)
	entry := engine.LibraryEntry{
		Name: "codex-skill",
		Kind: engine.KindSkill,
		Tool: engine.ToolCodex,
		Source: engine.LibrarySource{
			Kind:      engine.LibrarySourceLocalPath,
			LocalPath: src,
		},
	}

	if err := e.InstallLibraryEntry(entry, engine.InstallTarget{Kind: engine.InstallTargetPersonal}, engine.ActivationAuto); err != nil {
		t.Fatalf("InstallLibraryEntry: %v", err)
	}

	// Documented USER Codex skills path is AgentsHome/skills.
	dest := filepath.Join(f.roots.AgentsHome, "skills", "codex-skill")
	assertSnapshotsEqual(t, srcSnap, snapshotTree(t, dest))

	skill, ok := findSkill(e.Inventory(), engine.SourceCodex, "codex-skill")
	if !ok {
		t.Fatal("installed skill not in Codex inventory")
	}
	if skill.Location != dest {
		t.Fatalf("location = %q, want %q", skill.Location, dest)
	}
}

func TestInstallLibraryEntryLocalPathToProjectCodex(t *testing.T) {
	f := newFixture(t)
	repo := filepath.Join(f.root, "codex-proj")
	mkdirAll(t, repo)
	f.roots.ProjectRoots = []string{repo}
	f.roots.ClaudeProjectRoots = []string{repo}

	src := writeSkill(t, filepath.Join(f.root, "catalog", "cx-proj"), "cx-proj", "Codex project skill", "")
	srcSnap := snapshotTree(t, src)

	e := engine.New(f.roots)
	entry := engine.LibraryEntry{
		Name: "cx-proj",
		Kind: engine.KindSkill,
		Tool: engine.ToolCodex,
		Source: engine.LibrarySource{
			Kind:      engine.LibrarySourceLocalPath,
			LocalPath: src,
		},
	}

	if err := e.InstallLibraryEntry(entry, engine.InstallTarget{Kind: engine.InstallTargetProject, RepoRoot: repo}, engine.ActivationAuto); err != nil {
		t.Fatalf("InstallLibraryEntry: %v", err)
	}

	dest := filepath.Join(repo, ".agents", "skills", "cx-proj")
	assertSnapshotsEqual(t, srcSnap, snapshotTree(t, dest))

	skill, ok := findSkill(e.Inventory(), engine.SourceProject, "cx-proj")
	if !ok {
		t.Fatal("installed skill not in Project inventory")
	}
	if skill.Tool != engine.ToolCodex {
		t.Fatalf("tool = %q, want Codex", skill.Tool)
	}
}

func TestInstallLibraryEntryManualOnlyPersonalClaude(t *testing.T) {
	f := newFixture(t)
	src := writeSkill(t, filepath.Join(f.root, "catalog", "manual-skill"), "manual-skill", "Manual skill", "")

	e := engine.New(f.roots)
	entry := engine.LibraryEntry{
		Name: "manual-skill",
		Kind: engine.KindSkill,
		Tool: engine.ToolClaudeCode,
		Source: engine.LibrarySource{
			Kind:      engine.LibrarySourceLocalPath,
			LocalPath: src,
		},
	}

	if err := e.InstallLibraryEntry(entry, engine.InstallTarget{Kind: engine.InstallTargetPersonal}, engine.ActivationManualOnly); err != nil {
		t.Fatalf("InstallLibraryEntry: %v", err)
	}

	skill, ok := findSkill(e.Inventory(), engine.SourcePersonal, "manual-skill")
	if !ok {
		t.Fatal("installed skill missing from inventory")
	}
	if skill.Activation != engine.ActivationManualOnly {
		t.Fatalf("activation = %q, want Manual-only", skill.Activation)
	}

	data, err := os.ReadFile(filepath.Join(skill.Location, "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if !strings.Contains(string(data), "disable-model-invocation: true") {
		t.Fatalf("expected Manual-only frontmatter, got:\n%s", data)
	}

	// Source catalog copy must stay Auto (no frontmatter edit on source).
	srcData, err := os.ReadFile(filepath.Join(src, "SKILL.md"))
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	if strings.Contains(string(srcData), "disable-model-invocation") {
		t.Fatal("Install mutated source skill frontmatter")
	}
}

func TestInstallLibraryEntryOverwritesExistingAtTarget(t *testing.T) {
	f := newFixture(t)
	src := writeSkill(t, filepath.Join(f.root, "catalog", "refresh-me"), "refresh-me", "v2 body", "version: \"2\"\n")
	writeFile(t, filepath.Join(src, "new-file.txt"), "new\n")

	// Pre-existing destination with stale content.
	oldDest := writeSkill(t, filepath.Join(f.roots.ClaudeHome, "skills", "refresh-me"), "refresh-me", "v1 body", "version: \"1\"\n")
	writeFile(t, filepath.Join(oldDest, "stale.txt"), "stale\n")

	e := engine.New(f.roots)
	entry := engine.LibraryEntry{
		Name: "refresh-me",
		Kind: engine.KindSkill,
		Tool: engine.ToolClaudeCode,
		Source: engine.LibrarySource{
			Kind:      engine.LibrarySourceLocalPath,
			LocalPath: src,
		},
	}

	if err := e.InstallLibraryEntry(entry, engine.InstallTarget{Kind: engine.InstallTargetPersonal}, engine.ActivationAuto); err != nil {
		t.Fatalf("InstallLibraryEntry: %v", err)
	}

	assertSnapshotsEqual(t, snapshotTree(t, src), snapshotTree(t, filepath.Join(f.roots.ClaudeHome, "skills", "refresh-me")))
}

func TestInstallLibraryEntryPreservesUnrelatedOldSiblingDuringOverwrite(t *testing.T) {
	f := newFixture(t)
	src := writeSkill(t, filepath.Join(f.root, "catalog", "refresh-me"), "refresh-me", "v2 body", "")
	dest := writeSkill(t, filepath.Join(f.roots.ClaudeHome, "skills", "refresh-me"), "refresh-me", "v1 body", "")
	unrelated := dest + ".skillet-old"
	writeFile(t, filepath.Join(unrelated, "keep.txt"), "user-owned\n")

	e := engine.New(f.roots)
	entry := engine.LibraryEntry{
		Name: "refresh-me",
		Kind: engine.KindSkill,
		Tool: engine.ToolClaudeCode,
		Source: engine.LibrarySource{
			Kind:      engine.LibrarySourceLocalPath,
			LocalPath: src,
		},
	}

	if err := e.InstallLibraryEntry(entry, engine.InstallTarget{Kind: engine.InstallTargetPersonal}, engine.ActivationAuto); err != nil {
		t.Fatalf("InstallLibraryEntry: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(unrelated, "keep.txt"))
	if err != nil {
		t.Fatalf("unrelated sibling was removed: %v", err)
	}
	if string(data) != "user-owned\n" {
		t.Fatalf("unrelated sibling changed: %q", data)
	}
}

func TestInstallLibraryEntryRejectsUnresolvedProjectRoot(t *testing.T) {
	f := newFixture(t)
	f.roots.ProjectRoots = []string{filepath.Join(f.root, "known")}
	f.roots.ClaudeProjectRoots = []string{filepath.Join(f.root, "known")}
	mkdirAll(t, filepath.Join(f.root, "known"), filepath.Join(f.root, "unknown"))

	src := writeSkill(t, filepath.Join(f.root, "catalog", "s"), "s", "d", "")
	e := engine.New(f.roots)
	entry := engine.LibraryEntry{
		Name: "s",
		Kind: engine.KindSkill,
		Tool: engine.ToolClaudeCode,
		Source: engine.LibrarySource{
			Kind:      engine.LibrarySourceLocalPath,
			LocalPath: src,
		},
	}

	err := e.InstallLibraryEntry(entry, engine.InstallTarget{
		Kind:     engine.InstallTargetProject,
		RepoRoot: filepath.Join(f.root, "unknown"),
	}, engine.ActivationAuto)
	if err == nil {
		t.Fatal("expected error for unresolved project root")
	}
}

func TestInstallDestinationReportsPathAndExistence(t *testing.T) {
	f := newFixture(t)
	src := writeSkill(t, filepath.Join(f.root, "catalog", "path-skill"), "path-skill", "d", "")
	e := engine.New(f.roots)
	entry := engine.LibraryEntry{
		Name: "path-skill",
		Kind: engine.KindSkill,
		Tool: engine.ToolClaudeCode,
		Source: engine.LibrarySource{
			Kind:      engine.LibrarySourceLocalPath,
			LocalPath: src,
		},
	}
	target := engine.InstallTarget{Kind: engine.InstallTargetPersonal}

	dest, exists, err := e.InstallDestination(entry, target)
	if err != nil {
		t.Fatalf("InstallDestination: %v", err)
	}
	want := filepath.Join(f.roots.ClaudeHome, "skills", "path-skill")
	if dest != want {
		t.Fatalf("dest = %q, want %q", dest, want)
	}
	if exists {
		t.Fatal("expected exists=false before install")
	}

	if err := e.InstallLibraryEntry(entry, target, engine.ActivationAuto); err != nil {
		t.Fatalf("InstallLibraryEntry: %v", err)
	}
	_, exists, err = e.InstallDestination(entry, target)
	if err != nil {
		t.Fatalf("InstallDestination after: %v", err)
	}
	if !exists {
		t.Fatal("expected exists=true after install")
	}
}

func TestResolvedProjectRootsDedupes(t *testing.T) {
	f := newFixture(t)
	repo := filepath.Join(f.root, "r")
	f.roots.ProjectRoots = []string{repo, filepath.Join(f.root, "other")}
	f.roots.ClaudeProjectRoots = []string{repo}
	e := engine.New(f.roots)

	roots := e.ResolvedProjectRoots()
	if len(roots) != 2 {
		t.Fatalf("ResolvedProjectRoots = %#v, want 2 unique", roots)
	}
}

func TestInstallLibraryEntryFromSymlinkedLocalPath(t *testing.T) {
	f := newFixture(t)
	real := writeSkill(t, filepath.Join(f.root, "catalog", "real-skill"), "real-skill", "via symlink", "")
	writeFile(t, filepath.Join(real, "extra.txt"), "x\n")
	link := filepath.Join(f.root, "catalog", "linked-skill")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	srcSnap := snapshotTree(t, real)

	e := engine.New(f.roots)
	entry := engine.LibraryEntry{
		Name: "real-skill",
		Kind: engine.KindSkill,
		Tool: engine.ToolClaudeCode,
		Source: engine.LibrarySource{
			Kind:      engine.LibrarySourceLocalPath,
			LocalPath: link,
		},
	}
	if err := e.InstallLibraryEntry(entry, engine.InstallTarget{Kind: engine.InstallTargetPersonal}, engine.ActivationAuto); err != nil {
		t.Fatalf("InstallLibraryEntry from symlink: %v", err)
	}
	assertSnapshotsEqual(t, srcSnap, snapshotTree(t, filepath.Join(f.roots.ClaudeHome, "skills", "real-skill")))
}

func TestInstallLibraryEntryRequiresTool(t *testing.T) {
	f := newFixture(t)
	src := writeSkill(t, filepath.Join(f.root, "catalog", "no-tool"), "no-tool", "d", "")
	e := engine.New(f.roots)
	err := e.InstallLibraryEntry(engine.LibraryEntry{
		Name: "no-tool",
		Kind: engine.KindSkill,
		Source: engine.LibrarySource{
			Kind:      engine.LibrarySourceLocalPath,
			LocalPath: src,
		},
	}, engine.InstallTarget{Kind: engine.InstallTargetPersonal}, engine.ActivationAuto)
	if err == nil {
		t.Fatal("expected error when Tool is empty")
	}
}
