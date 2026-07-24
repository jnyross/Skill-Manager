package main

// Inventory and per-Skill commands: list, show, archive, restore, purge,
// suppress, unsuppress, manual-only, auto.

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

func runList(args []string, stdout, stderr io.Writer) int {
	cmd := newCommand("skillet list", "usage: skillet list [--json] [--source SOURCE] [--tool TOOL]",
		"Lists every Skill Skillet can see, the notices raised while scanning, and the\nArchive. Sources: Personal, Plugin, Codex, Project. Tools: claude-code, codex.")
	asJSON := cmd.flags.Bool("json", false, "emit the inventory as JSON")
	sourceFilter := cmd.flags.String("source", "", "only Skills from this Source")
	toolFilter := cmd.flags.String("tool", "", "only Skills governed by this Tool")
	if code, done := cmd.parse(args, stdout, stderr); done {
		return code
	}
	if cmd.flags.NArg() != 0 {
		return fail(stderr, usagef("list takes no arguments (got %q)", cmd.flags.Arg(0)))
	}

	var source engine.Source
	var tool engine.Tool
	var err error
	if *sourceFilter != "" {
		if source, err = parseSourceToken(*sourceFilter); err != nil {
			return fail(stderr, err)
		}
	}
	if *toolFilter != "" {
		if tool, err = parseToolToken(*toolFilter); err != nil {
			return fail(stderr, err)
		}
	}

	e, err := newEngineForCWD()
	if err != nil {
		return fail(stderr, err)
	}
	inventory := e.Inventory()
	skills := make([]engine.Skill, 0, len(inventory.Skills))
	for _, skill := range inventory.Skills {
		if source != "" && skill.Source != source {
			continue
		}
		if tool != "" && skill.Tool != tool {
			continue
		}
		skills = append(skills, skill)
	}

	// list reports every Skill, so it measures every Skill's footprint. That
	// walk is why Inventory() does not do it (engine.MeasureSkillFiles): a
	// one-shot command can afford it, a TUI refresh cannot.
	notices := append([]engine.Notice{}, inventory.Notices...)
	notices = append(notices, engine.MeasureAllSkillFiles(skills)...)
	archived, archiveNotices, archiveErr := e.ListArchive()
	if archiveErr != nil {
		notices = append(notices, engine.Notice{Message: "Archive unreadable: " + archiveErr.Error()})
	}
	notices = append(notices, archiveNotices...)

	if *asJSON {
		document := listJSON{
			SchemaVersion: jsonSchemaVersion,
			Skills:        newSkillsJSON(skills),
			Notices:       newNoticesJSON(notices),
			Archive:       newArchiveJSON(archived),
		}
		if err := writeJSON(stdout, document); err != nil {
			return fail(stderr, err)
		}
		return 0
	}

	printSkillTable(stdout, skills)
	printArchiveTable(stdout, archived)
	printNotices(stdout, notices)
	return 0
}

func printSkillTable(w io.Writer, skills []engine.Skill) {
	if len(skills) == 0 {
		fmt.Fprintln(w, "No Skills found.")
		return
	}
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "SOURCE\tTOOL\tKIND\tACTIVATION\tNAME\tDESCRIPTION")
	for _, skill := range skills {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\n",
			skill.Source, skill.Tool, skill.Kind, skill.Activation, skill.Name, truncate(oneLine(skill.Description), 60))
	}
	_ = table.Flush()
}

func printArchiveTable(w io.Writer, entries []engine.ArchiveEntry) {
	if len(entries) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Archive:")
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tNAME\tSOURCE\tTOOL\tARCHIVED")
	for _, entry := range entries {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", entry.ID, entry.Name, entry.Source, entry.Tool, entry.ArchivedAt.UTC().Format("2006-01-02 15:04:05Z"))
	}
	_ = table.Flush()
}

func printNotices(w io.Writer, notices []engine.Notice) {
	if len(notices) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Notices:")
	for _, notice := range notices {
		fmt.Fprintf(w, "  - %s\n", notice.Message)
	}
}

func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}

func runShow(args []string, stdout, stderr io.Writer) int {
	cmd := newCommand("skillet show", "usage: skillet show <name> [--json]",
		"Shows one Skill. Use Source:Name (or Source:Tool:Name) when a bare name is\nambiguous.")
	asJSON := cmd.flags.Bool("json", false, "emit the Skill as JSON")
	if code, done := cmd.parse(args, stdout, stderr); done {
		return code
	}
	if cmd.flags.NArg() != 1 {
		return fail(stderr, usagef("show takes exactly one Skill name"))
	}

	e, err := newEngineForCWD()
	if err != nil {
		return fail(stderr, err)
	}
	skill, err := resolveSkill(e.Inventory(), cmd.flags.Arg(0))
	if err != nil {
		return fail(stderr, err)
	}
	engine.MeasureSkillFiles(&skill)

	if *asJSON {
		if err := writeJSON(stdout, showJSON{SchemaVersion: jsonSchemaVersion, Skill: newSkillJSON(skill)}); err != nil {
			return fail(stderr, err)
		}
		return 0
	}

	table := tabwriter.NewWriter(stdout, 0, 0, 1, ' ', 0)
	fmt.Fprintf(table, "Name:\t%s\n", skill.Name)
	fmt.Fprintf(table, "Qualified name:\t%s\n", qualifiedSkillName(skill))
	fmt.Fprintf(table, "Source:\t%s\n", skill.Source)
	fmt.Fprintf(table, "Tool:\t%s\n", skill.Tool)
	fmt.Fprintf(table, "Kind:\t%s\n", skill.Kind)
	fmt.Fprintf(table, "Activation:\t%s\n", skill.Activation)
	fmt.Fprintf(table, "Location:\t%s\n", skill.Location)
	if skill.Plugin != nil {
		fmt.Fprintf(table, "Plugin:\t%s (marketplace %s, %d Skills)\n", skill.Plugin.Plugin, skill.Plugin.Marketplace, skill.Plugin.SkillCount)
	}
	if skill.DeclaredManualOnlyForClaude {
		fmt.Fprintf(table, "Declared Manual-only for Claude Code:\ttrue (no effect under Codex)\n")
	}
	fmt.Fprintf(table, "Description:\t%s\n", oneLine(skill.Description))
	// Cost, in the same order the TUI detail pane uses: the standing cost
	// first, then what invoking it costs, then what it occupies. Every figure
	// is an estimate and says so.
	fmt.Fprintf(table, "Cost per session (est.):\t%s\n", perSessionCostLine(skill))
	fmt.Fprintf(table, "Cost when invoked (est.):\t%s tokens (%s)\n",
		engine.FormatTokenEstimate(skill.BodyTokens), engine.FormatByteSize(skill.BodyBytes))
	fmt.Fprintf(table, "On disk:\t%s, %s\n", countedFiles(skill.FileCount), engine.FormatByteSize(skill.TotalBytes))
	_ = table.Flush()
	return 0
}

func countedFiles(count int) string {
	if count == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", count)
}

// perSessionCostLine states the standing cost of a Skill, and — when it has
// none — why, so a zero never reads as a measurement error.
func perSessionCostLine(skill engine.Skill) string {
	if skill.Activation == engine.ActivationAuto {
		return fmt.Sprintf("%s tokens (its description, injected into every %s session)",
			engine.FormatTokenEstimate(skill.DescriptionTokens), skill.Tool)
	}
	return fmt.Sprintf("~0 tokens (%s; %s tokens if set back to Auto-activation)",
		skill.Activation, engine.FormatTokenEstimate(skill.DescriptionTokens))
}

func runArchive(args []string, stdout, stderr io.Writer) int {
	cmd := newCommand("skillet archive", "usage: skillet archive <name> --yes",
		"Archives a Skill: it leaves its Source but is kept, recoverable, in Skillet's\nArchive. Undo it with \"skillet restore\".")
	confirmed := cmd.yesFlag()
	if code, done := cmd.parse(args, stdout, stderr); done {
		return code
	}
	if cmd.flags.NArg() != 1 {
		return fail(stderr, usagef("archive takes exactly one Skill name"))
	}
	if err := requireYes(*confirmed, "archive"); err != nil {
		return fail(stderr, err)
	}

	e, err := newEngineForCWD()
	if err != nil {
		return fail(stderr, err)
	}
	skill, err := resolveSkill(e.Inventory(), cmd.flags.Arg(0))
	if err != nil {
		return fail(stderr, err)
	}
	entry, err := e.Uninstall(skill.Location)
	if err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintf(stdout, "Archived %s %s %q from %s (archive id %s; restore with \"skillet restore %s --yes\")\n",
		entry.Source, entry.Kind, entry.Name, entry.OriginalLocation, entry.ID, entry.ID)
	return 0
}

func runRestore(args []string, stdout, stderr io.Writer) int {
	cmd := newCommand("skillet restore", "usage: skillet restore <id|name> --yes",
		"Restores an archived Skill to its original Source, exactly as it was. Accepts\nan archive id (see \"skillet list\") or an unambiguous archived Skill name.")
	confirmed := cmd.yesFlag()
	if code, done := cmd.parse(args, stdout, stderr); done {
		return code
	}
	if cmd.flags.NArg() != 1 {
		return fail(stderr, usagef("restore takes exactly one archive id or Skill name"))
	}
	if err := requireYes(*confirmed, "restore"); err != nil {
		return fail(stderr, err)
	}

	e, err := newEngineForCWD()
	if err != nil {
		return fail(stderr, err)
	}
	entry, err := resolveArchiveEntry(e, cmd.flags.Arg(0))
	if err != nil {
		return fail(stderr, err)
	}
	if err := e.Restore(entry.ID); err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintf(stdout, "Restored %s %s %q to %s (archive entry %s is gone)\n",
		entry.Source, entry.Kind, entry.Name, entry.OriginalLocation, entry.ID)
	return 0
}

func runPurge(args []string, stdout, stderr io.Writer) int {
	cmd := newCommand("skillet purge", "usage: skillet purge <id|name> --yes",
		"Permanently deletes an archived Skill. This is the only destructive command in\nSkillet and it always requires --yes.")
	confirmed := cmd.yesFlag()
	if code, done := cmd.parse(args, stdout, stderr); done {
		return code
	}
	if cmd.flags.NArg() != 1 {
		return fail(stderr, usagef("purge takes exactly one archive id or Skill name"))
	}
	if !*confirmed {
		return fail(stderr, usagef("purge permanently deletes an archived Skill and cannot be undone: re-run with --yes"))
	}

	e, err := newEngineForCWD()
	if err != nil {
		return fail(stderr, err)
	}
	entry, err := resolveArchiveEntry(e, cmd.flags.Arg(0))
	if err != nil {
		return fail(stderr, err)
	}
	if err := e.Purge(entry.ID); err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintf(stdout, "Purged archive entry %s (%s %s %q, originally at %s) — this was permanent\n",
		entry.ID, entry.Source, entry.Kind, entry.Name, entry.OriginalLocation)
	return 0
}

// resolveArchiveEntry accepts an archive id or, when unambiguous, the name of
// an archived Skill.
func resolveArchiveEntry(e *engine.Engine, arg string) (engine.ArchiveEntry, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return engine.ArchiveEntry{}, usagef("an archive id or Skill name is required")
	}
	entries, _, err := e.ListArchive()
	if err != nil {
		return engine.ArchiveEntry{}, err
	}
	for _, entry := range entries {
		if entry.ID == arg {
			return entry, nil
		}
	}
	var matches []engine.ArchiveEntry
	for _, entry := range entries {
		if strings.EqualFold(entry.Name, arg) {
			matches = append(matches, entry)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return engine.ArchiveEntry{}, fmt.Errorf("no Archive entry matches %q — run \"skillet list\" to see the Archive", arg)
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "%q matches %d Archive entries — re-run with one of these ids:", arg, len(matches))
	for _, entry := range matches {
		fmt.Fprintf(&builder, "\n  %s  (%s:%s, archived %s)", entry.ID, entry.Source, entry.Name, entry.ArchivedAt.UTC().Format("2006-01-02 15:04:05Z"))
	}
	return engine.ArchiveEntry{}, fmt.Errorf("%s", builder.String())
}

// skillMutation is the shape shared by suppress/unsuppress/manual-only/auto:
// resolve one Skill, apply one engine call, print one summary line.
type skillMutation struct {
	name    string
	usage   string
	notes   string
	apply   func(*engine.Engine, engine.Skill) error
	summary func(engine.Skill) string
}

func runSkillMutation(mutation skillMutation, args []string, stdout, stderr io.Writer) int {
	cmd := newCommand("skillet "+mutation.name, mutation.usage, mutation.notes)
	confirmed := cmd.yesFlag()
	if code, done := cmd.parse(args, stdout, stderr); done {
		return code
	}
	if cmd.flags.NArg() != 1 {
		return fail(stderr, usagef("%s takes exactly one Skill name", mutation.name))
	}
	if err := requireYes(*confirmed, mutation.name); err != nil {
		return fail(stderr, err)
	}
	e, err := newEngineForCWD()
	if err != nil {
		return fail(stderr, err)
	}
	skill, err := resolveSkill(e.Inventory(), cmd.flags.Arg(0))
	if err != nil {
		return fail(stderr, err)
	}
	if err := mutation.apply(e, skill); err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintln(stdout, mutation.summary(skill))
	return 0
}

func runSuppress(args []string, stdout, stderr io.Writer) int {
	return runSkillMutation(skillMutation{
		name:  "suppress",
		usage: "usage: skillet suppress <name> --yes",
		notes: "Hides one Skill from the model and from slash commands. A Plugin Skill's\nplugin stays installed and intact; a Codex Skill uses Codex's native per-skill\ndisable.",
		apply: func(e *engine.Engine, skill engine.Skill) error { return e.Suppress(skill) },
		summary: func(skill engine.Skill) string {
			return fmt.Sprintf("Suppressed %s %s %q at %s%s", skill.Source, skill.Kind, skill.Name, skill.Location, pluginSuffix(skill))
		},
	}, args, stdout, stderr)
}

func runUnsuppress(args []string, stdout, stderr io.Writer) int {
	return runSkillMutation(skillMutation{
		name:  "unsuppress",
		usage: "usage: skillet unsuppress <name> --yes",
		notes: "Reverses Suppress, making the Skill visible to the model and slash commands\nagain.",
		apply: func(e *engine.Engine, skill engine.Skill) error { return e.Unsuppress(skill) },
		summary: func(skill engine.Skill) string {
			return fmt.Sprintf("Unsuppressed %s %s %q at %s%s", skill.Source, skill.Kind, skill.Name, skill.Location, pluginSuffix(skill))
		},
	}, args, stdout, stderr)
}

// manual-only and auto live in cli_activation.go: they take many names and a
// sweep flag, so they do not fit runSkillMutation's one-name shape.

func pluginSuffix(skill engine.Skill) string {
	if skill.Plugin == nil {
		return ""
	}
	return fmt.Sprintf(" (plugin %s@%s, still installed)", skill.Plugin.Plugin, skill.Plugin.Marketplace)
}
