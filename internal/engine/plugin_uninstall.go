package engine

// Whole-plugin uninstall — the other half of the Plugin-skill action pair
// alongside Suppress (suppress.go). CONTEXT.md: a Plugin skill "cannot be
// removed alone without affecting its plugin"; this is that plugin-level
// removal, using Claude Code's own supported mechanism (its manifest,
// its plugin cache, and its settings.json enable map — see
// docs/research/skill-mechanisms.md, "Where things live on disk") rather
// than Skillet's Archive. Unlike archive.go's Uninstall, there is nothing to
// preserve for later restore: plugins are reinstallable from their
// marketplace, so this is a direct, irreversible-from-Skillet's-perspective
// deletion, not an Archive operation. It does still clean up one piece of
// Skillet-owned state: any Suppress records for skills in the removed
// plugin, which would otherwise become orphaned (see
// applySuppressions/suppress.go, which already tolerates and reports on
// orphaned records surfacing as a stale-suppression Notice — but a plugin
// deliberately uninstalled through Skillet itself should not leave that
// residue behind).
//
// Unlike install.go's marketplace plugin path, which shells out to the
// `claude` CLI (`claude plugin marketplace add` and `claude plugin install`),
// uninstall deliberately uses direct file edits: research turned up no
// documented `claude plugin uninstall` CLI, and the manifest/settings/cache
// files are plain JSON that Skillet can edit safely. This keeps uninstall
// deterministic, offline-capable, and reversible only via reinstallation from
// the marketplace rather than through Skillet's Archive.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UninstallPlugin removes an entire plugin: its installed_plugins.json
// entry, its cache directory (or directories — see below), any
// enabledPlugins settings.json entry, and every Skillet-owned Suppress
// record for its skills. plugin should come from a Skill.Plugin field on a
// recent Inventory() call (Marketplace + Plugin are the only fields used),
// mirroring how Suppress(skill Skill) takes a value from a recent scan
// rather than raw caller-supplied strings.
//
// A missing manifest file, or a manifest with no entry for this
// marketplace+plugin, is reported as an error rather than silently
// succeeding — v1 has no other way to distinguish "already uninstalled"
// from "never existed" or "typo'd", and a caller genuinely expects this
// call to have removed something. The error is a plain fmt.Errorf, not a
// panic, so it surfaces as a status message in the TUI (same pattern as
// every other mutating Engine method).
//
// Multiple scopes: installed_plugins.json's map value is an array of
// per-scope install records (v1 only ever scans/represents scope=="user";
// see scanPlugins in plugin.go). Per the issue's own guidance, removing
// only the user-scoped array entries and leaving a project-scoped one
// behind would leave the manifest key present with stale-looking content,
// so the entire map key — every scope's install record — is removed in one
// edit. Cache-directory deletion is narrower: only the InstallPath of
// scope=="user" entries is removed, since those are the only paths Skillet
// has ever scanned or shown the user; deleting a project-scoped path
// Skillet has never represented would be a surprising, undocumented side
// effect for a scope v1 doesn't otherwise support.
func (e *Engine) UninstallPlugin(plugin PluginInfo) error {
	if plugin.Plugin == "" || plugin.Marketplace == "" {
		return fmt.Errorf("uninstall plugin: marketplace and plugin name are required")
	}
	key := plugin.Plugin + "@" + plugin.Marketplace

	userInstallPaths, err := e.removePluginManifestEntry(key)
	if err != nil {
		return fmt.Errorf("uninstall plugin: %w", err)
	}

	for _, installPath := range userInstallPaths {
		if err := validatePluginCachePath(e.roots.ClaudeHome, installPath); err != nil {
			return fmt.Errorf("uninstall plugin: %w", err)
		}
		if err := os.RemoveAll(installPath); err != nil {
			return fmt.Errorf("uninstall plugin: remove cache directory %s: %w", installPath, err)
		}
	}

	if err := e.removeEnabledPluginsEntry(key); err != nil {
		return fmt.Errorf("uninstall plugin: %w", err)
	}

	if err := e.removePluginSuppressionRecords(plugin.Marketplace, plugin.Plugin); err != nil {
		return fmt.Errorf("uninstall plugin: %w", err)
	}

	return nil
}

// validatePluginCachePath checks that installPath resolves to an absolute
// directory strictly inside <claudeHome>/plugins/cache. It rejects the cache
// root itself, any ancestor, and any path that escapes via ".." components.
// This guard prevents a malformed or malicious installed_plugins.json entry
// from causing UninstallPlugin to delete arbitrary directories.
func validatePluginCachePath(claudeHome, installPath string) error {
	cacheRoot := filepath.Join(claudeHome, "plugins", "cache")
	absCacheRoot, err := filepath.Abs(cacheRoot)
	if err != nil {
		return fmt.Errorf("resolve plugin cache root: %w", err)
	}
	absPath, err := filepath.Abs(installPath)
	if err != nil {
		return fmt.Errorf("resolve plugin install path: %w", err)
	}

	rel, err := filepath.Rel(absCacheRoot, absPath)
	if err != nil {
		return fmt.Errorf("relativize plugin install path: %w", err)
	}
	if rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("plugin install path %q is outside cache root %q", installPath, cacheRoot)
	}
	return nil
}

// removePluginManifestEntry deletes key from installed_plugins.json's
// "plugins" map, leaving every other entry (and every other top-level
// field, e.g. "version") byte-identical apart from JSON re-indentation —
// see the package doc comment above for why a plain unmarshal/delete/
// remarshal is safe here (plain JSON, no comments to preserve, unlike
// codex_config.go's TOML byte-surgery). It returns the InstallPath of every
// scope=="user" entry in the deleted array, for the caller to remove from
// disk.
func (e *Engine) removePluginManifestEntry(key string) ([]string, error) {
	manifestPath := filepath.Join(e.roots.ClaudeHome, "plugins", "installed_plugins.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("plugin manifest not found: %s", manifestPath)
		}
		return nil, fmt.Errorf("read plugin manifest: %w", err)
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse plugin manifest: %w", err)
	}
	pluginsRaw, ok := root["plugins"]
	if !ok {
		return nil, fmt.Errorf("plugin manifest has no plugins map: %s", manifestPath)
	}
	var plugins map[string]json.RawMessage
	if err := json.Unmarshal(pluginsRaw, &plugins); err != nil {
		return nil, fmt.Errorf("parse plugin manifest plugins map: %w", err)
	}
	entryRaw, ok := plugins[key]
	if !ok {
		return nil, fmt.Errorf("plugin not found in manifest: %s", key)
	}

	var installs []pluginInstall
	if err := json.Unmarshal(entryRaw, &installs); err != nil {
		return nil, fmt.Errorf("parse plugin manifest entry %s: %w", key, err)
	}
	var userInstallPaths []string
	for _, install := range installs {
		if install.Scope == "user" && install.InstallPath != "" {
			userInstallPaths = append(userInstallPaths, install.InstallPath)
		}
	}

	delete(plugins, key)
	newPluginsRaw, err := json.Marshal(plugins)
	if err != nil {
		return nil, fmt.Errorf("marshal plugin manifest plugins map: %w", err)
	}
	root["plugins"] = newPluginsRaw

	newData, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal plugin manifest: %w", err)
	}
	if err := writeFilePreservingMode(manifestPath, string(newData)+"\n"); err != nil {
		return nil, fmt.Errorf("write plugin manifest: %w", err)
	}
	return userInstallPaths, nil
}

// removeEnabledPluginsEntry deletes key from settings.json's "enabledPlugins"
// map (Claude Code's own plugin enable/disable toggle — see
// docs/research/skill-mechanisms.md, "Enabled/disabled"), leaving every
// other key and every other enabledPlugins entry untouched. This is the
// first place in the codebase that reads or writes the user-level
// settings.json — a real, hand-edited config file with many keys Skillet
// doesn't otherwise know about (verified locally: env, permissions, model,
// enabledPlugins, extraKnownMarketplaces, and more), hence the same
// raw-map-and-delete-one-key approach as removePluginManifestEntry rather
// than unmarshalling into a narrow struct that would silently drop unknown
// fields on remarshal.
//
// A missing settings.json, a settings.json with no "enabledPlugins" map, or
// an enabledPlugins map with no entry for key are all treated as a
// successful no-op rather than an error: a plugin can be fully installed
// (and therefore uninstallable) without ever having an enabledPlugins entry
// (e.g. plugins are enabled by default unless explicitly disabled), so
// "no entry to remove" is an entirely expected, non-error outcome.
func (e *Engine) removeEnabledPluginsEntry(key string) error {
	settingsPath := filepath.Join(e.roots.ClaudeHome, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read settings.json: %w", err)
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("parse settings.json: %w", err)
	}
	enabledRaw, ok := root["enabledPlugins"]
	if !ok {
		return nil
	}
	var enabled map[string]json.RawMessage
	if err := json.Unmarshal(enabledRaw, &enabled); err != nil {
		return fmt.Errorf("parse settings.json enabledPlugins: %w", err)
	}
	if _, ok := enabled[key]; !ok {
		return nil
	}

	delete(enabled, key)
	newEnabledRaw, err := json.Marshal(enabled)
	if err != nil {
		return fmt.Errorf("marshal settings.json enabledPlugins: %w", err)
	}
	root["enabledPlugins"] = newEnabledRaw

	newData, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings.json: %w", err)
	}
	if err := writeFilePreservingMode(settingsPath, string(newData)+"\n"); err != nil {
		return fmt.Errorf("write settings.json: %w", err)
	}
	return nil
}

// removePluginSuppressionRecords deletes every Skillet-owned Suppress
// record (suppress.go) belonging to marketplace+plugin, regardless of which
// skill within the plugin it names — reuses loadSuppressionRecords and
// removeSuppressionRecord (both suppress.go) rather than re-scanning the
// suppression directory itself.
func (e *Engine) removePluginSuppressionRecords(marketplace, plugin string) error {
	records, err := loadSuppressionRecords(e.roots.DataDir)
	if err != nil {
		return fmt.Errorf("read suppression records: %w", err)
	}
	for _, record := range records {
		if record.Marketplace != marketplace || record.Plugin != plugin {
			continue
		}
		if err := removeSuppressionRecord(e.roots.DataDir, record.Marketplace, record.Plugin, record.SkillName); err != nil {
			return fmt.Errorf("remove suppression record for %s: %w", record.SkillName, err)
		}
	}
	return nil
}
