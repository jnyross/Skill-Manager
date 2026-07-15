package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jnyross/Skill-Manager/internal/engine"
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

func runWithInput(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		if len(args) == 1 && (args[0] == "--version" || args[0] == "version") {
			if version == "" || version == "dev" {
				fmt.Fprintf(stdout, "skillet development (commit %s, built %s)\n", commit, buildDate)
			} else {
				fmt.Fprintf(stdout, "skillet %s (commit %s, built %s)\n", version, commit, buildDate)
			}
			return 0
		}
		if args[0] == "setup" {
			return runSetup(args[1:], stdin, stdout, stderr)
		}
		fmt.Fprintln(stderr, "usage: skillet [--version|version|setup]")
		return 2
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	e := engine.New(engine.Roots{
		ClaudeHome:         filepath.Join(home, ".claude"),
		CodexHome:          filepath.Join(home, ".codex"),
		AgentsHome:         filepath.Join(home, ".agents"),
		DataDir:            filepath.Join(home, ".skillet"),
		ProjectRoots:       engine.FindProjectRoots(cwd),
		ClaudeProjectRoots: engine.FindClaudeProjectRoots(cwd),
	})
	model := tui.NewModel(e)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithOutput(stdout))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if model.SetupRequested() {
		result, err := workspaceSetup.RunTerminal(context.Background(), stdin, stdout, workspaceSetup.TerminalOptions{UseNativePicker: true})
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if result.Outcome == workspaceSetup.OutcomeBlocked {
			fmt.Fprintf(stderr, "Blocked: %s\n", result.NextAction)
			return 1
		}
	}
	return 0
}

func runSetup(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("skillet setup", flag.ContinueOnError)
	flags.SetOutput(stderr)
	pathValue := flags.String("path", "", "project folder to configure")
	bundlesValue := flags.String("bundles", "", "comma-separated Built-in catalog bundle ids")
	yes := flags.Bool("yes", false, "apply the reviewed plan without the final confirmation prompt")
	acceptDrift := flags.Bool("accept-drift", false, "explicitly accept every displayed material source drift")
	replaceConflicts := flags.Bool("replace-conflicts", false, "back up and replace every named required conflict")
	manualOnly := flags.String("manual-only", "", "comma-separated selected members to configure as manual-only")
	autoActivation := flags.String("auto", "", "comma-separated selected members to configure for automatic invocation")
	staticOnly := flags.Bool("static", false, "configure and statically verify without authenticated Tool probes")
	if err := flags.Parse(args); err != nil {
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
		return 1
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
