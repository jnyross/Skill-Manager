package engine_test

import (
	"os"
	"path/filepath"
	"testing"

	"skillet/internal/engine"
)

// setUpTwoPluginFixture writes two user-scoped plugins (plugin-x@marketplace-x
// with two skills, plugin-y@marketplace-y with one skill) plus a
// settings.json enabledPlugins entry for both, so tests can assert that
// uninstalling plugin-x leaves plugin-y's manifest entry, cache directory,
// and settings.json entry completely untouched.
func setUpTwoPluginFixture(t *testing.T) (f fixture, pluginXInstallPath, pluginYInstallPath string) {
	t.Helper()
	f = newFixture(t)
	pluginXInstallPath = filepath.Join(f.root, "plugin-cache", "marketplace-x", "plugin-x", "v1")
	pluginYInstallPath = filepath.Join(f.root, "plugin-cache", "marketplace-y", "plugin-y", "v1")
	writeSkill(t, filepath.Join(pluginXInstallPath, "skills", "skill-a"), "skill-a", "Skill A description", "")
	writeSkill(t, filepath.Join(pluginXInstallPath, "skills", "skill-b"), "skill-b", "Skill B description", "")
	writeSkill(t, filepath.Join(pluginYInstallPath, "skills", "skill-c"), "skill-c", "Skill C description", "")
	writePluginManifest(t, f.roots.ClaudeHome, map[string][]map[string]string{
		"plugin-x@marketplace-x": {
			{"scope": "user", "installPath": pluginXInstallPath, "version": "1.0.0", "installedAt": "2026-06-09T23:28:34.988Z", "gitCommitSha": "abc123"},
		},
		"plugin-y@marketplace-y": {
			{"scope": "user", "installPath": pluginYInstallPath, "version": "2.0.0", "installedAt": "2026-06-01T00:00:00.000Z", "gitCommitSha": "def456"},
		},
	})
	writeSettingsJSON(t, f.roots.ClaudeHome, map[string]any{
		"model": "claude-sonnet-5",
		"enabledPlugins": map[string]any{
			"plugin-x@marketplace-x": true,
			"plugin-y@marketplace-y": true,
		},
	})
	return f, pluginXInstallPath, pluginYInstallPath
}

func mustFindPluginInfo(t *testing.T, inv engine.Inventory, name string) engine.PluginInfo {
	t.Helper()
	skill := mustFindPluginSkill(t, inv, name)
	if skill.Plugin == nil {
		t.Fatalf("skill %s has nil Plugin", name)
	}
	return *skill.Plugin
}

func TestUninstallPluginRemovesManifestCacheAndSettingsEntry(t *testing.T) {
	f, pluginXInstallPath, pluginYInstallPath := setUpTwoPluginFixture(t)
	e := engine.New(f.roots)

	plugin := mustFindPluginInfo(t, e.Inventory(), "skill-a")
	if err := e.UninstallPlugin(plugin); err != nil {
		t.Fatalf("UninstallPlugin: %v", err)
	}

	// Skills disappear from the inventory (acceptance criterion).
	inv := e.Inventory()
	if _, ok := findSkill(inv, engine.SourcePlugin, "skill-a"); ok {
		t.Fatalf("skill-a still present after UninstallPlugin: %#v", inv.Skills)
	}
	if _, ok := findSkill(inv, engine.SourcePlugin, "skill-b"); ok {
		t.Fatalf("skill-b still present after UninstallPlugin: %#v", inv.Skills)
	}
	if _, ok := findSkill(inv, engine.SourcePlugin, "skill-c"); !ok {
		t.Fatalf("skill-c (unrelated plugin) unexpectedly removed: %#v", inv.Skills)
	}

	// Cache directory deleted; the other plugin's cache directory survives.
	if _, err := os.Stat(pluginXInstallPath); !os.IsNotExist(err) {
		t.Fatalf("plugin-x cache directory still exists: %v", err)
	}
	if _, err := os.Stat(pluginYInstallPath); err != nil {
		t.Fatalf("plugin-y cache directory unexpectedly removed: %v", err)
	}

	// Manifest entry removed; other plugin's entry (and its extra fields)
	// preserved byte-for-byte.
	manifest := readJSONFile(t, filepath.Join(f.roots.ClaudeHome, "plugins", "installed_plugins.json"))
	plugins, ok := manifest["plugins"].(map[string]any)
	if !ok {
		t.Fatalf("manifest plugins field missing or wrong type: %#v", manifest)
	}
	if _, ok := plugins["plugin-x@marketplace-x"]; ok {
		t.Fatalf("plugin-x manifest entry still present: %#v", plugins)
	}
	yEntries, ok := plugins["plugin-y@marketplace-y"].([]any)
	if !ok || len(yEntries) != 1 {
		t.Fatalf("plugin-y manifest entry missing or malformed: %#v", plugins)
	}
	yEntry := yEntries[0].(map[string]any)
	if yEntry["gitCommitSha"] != "def456" || yEntry["installedAt"] != "2026-06-01T00:00:00.000Z" {
		t.Fatalf("plugin-y manifest entry lost unrelated fields: %#v", yEntry)
	}
	if manifest["version"] != float64(2) {
		t.Fatalf("manifest top-level version field not preserved: %#v", manifest)
	}

	// settings.json enabledPlugins entry removed; other plugin's entry and
	// unrelated top-level keys preserved.
	settings := readJSONFile(t, filepath.Join(f.roots.ClaudeHome, "settings.json"))
	if settings["model"] != "claude-sonnet-5" {
		t.Fatalf("settings.json lost unrelated key: %#v", settings)
	}
	enabled, ok := settings["enabledPlugins"].(map[string]any)
	if !ok {
		t.Fatalf("settings.json enabledPlugins missing or wrong type: %#v", settings)
	}
	if _, ok := enabled["plugin-x@marketplace-x"]; ok {
		t.Fatalf("plugin-x still enabled in settings.json: %#v", enabled)
	}
	if enabled["plugin-y@marketplace-y"] != true {
		t.Fatalf("plugin-y enabledPlugins entry lost: %#v", enabled)
	}
}

func TestUninstallPluginCleansUpSuppressionRecords(t *testing.T) {
	f, _, _ := setUpTwoPluginFixture(t)
	e := engine.New(f.roots)

	skillA := mustFindPluginSkill(t, e.Inventory(), "skill-a")
	skillC := mustFindPluginSkill(t, e.Inventory(), "skill-c")
	if err := e.Suppress(skillA); err != nil {
		t.Fatalf("Suppress skill-a: %v", err)
	}
	// Re-fetch skill-b (unsuppressed) and suppress it too, so both of
	// plugin-x's skills have suppression records to be cleaned up.
	skillB := mustFindPluginSkill(t, e.Inventory(), "skill-b")
	if err := e.Suppress(skillB); err != nil {
		t.Fatalf("Suppress skill-b: %v", err)
	}
	if err := e.Suppress(skillC); err != nil {
		t.Fatalf("Suppress skill-c: %v", err)
	}

	records, err := os.ReadDir(filepath.Join(f.roots.DataDir, "suppressed"))
	if err != nil || len(records) != 3 {
		t.Fatalf("expected 3 suppression records before uninstall, got %v (err=%v)", records, err)
	}

	plugin := mustFindPluginInfo(t, e.Inventory(), "skill-a")
	if err := e.UninstallPlugin(plugin); err != nil {
		t.Fatalf("UninstallPlugin: %v", err)
	}

	records, err = os.ReadDir(filepath.Join(f.roots.DataDir, "suppressed"))
	if err != nil {
		t.Fatalf("read suppressed dir: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 suppression record to survive (skill-c's), got %d: %v", len(records), records)
	}

	// skill-c (unrelated plugin) must still show as Suppressed.
	inv := e.Inventory()
	remaining := mustFindPluginSkill(t, inv, "skill-c")
	if remaining.Activation != engine.ActivationSuppressed {
		t.Fatalf("skill-c activation = %q, want Suppressed", remaining.Activation)
	}
	if noticesContain(inv, "skill-a") || noticesContain(inv, "skill-b") {
		t.Fatalf("stale-suppression notices for removed plugin unexpectedly present: %#v", inv.Notices)
	}
}

func TestUninstallPluginMissingManifestEntryReturnsError(t *testing.T) {
	f, _, _ := setUpTwoPluginFixture(t)
	e := engine.New(f.roots)

	err := e.UninstallPlugin(engine.PluginInfo{Plugin: "does-not-exist", Marketplace: "marketplace-x"})
	if err == nil {
		t.Fatalf("expected an error uninstalling a plugin absent from the manifest")
	}

	// Nothing else should have been touched.
	inv := e.Inventory()
	if _, ok := findSkill(inv, engine.SourcePlugin, "skill-a"); !ok {
		t.Fatalf("unrelated plugin-x skill disappeared after a failed uninstall: %#v", inv.Skills)
	}
}

func TestUninstallPluginMissingManifestFileReturnsError(t *testing.T) {
	f := newFixture(t)
	e := engine.New(f.roots)

	err := e.UninstallPlugin(engine.PluginInfo{Plugin: "plugin-x", Marketplace: "marketplace-x"})
	if err == nil {
		t.Fatalf("expected an error uninstalling a plugin when no manifest file exists")
	}
}

func TestUninstallPluginMissingCacheDirStillCleansManifestAndSettings(t *testing.T) {
	f, pluginXInstallPath, _ := setUpTwoPluginFixture(t)
	e := engine.New(f.roots)
	plugin := mustFindPluginInfo(t, e.Inventory(), "skill-a")

	// Simulate the cache directory already being gone (e.g. removed
	// manually, or already cleaned up by Claude Code itself).
	if err := os.RemoveAll(pluginXInstallPath); err != nil {
		t.Fatalf("remove cache dir: %v", err)
	}

	if err := e.UninstallPlugin(plugin); err != nil {
		t.Fatalf("UninstallPlugin with missing cache dir: %v", err)
	}

	manifest := readJSONFile(t, filepath.Join(f.roots.ClaudeHome, "plugins", "installed_plugins.json"))
	plugins := manifest["plugins"].(map[string]any)
	if _, ok := plugins["plugin-x@marketplace-x"]; ok {
		t.Fatalf("plugin-x manifest entry still present: %#v", plugins)
	}
}

func TestUninstallPluginNoSettingsJSONFile(t *testing.T) {
	f, _, _ := setUpTwoPluginFixture(t)
	// Overwrite the fixture without a settings.json to simulate an install
	// that never had one.
	if err := os.Remove(filepath.Join(f.roots.ClaudeHome, "settings.json")); err != nil {
		t.Fatalf("remove settings.json fixture: %v", err)
	}
	e := engine.New(f.roots)
	plugin := mustFindPluginInfo(t, e.Inventory(), "skill-a")

	if err := e.UninstallPlugin(plugin); err != nil {
		t.Fatalf("UninstallPlugin with no settings.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.roots.ClaudeHome, "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("UninstallPlugin unexpectedly created a settings.json: %v", err)
	}
}

func TestUninstallPluginSettingsJSONWithoutEnabledPluginsEntry(t *testing.T) {
	f, _, _ := setUpTwoPluginFixture(t)
	// settings.json exists but has no enabledPlugins entry for this plugin
	// at all (e.g. it was only ever enabled at a scope Skillet doesn't
	// track, or the user disabled it by hand already).
	writeSettingsJSON(t, f.roots.ClaudeHome, map[string]any{
		"model":          "claude-sonnet-5",
		"enabledPlugins": map[string]any{"plugin-y@marketplace-y": true},
	})
	e := engine.New(f.roots)
	plugin := mustFindPluginInfo(t, e.Inventory(), "skill-a")

	if err := e.UninstallPlugin(plugin); err != nil {
		t.Fatalf("UninstallPlugin with no matching enabledPlugins entry: %v", err)
	}
	settings := readJSONFile(t, filepath.Join(f.roots.ClaudeHome, "settings.json"))
	if settings["model"] != "claude-sonnet-5" {
		t.Fatalf("settings.json lost unrelated key: %#v", settings)
	}
}

func TestUninstallPluginMultipleScopeEntriesRemovesWholeKeyButOnlyUserCacheDir(t *testing.T) {
	f := newFixture(t)
	userInstallPath := filepath.Join(f.root, "plugin-cache", "marketplace-x", "plugin-x", "v1")
	projectInstallPath := filepath.Join(f.root, "project-plugin-cache", "plugin-x")
	writeSkill(t, filepath.Join(userInstallPath, "skills", "skill-a"), "skill-a", "Skill A description", "")
	mkdirAll(t, projectInstallPath)
	writeFile(t, filepath.Join(projectInstallPath, "marker.txt"), "project-scoped install, not scanned by v1")
	writePluginManifest(t, f.roots.ClaudeHome, map[string][]map[string]string{
		"plugin-x@marketplace-x": {
			{"scope": "user", "installPath": userInstallPath, "version": "1.0.0"},
			{"scope": "project", "installPath": projectInstallPath, "version": "1.0.0"},
		},
	})
	e := engine.New(f.roots)
	plugin := mustFindPluginInfo(t, e.Inventory(), "skill-a")

	if err := e.UninstallPlugin(plugin); err != nil {
		t.Fatalf("UninstallPlugin: %v", err)
	}

	manifest := readJSONFile(t, filepath.Join(f.roots.ClaudeHome, "plugins", "installed_plugins.json"))
	plugins := manifest["plugins"].(map[string]any)
	if _, ok := plugins["plugin-x@marketplace-x"]; ok {
		t.Fatalf("plugin-x manifest key should be entirely removed (all scopes): %#v", plugins)
	}

	if _, err := os.Stat(userInstallPath); !os.IsNotExist(err) {
		t.Fatalf("user-scoped cache directory still exists: %v", err)
	}
	// The project-scoped install path is outside v1's scanned/represented
	// scope (scanPlugins only ever looks at scope=="user"); Skillet leaves
	// it on disk rather than deleting a path it has never inventoried.
	if _, err := os.Stat(projectInstallPath); err != nil {
		t.Fatalf("project-scoped install path unexpectedly removed: %v", err)
	}
}
