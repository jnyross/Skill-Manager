package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"

	"github.com/jnyross/Skill-Manager/internal/engine"
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

// shortHelpRowWidth is what bubbles/help would render a row at: each binding
// as "key desc", joined by the default three-column " • " separator.
func shortHelpRowWidth(row []key.Binding) int {
	width, shown := 0, 0
	for _, b := range row {
		h := b.Help()
		if h.Key == "" {
			continue // unset binding for this view
		}
		width += lipgloss.Width(h.Key) + 1 + lipgloss.Width(h.Desc)
		shown++
	}
	if shown > 1 {
		width += 3 * (shown - 1)
	}
	return width
}

func TestShortHelpRowsFit80Columns(t *testing.T) {
	maps := map[string]keyMap{
		"main":    mainKeyMap(engine.Skill{Source: engine.SourceCodex, Kind: engine.KindSkill}, true, false),
		"archive": archiveKeyMap(true, false),
		"library": libraryKeyMap(true, false),
		"bundle":  bundleKeyMap(true, false),
	}
	for name, km := range maps {
		for i, row := range km.ShortHelpRows() {
			if width := shortHelpRowWidth(row); width > 80 {
				t.Errorf("%s ShortHelpRows row %d is %d columns wide, want <= 80", name, i, width)
			}
		}
	}
}

func TestMainShortHelpExposesEveryModeChangingKey(t *testing.T) {
	m := mainKeyMap(engine.Skill{Source: engine.SourcePlugin, Kind: engine.KindSkill}, true, false)

	seen := map[string]bool{}
	for _, row := range m.ShortHelpRows() {
		for _, b := range row {
			for _, k := range b.Keys() {
				seen[k] = true
			}
		}
	}

	// Every key that changes mode or state used to be hidden until `?`.
	for _, k := range []string{"s", "m", "x", "l", "L", "B", "/", "u", "a", "S", "?", "q"} {
		if !seen[k] {
			t.Errorf("main compact help does not expose %q", k)
		}
	}
}

func TestMainFullHelpContainsLessFrequentActions(t *testing.T) {
	m := mainKeyMap(engine.Skill{Source: engine.SourcePersonal, Kind: engine.KindSkill}, true, false)
	full := m.FullHelp()

	var found []string
	for _, row := range full {
		for _, b := range row {
			found = append(found, b.Help().Desc)
		}
	}

	for _, desc := range []string{"Suppress", "Manual-only", "Uninstall plugin", "Library view", "Bundle view"} {
		present := false
		for _, d := range found {
			if d == desc {
				present = true
				break
			}
		}
		if !present {
			t.Fatalf("main FullHelp missing %q", desc)
		}
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
