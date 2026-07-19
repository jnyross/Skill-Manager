package main

import (
	"bytes"
	"context"
	"fmt"
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

func TestTopLevelHelpPrintsUsageToStdout(t *testing.T) {
	for _, arg := range []string{"-h", "--help"} {
		var stdout, stderr bytes.Buffer
		if code := run([]string{arg}, &stdout, &stderr); code != 0 {
			t.Fatalf("%s: exit code = %d, stderr = %q", arg, code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "usage: skillet") {
			t.Fatalf("%s: stdout = %q", arg, stdout.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("%s: stderr = %q", arg, stderr.String())
		}
	}
}

func TestSetupHelpPrintsUsageToStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"setup", "--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage of skillet setup") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestSetupCommandHandlesCanceledOutcome(t *testing.T) {
	oldDefaults := setupTerminalDefaults
	setupTerminalDefaults = func() workspaceSetup.TerminalOptions {
		return workspaceSetup.TerminalOptions{
			Picker: pickerFunc(func(context.Context) (string, error) { return "", workspaceSetup.ErrPickerCanceled }),
		}
	}
	t.Cleanup(func() { setupTerminalDefaults = oldDefaults })
	var stdout, stderr bytes.Buffer
	code := runWithInput([]string{"setup"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Setup canceled.") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestSetupCommandDrivesRealTerminalFlowAndFiles(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: probe\ndescription: Probe\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := probeCatalog("license-text")
	oldDefaults := setupTerminalDefaults
	setupTerminalDefaults = func() workspaceSetup.TerminalOptions {
		return workspaceSetup.TerminalOptions{Catalog: &c, Resolver: fixtureResolver{source: source}, ToolPreflight: func(context.Context) []workspaceSetup.ToolResult { return nil }}
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
	for _, label := range []string{"Receipt: ", "Local verification: "} {
		path := lineValue(t, stdout.String(), label)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("result names %s%s but it does not exist: %v", label, path, err)
		}
	}
}

func TestSetupCommandReturnsUsageErrorForUnknownBundle(t *testing.T) {
	c := probeCatalog("license-text")
	oldDefaults := setupTerminalDefaults
	setupTerminalDefaults = func() workspaceSetup.TerminalOptions {
		return workspaceSetup.TerminalOptions{Catalog: &c, ToolPreflight: func(context.Context) []workspaceSetup.ToolResult { return nil }}
	}
	t.Cleanup(func() { setupTerminalDefaults = oldDefaults })
	target := filepath.Join(t.TempDir(), "project")
	var stdout, stderr bytes.Buffer

	code := runWithInput([]string{"setup", "--path", target, "--bundles", "not-a-real-bundle", "--yes", "--static"}, strings.NewReader(""), &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit = %d, want 2; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown built-in catalog bundle") {
		t.Fatalf("stderr does not report unknown bundle: %q", stderr.String())
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("unknown bundle setup mutated the target: %v", err)
	}
}

func TestSetupCommandFocusesBlockedOutcomeOnStderr(t *testing.T) {
	c := probeCatalog("declaration-only")
	oldDefaults := setupTerminalDefaults
	setupTerminalDefaults = func() workspaceSetup.TerminalOptions {
		return workspaceSetup.TerminalOptions{Catalog: &c, ToolPreflight: func(context.Context) []workspaceSetup.ToolResult { return nil }}
	}
	t.Cleanup(func() { setupTerminalDefaults = oldDefaults })
	target := filepath.Join(t.TempDir(), "project")
	var stdout, stderr bytes.Buffer

	code := runWithInput([]string{"setup", "--path", target, "--bundles", "probe-bundle", "--yes", "--static"}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "Blocked:") || !strings.Contains(stderr.String(), "license evidence") {
		t.Fatalf("stderr does not focus the blocker: %q", stderr.String())
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("blocked setup mutated the target: %v", err)
	}
}

// lineValue extracts the value after a "Label: " prefix from one output line.
func lineValue(t *testing.T, output, label string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, label) {
			return strings.TrimSpace(strings.TrimPrefix(line, label))
		}
	}
	t.Fatalf("output has no %q line: %q", label, output)
	return ""
}

// probeCatalog is the fixture catalog shared by the setup command tests;
// licenseEvidence decides whether the governance gate blocks. It pads the
// requested member out to 48 members and a single covering bundle so the
// injected catalog passes catalog.Validate().
func probeCatalog(licenseEvidence string) catalog.Catalog {
	member := catalog.Member{
		Name: "probe", Family: "probe", UpstreamActivation: "manual-only", VerificationPrompt: "Return only SKILLET_DISCOVERED_PROBE.",
		Source:  catalog.Source{Repository: "fixture", Subpath: "skills/probe", ReviewedRevision: strings.Repeat("a", 40), ContentSHA256: strings.Repeat("a", 64), MetadataSHA256: strings.Repeat("c", 64), DependencyEvidenceSHA256: strings.Repeat("d", 64), ExternalActionEvidenceSHA256: strings.Repeat("e", 64)},
		License: catalog.License{SPDX: "MIT", Notice: "LICENSE", NoticeSHA256: strings.Repeat("b", 64), Evidence: licenseEvidence},
		Recipes: []catalog.Recipe{{Tool: "claude-code", Scope: "project", Artifact: "direct-skill"}, {Tool: "codex", Scope: "project", Artifact: "direct-skill"}},
	}
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
	bundles := make([]catalog.Bundle, len(members))
	for i, name := range names {
		bundles[i] = catalog.Bundle{ID: name + "-bundle", Name: name, Members: []string{name}}
	}
	return catalog.Catalog{SchemaVersion: 1, Version: "test.1", ReviewedDate: "2026-07-15", Members: members, Bundles: bundles}
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

type pickerFunc func(context.Context) (string, error)

func (function pickerFunc) Pick(ctx context.Context) (string, error) { return function(ctx) }

func withBuildIdentity(t *testing.T, v, c, d string) {
	t.Helper()
	oldVersion, oldCommit, oldBuildDate := version, commit, buildDate
	version, commit, buildDate = v, c, d
	t.Cleanup(func() {
		version, commit, buildDate = oldVersion, oldCommit, oldBuildDate
	})
}
