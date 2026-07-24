package engine

// Context-cost accounting (WP5).
//
// Skillet exists because context is finite. Every Auto-activating Skill's
// description is injected into every session with the Tool, whether or not the
// Skill is ever used — a standing cost the user could not see before this file
// existed. These helpers put a number on it.
//
// Everything here is an ESTIMATE and must be labelled as one wherever it
// surfaces. Skillet deliberately ships no tokenizer: the real token count
// depends on the model, and pulling a tokenizer in would add a dependency and
// a per-scan cost far larger than the accuracy is worth. Ranking Skills against
// each other correctly matters much more than exactness, and a bytes-per-token
// constant ranks them the same way a real tokenizer would for ordinary prose.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const (
	// bytesPerTokenEstimate is the divisor behind every token number Skillet
	// shows. Roughly four bytes of English prose or Markdown per token is the
	// widely used rule of thumb for current tokenizers.
	bytesPerTokenEstimate = 4

	// bodyCostCapBytes bounds what Skillet is willing to treat as an ordinary
	// SKILL.md. Past it the file is anomalous rather than a Skill body, the
	// bytes-per-token rule of thumb stops being trustworthy (a 2 MB file is
	// almost certainly embedded data, not prose), and the Skill earns a Notice
	// so the estimate is never quietly believed.
	bodyCostCapBytes = 2 << 20

	// costWalkFileBudget bounds the per-Skill directory walk. A Skill folder
	// holding more files than this has something in it that is not skill
	// payload (a vendored dependency tree, a checkout); Skillet stops counting
	// and says so rather than turning every refresh into a full tree walk.
	costWalkFileBudget = 5000
)

// TokenEstimateBytesPerToken is the constant behind EstimateTokens, exported so
// a machine-readable surface can state the method rather than making a consumer
// reverse-engineer it from the numbers.
const TokenEstimateBytesPerToken = bytesPerTokenEstimate

// EstimateTokens converts a byte count to an estimated token count. It is the
// single estimator behind every cost number in Skillet — the TUI, the CLI, and
// the JSON surface all go through it, so they can never disagree.
func EstimateTokens(byteCount int64) int {
	if byteCount <= 0 {
		return 0
	}
	return int((byteCount + bytesPerTokenEstimate - 1) / bytesPerTokenEstimate)
}

// FormatTokenEstimate renders a token estimate for people. The leading "~" is
// part of the contract: no caller may print a bare token number as if it were
// exact.
func FormatTokenEstimate(tokens int) string {
	switch {
	case tokens < 0:
		return "~0"
	case tokens < 10_000:
		return "~" + groupThousands(tokens)
	default:
		return fmt.Sprintf("~%.1fk", float64(tokens)/1000)
	}
}

// FormatByteSize renders a file size for people.
func FormatByteSize(bytes int64) string {
	switch {
	case bytes < 0:
		return "0 B"
	case bytes < 1024:
		return fmt.Sprintf("%d B", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	}
}

func groupThousands(value int) string {
	text := fmt.Sprintf("%d", value)
	if len(text) <= 3 {
		return text
	}
	var out []byte
	for index, digit := range []byte(text) {
		if index > 0 && (len(text)-index)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, digit)
	}
	return string(out)
}

// applyBodyCost records what one Skill costs, using the size of the SKILL.md
// (or prompt file) that the scan has *already opened* to read its frontmatter —
// see parseFrontmatter, which fstats the open handle. The whole eager cost pass
// is therefore arithmetic plus one fstat per Skill: no extra file is opened, no
// directory is walked, and a refresh costs what it did before Skillet knew
// anything about cost.
//
// bodyBytes is 0 when the size could not be determined, which reads as an
// unknown cost rather than a free one — the fields simply stay zero.
func applyBodyCost(skill *Skill, bodyBytes int64) []Notice {
	skill.DescriptionTokens = EstimateTokens(int64(len(skill.Description)))
	skill.BodyBytes = bodyBytes
	skill.BodyTokens = EstimateTokens(bodyBytes)
	if bodyBytes > bodyCostCapBytes {
		return []Notice{{Message: fmt.Sprintf(
			"%s: %s is %s, past Skillet's %s cost-estimate cap — treat its cost estimate as a rough lower bound",
			skill.Name, bodyPathFor(*skill), FormatByteSize(bodyBytes), FormatByteSize(bodyCostCapBytes))}}
	}
	return nil
}

// skillBodyFileName is the file whose size is a Skill's invoked cost. Claude
// Code and Codex both use the same name for a directory-shaped Skill.
const skillBodyFileName = "SKILL.md"

// bodyPathFor is the file a Skill's invoked cost is measured on: the prompt
// file itself for a Codex prompt, the SKILL.md inside the folder otherwise.
func bodyPathFor(skill Skill) string {
	if skill.Kind == KindPrompt {
		return skill.Location
	}
	return filepath.Join(skill.Location, skillBodyFileName)
}

// MeasureSkillFiles fills in FileCount and TotalBytes — what the Skill occupies
// on disk, references and scripts and assets included.
//
// This is deliberately NOT part of Inventory(). Those two fields are the only
// cost numbers that cannot be had without walking the Skill's directory, and on
// a realistic inventory that walk costs more than the entire rest of the scan
// (measured: +1.7 ms against a 1.6 ms scan on the 50-Skill benchmark fixture,
// dominated by one lstat per payload file). Since they are supporting detail —
// shown for the Skill the user is looking at, and by the one-shot CLI — Skillet
// measures them for the Skills it is about to show instead of for every Skill on
// every refresh. The cost numbers that drive behaviour, DescriptionTokens and
// BodyTokens, are always populated by the scan.
//
// The walk is scoped to the Skill's own directory: a Skill's folder is its
// payload, never a container of other Skills. Symlinks are not followed, so a
// Skill can neither walk itself into a loop nor count a whole target tree as its
// own. It is bounded by costWalkFileBudget; past that the counts are a lower
// bound and the returned Notice says so.
func MeasureSkillFiles(skill *Skill) []Notice {
	if skill == nil {
		return nil
	}
	if skill.Kind == KindPrompt {
		// A Codex prompt is a single file, so its "directory" is exactly that
		// one file. The fields keep their meaning rather than staying zero.
		skill.FileCount = 1
		skill.TotalBytes = skill.BodyBytes
		return nil
	}

	var fileCount int
	var totalBytes int64
	budgetHit := false
	stack := []string{skill.Location}
	for len(stack) > 0 && !budgetHit {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		entries, err := os.ReadDir(current)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				stack = append(stack, filepath.Join(current, entry.Name()))
				continue
			}
			if fileCount >= costWalkFileBudget {
				budgetHit = true
				break
			}
			isBody := current == skill.Location && entry.Name() == skillBodyFileName
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if !info.Mode().IsRegular() {
				if !isBody {
					continue
				}
				// Keeping a Skill under version control elsewhere and symlinking
				// its SKILL.md is ordinary; only that one file pays for the
				// second stat needed to size it.
				resolved, statErr := os.Stat(filepath.Join(current, entry.Name()))
				if statErr != nil {
					continue
				}
				info = resolved
			}
			fileCount++
			totalBytes += info.Size()
		}
	}

	skill.FileCount = fileCount
	skill.TotalBytes = totalBytes
	if budgetHit {
		return []Notice{{Message: fmt.Sprintf(
			"%s: stopped counting files at %d — its size on disk is a lower bound", skill.Name, costWalkFileBudget)}}
	}
	return nil
}

// MeasureAllSkillFiles is MeasureSkillFiles over a whole slice, for the
// one-shot CLI, where paying for every Skill's walk once is the right trade.
func MeasureAllSkillFiles(skills []Skill) []Notice {
	var notices []Notice
	for index := range skills {
		notices = append(notices, MeasureSkillFiles(&skills[index])...)
	}
	return notices
}

// ToolCost is one Tool's share of the standing per-session cost.
type ToolCost struct {
	Tool              Tool
	Skills            int
	DescriptionTokens int
}

// ContextCost is the aggregate Skillet leads with: what Auto-activation costs
// the user in every single session, per Tool.
//
// Only ActivationAuto Skills count. A Manual-only, Disabled, or Suppressed
// Skill is not offered to the model on its own judgement, so its description is
// not part of the standing cost — and Excluded records how many Skills that is,
// because an aggregate whose exclusions are invisible invites the wrong
// conclusion.
type ContextCost struct {
	ByTool            []ToolCost
	Skills            int
	DescriptionTokens int
	Excluded          int
}

// SummarizeContextCost aggregates the per-session description cost of skills.
// The per-Tool slice is ordered Claude Code first, then Codex, then anything
// else alphabetically, so the header and the CLI never reorder between runs.
func SummarizeContextCost(skills []Skill) ContextCost {
	var summary ContextCost
	byTool := make(map[Tool]*ToolCost)
	for _, skill := range skills {
		if skill.Activation != ActivationAuto {
			summary.Excluded++
			continue
		}
		summary.Skills++
		summary.DescriptionTokens += skill.DescriptionTokens
		entry, ok := byTool[skill.Tool]
		if !ok {
			entry = &ToolCost{Tool: skill.Tool}
			byTool[skill.Tool] = entry
		}
		entry.Skills++
		entry.DescriptionTokens += skill.DescriptionTokens
	}
	for _, entry := range byTool {
		summary.ByTool = append(summary.ByTool, *entry)
	}
	sort.SliceStable(summary.ByTool, func(i, j int) bool {
		return toolSortOrder(summary.ByTool[i].Tool) < toolSortOrder(summary.ByTool[j].Tool)
	})
	return summary
}

func toolSortOrder(tool Tool) int {
	switch tool {
	case ToolClaudeCode:
		return 0
	case ToolCodex:
		return 1
	default:
		return 2
	}
}

// SortByDescriptionCost orders skills by what they cost every session, most
// expensive first, with name as the tiebreak so equal-cost Skills keep a stable
// order. It sorts a copy: the caller's slice, and the inventory order the rest
// of Skillet relies on, are untouched.
func SortByDescriptionCost(skills []Skill) []Skill {
	sorted := append([]Skill(nil), skills...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].DescriptionTokens != sorted[j].DescriptionTokens {
			return sorted[i].DescriptionTokens > sorted[j].DescriptionTokens
		}
		return sorted[i].Name < sorted[j].Name
	})
	return sorted
}
