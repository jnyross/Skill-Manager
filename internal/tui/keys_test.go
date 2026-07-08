package tui

import (
	"testing"

	"skillet/internal/engine"
)

func TestCanArchiveSkillMatchesMainHandlerGate(t *testing.T) {
	cases := []struct {
		name  string
		skill engine.Skill
		want  bool
	}{
		{
			name:  "personal skill",
			skill: engine.Skill{Source: engine.SourcePersonal, Kind: engine.KindSkill},
			want:  true,
		},
		{
			name:  "codex prompt",
			skill: engine.Skill{Source: engine.SourceCodex, Kind: engine.KindPrompt},
			want:  true,
		},
		{
			name:  "plugin skill",
			skill: engine.Skill{Source: engine.SourcePlugin, Kind: engine.KindSkill},
			want:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := canArchiveSkill(tc.skill); got != tc.want {
				t.Fatalf("canArchiveSkill(%#v) = %t, want %t", tc.skill, got, tc.want)
			}
		})
	}
}

func TestCanSuppressSkillMatchesMainHandlerGate(t *testing.T) {
	cases := []struct {
		name  string
		skill engine.Skill
		want  bool
	}{
		{
			name:  "plugin prompt",
			skill: engine.Skill{Source: engine.SourcePlugin, Kind: engine.KindPrompt},
			want:  true,
		},
		{
			name:  "codex skill",
			skill: engine.Skill{Source: engine.SourceCodex, Kind: engine.KindSkill},
			want:  true,
		},
		{
			name:  "codex prompt",
			skill: engine.Skill{Source: engine.SourceCodex, Kind: engine.KindPrompt},
			want:  false,
		},
		{
			name:  "personal skill",
			skill: engine.Skill{Source: engine.SourcePersonal, Kind: engine.KindSkill},
			want:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := canSuppressSkill(tc.skill); got != tc.want {
				t.Fatalf("canSuppressSkill(%#v) = %t, want %t", tc.skill, got, tc.want)
			}
		})
	}
}

func TestCanUninstallPluginMatchesMainHandlerGate(t *testing.T) {
	plugin := engine.PluginInfo{Plugin: "example", Marketplace: "marketplace", SkillCount: 1}

	cases := []struct {
		name  string
		skill engine.Skill
		want  bool
	}{
		{
			name:  "plugin skill with plugin info",
			skill: engine.Skill{Source: engine.SourcePlugin, Kind: engine.KindSkill, Plugin: &plugin},
			want:  true,
		},
		{
			name:  "plugin skill without plugin info",
			skill: engine.Skill{Source: engine.SourcePlugin, Kind: engine.KindSkill},
			want:  false,
		},
		{
			name:  "personal skill with plugin info",
			skill: engine.Skill{Source: engine.SourcePersonal, Kind: engine.KindSkill, Plugin: &plugin},
			want:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := canUninstallPlugin(tc.skill); got != tc.want {
				t.Fatalf("canUninstallPlugin(%#v) = %t, want %t", tc.skill, got, tc.want)
			}
		})
	}
}

func TestCanToggleManualOnlyMatchesMainHandlerGate(t *testing.T) {
	cases := []struct {
		name  string
		skill engine.Skill
		want  bool
	}{
		{
			name:  "personal skill",
			skill: engine.Skill{Source: engine.SourcePersonal, Kind: engine.KindSkill},
			want:  true,
		},
		{
			name:  "codex skill",
			skill: engine.Skill{Source: engine.SourceCodex, Kind: engine.KindSkill},
			want:  true,
		},
		{
			name:  "codex prompt",
			skill: engine.Skill{Source: engine.SourceCodex, Kind: engine.KindPrompt},
			want:  false,
		},
		{
			name:  "plugin skill",
			skill: engine.Skill{Source: engine.SourcePlugin, Kind: engine.KindSkill},
			want:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := canToggleManualOnly(tc.skill); got != tc.want {
				t.Fatalf("canToggleManualOnly(%#v) = %t, want %t", tc.skill, got, tc.want)
			}
		})
	}
}

func TestMainKeyMapDisablesDeadRowActions(t *testing.T) {
	m := mainKeyMap(engine.Skill{Source: engine.SourcePersonal, Kind: engine.KindSkill}, true, false)
	if !m.archive.Enabled() {
		t.Fatalf("archive binding disabled for Personal skill")
	}
	if m.suppress.Enabled() {
		t.Fatalf("suppress binding enabled for Personal skill")
	}
	if !m.manualOnly.Enabled() {
		t.Fatalf("manual-only binding disabled for Personal skill")
	}
	if m.uninstallPlugin.Enabled() {
		t.Fatalf("uninstall plugin binding enabled for Personal skill")
	}
}

func TestArchiveKeyMapRequiresSelectedEntryForRestoreAndPurge(t *testing.T) {
	empty := archiveKeyMap(false, false)
	if empty.restore.Enabled() {
		t.Fatalf("restore binding enabled without archive selection")
	}
	if empty.purge.Enabled() {
		t.Fatalf("purge binding enabled without archive selection")
	}

	selected := archiveKeyMap(true, false)
	if !selected.restore.Enabled() {
		t.Fatalf("restore binding disabled with archive selection")
	}
	if !selected.purge.Enabled() {
		t.Fatalf("purge binding disabled with archive selection")
	}
}
