package engine_test

// Tests for context-cost accounting (internal/engine/cost.go). The numbers are
// estimates by design, so these tests pin the two things that must be exact:
// the arithmetic of the shared estimator, and which Skills are counted in the
// aggregate.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

func TestEstimateTokens(t *testing.T) {
	t.Parallel()
	cases := []struct {
		bytes int64
		want  int
	}{
		{bytes: -10, want: 0},
		{bytes: 0, want: 0},
		{bytes: 1, want: 1},
		{bytes: 4, want: 1},
		{bytes: 5, want: 2},
		{bytes: 400, want: 100},
		{bytes: 4001, want: 1001},
	}
	for _, tc := range cases {
		if got := engine.EstimateTokens(tc.bytes); got != tc.want {
			t.Errorf("EstimateTokens(%d) = %d, want %d", tc.bytes, got, tc.want)
		}
	}
}

// TestFormatTokenEstimateAlwaysReadsAsAnEstimate is a product guarantee, not a
// formatting preference: a bare number would be read as a measurement.
func TestFormatTokenEstimateAlwaysReadsAsAnEstimate(t *testing.T) {
	t.Parallel()
	for _, tokens := range []int{-1, 0, 1, 999, 1234, 99999} {
		got := engine.FormatTokenEstimate(tokens)
		if !strings.HasPrefix(got, "~") {
			t.Errorf("FormatTokenEstimate(%d) = %q, want a leading ~", tokens, got)
		}
	}
	if got := engine.FormatTokenEstimate(1234); got != "~1,234" {
		t.Errorf("FormatTokenEstimate(1234) = %q, want ~1,234", got)
	}
	if got := engine.FormatTokenEstimate(12345); got != "~12.3k" {
		t.Errorf("FormatTokenEstimate(12345) = %q, want ~12.3k", got)
	}
}

func TestFormatByteSize(t *testing.T) {
	t.Parallel()
	cases := map[int64]string{
		0:               "0 B",
		512:             "512 B",
		2048:            "2.0 KB",
		3 * 1024 * 1024: "3.0 MB",
	}
	for bytes, want := range cases {
		if got := engine.FormatByteSize(bytes); got != want {
			t.Errorf("FormatByteSize(%d) = %q, want %q", bytes, got, want)
		}
	}
}

// TestInventoryPopulatesCostForEverySource is the fixture test: the scan must
// fill in the per-session and invoked cost of every Skill from all four
// Sources, plus Codex's file-shaped prompts.
func TestInventoryPopulatesCostForEverySource(t *testing.T) {
	f := newFixture(t)

	writeSkill(t, filepath.Join(f.roots.ClaudeHome, "skills", "personal-one"), "personal-one", "Personal skill description", "")
	writeSkill(t, filepath.Join(f.roots.CodexHome, "skills", "codex-one"), "codex-one", "Codex skill description", "")
	writePrompt(t, filepath.Join(f.roots.CodexHome, "prompts", "prompt-one.md"), "Codex prompt description")

	installPath := filepath.Join(f.root, "plugin-cache", "market", "plugin-x", "1.0.0")
	writeSkill(t, filepath.Join(installPath, "skills", "plugin-one"), "plugin-one", "Plugin skill description", "")
	writePluginManifest(t, f.roots.ClaudeHome, map[string][]map[string]string{
		"plugin-x@market": {{"scope": "user", "installPath": installPath}},
	})

	repo := filepath.Join(f.root, "repo")
	mkdirAll(t, filepath.Join(repo, ".git"))
	writeSkill(t, filepath.Join(repo, ".claude", "skills", "project-claude"), "project-claude", "Project Claude skill description", "")
	writeSkill(t, filepath.Join(repo, ".agents", "skills", "project-codex"), "project-codex", "Project Codex skill description", "")
	f.roots.ClaudeProjectRoots = engine.FindClaudeProjectRoots(repo)
	f.roots.ProjectRoots = engine.FindProjectRoots(repo)

	inv := engine.New(f.roots).Inventory()
	if len(inv.Skills) != 6 {
		t.Fatalf("fixture produced %d Skills, want 6: %#v", len(inv.Skills), inv.Skills)
	}

	for _, skill := range inv.Skills {
		if want := engine.EstimateTokens(int64(len(skill.Description))); skill.DescriptionTokens != want {
			t.Errorf("%s: DescriptionTokens = %d, want %d", skill.Name, skill.DescriptionTokens, want)
		}
		if skill.DescriptionTokens == 0 {
			t.Errorf("%s: DescriptionTokens is 0", skill.Name)
		}

		info, err := os.Stat(bodyPath(skill))
		if err != nil {
			t.Fatalf("stat %s: %v", bodyPath(skill), err)
		}
		if skill.BodyBytes != info.Size() {
			t.Errorf("%s: BodyBytes = %d, want the real file size %d", skill.Name, skill.BodyBytes, info.Size())
		}
		if want := engine.EstimateTokens(info.Size()); skill.BodyTokens != want {
			t.Errorf("%s: BodyTokens = %d, want %d", skill.Name, skill.BodyTokens, want)
		}

		// FileCount and TotalBytes are measured on demand, so the scan leaves
		// them at zero — see engine.MeasureSkillFiles.
		if skill.FileCount != 0 || skill.TotalBytes != 0 {
			t.Errorf("%s: Inventory() filled in the on-demand fields (%d files, %d bytes)", skill.Name, skill.FileCount, skill.TotalBytes)
		}

		measured := skill
		if notices := engine.MeasureSkillFiles(&measured); len(notices) != 0 {
			t.Errorf("%s: unexpected notices %#v", skill.Name, notices)
		}
		if measured.FileCount < 1 {
			t.Errorf("%s: MeasureSkillFiles left FileCount at %d", skill.Name, measured.FileCount)
		}
		if measured.TotalBytes < measured.BodyBytes {
			t.Errorf("%s: TotalBytes %d is less than BodyBytes %d", skill.Name, measured.TotalBytes, measured.BodyBytes)
		}
	}
}

func bodyPath(skill engine.Skill) string {
	if skill.Kind == engine.KindPrompt {
		return skill.Location
	}
	return filepath.Join(skill.Location, "SKILL.md")
}

// TestMeasureSkillFilesCountsTheWholeSkillDirectory covers the payload a real
// plugin Skill ships: references, scripts, and nested assets.
func TestMeasureSkillFilesCountsTheWholeSkillDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	folder := filepath.Join(dir, "big")
	writeSkill(t, folder, "big", "A skill with payload", "")
	writeFile(t, filepath.Join(folder, "references", "one.md"), strings.Repeat("a", 100))
	writeFile(t, filepath.Join(folder, "references", "deep", "two.md"), strings.Repeat("b", 200))
	writeFile(t, filepath.Join(folder, "scripts", "run.sh"), strings.Repeat("c", 300))

	body, err := os.Stat(filepath.Join(folder, "SKILL.md"))
	if err != nil {
		t.Fatalf("stat body: %v", err)
	}

	skill := engine.Skill{Name: "big", Kind: engine.KindSkill, Location: folder, BodyBytes: body.Size()}
	if notices := engine.MeasureSkillFiles(&skill); len(notices) != 0 {
		t.Fatalf("unexpected notices: %#v", notices)
	}
	if skill.FileCount != 4 {
		t.Errorf("FileCount = %d, want 4 (SKILL.md + 3 payload files)", skill.FileCount)
	}
	if want := body.Size() + 600; skill.TotalBytes != want {
		t.Errorf("TotalBytes = %d, want %d", skill.TotalBytes, want)
	}
}

// TestMeasureSkillFilesStaysInsideTheSkillDirectory pins the scoping rule: a
// Skill folder is its own payload, and a symlink inside it is not an invitation
// to count someone else's tree.
func TestMeasureSkillFilesStaysInsideTheSkillDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	folder := filepath.Join(dir, "linked")
	writeSkill(t, folder, "linked", "A skill with a symlink", "")

	outside := filepath.Join(dir, "outside")
	writeFile(t, filepath.Join(outside, "huge.bin"), strings.Repeat("x", 10_000))
	if err := os.Symlink(outside, filepath.Join(folder, "elsewhere")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	// A sibling directory must not be reached either.
	writeFile(t, filepath.Join(dir, "sibling.md"), strings.Repeat("y", 5_000))

	skill := engine.Skill{Name: "linked", Kind: engine.KindSkill, Location: folder}
	engine.MeasureSkillFiles(&skill)
	if skill.FileCount != 1 {
		t.Errorf("FileCount = %d, want 1 — the symlink and the sibling must not be counted", skill.FileCount)
	}
	if skill.TotalBytes > 1_000 {
		t.Errorf("TotalBytes = %d, which means something outside the Skill directory was counted", skill.TotalBytes)
	}
}

// TestMeasureSkillFilesMeasuresAPromptAsOneFile documents the Codex prompt
// choice: a prompt has no directory, so its file is its whole footprint.
func TestMeasureSkillFilesMeasuresAPromptAsOneFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "review.md")
	writePrompt(t, path, "A prompt")

	skill := engine.Skill{Name: "review", Kind: engine.KindPrompt, Location: path, BodyBytes: 321}
	engine.MeasureSkillFiles(&skill)
	if skill.FileCount != 1 {
		t.Errorf("FileCount = %d, want 1", skill.FileCount)
	}
	if skill.TotalBytes != 321 {
		t.Errorf("TotalBytes = %d, want the body size 321", skill.TotalBytes)
	}
}

// TestOversizedSkillRaisesANotice covers the 2 MB cap: past it Skillet says the
// estimate is not to be trusted rather than printing a confident number.
func TestOversizedSkillRaisesANotice(t *testing.T) {
	f := newFixture(t)
	folder := filepath.Join(f.roots.ClaudeHome, "skills", "huge")
	mkdirAll(t, folder)
	header := "---\nname: \"huge\"\ndescription: \"A huge skill\"\n---\n"
	if err := os.WriteFile(filepath.Join(folder, "SKILL.md"), []byte(header+strings.Repeat("z", 2<<20)), 0o644); err != nil {
		t.Fatalf("write huge skill: %v", err)
	}

	inv := engine.New(f.roots).Inventory()
	if !noticesContain(inv, "past Skillet's 2.0 MB cost-estimate cap") {
		t.Fatalf("no over-cap notice: %#v", inv.Notices)
	}
	skill, ok := findSkill(inv, engine.SourcePersonal, "huge")
	if !ok {
		t.Fatalf("huge skill missing from inventory")
	}
	if skill.BodyTokens == 0 {
		t.Errorf("an over-cap Skill still needs a cost estimate, got 0")
	}
}

func TestSummarizeContextCostCountsOnlyAutoActivation(t *testing.T) {
	t.Parallel()
	skills := []engine.Skill{
		{Name: "a", Tool: engine.ToolClaudeCode, Activation: engine.ActivationAuto, DescriptionTokens: 100},
		{Name: "b", Tool: engine.ToolClaudeCode, Activation: engine.ActivationAuto, DescriptionTokens: 25},
		{Name: "c", Tool: engine.ToolClaudeCode, Activation: engine.ActivationManualOnly, DescriptionTokens: 1000},
		{Name: "d", Tool: engine.ToolClaudeCode, Activation: engine.ActivationSuppressed, DescriptionTokens: 1000},
		{Name: "e", Tool: engine.ToolCodex, Activation: engine.ActivationAuto, DescriptionTokens: 40},
		{Name: "f", Tool: engine.ToolCodex, Activation: engine.ActivationDisabled, DescriptionTokens: 1000},
	}

	summary := engine.SummarizeContextCost(skills)
	if summary.DescriptionTokens != 165 {
		t.Errorf("total = %d, want 165 (only the Auto Skills)", summary.DescriptionTokens)
	}
	if summary.Skills != 3 {
		t.Errorf("counted %d Skills, want 3", summary.Skills)
	}
	if summary.Excluded != 3 {
		t.Errorf("excluded %d Skills, want 3", summary.Excluded)
	}
	if len(summary.ByTool) != 2 {
		t.Fatalf("ByTool = %#v, want two Tools", summary.ByTool)
	}
	if summary.ByTool[0].Tool != engine.ToolClaudeCode || summary.ByTool[0].DescriptionTokens != 125 {
		t.Errorf("Claude Code row = %#v, want 125 tokens first", summary.ByTool[0])
	}
	if summary.ByTool[1].Tool != engine.ToolCodex || summary.ByTool[1].DescriptionTokens != 40 {
		t.Errorf("Codex row = %#v, want 40 tokens", summary.ByTool[1])
	}
}

func TestSortByDescriptionCost(t *testing.T) {
	t.Parallel()
	skills := []engine.Skill{
		{Name: "cheap", DescriptionTokens: 5},
		{Name: "beta", DescriptionTokens: 50},
		{Name: "alpha", DescriptionTokens: 50},
		{Name: "dear", DescriptionTokens: 500},
	}
	sorted := engine.SortByDescriptionCost(skills)
	got := []string{sorted[0].Name, sorted[1].Name, sorted[2].Name, sorted[3].Name}
	want := []string{"dear", "alpha", "beta", "cheap"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
	if skills[0].Name != "cheap" {
		t.Errorf("SortByDescriptionCost mutated the caller's slice: %#v", skills)
	}
}
