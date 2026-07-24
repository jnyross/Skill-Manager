package engine_test

// Subprocess timeouts and cancellation. Every external program the engine runs
// goes through a context with a deadline, so a hung `git clone` or `npx`
// surfaces as an error naming the program and the limit rather than a UI that
// never comes back.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

// hangingRunner never returns until its context ends — a stand-in for a clone
// waiting on an unreachable host or a credential prompt.
type hangingRunner struct {
	started chan struct{}
}

func (h *hangingRunner) Run(ctx context.Context, _ engine.Command) (engine.CommandResult, error) {
	if h.started != nil {
		select {
		case h.started <- struct{}{}:
		default:
		}
	}
	<-ctx.Done()
	return engine.CommandResult{}, ctx.Err()
}

func gitEntry() engine.LibraryEntry {
	return engine.LibraryEntry{
		Name:   "from-git",
		Kind:   engine.KindSkill,
		Tool:   engine.ToolClaudeCode,
		Source: engine.LibrarySource{Kind: engine.LibrarySourceGit, GitURL: "https://example.invalid/repo.git"},
	}
}

func TestInstallTimesOutOnAHangingSubprocess(t *testing.T) {
	f := newFixture(t)
	e := engine.NewWithCommandRunner(f.roots, &hangingRunner{})
	e.SetInstallTimeout(50 * time.Millisecond)

	start := time.Now()
	err := e.InstallLibraryEntry(gitEntry(), engine.InstallTarget{Kind: engine.InstallTargetPersonal}, engine.ActivationAuto)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("install with a hanging subprocess should fail")
	}
	if !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("error = %v, want a timeout message", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("install waited %s; the timeout was not applied", elapsed)
	}
}

func TestInstallContextCancelStopsTheSubprocess(t *testing.T) {
	f := newFixture(t)
	runner := &hangingRunner{started: make(chan struct{}, 1)}
	e := engine.NewWithCommandRunner(f.roots, runner)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- e.InstallLibraryEntryContext(ctx, gitEntry(), engine.InstallTarget{Kind: engine.InstallTargetPersonal}, engine.ActivationAuto)
	}()

	select {
	case <-runner.started:
	case <-time.After(5 * time.Second):
		t.Fatalf("the runner was never invoked")
	}
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("a cancelled install should return an error")
		}
		if !strings.Contains(err.Error(), "cancelled") {
			t.Fatalf("error = %v, want a cancellation message", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("cancelling the context did not stop the install")
	}
}

func TestRunProbeTimesOutIndependently(t *testing.T) {
	f := newFixture(t)
	e := engine.NewWithCommandRunner(f.roots, &hangingRunner{})
	e.SetProbeTimeout(50 * time.Millisecond)

	_, err := e.RunProbe(context.Background(), engine.Command{Name: "claude", Args: []string{"--version"}})
	if err == nil || !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("RunProbe error = %v, want a timeout message", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("RunProbe error should wrap context.DeadlineExceeded, got %v", err)
	}
}

func TestDefaultTimeoutsAreTheDocumentedValues(t *testing.T) {
	if engine.DefaultInstallTimeout != 120*time.Second {
		t.Fatalf("DefaultInstallTimeout = %s, want 120s", engine.DefaultInstallTimeout)
	}
	if engine.DefaultProbeTimeout != 30*time.Second {
		t.Fatalf("DefaultProbeTimeout = %s, want 30s", engine.DefaultProbeTimeout)
	}
}
