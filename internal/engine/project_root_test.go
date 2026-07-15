package engine_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

func TestFindProjectRootsWalksEveryParentToRepoRoot(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	cwd := filepath.Join(repo, "alpha", "bravo", "charlie")
	mkdirAll(t, filepath.Join(repo, ".git"), cwd)

	got := engine.FindProjectRoots(cwd)
	want := []string{
		cwd,
		filepath.Dir(cwd),
		filepath.Dir(filepath.Dir(cwd)),
		repo,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FindProjectRoots() = %#v, want %#v", got, want)
	}
}

func TestFindProjectRootsDedupesRepoRoot(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	mkdirAll(t, filepath.Join(repo, ".git"))

	got := engine.FindProjectRoots(repo)
	want := []string{repo}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FindProjectRoots() = %#v, want %#v", got, want)
	}
}

func TestProjectCodexSkillAtIntermediateAncestor(t *testing.T) {
	f := newFixture(t)
	repo := filepath.Join(f.root, "repo")
	cwd := filepath.Join(repo, "alpha", "bravo", "charlie")
	mkdirAll(t, filepath.Join(repo, ".git"), cwd)
	location := writeSkill(t, filepath.Join(repo, "alpha", ".agents", "skills", "ancestor-codex"), "ancestor-codex", "Intermediate ancestor", "")

	f.roots.ProjectRoots = engine.FindProjectRoots(cwd)
	f.roots.ClaudeProjectRoots = engine.FindClaudeProjectRoots(cwd)
	inv := engine.New(f.roots).Inventory()
	skill, ok := findSkill(inv, engine.SourceProject, "ancestor-codex")
	if !ok {
		t.Fatalf("intermediate Codex skill missing: roots=%#v skills=%#v", f.roots.ProjectRoots, inv.Skills)
	}
	if skill.Location != location || skill.Tool != engine.ToolCodex {
		t.Fatalf("intermediate Codex skill = %#v, want location %q and Codex tool", skill, location)
	}
}

func TestFindProjectRootsAcceptsGitFile(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	cwd := filepath.Join(repo, "alpha")
	mkdirAll(t, cwd)
	writeFile(t, filepath.Join(repo, ".git"), "gitdir: ../repo.git\n")

	got := engine.FindProjectRoots(cwd)
	want := []string{
		cwd,
		repo,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FindProjectRoots() = %#v, want %#v", got, want)
	}
}

func TestFindClaudeProjectRootsWalksEveryParentToRepoRoot(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	cwd := filepath.Join(repo, "alpha", "bravo", "charlie")
	mkdirAll(t, filepath.Join(repo, ".git"), cwd)

	got := engine.FindClaudeProjectRoots(cwd)
	want := []string{
		cwd,
		filepath.Dir(cwd),
		filepath.Dir(filepath.Dir(cwd)),
		repo,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FindClaudeProjectRoots() = %#v, want %#v", got, want)
	}
}

func TestFindClaudeProjectRootsDedupesRepoRoot(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	mkdirAll(t, filepath.Join(repo, ".git"))

	got := engine.FindClaudeProjectRoots(repo)
	want := []string{repo}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FindClaudeProjectRoots() = %#v, want %#v", got, want)
	}
}

func TestProjectRootsWithoutGit(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "workspace", "nested")
	mkdirAll(t, cwd)
	if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
		t.Fatal("fixture unexpectedly contains .git")
	}

	gotCodex := engine.FindProjectRoots(cwd)
	wantCodex := []string{
		cwd,
		filepath.Dir(cwd),
	}
	if !reflect.DeepEqual(gotCodex, wantCodex) {
		t.Fatalf("FindProjectRoots() = %#v, want %#v", gotCodex, wantCodex)
	}

	gotClaude := engine.FindClaudeProjectRoots(cwd)
	wantClaude := []string{cwd}
	if !reflect.DeepEqual(gotClaude, wantClaude) {
		t.Fatalf("FindClaudeProjectRoots() = %#v, want %#v", gotClaude, wantClaude)
	}
}
