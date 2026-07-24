package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

// blockingRunner stands in for a slow `git clone`: it never finishes on its
// own, only when the caller's context is cancelled. started closes as soon as
// the subprocess would have begun.
type blockingRunner struct {
	started chan struct{}
	seenCtx chan error
}

func newBlockingRunner() *blockingRunner {
	return &blockingRunner{started: make(chan struct{}, 1), seenCtx: make(chan error, 1)}
}

func (r *blockingRunner) Run(ctx context.Context, _ engine.Command) (engine.CommandResult, error) {
	select {
	case r.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	select {
	case r.seenCtx <- ctx.Err():
	default:
	}
	return engine.CommandResult{}, ctx.Err()
}

// newInstallInFlightModel returns a model with a git-backed Library entry whose
// Install has been dispatched and is blocked inside the runner.
func newInstallInFlightModel(t *testing.T) (*Model, *blockingRunner) {
	t.Helper()
	root := t.TempDir()
	roots := engine.Roots{
		ClaudeHome: filepath.Join(root, "claude"),
		CodexHome:  filepath.Join(root, "codex"),
		AgentsHome: filepath.Join(root, "agents"),
		DataDir:    filepath.Join(root, "data"),
	}
	for _, path := range []string{filepath.Join(roots.ClaudeHome, "skills"), roots.DataDir} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeTUISkill(t, filepath.Join(roots.ClaudeHome, "skills", "alpha"), "alpha", "first")

	runner := newBlockingRunner()
	e := engine.NewWithCommandRunner(roots, runner)
	if _, err := e.AddLibraryEntry(engine.LibraryEntry{
		Name:   "remote",
		Kind:   engine.KindSkill,
		Tool:   engine.ToolClaudeCode,
		Source: engine.LibrarySource{Kind: engine.LibrarySourceGit, GitURL: "https://example.invalid/remote.git"},
	}); err != nil {
		t.Fatal(err)
	}

	m := NewModel(e)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	pressTUIKey(m, "L")
	pressTUIKey(m, "i")
	if m.installPicker == nil {
		t.Fatal("Install did not open the target picker")
	}
	pressTUIKey(m, "enter")

	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("install work never reached the command runner")
	}
	if m.installing != "remote" {
		t.Fatalf("installing = %q, want remote", m.installing)
	}
	t.Cleanup(func() {
		if m.installCancel != nil {
			m.installCancel()
		}
	})
	return m, runner
}

// An install in flight must look alive — a spinner frame plus the step it
// dispatched — and must refuse the destructive keys until it is over.
func TestInstallRendersSpinnerFrameAndGatesDestructiveKeys(t *testing.T) {
	m, _ := newInstallInFlightModel(t)

	frame := strings.TrimSpace(m.spinner.View())
	if frame == "" {
		t.Fatal("spinner rendered no frame")
	}
	view := m.View()
	if !strings.Contains(view, frame) {
		t.Fatalf("view has no spinner frame %q: %q", frame, view)
	}
	for _, want := range []string{"Installing remote…", installStepCloning, "esc cancels"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q: %q", want, view)
		}
	}

	// Library view: remove-entry and a second Install are refused.
	for _, key := range []string{"d", "i"} {
		pressTUIKey(m, key)
		if m.pending != nil || m.installPicker != nil {
			t.Fatalf("Library key %q was not gated during an install", key)
		}
		if m.statusLevel != statusError || !strings.Contains(m.status, "Install in progress") {
			t.Fatalf("Library key %q status = %q (level %v)", key, m.status, m.statusLevel)
		}
		// The refusal has to be visible even though the spinner line normally
		// stands in for the status while an install runs.
		if !strings.Contains(m.View(), "Install in progress") {
			t.Fatalf("Library key %q refusal is invisible: %q", key, m.View())
		}
	}

	// Main view: Archive and Uninstall plugin are refused.
	pressTUIKey(m, "L")
	if m.view != mainView {
		t.Fatalf("view = %v, want mainView", m.view)
	}
	for _, key := range []string{"u", "x", "s", "m", "l"} {
		pressTUIKey(m, key)
		if m.pending != nil {
			t.Fatalf("main key %q staged a confirmation during an install", key)
		}
		if !strings.Contains(m.status, "Install in progress") {
			t.Fatalf("main key %q status = %q", key, m.status)
		}
	}

	// Archive view: Purge and Restore are refused.
	pressTUIKey(m, "a")
	for _, key := range []string{"p", "r"} {
		pressTUIKey(m, key)
		if m.pending != nil {
			t.Fatalf("Archive key %q staged a confirmation during an install", key)
		}
	}

	if m.installing != "remote" {
		t.Fatalf("install was disturbed by the gated keys: %q", m.installing)
	}
}

// esc must cancel through the engine's context seam, and the resulting engine
// error must read as a cancellation rather than a failure.
func TestEscCancelsInFlightInstall(t *testing.T) {
	m, runner := newInstallInFlightModel(t)

	pressTUIKey(m, "esc")

	select {
	case err := <-runner.seenCtx:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("runner saw ctx error %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("esc did not cancel the install's context")
	}

	// The engine reports the cancellation the way WP1 rewrites it.
	m.Update(installFinishedMsg{err: errors.New("git cancelled: signal: killed"), desc: "remote"})
	if m.installing != "" {
		t.Fatalf("installing = %q after cancellation", m.installing)
	}
	if !strings.Contains(m.status, "cancelled") {
		t.Fatalf("status = %q, want a cancellation", m.status)
	}
	if m.statusLevel == statusError {
		t.Fatalf("a user cancellation was reported as an error: %q", m.status)
	}
}

// Quitting mid-install must wait for the cancelled work to return rather than
// exiting and orphaning the child process.
func TestQuitDuringInstallCancelsThenQuits(t *testing.T) {
	m, runner := newInstallInFlightModel(t)

	_, cmd := m.Update(tuiKeyMsg("q"))
	if cmd != nil {
		t.Fatal("q quit immediately during an install")
	}
	if !m.quitAfterInstall {
		t.Fatal("q did not defer the quit until the install returned")
	}
	select {
	case <-runner.seenCtx:
	case <-time.After(2 * time.Second):
		t.Fatal("q did not cancel the install's context")
	}

	_, cmd = m.Update(installFinishedMsg{err: errors.New("git cancelled: signal: killed"), desc: "remote"})
	if cmd == nil {
		t.Fatal("the deferred quit never fired")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("the deferred command was not a quit")
	}
}

// A timeout must name the operation and how long the user actually waited.
func TestInstallTimeoutIsSurfacedWithOperationAndElapsedTime(t *testing.T) {
	m := NewModel(engine.New(engine.Roots{}))
	m.installing = "remote"
	m.installStarted = time.Now().Add(-2 * time.Second)

	m.Update(installFinishedMsg{
		err:  errors.New("git timed out after 2m0s: signal: killed"),
		desc: "remote",
	})

	if m.statusLevel != statusError {
		t.Fatalf("timeout status level = %v, want statusError", m.statusLevel)
	}
	for _, want := range []string{`Install of "remote"`, "timed out after 2s", "git timed out after 2m0s"} {
		if !strings.Contains(m.status, want) {
			t.Fatalf("status = %q, missing %q", m.status, want)
		}
	}
}

// The spinner stops ticking once nothing is in flight, so an idle TUI does no
// per-frame work.
func TestSpinnerTickStopsWhenNoInstallIsRunning(t *testing.T) {
	m := NewModel(engine.New(engine.Roots{}))
	if _, cmd := m.Update(m.spinner.Tick()); cmd != nil {
		t.Fatal("spinner kept ticking with no install in flight")
	}
}
