package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The main view must stay renderable and navigable at narrow terminal
// sizes: every rendered line fits the terminal width and cursor movement
// keeps a selection.
func TestMainViewRendersWithinNarrowTerminal(t *testing.T) {
	e, roots, _, _ := newPhase3TUIFixture(t)
	writeTUISkill(t, filepath.Join(roots.ClaudeHome, "skills", "alpha"), "alpha", "first")
	writeTUISkill(t, filepath.Join(roots.ClaudeHome, "skills", "beta"), "beta", "second")
	m := NewModel(e)

	for _, size := range []struct{ width, height int }{{20, 6}, {34, 8}, {60, 12}} {
		m.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
		view := m.View()
		if strings.TrimSpace(view) == "" {
			t.Fatalf("view empty at %dx%d", size.width, size.height)
		}
		for _, line := range strings.Split(view, "\n") {
			if got := lipgloss.Width(line); got > size.width {
				t.Errorf("line wider than %d-column terminal (%d): %q", size.width, got, line)
			}
		}
		pressTUIKey(m, "down")
		pressTUIKey(m, "up")
		if _, ok := m.selectedMainSkill(); !ok {
			t.Fatalf("selection lost after navigation at %dx%d", size.width, size.height)
		}
	}
}

// Setup is secondary in the compact help, so the full help is where it has to
// be documented. Bubbles renders full help in columns and drops the columns a
// narrow terminal cannot fit, so this asserts the content at a width that fits
// them all — and the narrow case below asserts what actually matters there,
// that nothing overflows.
func TestFullHelpDocumentsSetupAndTheSecondaryDestinations(t *testing.T) {
	e, roots, _, _ := newPhase3TUIFixture(t)
	writeTUISkill(t, filepath.Join(roots.ClaudeHome, "skills", "alpha"), "alpha", "first")
	m := NewModel(e)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	pressTUIKey(m, "?")
	view := m.View()
	for _, want := range []string{"Setup workspace", "Library view", "Bundle view", "More"} {
		if !strings.Contains(view, want) {
			t.Errorf("full help does not document %q: %q", want, view)
		}
	}
}

func TestFullHelpFitsNarrowTerminal(t *testing.T) {
	const width = 30
	e, roots, _, _ := newPhase3TUIFixture(t)
	writeTUISkill(t, filepath.Join(roots.ClaudeHome, "skills", "alpha"), "alpha", "first")
	m := NewModel(e)
	m.Update(tea.WindowSizeMsg{Width: width, Height: 8})
	pressTUIKey(m, "?")
	view := m.View()
	for _, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Errorf("full-help line wider than terminal (%d): %q", got, line)
		}
	}
}

// Setup is a main-view action: neither lowercase s in the main view nor
// S in the Archive, Library, or Bundle views may request Setup, so the
// shortcut cannot disrupt existing navigation or view-local bindings.
func TestSetupShortcutConfinedToMainView(t *testing.T) {
	e, _, _, _ := newPhase3TUIFixture(t)
	m := NewModel(e)

	pressTUIKey(m, "s")
	if m.SetupRequested() {
		t.Fatal("lowercase s requested Setup from the main view")
	}
	for _, toggle := range []string{"a", "L", "B"} {
		pressTUIKey(m, toggle)
		pressTUIKey(m, "S")
		if m.SetupRequested() {
			t.Fatalf("S requested Setup from the %q view", toggle)
		}
		pressTUIKey(m, toggle)
	}

	pressTUIKey(m, "S")
	if !m.SetupRequested() {
		t.Fatal("S no longer requests Setup from the main view")
	}
}
