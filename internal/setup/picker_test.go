package setup_test

import (
	"context"
	"errors"
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
		Catalog: catalog.Catalog{Version: "test"}, UseNativePicker: true,
		Picker: pickerFunc(func(context.Context) (string, error) { return "", setup.ErrPickerCanceled }),
	})
	if err != nil || result.Outcome != setup.OutcomeBlocked {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	if !strings.Contains(result.NextAction, "canceled") {
		t.Fatalf("next action = %q", result.NextAction)
	}
}

func TestTerminalPreflightCancellationLeavesWorkspaceUntouched(t *testing.T) {
	target := filepath.Join(t.TempDir(), "project")
	resolved := resolvedProbe(t)
	c := catalog.Catalog{Version: "test", Members: []catalog.Member{resolved.Member}, Bundles: []catalog.Bundle{{ID: "probe", Name: "Probe", Members: []string{resolved.Member.Name}}}}
	var output strings.Builder
	result, err := setup.RunTerminal(context.Background(), strings.NewReader("\n\nn\n"), &output, setup.TerminalOptions{
		Catalog: c, Path: target, BundleIDs: []string{"probe"},
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
		Catalog: catalog.Catalog{Version: "test"}, UseNativePicker: true,
		Picker: pickerFunc(func(context.Context) (string, error) { return "", errors.New("picker failed") }),
	})
	if err != nil || result.Outcome != setup.OutcomeBlocked {
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
