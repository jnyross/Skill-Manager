package engine_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"skillet/internal/engine"
)

func TestFindProjectRootsUsesCodexThreeCandidates(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	cwd := filepath.Join(repo, "alpha", "bravo", "charlie")
	mkdirAll(t, filepath.Join(repo, ".git"), cwd)

	got := engine.FindProjectRoots(cwd)
	want := []string{
		cwd,
		filepath.Dir(cwd),
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
	want := []string{
		repo,
		filepath.Dir(repo),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FindProjectRoots() = %#v, want %#v", got, want)
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
