package engine_test

import (
	"path/filepath"
	"testing"

	"skillet/internal/engine"
)

func TestProjectClaudeSkillAtRepoRoot(t *testing.T) {
	f := newFixture(t)
	repo := filepath.Join(f.root, "repo")
	cwd := filepath.Join(repo, "alpha", "bravo", "charlie")
	mkdirAll(t, filepath.Join(repo, ".git"), cwd)
	location := writeSkill(t, filepath.Join(repo, ".claude", "skills", "repo-claude"), "repo-claude", "Repo Claude description", "")

	f.roots.ProjectRoots = engine.FindProjectRoots(cwd)
	f.roots.ClaudeProjectRoots = engine.FindClaudeProjectRoots(cwd)

	inv := engine.New(f.roots).Inventory()
	if len(inv.Notices) != 0 {
		t.Fatalf("got notices for missing project candidates, want none: %#v", inv.Notices)
	}

	skill, ok := findSkill(inv, engine.SourceProject, "repo-claude")
	if !ok {
		t.Fatalf("project Claude skill missing: %#v", inv.Skills)
	}
	if skill.Tool != engine.ToolClaudeCode {
		t.Fatalf("project Claude skill tool = %q, want %q", skill.Tool, engine.ToolClaudeCode)
	}
	if skill.Source != engine.SourceProject {
		t.Fatalf("project Claude skill source = %q, want Project", skill.Source)
	}
	if skill.Location != location {
		t.Fatalf("project Claude skill location = %q, want %q", skill.Location, location)
	}
}

func TestProjectCodexSkillManualOnly(t *testing.T) {
	f := newFixture(t)
	repo := filepath.Join(f.root, "repo")
	cwd := filepath.Join(repo, "alpha", "bravo")
	mkdirAll(t, filepath.Join(repo, ".git"), cwd)
	location := writeSkill(t, filepath.Join(repo, ".agents", "skills", "repo-codex"), "repo-codex", "Repo Codex description", "")
	writeFile(t, filepath.Join(location, "agents", "openai.yaml"), "policy:\n  allow_implicit_invocation: false\n")

	f.roots.ProjectRoots = engine.FindProjectRoots(cwd)
	f.roots.ClaudeProjectRoots = engine.FindClaudeProjectRoots(cwd)

	inv := engine.New(f.roots).Inventory()
	if len(inv.Notices) != 0 {
		t.Fatalf("got notices for missing project candidates, want none: %#v", inv.Notices)
	}

	skill, ok := findSkill(inv, engine.SourceProject, "repo-codex")
	if !ok {
		t.Fatalf("project Codex skill missing: %#v", inv.Skills)
	}
	if skill.Tool != engine.ToolCodex {
		t.Fatalf("project Codex skill tool = %q, want %q", skill.Tool, engine.ToolCodex)
	}
	if skill.Source != engine.SourceProject {
		t.Fatalf("project Codex skill source = %q, want Project", skill.Source)
	}
	if skill.Activation != engine.ActivationManualOnly {
		t.Fatalf("project Codex skill activation = %q, want Manual-only", skill.Activation)
	}
	if skill.Location != location {
		t.Fatalf("project Codex skill location = %q, want %q", skill.Location, location)
	}
}
