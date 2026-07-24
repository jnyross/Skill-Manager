package setup_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/catalog"
	"github.com/jnyross/Skill-Manager/internal/setup"
)

// progressResolver reports one line and records the Progress it was handed, so
// the test can prove RunTerminal wires the seam rather than the resolver
// printing to stdout itself.
type progressResolver struct {
	resolved setup.ResolvedMember
	progress setup.Progress
	wired    *bool
}

func (resolver progressResolver) WithProgress(progress setup.Progress) setup.MemberResolver {
	*resolver.wired = true
	resolver.progress = progress
	return resolver
}

func (resolver progressResolver) ResolveMembers(context.Context, []catalog.Member) ([]setup.ResolvedMember, func(), error) {
	if resolver.progress != nil {
		resolver.progress("Source 1/1 fixture — cloning")
	}
	return []setup.ResolvedMember{resolver.resolved}, func() {}, nil
}

// A resolver's progress must reach the wizard's own output writer, so the
// blocking clones are never a silent stall.
func TestRunTerminalRoutesResolverProgressToItsOutput(t *testing.T) {
	target := filepath.Join(t.TempDir(), "project")
	resolved := resolvedProbe(t)
	c := validTestCatalog(resolved.Member)
	wired := false
	var output strings.Builder

	if _, err := setup.RunTerminal(context.Background(), strings.NewReader("\n\nn\n"), &output, setup.TerminalOptions{
		Catalog: &c, Path: target, BundleIDs: []string{"probe"},
		Resolver:      progressResolver{resolved: resolved, wired: &wired},
		ToolPreflight: func(context.Context) []setup.ToolResult { return nil },
	}); err != nil {
		t.Fatal(err)
	}
	if !wired {
		t.Fatal("RunTerminal did not offer the resolver a Progress")
	}
	if !strings.Contains(output.String(), "Source 1/1 fixture — cloning") {
		t.Fatalf("progress line missing from wizard output: %q", output.String())
	}
}

// GitResolver names each Source and its step before the blocking clone starts,
// not after it finishes.
func TestGitResolverReportsEachSourceBeforeCloning(t *testing.T) {
	var lines []string
	resolver := setup.GitResolver{TempParent: t.TempDir()}.WithProgress(func(line string) {
		lines = append(lines, line)
	})
	member := reviewedMember()
	member.Source.Repository = filepath.Join(t.TempDir(), "no-such-repository")

	if _, _, err := resolver.ResolveMembers(context.Background(), []catalog.Member{member}); err == nil {
		t.Fatal("cloning a nonexistent repository unexpectedly succeeded")
	}
	if len(lines) != 1 {
		t.Fatalf("progress lines = %#v, want exactly the pre-clone line", lines)
	}
	for _, want := range []string{"Source 1/1", member.Source.Repository, "cloning"} {
		if !strings.Contains(lines[0], want) {
			t.Fatalf("progress line %q missing %q", lines[0], want)
		}
	}
}

// A resolver that does not report progress keeps working unchanged.
func TestResolverWithoutProgressIsUnaffected(t *testing.T) {
	target := filepath.Join(t.TempDir(), "project")
	resolved := resolvedProbe(t)
	c := validTestCatalog(resolved.Member)
	var output strings.Builder

	if _, err := setup.RunTerminal(context.Background(), strings.NewReader("\n\nn\n"), &output, setup.TerminalOptions{
		Catalog: &c, Path: target, BundleIDs: []string{"probe"},
		Resolver: resolverFunc(func(context.Context, []catalog.Member) ([]setup.ResolvedMember, func(), error) {
			return []setup.ResolvedMember{resolved}, func() {}, nil
		}),
		ToolPreflight: func(context.Context) []setup.ToolResult { return nil },
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Apply this exact setup plan?") {
		t.Fatalf("output = %q", output.String())
	}
}
