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

// The full-help toggle must show the Setup binding and still respect a
// narrow width rather than overflowing.
func TestFullHelpShowsSetupAndFitsNarrowTerminal(t *testing.T) {
	const width = 30
	e, roots, _, _ := newPhase3TUIFixture(t)
	writeTUISkill(t, filepath.Join(roots.ClaudeHome, "skills", "alpha"), "alpha", "first")
	m := NewModel(e)
	m.Update(tea.WindowSizeMsg{Width: width, Height: 8})
	pressTUIKey(m, "?")
	view := m.View()
	if !strings.Contains(view, "Setup workspace") {
		t.Fatalf("full help does not document the Setup binding: %q", view)
	}
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
