package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jnyross/Skill-Manager/internal/catalog"
	"github.com/jnyross/Skill-Manager/internal/setup"
)

type artifact struct {
	SchemaVersion      int             `json:"schemaVersion"`
	CatalogVersion     string          `json:"catalogVersion"`
	GovernanceBlockers []string        `json:"governanceBlockers"`
	Outcome            setup.Outcome   `json:"outcome"`
	RepeatNoOp         bool            `json:"repeatNoOp"`
	RemovalVerified    bool            `json:"removalVerified"`
	NegativeControls   map[string]bool `json:"negativeControls,omitempty"`
	Lanes              []lane          `json:"lanes"`
}

type lane struct {
	Member              string                   `json:"member"`
	Tool                string                   `json:"tool"`
	Source              string                   `json:"source"`
	SourceSubpath       string                   `json:"sourceSubpath"`
	ReviewedRevision    string                   `json:"reviewedRevision"`
	ResolvedRevision    string                   `json:"resolvedRevision"`
	ContentSHA256       string                   `json:"contentSHA256"`
	Destination         string                   `json:"destination"`
	Recipe              string                   `json:"recipe"`
	RequestedActivation setup.Activation         `json:"requestedActivation"`
	ObservedActivation  setup.Activation         `json:"observedActivation"`
	StaticVerified      bool                     `json:"staticVerified"`
	Executable          string                   `json:"executable,omitempty"`
	Authenticated       bool                     `json:"authenticated"`
	RuntimeVerified     bool                     `json:"runtimeVerified"`
	ProbeReason         string                   `json:"probeReason,omitempty"`
	DependencyResults   []setup.DependencyResult `json:"dependencyResults"`
	Warnings            []string                 `json:"warnings,omitempty"`
}

func main() {
	var sourceRoot, output string
	var latest, live, acceptDrift bool
	flag.StringVar(&sourceRoot, "source-root", "", "reviewed source checkout root")
	flag.StringVar(&output, "output", "catalog-matrix.json", "evidence artifact path")
	flag.BoolVar(&latest, "latest", false, "resolve latest source revisions instead of reviewed checkouts")
	flag.BoolVar(&live, "live", false, "run authenticated Claude Code and Codex discovery probes")
	flag.BoolVar(&acceptDrift, "accept-drift", false, "record explicit acceptance of all material latest-source drift")
	flag.Parse()

	ctx := context.Background()
	c, err := catalog.Load()
	must(err)
	resolved, cleanup, err := resolve(ctx, c, sourceRoot, latest)
	must(err)
	defer cleanup()
	accept := make(map[string]bool)
	for _, member := range resolved {
		if member.Drift.Material {
			if !acceptDrift {
				must(fmt.Errorf("%s has material drift; inspect it and rerun with --accept-drift", member.Member.Name))
			}
			accept[member.Member.Name] = true
		}
	}
	target, err := os.MkdirTemp("", "skillet-catalog-matrix-")
	must(err)
	must(os.Remove(target))
	defer os.RemoveAll(target)
	service := setup.NewService()
	if live {
		service = setup.NewServiceWith(setup.CommandProber{Timeout: 90 * time.Second, Concurrency: 4}, setup.ApplyHooks{})
	}
	request := setup.Request{TargetPath: target, CatalogVersion: c.Version, BundleIDs: c.BundleIDs(), Members: resolved, AcceptDrift: accept}
	plan, err := service.Plan(ctx, request)
	must(err)
	if len(plan.Blockers) != 0 {
		must(fmt.Errorf("matrix plan blocked: %v", plan.Blockers))
	}
	result, err := service.Apply(ctx, plan)
	must(err)

	var receipt setup.WorkspaceReceipt
	readJSON(filepath.Join(target, ".skillet", "workspace.json"), &receipt)
	var local setup.LocalReceipt
	readJSON(filepath.Join(target, ".skillet", "workspace.local.json"), &local)
	evidence := artifact{
		SchemaVersion: 1, CatalogVersion: c.Version, GovernanceBlockers: c.GovernanceBlockers(),
		Outcome: result.Outcome, Lanes: buildLanes(resolved, receipt, local),
	}
	if live {
		evidence.NegativeControls = setup.RunUnknownSkillControls(ctx, target)
	}
	repeat, err := service.Plan(ctx, request)
	must(err)
	evidence.RepeatNoOp = repeat.NoOp
	removalRequest := setup.Request{TargetPath: target, CatalogVersion: c.Version, BundleIDs: []string{}, Members: []setup.ResolvedMember{}, ReplaceConflicts: true}
	removalPlan, err := setup.NewService().Plan(ctx, removalRequest)
	must(err)
	_, err = setup.NewService().Apply(ctx, removalPlan)
	must(err)
	evidence.RemovalVerified = allRemoved(target, c)
	if err := validateEvidence(evidence, live); err != nil {
		must(err)
	}
	if len(evidence.Lanes) != 96 || !evidence.RepeatNoOp || !evidence.RemovalVerified {
		must(fmt.Errorf("incomplete matrix: lanes=%d repeatNoOp=%t removalVerified=%t", len(evidence.Lanes), evidence.RepeatNoOp, evidence.RemovalVerified))
	}
	bytes, err := json.MarshalIndent(evidence, "", "  ")
	must(err)
	must(os.MkdirAll(filepath.Dir(output), 0o755))
	must(os.WriteFile(output, append(bytes, '\n'), 0o644))
	fmt.Printf("wrote %d-lane catalog evidence to %s (outcome %s)\n", len(evidence.Lanes), output, evidence.Outcome)
}

func resolve(ctx context.Context, c catalog.Catalog, root string, latest bool) ([]setup.ResolvedMember, func(), error) {
	if latest {
		return (setup.GitResolver{}).ResolveMembers(ctx, c.Members)
	}
	if root == "" {
		return nil, func() {}, fmt.Errorf("--source-root is required unless --latest is used")
	}
	directories := map[string]string{
		"https://github.com/mattpocock/skills.git": "matt", "https://github.com/obra/superpowers.git": "superpowers",
		"https://github.com/vercel-labs/agent-skills.git": "vercel", "https://github.com/anthropics/skills.git": "anthropic",
		"https://github.com/dotnet/skills.git": "dotnet",
	}
	resolved := make([]setup.ResolvedMember, 0, len(c.Members))
	for _, member := range c.Members {
		item, err := setup.InspectBoundary(member, filepath.Join(root, directories[member.Source.Repository]), member.Source.ReviewedRevision)
		if err != nil {
			return nil, func() {}, err
		}
		item.Drift = setup.CompareDrift(member, item.Evidence)
		resolved = append(resolved, item)
	}
	return resolved, func() {}, nil
}

func buildLanes(resolved []setup.ResolvedMember, receipt setup.WorkspaceReceipt, local setup.LocalReceipt) []lane {
	resolvedByName := make(map[string]setup.ResolvedMember)
	for _, member := range resolved {
		resolvedByName[member.Member.Name] = member
	}
	toolResults := make(map[string]setup.ToolResult)
	for _, result := range local.ToolResults {
		toolResults[result.Tool] = result
	}
	var lanes []lane
	for _, member := range receipt.Members {
		resolvedMember := resolvedByName[member.Name]
		for _, view := range member.Views {
			toolResult := toolResults[view.Tool]
			laneDependencies := make([]setup.DependencyResult, 0)
			for _, dependency := range toolResult.Dependencies {
				if dependency.Member == member.Name {
					laneDependencies = append(laneDependencies, dependency)
				}
			}
			runtimeVerified := false
			probeReason := ""
			for _, probe := range toolResult.MemberProbes {
				if probe.Name == member.Name {
					runtimeVerified = probe.Discovered
					probeReason = probe.Reason
				}
			}
			if len(receipt.Members) == 0 {
				runtimeVerified = toolResult.RuntimeVerified
			}
			observed, observeErr := setup.ObservePlacedActivation(local.AbsoluteTarget, view)
			if observeErr != nil {
				observed = "invalid"
			}
			lanes = append(lanes, lane{
				Member: member.Name, Tool: view.Tool, Source: resolvedMember.Member.Source.Repository,
				SourceSubpath: resolvedMember.Member.Source.Subpath, ReviewedRevision: member.ReviewedRevision,
				ResolvedRevision: member.ResolvedRevision, ContentSHA256: member.ResolvedSHA256,
				Destination: view.RelativeDestination, Recipe: "project/direct-skill",
				RequestedActivation: member.Activation, ObservedActivation: observed,
				StaticVerified: toolResult.StaticVerified, Executable: filepath.Base(toolResult.Executable),
				Authenticated: toolResult.Authenticated, RuntimeVerified: runtimeVerified,
				ProbeReason:       probeReason,
				DependencyResults: laneDependencies, Warnings: view.Warnings,
			})
		}
	}
	sort.Slice(lanes, func(i, j int) bool {
		if lanes[i].Member == lanes[j].Member {
			return lanes[i].Tool < lanes[j].Tool
		}
		return lanes[i].Member < lanes[j].Member
	})
	return lanes
}

func validateEvidence(evidence artifact, live bool) error {
	if len(evidence.Lanes) != 96 || !evidence.RepeatNoOp || !evidence.RemovalVerified {
		return fmt.Errorf("incomplete matrix: lanes=%d repeatNoOp=%t removalVerified=%t", len(evidence.Lanes), evidence.RepeatNoOp, evidence.RemovalVerified)
	}
	for _, lane := range evidence.Lanes {
		if !lane.StaticVerified || lane.ObservedActivation != lane.RequestedActivation {
			return fmt.Errorf("lane %s/%s failed static or activation verification", lane.Member, lane.Tool)
		}
		for _, dependency := range lane.DependencyResults {
			if !dependency.Optional && !dependency.Ready {
				return fmt.Errorf("lane %s/%s missing required dependency %s", lane.Member, lane.Tool, dependency.Name)
			}
		}
		if live && (!lane.Authenticated || !lane.RuntimeVerified) {
			return fmt.Errorf("lane %s/%s failed authenticated runtime verification: %s", lane.Member, lane.Tool, lane.ProbeReason)
		}
	}
	if live {
		if evidence.Outcome != setup.OutcomeVerified || !evidence.NegativeControls["claude-code"] || !evidence.NegativeControls["codex"] {
			return fmt.Errorf("live matrix failed outcome or unknown-skill controls: outcome=%s controls=%v", evidence.Outcome, evidence.NegativeControls)
		}
	}
	return nil
}

func allRemoved(target string, c catalog.Catalog) bool {
	for _, member := range c.Members {
		for _, root := range []string{".claude/skills", ".agents/skills", ".skillet/managed/skills"} {
			if _, err := os.Stat(filepath.Join(target, root, member.Name, "SKILL.md")); !os.IsNotExist(err) {
				return false
			}
		}
	}
	return true
}

func readJSON(filename string, destination any) {
	bytes, err := os.ReadFile(filename)
	must(err)
	must(json.Unmarshal(bytes, destination))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
