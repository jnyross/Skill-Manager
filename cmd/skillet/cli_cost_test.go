package main

// Tests for `skillet cost` and the cost fields on the other JSON surfaces. Like
// the rest of cli_test.go these run entirely against a fixture home directory.

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCostReportsPerToolTotalsAndNamesItsExclusions(t *testing.T) {
	newFixture(t)

	code, stdout, stderr := runCLI(t, "cost")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}

	for _, want := range []string{
		"Every session (estimated)",
		"Claude Code",
		"Codex",
		"Most expensive Skills by description",
		"Estimates only",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("cost output is missing %q:\n%s", want, stdout)
		}
	}
	// The fixture has a Manual-only Personal Skill and a Codex prompt, so the
	// exclusion sentence must appear rather than the "none excluded" one.
	if !strings.Contains(stdout, "Skills are excluded") {
		t.Errorf("cost output does not explain its exclusions:\n%s", stdout)
	}
	if strings.Contains(stdout, "none are excluded") {
		t.Errorf("cost output claims nothing is excluded:\n%s", stdout)
	}
}

func TestCostJSONCarriesTotalsMethodAndRanking(t *testing.T) {
	newFixture(t)

	code, stdout, stderr := runCLI(t, "cost", "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}

	var document struct {
		SchemaVersion int `json:"schemaVersion"`
		Estimate      struct {
			Method        string `json:"method"`
			BytesPerToken int    `json:"bytesPerToken"`
			Exact         bool   `json:"exact"`
		} `json:"estimate"`
		PerSession struct {
			DescriptionTokens int `json:"descriptionTokens"`
			Skills            int `json:"skills"`
			ExcludedSkills    int `json:"excludedSkills"`
			ByTool            []struct {
				Tool              string `json:"tool"`
				Skills            int    `json:"skills"`
				DescriptionTokens int    `json:"descriptionTokens"`
			} `json:"byTool"`
		} `json:"perSession"`
		TopByDescriptionCost []struct {
			Name string `json:"name"`
			Cost struct {
				DescriptionTokens int `json:"descriptionTokens"`
				FileCount         int `json:"fileCount"`
			} `json:"cost"`
		} `json:"topByDescriptionCost"`
	}
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("cost --json is not valid JSON: %v\n%s", err, stdout)
	}

	if document.SchemaVersion != jsonSchemaVersion {
		t.Errorf("schemaVersion = %d, want %d", document.SchemaVersion, jsonSchemaVersion)
	}
	if document.Estimate.Exact {
		t.Error("cost --json claims its numbers are exact")
	}
	if document.Estimate.BytesPerToken != 4 || document.Estimate.Method == "" {
		t.Errorf("estimate block does not state the method: %+v", document.Estimate)
	}
	if document.PerSession.DescriptionTokens <= 0 || document.PerSession.Skills <= 0 {
		t.Errorf("perSession totals look empty: %+v", document.PerSession)
	}
	if document.PerSession.ExcludedSkills <= 0 {
		t.Errorf("the fixture has non-Auto Skills, so excludedSkills must be > 0: %+v", document.PerSession)
	}

	// The per-Tool rows must add up to the total, or the breakdown is a lie.
	total := 0
	for _, tool := range document.PerSession.ByTool {
		total += tool.DescriptionTokens
	}
	if total != document.PerSession.DescriptionTokens {
		t.Errorf("byTool sums to %d but the total says %d", total, document.PerSession.DescriptionTokens)
	}

	if len(document.TopByDescriptionCost) == 0 {
		t.Fatal("topByDescriptionCost is empty")
	}
	for index := 1; index < len(document.TopByDescriptionCost); index++ {
		previous := document.TopByDescriptionCost[index-1].Cost.DescriptionTokens
		current := document.TopByDescriptionCost[index].Cost.DescriptionTokens
		if current > previous {
			t.Fatalf("ranking is not descending: %+v", document.TopByDescriptionCost)
		}
	}
	if document.TopByDescriptionCost[0].Cost.FileCount == 0 {
		t.Error("ranked Skills should carry their measured on-disk footprint")
	}
}

// The cost fields are additive: they must not change the schema version other
// consumers pin.
func TestCostFieldsDoNotBumpTheSchemaVersion(t *testing.T) {
	newFixture(t)

	code, stdout, _ := runCLI(t, "list", "--json")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	var document struct {
		SchemaVersion int `json:"schemaVersion"`
		Skills        []struct {
			Name string `json:"name"`
			Cost struct {
				DescriptionTokens int   `json:"descriptionTokens"`
				BodyBytes         int64 `json:"bodyBytes"`
				BodyTokens        int   `json:"bodyTokens"`
				FileCount         int   `json:"fileCount"`
				TotalBytes        int64 `json:"totalBytes"`
			} `json:"cost"`
		} `json:"skills"`
	}
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("list --json is not valid JSON: %v", err)
	}
	if document.SchemaVersion != 1 {
		t.Errorf("schemaVersion = %d, want 1 — adding fields must not bump it", document.SchemaVersion)
	}
	for _, skill := range document.Skills {
		if skill.Cost.DescriptionTokens <= 0 || skill.Cost.BodyTokens <= 0 {
			t.Errorf("%s has no cost estimate: %+v", skill.Name, skill.Cost)
		}
		if skill.Cost.FileCount < 1 || skill.Cost.TotalBytes < skill.Cost.BodyBytes {
			t.Errorf("%s has no on-disk footprint: %+v", skill.Name, skill.Cost)
		}
	}
}

func TestShowPrintsCostAndExplainsAZeroPerSessionCost(t *testing.T) {
	newFixture(t)

	code, stdout, stderr := runCLI(t, "show", "Personal:writing")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Cost per session (est.)") || !strings.Contains(stdout, "injected into every Claude Code session") {
		t.Errorf("show does not explain the per-session cost:\n%s", stdout)
	}

	// The Personal "review" Skill declares disable-model-invocation, so it is
	// Manual-only and costs nothing per session.
	code, stdout, stderr = runCLI(t, "show", "Personal:review")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "~0 tokens (Manual-only") {
		t.Errorf("show does not report a Manual-only Skill as free per session:\n%s", stdout)
	}
	if !strings.Contains(stdout, "if set back to Auto-activation") {
		t.Errorf("show does not say what re-enabling would cost:\n%s", stdout)
	}
}

func TestCostRejectsArguments(t *testing.T) {
	newFixture(t)
	code, _, stderr := runCLI(t, "cost", "writing")
	if code != 2 {
		t.Fatalf("exit %d, want 2 for a usage error", code)
	}
	if !strings.Contains(stderr, "cost takes no arguments") {
		t.Errorf("stderr = %q", stderr)
	}
}
