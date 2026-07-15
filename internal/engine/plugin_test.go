package engine_test

import (
	"path/filepath"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

func TestPluginScanNestedSkillsAndSkillCount(t *testing.T) {
	f := newFixture(t)
	installPath := filepath.Join(f.root, "plugin-cache", "market", "plugin-x", "v1")
	writeSkill(t, filepath.Join(installPath, "skills", "engineering", "foo"), "foo", "Foo description", "")
	writeSkill(t, filepath.Join(installPath, "skills", "deep", "category", "bar"), "bar", "Bar description", "")
	writePluginManifest(t, f.roots.ClaudeHome, map[string][]map[string]string{
		"plugin-x@marketplace-x": {
			{"scope": "user", "installPath": installPath, "version": "1.0.0"},
			{"scope": "project", "installPath": filepath.Join(f.root, "ignored"), "version": "1.0.0"},
		},
	})

	inv := engine.New(f.roots).Inventory()
	skills := sourceSkills(inv, engine.SourcePlugin)
	if len(skills) != 2 {
		t.Fatalf("got %d plugin skills, want 2: %#v", len(skills), skills)
	}
	for _, name := range []string{"foo", "bar"} {
		skill, ok := findSkill(inv, engine.SourcePlugin, name)
		if !ok {
			t.Fatalf("missing plugin skill %s in %#v", name, inv.Skills)
		}
		if skill.Plugin == nil {
			t.Fatalf("%s plugin info is nil", name)
		}
		if skill.Plugin.Plugin != "plugin-x" || skill.Plugin.Marketplace != "marketplace-x" {
			t.Fatalf("%s plugin info = %#v", name, skill.Plugin)
		}
		if skill.Plugin.SkillCount != 2 {
			t.Fatalf("%s skill count = %d, want 2", name, skill.Plugin.SkillCount)
		}
		if skill.Activation != engine.ActivationAuto {
			t.Fatalf("%s activation = %q, want Auto", name, skill.Activation)
		}
		if skill.Tool != engine.ToolClaudeCode {
			t.Fatalf("%s tool = %q, want Claude Code", name, skill.Tool)
		}
	}
}

func TestPluginMissingInstallPathNoticeDoesNotStopOtherPlugins(t *testing.T) {
	f := newFixture(t)
	installPath := filepath.Join(f.root, "plugin-cache", "market", "plugin-good", "v1")
	writeSkill(t, filepath.Join(installPath, "skills", "good"), "good", "Good description", "")
	writePluginManifest(t, f.roots.ClaudeHome, map[string][]map[string]string{
		"plugin-missing@marketplace-x": {
			{"scope": "user", "installPath": filepath.Join(f.root, "does-not-exist"), "version": "1.0.0"},
		},
		"plugin-good@marketplace-x": {
			{"scope": "user", "installPath": installPath, "version": "1.0.0"},
		},
	})

	inv := engine.New(f.roots).Inventory()
	if _, ok := findSkill(inv, engine.SourcePlugin, "good"); !ok {
		t.Fatalf("good plugin skill missing: %#v", inv.Skills)
	}
	if !noticesContain(inv, "Plugin plugin-missing@marketplace-x: install path not found") {
		t.Fatalf("missing install path notice: %#v", inv.Notices)
	}
}
