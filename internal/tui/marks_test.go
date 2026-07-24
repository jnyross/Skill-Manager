package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

// newMarkFixture builds an inventory of Personal Skills, which are the ones
// Manual-only applies to, with descriptions long enough to carry a non-zero
// estimated cost.
func newMarkFixture(t *testing.T, names ...string) (*engine.Engine, string) {
	t.Helper()
	e, roots, _, _ := newPhase3TUIFixture(t)
	description := "A description with enough words in it to be worth a handful of estimated tokens per session."
	for _, name := range names {
		writeTUISkill(t, filepath.Join(roots.ClaudeHome, "skills", name), name, description)
	}
	return e, filepath.Join(roots.ClaudeHome, "skills")
}

func newSizedMarkModel(t *testing.T, e *engine.Engine) *Model {
	t.Helper()
	m := NewModel(e)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return m
}

func TestSpaceMarksAndUnmarksTheSkillUnderTheCursor(t *testing.T) {
	e, _ := newMarkFixture(t, "alpha", "beta")
	m := newSizedMarkModel(t, e)

	pressTUIKey(m, " ")
	if m.marks.len() != 1 {
		t.Fatalf("space did not mark the selected Skill: %d marked", m.marks.len())
	}
	selected, _ := m.selectedMainSkill()
	if !m.marks.has(selected) {
		t.Fatalf("space marked something other than the row under the cursor")
	}
	if !strings.Contains(m.View(), markGlyph) {
		t.Error("a marked row does not show the mark glyph")
	}

	pressTUIKey(m, " ")
	if m.marks.len() != 0 {
		t.Fatalf("space did not unmark the selected Skill: %d marked", m.marks.len())
	}
	if strings.Contains(m.View(), markGlyph) {
		t.Error("the mark glyph survived unmarking")
	}
}

// Filtering down, marking, clearing the filter and marking more is the actual
// workflow — a Skill marked under one filter must still be marked under the
// next one and after the filter is gone.
func TestMarksSurviveAFilterCycle(t *testing.T) {
	e, _ := newMarkFixture(t, "alpha", "beta", "gamma")
	m := newSizedMarkModel(t, e)

	pressTUIKey(m, "/")
	typeTUIText(m, "alpha")
	pressTUIKey(m, "enter")
	pressTUIKey(m, " ")
	if m.marks.len() != 1 {
		t.Fatalf("marking inside a filter left %d marks", m.marks.len())
	}

	pressTUIKey(m, "esc") // clears the filter, not the marks
	if m.marks.len() != 1 {
		t.Fatalf("clearing the filter dropped the mark: %d marked", m.marks.len())
	}

	pressTUIKey(m, "/")
	typeTUIText(m, "gamma")
	pressTUIKey(m, "enter")
	pressTUIKey(m, " ")
	pressTUIKey(m, "esc")
	if m.marks.len() != 2 {
		t.Fatalf("marks across two filter passes = %d, want 2", m.marks.len())
	}

	names := map[string]bool{}
	for _, skill := range m.markedSkills() {
		names[skill.Name] = true
	}
	if !names["alpha"] || !names["gamma"] {
		t.Fatalf("marked Skills = %v, want alpha and gamma", names)
	}
}

func TestEscClearsMarksOnceTheFilterIsGone(t *testing.T) {
	e, _ := newMarkFixture(t, "alpha", "beta")
	m := newSizedMarkModel(t, e)

	pressTUIKey(m, " ")
	pressTUIKey(m, "esc")
	if m.marks.len() != 0 {
		t.Fatalf("esc left %d marks", m.marks.len())
	}
	if !strings.Contains(m.status, "Cleared 1 mark") {
		t.Errorf("status = %q, want it to report the cleared marks", m.status)
	}
}

func TestBulkManualOnlyWithNothingMarkedSaysSo(t *testing.T) {
	e, _ := newMarkFixture(t, "alpha")
	m := newSizedMarkModel(t, e)

	pressTUIKey(m, "M")
	if m.pending != nil {
		t.Fatal("M staged a confirmation with nothing marked")
	}
	if m.statusLevel != statusError || !strings.Contains(m.status, "Nothing marked") {
		t.Fatalf("status = %q (level %d), want an error naming the empty selection", m.status, m.statusLevel)
	}
	if !strings.Contains(m.status, "space") {
		t.Errorf("status = %q, want it to name the key that marks", m.status)
	}
}

// The confirmation has to answer both halves of the decision: how much is
// changing, and what it buys.
func TestBulkManualOnlyConfirmationStatesCountAndSavings(t *testing.T) {
	e, _ := newMarkFixture(t, "alpha", "beta")
	m := newSizedMarkModel(t, e)

	pressTUIKey(m, " ")
	pressTUIKey(m, "down")
	pressTUIKey(m, " ")
	pressTUIKey(m, "M")

	if m.pending == nil {
		t.Fatal("M did not stage a confirmation")
	}
	savings := engine.EstimateManualOnlySavings(m.markedSkills())
	if savings.Skills != 2 || savings.Tokens <= 0 {
		t.Fatalf("fixture savings = %#v, want 2 Skills and a non-zero estimate", savings)
	}
	description := m.pending.description
	if !strings.Contains(description, "2 Skills") {
		t.Errorf("confirmation = %q, want the count of Skills changing", description)
	}
	if !strings.Contains(description, engine.FormatTokenEstimate(savings.Tokens)) {
		t.Errorf("confirmation = %q, want the estimated saving %s", description, engine.FormatTokenEstimate(savings.Tokens))
	}
	if !strings.Contains(description, "per session") || !strings.Contains(description, "estimated") {
		t.Errorf("confirmation = %q, want it to say the saving is a per-session estimate", description)
	}
}

func TestBulkManualOnlyAppliesAndClearsMarks(t *testing.T) {
	e, _ := newMarkFixture(t, "alpha", "beta", "gamma")
	m := newSizedMarkModel(t, e)

	pressTUIKey(m, " ")
	pressTUIKey(m, "down")
	pressTUIKey(m, " ")
	pressTUIKey(m, "M")
	pressTUIKey(m, "y")

	if m.marks.len() != 0 {
		t.Fatalf("marks survived a successful bulk action: %d marked", m.marks.len())
	}
	if m.statusLevel != statusInfo {
		t.Fatalf("status level = %d, want an informational report: %q", m.statusLevel, m.status)
	}
	if !strings.Contains(m.status, "2 Skills") || !strings.Contains(m.status, "saving") {
		t.Errorf("status = %q, want it to report the count and the saving", m.status)
	}

	manualOnly := 0
	for _, skill := range m.inv.Skills {
		if skill.Activation == engine.ActivationManualOnly {
			manualOnly++
		}
	}
	if manualOnly != 2 {
		t.Fatalf("%d Skills are Manual-only, want 2", manualOnly)
	}
}

// A Skill that could not be changed must be named, not folded into a success
// message. The failures also stay marked, so retrying is one keypress.
func TestBulkManualOnlyReportsPartialFailure(t *testing.T) {
	e, skillsDir := newMarkFixture(t, "alpha", "beta")
	m := newSizedMarkModel(t, e)

	pressTUIKey(m, " ")
	pressTUIKey(m, "down")
	pressTUIKey(m, " ")
	pressTUIKey(m, "M")

	// Pull one marked Skill out from under the confirmed action.
	if err := os.RemoveAll(filepath.Join(skillsDir, "beta")); err != nil {
		t.Fatal(err)
	}
	pressTUIKey(m, "y")

	if m.statusLevel != statusError {
		t.Fatalf("status level = %d, want an error for a partial failure: %q", m.statusLevel, m.status)
	}
	if !strings.Contains(m.status, "1 Skill") {
		t.Errorf("status = %q, want the count that did change", m.status)
	}
	if !strings.Contains(m.status, "1 failure") || !strings.Contains(m.status, "beta") {
		t.Errorf("status = %q, want the failure count and the Skill that failed", m.status)
	}
	if m.marks.len() != 1 {
		t.Fatalf("%d marks after a partial failure, want the failed Skill left marked", m.marks.len())
	}
}

func TestBulkManualOnlyRefusesWhenEverythingMarkedIsAlreadyManualOnly(t *testing.T) {
	e, _ := newMarkFixture(t, "alpha")
	m := newSizedMarkModel(t, e)

	pressTUIKey(m, "m")
	pressTUIKey(m, "y")
	pressTUIKey(m, " ")
	pressTUIKey(m, "M")

	if m.pending != nil {
		t.Fatal("M staged a confirmation for a no-op")
	}
	if !strings.Contains(m.status, "already Manual-only") {
		t.Errorf("status = %q, want it to say there is nothing to do", m.status)
	}
}

func TestMarkedLineReportsCountAndSaving(t *testing.T) {
	e, _ := newMarkFixture(t, "alpha", "beta")
	m := newSizedMarkModel(t, e)

	if m.markedLine() != "" {
		t.Fatal("the marked line renders with nothing marked")
	}
	pressTUIKey(m, " ")

	line := m.markedLine()
	if !strings.Contains(line, "1 Skill marked") {
		t.Errorf("marked line = %q, want the marked count", line)
	}
	savings := engine.EstimateManualOnlySavings(m.markedSkills())
	if !strings.Contains(line, engine.FormatTokenEstimate(savings.Tokens)) {
		t.Errorf("marked line = %q, want the saving the selection is worth", line)
	}
	if !strings.Contains(m.View(), "1 Skill marked") {
		t.Error("the marked count is not on screen")
	}
}

// Marking is offered exactly where Manual-only is, and refused with the same
// reason everywhere else — a mark that could never be acted on is worse than
// no mark.
func TestMarkingRefusesSkillsManualOnlyCannotApplyTo(t *testing.T) {
	e, roots, _, _ := newPhase3TUIFixture(t)
	if err := os.MkdirAll(filepath.Join(roots.CodexHome, "prompts"), 0o755); err != nil {
		t.Fatal(err)
	}
	prompt := "---\ndescription: A Codex prompt, which only ever runs when it is invoked.\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(roots.CodexHome, "prompts", "note.md"), []byte(prompt), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newSizedMarkModel(t, e)
	selected, ok := m.selectedMainSkill()
	if !ok || selected.Kind != engine.KindPrompt {
		t.Fatalf("fixture did not produce a Codex prompt to test against: %#v", selected)
	}

	pressTUIKey(m, " ")
	if m.marks.len() != 0 {
		t.Fatal("space marked a Skill that cannot be made Manual-only")
	}
	if m.statusLevel != statusError || !strings.Contains(m.status, "Manual-only") {
		t.Errorf("status = %q, want the same reason the m key gives", m.status)
	}
}

func TestMoreMenuOpensTheSecondaryDestinations(t *testing.T) {
	e, _ := newMarkFixture(t, "alpha")
	m := newSizedMarkModel(t, e)

	pressTUIKey(m, "o")
	if m.moreMenu == nil {
		t.Fatal("o did not open the More menu")
	}
	view := m.View()
	for _, want := range []string{"Library", "Bundles", "Setup"} {
		if !strings.Contains(view, want) {
			t.Errorf("More menu does not offer %q: %q", want, view)
		}
	}

	pressTUIKey(m, "L")
	if m.moreMenu != nil {
		t.Fatal("choosing a destination left the More menu open")
	}
	if m.view != libraryView {
		t.Fatalf("view = %v, want the Library view", m.view)
	}
}

func TestMoreMenuEscClosesWithoutNavigating(t *testing.T) {
	e, _ := newMarkFixture(t, "alpha")
	m := newSizedMarkModel(t, e)

	pressTUIKey(m, "o")
	pressTUIKey(m, "esc")
	if m.moreMenu != nil {
		t.Fatal("esc did not close the More menu")
	}
	if m.view != mainView {
		t.Fatalf("view = %v, want to still be on the main view", m.view)
	}
}

// The secondary destinations still answer to their own keys; the menu is an
// additional way in, not a replacement.
func TestSecondaryKeysStillWorkDirectly(t *testing.T) {
	e, _ := newMarkFixture(t, "alpha")
	m := newSizedMarkModel(t, e)

	for _, tc := range []struct {
		key  string
		want viewState
	}{{"L", libraryView}, {"B", bundleView}, {"a", archiveView}} {
		pressTUIKey(m, tc.key)
		if m.view != tc.want {
			t.Fatalf("%q went to view %v, want %v", tc.key, m.view, tc.want)
		}
		pressTUIKey(m, "esc")
	}
	pressTUIKey(m, "S")
	if !m.SetupRequested() {
		t.Fatal("S no longer requests Setup directly")
	}
}
