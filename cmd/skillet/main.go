package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jnyross/Skill-Manager/internal/catalog"
	workspaceSetup "github.com/jnyross/Skill-Manager/internal/setup"
	"github.com/jnyross/Skill-Manager/internal/tui"
)

var (
	version               = "dev"
	commit                = "unknown"
	buildDate             = "unknown"
	setupTerminalDefaults = func() workspaceSetup.TerminalOptions { return workspaceSetup.TerminalOptions{} }
)

func main() {
	if code := run(os.Args[1:], os.Stdout, os.Stderr); code != 0 {
		os.Exit(code)
	}
}

func run(args []string, stdout, stderr io.Writer) int {
	return runWithInput(args, os.Stdin, stdout, stderr)
}

// runWithInput dispatches the command tree. No arguments still launches the
// TUI; every other entry point is scriptable (see cli.go and docs/agents/
// cli.md).
func runWithInput(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		command, rest := args[0], args[1:]
		switch command {
		case "-h", "--help", "help":
			printTopLevelHelp(stdout)
			return 0
		case "--version", "version":
			if len(rest) != 0 {
				fmt.Fprintf(stderr, "version takes no arguments (got %q)\n", rest[0])
				fmt.Fprintln(stderr, usageSummary)
				return 2
			}
			printVersion(stdout)
			return 0
		case "setup":
			return runSetup(rest, stdin, stdout, stderr)
		case "list":
			return runList(rest, stdout, stderr)
		case "show":
			return runShow(rest, stdout, stderr)
		case "archive":
			return runArchive(rest, stdout, stderr)
		case "restore":
			return runRestore(rest, stdout, stderr)
		case "purge":
			return runPurge(rest, stdout, stderr)
		case "suppress":
			return runSuppress(rest, stdout, stderr)
		case "unsuppress":
			return runUnsuppress(rest, stdout, stderr)
		case "manual-only":
			return runManualOnly(rest, stdout, stderr)
		case "auto":
			return runAuto(rest, stdout, stderr)
		case "library":
			return runLibrary(rest, stdout, stderr)
		case "bundle":
			return runBundle(rest, stdout, stderr)
		case "install":
			return runInstall(rest, stdout, stderr)
		}
		fmt.Fprintf(stderr, "unknown command %q\n", command)
		fmt.Fprintln(stderr, usageSummary)
		return 2
	}

	return runInteractive(stdin, stdout, stderr)
}

// runTUISession and runSetupWizard are the two halves of the interactive
// round-trip, kept as variables so the seam is testable without a real
// terminal.
var (
	runTUISession  = defaultTUISession
	runSetupWizard = defaultSetupWizard
)

func defaultTUISession(stdout io.Writer, status string, statusIsError bool) (bool, error) {
	e, err := newEngineForCWD()
	if err != nil {
		return false, err
	}
	model := tui.NewModel(e)
	model.SetInitialStatus(status, statusIsError)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithOutput(stdout))
	if _, err := p.Run(); err != nil {
		return false, err
	}
	return model.SetupRequested(), nil
}

func defaultSetupWizard(stdin io.Reader, stdout io.Writer) (workspaceSetup.Result, error) {
	return workspaceSetup.RunTerminal(context.Background(), stdin, stdout, workspaceSetup.TerminalOptions{UseNativePicker: true})
}

// runInteractive is the no-argument entry point: the TUI, and — when `S` asks
// for it — the line-oriented Setup wizard, after which the TUI comes back with
// the Setup outcome on its status line. The wizard itself is unchanged and
// still runs on the raw terminal: tea.Program.Run has already left the
// alternate screen by the time it returns, and the next loop iteration enters a
// fresh one.
func runInteractive(stdin io.Reader, stdout, stderr io.Writer) int {
	var status string
	var statusIsError bool
	for {
		setupRequested, err := runTUISession(stdout, status, statusIsError)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if !setupRequested {
			return 0
		}
		result, err := runSetupWizard(stdin, stdout)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		status, statusIsError = setupStatusLine(result)
	}
}

// setupStatusLine summarizes a finished wizard run for the status line of the
// relaunched TUI, using CONTEXT.md's Setup outcome words verbatim. The second
// return value is whether it should read as an error.
func setupStatusLine(result workspaceSetup.Result) (string, bool) {
	switch result.Outcome {
	case workspaceSetup.OutcomeCanceled:
		return "Setup canceled — nothing was changed.", false
	case "":
		return "Setup returned no outcome.", true
	case workspaceSetup.OutcomeBlocked, workspaceSetup.OutcomePartial:
		return withNextAction("Setup outcome: "+string(result.Outcome)+".", result.NextAction), true
	default:
		return withNextAction("Setup outcome: "+string(result.Outcome)+".", result.NextAction), false
	}
}

func withNextAction(line, nextAction string) string {
	if strings.TrimSpace(nextAction) == "" {
		return line
	}
	return line + " Next: " + nextAction
}

func printVersion(stdout io.Writer) {
	if version == "" || version == "dev" {
		fmt.Fprintf(stdout, "skillet development (commit %s, built %s)\n", commit, buildDate)
		return
	}
	fmt.Fprintf(stdout, "skillet %s (commit %s, built %s)\n", version, commit, buildDate)
}

func runSetup(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("skillet setup", flag.ContinueOnError)
	flags.SetOutput(stdout)
	pathValue := flags.String("path", "", "project folder to configure")
	bundlesValue := flags.String("bundles", "", "comma-separated Built-in catalog bundle ids")
	yes := flags.Bool("yes", false, "apply the reviewed plan without the final confirmation prompt")
	acceptDrift := flags.Bool("accept-drift", false, "explicitly accept every displayed material source drift")
	replaceConflicts := flags.Bool("replace-conflicts", false, "back up and replace every named required conflict")
	manualOnly := flags.String("manual-only", "", "comma-separated selected members to configure as manual-only")
	autoActivation := flags.String("auto", "", "comma-separated selected members to configure for automatic invocation")
	staticOnly := flags.Bool("static", false, "configure and statically verify without authenticated Tool probes")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: skillet setup [--path PATH] [--bundles IDS] [--manual-only MEMBERS] [--auto MEMBERS] [--yes] [--accept-drift] [--replace-conflicts] [--static]")
		return 2
	}
	options := setupTerminalDefaults()
	var service = options.Service
	if *staticOnly {
		service = workspaceSetup.NewService()
	}
	activation := make(map[string]workspaceSetup.Activation)
	for _, name := range splitComma(*manualOnly) {
		activation[name] = workspaceSetup.ActivationManualOnly
	}
	for _, name := range splitComma(*autoActivation) {
		if activation[name] == workspaceSetup.ActivationManualOnly {
			fmt.Fprintf(stderr, "activation override for %s appears in both --manual-only and --auto\n", name)
			return 2
		}
		activation[name] = workspaceSetup.ActivationAuto
	}
	result, err := workspaceSetup.RunTerminal(context.Background(), stdin, stdout, workspaceSetup.TerminalOptions{
		Catalog: options.Catalog, Resolver: options.Resolver, ToolPreflight: options.ToolPreflight,
		Path: *pathValue, BundleIDs: splitComma(*bundlesValue), AssumeYes: *yes,
		AcceptAllDrift: *acceptDrift, ReplaceConflicts: *replaceConflicts, Activation: activation, Service: service,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		if errors.Is(err, catalog.ErrUnknownBundle) {
			return 2
		}
		return 1
	}
	if result.Outcome == workspaceSetup.OutcomeCanceled {
		fmt.Fprintln(stdout, "Setup canceled.")
		return 0
	}
	if result.Outcome == workspaceSetup.OutcomeBlocked {
		if result.NextAction != "" {
			fmt.Fprintf(stderr, "Blocked: %s\n", result.NextAction)
		}
		return 1
	}
	return 0
}

func splitComma(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var values []string
	for _, item := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}
