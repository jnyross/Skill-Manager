package setup_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/catalog"
	"github.com/jnyross/Skill-Manager/internal/setup"
)

func TestReviewedCatalogStaticMatrixInBothTools(t *testing.T) {
	sourceRoot := os.Getenv("SKILLET_CATALOG_SOURCE_ROOT")
	if sourceRoot == "" {
		t.Skip("set SKILLET_CATALOG_SOURCE_ROOT to the five reviewed source checkouts")
	}
	c, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	repositories := map[string]string{
		"https://github.com/mattpocock/skills.git":        "matt",
		"https://github.com/obra/superpowers.git":         "superpowers",
		"https://github.com/vercel-labs/agent-skills.git": "vercel",
		"https://github.com/anthropics/skills.git":        "anthropic",
		"https://github.com/dotnet/skills.git":            "dotnet",
	}
	resolved := make([]setup.ResolvedMember, 0, len(c.Members))
	for _, member := range c.Members {
		repositoryRoot := filepath.Join(sourceRoot, repositories[member.Source.Repository])
		item, err := setup.InspectBoundary(member, repositoryRoot, member.Source.ReviewedRevision)
		if err != nil {
			t.Fatalf("%s: %v", member.Name, err)
		}
		item.Drift = setup.CompareDrift(member, item.Evidence)
		if item.Drift.Material {
			t.Fatalf("reviewed source for %s drifted from embedded evidence: %#v", member.Name, item.Drift)
		}
		resolved = append(resolved, item)
	}

	target := filepath.Join(t.TempDir(), "matrix workspace")
	service := setup.NewService()
	request := setup.Request{TargetPath: target, CatalogVersion: c.Version, BundleIDs: c.BundleIDs(), Members: resolved}
	plan, err := service.Plan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Apply(context.Background(), plan)
	if err != nil || result.Outcome != setup.OutcomeConfiguredUnverified {
		t.Fatalf("matrix result = %#v, err=%v", result, err)
	}
	for _, member := range c.Members {
		for _, relative := range []string{
			filepath.Join(".claude", "skills", member.Name, "SKILL.md"),
			filepath.Join(".agents", "skills", member.Name, "SKILL.md"),
		} {
			if _, err := os.Stat(filepath.Join(target, relative)); err != nil {
				t.Errorf("%s lane missing: %v", relative, err)
			}
		}
	}
	repeat, err := service.Plan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !repeat.NoOp {
		t.Fatalf("48-member repeat setup is not a no-op: blockers=%v", repeat.Blockers)
	}
}
