package main

// skillet manual-only / skillet auto — the Activation sweep.
//
// Skillet's headline job is getting a machine down to the bare minimum set of
// Skills that Auto-activate, and the friction is doing that one Skill at a
// time. So both commands take many names, and `manual-only` additionally
// takes `--all` with an `--except` list, making
//
//     skillet manual-only --all --except code-review,handoff --yes
//
// the one-liner that gets a machine to its minimum set.
//
// Two rules keep a sweep trustworthy. Nothing is written until every name has
// resolved: an ambiguous name aborts the whole command rather than
// half-applying it. And a preview always precedes the commit — without --yes
// the command prints exactly what it would change, and the estimated
// per-session saving, then refuses.
//
// Every token figure here is an estimate (engine.EstimateTokens) and says so.

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

func runManualOnly(args []string, stdout, stderr io.Writer) int {
	return runActivationSweep(true, args, stdout, stderr)
}

func runAuto(args []string, stdout, stderr io.Writer) int {
	return runActivationSweep(false, args, stdout, stderr)
}

// activationWording carries the direction-dependent copy, so the sweep itself
// is written once. The vocabulary is CONTEXT.md's: a Skill is set to
// Manual-only, or back to Auto-activation.
type activationWording struct {
	command string // subcommand name
	verb    string // what the summary line says was done
	already string // how "no change needed" reads
}

func wordingFor(manualOnly bool) activationWording {
	if manualOnly {
		return activationWording{
			command: "manual-only",
			verb:    "to Manual-only",
			already: "already Manual-only",
		}
	}
	return activationWording{
		command: "auto",
		verb:    "to Auto-activation",
		already: "already Auto-activating",
	}
}

func runActivationSweep(manualOnly bool, args []string, stdout, stderr io.Writer) int {
	wording := wordingFor(manualOnly)

	var usage, notes string
	if manualOnly {
		usage = "usage: skillet manual-only <name>... --yes\n       skillet manual-only --all [--except NAME[,NAME...]] --yes"
		notes = "Turns Auto-activation off, so a Skill runs only when it is invoked\n" +
			"explicitly. Takes any number of Skill names, or --all to sweep every\n" +
			"Skill that currently Auto-activates — keeping the ones named in --except.\n" +
			"Without --yes it prints exactly what it would change, and the estimated\n" +
			"per-session saving, and changes nothing. Reverse it with \"skillet auto\"."
	} else {
		usage = "usage: skillet auto <name>... --yes"
		notes = "Turns Auto-activation back on, so the model may invoke a Skill on its own\n" +
			"judgement. Takes any number of Skill names. Without --yes it prints what it\n" +
			"would change, and the estimated per-session cost that would add."
	}

	cmd := newCommand("skillet "+wording.command, usage, notes)
	confirmed := cmd.yesFlag()
	var all *bool
	var except *string
	if manualOnly {
		all = cmd.flags.Bool("all", false, "sweep every Skill that currently Auto-activates")
		except = cmd.flags.String("except", "", "with --all: comma-separated Skill names to leave Auto-activating")
	} else {
		all = new(bool)
		except = new(string)
	}
	if code, done := cmd.parse(args, stdout, stderr); done {
		return code
	}

	exceptNames := splitNameList(*except)
	switch {
	case *all && cmd.flags.NArg() > 0:
		return fail(stderr, usagef("--all sweeps every Auto-activating Skill, so it cannot be combined with Skill names (got %q)", cmd.flags.Arg(0)))
	case !*all && cmd.flags.NArg() == 0:
		if manualOnly {
			return fail(stderr, usagef("manual-only takes at least one Skill name, or --all"))
		}
		return fail(stderr, usagef("auto takes at least one Skill name"))
	case !*all && len(exceptNames) > 0:
		return fail(stderr, usagef("--except only applies to --all: name the Skills to change instead"))
	}

	e, err := newEngineForCWD()
	if err != nil {
		return fail(stderr, err)
	}
	inventory := e.Inventory()

	// Resolution comes first and in full: every name, and every --except name,
	// must resolve before anything is written, so an ambiguous name aborts the
	// whole command rather than half-applying it.
	var targets []engine.Skill
	if *all {
		targets, err = sweepTargets(inventory, exceptNames)
	} else {
		targets, err = resolveSkillList(inventory, cmd.flags.Args())
	}
	if err != nil {
		return fail(stderr, err)
	}
	if len(targets) == 0 {
		fmt.Fprintf(stdout, "Nothing to change: no Skill is currently Auto-activating (outside --except).\n")
		return 0
	}

	if !*confirmed {
		printActivationPreview(stdout, targets, manualOnly, wording)
		return fail(stderr, requireYes(false, wording.command))
	}

	results := e.SetManualOnlyBulk(targets, manualOnly)
	return reportActivationResults(stdout, stderr, results, manualOnly, wording)
}

// splitNameList parses a comma-separated flag value, ignoring empty entries so
// a trailing comma is not an error.
func splitNameList(value string) []string {
	var names []string
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	return names
}

// resolveSkillList resolves every argument before returning, joining every
// failure into one error so a caller fixing several bad names sees them all at
// once — and so no write has happened by the time they do. Duplicates are
// collapsed: naming a Skill twice changes it once.
func resolveSkillList(inventory engine.Inventory, args []string) ([]engine.Skill, error) {
	var (
		skills   []engine.Skill
		problems []string
		seen     = map[string]bool{}
	)
	for _, arg := range args {
		skill, err := resolveSkill(inventory, arg)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		if seen[skill.Location] {
			continue
		}
		seen[skill.Location] = true
		skills = append(skills, skill)
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("%s\nNothing was changed.", strings.Join(problems, "\n"))
	}
	return skills, nil
}

// sweepTargets is --all: every Skill that currently Auto-activates and whose
// Activation Skillet can actually change, minus the --except list, ordered
// most expensive first so the biggest savings read at the top.
func sweepTargets(inventory engine.Inventory, exceptNames []string) ([]engine.Skill, error) {
	kept, err := resolveSkillList(inventory, exceptNames)
	if err != nil {
		return nil, err
	}
	keptLocations := make(map[string]bool, len(kept))
	for _, skill := range kept {
		keptLocations[skill.Location] = true
	}

	var targets []engine.Skill
	for _, skill := range engine.SortByDescriptionCost(inventory.Skills) {
		if skill.Activation != engine.ActivationAuto || keptLocations[skill.Location] {
			continue
		}
		if engine.ActivationChangeSupported(skill) != nil {
			// Plugin Skills and Codex prompts are not part of a sweep: naming
			// one explicitly still reports why, but --all does not manufacture
			// failures out of Skills it was never able to change.
			continue
		}
		targets = append(targets, skill)
	}
	return targets, nil
}

// describeSkill is the one-phrase form used in every line here, in CONTEXT.md's
// vocabulary: Source, Kind, name.
func describeSkill(skill engine.Skill) string {
	return fmt.Sprintf("%s %s %q", skill.Source, skill.Kind, skill.Name)
}

// printActivationPreview is the no---yes half: exactly what would change, and
// what it is estimated to be worth per session.
func printActivationPreview(w io.Writer, targets []engine.Skill, manualOnly bool, wording activationWording) {
	var changing, unchanged []engine.Skill
	for _, skill := range targets {
		if engine.ActivationChangeSupported(skill) != nil || engine.ActivationAlreadySet(skill, manualOnly) {
			unchanged = append(unchanged, skill)
			continue
		}
		changing = append(changing, skill)
	}

	if len(changing) == 0 {
		fmt.Fprintf(w, "Nothing would change: no named Skill is eligible to be set %s.\n", wording.verb)
	} else {
		fmt.Fprintf(w, "Would set %s %s:\n", countedSkills(len(changing)), wording.verb)
		table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(table, "  SOURCE\tTOOL\tNAME\tPER SESSION (EST.)")
		for _, skill := range changing {
			fmt.Fprintf(table, "  %s\t%s\t%s\t%s tokens\n",
				skill.Source, skill.Tool, skill.Name, engine.FormatTokenEstimate(skill.DescriptionTokens))
		}
		_ = table.Flush()
	}
	for _, skill := range unchanged {
		if err := engine.ActivationChangeSupported(skill); err != nil {
			fmt.Fprintf(w, "  (skipped) %s\n", err)
			continue
		}
		fmt.Fprintf(w, "  (skipped) %s is %s\n", describeSkill(skill), wording.already)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, activationEffectLine(changing, manualOnly))
	fmt.Fprintln(w, "Nothing has been changed yet.")
}

// reportActivationResults prints the per-Skill outcome and the summary every
// bulk command owes its caller: changed, already in that state, failed (with
// the reason), and the estimated per-session effect. It returns the exit code:
// 1 if any Skill failed, even when others succeeded.
func reportActivationResults(stdout, stderr io.Writer, results []engine.BulkResult, manualOnly bool, wording activationWording) int {
	var changed, already []engine.Skill
	var failures []engine.BulkResult
	for _, result := range results {
		switch {
		case result.Err != nil:
			failures = append(failures, result)
		case engine.ActivationAlreadySet(result.Skill, manualOnly):
			already = append(already, result.Skill)
		default:
			changed = append(changed, result.Skill)
		}
	}

	for _, skill := range changed {
		fmt.Fprintf(stdout, "Set %s %s at %s\n", describeSkill(skill), wording.verb, skill.Location)
	}
	for _, skill := range already {
		fmt.Fprintf(stdout, "%s was %s — nothing to change\n", describeSkill(skill), wording.already)
	}
	for _, failure := range failures {
		fmt.Fprintf(stderr, "Could not change %s: %s\n", describeSkill(failure.Skill), failureReason(failure))
	}

	fmt.Fprintf(stdout, "\n%s changed, %d %s, %d could not be changed.\n",
		countedSkills(len(changed)), len(already), wording.already, len(failures))
	fmt.Fprintln(stdout, activationEffectLine(changed, manualOnly))
	if len(failures) > 0 {
		fmt.Fprintln(stdout, "Every Skill that could be changed was changed; the reasons for the rest are above.")
		return 1
	}
	return 0
}

// failureReason strips the Skill description the engine's own error carries,
// so a line that already names the Skill does not name it twice. An error from
// deeper down (a failed write) has no such prefix and is printed whole.
func failureReason(failure engine.BulkResult) string {
	return strings.TrimPrefix(failure.Err.Error(), describeSkill(failure.Skill)+": ")
}

// activationEffectLine states what a set of changes is worth per session, in
// the direction the command runs: Manual-only saves, Auto adds.
func activationEffectLine(skills []engine.Skill, manualOnly bool) string {
	if manualOnly {
		savings := engine.EstimateManualOnlySavings(skills)
		return fmt.Sprintf("Estimated saving: %s tokens per session, across %s (estimate — Skillet sizes files rather than running a tokenizer).",
			engine.FormatTokenEstimate(savings.Tokens), countedSkills(savings.Skills))
	}
	added := estimateAutoAddedCost(skills)
	return fmt.Sprintf("Estimated added cost: %s tokens per session, across %s (estimate — Skillet sizes files rather than running a tokenizer).",
		engine.FormatTokenEstimate(added.Tokens), countedSkills(added.Skills))
}

// estimateAutoAddedCost is EstimateManualOnlySavings' mirror: what restoring
// Auto-activation on these Skills would start costing every session. Only
// Manual-only Skills contribute — a Suppressed or Disabled Skill stays hidden
// from the model whatever this toggle says, so it would not start costing
// anything.
func estimateAutoAddedCost(skills []engine.Skill) engine.ContextSavings {
	var added engine.ContextSavings
	for _, skill := range skills {
		if skill.Activation != engine.ActivationManualOnly {
			continue
		}
		if engine.ActivationChangeSupported(skill) != nil {
			continue
		}
		added.Skills++
		added.Tokens += skill.DescriptionTokens
	}
	return added
}

func countedSkills(count int) string {
	if count == 1 {
		return "1 Skill"
	}
	return fmt.Sprintf("%d Skills", count)
}
