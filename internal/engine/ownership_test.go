package engine_test

// Write-ownership: Skillet removes only the config.toml entries it wrote,
// never deletes a config.toml or a policy file it did not create, refuses to
// follow a symlinked Codex policy file out of the skill tree, and refuses to
// write into a location that vanished between the scan and the action.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

func TestUnsuppressCodexRemovesOnlySkilletAuthoredEntry(t *testing.T) {
	f := newFixture(t)
	folder := writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "codex-skill"), "codex-skill", "Codex description", "")
	skillMD := filepath.Join(folder, "SKILL.md")
	// A hand-written entry for the *same* skill, carrying a key Skillet never
	// writes. Skillet's own entry is added on top by Suppress, keyed by path.
	handWritten := "[[skills.config]]\nname = \"codex-skill\"\nenabled = false\nnotes = \"hand tuned\"\n"
	writeFile(t, filepath.Join(f.roots.CodexHome, "config.toml"), "[profile]\nname = \"default\"\n\n"+handWritten)

	e := engine.New(f.roots)
	skill := engine.Skill{Name: "codex-skill", Source: engine.SourceCodex, Kind: engine.KindSkill, Location: folder}
	// Suppress would no-op on an already-disabled skill, so write Skillet's
	// own block the way suppressCodex does by suppressing a skill whose
	// hand-written entry names a different skill first.
	writeFile(t, filepath.Join(f.roots.CodexHome, "config.toml"), "[profile]\nname = \"default\"\n\n[[skills.config]]\nname = \"other-skill\"\nenabled = false\n")
	if err := e.Suppress(skill); err != nil {
		t.Fatalf("Suppress: %v", err)
	}
	// Now add the hand-written entry for the same skill alongside Skillet's.
	current, err := os.ReadFile(filepath.Join(f.roots.CodexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	writeFile(t, filepath.Join(f.roots.CodexHome, "config.toml"), string(current)+handWritten)

	err = e.Unsuppress(skill)
	if err == nil {
		t.Fatalf("Unsuppress should report the hand-written entry it deliberately left in place")
	}
	if !strings.Contains(err.Error(), "hand-written") {
		t.Fatalf("error = %v, want it to name the hand-written entry", err)
	}

	got, err := os.ReadFile(filepath.Join(f.roots.CodexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config after unsuppress: %v", err)
	}
	if !strings.Contains(string(got), "hand tuned") {
		t.Fatalf("the hand-written entry was removed:\n%s", got)
	}
	if strings.Contains(string(got), strconv.Quote(skillMD)) {
		t.Fatalf("Skillet's own path-keyed entry should be gone:\n%s", got)
	}
	if !strings.Contains(string(got), "other-skill") {
		t.Fatalf("an unrelated entry was lost:\n%s", got)
	}
}

func TestUnsuppressCodexNeverDeletesAConfigItDidNotCreate(t *testing.T) {
	f := newFixture(t)
	folder := writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "codex-skill"), "codex-skill", "Codex description", "")
	configPath := filepath.Join(f.roots.CodexHome, "config.toml")
	// The user's own config.toml, empty but present — a file Skillet must not
	// delete even though removing its own block leaves nothing behind.
	writeFile(t, configPath, "")

	e := engine.New(f.roots)
	skill := engine.Skill{Name: "codex-skill", Source: engine.SourceCodex, Kind: engine.KindSkill, Location: folder}
	if err := e.Suppress(skill); err != nil {
		t.Fatalf("Suppress: %v", err)
	}
	if err := e.Unsuppress(skill); err != nil {
		t.Fatalf("Unsuppress: %v", err)
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config.toml must survive an un-suppress when Skillet did not create it: %v", err)
	}
	if strings.TrimSpace(string(got)) != "" {
		t.Fatalf("config.toml = %q, want empty", got)
	}
}

func TestUnsetManualOnlyNeverDeletesAPolicyFileItDidNotCreate(t *testing.T) {
	f := newFixture(t)
	folder := writeSkill(t, filepath.Join(f.roots.AgentsHome, "skills", "codex-loop"), "codex-loop", "Codex loop description", "")
	policyPath := filepath.Join(folder, "agents", "openai.yaml")
	// A policy file the user already had, whose only content is the field
	// Skillet also manages: removing the field empties it, but the file is
	// not Skillet's to delete.
	writeFile(t, policyPath, "policy:\n  allow_implicit_invocation: false\n")

	e := engine.New(f.roots)
	skill, ok := findSkill(e.Inventory(), engine.SourceCodex, "codex-loop")
	if !ok {
		t.Fatalf("codex skill not found")
	}
	if err := e.SetManualOnly(skill, false); err != nil {
		t.Fatalf("SetManualOnly(false): %v", err)
	}

	got, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("policy file must survive when Skillet did not create it: %v", err)
	}
	if strings.TrimSpace(string(got)) != "" {
		t.Fatalf("policy file = %q, want empty", got)
	}
}

func TestSetManualOnlyRejectsSymlinkedCodexPolicyFile(t *testing.T) {
	f := newFixture(t)
	folder := writeSkill(t, filepath.Join(f.roots.AgentsHome, "skills", "codex-loop"), "codex-loop", "Codex loop description", "")
	outside := filepath.Join(f.root, "outside.yaml")
	writeFile(t, outside, "policy:\n  something_else: true\n")
	mkdirAll(t, filepath.Join(folder, "agents"))
	if err := os.Symlink(outside, filepath.Join(folder, "agents", "openai.yaml")); err != nil {
		t.Fatalf("symlink policy file: %v", err)
	}

	e := engine.New(f.roots)
	skill := engine.Skill{Name: "codex-loop", Source: engine.SourceCodex, Kind: engine.KindSkill, Location: folder}
	if err := e.SetManualOnly(skill, true); err == nil {
		t.Fatalf("SetManualOnly should refuse to follow a symlinked openai.yaml")
	}
	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatalf("read outside file: %v", err)
	}
	if strings.Contains(string(got), "allow_implicit_invocation") {
		t.Fatalf("the write followed the symlink out of the skill tree:\n%s", got)
	}
}

func TestSetManualOnlyRejectsSymlinkedAgentsDirectory(t *testing.T) {
	f := newFixture(t)
	folder := writeSkill(t, filepath.Join(f.roots.AgentsHome, "skills", "codex-loop"), "codex-loop", "Codex loop description", "")
	outsideDir := filepath.Join(f.root, "outside-agents")
	mkdirAll(t, outsideDir)
	if err := os.Symlink(outsideDir, filepath.Join(folder, "agents")); err != nil {
		t.Fatalf("symlink agents dir: %v", err)
	}

	e := engine.New(f.roots)
	skill := engine.Skill{Name: "codex-loop", Source: engine.SourceCodex, Kind: engine.KindSkill, Location: folder}
	if err := e.SetManualOnly(skill, true); err == nil {
		t.Fatalf("SetManualOnly should refuse a symlinked agents/ directory")
	}
	if _, err := os.Stat(filepath.Join(outsideDir, "openai.yaml")); !os.IsNotExist(err) {
		t.Fatalf("a policy file was created outside the skill tree, stat err = %v", err)
	}
}

// A plugin update between the user's confirm and the write replaces the cache
// directory wholesale; writing into the captured path would recreate a dead
// directory nothing reads.
func TestMutationsRefuseAStaleSkillLocation(t *testing.T) {
	f := newFixture(t)
	gone := filepath.Join(f.roots.ClaudeHome, "plugins", "cache", "catalog", "demo", "1.0.0", "skills", "vanished")

	e := engine.New(f.roots)
	pluginSkill := engine.Skill{
		Name:     "vanished",
		Source:   engine.SourcePlugin,
		Tool:     engine.ToolClaudeCode,
		Kind:     engine.KindSkill,
		Location: gone,
		Plugin:   &engine.PluginInfo{Plugin: "demo", Marketplace: "catalog"},
	}
	err := e.Suppress(pluginSkill)
	if err == nil {
		t.Fatalf("Suppress should refuse a location that no longer exists")
	}
	if !strings.Contains(err.Error(), "no longer at") {
		t.Fatalf("error = %v, want it to explain the location vanished", err)
	}
	if _, statErr := os.Stat(gone); !os.IsNotExist(statErr) {
		t.Fatalf("Suppress recreated the dead directory, stat err = %v", statErr)
	}

	personalSkill := engine.Skill{
		Name:     "vanished-personal",
		Source:   engine.SourcePersonal,
		Tool:     engine.ToolClaudeCode,
		Kind:     engine.KindSkill,
		Location: filepath.Join(f.roots.ClaudeHome, "skills", "vanished-personal"),
	}
	if err := e.SetManualOnly(personalSkill, true); err == nil || !strings.Contains(err.Error(), "no longer at") {
		t.Fatalf("SetManualOnly error = %v, want a stale-location error", err)
	}

	codexSkill := engine.Skill{
		Name:     "vanished-codex",
		Source:   engine.SourceCodex,
		Tool:     engine.ToolCodex,
		Kind:     engine.KindSkill,
		Location: filepath.Join(f.roots.CodexHome, "skills", "vanished-codex"),
	}
	if err := e.SetManualOnly(codexSkill, true); err == nil || !strings.Contains(err.Error(), "no longer at") {
		t.Fatalf("SetManualOnly (Codex) error = %v, want a stale-location error", err)
	}
	if _, statErr := os.Stat(filepath.Join(codexSkill.Location, "agents")); !os.IsNotExist(statErr) {
		t.Fatalf("SetManualOnly created agents/ under a dead location, stat err = %v", statErr)
	}
}

// Unsuppress is deliberately exempt: when the plugin's cached copy has gone,
// the Skillet-owned record is exactly what the user still needs to clear.
func TestUnsuppressStillClearsTheRecordWhenThePluginCacheIsGone(t *testing.T) {
	f := newFixture(t)
	folder := writeSkill(t, filepath.Join(f.roots.ClaudeHome, "plugins", "cache", "catalog", "demo", "1.0.0", "skills", "demo-skill"), "demo-skill", "Demo", "")
	skill := engine.Skill{
		Name:     "demo-skill",
		Source:   engine.SourcePlugin,
		Tool:     engine.ToolClaudeCode,
		Kind:     engine.KindSkill,
		Location: folder,
		Plugin:   &engine.PluginInfo{Plugin: "demo", Marketplace: "catalog"},
	}

	e := engine.New(f.roots)
	if err := e.Suppress(skill); err != nil {
		t.Fatalf("Suppress: %v", err)
	}
	if err := os.RemoveAll(folder); err != nil {
		t.Fatalf("remove cached plugin skill: %v", err)
	}

	if err := e.Unsuppress(skill); err != nil {
		t.Fatalf("Unsuppress should still clear the record for a vanished plugin skill: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(f.roots.DataDir, "suppressed"))
	if err == nil && len(entries) != 0 {
		t.Fatalf("suppression record survived: %v", entries)
	} else if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read suppressions: %v", err)
	}
}
