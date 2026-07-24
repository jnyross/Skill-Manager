package main

// Tests for the Activation sweep (manual-only / auto). Like the rest of the
// package's tests these run against newFixture's throwaway HOME and project
// directory, so nothing here can read or write the real ~/.claude, ~/.codex,
// ~/.agents, or ~/.skillet.

import (
	"path/filepath"
	"strings"
	"testing"
)

func personalSkillMD(f fixture, name string) string {
	return filepath.Join(f.home, ".claude", "skills", name, "SKILL.md")
}

func isManualOnly(t *testing.T, path string) bool {
	t.Helper()
	return strings.Contains(readFile(t, path), "disable-model-invocation: true")
}

func TestManualOnlyAppliesEveryNamedSkillInOneCommand(t *testing.T) {
	f := newFixture(t)
	code, stdout, stderr := runCLI(t, "manual-only", "writing", "refactor", "--yes")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !isManualOnly(t, personalSkillMD(f, "writing")) {
		t.Fatalf("Personal Skill \"writing\" was not set Manual-only")
	}
	codexPolicy := filepath.Join(f.home, ".agents", "skills", "refactor", "agents", "openai.yaml")
	if content := readFile(t, codexPolicy); !strings.Contains(content, "allow_implicit_invocation: false") {
		t.Fatalf("Codex Skill \"refactor\" policy = %q", content)
	}
	for _, want := range []string{"2 Skills changed", "Estimated saving:", "tokens per session"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

// A Skill already in the requested state is reported as such, not counted as a
// change, and not rewritten.
func TestManualOnlyReportsSkillsAlreadyInThatState(t *testing.T) {
	f := newFixture(t)
	before := readFile(t, personalSkillMD(f, "review"))

	code, stdout, stderr := runCLI(t, "manual-only", "Personal:review", "--yes")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "already Manual-only") {
		t.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(stdout, "0 Skills changed") {
		t.Fatalf("an already-Manual-only Skill must not count as a change:\n%s", stdout)
	}
	if readFile(t, personalSkillMD(f, "review")) != before {
		t.Fatalf("an already-Manual-only Skill was rewritten")
	}
}

// The load-bearing safety property: one ambiguous name aborts the whole
// command before anything is written, rather than half-applying it.
func TestManualOnlyAbortsOnAnAmbiguousNameBeforeWritingAnything(t *testing.T) {
	f := newFixture(t)
	before := readFile(t, personalSkillMD(f, "writing"))

	// "review" is Personal, Project/Claude Code, and Project/Codex here.
	code, _, stderr := runCLI(t, "manual-only", "writing", "review", "--yes")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "matches 3 Skills") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Nothing was changed.") {
		t.Fatalf("stderr must say nothing was applied:\n%s", stderr)
	}
	if readFile(t, personalSkillMD(f, "writing")) != before {
		t.Fatalf("an unambiguous Skill named alongside an ambiguous one was written anyway")
	}
}

func TestManualOnlyExitsOneWhenSomeSkillsCannotChangeButStillAppliesTheRest(t *testing.T) {
	f := newFixture(t)
	// "lint" is a Plugin Skill: its per-Skill control is Suppress, not
	// Manual-only. "deploy" is a Codex prompt: no Auto-activation to turn off.
	code, stdout, stderr := runCLI(t, "manual-only", "writing", "lint", "deploy", "--yes")
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (some Skills failed)", code)
	}
	if !isManualOnly(t, personalSkillMD(f, "writing")) {
		t.Fatalf("the changeable Skill must still have been changed")
	}
	if !strings.Contains(stderr, "Suppress") || !strings.Contains(stderr, "Codex prompt") {
		t.Fatalf("stderr must give a reason per failure:\n%s", stderr)
	}
	for _, want := range []string{"1 Skill changed", "2 could not be changed", "Every Skill that could be changed was changed"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestManualOnlyAllWithoutYesPreviewsAndChangesNothing(t *testing.T) {
	f := newFixture(t)
	before := readFile(t, personalSkillMD(f, "writing"))

	code, stdout, stderr := runCLI(t, "manual-only", "--all")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (refused without --yes)", code)
	}
	if !strings.Contains(stderr, "--yes") {
		t.Fatalf("stderr = %q", stderr)
	}
	for _, want := range []string{"Would set", "writing", "Estimated saving:", "Nothing has been changed yet."} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("preview missing %q:\n%s", want, stdout)
		}
	}
	if readFile(t, personalSkillMD(f, "writing")) != before {
		t.Fatalf("a refused --all wrote to disk")
	}
	// The preview must not offer a Plugin Skill or a Codex prompt: --all only
	// sweeps what it can actually change.
	if strings.Contains(stdout, "lint") || strings.Contains(stdout, "deploy") {
		t.Fatalf("--all previewed a Skill it cannot change:\n%s", stdout)
	}
}

func TestManualOnlyAllExceptSweepsEverythingElse(t *testing.T) {
	f := newFixture(t)
	code, stdout, stderr := runCLI(t, "manual-only", "--all", "--except", "writing,Project:claude-code:review", "--yes")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if isManualOnly(t, personalSkillMD(f, "writing")) {
		t.Fatalf("--except must have kept \"writing\" Auto-activating")
	}
	keptProject := filepath.Join(f.project, ".claude", "skills", "review", "SKILL.md")
	if isManualOnly(t, keptProject) {
		t.Fatalf("--except must have kept the Project Claude Code Skill Auto-activating")
	}
	// Everything else that Auto-activates was swept.
	swept := filepath.Join(f.home, ".agents", "skills", "refactor", "agents", "openai.yaml")
	if content := readFile(t, swept); !strings.Contains(content, "allow_implicit_invocation: false") {
		t.Fatalf("the Codex Skill was not swept: %q", content)
	}
	if !strings.Contains(stdout, "Estimated saving:") {
		t.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(stdout, "0 could not be changed") {
		t.Fatalf("--all must not manufacture failures:\n%s", stdout)
	}

	// The real point of the sweep: the standing per-session cost drops.
	code, costOut, _ := runCLI(t, "cost")
	if code != 0 {
		t.Fatalf("cost exit = %d", code)
	}
	if !strings.Contains(costOut, "Skills are excluded") {
		t.Fatalf("cost report does not show the exclusions:\n%s", costOut)
	}
}

// An --except name that is itself ambiguous must abort the sweep before any
// write, exactly as an ambiguous target does.
func TestManualOnlyAllRejectsAnAmbiguousExceptNameBeforeWriting(t *testing.T) {
	f := newFixture(t)
	before := readFile(t, personalSkillMD(f, "writing"))

	code, _, stderr := runCLI(t, "manual-only", "--all", "--except", "review", "--yes")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "matches 3 Skills") {
		t.Fatalf("stderr = %q", stderr)
	}
	if readFile(t, personalSkillMD(f, "writing")) != before {
		t.Fatalf("an ambiguous --except name must abort before any write")
	}
}

func TestManualOnlyRejectsAllCombinedWithNames(t *testing.T) {
	newFixture(t)
	code, _, stderr := runCLI(t, "manual-only", "--all", "writing", "--yes")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "cannot be combined") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestManualOnlyRejectsExceptWithoutAll(t *testing.T) {
	newFixture(t)
	code, _, stderr := runCLI(t, "manual-only", "writing", "--except", "review", "--yes")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--except only applies to --all") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestActivationCommandsRequireAName(t *testing.T) {
	newFixture(t)
	for _, command := range []string{"manual-only", "auto"} {
		code, _, stderr := runCLI(t, command, "--yes")
		if code != 2 {
			t.Fatalf("%s with no name exit = %d, want 2", command, code)
		}
		if !strings.Contains(stderr, "at least one Skill name") {
			t.Fatalf("%s stderr = %q", command, stderr)
		}
	}
	// --all belongs to manual-only only: `auto --all` is an unknown flag.
	code, _, stderr := runCLI(t, "auto", "--all", "--yes")
	if code != 2 {
		t.Fatalf("auto --all exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "flag provided but not defined") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestAutoRestoresSeveralSkillsAndReportsTheAddedCost(t *testing.T) {
	f := newFixture(t)
	if code, _, stderr := runCLI(t, "manual-only", "writing", "refactor", "--yes"); code != 0 {
		t.Fatalf("setup exit = %d, stderr = %q", code, stderr)
	}

	code, stdout, stderr := runCLI(t, "auto", "writing", "refactor", "--yes")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if isManualOnly(t, personalSkillMD(f, "writing")) {
		t.Fatalf("auto did not restore Auto-activation")
	}
	for _, want := range []string{"2 Skills changed", "Estimated added cost:"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

// Naming the same Skill twice changes it once and is not an error.
func TestActivationSweepCollapsesDuplicateNames(t *testing.T) {
	newFixture(t)
	code, stdout, stderr := runCLI(t, "manual-only", "writing", "Personal:writing", "--yes")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "1 Skill changed") {
		t.Fatalf("stdout = %q", stdout)
	}
}
