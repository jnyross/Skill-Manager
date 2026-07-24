package main

// skillet cost — what the installed Skills cost in context.
//
// The headline is the standing per-session cost: while a Skill Auto-activates,
// its description is injected into every session with its Tool whether the
// Skill is ever used or not. That is the number that changes behaviour, so it
// leads, it is broken down per Tool, and it states which Skills it leaves out.
// The top-10 ranking below it answers the obvious follow-up question — which
// Skills are actually responsible.
//
// Every number is an estimate (engine.EstimateTokens). The output says so in
// words, and every token figure carries a "~".

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

// costTopN is how many Skills the ranking shows. Ten is enough to find the
// expensive ones without turning the command into a second "skillet list".
const costTopN = 10

type costEstimateJSON struct {
	// Method and BytesPerToken state how the numbers were produced, so a
	// consumer never has to guess whether they are exact.
	Method        string `json:"method"`
	BytesPerToken int    `json:"bytesPerToken"`
	Exact         bool   `json:"exact"`
}

type toolCostJSON struct {
	Tool              string `json:"tool"`
	Skills            int    `json:"skills"`
	DescriptionTokens int    `json:"descriptionTokens"`
}

type perSessionCostJSON struct {
	// DescriptionTokens is the total injected into every session by
	// Auto-activating Skills. Skills counts those Skills; ExcludedSkills counts
	// the Manual-only, Disabled, and Suppressed Skills left out of the total.
	DescriptionTokens int            `json:"descriptionTokens"`
	Skills            int            `json:"skills"`
	ExcludedSkills    int            `json:"excludedSkills"`
	ByTool            []toolCostJSON `json:"byTool"`
}

type costJSONDocument struct {
	SchemaVersion int                `json:"schemaVersion"`
	Estimate      costEstimateJSON   `json:"estimate"`
	PerSession    perSessionCostJSON `json:"perSession"`
	// TopByDescriptionCost is the ranking, most expensive first, across every
	// Skill — including the ones excluded from the per-session total, because
	// "this Manual-only Skill would cost a lot" is useful to know.
	TopByDescriptionCost []skillJSON  `json:"topByDescriptionCost"`
	Notices              []noticeJSON `json:"notices"`
}

func runCost(args []string, stdout, stderr io.Writer) int {
	cmd := newCommand("skillet cost", "usage: skillet cost [--json]",
		"Estimates what the installed Skills cost in context: the standing per-session\ncost of Auto-activation, per Tool, and the Skills responsible for most of it.\nEvery number is an estimate — Skillet sizes files rather than tokenizing them.")
	asJSON := cmd.flags.Bool("json", false, "emit the cost report as JSON")
	if code, done := cmd.parse(args, stdout, stderr); done {
		return code
	}
	if cmd.flags.NArg() != 0 {
		return fail(stderr, usagef("cost takes no arguments (got %q)", cmd.flags.Arg(0)))
	}

	e, err := newEngineForCWD()
	if err != nil {
		return fail(stderr, err)
	}
	inventory := e.Inventory()
	notices := inventory.Notices
	summary := engine.SummarizeContextCost(inventory.Skills)

	top := engine.SortByDescriptionCost(inventory.Skills)
	if len(top) > costTopN {
		top = top[:costTopN]
	}
	// Only the Skills actually printed pay for a directory walk.
	notices = append(notices, engine.MeasureAllSkillFiles(top)...)

	if *asJSON {
		document := costJSONDocument{
			SchemaVersion: jsonSchemaVersion,
			Estimate: costEstimateJSON{
				Method:        fmt.Sprintf("bytes/%d", engine.TokenEstimateBytesPerToken),
				BytesPerToken: engine.TokenEstimateBytesPerToken,
				Exact:         false,
			},
			PerSession:           newPerSessionCostJSON(summary),
			TopByDescriptionCost: newSkillsJSON(top),
			Notices:              newNoticesJSON(notices),
		}
		if err := writeJSON(stdout, document); err != nil {
			return fail(stderr, err)
		}
		return 0
	}

	printCostReport(stdout, summary, top)
	printNotices(stdout, notices)
	return 0
}

func newPerSessionCostJSON(summary engine.ContextCost) perSessionCostJSON {
	byTool := make([]toolCostJSON, 0, len(summary.ByTool))
	for _, tool := range summary.ByTool {
		byTool = append(byTool, toolCostJSON{
			Tool:              string(tool.Tool),
			Skills:            tool.Skills,
			DescriptionTokens: tool.DescriptionTokens,
		})
	}
	return perSessionCostJSON{
		DescriptionTokens: summary.DescriptionTokens,
		Skills:            summary.Skills,
		ExcludedSkills:    summary.Excluded,
		ByTool:            byTool,
	}
}

func printCostReport(w io.Writer, summary engine.ContextCost, top []engine.Skill) {
	fmt.Fprintln(w, "Every session (estimated)")
	fmt.Fprintln(w, "Auto-activating Skills inject their description into every session with their")
	fmt.Fprintln(w, "Tool, used or not. That standing cost is:")
	fmt.Fprintln(w)

	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "  TOOL\tSKILLS\tTOKENS")
	for _, tool := range summary.ByTool {
		fmt.Fprintf(table, "  %s\t%d\t%s\n", tool.Tool, tool.Skills, engine.FormatTokenEstimate(tool.DescriptionTokens))
	}
	if len(summary.ByTool) > 1 {
		fmt.Fprintf(table, "  %s\t%d\t%s\n", "Total", summary.Skills, engine.FormatTokenEstimate(summary.DescriptionTokens))
	}
	if len(summary.ByTool) == 0 {
		fmt.Fprintln(table, "  (nothing Auto-activates)\t0\t~0")
	}
	_ = table.Flush()

	fmt.Fprintln(w)
	if summary.Excluded > 0 {
		fmt.Fprintf(w, "%d Skills are excluded: Manual-only, Disabled, and Suppressed Skills are not\noffered to the model on its own judgement, so they cost nothing per session.\n", summary.Excluded)
	} else {
		fmt.Fprintln(w, "Every Skill Auto-activates, so none are excluded from that total.")
	}

	if len(top) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Most expensive Skills by description (estimated, top %d)\n", len(top))
		ranking := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(ranking, "  TOKENS\tACTIVATION\tSOURCE\tTOOL\tNAME\tON DISK")
		for _, skill := range top {
			fmt.Fprintf(ranking, "  %s\t%s\t%s\t%s\t%s\t%s\n",
				engine.FormatTokenEstimate(skill.DescriptionTokens), skill.Activation, skill.Source, skill.Tool,
				skill.Name, engine.FormatByteSize(skill.TotalBytes))
		}
		_ = ranking.Flush()
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "Estimates only: Skillet sizes files (about %d bytes per token) rather than\nrunning a tokenizer, so treat these as a ranking, not a measurement.\n",
		engine.TokenEstimateBytesPerToken)
}
