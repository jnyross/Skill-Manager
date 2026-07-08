package engine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skillet/internal/engine"
)

// setUpSuppressibleFixture creates a fixture with a single user-scoped
// plugin install (plugin-x@marketplace-x) containing one skill, and returns
// the fixture plus the paths needed to simulate a plugin update later.
func setUpSuppressibleFixture(t *testing.T) (fixture, string, string) {
	t.Helper()
	f := newFixture(t)
	installPathV1 := filepath.Join(f.root, "plugin-cache", "marketplace-x", "plugin-x", "v1")
	writeSkill(t, filepath.Join(installPathV1, "skills", "loop-me"), "loop-me", "Loop description", "version: \"1.0.0\"\n")
	writePluginManifest(t, f.roots.ClaudeHome, map[string][]map[string]string{
		"plugin-x@marketplace-x": {
			{"scope": "user", "installPath": installPathV1, "version": "1.0.0"},
		},
	})
	return f, installPathV1, filepath.Join(f.root, "plugin-cache", "marketplace-x", "plugin-x", "v2")
}

func mustFindPluginSkill(t *testing.T, inv engine.Inventory, name string) engine.Skill {
	t.Helper()
	skill, ok := findSkill(inv, engine.SourcePlugin, name)
	if !ok {
		t.Fatalf("plugin skill %s not found: %#v", name, inv.Skills)
	}
	return skill
}

func TestSuppressEditsFrontmatterAndPreservesRest(t *testing.T) {
	f, installPathV1, _ := setUpSuppressibleFixture(t)
	e := engine.New(f.roots)

	skill := mustFindPluginSkill(t, e.Inventory(), "loop-me")
	if skill.Activation != engine.ActivationAuto {
		t.Fatalf("activation before suppress = %q, want Auto", skill.Activation)
	}

	if err := e.Suppress(skill); err != nil {
		t.Fatalf("Suppress: %v", err)
	}

	skillMDPath := filepath.Join(installPathV1, "skills", "loop-me", "SKILL.md")
	data, err := os.ReadFile(skillMDPath)
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "disable-model-invocation: true") {
		t.Fatalf("SKILL.md missing disable-model-invocation edit:\n%s", content)
	}
	if !strings.Contains(content, "user-invocable: false") {
		t.Fatalf("SKILL.md missing user-invocable edit:\n%s", content)
	}
	if !strings.Contains(content, `name: "loop-me"`) {
		t.Fatalf("SKILL.md lost its name field:\n%s", content)
	}
	if !strings.Contains(content, `description: "Loop description"`) {
		t.Fatalf("SKILL.md lost its description field:\n%s", content)
	}
	if !strings.Contains(content, `version: "1.0.0"`) {
		t.Fatalf("SKILL.md lost an unrelated existing field:\n%s", content)
	}
	if !strings.Contains(content, "Body\n") {
		t.Fatalf("SKILL.md lost its body:\n%s", content)
	}

	// Suppressed state must be visible in the inventory list.
	inv := e.Inventory()
	suppressed := mustFindPluginSkill(t, inv, "loop-me")
	if suppressed.Activation != engine.ActivationSuppressed {
		t.Fatalf("activation after suppress = %q, want Suppressed", suppressed.Activation)
	}
}

func TestSuppressSelfHealsAfterPluginUpdate(t *testing.T) {
	f, installPathV1, installPathV2 := setUpSuppressibleFixture(t)
	e := engine.New(f.roots)

	skill := mustFindPluginSkill(t, e.Inventory(), "loop-me")
	if err := e.Suppress(skill); err != nil {
		t.Fatalf("Suppress: %v", err)
	}

	// Simulate a plugin update: a new version directory with a fresh,
	// unedited copy of the skill, and installed_plugins.json now pointing at
	// it instead of v1 (the old cache directory is left behind, as Claude
	// Code itself does until its 7-day cleanup — see skill-mechanisms.md).
	writeSkill(t, filepath.Join(installPathV2, "skills", "loop-me"), "loop-me", "Loop description", "version: \"2.0.0\"\n")
	writePluginManifest(t, f.roots.ClaudeHome, map[string][]map[string]string{
		"plugin-x@marketplace-x": {
			{"scope": "user", "installPath": installPathV2, "version": "2.0.0"},
		},
	})

	// Sanity check: the fresh v2 copy does NOT carry the suppression edit
	// yet — the update genuinely reset it.
	v2Path := filepath.Join(installPathV2, "skills", "loop-me", "SKILL.md")
	before, err := os.ReadFile(v2Path)
	if err != nil {
		t.Fatalf("read v2 SKILL.md: %v", err)
	}
	if strings.Contains(string(before), "disable-model-invocation") {
		t.Fatalf("test setup invalid: v2 copy already carries the suppression edit")
	}

	// The next Inventory() call is the only trigger required — no separate
	// "heal" step.
	inv := e.Inventory()
	healed := mustFindPluginSkill(t, inv, "loop-me")
	if healed.Activation != engine.ActivationSuppressed {
		t.Fatalf("activation after self-heal = %q, want Suppressed", healed.Activation)
	}
	wantLocation := filepath.Join(installPathV2, "skills", "loop-me")
	if healed.Location != wantLocation {
		t.Fatalf("location after self-heal = %q, want %q (the new v2 cache dir)", healed.Location, wantLocation)
	}

	after, err := os.ReadFile(v2Path)
	if err != nil {
		t.Fatalf("read v2 SKILL.md after heal: %v", err)
	}
	content := string(after)
	if !strings.Contains(content, "disable-model-invocation: true") || !strings.Contains(content, "user-invocable: false") {
		t.Fatalf("v2 SKILL.md not re-suppressed after update:\n%s", content)
	}
	if !strings.Contains(content, `version: "2.0.0"`) {
		t.Fatalf("v2 SKILL.md lost its own fields after self-heal:\n%s", content)
	}

	// The orphaned v1 copy must be left exactly as Suppress originally wrote
	// it (Skillet doesn't reach back into old cache directories).
	v1Path := filepath.Join(installPathV1, "skills", "loop-me", "SKILL.md")
	v1Content, err := os.ReadFile(v1Path)
	if err != nil {
		t.Fatalf("read v1 SKILL.md: %v", err)
	}
	if !strings.Contains(string(v1Content), "disable-model-invocation: true") {
		t.Fatalf("v1 SKILL.md unexpectedly lost its suppression edit:\n%s", v1Content)
	}
}

func TestUnsuppressRevertsEditAndRemovesRecord(t *testing.T) {
	f, installPathV1, _ := setUpSuppressibleFixture(t)
	e := engine.New(f.roots)

	skill := mustFindPluginSkill(t, e.Inventory(), "loop-me")
	if err := e.Suppress(skill); err != nil {
		t.Fatalf("Suppress: %v", err)
	}

	suppressed := mustFindPluginSkill(t, e.Inventory(), "loop-me")
	if err := e.Unsuppress(suppressed); err != nil {
		t.Fatalf("Unsuppress: %v", err)
	}

	skillMDPath := filepath.Join(installPathV1, "skills", "loop-me", "SKILL.md")
	data, err := os.ReadFile(skillMDPath)
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "disable-model-invocation") || strings.Contains(content, "user-invocable") {
		t.Fatalf("SKILL.md still carries suppression edit after Unsuppress:\n%s", content)
	}
	if !strings.Contains(content, `version: "1.0.0"`) {
		t.Fatalf("SKILL.md lost an unrelated field after Unsuppress:\n%s", content)
	}

	inv := e.Inventory()
	reverted := mustFindPluginSkill(t, inv, "loop-me")
	if reverted.Activation != engine.ActivationAuto {
		t.Fatalf("activation after unsuppress = %q, want Auto", reverted.Activation)
	}
	if noticesContain(inv, "loop-me") {
		t.Fatalf("stale-suppression notice unexpectedly present after Unsuppress: %#v", inv.Notices)
	}
}

func TestUnsuppressAfterPluginUpdateRevertsCurrentCopy(t *testing.T) {
	f, _, installPathV2 := setUpSuppressibleFixture(t)
	e := engine.New(f.roots)

	skill := mustFindPluginSkill(t, e.Inventory(), "loop-me")
	if err := e.Suppress(skill); err != nil {
		t.Fatalf("Suppress: %v", err)
	}

	writeSkill(t, filepath.Join(installPathV2, "skills", "loop-me"), "loop-me", "Loop description", "version: \"2.0.0\"\n")
	writePluginManifest(t, f.roots.ClaudeHome, map[string][]map[string]string{
		"plugin-x@marketplace-x": {
			{"scope": "user", "installPath": installPathV2, "version": "2.0.0"},
		},
	})

	// Trigger self-heal onto v2 first, as would happen on any normal run.
	healed := mustFindPluginSkill(t, e.Inventory(), "loop-me")
	if healed.Activation != engine.ActivationSuppressed {
		t.Fatalf("activation after self-heal = %q, want Suppressed", healed.Activation)
	}

	if err := e.Unsuppress(healed); err != nil {
		t.Fatalf("Unsuppress after update: %v", err)
	}

	v2Path := filepath.Join(installPathV2, "skills", "loop-me", "SKILL.md")
	data, err := os.ReadFile(v2Path)
	if err != nil {
		t.Fatalf("read v2 SKILL.md: %v", err)
	}
	if strings.Contains(string(data), "disable-model-invocation") {
		t.Fatalf("v2 SKILL.md still suppressed after Unsuppress:\n%s", data)
	}

	final := mustFindPluginSkill(t, e.Inventory(), "loop-me")
	if final.Activation != engine.ActivationAuto {
		t.Fatalf("activation after unsuppress post-update = %q, want Auto", final.Activation)
	}
}

func TestSuppressionStaleAfterPluginUninstalledSurfacesNoticeWithoutCrash(t *testing.T) {
	f, installPathV1, _ := setUpSuppressibleFixture(t)
	e := engine.New(f.roots)

	skill := mustFindPluginSkill(t, e.Inventory(), "loop-me")
	if err := e.Suppress(skill); err != nil {
		t.Fatalf("Suppress: %v", err)
	}

	// Simulate the plugin being uninstalled entirely outside of Skillet:
	// its manifest entry disappears.
	writePluginManifest(t, f.roots.ClaudeHome, map[string][]map[string]string{})

	inv := e.Inventory()
	if _, ok := findSkill(inv, engine.SourcePlugin, "loop-me"); ok {
		t.Fatalf("uninstalled plugin's skill should not appear in inventory: %#v", inv.Skills)
	}
	if !noticesContain(inv, "loop-me") {
		t.Fatalf("expected a stale-suppression notice mentioning loop-me: %#v", inv.Notices)
	}

	// A second call must behave identically (no crash, notice persists) —
	// the stale record isn't silently dropped after being reported once.
	inv2 := e.Inventory()
	if !noticesContain(inv2, "loop-me") {
		t.Fatalf("expected the stale-suppression notice to persist across calls: %#v", inv2.Notices)
	}

	// The record itself must actually survive on disk (not just "no error"):
	// reinstalling the plugin at the same marketplace/plugin name must
	// resume suppression automatically, with no call to Suppress again.
	writePluginManifest(t, f.roots.ClaudeHome, map[string][]map[string]string{
		"plugin-x@marketplace-x": {
			{"scope": "user", "installPath": installPathV1, "version": "1.0.0"},
		},
	})
	inv3 := e.Inventory()
	reinstalled := mustFindPluginSkill(t, inv3, "loop-me")
	if reinstalled.Activation != engine.ActivationSuppressed {
		t.Fatalf("activation after reinstall = %q, want Suppressed (record should have survived the uninstall)", reinstalled.Activation)
	}
	if noticesContain(inv3, "no longer found") {
		t.Fatalf("stale notice should be gone once the plugin reappears: %#v", inv3.Notices)
	}
}

func TestSuppressRejectsNonPluginSkill(t *testing.T) {
	f := newFixture(t)
	writeSkill(t, filepath.Join(f.roots.ClaudeHome, "skills", "personal-one"), "personal-one", "A personal skill", "")
	e := engine.New(f.roots)

	skill, ok := findSkill(e.Inventory(), engine.SourcePersonal, "personal-one")
	if !ok {
		t.Fatalf("personal skill not found")
	}
	if err := e.Suppress(skill); err == nil {
		t.Fatalf("Suppress on a Personal skill should return an error")
	}
	if err := e.Unsuppress(skill); err == nil {
		t.Fatalf("Unsuppress on a Personal skill should return an error")
	}
}

func TestSuppressReadOnlySessionLeavesTreeByteIdenticalWhenNoSuppressions(t *testing.T) {
	f, _, _ := setUpSuppressibleFixture(t)
	e := engine.New(f.roots)

	before := snapshotTree(t, f.root)
	_ = e.Inventory()
	_ = e.Inventory()
	after := snapshotTree(t, f.root)
	assertSnapshotsEqual(t, before, after)
}
