package engine

// The one property of SetManualOnlyBulk that is invisible from outside the
// package: the whole sweep runs under a single lock acquisition. The advisory
// lock (lock.go) is not reentrant, so a bulk entry point that looped over the
// public SetManualOnly would either deadlock or — with the in-process mutex
// re-entered per call — turn one transaction into N, which is exactly the cost
// the bulk action exists to remove.
//
// Like safety_internal_test.go and write_fault_test.go, these tests mutate
// package-global state (engineLockAcquired) and must never run in parallel.

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// countLockAcquisitions installs the lock observer for one test and returns a
// function reporting how many times the lock has been taken since.
func countLockAcquisitions(t *testing.T) func() int {
	t.Helper()
	count := 0
	engineLockAcquired = func() { count++ }
	t.Cleanup(func() { engineLockAcquired = nil })
	return func() int { return count }
}

// writePersonalSkill creates a Personal (Claude Code, user-level) Skill that
// Auto-activates.
func (f faultFixture) writePersonalSkill(t *testing.T, name string) {
	t.Helper()
	folder := filepath.Join(f.roots.ClaudeHome, "skills", name)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	content := fmt.Sprintf("---\nname: %q\ndescription: \"Description for %s\"\n---\nBody\n", name, name)
	if err := os.WriteFile(filepath.Join(folder, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

func (f faultFixture) personalSkill(t *testing.T, name string) Skill {
	t.Helper()
	for _, skill := range f.e.Inventory().Skills {
		if skill.Source == SourcePersonal && skill.Name == name {
			return skill
		}
	}
	t.Fatalf("Personal Skill %q not found", name)
	return Skill{}
}

func TestSetManualOnlyBulkTakesTheMutationLockExactlyOnce(t *testing.T) {
	f := newFaultFixture(t)
	names := []string{"one", "two", "three", "four"}
	for _, name := range names {
		f.writePersonalSkill(t, name)
	}
	skills := make([]Skill, 0, len(names))
	for _, name := range names {
		skills = append(skills, f.personalSkill(t, name))
	}

	acquisitions := countLockAcquisitions(t)
	results := f.e.SetManualOnlyBulk(skills, true)
	for index, result := range results {
		if result.Err != nil {
			t.Fatalf("result %d failed: %v", index, result.Err)
		}
	}
	if got := acquisitions(); got != 1 {
		t.Fatalf("a %d-Skill bulk change took the lock %d times, want exactly 1", len(skills), got)
	}

	// The comparison that gives that number meaning: the single-Skill method
	// still takes it once per call, so the saving is real and not an artefact
	// of the observer.
	before := acquisitions()
	for _, skill := range skills {
		if err := f.e.SetManualOnly(skill, false); err != nil {
			t.Fatalf("SetManualOnly: %v", err)
		}
	}
	if got := acquisitions() - before; got != len(skills) {
		t.Fatalf("%d single-Skill calls took the lock %d times, want %d", len(skills), got, len(skills))
	}
}

// A sweep containing Skills the engine refuses still holds the lock once: the
// refusals are decided inside the locked section and must not release and
// retake it.
func TestSetManualOnlyBulkTakesTheLockOnceEvenWithRejectedSkills(t *testing.T) {
	f := newFaultFixture(t)
	f.writePersonalSkill(t, "changeable")
	f.writeCodexSkillWithoutConfig(t, "codex-one")
	skills := []Skill{
		f.personalSkill(t, "changeable"),
		{Name: "deploy", Source: SourceCodex, Kind: KindPrompt, Tool: ToolCodex, Activation: ActivationManualOnly},
		{Name: "lint", Source: SourcePlugin, Kind: KindSkill, Tool: ToolClaudeCode, Activation: ActivationAuto},
		f.codexSkill(t, "codex-one"),
	}

	acquisitions := countLockAcquisitions(t)
	results := f.e.SetManualOnlyBulk(skills, true)
	if got := acquisitions(); got != 1 {
		t.Fatalf("lock taken %d times, want exactly 1", got)
	}
	if results[0].Err != nil || results[3].Err != nil {
		t.Fatalf("changeable Skills failed: %v / %v", results[0].Err, results[3].Err)
	}
	if results[1].Err == nil || results[2].Err == nil {
		t.Fatalf("a Codex prompt and a Plugin Skill must both report why they were not changed: %+v", results)
	}
}

// An empty sweep is a no-op and must not create a lock file at all.
func TestSetManualOnlyBulkOnAnEmptyListTakesNoLock(t *testing.T) {
	f := newFaultFixture(t)
	acquisitions := countLockAcquisitions(t)
	if results := f.e.SetManualOnlyBulk(nil, true); len(results) != 0 {
		t.Fatalf("results = %+v, want none", results)
	}
	if got := acquisitions(); got != 0 {
		t.Fatalf("an empty bulk change took the lock %d times, want 0", got)
	}
}
