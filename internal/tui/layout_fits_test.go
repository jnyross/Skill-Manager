package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jnyross/Skill-Manager/internal/engine"
)

// newCrowdedFixture builds an inventory that exercises everything competing
// for vertical space at once: enough Skills to fill the list, and several
// Notices, which render below it.
func newCrowdedFixture(t *testing.T, skills, broken int) *engine.Engine {
	t.Helper()
	root := t.TempDir()
	claude := filepath.Join(root, "claude")
	for index := range skills {
		dir := filepath.Join(claude, "skills", "skill-"+string(rune('a'+index%26))+string(rune('a'+index/26)))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		body := "---\nname: " + filepath.Base(dir) + "\ndescription: A reasonably wordy description so the detail pane has real content to wrap across several lines.\n---\n\nBody.\n"
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Malformed Skills raise Notices, which render under the list.
	for index := range broken {
		dir := filepath.Join(claude, "skills", "broken-"+string(rune('a'+index)))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("no frontmatter\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return engine.New(engine.Roots{
		ClaudeHome: claude,
		CodexHome:  filepath.Join(root, "codex"),
		AgentsHome: filepath.Join(root, "agents"),
		DataDir:    filepath.Join(root, "data"),
	})
}

// The main view must fit the terminal it was given. When it does not, the
// terminal scrolls and the top lines — the title, the per-session cost
// header, and the first help row — are pushed off screen where the user
// never sees them. Every element that reserves vertical space has to be
// accounted for in resizeList, so this pins the whole budget rather than any
// one line item.
func TestMainViewFitsTheTerminal(t *testing.T) {
	for _, size := range mainViewSizes {
		m := NewModel(newCrowdedFixture(t, 40, 3))
		m.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})

		if len(m.inv.Notices) == 0 {
			t.Fatal("fixture raised no Notices, so the notice budget is untested")
		}

		assertMainViewFits(t, m, size.width, size.height, "the default state")
	}
}

// mainViewSizes is the terminal shapes every main-view state has to survive.
var mainViewSizes = []struct{ width, height int }{
	{100, 24},
	{100, 30},
	{120, 40},
	{80, 20},
	{200, 60},
}

// assertMainViewFits is the whole invariant in one place: the rendered view is
// no taller than the terminal, so nothing scrolls off the top.
func assertMainViewFits(t *testing.T, m *Model, width, height int, state string) {
	t.Helper()
	lines := strings.Split(strings.TrimRight(m.View(), "\n"), "\n")
	if len(lines) > height {
		t.Errorf("%s at %dx%d rendered %d lines, which does not fit; the top %d line(s) scroll off:\nfirst line: %q",
			state, width, height, len(lines), len(lines)-height, lines[0])
	}
}

// Every line the main view can add — the marked-count line, the More overlay —
// has to be paid for out of resizeList's budget. This is exactly the failure
// the previous round of testing missed: a new element rendered taller than what
// was reserved for it, and the title and cost header went off the top.
func TestMainViewFitsTheTerminalWithMarksActive(t *testing.T) {
	for _, size := range mainViewSizes {
		m := NewModel(newCrowdedFixture(t, 40, 3))
		m.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})

		// Mark several rows the way a user would, which is also what puts the
		// marked-count line on screen.
		for range 5 {
			pressTUIKey(m, " ")
			pressTUIKey(m, "down")
		}
		if m.marks.len() == 0 {
			t.Fatal("fixture marked nothing, so the marked-line budget is untested")
		}
		if m.markedLine() == "" {
			t.Fatal("marks are set but the marked line does not render")
		}
		assertMainViewFits(t, m, size.width, size.height, "marks active")

		// And with a filter applied on top of the marks, which is the state the
		// mark-filter-mark workflow spends most of its time in.
		pressTUIKey(m, "/")
		typeTUIText(m, "skill")
		pressTUIKey(m, "enter")
		assertMainViewFits(t, m, size.width, size.height, "marks active under a filter")
	}
}

func TestMainViewFitsTheTerminalWithTheMoreMenuOpen(t *testing.T) {
	for _, size := range mainViewSizes {
		m := NewModel(newCrowdedFixture(t, 40, 3))
		m.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
		pressTUIKey(m, "o")
		if m.moreMenu == nil {
			t.Fatal("o did not open the More menu")
		}
		assertMainViewFits(t, m, size.width, size.height, "More menu open")
	}
}

// The title and the cost header are the first two lines, so if the view
// overflows they are the first casualties. Assert they are actually present
// in what the user sees.
func TestMainViewKeepsItsTitleAndCostHeaderOnScreen(t *testing.T) {
	m := NewModel(newCrowdedFixture(t, 40, 3))
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	lines := strings.Split(strings.TrimRight(m.View(), "\n"), "\n")
	if len(lines) > 30 {
		t.Fatalf("view does not fit, so nothing about the top is meaningful: %d lines", len(lines))
	}
	if !strings.Contains(lines[0], "Skillet") {
		t.Errorf("first line = %q, want the Skillet title", lines[0])
	}
	if !strings.Contains(lines[1], "Every session") {
		t.Errorf("second line = %q, want the per-session cost header", lines[1])
	}
}
