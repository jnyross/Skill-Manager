package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"skillet/internal/engine"
)

func TestBuildInstallTargetOptionsPersonalOnly(t *testing.T) {
	e := engine.New(engine.Roots{})
	opts := buildInstallTargetOptions(e)
	if len(opts) != 1 {
		t.Fatalf("len = %d, want 1", len(opts))
	}
	if opts[0].label != "Personal" || opts[0].target.Kind != engine.InstallTargetPersonal {
		t.Fatalf("option = %#v", opts[0])
	}
}

func TestBuildInstallTargetOptionsIncludesResolvedProjects(t *testing.T) {
	repoA := filepath.Join(t.TempDir(), "a")
	repoB := filepath.Join(t.TempDir(), "b")
	e := engine.New(engine.Roots{
		ProjectRoots:       []string{repoA, repoB},
		ClaudeProjectRoots: []string{repoA}, // dedupe with ProjectRoots
	})
	opts := buildInstallTargetOptions(e)
	if len(opts) != 3 {
		t.Fatalf("len = %d, want 3 (Personal + 2 projects), got %#v", len(opts), opts)
	}
	if opts[0].target.Kind != engine.InstallTargetPersonal {
		t.Fatalf("first option should be Personal: %#v", opts[0])
	}
	labels := opts[1].label + " " + opts[2].label
	if !strings.Contains(labels, "Project: ") {
		t.Fatalf("project labels missing: %q", labels)
	}
	if opts[1].target.RepoRoot == "" || opts[2].target.RepoRoot == "" {
		t.Fatalf("project options missing RepoRoot: %#v", opts)
	}
}

func TestRenderInstallPickerDescriptionMarksCursor(t *testing.T) {
	opts := []installTargetOption{
		{label: "Personal"},
		{label: "Project: /tmp/r"},
	}
	desc := renderInstallPickerDescription("my-skill", opts, 1)
	if !strings.Contains(desc, `Install "my-skill"`) {
		t.Fatalf("missing entry name: %s", desc)
	}
	if !strings.Contains(desc, "> Project: /tmp/r") {
		t.Fatalf("cursor not on project row: %s", desc)
	}
	if !strings.Contains(desc, " enter to select") && !strings.Contains(desc, "enter to select") {
		t.Fatalf("missing select hint: %s", desc)
	}
}
