package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

func TestLibraryInstallFlowChoosesProjectAndConfirmsReplacement(t *testing.T) {
	e, roots, root, repo := newPhase3TUIFixture(t)
	source := writeTUISkill(t, filepath.Join(root, "sources", "reviewer"), "reviewer", "current")
	destination := writeTUISkill(t, filepath.Join(repo, ".claude", "skills", "reviewer"), "reviewer", "stale")
	entry, err := e.AddLibraryEntry(engine.LibraryEntry{
		Name:   "reviewer",
		Kind:   engine.KindSkill,
		Tool:   engine.ToolClaudeCode,
		Source: engine.LibrarySource{Kind: engine.LibrarySourceLocalPath, LocalPath: source},
	})
	if err != nil {
		t.Fatal(err)
	}

	m := NewModel(e)
	pressTUIKey(m, "L")
	pressTUIKey(m, "i")
	if m.installPicker == nil {
		t.Fatal("Library Install did not open the target picker")
	}
	pressTUIKey(m, "down")
	pressTUIKey(m, "enter")
	if m.pending == nil || m.pending.entry.ID != entry.ID {
		t.Fatalf("replacement did not require confirmation: %#v", m.pending)
	}
	pressTUIKey(m, "y")

	data, err := os.ReadFile(filepath.Join(destination, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "current") {
		t.Fatalf("installed content = %q", data)
	}
	if !strings.Contains(m.status, "Installed") || !strings.Contains(m.status, repo) {
		t.Fatalf("status = %q", m.status)
	}
	if _, err := os.Stat(filepath.Join(roots.DataDir, "library", entry.ID+".json")); err != nil {
		t.Fatalf("Install removed Library record: %v", err)
	}
}

func TestMainSetupShortcutRequestsSharedSetupFlow(t *testing.T) {
	e, _, _, _ := newPhase3TUIFixture(t)
	model := NewModel(e)
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	got := updated.(*Model)
	if !got.SetupRequested() {
		t.Fatal("S did not request Setup")
	}
	if command == nil {
		t.Fatal("S did not quit the inventory TUI before opening Setup")
	}
}

func TestBundleInstallFlowAppliesEachRememberedActivation(t *testing.T) {
	e, _, root, repo := newPhase3TUIFixture(t)
	aSource := writeTUISkill(t, filepath.Join(root, "sources", "auto"), "auto", "auto")
	mSource := writeTUISkill(t, filepath.Join(root, "sources", "manual"), "manual", "manual")
	a, err := e.AddLibraryEntry(engine.LibraryEntry{Name: "auto", Kind: engine.KindSkill, Tool: engine.ToolClaudeCode, Source: engine.LibrarySource{Kind: engine.LibrarySourceLocalPath, LocalPath: aSource}})
	if err != nil {
		t.Fatal(err)
	}
	manual, err := e.AddLibraryEntry(engine.LibraryEntry{Name: "manual", Kind: engine.KindSkill, Tool: engine.ToolClaudeCode, Source: engine.LibrarySource{Kind: engine.LibrarySourceLocalPath, LocalPath: mSource}})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := e.CreateBundle("review loop")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.AddBundleMember(bundle.ID, a.ID, engine.ActivationAuto); err != nil {
		t.Fatal(err)
	}
	if err := e.AddBundleMember(bundle.ID, manual.ID, engine.ActivationManualOnly); err != nil {
		t.Fatal(err)
	}

	m := NewModel(e)
	pressTUIKey(m, "B")
	pressTUIKey(m, "i")
	pressTUIKey(m, "down")
	pressTUIKey(m, "enter")

	if _, err := os.Stat(filepath.Join(repo, ".claude", "skills", "auto", "SKILL.md")); err != nil {
		t.Fatalf("Auto member missing: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(repo, ".claude", "skills", "manual", "SKILL.md"))
	if err != nil {
		t.Fatalf("Manual-only member missing: %v", err)
	}
	if !strings.Contains(string(data), "disable-model-invocation: true") {
		t.Fatalf("Manual-only Activation not applied: %s", data)
	}
	if !strings.Contains(m.status, `Installed Bundle "review loop"`) {
		t.Fatalf("status = %q", m.status)
	}
}

func TestLibraryNewEntryFlowCreatesLocalPathDescriptor(t *testing.T) {
	e, _, root, _ := newPhase3TUIFixture(t)
	source := writeTUISkill(t, filepath.Join(root, "sources", "local"), "local", "local")
	m := NewModel(e)

	pressTUIKey(m, "L")
	pressTUIKey(m, "n")
	pressTUIKey(m, "enter")
	typeTUIText(m, "local")
	pressTUIKey(m, "enter")
	typeTUIText(m, "claude-code")
	pressTUIKey(m, "enter")
	typeTUIText(m, source)
	pressTUIKey(m, "enter")

	entries, err := e.ListLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("Library entries = %#v", entries)
	}
	if entries[0].Name != "local" || entries[0].Tool != engine.ToolClaudeCode || entries[0].Source.LocalPath != source {
		t.Fatalf("Library entry = %#v", entries[0])
	}
}

func newPhase3TUIFixture(t *testing.T) (*engine.Engine, engine.Roots, string, string) {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	roots := engine.Roots{
		ClaudeHome:         filepath.Join(root, "claude"),
		CodexHome:          filepath.Join(root, "codex"),
		AgentsHome:         filepath.Join(root, "agents"),
		DataDir:            filepath.Join(root, "data"),
		ProjectRoots:       []string{repo},
		ClaudeProjectRoots: []string{repo},
	}
	for _, path := range []string{
		filepath.Join(roots.ClaudeHome, "skills"),
		filepath.Join(roots.ClaudeHome, "plugins"),
		filepath.Join(roots.CodexHome, "skills"),
		filepath.Join(roots.AgentsHome, "skills"),
		roots.DataDir,
		repo,
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return engine.New(roots), roots, root, repo
}

func writeTUISkill(t *testing.T, dir, name, description string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	contents := "---\nname: " + name + "\ndescription: " + description + "\n---\nBody\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func tuiKeyMsg(key string) tea.KeyMsg {
	switch key {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
}

func pressTUIKey(m *Model, key string) {
	_, cmd := m.Update(tuiKeyMsg(key))
	drainTUICmd(m, cmd)
}

func typeTUIText(m *Model, value string) {
	for _, r := range value {
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		drainTUICmd(m, cmd)
	}
}

// drainTUICmd runs a command tree to completion against the model, the way the
// Bubble Tea runtime would. Filtering depends on this: the Bubbles list
// computes matches in a tea.Cmd and delivers them as a list.FilterMatchesMsg,
// and those commands arrive batched.
func drainTUICmd(m *Model, cmd tea.Cmd) {
	for depth := 0; cmd != nil && depth < 64; depth++ {
		msg := runTUICmd(cmd)
		if msg == nil {
			return
		}
		batch, ok := msg.(tea.BatchMsg)
		if !ok {
			_, cmd = m.Update(msg)
			continue
		}
		var next []tea.Cmd
		for _, sub := range batch {
			if sub == nil {
				continue
			}
			subMsg := runTUICmd(sub)
			if subMsg == nil {
				continue
			}
			if _, out := m.Update(subMsg); out != nil {
				next = append(next, out)
			}
		}
		cmd = tea.Batch(next...)
	}
}

// runTUICmd runs one command, giving up on anything that blocks. The text
// input's cursor-blink command sleeps for about half a second by design; the
// real runtime handles that concurrently, and a test has nothing to learn by
// waiting for it.
func runTUICmd(cmd tea.Cmd) tea.Msg {
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	select {
	case msg := <-result:
		return msg
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}
