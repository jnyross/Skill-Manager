package engine_test

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/engine"
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
	if agent.Tool != engine.ToolCodex {
		t.Fatalf("agent skill tool = %q, want Codex", agent.Tool)
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
	if prompt.Tool != engine.ToolCodex {
		t.Fatalf("prompt tool = %q, want Codex", prompt.Tool)
	}
	if prompt.Activation != engine.ActivationManualOnly {
		t.Fatalf("prompt activation = %q, want Manual-only", prompt.Activation)
	}
	if prompt.Location != filepath.Join(f.roots.CodexHome, "prompts", "ideas.md") {
		t.Fatalf("prompt location = %q", prompt.Location)
	}
}

// TestCodexDisabledConfigEntryWithBothPathAndNameIsIgnored covers issue #12:
// Codex's real config validator (core-skills/src/config_rules.rs, per
// docs/research/skill-mechanisms.md's "Re-verification against codex-cli
// 0.143.0" section) ignores any [[skills.config]] entry that sets both
// `path` and `name` ("ignoring skills.config entry with both path and name
// selectors"), logging a warning and never disabling the skill. A malformed
// entry like this — one Skillet itself never writes, but a human or another
// tool might — must not be treated as a match by readCodexDisabledConfig,
// or Skillet's reported Activation would disagree with what Codex itself
// actually does.
func TestCodexDisabledConfigEntryWithBothPathAndNameIsIgnored(t *testing.T) {
	f := newFixture(t)
	codexSkill := writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "codex-skill"), "codex-skill", "Codex description", "")
	writeFile(t, filepath.Join(f.roots.CodexHome, "config.toml"),
		"[[skills.config]]\npath = "+strconv.Quote(filepath.Join(codexSkill, "SKILL.md"))+"\nname = \"codex-skill\"\nenabled = false\n")

	inv := engine.New(f.roots).Inventory()
	codex, ok := findSkill(inv, engine.SourceCodex, "codex-skill")
	if !ok {
		t.Fatalf("codex skill missing: %#v", inv.Skills)
	}
	if codex.Activation == engine.ActivationDisabled {
		t.Fatalf("codex skill activation = %q, want not Disabled (entry with both path and name must be ignored)", codex.Activation)
	}
}

// TestCodexDisabledConfigEntryWithNeitherPathNorNameIsIgnored is a
// regression test for the "neither selector" half of the same Codex
// validation rule ("ignoring skills.config entry without a path or name
// selector") — already true before issue #12's fix, since the old code
// only ever populated result.paths/result.names when a key was non-empty,
// but pinned down explicitly here alongside the "both" case above.
func TestCodexDisabledConfigEntryWithNeitherPathNorNameIsIgnored(t *testing.T) {
	f := newFixture(t)
	writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "codex-skill"), "codex-skill", "Codex description", "")
	writeFile(t, filepath.Join(f.roots.CodexHome, "config.toml"), "[[skills.config]]\nenabled = false\n")

	inv := engine.New(f.roots).Inventory()
	codex, ok := findSkill(inv, engine.SourceCodex, "codex-skill")
	if !ok {
		t.Fatalf("codex skill missing: %#v", inv.Skills)
	}
	if codex.Activation == engine.ActivationDisabled {
		t.Fatalf("codex skill activation = %q, want not Disabled (entry with neither path nor name must be ignored)", codex.Activation)
	}
}

// TestCodexSkillDeclaresManualOnlyForClaude covers the activation-semantics
// finding: a Codex skill whose SKILL.md declares disable-model-invocation:
// true is manual-only for Claude Code, but Codex runtime activation is
// governed by agents/openai.yaml, so without an explicit disallow there the
// skill remains Auto in Codex and the detail pane must surface the mismatch.
func TestCodexSkillDeclaresManualOnlyForClaude(t *testing.T) {
	f := newFixture(t)
	manualOnlyClaude := writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "manual-claude"), "manual-claude", "Codex description", "disable-model-invocation: true\n")

	inv := engine.New(f.roots).Inventory()
	skill, ok := findSkill(inv, engine.SourceCodex, "manual-claude")
	if !ok {
		t.Fatalf("skill missing: %#v", inv.Skills)
	}
	if skill.Location != manualOnlyClaude {
		t.Fatalf("skill location = %q, want %q", skill.Location, manualOnlyClaude)
	}
	if skill.Activation != engine.ActivationAuto {
		t.Fatalf("skill activation = %q, want Auto (Codex ignores disable-model-invocation)", skill.Activation)
	}
	if !skill.DeclaredManualOnlyForClaude {
		t.Fatalf("skill DeclaredManualOnlyForClaude = false, want true")
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
