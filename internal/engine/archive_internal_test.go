package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyLocationReturnsMatchedProjectOriginRepo(t *testing.T) {
	root := t.TempDir()
	outer := filepath.Join(root, "repo")
	inner := filepath.Join(outer, "nested")
	roots := Roots{
		ClaudeHome:         filepath.Join(root, "claude"),
		CodexHome:          filepath.Join(root, "codex"),
		AgentsHome:         filepath.Join(root, "agents"),
		DataDir:            filepath.Join(root, "data"),
		ClaudeProjectRoots: []string{outer, inner},
		ProjectRoots:       []string{outer, inner},
	}
	e := New(roots)

	claudeLocation := filepath.Join(inner, ".claude", "skills", "project-claude")
	source, kind, tool, originRepo, err := e.classifyLocation(claudeLocation)
	if err != nil {
		t.Fatalf("classify Claude project location: %v", err)
	}
	if source != SourceProject || kind != KindSkill || tool != ToolClaudeCode || originRepo != inner {
		t.Fatalf("classify Claude project = (%q, %q, %q, %q), want (%q, %q, %q, %q)", source, kind, tool, originRepo, SourceProject, KindSkill, ToolClaudeCode, inner)
	}

	codexLocation := filepath.Join(inner, ".agents", "skills", "project-codex")
	source, kind, tool, originRepo, err = e.classifyLocation(codexLocation)
	if err != nil {
		t.Fatalf("classify Codex project location: %v", err)
	}
	if source != SourceProject || kind != KindSkill || tool != ToolCodex || originRepo != inner {
		t.Fatalf("classify Codex project = (%q, %q, %q, %q), want (%q, %q, %q, %q)", source, kind, tool, originRepo, SourceProject, KindSkill, ToolCodex, inner)
	}
}

func TestNewArchiveIDSurfacesStatError(t *testing.T) {
	root := t.TempDir()
	e := New(Roots{DataDir: root})
	archiveRoot := filepath.Join(root, "archive")
	if err := os.WriteFile(archiveRoot, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write archive root file: %v", err)
	}

	_, err := e.newArchiveID("skill")
	if err == nil {
		t.Fatalf("expected error when archive root is not a directory, got nil")
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	data := []byte("hello world\n")

	if err := writeFileAtomic(path, data, 0o640); err != nil {
		t.Fatalf("write file atomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("data = %q, want %q", got, data)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("perm = %o, want 0o640", info.Mode().Perm())
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestMoveDirectoryCopiesTreeAndRemovesSource(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	dst := filepath.Join(t.TempDir(), "dst")
	if err := os.MkdirAll(filepath.Join(src, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "nested", "file.txt"), []byte("content"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := moveDirectory(src, dst); err != nil {
		t.Fatalf("move directory: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source still exists after move")
	}
	got, err := os.ReadFile(filepath.Join(dst, "nested", "file.txt"))
	if err != nil {
		t.Fatalf("read moved file: %v", err)
	}
	if string(got) != "content" {
		t.Fatalf("moved content = %q, want %q", got, "content")
	}
}

func TestMoveFileCopiesAndRemovesSource(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.txt")
	dst := filepath.Join(t.TempDir(), "dst.txt")
	if err := os.WriteFile(src, []byte("prompt"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	if err := moveFile(src, dst, 0o644); err != nil {
		t.Fatalf("move file: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source still exists after move")
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read moved file: %v", err)
	}
	if string(got) != "prompt" {
		t.Fatalf("moved content = %q, want %q", got, "prompt")
	}
}
