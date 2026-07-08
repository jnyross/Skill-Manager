package engine

import (
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
