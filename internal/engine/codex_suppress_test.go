package engine_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

// Codex config.toml shape ([[skills.config]] path/name/enabled) encodes the
// conventions verified locally in docs/research/skill-mechanisms.md.

func TestSuppressCodexSkillWritesNativeDisableEntryPreservingRest(t *testing.T) {
	f := newFixture(t)
	skillFolder := writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "codex-skill"), "codex-skill", "Codex description", "")
	skillMD := filepath.Join(skillFolder, "SKILL.md")

	configBefore := "[profile]\nname = \"default\"\n"
	writeFile(t, filepath.Join(f.roots.CodexHome, "config.toml"), configBefore)

	e := engine.New(f.roots)
	skill, ok := findSkill(e.Inventory(), engine.SourceCodex, "codex-skill")
	if !ok {
		t.Fatalf("codex skill not found")
	}
	if skill.Activation != engine.ActivationAuto {
		t.Fatalf("activation before suppress = %q, want Auto", skill.Activation)
	}

	if err := e.Suppress(skill); err != nil {
		t.Fatalf("Suppress: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(f.roots.CodexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config after suppress: %v", err)
	}
	want := configBefore + "[[skills.config]]\npath = " + strconv.Quote(skillMD) + "\nenabled = false\n"
	if string(got) != want {
		t.Fatalf("config after suppress = %q, want %q", string(got), want)
	}

	suppressed, ok := findSkill(e.Inventory(), engine.SourceCodex, "codex-skill")
	if !ok {
		t.Fatalf("codex skill missing after suppress")
	}
	if suppressed.Activation != engine.ActivationDisabled {
		t.Fatalf("activation after suppress = %q, want Disabled", suppressed.Activation)
	}
}

func TestSuppressCodexSkillIsIdempotent(t *testing.T) {
	f := newFixture(t)
	writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "codex-skill"), "codex-skill", "Codex description", "")

	e := engine.New(f.roots)
	skill, _ := findSkill(e.Inventory(), engine.SourceCodex, "codex-skill")
	if err := e.Suppress(skill); err != nil {
		t.Fatalf("first Suppress: %v", err)
	}
	afterFirst, err := os.ReadFile(filepath.Join(f.roots.CodexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	skill2, _ := findSkill(e.Inventory(), engine.SourceCodex, "codex-skill")
	if err := e.Suppress(skill2); err != nil {
		t.Fatalf("second Suppress: %v", err)
	}
	afterSecond, err := os.ReadFile(filepath.Join(f.roots.CodexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config after second suppress: %v", err)
	}
	if string(afterFirst) != string(afterSecond) {
		t.Fatalf("suppress is not idempotent: first=%q second=%q", afterFirst, afterSecond)
	}
}

func TestSuppressCodexKeysNewEntryByPathNotName(t *testing.T) {
	f := newFixture(t)
	writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "codex-skill"), "codex-skill", "Codex description", "")

	e := engine.New(f.roots)
	skill, _ := findSkill(e.Inventory(), engine.SourceCodex, "codex-skill")
	if err := e.Suppress(skill); err != nil {
		t.Fatalf("Suppress: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(f.roots.CodexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(got), "path = ") {
		t.Fatalf("expected new entry to be keyed by path, got: %q", got)
	}
	if strings.Contains(string(got), "name = ") {
		t.Fatalf("expected new entry not to be keyed by name, got: %q", got)
	}
}

func TestSuppressProjectCodexSkillDispatchesToCodexMechanism(t *testing.T) {
	f := newFixture(t)
	folder := writeSkill(t, filepath.Join(f.root, "repo", ".agents", "skills", "project-codex"), "project-codex", "Project Codex description", "")
	skillMD := filepath.Join(folder, "SKILL.md")
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
		t.Fatalf("Suppress: %v", err)
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config after suppress: %v", err)
	}
	want := "[[skills.config]]\npath = " + strconv.Quote(skillMD) + "\nenabled = false\n"
	if string(got) != want {
		t.Fatalf("config after suppress = %q, want %q", string(got), want)
	}

	if err := e.Unsuppress(skill); err != nil {
		t.Fatalf("Unsuppress: %v", err)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("config.toml should not exist after Unsuppress round trip, got err=%v", err)
	}
}

func TestUnsuppressCodexSkillRoundTripsConfigCreatedFromScratch(t *testing.T) {
	f := newFixture(t)
	writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "codex-skill"), "codex-skill", "Codex description", "")
	configPath := filepath.Join(f.roots.CodexHome, "config.toml")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("test setup invalid: config.toml already exists")
	}

	before := snapshotTree(t, f.root)
	e := engine.New(f.roots)
	skill, _ := findSkill(e.Inventory(), engine.SourceCodex, "codex-skill")

	if err := e.Suppress(skill); err != nil {
		t.Fatalf("Suppress: %v", err)
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config.toml should exist after Suppress: %v", err)
	}

	suppressed, _ := findSkill(e.Inventory(), engine.SourceCodex, "codex-skill")
	if err := e.Unsuppress(suppressed); err != nil {
		t.Fatalf("Unsuppress: %v", err)
	}

	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("config.toml should not exist after Unsuppress round trip, got err=%v", err)
	}

	after := snapshotTree(t, f.root)
	assertSnapshotsEqual(t, before, after)
}

func TestUnsuppressCodexSkillRoundTripsPreservingExistingConfig(t *testing.T) {
	f := newFixture(t)
	writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "codex-skill"), "codex-skill", "Codex description", "")
	configBefore := "[profile]\nname = \"default\"\n\n[[skills.config]]\nname = \"other-skill\"\nenabled = false\n"
	writeFile(t, filepath.Join(f.roots.CodexHome, "config.toml"), configBefore)

	before := snapshotTree(t, f.root)
	e := engine.New(f.roots)
	skill, _ := findSkill(e.Inventory(), engine.SourceCodex, "codex-skill")

	if err := e.Suppress(skill); err != nil {
		t.Fatalf("Suppress: %v", err)
	}
	suppressed, _ := findSkill(e.Inventory(), engine.SourceCodex, "codex-skill")
	if err := e.Unsuppress(suppressed); err != nil {
		t.Fatalf("Unsuppress: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(f.roots.CodexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config after round trip: %v", err)
	}
	if string(got) != configBefore {
		t.Fatalf("config after round trip = %q, want %q", string(got), configBefore)
	}

	after := snapshotTree(t, f.root)
	assertSnapshotsEqual(t, before, after)
}

// Per docs/research/skill-mechanisms.md, name-keying is inferred/observed
// (not documented) for entries like `render:render-debug`; Suppress always
// writes path-keyed entries (the documented key), but Un-suppress must
// still remove a pre-existing name-keyed entry for the same skill, since
// readCodexDisabledConfig (codex.go) already matches either form when
// deciding a skill's Activation.
func TestUnsuppressCodexSkillMatchesByNameKeyedEntry(t *testing.T) {
	f := newFixture(t)
	writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "codex-skill"), "codex-skill", "Codex description", "")
	configBefore := "[[skills.config]]\nname = \"codex-skill\"\nenabled = false\n"
	writeFile(t, filepath.Join(f.roots.CodexHome, "config.toml"), configBefore)

	e := engine.New(f.roots)
	skill, _ := findSkill(e.Inventory(), engine.SourceCodex, "codex-skill")
	if skill.Activation != engine.ActivationDisabled {
		t.Fatalf("activation with name-keyed entry = %q, want Disabled", skill.Activation)
	}

	if err := e.Unsuppress(skill); err != nil {
		t.Fatalf("Unsuppress: %v", err)
	}

	// Ownership rule (see ownership.go): the entry was hand-written, not
	// Skillet's, so removing it empties config.toml but does not delete it —
	// Skillet only deletes a config.toml it created itself.
	got, err := os.ReadFile(filepath.Join(f.roots.CodexHome, "config.toml"))
	if err != nil {
		t.Fatalf("config.toml should be left in place when Skillet did not create it: %v", err)
	}
	if strings.TrimSpace(string(got)) != "" {
		t.Fatalf("config.toml after unsuppress = %q, want empty", got)
	}

	reverted, ok := findSkill(e.Inventory(), engine.SourceCodex, "codex-skill")
	if !ok {
		t.Fatalf("codex skill missing after unsuppress")
	}
	if reverted.Activation != engine.ActivationAuto {
		t.Fatalf("activation after unsuppress = %q, want Auto", reverted.Activation)
	}
}

func TestUnsuppressCodexSkillNoMatchingEntryIsNoop(t *testing.T) {
	f := newFixture(t)
	writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "plain-skill"), "plain-skill", "Plain skill", "")
	configBefore := "[[skills.config]]\nname = \"unrelated-skill\"\nenabled = false\n"
	writeFile(t, filepath.Join(f.roots.CodexHome, "config.toml"), configBefore)

	e := engine.New(f.roots)
	skill, _ := findSkill(e.Inventory(), engine.SourceCodex, "plain-skill")
	if err := e.Unsuppress(skill); err != nil {
		t.Fatalf("Unsuppress: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(f.roots.CodexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(got) != configBefore {
		t.Fatalf("config = %q, want untouched %q", string(got), configBefore)
	}
}

func TestUnsuppressCodexSkillNoConfigFileIsNoop(t *testing.T) {
	f := newFixture(t)
	writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "plain-skill"), "plain-skill", "Plain skill", "")

	e := engine.New(f.roots)
	skill, _ := findSkill(e.Inventory(), engine.SourceCodex, "plain-skill")
	if err := e.Unsuppress(skill); err != nil {
		t.Fatalf("Unsuppress on a skill with no config.toml at all: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.roots.CodexHome, "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("Unsuppress should not create config.toml, got err=%v", err)
	}
}

func TestSuppressAndUnsuppressRejectNonCodexSkillSourcesAndKinds(t *testing.T) {
	f := newFixture(t)
	writeSkill(t, filepath.Join(f.roots.ClaudeHome, "skills", "personal-one"), "personal-one", "A personal skill", "")
	writePrompt(t, filepath.Join(f.roots.CodexHome, "prompts", "ideas.md"), "Ideas prompt")

	e := engine.New(f.roots)
	inv := e.Inventory()

	personal, ok := findSkill(inv, engine.SourcePersonal, "personal-one")
	if !ok {
		t.Fatalf("personal skill not found")
	}
	if err := e.Suppress(personal); err == nil {
		t.Fatalf("Suppress on a Personal skill should return an error")
	}
	if err := e.Unsuppress(personal); err == nil {
		t.Fatalf("Unsuppress on a Personal skill should return an error")
	}

	prompt, ok := findSkill(inv, engine.SourceCodex, "ideas")
	if !ok {
		t.Fatalf("codex prompt not found")
	}
	if err := e.Suppress(prompt); err == nil {
		t.Fatalf("Suppress on a Codex prompt should return an error")
	}
	if err := e.Unsuppress(prompt); err == nil {
		t.Fatalf("Unsuppress on a Codex prompt should return an error")
	}

	projectClaude := engine.Skill{
		Name:     "project-claude",
		Source:   engine.SourceProject,
		Tool:     engine.ToolClaudeCode,
		Kind:     engine.KindSkill,
		Location: filepath.Join(f.root, "repo", ".claude", "skills", "project-claude"),
	}
	if err := e.Suppress(projectClaude); err == nil {
		t.Fatalf("Suppress on a Project Claude Code skill should return an error")
	}
	if err := e.Unsuppress(projectClaude); err == nil {
		t.Fatalf("Unsuppress on a Project Claude Code skill should return an error")
	}
}

func TestSuppressReadOnlySessionLeavesCodexTreeByteIdenticalWhenNoSuppressions(t *testing.T) {
	f := newFixture(t)
	writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "codex-skill"), "codex-skill", "Codex description", "")
	configBefore := "[[skills.config]]\nname = \"other-skill\"\nenabled = false\n"
	writeFile(t, filepath.Join(f.roots.CodexHome, "config.toml"), configBefore)

	e := engine.New(f.roots)
	before := snapshotTree(t, f.root)
	_ = e.Inventory()
	_ = e.Inventory()
	after := snapshotTree(t, f.root)
	assertSnapshotsEqual(t, before, after)
}
