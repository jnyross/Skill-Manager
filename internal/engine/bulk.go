package engine

// Bulk Activation changes.
//
// Skillet's headline job is getting a machine down to the bare minimum set of
// Skills that Auto-activate. Doing that one Skill at a time is the friction:
// on a real machine 44 Skills Auto-activate, and 44 separate confirmations —
// each taking and releasing the cross-process mutation lock, each re-scanning
// afterwards — is the reason people never finish the sweep.
//
// SetManualOnlyBulk is therefore a single transaction: one withEngineLock
// acquisition (lock.go's lock is NOT reentrant, so this calls the *Locked
// inner function per Skill rather than the public SetManualOnly), and one
// pass that continues past individual failures so a single unwritable Skill
// cannot block the rest of the sweep.
//
// EstimateManualOnlySavings is the preview half: what the pending change
// would remove from every session, so the user sees the number before
// committing rather than after.

import "fmt"

// BulkResult reports one Skill's outcome in a bulk Activation change.
type BulkResult struct {
	Skill Skill
	Err   error // nil when the change applied
}

// SetManualOnlyBulk sets Manual-only (or restores Auto) on every listed
// Skill under a single lock acquisition, continuing past individual
// failures so one unwritable Skill cannot block the rest.
//
// Results come back in the same order as skills, one per input Skill.
//
// A Skill already in the requested state is reported with a nil Err and is
// not written at all. A Skill whose Activation Skillet cannot change —
// a Codex prompt, which has no Auto-activation to turn off, or a Plugin
// Skill, whose per-Skill control is Suppress — gets a descriptive Err rather
// than being silently skipped, so a caller's counts always add up.
//
// If the lock itself cannot be taken, every result carries that error: the
// whole sweep failed, and nothing was written.
func (e *Engine) SetManualOnlyBulk(skills []Skill, manualOnly bool) []BulkResult {
	results := make([]BulkResult, len(skills))
	for index, skill := range skills {
		results[index] = BulkResult{Skill: skill}
	}
	if len(skills) == 0 {
		return results
	}

	// fn never returns an error: per-Skill failures are recorded in results so
	// the sweep continues. Only lock acquisition can fail the whole call.
	lockErr := withEngineLock(e.roots.DataDir, func() error {
		for index, skill := range skills {
			if err := ActivationChangeSupported(skill); err != nil {
				results[index].Err = err
				continue
			}
			if ActivationAlreadySet(skill, manualOnly) {
				continue
			}
			results[index].Err = e.setManualOnlyLocked(skill, manualOnly)
		}
		return nil
	})
	if lockErr != nil {
		for index := range results {
			results[index].Err = lockErr
		}
	}
	return results
}

// ActivationChangeSupported reports whether Skillet can turn skill's
// Auto-activation on or off at all, returning a descriptive error when it
// cannot. It is the same rule setManualOnlyLocked enforces, exposed so a
// caller can build an accurate preview — and an accurate savings estimate —
// before writing anything.
func ActivationChangeSupported(skill Skill) error {
	switch {
	case skill.Kind == KindPrompt:
		return fmt.Errorf("%s %s %q: a Codex prompt has no Auto-activation to change — it already runs only when it is invoked explicitly",
			skill.Source, skill.Kind, skill.Name)
	case skill.Source == SourcePlugin:
		return fmt.Errorf("%s %s %q: a Plugin Skill's Activation cannot be changed in place — Suppress it instead, which leaves its plugin installed and intact",
			skill.Source, skill.Kind, skill.Name)
	case skill.Source == SourcePersonal, skill.Source == SourceCodex, skill.Source == SourceProject:
		return nil
	default:
		return fmt.Errorf("%s %s %q: Activation cannot be changed for this Source", skill.Source, skill.Kind, skill.Name)
	}
}

// ActivationAlreadySet reports whether skill is already in the state a
// SetManualOnly(skill, manualOnly) call would put it in, so callers can skip
// the write and report it as "already Manual-only" rather than as a change.
//
// Only the two states the toggle produces count. A Suppressed or Disabled
// Skill is neither Auto nor Manual-only, so it is not "already set": applying
// the change to it does real work.
func ActivationAlreadySet(skill Skill, manualOnly bool) bool {
	if manualOnly {
		return skill.Activation == ActivationManualOnly
	}
	return skill.Activation == ActivationAuto
}

// ContextSavings is the per-session cost a pending Activation change would
// remove. Tokens is an estimate, like every figure Skillet reports.
type ContextSavings struct {
	Skills int // how many Skills would actually change
	Tokens int // estimated per-session tokens saved
}

// EstimateManualOnlySavings reports what setting the given Skills to
// Manual-only would save per session. Skills that do not currently
// Auto-activate contribute nothing, because they already cost nothing.
//
// Skills whose Activation Skillet cannot change (Codex prompts, Plugin
// Skills — see ActivationChangeSupported) contribute nothing either: they
// would not actually change, and a preview that counted them would promise a
// saving the sweep cannot deliver.
func EstimateManualOnlySavings(skills []Skill) ContextSavings {
	var savings ContextSavings
	for _, skill := range skills {
		if skill.Activation != ActivationAuto {
			continue
		}
		if ActivationChangeSupported(skill) != nil {
			continue
		}
		savings.Skills++
		savings.Tokens += skill.DescriptionTokens
	}
	return savings
}
