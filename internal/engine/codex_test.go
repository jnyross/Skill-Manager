package engine_test

import (
	"path/filepath"
	"strconv"
	"testing"

	"skillet/internal/engine"
)

// Fixture shapes here (agents/openai.yaml policy, prompt frontmatter,
// config.toml [[skills.config]] path/enabled) encode the conventions
// verified locally in docs/research/skill-mechanisms.md; re-verify
// against a fresh public Codex install before calling v1 done, per that
// doc's "Gaps / uncertainties" section.
func TestCodexSkillsPromptsAndDisabledConfig(t *testing.T) {
	f := newFixture(t)
	agentSkill := writeSkill(t, filepath.Join(f.roots.AgentsHome, "skills", "agent-skill"), "agent-skill", "Agent description", "")
	writeFile(t, filepath.Join(agentSkill, "agents", "openai.yaml"), "policy:\n  allow_implicit_invocation: false\n")
	codexSkill := writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "codex-skill"), "codex-skill", "Codex description", "")
	writePrompt(t, filepath.Join(f.roots.CodexHome, "prompts", "ideas.md"), "Ideas prompt")
	writeFile(t, filepath.Join(f.roots.CodexHome, "config.toml"), "[[skills.config]]\npath = "+strconv.Quote(filepath.Join(codexSkill, "SKILL.md"))+"\nenabled = false\n")

	inv := engine.New(f.roots).Inventory()
	agent, ok := findSkill(inv, engine.SourceCodex, "agent-skill")
	if !ok {
		t.Fatalf("agent skill missing: %#v", inv.Skills)
	}
	if agent.Location != agentSkill {
		t.Fatalf("agent skill location = %q, want %q", agent.Location, agentSkill)
	}
	if agent.Activation != engine.ActivationManualOnly {
		t.Fatalf("agent skill activation = %q, want Manual-only", agent.Activation)
	}

	codex, ok := findSkill(inv, engine.SourceCodex, "codex-skill")
	if !ok {
		t.Fatalf("codex skill missing: %#v", inv.Skills)
	}
	if codex.Activation != engine.ActivationDisabled {
		t.Fatalf("codex skill activation = %q, want Disabled", codex.Activation)
	}

	prompt, ok := findSkill(inv, engine.SourceCodex, "ideas")
	if !ok {
		t.Fatalf("prompt missing: %#v", inv.Skills)
	}
	if prompt.Kind != engine.KindPrompt {
		t.Fatalf("prompt kind = %q, want prompt", prompt.Kind)
	}
	if prompt.Activation != engine.ActivationManualOnly {
		t.Fatalf("prompt activation = %q, want Manual-only", prompt.Activation)
	}
	if prompt.Location != filepath.Join(f.roots.CodexHome, "prompts", "ideas.md") {
		t.Fatalf("prompt location = %q", prompt.Location)
	}
}

func TestCodexAgentsHomeShadowsCodexHomeOnNameCollision(t *testing.T) {
	f := newFixture(t)
	agentSkill := writeSkill(t, filepath.Join(f.roots.AgentsHome, "skills", "shared"), "shared", "Agent copy", "")
	writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "shared"), "shared", "Codex copy", "")

	inv := engine.New(f.roots).Inventory()
	var matches []engine.Skill
	for _, skill := range inv.Skills {
		if skill.Source == engine.SourceCodex && skill.Name == "shared" {
			matches = append(matches, skill)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("got %d shared codex skills, want 1: %#v", len(matches), matches)
	}
	if matches[0].Location != agentSkill {
		t.Fatalf("shadowed skill location = %q, want %q", matches[0].Location, agentSkill)
	}
	if matches[0].Description != "Agent copy" {
		t.Fatalf("shadowed skill description = %q, want Agent copy", matches[0].Description)
	}
}
