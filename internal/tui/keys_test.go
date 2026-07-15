package tui

import (
	"strings"
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
			name:  "project claude skill",
			skill: engine.Skill{Source: engine.SourceProject, Tool: engine.ToolClaudeCode, Kind: engine.KindSkill},
			want:  true,
		},
		{
			name:  "project codex skill",
			skill: engine.Skill{Source: engine.SourceProject, Tool: engine.ToolCodex, Kind: engine.KindSkill},
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
			name:  "project codex skill",
			skill: engine.Skill{Source: engine.SourceProject, Tool: engine.ToolCodex, Kind: engine.KindSkill},
			want:  true,
		},
		{
			name:  "project claude skill",
			skill: engine.Skill{Source: engine.SourceProject, Tool: engine.ToolClaudeCode, Kind: engine.KindSkill},
			want:  false,
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
			name:  "project claude skill",
			skill: engine.Skill{Source: engine.SourceProject, Tool: engine.ToolClaudeCode, Kind: engine.KindSkill},
			want:  true,
		},
		{
			name:  "project codex skill",
			skill: engine.Skill{Source: engine.SourceProject, Tool: engine.ToolCodex, Kind: engine.KindSkill},
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

func TestBundleKeyMapDisablesRowActionsWithoutSelection(t *testing.T) {
	empty := bundleKeyMap(false, false)
	for name, binding := range map[string]bool{
		"move":             empty.move.Enabled(),
		"expand":           empty.expand.Enabled(),
		"add member":       empty.addMember.Enabled(),
		"remove member":    empty.removeMember.Enabled(),
		"cycle activation": empty.manualOnly.Enabled(),
		"install bundle":   empty.libraryInstall.Enabled(),
		"delete bundle":    empty.libraryRemove.Enabled(),
	} {
		if binding {
			t.Fatalf("%s binding enabled without a Bundle selection", name)
		}
	}
	if !empty.create.Enabled() {
		t.Fatal("new Bundle binding disabled in an empty Bundle view")
	}
}

func TestRejectReasonsMatchGates(t *testing.T) {
	projectClaude := engine.Skill{Source: engine.SourceProject, Tool: engine.ToolClaudeCode, Kind: engine.KindSkill}
	if reason := archiveUnavailableReason(projectClaude); reason != "" {
		t.Fatalf("archiveUnavailableReason(project claude) = %q, want empty", reason)
	}

	plugin := engine.Skill{Source: engine.SourcePlugin, Kind: engine.KindSkill}
	reason := archiveUnavailableReason(plugin)
	if reason == "" {
		t.Fatal("archiveUnavailableReason(plugin) empty, want reject reason")
	}
	if !strings.Contains(reason, "Project") {
		t.Fatalf("archiveUnavailableReason(plugin) = %q, want Project mentioned", reason)
	}

	if reason := suppressUnavailableReason(projectClaude); reason == "" {
		t.Fatal("suppressUnavailableReason(project claude) empty, want reject reason")
	}
	if reason := suppressUnavailableReason(engine.Skill{Source: engine.SourceProject, Tool: engine.ToolCodex, Kind: engine.KindSkill}); reason != "" {
		t.Fatalf("suppressUnavailableReason(project codex) = %q, want empty", reason)
	}

	manualReason := manualOnlyUnavailableReason(plugin)
	if manualReason == "" {
		t.Fatal("manualOnlyUnavailableReason(plugin) empty, want reject reason")
	}
	if !strings.Contains(manualReason, "Project") {
		t.Fatalf("manualOnlyUnavailableReason(plugin) = %q, want Project mentioned", manualReason)
	}
	if reason := manualOnlyUnavailableReason(projectClaude); reason != "" {
		t.Fatalf("manualOnlyUnavailableReason(project claude) = %q, want empty", reason)
	}
}

func TestNeedsCodexRestartHint(t *testing.T) {
	cases := []struct {
		name  string
		skill engine.Skill
		want  bool
	}{
		{name: "user codex skill", skill: engine.Skill{Source: engine.SourceCodex, Tool: engine.ToolCodex, Kind: engine.KindSkill}, want: true},
		{name: "project codex skill", skill: engine.Skill{Source: engine.SourceProject, Tool: engine.ToolCodex, Kind: engine.KindSkill}, want: true},
		{name: "plugin skill", skill: engine.Skill{Source: engine.SourcePlugin, Tool: engine.ToolClaudeCode, Kind: engine.KindSkill}, want: false},
		{name: "project claude skill", skill: engine.Skill{Source: engine.SourceProject, Tool: engine.ToolClaudeCode, Kind: engine.KindSkill}, want: false},
		{name: "personal skill", skill: engine.Skill{Source: engine.SourcePersonal, Tool: engine.ToolClaudeCode, Kind: engine.KindSkill}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := needsCodexRestartHint(tc.skill); got != tc.want {
				t.Fatalf("needsCodexRestartHint(%#v) = %t, want %t", tc.skill, got, tc.want)
			}
		})
	}
}
