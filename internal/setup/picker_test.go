package setup_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/catalog"
	"github.com/jnyross/Skill-Manager/internal/setup"
)

func TestNativePickerCancellationIsSideEffectFree(t *testing.T) {
	var output strings.Builder
	result, err := setup.RunTerminal(context.Background(), strings.NewReader("ignored\n"), &output, setup.TerminalOptions{
		UseNativePicker: true,
		Picker:          pickerFunc(func(context.Context) (string, error) { return "", setup.ErrPickerCanceled }),
	})
	if err != nil || result.Outcome != setup.OutcomeCanceled {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	if !strings.Contains(result.NextAction, "canceled") {
		t.Fatalf("next action = %q", result.NextAction)
	}
}

func TestTerminalPreflightCancellationLeavesWorkspaceUntouched(t *testing.T) {
	target := filepath.Join(t.TempDir(), "project")
	resolved := resolvedProbe(t)
	c := validTestCatalog(resolved.Member)
	var output strings.Builder
	result, err := setup.RunTerminal(context.Background(), strings.NewReader("\n\nn\n"), &output, setup.TerminalOptions{
		Catalog: &c, Path: target, BundleIDs: []string{"probe"},
		Resolver: resolverFunc(func(context.Context, []catalog.Member) ([]setup.ResolvedMember, func(), error) {
			return []setup.ResolvedMember{resolved}, func() {}, nil
		}),
		ToolPreflight: func(context.Context) []setup.ToolResult {
			return []setup.ToolResult{{Tool: "claude-code", Executable: "claude", Authenticated: false, Reason: "not authenticated"}}
		},
	})
	if err != nil || result.Outcome != setup.OutcomeBlocked {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("canceled preflight mutated target: %v", statErr)
	}
	for _, expected := range []string{"destinations:", "Tool preflight:", "not authenticated", "Apply this exact setup plan?"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("output missing %q: %s", expected, output.String())
		}
	}
}

func TestNativePickerFailureOffersGuardedTerminalFallback(t *testing.T) {
	var output strings.Builder
	result, err := setup.RunTerminal(context.Background(), strings.NewReader("\n"), &output, setup.TerminalOptions{
		UseNativePicker: true,
		Picker:          pickerFunc(func(context.Context) (string, error) { return "", errors.New("picker failed") }),
	})
	if err != nil || result.Outcome != setup.OutcomeCanceled {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	if !strings.Contains(output.String(), "guarded terminal path entry") {
		t.Fatalf("output = %q", output.String())
	}
}

type pickerFunc func(context.Context) (string, error)

func (function pickerFunc) Pick(ctx context.Context) (string, error) { return function(ctx) }

type resolverFunc func(context.Context, []catalog.Member) ([]setup.ResolvedMember, func(), error)

func (function resolverFunc) ResolveMembers(ctx context.Context, members []catalog.Member) ([]setup.ResolvedMember, func(), error) {
	return function(ctx, members)
}

// validTestCatalog returns a catalog that passes catalog.Validate() for the
// given real member, by padding it with dummy members and a single covering
// bundle.
func validTestCatalog(member catalog.Member) catalog.Catalog {
	members := []catalog.Member{member}
	names := []string{member.Name}
	for i := 1; i < 48; i++ {
		name := fmt.Sprintf("dummy-%02d", i)
		names = append(names, name)
		members = append(members, catalog.Member{
			Name: name, Family: "dummy", UpstreamActivation: "manual-only", VerificationPrompt: "Return only SKILLET_DISCOVERED_" + name + ".",
			Source:  catalog.Source{Repository: "fixture", Subpath: "skills/" + name, ReviewedRevision: strings.Repeat("a", 40), ContentSHA256: strings.Repeat("0", 64), MetadataSHA256: strings.Repeat("1", 64), DependencyEvidenceSHA256: strings.Repeat("2", 64), ExternalActionEvidenceSHA256: strings.Repeat("3", 64)},
			License: catalog.License{SPDX: "MIT", Notice: "LICENSE", NoticeSHA256: strings.Repeat("b", 64), Evidence: "license-text"},
			Recipes: []catalog.Recipe{{Tool: "claude-code", Scope: "project", Artifact: "direct-skill"}, {Tool: "codex", Scope: "project", Artifact: "direct-skill"}},
		})
	}
	return catalog.Catalog{SchemaVersion: 1, Version: "test.1", ReviewedDate: "2026-07-15", Members: members, Bundles: []catalog.Bundle{{ID: "probe", Name: "Probe", Members: names}}}
}
