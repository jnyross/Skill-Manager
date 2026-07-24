package main

// Scriptable command surface (WP3). Every subcommand here talks to
// internal/engine directly — never to internal/tui — so an agent session or a
// CI job can do anything the TUI can do. The vocabulary in every message and
// every JSON field name is CONTEXT.md's: Source, Tool, Skill, Archive,
// Restore, Purge, Suppress, Manual-only, Library, Bundle, Install.

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

// usageSummary is the one-line pointer printed on a usage error. The word
// "usage:" is load-bearing for the top-level tests.
const usageSummary = "usage: skillet [command] [flags] — run \"skillet --help\" for the full command tree"

// usageError marks a caller mistake (bad flag, missing argument, unknown
// token) so the dispatcher can exit 2 instead of 1. Operation failures — a
// Skill that does not exist, a write that failed — stay plain errors and
// exit 1.
type usageError struct{ message string }

func (e usageError) Error() string { return e.message }

func usagef(format string, args ...any) error {
	return usageError{message: fmt.Sprintf(format, args...)}
}

// exitCodeFor maps an error to the documented exit codes: 2 for usage, 1 for
// everything else.
func exitCodeFor(err error) int {
	var usage usageError
	if errors.As(err, &usage) {
		return 2
	}
	return 1
}

// fail prints err and returns its exit code, appending the usage pointer for
// usage errors so a mistyped command always says where to look.
func fail(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, err)
	code := exitCodeFor(err)
	if code == 2 {
		fmt.Fprintln(stderr, usageSummary)
	}
	return code
}

func printTopLevelHelp(w io.Writer) {
	fmt.Fprint(w, `skillet — manage agent Skills across Claude Code and Codex

usage: skillet [command] [flags]

Run skillet with no command to launch the TUI.

Inventory
  list [--json] [--source SOURCE] [--tool TOOL]   List every Skill, plus notices
  show <name> [--json]                            Show one Skill in detail
  cost [--json]                                   Estimate what Skills cost in context

Archive
  archive <name> --yes                            Archive a Skill (reversible)
  restore <id|name> --yes                         Restore an archived Skill to its Source
  purge <id|name> --yes                           Permanently delete an archived Skill

Activation
  suppress <name> --yes                           Hide a Skill from the model and slash commands
  unsuppress <name> --yes                         Undo Suppress
  manual-only <name> --yes                        Turn Auto-activation off
  auto <name> --yes                               Turn Auto-activation back on

Library and Bundles
  library list [--json]                           List Library entries
  library add --name NAME <source flags> --yes    Add a Library entry
  library remove <id|name> --yes                  Remove a Library entry
  bundle list [--json]                            List Bundles
  bundle install <bundle> --target T --yes        Install every member of a Bundle
  install <id|name> --target T --yes              Install one Library entry

Other
  setup [flags]                                   Guided agent-ready workspace setup
  version                                         Print the Skillet version

Naming
  A bare Skill name works when it is unambiguous. Otherwise qualify it as
  Source:Name (for example Project:review), or Source:Tool:Name when one
  Source holds that name under both Tools (for example Project:codex:review).
  Sources: Personal, Plugin, Codex, Project. Tools: claude-code, codex.

Targets
  --target personal installs at the user level; --target PATH installs into
  that repository (the path must be a real directory).

Conventions
  Every command that changes anything on disk requires --yes, mirroring the
  TUI's confirmation, and prints a one-line summary of exactly what changed.
  Exit codes: 0 success, 1 operation error, 2 usage error.

Run "skillet <command> --help" for a command's flags.
`)
}

// command bundles a FlagSet with the usage text printed for --help and for a
// flag error, so every subcommand documents itself the same way.
type command struct {
	flags *flag.FlagSet
	usage string
	notes string
}

func newCommand(name, usage, notes string) *command {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Usage = func() {}
	return &command{flags: flags, usage: usage, notes: notes}
}

func (c *command) printHelp(w io.Writer) {
	fmt.Fprintln(w, c.usage)
	if c.notes != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, strings.TrimRight(c.notes, "\n"))
	}
	hasFlags := false
	c.flags.VisitAll(func(*flag.Flag) { hasFlags = true })
	if hasFlags {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Flags:")
		c.flags.SetOutput(w)
		c.flags.PrintDefaults()
		c.flags.SetOutput(io.Discard)
	}
}

// permute moves flags ahead of positional arguments so "skillet show writing
// --json" behaves like "skillet show --json writing". The stdlib flag package
// stops at the first non-flag; scripts and humans both expect otherwise.
// Unknown flags are passed through untouched so Parse still reports them.
func (c *command) permute(args []string) []string {
	var flagArgs, positional []string
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--" {
			positional = append(positional, args[index+1:]...)
			break
		}
		if len(argument) < 2 || argument[0] != '-' {
			positional = append(positional, argument)
			continue
		}
		name := strings.TrimLeft(argument, "-")
		if strings.Contains(name, "=") {
			flagArgs = append(flagArgs, argument)
			continue
		}
		definition := c.flags.Lookup(name)
		if definition != nil && !isBoolFlag(definition) && index+1 < len(args) {
			flagArgs = append(flagArgs, argument, args[index+1])
			index++
			continue
		}
		flagArgs = append(flagArgs, argument)
	}
	return append(flagArgs, positional...)
}

func isBoolFlag(definition *flag.Flag) bool {
	boolean, ok := definition.Value.(interface{ IsBoolFlag() bool })
	return ok && boolean.IsBoolFlag()
}

// parse handles --help (exit 0, help on stdout) and flag errors (exit 2, help
// on stderr). It returns done=true when the caller should return code.
func (c *command) parse(args []string, stdout, stderr io.Writer) (code int, done bool) {
	err := c.flags.Parse(c.permute(args))
	switch {
	case errors.Is(err, flag.ErrHelp):
		c.printHelp(stdout)
		return 0, true
	case err != nil:
		fmt.Fprintln(stderr, err)
		c.printHelp(stderr)
		return 2, true
	}
	return 0, false
}

// yesFlag registers the confirmation flag every mutating command requires.
func (c *command) yesFlag() *bool {
	return c.flags.Bool("yes", false, "confirm this change (required: the TUI would ask before doing it)")
}

func requireYes(confirmed bool, action string) error {
	if confirmed {
		return nil
	}
	return usagef("%s changes files on disk and needs confirmation: re-run with --yes", action)
}

// newEngineAt builds an Engine rooted at the user's home directory, with
// Project Sources discovered from dir. dir is the current working directory
// for read commands and the Install target for a Project Install, so an
// Install into a repository the user named on the command line resolves that
// repository's roots rather than the shell's.
func newEngineAt(dir string) (*engine.Engine, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	codexRoots, claudeRoots := engine.FindProjectRootsForTools(dir)
	return engine.New(engine.Roots{
		ClaudeHome:         filepath.Join(home, ".claude"),
		CodexHome:          filepath.Join(home, ".codex"),
		AgentsHome:         filepath.Join(home, ".agents"),
		DataDir:            filepath.Join(home, ".skillet"),
		ProjectRoots:       codexRoots,
		ClaudeProjectRoots: claudeRoots,
	}), nil
}

func newEngineForCWD() (*engine.Engine, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return newEngineAt(cwd)
}

// --- name resolution -------------------------------------------------------

func toolToken(tool engine.Tool) string {
	if tool == engine.ToolCodex {
		return "codex"
	}
	return "claude-code"
}

// qualifiedSkillName is the Source:Name form the CLI accepts and prints.
func qualifiedSkillName(skill engine.Skill) string {
	return string(skill.Source) + ":" + skill.Name
}

// fullyQualifiedSkillName adds the Tool, for the one ambiguity Source:Name
// cannot resolve: a Project Skill of the same name under both Tools.
func fullyQualifiedSkillName(skill engine.Skill) string {
	return string(skill.Source) + ":" + toolToken(skill.Tool) + ":" + skill.Name
}

func parseSourceToken(token string) (engine.Source, error) {
	for _, source := range []engine.Source{engine.SourcePersonal, engine.SourcePlugin, engine.SourceCodex, engine.SourceProject} {
		if strings.EqualFold(token, string(source)) {
			return source, nil
		}
	}
	return "", usagef("unknown Source %q: use Personal, Plugin, Codex, or Project", token)
}

func parseToolToken(token string) (engine.Tool, error) {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "claude-code", "claude code", "claudecode", "claude":
		return engine.ToolClaudeCode, nil
	case "codex":
		return engine.ToolCodex, nil
	}
	return "", usagef("unknown Tool %q: use claude-code or codex", token)
}

// skillSelector is a parsed NAME / Source:Name / Source:Tool:Name argument.
type skillSelector struct {
	name   string
	source engine.Source
	tool   engine.Tool
}

func parseSkillSelector(arg string) (skillSelector, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return skillSelector{}, usagef("a Skill name is required")
	}
	parts := strings.Split(arg, ":")
	selector := skillSelector{name: strings.TrimSpace(parts[len(parts)-1])}
	if selector.name == "" {
		return skillSelector{}, usagef("%q has no Skill name", arg)
	}
	switch len(parts) {
	case 1:
	case 2:
		source, err := parseSourceToken(parts[0])
		if err != nil {
			return skillSelector{}, err
		}
		selector.source = source
	case 3:
		source, err := parseSourceToken(parts[0])
		if err != nil {
			return skillSelector{}, err
		}
		tool, err := parseToolToken(parts[1])
		if err != nil {
			return skillSelector{}, err
		}
		selector.source, selector.tool = source, tool
	default:
		return skillSelector{}, usagef("%q is not a valid Skill name: use NAME, Source:Name, or Source:Tool:Name", arg)
	}
	return selector, nil
}

func (s skillSelector) matches(skill engine.Skill) bool {
	if !strings.EqualFold(skill.Name, s.name) {
		return false
	}
	if s.source != "" && skill.Source != s.source {
		return false
	}
	if s.tool != "" && skill.Tool != s.tool {
		return false
	}
	return true
}

// resolveSkill accepts a bare name when it identifies exactly one Skill. On
// ambiguity it fails with the qualified candidates, which are themselves
// valid input.
func resolveSkill(inventory engine.Inventory, arg string) (engine.Skill, error) {
	selector, err := parseSkillSelector(arg)
	if err != nil {
		return engine.Skill{}, err
	}
	var matches []engine.Skill
	for _, skill := range inventory.Skills {
		if selector.matches(skill) {
			matches = append(matches, skill)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return engine.Skill{}, fmt.Errorf("no Skill matches %q — run \"skillet list\" to see the inventory", arg)
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "%q matches %d Skills — re-run with one of:", arg, len(matches))
	for _, candidate := range candidateNames(matches) {
		fmt.Fprintf(&builder, "\n  %s", candidate)
	}
	return engine.Skill{}, errors.New(builder.String())
}

// candidateNames renders each ambiguous match as the shortest qualified form
// that is unique within the match set, followed by its Location.
func candidateNames(matches []engine.Skill) []string {
	counts := make(map[string]int, len(matches))
	for _, skill := range matches {
		counts[qualifiedSkillName(skill)]++
	}
	names := make([]string, 0, len(matches))
	for _, skill := range matches {
		name := qualifiedSkillName(skill)
		if counts[name] > 1 {
			name = fullyQualifiedSkillName(skill)
		}
		names = append(names, fmt.Sprintf("%s  (%s)", name, skill.Location))
	}
	sort.Strings(names)
	return names
}

// --- Install targets -------------------------------------------------------

// resolveInstallTarget turns --target into an engine.InstallTarget and the
// directory the Engine should discover Project roots from. "personal" targets
// the user level; anything else is a repository path, which must exist.
func resolveInstallTarget(value string) (engine.InstallTarget, string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return engine.InstallTarget{}, "", usagef("--target is required: personal or a repository path")
	}
	if strings.EqualFold(value, "personal") {
		cwd, err := os.Getwd()
		if err != nil {
			return engine.InstallTarget{}, "", err
		}
		return engine.InstallTarget{Kind: engine.InstallTargetPersonal}, cwd, nil
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return engine.InstallTarget{}, "", usagef("--target %q: %v", value, err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return engine.InstallTarget{}, "", usagef("--target %q: %v", value, err)
	}
	if !info.IsDir() {
		return engine.InstallTarget{}, "", usagef("--target %q is not a directory", value)
	}
	return engine.InstallTarget{Kind: engine.InstallTargetProject, RepoRoot: absolute}, absolute, nil
}

func describeTarget(target engine.InstallTarget) string {
	if target.Kind == engine.InstallTargetProject {
		return "project " + target.RepoRoot
	}
	return "Personal"
}
