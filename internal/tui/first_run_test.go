package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

// newFreshHomeFixture is a brand-new machine: none of the standard skill
// directories exist yet. Nothing is created, which is exactly the state that
// used to produce three error-shaped Notices on the very first screen.
func newFreshHomeFixture(t *testing.T) *engine.Engine {
	t.Helper()
	root := t.TempDir()
	return engine.New(engine.Roots{
		ClaudeHome: filepath.Join(root, "claude"),
		CodexHome:  filepath.Join(root, "codex"),
		AgentsHome: filepath.Join(root, "agents"),
		DataDir:    filepath.Join(root, "data"),
	})
}

func TestFreshHomeShowsWelcomingEmptyStateWithNoNotices(t *testing.T) {
	m := NewModel(newFreshHomeFixture(t))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// The engine still raises the notices; the TUI is what recognizes them as
	// the ordinary fresh-machine state.
	if len(m.inv.Notices) == 0 {
		t.Skip("engine no longer raises missing-directory notices; the render filter is now redundant")
	}
	if got := m.visibleInventoryNotices(); len(got) != 0 {
		t.Fatalf("fresh machine shows %d notice(s): %#v", len(got), got)
	}

	view := m.View()
	if strings.Contains(view, "Notices") {
		t.Fatalf("fresh machine renders a Notices section: %q", view)
	}
	if strings.Contains(view, "not found") {
		t.Fatalf("fresh machine renders a not-found line: %q", view)
	}
	if m.statusLevel == statusError || m.status != "" {
		t.Fatalf("fresh machine opens with a status line: %q (level %v)", m.status, m.statusLevel)
	}
	for _, want := range []string{"No skills yet.", "Press S for guided Setup", "L to open the Library"} {
		if !strings.Contains(view, want) {
			t.Fatalf("empty state missing %q: %q", want, view)
		}
	}
	// statusErrorStyle is the only error styling the main view applies, and it
	// is only reachable through statusLevel == statusError, asserted above.
	// (Asserting on ANSI directly is not possible here: lipgloss degrades to a
	// no-op profile under `go test`, where there is no terminal.)
}

// Only the "the standard directory is simply absent" notice is filtered.
// Anything that says something actually went wrong still reaches the user.
func TestAnomalousNoticesSurviveTheFreshMachineFilter(t *testing.T) {
	kept := []engine.Notice{
		{Message: "Personal skills directory unreadable: /home/x/.claude/skills: permission denied"},
		{Message: "Codex skills directory unreadable: /home/x/.codex/skills: permission denied"},
		{Message: "Skipped broken: SKILL.md: missing name"},
		{Message: "Plugin install path missing: /home/x/.claude/plugins/cache/gone"},
	}
	dropped := []engine.Notice{
		{Message: "Personal skills directory not found: /home/x/.claude/skills"},
		{Message: "Project Claude skills directory not found: /repo/.claude/skills"},
		{Message: "Codex skills directory not found: /home/x/.codex/skills"},
	}

	got := visibleNotices(append(append([]engine.Notice(nil), dropped...), kept...))
	if len(got) != len(kept) {
		t.Fatalf("visibleNotices kept %d notice(s), want %d: %#v", len(got), len(kept), got)
	}
	for index, notice := range kept {
		if got[index] != notice {
			t.Fatalf("notice %d = %#v, want %#v", index, got[index], notice)
		}
	}
}

// The filter must not hide a real inventory: once a skill exists the empty
// state is gone and genuine notices still render.
func TestRealNoticesStillRenderAlongsideSkills(t *testing.T) {
	root := t.TempDir()
	roots := engine.Roots{
		ClaudeHome: filepath.Join(root, "claude"),
		CodexHome:  filepath.Join(root, "codex"),
		AgentsHome: filepath.Join(root, "agents"),
		DataDir:    filepath.Join(root, "data"),
	}
	if err := os.MkdirAll(filepath.Join(roots.ClaudeHome, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTUISkill(t, filepath.Join(roots.ClaudeHome, "skills", "alpha"), "alpha", "first")
	// A malformed skill: a directory with no readable SKILL.md.
	if err := os.MkdirAll(filepath.Join(roots.ClaudeHome, "skills", "broken"), 0o755); err != nil {
		t.Fatal(err)
	}

	m := NewModel(engine.New(roots))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	view := m.View()
	if !strings.Contains(view, "Notices") || !strings.Contains(view, "broken") {
		t.Fatalf("a malformed skill was filtered out of the Notices section: %q", view)
	}
}
