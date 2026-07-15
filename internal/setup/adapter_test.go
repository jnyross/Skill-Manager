package setup_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/catalog"
	"github.com/jnyross/Skill-Manager/internal/setup"
)

func TestToolAdaptersRenderDeterministicProjectViews(t *testing.T) {
	source := t.TempDir()
	writeFile(t, filepath.Join(source, "SKILL.md"), "---\nname: probe\ndescription: Probe\n---\nBody\n")
	member := catalog.Member{Name: "probe", Recipes: []catalog.Recipe{
		{Tool: "claude-code", Scope: "project", Artifact: "direct-skill"},
		{Tool: "codex", Scope: "project", Artifact: "direct-skill"},
	}}

	claude := setup.NewClaudeAdapter()
	codex := setup.NewCodexAdapter()
	claudeView, err := claude.Render(setup.RenderRequest{Member: member, SourceDir: source, Activation: setup.ActivationManualOnly, ActivationOverride: true})
	if err != nil {
		t.Fatalf("Claude Render: %v", err)
	}
	codexView, err := codex.Render(setup.RenderRequest{Member: member, SourceDir: source, Activation: setup.ActivationManualOnly, ActivationOverride: true})
	if err != nil {
		t.Fatalf("Codex Render: %v", err)
	}
	if claudeView.RelativeDestination != ".claude/skills/probe" || codexView.RelativeDestination != ".agents/skills/probe" {
		t.Fatalf("destinations = %q, %q", claudeView.RelativeDestination, codexView.RelativeDestination)
	}
	if !strings.Contains(string(claudeView.Files["SKILL.md"]), "disable-model-invocation: true") {
		t.Fatalf("Claude activation overlay missing: %s", claudeView.Files["SKILL.md"])
	}
	if !strings.Contains(string(codexView.Files["agents/openai.yaml"]), "allow_implicit_invocation: false") {
		t.Fatalf("Codex activation overlay missing: %s", codexView.Files["agents/openai.yaml"])
	}
	if claudeView.CanonicalContentSHA256 != codexView.CanonicalContentSHA256 {
		t.Fatalf("canonical identity drifted: %s != %s", claudeView.CanonicalContentSHA256, codexView.CanonicalContentSHA256)
	}
}

func TestToolAdaptersPreserveUpstreamActivationWithoutOverride(t *testing.T) {
	source := t.TempDir()
	original := "---\nname: probe\ndescription: Probe\ndisable-model-invocation: true\n---\nBody\n"
	writeFile(t, filepath.Join(source, "SKILL.md"), original)
	member := catalog.Member{Name: "probe", Recipes: []catalog.Recipe{
		{Tool: "claude-code", Scope: "project", Artifact: "direct-skill"},
		{Tool: "codex", Scope: "project", Artifact: "direct-skill"},
	}}
	for _, adapter := range []setup.ToolAdapter{setup.NewClaudeAdapter(), setup.NewCodexAdapter()} {
		view, err := adapter.Render(setup.RenderRequest{Member: member, SourceDir: source})
		if err != nil {
			t.Fatalf("%s Render: %v", adapter.Tool(), err)
		}
		if string(view.Files["SKILL.md"]) != original {
			t.Fatalf("%s changed upstream activation without override", adapter.Tool())
		}
	}
}

func TestToolAdapterRejectsUnsupportedRecipeBeforeRendering(t *testing.T) {
	source := t.TempDir()
	writeFile(t, filepath.Join(source, "SKILL.md"), "---\nname: probe\n---\n")
	member := catalog.Member{Name: "probe", Recipes: []catalog.Recipe{{Tool: "codex", Scope: "personal", Artifact: "plugin"}}}
	if _, err := setup.NewCodexAdapter().Render(setup.RenderRequest{Member: member, SourceDir: source}); err == nil {
		t.Fatal("unsupported Personal plugin recipe accepted as Project direct skill")
	}
}

func TestToolAdapterRejectsUnsupportedConstructCapabilityBeforeRendering(t *testing.T) {
	source := t.TempDir()
	writeFile(t, filepath.Join(source, "SKILL.md"), "---\nname: probe\n---\n")
	member := catalog.Member{Name: "probe", Recipes: []catalog.Recipe{{Tool: "codex", Scope: "project", Artifact: "direct-skill", Requires: []string{"hooks"}}}}
	if _, err := setup.NewCodexAdapter().Render(setup.RenderRequest{Member: member, SourceDir: source}); err == nil || !strings.Contains(err.Error(), "unsupported hooks capability") {
		t.Fatalf("unsupported hooks capability error = %v", err)
	}
}

func writeFile(t *testing.T, filename, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
