package engine_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"skillet/internal/engine"
)

type recordingRunner struct {
	commands []engine.Command
	errAt    int
	result   *engine.CommandResult
}

func (r *recordingRunner) Run(command engine.Command) (engine.CommandResult, error) {
	r.commands = append(r.commands, command)
	if r.errAt > 0 && len(r.commands) == r.errAt {
		return engine.CommandResult{Stderr: "boom"}, fmt.Errorf("exit 1")
	}
	if r.result != nil {
		return *r.result, nil
	}
	return engine.CommandResult{Stdout: "✔ Successfully installed"}, nil
}

func TestInstallLibraryEntryMarketplaceRejectsExitZeroWithoutSuccessMarker(t *testing.T) {
	f := newFixture(t)
	writeFile(t, filepath.Join(f.roots.ClaudeHome, "plugins", "known_marketplaces.json"), `{"catalog": {}}`)
	result := engine.CommandResult{Stderr: "Error: plugin not found"}
	r := &recordingRunner{result: &result}
	e := engine.NewWithCommandRunner(f.roots, r)
	entry := engine.LibraryEntry{Name: "plugin", Source: engine.LibrarySource{Kind: engine.LibrarySourceMarketplace, Marketplace: "catalog", PluginName: "missing"}}
	err := e.InstallLibraryEntry(entry, engine.InstallTarget{Kind: engine.InstallTargetPersonal}, engine.ActivationAuto)
	if err == nil || !strings.Contains(err.Error(), "did not report successful") {
		t.Fatalf("error = %v", err)
	}
}

func TestInstallLibraryEntryGitSubpathToPersonalAndProject(t *testing.T) {
	remote := makeGitFixture(t)

	for _, tc := range []struct {
		name   string
		target engine.InstallTarget
		dest   func(f fixture) string
		roots  func(*fixture)
	}{
		{
			name:   "personal",
			target: engine.InstallTarget{Kind: engine.InstallTargetPersonal},
			dest:   func(f fixture) string { return filepath.Join(f.roots.ClaudeHome, "skills", "from-git") },
			roots:  func(*fixture) {},
		},
		{
			name: "project",
			dest: func(f fixture) string { return filepath.Join(f.root, "repo", ".claude", "skills", "from-git") },
			roots: func(f *fixture) {
				repo := filepath.Join(f.root, "repo")
				mkdirAll(t, repo)
				f.roots.ClaudeProjectRoots = []string{repo}
			},
			target: engine.InstallTarget{Kind: engine.InstallTargetProject},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			tc.roots(&f)
			if tc.target.Kind == engine.InstallTargetProject {
				tc.target.RepoRoot = filepath.Join(f.root, "repo")
			}
			e := engine.New(f.roots)
			entry := engine.LibraryEntry{Name: "from-git", Kind: engine.KindSkill, Tool: engine.ToolClaudeCode, Source: engine.LibrarySource{Kind: engine.LibrarySourceGit, GitURL: "file://" + remote, GitRef: "main", GitSubPath: "skills/from-git"}}
			if err := e.InstallLibraryEntry(entry, tc.target, engine.ActivationAuto); err != nil {
				t.Fatalf("InstallLibraryEntry: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(tc.dest(f), "SKILL.md"))
			if err != nil {
				t.Fatalf("read installed skill: %v", err)
			}
			if !strings.Contains(string(data), "from fixture") {
				t.Fatalf("installed content = %q", data)
			}
		})
	}
}

func TestInstallLibraryEntrySkillsShDispatch(t *testing.T) {
	for _, tc := range []struct {
		name     string
		tool     engine.Tool
		target   engine.InstallTargetKind
		wantArgs []string
		skill    string
	}{
		{"personal claude", engine.ToolClaudeCode, engine.InstallTargetPersonal, []string{"skills", "add", "owner/repo", "-a", "claude-code", "-y", "--copy", "--skill", "one", "-g"}, "one"},
		{"project codex all", engine.ToolCodex, engine.InstallTargetProject, []string{"skills", "add", "owner/repo", "-a", "codex", "-y", "--copy", "--skill", "*"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			repo := filepath.Join(f.root, "repo")
			mkdirAll(t, repo)
			f.roots.ProjectRoots = []string{repo}
			r := &recordingRunner{}
			e := engine.NewWithCommandRunner(f.roots, r)
			target := engine.InstallTarget{Kind: tc.target}
			if tc.target == engine.InstallTargetProject {
				target.RepoRoot = repo
			}
			entry := engine.LibraryEntry{Name: "one", Kind: engine.KindSkill, Tool: tc.tool, Source: engine.LibrarySource{Kind: engine.LibrarySourceSkillsSh, SkillsShRepo: "owner/repo", SkillsShSkill: tc.skill}}
			if err := e.InstallLibraryEntry(entry, target, engine.ActivationAuto); err != nil {
				t.Fatalf("InstallLibraryEntry: %v", err)
			}
			if len(r.commands) != 1 {
				t.Fatalf("commands = %#v", r.commands)
			}
			if r.commands[0].Name != "npx" || !reflect.DeepEqual(r.commands[0].Args, tc.wantArgs) {
				t.Fatalf("command = %#v", r.commands[0])
			}
			wantDir := ""
			if tc.target == engine.InstallTargetProject {
				wantDir = repo
			}
			if r.commands[0].Dir != wantDir {
				t.Fatalf("dir = %q, want %q", r.commands[0].Dir, wantDir)
			}
		})
	}
}

func TestSkillsShUsesActualSkillNameForDestination(t *testing.T) {
	f := newFixture(t)
	dest := filepath.Join(f.roots.ClaudeHome, "skills", "actual")
	mkdirAll(t, dest)
	e := engine.New(f.roots)
	entry := engine.LibraryEntry{Name: "display", Kind: engine.KindSkill, Tool: engine.ToolClaudeCode, Source: engine.LibrarySource{Kind: engine.LibrarySourceSkillsSh, SkillsShRepo: "owner/repo", SkillsShSkill: "actual"}}
	got, exists, err := e.InstallDestination(entry, engine.InstallTarget{Kind: engine.InstallTargetPersonal})
	if err != nil || !exists || got != dest {
		t.Fatalf("InstallDestination = %q, %t, %v; want %q, true", got, exists, err, dest)
	}
}

func TestSkillsShAllAppliesManualOnlyUsingLockEntries(t *testing.T) {
	f := newFixture(t)
	repo := filepath.Join(f.root, "repo")
	f.roots.ClaudeProjectRoots = []string{repo}
	writeSkill(t, filepath.Join(repo, ".claude", "skills", "one"), "one", "one", "")
	writeFile(t, filepath.Join(repo, "skills-lock.json"), `{"skills":{"one":{"source":"owner/repo"},"other":{"source":"someone/else"}}}`)
	r := &recordingRunner{}
	e := engine.NewWithCommandRunner(f.roots, r)
	entry := engine.LibraryEntry{Name: "package", Kind: engine.KindSkill, Tool: engine.ToolClaudeCode, Source: engine.LibrarySource{Kind: engine.LibrarySourceSkillsSh, SkillsShRepo: "owner/repo"}}
	if err := e.InstallLibraryEntry(entry, engine.InstallTarget{Kind: engine.InstallTargetProject, RepoRoot: repo}, engine.ActivationManualOnly); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(repo, ".claude", "skills", "one", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "disable-model-invocation: true") {
		t.Fatalf("Manual-only not applied: %s", data)
	}
}

func TestSkillsShAllPersonalUsesXDGStateLockForManualOnly(t *testing.T) {
	f := newFixture(t)
	xdg := filepath.Join(f.root, "xdg-state")
	t.Setenv("XDG_STATE_HOME", xdg)
	writeSkill(t, filepath.Join(f.roots.ClaudeHome, "skills", "one"), "one", "one", "")
	writeFile(t, filepath.Join(xdg, "skills", ".skill-lock.json"), `{"skills":{"one":{"source":"owner/repo"}}}`)
	e := engine.NewWithCommandRunner(f.roots, &recordingRunner{})
	entry := engine.LibraryEntry{Name: "package", Kind: engine.KindSkill, Tool: engine.ToolClaudeCode, Source: engine.LibrarySource{Kind: engine.LibrarySourceSkillsSh, SkillsShRepo: "owner/repo"}}
	if err := e.InstallLibraryEntry(entry, engine.InstallTarget{Kind: engine.InstallTargetPersonal}, engine.ActivationManualOnly); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(f.roots.ClaudeHome, "skills", "one", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "disable-model-invocation: true") {
		t.Fatalf("Manual-only not applied: %s", data)
	}
}

func TestInstallLibraryEntryMarketplaceAddsUnknownThenInstalls(t *testing.T) {
	f := newFixture(t)
	repo := filepath.Join(f.root, "repo")
	mkdirAll(t, repo)
	f.roots.ClaudeProjectRoots = []string{repo}
	r := &recordingRunner{}
	e := engine.NewWithCommandRunner(f.roots, r)
	entry := engine.LibraryEntry{Name: "plugin", Source: engine.LibrarySource{Kind: engine.LibrarySourceMarketplace, Marketplace: "catalog", PluginName: "plugin", MarketplaceSource: "owner/catalog"}}
	if err := e.InstallLibraryEntry(entry, engine.InstallTarget{Kind: engine.InstallTargetProject, RepoRoot: repo}, engine.ActivationAuto); err != nil {
		t.Fatalf("InstallLibraryEntry: %v", err)
	}
	want := []engine.Command{{Name: "claude", Args: []string{"plugin", "marketplace", "add", "owner/catalog", "--scope", "user"}}, {Name: "claude", Args: []string{"plugin", "install", "plugin@catalog", "--scope", "project"}, Dir: repo}}
	if !reflect.DeepEqual(r.commands, want) {
		t.Fatalf("commands = %#v, want %#v", r.commands, want)
	}
}

func TestInstallLibraryEntryMarketplaceKnownSkipsAdd(t *testing.T) {
	f := newFixture(t)
	writeFile(t, filepath.Join(f.roots.ClaudeHome, "plugins", "known_marketplaces.json"), `{"catalog": {"source": {"source": "github", "repo": "owner/catalog"}}}`)
	r := &recordingRunner{}
	e := engine.NewWithCommandRunner(f.roots, r)
	entry := engine.LibraryEntry{Name: "plugin", Source: engine.LibrarySource{Kind: engine.LibrarySourceMarketplace, Marketplace: "catalog", PluginName: "plugin"}}
	if err := e.InstallLibraryEntry(entry, engine.InstallTarget{Kind: engine.InstallTargetPersonal}, engine.ActivationAuto); err != nil {
		t.Fatalf("InstallLibraryEntry: %v", err)
	}
	if len(r.commands) != 1 || !reflect.DeepEqual(r.commands[0].Args, []string{"plugin", "install", "plugin@catalog", "--scope", "user"}) {
		t.Fatalf("commands = %#v", r.commands)
	}
}

func TestInstallLibraryEntryMarketplaceUnknownRequiresSource(t *testing.T) {
	f := newFixture(t)
	r := &recordingRunner{}
	e := engine.NewWithCommandRunner(f.roots, r)
	entry := engine.LibraryEntry{Name: "plugin", Source: engine.LibrarySource{Kind: engine.LibrarySourceMarketplace, Marketplace: "catalog", PluginName: "plugin"}}
	err := e.InstallLibraryEntry(entry, engine.InstallTarget{Kind: engine.InstallTargetPersonal}, engine.ActivationAuto)
	if err == nil || !strings.Contains(err.Error(), "marketplace source") {
		t.Fatalf("error = %v", err)
	}
	if len(r.commands) != 0 {
		t.Fatalf("commands = %#v", r.commands)
	}
}

func makeGitFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	work := filepath.Join(dir, "work")
	remote := filepath.Join(dir, "remote.git")
	mkdirAll(t, work)
	writeSkill(t, filepath.Join(work, "skills", "from-git"), "from-git", "from fixture", "")
	for _, args := range [][]string{{"init", "-b", "main", work}, {"-C", work, "add", "."}, {"-C", work, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "fixture"}, {"clone", "--bare", work, remote}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return remote
}
