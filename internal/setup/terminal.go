package setup

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/jnyross/Skill-Manager/internal/catalog"
)

type MemberResolver interface {
	ResolveMembers(context.Context, []catalog.Member) ([]ResolvedMember, func(), error)
}

type TerminalOptions struct {
	Catalog          *catalog.Catalog
	Resolver         MemberResolver
	Service          *Service
	Path             string
	BundleIDs        []string
	AssumeYes        bool
	AcceptAllDrift   bool
	ReplaceConflicts bool
	Activation       map[string]Activation
	UseNativePicker  bool
	Picker           FolderPicker
	ToolPreflight    func(context.Context) []ToolResult
}

func RunTerminal(ctx context.Context, input io.Reader, output io.Writer, options TerminalOptions) (Result, error) {
	var c catalog.Catalog
	if options.Catalog == nil {
		loaded, err := catalog.Load()
		if err != nil {
			return Result{}, err
		}
		c = loaded
	} else {
		if err := options.Catalog.Validate(); err != nil {
			return Result{}, fmt.Errorf("invalid injected catalog: %w", err)
		}
		c = *options.Catalog
	}
	resolver := options.Resolver
	if resolver == nil {
		resolver = GitResolver{}
	}
	service := options.Service
	if service == nil {
		service = NewLiveService()
	}
	reader := bufio.NewReader(input)

	target := strings.TrimSpace(options.Path)
	if target == "" && options.UseNativePicker {
		picker := options.Picker
		if picker == nil {
			picker = NativeFolderPicker{}
		}
		picked, pickErr := picker.Pick(ctx)
		if pickErr == nil {
			target = strings.TrimSpace(picked)
		} else if pickErr == ErrPickerCanceled {
			return Result{Outcome: OutcomeCanceled, NextAction: "Setup canceled in the folder picker"}, nil
		} else {
			fmt.Fprintf(output, "Native folder picker unavailable: %v\nUsing guarded terminal path entry.\n", pickErr)
		}
	}
	if target == "" {
		fmt.Fprint(output, "Project folder (blank cancels): ")
		target = readLine(reader)
		if target == "" {
			return Result{Outcome: OutcomeCanceled, NextAction: "Setup canceled before selecting a folder"}, nil
		}
	}

	bundleIDs := append([]string(nil), options.BundleIDs...)
	if len(bundleIDs) == 0 {
		recommended := make(map[string]bool)
		for _, id := range c.RecommendedBundleIDs(target) {
			recommended[id] = true
		}
		fmt.Fprintf(output, "Built-in catalog %s bundles:\n", c.Version)
		for _, bundle := range c.Bundles {
			recommendation := ""
			if recommended[bundle.ID] {
				recommendation = " [recommended from project signals]"
			}
			fmt.Fprintf(output, "  %s — %s (%d members)%s\n", bundle.ID, bundle.Name, len(bundle.Members), recommendation)
			if bundle.OverlapWarning != "" {
				fmt.Fprintf(output, "    warning: %s\n", bundle.OverlapWarning)
			}
		}
		fmt.Fprint(output, "Bundle ids, comma separated (blank cancels): ")
		line := readLine(reader)
		if line == "" {
			return Result{Outcome: OutcomeCanceled, NextAction: "Setup canceled before selecting a Bundle"}, nil
		}
		bundleIDs = splitIDs(line)
	}
	selection, err := c.SelectBundles(bundleIDs)
	if err != nil {
		return Result{}, err
	}
	for _, member := range selection.Members {
		fmt.Fprintf(output, "  %s — %s, activation=%s, %s@%s:%s\n", member.Name, member.License.SPDX, member.UpstreamActivation, member.Source.Repository, member.Source.ReviewedRevision[:12], member.Source.Subpath)
		fmt.Fprintf(output, "    destinations: .claude/skills/%s, .agents/skills/%s\n", member.Name, member.Name)
		if len(member.Dependencies) != 0 {
			for _, dependency := range member.Dependencies {
				ready, reason := dependencyReady(dependency.Name)
				fmt.Fprintf(output, "    dependency: %s ready=%t optional=%t — %s (%s)\n", dependency.Name, ready, dependency.Optional, dependency.Reason, reason)
			}
		}
		if len(member.Scripts) != 0 {
			fmt.Fprintf(output, "    scripts: %s\n", strings.Join(member.Scripts, ", "))
		}
		for _, action := range member.ExternalActions {
			fmt.Fprintf(output, "    external action when invoked: %s\n", action)
		}
		if member.License.Evidence != "license-text" {
			return Result{Outcome: OutcomeBlocked, NextAction: fmt.Sprintf("%s has %s license evidence; complete notice evidence before setup", member.Name, member.License.Evidence)}, nil
		}
	}
	preflight := options.ToolPreflight
	if preflight == nil {
		preflight = PreflightToolReadiness
	}
	for _, tool := range preflight(ctx) {
		fmt.Fprintf(output, "Tool preflight: %s executable=%s version=%q authenticated=%t\n", tool.Tool, tool.Executable, tool.Version, tool.Authenticated)
		if tool.Reason != "" {
			fmt.Fprintf(output, "  %s\n", tool.Reason)
		}
	}
	activation := make(map[string]Activation)
	for name, value := range options.Activation {
		activation[name] = value
	}
	if !options.AssumeYes && len(selection.Members) != 0 {
		fmt.Fprint(output, "Members to force manual-only, comma separated (blank keeps upstream activation): ")
		for _, name := range splitIDs(readLine(reader)) {
			activation[name] = ActivationManualOnly
		}
		fmt.Fprint(output, "Members to force auto, comma separated (blank keeps current selection): ")
		for _, name := range splitIDs(readLine(reader)) {
			activation[name] = ActivationAuto
		}
	}
	selectedNames := make(map[string]bool, len(selection.Members))
	for _, member := range selection.Members {
		selectedNames[member.Name] = true
	}
	for name, value := range activation {
		if !selectedNames[name] {
			return Result{}, fmt.Errorf("activation override references unselected member %q", name)
		}
		if value != ActivationAuto && value != ActivationManualOnly {
			return Result{}, fmt.Errorf("activation override for %s is %q", name, value)
		}
		fmt.Fprintf(output, "Activation override: %s -> %s for Claude Code and Codex\n", name, value)
	}
	for _, warning := range selection.Warnings {
		fmt.Fprintf(output, "warning: %s\n", warning)
	}

	resolved, cleanup, err := resolver.ResolveMembers(ctx, selection.Members)
	if err != nil {
		return Result{Outcome: OutcomeBlocked, NextAction: err.Error()}, nil
	}
	defer cleanup()
	acceptDrift := make(map[string]bool)
	for _, member := range resolved {
		if !member.Drift.Material {
			continue
		}
		fmt.Fprintf(output, "Material latest-source drift for %s (%s -> %s):\n", member.Member.Name, shortRevision(member.Drift.ReviewedRevision), shortRevision(member.Drift.ResolvedRevision))
		for _, change := range member.Drift.Changes {
			fmt.Fprintf(output, "  %s: %s -> %s\n", change.Class, compact(change.Reviewed), compact(change.Resolved))
		}
		if options.AcceptAllDrift {
			acceptDrift[member.Member.Name] = true
		} else if !options.AssumeYes && promptYes(reader, output, "Accept this unreviewed drift?") {
			acceptDrift[member.Member.Name] = true
		} else {
			return Result{Outcome: OutcomeBlocked, NextAction: fmt.Sprintf("Review %s and explicitly accept its drift before retrying", member.Member.Name)}, nil
		}
	}

	request := Request{
		TargetPath: target, CatalogVersion: c.Version, BundleIDs: bundleIDs, Members: resolved,
		AcceptDrift: acceptDrift, ReplaceConflicts: options.ReplaceConflicts, Activation: activation,
	}
	plan, err := service.Plan(ctx, request)
	if err != nil {
		return Result{}, err
	}
	fmt.Fprintf(output, "Target: %s\n", plan.TargetPath)
	if plan.NeedGitInit {
		fmt.Fprintln(output, "Git: initialize repository")
	} else {
		fmt.Fprintln(output, "Git: existing repository")
	}
	for _, change := range plan.Changes {
		fmt.Fprintf(output, "  %-32s %s\n", change.State, change.Path)
	}
	for _, warning := range plan.Warnings {
		fmt.Fprintf(output, "warning: %s\n", warning)
	}
	if len(plan.Blockers) != 0 && !request.ReplaceConflicts {
		conflictsOnly := true
		for _, blocker := range plan.Blockers {
			if !strings.Contains(blocker, "authorize a recoverable backup") {
				conflictsOnly = false
			}
			fmt.Fprintf(output, "blocked: %s\n", blocker)
		}
		if conflictsOnly && (options.ReplaceConflicts || (!options.AssumeYes && promptYes(reader, output, "Back up and replace every named conflict?"))) {
			request.ReplaceConflicts = true
			plan, err = service.Plan(ctx, request)
			if err != nil {
				return Result{}, err
			}
		} else {
			return Result{Outcome: OutcomeBlocked, NextAction: strings.Join(plan.Blockers, "; ")}, nil
		}
	}
	if len(plan.Blockers) != 0 {
		return Result{Outcome: OutcomeBlocked, NextAction: strings.Join(plan.Blockers, "; ")}, nil
	}
	if !options.AssumeYes && !promptYes(reader, output, "Apply this exact setup plan?") {
		return Result{Outcome: OutcomeBlocked, NextAction: "Setup canceled before mutation"}, nil
	}
	result, err := service.Apply(ctx, plan)
	if err != nil {
		return result, err
	}
	fmt.Fprintf(output, "Outcome: %s\nReceipt: %s\nLocal verification: %s\n", result.Outcome, result.ReceiptPath, result.LocalReceiptPath)
	if result.NextAction != "" {
		fmt.Fprintf(output, "Next: %s\n", result.NextAction)
	}
	return result, nil
}

func readLine(reader *bufio.Reader) string {
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func promptYes(reader *bufio.Reader, output io.Writer, question string) bool {
	fmt.Fprintf(output, "%s [y/N] ", question)
	answer := strings.ToLower(readLine(reader))
	return answer == "y" || answer == "yes"
}

func splitIDs(value string) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, item := range strings.Split(value, ",") {
		id := strings.TrimSpace(item)
		if id != "" && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	sort.Strings(ids)
	return ids
}

func shortRevision(value string) string {
	if len(value) > 12 {
		return value[:12]
	}
	return value
}

func compact(value string) string {
	if len(value) > 32 {
		return value[:32] + "…"
	}
	return value
}
