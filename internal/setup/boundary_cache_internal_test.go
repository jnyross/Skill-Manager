package setup

// Boundary-read cost tests. These install a package-global counting hook and
// must not be made parallel.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/catalog"
)

func countBoundaryReads(t *testing.T) map[string]int {
	t.Helper()
	counts := make(map[string]int)
	boundaryReadHook = func(root string) { counts[root]++ }
	t.Cleanup(func() { boundaryReadHook = nil })
	return counts
}

func costMember(t *testing.T, name string) ResolvedMember {
	t.Helper()
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: Probe\n---\nBody\n"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(source, "agents"), 0o755); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "agents", "openai.yaml"), []byte("policy:\n  allow_implicit_invocation: true\n"), 0o644); err != nil {
		t.Fatalf("write openai.yaml: %v", err)
	}
	return ResolvedMember{
		Member: catalog.Member{
			Name: name, Family: "fixture", UpstreamActivation: "auto",
			Recipes: []catalog.Recipe{
				{Tool: "claude-code", Scope: "project", Artifact: "direct-skill"},
				{Tool: "codex", Scope: "project", Artifact: "direct-skill"},
			},
		},
		SourceDir: source,
	}
}

// TestPlanReadsEachSourceBoundaryOnce pins requirement 6: Plan itself read
// every member's source boundary, and then each of the two tool adapters read
// the very same directory again — three full walks and three full slurps per
// member, per Plan call.
func TestPlanReadsEachSourceBoundaryOnce(t *testing.T) {
	members := []ResolvedMember{costMember(t, "alpha"), costMember(t, "beta")}
	counts := countBoundaryReads(t)

	service := NewService()
	if _, err := service.Plan(context.Background(), Request{
		TargetPath: filepath.Join(t.TempDir(), "workspace"), CatalogVersion: "test.1",
		BundleIDs: []string{"fixture-bundle"}, Members: members,
	}); err != nil {
		t.Fatalf("Plan: %v", err)
	}

	for _, member := range members {
		if got := counts[member.SourceDir]; got != 1 {
			t.Fatalf("%s source boundary read %d times in one Plan, want 1", member.Member.Name, got)
		}
	}
}

// TestRunScopedCacheSurvivesReplanning covers the plan-twice path in
// RunTerminal: a conflict prompt re-plans the same request, which used to
// re-read every member from disk a second time.
func TestRunScopedCacheSurvivesReplanning(t *testing.T) {
	members := []ResolvedMember{costMember(t, "alpha")}
	counts := countBoundaryReads(t)

	service := NewService()
	request := Request{
		TargetPath: filepath.Join(t.TempDir(), "workspace"), CatalogVersion: "test.1",
		BundleIDs: []string{"fixture-bundle"}, Members: members,
		boundaries: newBoundaryCache(),
	}
	for i := 0; i < 2; i++ {
		if _, err := service.Plan(context.Background(), request); err != nil {
			t.Fatalf("Plan %d: %v", i, err)
		}
	}
	if got := counts[members[0].SourceDir]; got != 1 {
		t.Fatalf("source boundary read %d times across two Plans, want 1", got)
	}
}

// TestBoundaryCacheHandsOutIndependentMaps guards the memoization itself: a
// caller that mutates the maps it was given (Render does, when applying an
// activation override) must not corrupt what the next caller sees.
func TestBoundaryCacheHandsOutIndependentMaps(t *testing.T) {
	member := costMember(t, "alpha")
	cache := newBoundaryCache()
	first, firstModes, err := cache.read(member.SourceDir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	delete(first, "SKILL.md")
	firstModes["agents/openai.yaml"] = 0o777

	second, secondModes, err := cache.read(member.SourceDir)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if _, ok := second["SKILL.md"]; !ok {
		t.Fatal("a previous caller's delete leaked into the cached boundary")
	}
	if secondModes["agents/openai.yaml"] == 0o777 {
		t.Fatal("a previous caller's mode write leaked into the cached boundary")
	}
}

// TestBoundaryCacheDoesNotCacheDestinationVerification is the safety half of
// requirement 6: verification re-reads what was actually placed on disk, and
// must never be served from the source-boundary cache.
func TestBoundaryCacheDoesNotCacheDestinationVerification(t *testing.T) {
	member := costMember(t, "alpha")
	target := filepath.Join(t.TempDir(), "workspace")
	cache := newBoundaryCache()
	view, err := NewClaudeAdapter().Render(RenderRequest{
		Member: member.Member, SourceDir: member.SourceDir, Activation: ActivationAuto, boundaries: cache,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	service := NewService()
	plan, err := service.Plan(context.Background(), Request{
		TargetPath: target, CatalogVersion: "test.1", BundleIDs: []string{"fixture-bundle"},
		Members: []ResolvedMember{member}, boundaries: cache,
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := service.Apply(context.Background(), plan); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	placed := filepath.Join(plan.TargetPath, filepath.FromSlash(view.RelativeDestination), "SKILL.md")
	if err := os.WriteFile(placed, []byte("tampered\n"), 0o644); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if err := VerifyPlacedView(plan.TargetPath, view, ActivationAuto); err == nil {
		t.Fatal("verification passed against tampered files; a cached destination read would hide this")
	}
}
