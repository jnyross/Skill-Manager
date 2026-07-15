package catalog_test

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/catalog"
)

func TestBuiltInCatalogHasExactApprovedMembership(t *testing.T) {
	c, err := catalog.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{
		"ask-matt", "code-review", "codebase-design", "diagnosing-bugs", "domain-modeling",
		"grill-me", "grill-with-docs", "grilling", "handoff", "implement",
		"improve-codebase-architecture", "prototype", "research", "setup-matt-pocock-skills",
		"tdd", "teach", "to-spec", "to-tickets", "triage", "wayfinder", "writing-great-skills",
		"brainstorming", "dispatching-parallel-agents", "executing-plans", "finishing-a-development-branch",
		"receiving-code-review", "requesting-code-review", "subagent-driven-development", "systematic-debugging",
		"test-driven-development", "using-git-worktrees", "using-superpowers", "verification-before-completion",
		"writing-plans", "writing-skills",
		"web-design-guidelines", "vercel-react-best-practices", "vercel-composition-patterns", "writing-guidelines",
		"skill-creator", "frontend-design", "webapp-testing",
		"setup-local-sdk", "dotnet-webapi", "optimizing-ef-core-queries", "analyzing-dotnet-performance",
		"run-tests", "writing-mstest-tests",
	}
	got := make([]string, 0, len(c.Members))
	for _, member := range c.Members {
		got = append(got, member.Name)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("catalog membership = %#v, want %#v", got, want)
	}
}

func TestCatalogSelectionIsSideEffectFreeAndExplicit(t *testing.T) {
	c, err := catalog.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	selected, err := c.SelectBundles([]string{"anthropic-ui"})
	if err != nil {
		t.Fatalf("SelectBundles: %v", err)
	}
	if len(selected.Members) != 3 {
		t.Fatalf("selected members = %d, want 3", len(selected.Members))
	}
	if len(c.Members) != 48 {
		t.Fatalf("selection mutated catalog: %d members", len(c.Members))
	}
	if _, err := c.SelectBundles([]string{"not-a-bundle"}); err == nil {
		t.Fatal("unknown bundle accepted")
	}
}

func TestCatalogGovernanceMetadataIsComplete(t *testing.T) {
	c, err := catalog.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, member := range c.Members {
		if member.Source.Repository == "" || member.Source.Subpath == "" || len(member.Source.ReviewedRevision) != 40 {
			t.Errorf("%s has incomplete source evidence: %#v", member.Name, member.Source)
		}
		if member.Source.ContentSHA256 == "" || member.License.SPDX == "" || member.License.Notice == "" {
			t.Errorf("%s has incomplete content/license evidence", member.Name)
		}
		if member.UpstreamActivation == "" || member.VerificationPrompt == "" {
			t.Errorf("%s has incomplete activation/probe evidence", member.Name)
		}
		if len(member.Recipes) != 2 || member.Recipes[0].Scope != "project" || member.Recipes[1].Scope != "project" {
			t.Errorf("%s recipes = %#v, want two Project direct-skill recipes", member.Name, member.Recipes)
		}
	}
}

func TestTrackerPublishingSkillsDiscloseNetworkCLIAndSideEffects(t *testing.T) {
	c, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"setup-matt-pocock-skills", "to-spec", "to-tickets", "triage", "wayfinder"} {
		var found *catalog.Member
		for index := range c.Members {
			if c.Members[index].Name == name {
				found = &c.Members[index]
				break
			}
		}
		if found == nil {
			t.Fatalf("missing %s", name)
		}
		dependencies := make(map[string]bool)
		for _, dependency := range found.Dependencies {
			dependencies[dependency.Name] = true
		}
		if !dependencies["gh"] || !dependencies["network"] || len(found.ExternalActions) == 0 {
			t.Errorf("%s disclosures = dependencies %v actions %v", name, found.Dependencies, found.ExternalActions)
		}
	}
}

func TestDotnetBundleRecommendationRequiresObservableProjectSignal(t *testing.T) {
	c, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if got := c.RecommendedBundleIDs(root); len(got) != 0 {
		t.Fatalf("empty project recommendations = %v", got)
	}
	if err := os.WriteFile(filepath.Join(root, "App.csproj"), []byte("<Project />\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := c.RecommendedBundleIDs(root)
	if !reflect.DeepEqual(got, []string{"dotnet-starter"}) {
		t.Fatalf(".NET project recommendations = %v", got)
	}
}
