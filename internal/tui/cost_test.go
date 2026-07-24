package tui

// Tests for the cost surfaces (WP5): the detail pane's Cost section, the
// sort-by-cost toggle, and the per-session aggregate in the main header.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

func costFixtureSkill(activation engine.ActivationState) engine.Skill {
	return engine.Skill{
		Name:              "example",
		Description:       "A helpful skill.",
		Source:            engine.SourcePersonal,
		Tool:              engine.ToolClaudeCode,
		Kind:              engine.KindSkill,
		Location:          "/tmp/example",
		Activation:        activation,
		DescriptionTokens: 120,
		BodyBytes:         5400,
		BodyTokens:        1350,
		FileCount:         7,
		TotalBytes:        42000,
	}
}

func TestDetailCostSectionSeparatesPerSessionFromInvokedCost(t *testing.T) {
	t.Parallel()
	content := detailContent(costFixtureSkill(engine.ActivationAuto), true, 80)

	if !strings.Contains(content, "Cost (estimated)") {
		t.Fatalf("no Cost section:\n%s", content)
	}
	for _, want := range []string{"Per session", "~120 tokens", "When invoked", "~1,350 tokens", "5.3 KB", "On disk", "7 files"} {
		if !strings.Contains(content, want) {
			t.Errorf("Cost section is missing %q:\n%s", want, content)
		}
	}
	if !strings.Contains(content, "injected into every Claude Code session") {
		t.Errorf("the per-session line must say what the cost is for:\n%s", content)
	}
}

// A Manual-only Skill costs nothing per session. Showing its description cost
// as if it were standing would be the single most misleading thing this pane
// could do.
func TestDetailCostSectionShowsZeroPerSessionWhenNotAuto(t *testing.T) {
	t.Parallel()
	content := detailContent(costFixtureSkill(engine.ActivationManualOnly), true, 80)

	if !strings.Contains(content, "Per session   ~0 tokens") {
		t.Errorf("a Manual-only Skill must show no per-session cost:\n%s", content)
	}
	if !strings.Contains(content, "Manual-only, so its description is not injected") {
		t.Errorf("the reason must be stated:\n%s", content)
	}
	if !strings.Contains(content, "~120 tokens if set back to Auto") {
		t.Errorf("the counterfactual cost must still be visible:\n%s", content)
	}
}

// Every user-facing cost number carries a "~" and the section says "estimated".
func TestDetailCostNumbersAreLabelledEstimates(t *testing.T) {
	t.Parallel()
	content := detailContent(costFixtureSkill(engine.ActivationAuto), true, 80)
	if !strings.Contains(content, "Cost (estimated)") {
		t.Fatalf("the Cost label must say estimated:\n%s", content)
	}
	if strings.Contains(content, " 120 tokens") {
		t.Errorf("a bare token count leaked into the pane:\n%s", content)
	}
}

func TestDetailPaneMeasuresTheSelectedSkillOnDisk(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	folder := filepath.Join(dir, "measured")
	if err := os.MkdirAll(filepath.Join(folder, "references"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, "SKILL.md"), []byte(strings.Repeat("a", 400)), 0o644); err != nil {
		t.Fatalf("write body: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, "references", "guide.md"), []byte(strings.Repeat("b", 600)), 0o644); err != nil {
		t.Fatalf("write reference: %v", err)
	}

	pane := newDetailPane()
	pane.setSize(80, 20)
	// The engine leaves FileCount and TotalBytes unmeasured; the pane fills them
	// in for the Skill the user is actually looking at.
	pane.setSkill(engine.Skill{
		Name: "measured", Source: engine.SourcePersonal, Tool: engine.ToolClaudeCode,
		Kind: engine.KindSkill, Location: folder, Activation: engine.ActivationAuto,
		Description: "Measured skill", DescriptionTokens: 4, BodyBytes: 400, BodyTokens: 100,
	}, true)

	if !strings.Contains(pane.content, "2 files") {
		t.Errorf("pane did not measure the Skill directory:\n%s", pane.content)
	}
	if got := pane.measured[folder]; got.files != 2 || got.bytes != 1000 {
		t.Errorf("cached footprint = %+v, want 2 files / 1000 bytes", got)
	}
}

func TestCostHeaderCountsOnlyAutoAndSaysSo(t *testing.T) {
	t.Parallel()
	m := NewModel(engine.New(engine.Roots{}))
	m.width = 120
	m.inv = engine.Inventory{Skills: []engine.Skill{
		{Name: "a", Tool: engine.ToolClaudeCode, Activation: engine.ActivationAuto, DescriptionTokens: 100},
		{Name: "b", Tool: engine.ToolClaudeCode, Activation: engine.ActivationManualOnly, DescriptionTokens: 900},
		{Name: "c", Tool: engine.ToolCodex, Activation: engine.ActivationAuto, DescriptionTokens: 40},
		{Name: "d", Tool: engine.ToolCodex, Activation: engine.ActivationSuppressed, DescriptionTokens: 900},
	}}

	line := m.costHeaderLine()
	for _, want := range []string{"est.", "Claude Code ~100", "Codex ~40", "Auto descriptions only", "2 excluded"} {
		if !strings.Contains(line, want) {
			t.Errorf("header %q is missing %q", line, want)
		}
	}
	if strings.Contains(line, "900") || strings.Contains(line, "~1,040") {
		t.Errorf("header %q counted a Skill that does not Auto-activate", line)
	}
}

func TestCostHeaderFitsNarrowTerminalsAndStaysHonest(t *testing.T) {
	t.Parallel()
	m := NewModel(engine.New(engine.Roots{}))
	m.inv = engine.Inventory{Skills: []engine.Skill{
		{Name: "a", Tool: engine.ToolClaudeCode, Activation: engine.ActivationAuto, DescriptionTokens: 1430},
		{Name: "b", Tool: engine.ToolCodex, Activation: engine.ActivationAuto, DescriptionTokens: 210},
		{Name: "c", Tool: engine.ToolCodex, Activation: engine.ActivationDisabled, DescriptionTokens: 900},
	}}

	for _, width := range []int{20, 34, 60, 100, 200} {
		m.width = width
		line := m.costHeaderLine()
		if lipgloss.Width(line) > width {
			t.Errorf("header at width %d is %d columns: %q", width, lipgloss.Width(line), line)
		}
		if !strings.Contains(line, "~") {
			t.Errorf("header at width %d dropped the estimate marker: %q", width, line)
		}
	}
}

func TestCostHeaderIsEmptyWithNoSkills(t *testing.T) {
	t.Parallel()
	m := NewModel(engine.New(engine.Roots{}))
	m.width = 100
	if line := m.costHeaderLine(); line != "" {
		t.Errorf("empty inventory produced a header: %q", line)
	}
}

func TestSortByCostRanksTheListAndFlattensGroups(t *testing.T) {
	t.Parallel()
	m := NewModel(engine.New(engine.Roots{}))
	m.width = 100
	m.inv = engine.Inventory{Skills: []engine.Skill{
		{Name: "cheap", Source: engine.SourcePersonal, Activation: engine.ActivationAuto, DescriptionTokens: 10},
		{Name: "dear", Source: engine.SourceCodex, Activation: engine.ActivationAuto, DescriptionTokens: 900},
	}}

	grouped := buildListItems(m.inv)
	if _, ok := grouped[0].(groupHeaderItem); !ok {
		t.Fatalf("the default view groups by Source, got %T first", grouped[0])
	}

	m.sortByCost = true
	m.inv.Skills = engine.SortByDescriptionCost(m.inv.Skills)
	ranked := buildCostSortedListItems(m.inv)
	if len(ranked) != 2 {
		t.Fatalf("ranked list has %d rows, want 2 with no Source headers", len(ranked))
	}
	first, ok := ranked[0].(skillItem)
	if !ok || first.skill.Name != "dear" {
		t.Fatalf("most expensive Skill is not first: %#v", ranked[0])
	}
}

// The toggle is a view change: it must report what it did and leave the
// inventory intact.
func TestSortByCostKeyTogglesAndReports(t *testing.T) {
	t.Parallel()
	m := NewModel(engine.New(engine.Roots{}))
	m.width = 100

	m.updateMain("c")
	if !m.sortByCost {
		t.Fatal("c did not turn on the cost ranking")
	}
	if !strings.Contains(m.status, "cost per session") {
		t.Errorf("status = %q, want it to name the new order", m.status)
	}

	m.updateMain("c")
	if m.sortByCost {
		t.Fatal("c did not turn the cost ranking back off")
	}
	if !strings.Contains(m.status, "Grouped by Source") {
		t.Errorf("status = %q, want it to name the restored order", m.status)
	}
}

// The sort key must be documented where the user looks for it.
func TestSortByCostKeyIsDiscoverable(t *testing.T) {
	t.Parallel()
	km := mainKeyMap(engine.Skill{}, false, false)
	found := false
	for _, row := range km.ShortHelpRows() {
		for _, binding := range row {
			if binding.Help().Key == "c" {
				found = true
			}
		}
	}
	if !found {
		t.Error("c is missing from the compact help rows")
	}
}
