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
	for _, size := range []struct{ width, height int }{
		{100, 24},
		{100, 30},
		{120, 40},
		{80, 20},
		{200, 60},
	} {
		m := NewModel(newCrowdedFixture(t, 40, 3))
		m.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})

		if len(m.inv.Notices) == 0 {
			t.Fatal("fixture raised no Notices, so the notice budget is untested")
		}

		lines := strings.Split(strings.TrimRight(m.View(), "\n"), "\n")
		if len(lines) > size.height {
			t.Errorf("at %dx%d the view rendered %d lines, which does not fit; the top %d line(s) scroll off:\nfirst line: %q",
				size.width, size.height, len(lines), len(lines)-size.height, lines[0])
		}
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
