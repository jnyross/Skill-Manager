package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/catalog"
	workspaceSetup "github.com/jnyross/Skill-Manager/internal/setup"
)

func TestVersionCommandReportsInjectedReleaseIdentity(t *testing.T) {
	withBuildIdentity(t, "0.1.0-rc.1", "abc1234", "2026-07-15T12:00:00Z")
	var stdout, stderr bytes.Buffer

	if code := run([]string{"--version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	want := "skillet 0.1.0-rc.1 (commit abc1234, built 2026-07-15T12:00:00Z)\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestVersionCommandNeverPresentsDefaultBuildAsRelease(t *testing.T) {
	withBuildIdentity(t, "dev", "unknown", "unknown")
	var stdout, stderr bytes.Buffer

	if code := run([]string{"version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "development") || strings.Contains(got, "0.1.0") {
		t.Fatalf("development identity = %q", got)
	}
}

func TestUnknownCommandFailsWithoutStartingTUI(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"unknown"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestSetupCommandDrivesRealTerminalFlowAndFiles(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: probe\ndescription: Probe\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	member := catalog.Member{
		Name: "probe", UpstreamActivation: "manual-only", VerificationPrompt: "Return only SKILLET_DISCOVERED_PROBE.",
		Source:  catalog.Source{Repository: "fixture", Subpath: "skills/probe", ReviewedRevision: strings.Repeat("a", 40), ContentSHA256: strings.Repeat("a", 64)},
		License: catalog.License{SPDX: "MIT", Notice: "LICENSE", NoticeSHA256: strings.Repeat("b", 64), Evidence: "license-text"},
		Recipes: []catalog.Recipe{{Tool: "claude-code", Scope: "project", Artifact: "direct-skill"}, {Tool: "codex", Scope: "project", Artifact: "direct-skill"}},
	}
	c := catalog.Catalog{Version: "test.1", Members: []catalog.Member{member}, Bundles: []catalog.Bundle{{ID: "probe-bundle", Name: "Probe", Members: []string{"probe"}}}}
	oldDefaults := setupTerminalDefaults
	setupTerminalDefaults = func() workspaceSetup.TerminalOptions {
		return workspaceSetup.TerminalOptions{Catalog: c, Resolver: fixtureResolver{source: source}, ToolPreflight: func(context.Context) []workspaceSetup.ToolResult { return nil }}
	}
	t.Cleanup(func() { setupTerminalDefaults = oldDefaults })
	target := filepath.Join(t.TempDir(), "project")
	var stdout, stderr bytes.Buffer
	code := runWithInput([]string{"setup", "--path", target, "--bundles", "probe-bundle", "--auto", "probe", "--yes", "--static"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Outcome: Configured-unverified") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	for _, relative := range []string{"AGENTS.md", ".claude/skills/probe/SKILL.md", ".agents/skills/probe/SKILL.md", ".skillet/workspace.json"} {
		if _, err := os.Stat(filepath.Join(target, relative)); err != nil {
			t.Errorf("%s missing: %v", relative, err)
		}
	}
	claudeSkill, _ := os.ReadFile(filepath.Join(target, ".claude", "skills", "probe", "SKILL.md"))
	if !strings.Contains(string(claudeSkill), "disable-model-invocation: false") {
		t.Fatal("--auto did not render Claude activation")
	}
	codexConfig, _ := os.ReadFile(filepath.Join(target, ".agents", "skills", "probe", "agents", "openai.yaml"))
	if !strings.Contains(string(codexConfig), "allow_implicit_invocation: true") {
		t.Fatal("--auto did not render Codex activation")
	}
}

type fixtureResolver struct{ source string }

func (resolver fixtureResolver) ResolveMembers(_ context.Context, members []catalog.Member) ([]workspaceSetup.ResolvedMember, func(), error) {
	resolved := make([]workspaceSetup.ResolvedMember, len(members))
	for index, member := range members {
		resolved[index] = workspaceSetup.ResolvedMember{
			Member: member, SourceDir: resolver.source,
			Evidence: workspaceSetup.BoundaryEvidence{Revision: member.Source.ReviewedRevision, Subpath: member.Source.Subpath, ContentSHA256: member.Source.ContentSHA256, LicenseSHA256: member.License.NoticeSHA256},
		}
	}
	return resolved, func() {}, nil
}

func withBuildIdentity(t *testing.T, v, c, d string) {
	t.Helper()
	oldVersion, oldCommit, oldBuildDate := version, commit, buildDate
	version, commit, buildDate = v, c, d
	t.Cleanup(func() {
		version, commit, buildDate = oldVersion, oldCommit, oldBuildDate
	})
}
