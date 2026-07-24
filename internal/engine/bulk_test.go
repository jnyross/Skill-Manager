package engine_test

// Bulk Activation changes (internal/engine/bulk.go). The lock-acquisition
// count is asserted in the internal test file instead (bulk_internal_test.go),
// where the lock's own primitives are visible.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

// bulkFixture builds one throwaway machine holding, in this order: a Personal
// Skill that Auto-activates, a Personal Skill already Manual-only, a Codex
// prompt (no Auto-activation to change), and a Plugin Skill (Suppress, not
// Manual-only, is its control).
func bulkFixture(t *testing.T) (fixture, *engine.Engine) {
	t.Helper()
	f := newFixture(t)
	writeSkill(t, filepath.Join(f.roots.ClaudeHome, "skills", "auto-one"), "auto-one", "Auto one description", "")
	writeSkill(t, filepath.Join(f.roots.ClaudeHome, "skills", "auto-two"), "auto-two", "Auto two description", "")
	writeSkill(t, filepath.Join(f.roots.ClaudeHome, "skills", "already"), "already", "Already manual", "disable-model-invocation: true\n")
	writePrompt(t, filepath.Join(f.roots.CodexHome, "prompts", "deploy.md"), "Deploy the service")

	pluginInstall := filepath.Join(f.roots.ClaudeHome, "plugins", "cache", "demo")
	writeSkill(t, filepath.Join(pluginInstall, "skills", "lint"), "lint", "Lint the tree", "")
	writePluginManifest(t, f.roots.ClaudeHome, map[string][]map[string]string{
		"demo@market": {{"scope": "user", "installPath": pluginInstall}},
	})
	return f, engine.New(f.roots)
}

func mustFind(t *testing.T, inv engine.Inventory, source engine.Source, name string) engine.Skill {
	t.Helper()
	skill, ok := findSkill(inv, source, name)
	if !ok {
		t.Fatalf("%s Skill %q not found in inventory", source, name)
	}
	return skill
}

func TestSetManualOnlyBulkAppliesWhatItCanAndReportsInInputOrder(t *testing.T) {
	f, e := bulkFixture(t)
	inv := e.Inventory()
	input := []engine.Skill{
		mustFind(t, inv, engine.SourcePersonal, "auto-one"),
		mustFind(t, inv, engine.SourcePersonal, "already"),
		mustFind(t, inv, engine.SourceCodex, "deploy"),
		mustFind(t, inv, engine.SourcePlugin, "lint"),
		mustFind(t, inv, engine.SourcePersonal, "auto-two"),
	}

	results := e.SetManualOnlyBulk(input, true)

	if len(results) != len(input) {
		t.Fatalf("got %d results for %d Skills", len(results), len(input))
	}
	for index, result := range results {
		if result.Skill.Name != input[index].Name || result.Skill.Source != input[index].Source {
			t.Fatalf("result %d is %s %q, want %s %q — results must keep input order",
				index, result.Skill.Source, result.Skill.Name, input[index].Source, input[index].Name)
		}
	}
	for _, index := range []int{0, 1, 4} {
		if results[index].Err != nil {
			t.Fatalf("result %d (%q) failed: %v", index, results[index].Skill.Name, results[index].Err)
		}
	}
	// A Codex prompt and a Plugin Skill must say why, not be silently skipped.
	if results[2].Err == nil || !strings.Contains(results[2].Err.Error(), "Codex prompt") {
		t.Fatalf("prompt result = %v, want a descriptive error", results[2].Err)
	}
	if results[3].Err == nil || !strings.Contains(results[3].Err.Error(), "Suppress") {
		t.Fatalf("Plugin result = %v, want an error pointing at Suppress", results[3].Err)
	}

	after := e.Inventory()
	for _, name := range []string{"auto-one", "auto-two", "already"} {
		if got := mustFind(t, after, engine.SourcePersonal, name).Activation; got != engine.ActivationManualOnly {
			t.Fatalf("%q Activation = %q, want Manual-only", name, got)
		}
	}
	if got := mustFind(t, after, engine.SourcePlugin, "lint").Activation; got != engine.ActivationAuto {
		t.Fatalf("a rejected Plugin Skill must be untouched, got Activation %q", got)
	}
	// The already-Manual-only Skill must not have been rewritten at all.
	content := readFileString(t, filepath.Join(f.roots.ClaudeHome, "skills", "already", "SKILL.md"))
	if strings.Count(content, "disable-model-invocation") != 1 {
		t.Fatalf("an already-Manual-only Skill was rewritten:\n%s", content)
	}
}

func TestSetManualOnlyBulkSkipsAlreadyCorrectSkillsWithoutWriting(t *testing.T) {
	f, e := bulkFixture(t)
	skill := mustFind(t, e.Inventory(), engine.SourcePersonal, "already")
	path := filepath.Join(f.roots.ClaudeHome, "skills", "already", "SKILL.md")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	beforeContent := readFileString(t, path)

	results := e.SetManualOnlyBulk([]engine.Skill{skill}, true)
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("results = %+v, want one nil-error result", results)
	}

	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if readFileString(t, path) != beforeContent {
		t.Fatalf("an already-Manual-only Skill was rewritten")
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("an already-Manual-only Skill was rewritten (mtime moved)")
	}
}

func TestSetManualOnlyBulkRestoresAutoActivation(t *testing.T) {
	_, e := bulkFixture(t)
	inv := e.Inventory()
	if results := e.SetManualOnlyBulk([]engine.Skill{
		mustFind(t, inv, engine.SourcePersonal, "auto-one"),
		mustFind(t, inv, engine.SourcePersonal, "auto-two"),
	}, true); results[0].Err != nil || results[1].Err != nil {
		t.Fatalf("setting Manual-only failed: %+v", results)
	}

	inv = e.Inventory()
	results := e.SetManualOnlyBulk([]engine.Skill{
		mustFind(t, inv, engine.SourcePersonal, "auto-one"),
		mustFind(t, inv, engine.SourcePersonal, "auto-two"),
	}, false)
	for index, result := range results {
		if result.Err != nil {
			t.Fatalf("restoring Auto for result %d failed: %v", index, result.Err)
		}
	}
	after := e.Inventory()
	for _, name := range []string{"auto-one", "auto-two"} {
		if got := mustFind(t, after, engine.SourcePersonal, name).Activation; got != engine.ActivationAuto {
			t.Fatalf("%q Activation = %q, want Auto", name, got)
		}
	}
}

func TestSetManualOnlyBulkContinuesPastAnUnwritableSkill(t *testing.T) {
	_, e := bulkFixture(t)
	inv := e.Inventory()
	blocked := mustFind(t, inv, engine.SourcePersonal, "auto-one")
	// Replace the Skill's own SKILL.md with a directory: the frontmatter edit
	// cannot succeed, but the rest of the sweep must still run.
	skillMD := filepath.Join(blocked.Location, "SKILL.md")
	if err := os.Remove(skillMD); err != nil {
		t.Fatalf("remove SKILL.md: %v", err)
	}
	if err := os.Mkdir(skillMD, 0o755); err != nil {
		t.Fatalf("mkdir over SKILL.md: %v", err)
	}

	results := e.SetManualOnlyBulk([]engine.Skill{
		blocked,
		mustFind(t, inv, engine.SourcePersonal, "auto-two"),
	}, true)

	if results[0].Err == nil {
		t.Fatalf("an unwritable Skill must report its failure")
	}
	if results[1].Err != nil {
		t.Fatalf("one unwritable Skill must not block the rest: %v", results[1].Err)
	}
	if got := mustFind(t, e.Inventory(), engine.SourcePersonal, "auto-two").Activation; got != engine.ActivationManualOnly {
		t.Fatalf("auto-two Activation = %q, want Manual-only", got)
	}
}

func TestSetManualOnlyBulkOnAnEmptyListDoesNothing(t *testing.T) {
	_, e := bulkFixture(t)
	if results := e.SetManualOnlyBulk(nil, true); len(results) != 0 {
		t.Fatalf("results = %+v, want none", results)
	}
}

// --- savings arithmetic ---

func TestEstimateManualOnlySavingsCountsOnlyWhatWouldActuallyChange(t *testing.T) {
	skills := []engine.Skill{
		{Name: "auto-a", Source: engine.SourcePersonal, Kind: engine.KindSkill, Activation: engine.ActivationAuto, DescriptionTokens: 40},
		{Name: "auto-b", Source: engine.SourcePersonal, Kind: engine.KindSkill, Activation: engine.ActivationAuto, DescriptionTokens: 60},
		// Already Manual-only: costs nothing per session, so saves nothing.
		{Name: "manual", Source: engine.SourcePersonal, Kind: engine.KindSkill, Activation: engine.ActivationManualOnly, DescriptionTokens: 500},
		// Neither is offered to the model on its own judgement either.
		{Name: "disabled", Source: engine.SourceCodex, Kind: engine.KindSkill, Activation: engine.ActivationDisabled, DescriptionTokens: 500},
		{Name: "suppressed", Source: engine.SourcePlugin, Kind: engine.KindSkill, Activation: engine.ActivationSuppressed, DescriptionTokens: 500},
		// Auto, but Skillet cannot change their Activation, so promising their
		// tokens as a saving would be a lie.
		{Name: "plugin", Source: engine.SourcePlugin, Kind: engine.KindSkill, Activation: engine.ActivationAuto, DescriptionTokens: 500},
		{Name: "prompt", Source: engine.SourceCodex, Kind: engine.KindPrompt, Activation: engine.ActivationAuto, DescriptionTokens: 500},
	}

	savings := engine.EstimateManualOnlySavings(skills)
	if savings.Skills != 2 || savings.Tokens != 100 {
		t.Fatalf("savings = %+v, want {Skills:2 Tokens:100}", savings)
	}
	if empty := engine.EstimateManualOnlySavings(nil); empty.Skills != 0 || empty.Tokens != 0 {
		t.Fatalf("empty savings = %+v, want zero", empty)
	}
}

// The saving a sweep previews must be exactly the drop SummarizeContextCost
// reports after it runs — a preview that does not match the outcome is worse
// than no preview.
func TestEstimateManualOnlySavingsMatchesTheRealContextCostDrop(t *testing.T) {
	_, e := bulkFixture(t)
	inv := e.Inventory()
	before := engine.SummarizeContextCost(inv.Skills)

	var targets []engine.Skill
	for _, skill := range inv.Skills {
		if skill.Activation == engine.ActivationAuto && engine.ActivationChangeSupported(skill) == nil {
			targets = append(targets, skill)
		}
	}
	predicted := engine.EstimateManualOnlySavings(targets)
	if predicted.Skills == 0 {
		t.Fatalf("fixture has no Auto-activating changeable Skills")
	}

	for index, result := range e.SetManualOnlyBulk(targets, true) {
		if result.Err != nil {
			t.Fatalf("bulk result %d failed: %v", index, result.Err)
		}
	}

	after := engine.SummarizeContextCost(e.Inventory().Skills)
	if got := before.DescriptionTokens - after.DescriptionTokens; got != predicted.Tokens {
		t.Fatalf("actual drop = %d tokens, preview promised %d", got, predicted.Tokens)
	}
	if got := before.Skills - after.Skills; got != predicted.Skills {
		t.Fatalf("actual Skill drop = %d, preview promised %d", got, predicted.Skills)
	}
}

func TestActivationChangeSupportedAndAlreadySet(t *testing.T) {
	personal := engine.Skill{Name: "p", Source: engine.SourcePersonal, Kind: engine.KindSkill, Activation: engine.ActivationAuto}
	if err := engine.ActivationChangeSupported(personal); err != nil {
		t.Fatalf("a Personal Skill must be changeable: %v", err)
	}
	if !engine.ActivationAlreadySet(personal, false) {
		t.Fatalf("an Auto Skill is already in the Auto state")
	}
	if engine.ActivationAlreadySet(personal, true) {
		t.Fatalf("an Auto Skill is not already Manual-only")
	}
	// Suppressed and Disabled are neither state, so neither direction is a
	// no-op for them.
	suppressed := engine.Skill{Name: "s", Source: engine.SourceCodex, Kind: engine.KindSkill, Activation: engine.ActivationDisabled}
	if engine.ActivationAlreadySet(suppressed, true) || engine.ActivationAlreadySet(suppressed, false) {
		t.Fatalf("a Disabled Skill must not count as already set either way")
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
