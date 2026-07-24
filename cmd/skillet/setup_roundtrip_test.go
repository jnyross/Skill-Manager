package main

import (
	"bytes"
	"io"
	"strings"
	"testing"

	workspaceSetup "github.com/jnyross/Skill-Manager/internal/setup"
)

// stubTUISession replaces the real Bubble Tea program at the main.go seam. It
// records the status line each session opened with and replays a scripted
// answer to "did the user press S?".
type stubTUISession struct {
	requests     []bool
	statuses     []string
	statusErrors []bool
	calls        int
}

func (s *stubTUISession) run(_ io.Writer, status string, isError bool) (bool, error) {
	s.statuses = append(s.statuses, status)
	s.statusErrors = append(s.statusErrors, isError)
	index := s.calls
	s.calls++
	if index >= len(s.requests) {
		return false, nil
	}
	return s.requests[index], nil
}

func withStubbedInteractiveSeam(t *testing.T, session *stubTUISession, wizard func(io.Reader, io.Writer) (workspaceSetup.Result, error)) {
	t.Helper()
	oldSession, oldWizard := runTUISession, runSetupWizard
	runTUISession = session.run
	runSetupWizard = wizard
	t.Cleanup(func() { runTUISession, runSetupWizard = oldSession, oldWizard })
}

// Pressing S must run the line-oriented wizard and then bring the user back to
// the TUI with the Setup outcome on the status line — not drop them at the
// shell.
func TestSetupRoundTripRelaunchesTUIWithOutcome(t *testing.T) {
	session := &stubTUISession{requests: []bool{true, false}}
	wizardRuns := 0
	withStubbedInteractiveSeam(t, session, func(_ io.Reader, stdout io.Writer) (workspaceSetup.Result, error) {
		wizardRuns++
		// The wizard is still line-oriented on the raw terminal.
		io.WriteString(stdout, "Outcome: Verified\n")
		return workspaceSetup.Result{Outcome: workspaceSetup.OutcomeVerified}, nil
	})

	var stdout, stderr bytes.Buffer
	if code := runWithInput(nil, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	if wizardRuns != 1 {
		t.Fatalf("wizard ran %d time(s), want 1", wizardRuns)
	}
	if session.calls != 2 {
		t.Fatalf("TUI ran %d time(s), want 2 (before and after Setup)", session.calls)
	}
	if session.statuses[0] != "" {
		t.Fatalf("first session opened with a status: %q", session.statuses[0])
	}
	if !strings.Contains(session.statuses[1], "Setup outcome: Verified") {
		t.Fatalf("relaunched TUI status = %q", session.statuses[1])
	}
	if session.statusErrors[1] {
		t.Fatal("a Verified outcome was reported as an error")
	}
	if !strings.Contains(stdout.String(), "Outcome: Verified") {
		t.Fatalf("wizard output did not reach stdout: %q", stdout.String())
	}
}

// A Blocked outcome also returns to the TUI, carrying the blocker as an error
// status rather than exiting to the shell with a bare message.
func TestSetupRoundTripReturnsBlockedOutcomeAsErrorStatus(t *testing.T) {
	session := &stubTUISession{requests: []bool{true, false}}
	withStubbedInteractiveSeam(t, session, func(io.Reader, io.Writer) (workspaceSetup.Result, error) {
		return workspaceSetup.Result{Outcome: workspaceSetup.OutcomeBlocked, NextAction: "Authorize a recoverable backup"}, nil
	})

	var stdout, stderr bytes.Buffer
	if code := runWithInput(nil, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(session.statuses[1], "Setup outcome: Blocked") ||
		!strings.Contains(session.statuses[1], "Authorize a recoverable backup") {
		t.Fatalf("relaunched TUI status = %q", session.statuses[1])
	}
	if !session.statusErrors[1] {
		t.Fatal("a Blocked outcome was not reported as an error")
	}
}

// Pressing S repeatedly keeps round-tripping; the loop only ends when the user
// quits the TUI without asking for Setup.
func TestSetupRoundTripRepeatsUntilTheUserQuits(t *testing.T) {
	session := &stubTUISession{requests: []bool{true, true, false}}
	withStubbedInteractiveSeam(t, session, func(io.Reader, io.Writer) (workspaceSetup.Result, error) {
		return workspaceSetup.Result{Outcome: workspaceSetup.OutcomeCanceled}, nil
	})

	var stdout, stderr bytes.Buffer
	if code := runWithInput(nil, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	if session.calls != 3 {
		t.Fatalf("TUI ran %d time(s), want 3", session.calls)
	}
	if !strings.Contains(session.statuses[2], "Setup canceled") {
		t.Fatalf("status after cancellation = %q", session.statuses[2])
	}
	if session.statusErrors[2] {
		t.Fatal("a cancellation was reported as an error")
	}
}

func TestSetupStatusLineUsesTheContextOutcomeVocabulary(t *testing.T) {
	tests := []struct {
		result  workspaceSetup.Result
		want    string
		isError bool
	}{
		{workspaceSetup.Result{Outcome: workspaceSetup.OutcomeVerified}, "Setup outcome: Verified.", false},
		{workspaceSetup.Result{Outcome: workspaceSetup.OutcomeConfiguredUnverified}, "Setup outcome: Configured-unverified.", false},
		{workspaceSetup.Result{Outcome: workspaceSetup.OutcomePartial, NextAction: "Undo the Tool change"}, "Setup outcome: Partial. Next: Undo the Tool change", true},
		{workspaceSetup.Result{Outcome: workspaceSetup.OutcomeBlocked}, "Setup outcome: Blocked.", true},
		{workspaceSetup.Result{Outcome: workspaceSetup.OutcomeCanceled}, "Setup canceled — nothing was changed.", false},
		{workspaceSetup.Result{}, "Setup returned no outcome.", true},
	}
	for _, test := range tests {
		line, isError := setupStatusLine(test.result)
		if line != test.want || isError != test.isError {
			t.Errorf("setupStatusLine(%q) = (%q, %t), want (%q, %t)", test.result.Outcome, line, isError, test.want, test.isError)
		}
	}
}

// A wizard that fails outright still reports to stderr and exits non-zero
// rather than looping.
func TestSetupRoundTripReportsWizardFailure(t *testing.T) {
	session := &stubTUISession{requests: []bool{true}}
	withStubbedInteractiveSeam(t, session, func(io.Reader, io.Writer) (workspaceSetup.Result, error) {
		return workspaceSetup.Result{}, io.ErrUnexpectedEOF
	})

	var stdout, stderr bytes.Buffer
	if code := runWithInput(nil, strings.NewReader(""), &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), io.ErrUnexpectedEOF.Error()) {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if session.calls != 1 {
		t.Fatalf("TUI ran %d time(s), want 1", session.calls)
	}
}
