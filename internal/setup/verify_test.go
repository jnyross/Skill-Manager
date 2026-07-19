package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCodexSkillChallengeUsesExactManagedContent(t *testing.T) {
	target := t.TempDir()
	skillDir := filepath.Join(target, ".agents", "skills", "example")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	contents := "---\nname: example\ndescription: Not the expected answer.\n---\n\n# Exact heading\n\nBody.\n\nFinal unique line.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	root, err := os.OpenRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	challenge, err := codexSkillChallenge(root, "example")
	if err != nil {
		t.Fatal(err)
	}
	if challenge != "5c570c9d54cc22dd9e33f6f097232bbb752019c309c4385e3c19a2f167c1e4af" {
		t.Fatalf("challenge = %q", challenge)
	}
}

func TestIsolatedCodexEnvironmentRetainsOnlyAuthentication(t *testing.T) {
	originalHome := t.TempDir()
	originalUserHome := t.TempDir()
	authPath := filepath.Join(originalHome, "auth.json")
	if err := os.WriteFile(authPath, []byte("test-only"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(originalHome, "skills", "shadow"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(originalUserHome, ".agents", "skills", "shadow"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", originalHome)
	t.Setenv("HOME", originalUserHome)

	environment, cleanup, err := isolatedCodexEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	temporaryHome := environmentValue(environment, "CODEX_HOME")
	if got := environmentValue(environment, "HOME"); got != temporaryHome {
		t.Fatalf("isolated HOME = %q, want CODEX_HOME %q", got, temporaryHome)
	}
	if err := os.WriteFile(filepath.Join(temporaryHome, "auth.json"), []byte("changed-copy"), 0o600); err != nil {
		t.Fatal(err)
	}
	originalAuth, err := os.ReadFile(authPath)
	if err != nil || string(originalAuth) != "test-only" {
		t.Fatalf("isolated auth write changed source: %q, %v", originalAuth, err)
	}
	if _, err := os.Stat(filepath.Join(temporaryHome, "skills")); !os.IsNotExist(err) {
		t.Fatalf("isolated home copied user skills: %v", err)
	}
	if _, err := os.Stat(filepath.Join(temporaryHome, ".agents")); !os.IsNotExist(err) {
		t.Fatalf("isolated HOME copied user .agents skills: %v", err)
	}
	cleanup()

	if temporaryHome == "" || temporaryHome == originalHome {
		t.Fatalf("isolated CODEX_HOME = %q", temporaryHome)
	}
	if _, err := os.Stat(temporaryHome); !os.IsNotExist(err) {
		t.Fatalf("cleanup left isolated home: %v", err)
	}
}

func TestClaudeCommandDiscoveryUsesNativeParserState(t *testing.T) {
	positive := []byte(`{"type":"result","subtype":"success","is_error":false,"num_turns":1,"result":"Skill-specific response"}`)
	if discovered, reason := claudeCommandDiscovered(positive, "example"); !discovered {
		t.Fatalf("recognized skill was rejected: %s", reason)
	}

	unknown := []byte(`{"type":"result","subtype":"success","is_error":false,"num_turns":0,"result":"Unknown command: /example"}`)
	if discovered, _ := claudeCommandDiscovered(unknown, "example"); discovered {
		t.Fatal("unknown native command passed discovery")
	}
}

func TestClaudeVerificationEnvironmentDisablesBundledSkillCollisions(t *testing.T) {
	environment := withEnvironmentValue([]string{"PATH=/bin", "CLAUDE_CODE_DISABLE_BUNDLED_SKILLS=0"}, "CLAUDE_CODE_DISABLE_BUNDLED_SKILLS", "1")
	if got := environmentValue(environment, "CLAUDE_CODE_DISABLE_BUNDLED_SKILLS"); got != "1" {
		t.Fatalf("CLAUDE_CODE_DISABLE_BUNDLED_SKILLS = %q", got)
	}
	if len(environment) != 2 {
		t.Fatalf("duplicate environment values survived: %v", environment)
	}
}

func TestSHA256RecognitionIsExact(t *testing.T) {
	if !looksLikeSHA256(strings.Repeat("a", 64)) {
		t.Fatal("valid lowercase SHA-256 was rejected")
	}
	for _, invalid := range []string{strings.Repeat("a", 63), strings.Repeat("A", 64), "skill unavailable"} {
		if looksLikeSHA256(invalid) {
			t.Fatalf("invalid digest %q was accepted", invalid)
		}
	}
}

func environmentValue(environment []string, key string) string {
	prefix := key + "="
	for _, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func TestExtractCodexAgentMessage(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"no matching event", `{"type":"foo"}`, ""},
		{"wrong item type", `{"type":"item.completed","item":{"type":"tool_message","text":"hello"}}`, ""},
		{"matching", `{"type":"item.completed","item":{"type":"agent_message","text":"  hello world  "}}`, "hello world"},
		{"last match wins", "{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"first\"}}\n{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"second\"}}", "second"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := extractCodexAgentMessage([]byte(c.input)); got != c.want {
				t.Fatalf("extractCodexAgentMessage(%q) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}

func TestCodexChallengePrompt(t *testing.T) {
	prompt := codexChallengePrompt("example-skill")
	if !strings.Contains(prompt, "$example-skill") {
		t.Fatalf("prompt missing skill variable: %q", prompt)
	}
	if !strings.Contains(prompt, "SHA-256") {
		t.Fatalf("prompt missing SHA-256 instruction: %q", prompt)
	}
	if strings.Contains(prompt, "\n") {
		t.Fatalf("prompt contains unexpected newline: %q", prompt)
	}
}

type fakeCommandRunner struct {
	runHook            func(cmd *exec.Cmd) error
	combinedOutputHook func(cmd *exec.Cmd) ([]byte, error)
}

func (f fakeCommandRunner) Run(cmd *exec.Cmd) error {
	if f.runHook != nil {
		return f.runHook(cmd)
	}
	return nil
}

func (f fakeCommandRunner) CombinedOutput(cmd *exec.Cmd) ([]byte, error) {
	if f.combinedOutputHook != nil {
		return f.combinedOutputHook(cmd)
	}
	return nil, nil
}

func setRunner(t *testing.T, runner commandRunner) {
	t.Helper()
	old := defaultCommandRunner
	defaultCommandRunner = runner
	t.Cleanup(func() { defaultCommandRunner = old })
}

func TestCheckAuthentication(t *testing.T) {
	cases := []struct {
		name      string
		tool      string
		wantArgs  []string
		returnErr error
		want      bool
	}{
		{
			name:     "claude authenticated",
			tool:     "claude-code",
			wantArgs: []string{"auth", "status", "--json"},
			want:     true,
		},
		{
			name:     "codex authenticated",
			tool:     "codex",
			wantArgs: []string{"login", "status"},
			want:     true,
		},
		{
			name:      "claude unauthenticated",
			tool:      "claude-code",
			wantArgs:  []string{"auth", "status", "--json"},
			returnErr: errors.New("not logged in"),
			want:      false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var gotArgs []string
			setRunner(t, fakeCommandRunner{
				runHook: func(cmd *exec.Cmd) error {
					gotArgs = cmd.Args
					return c.returnErr
				},
			})

			got := checkAuthentication(context.Background(), c.tool, "/fake/bin")
			if got != c.want {
				t.Fatalf("checkAuthentication() = %v, want %v", got, c.want)
			}
			if len(gotArgs) == 0 || gotArgs[0] != "/fake/bin" {
				t.Fatalf("unexpected executable in command args: %v", gotArgs)
			}
			if !reflect.DeepEqual(gotArgs[1:], c.wantArgs) {
				t.Fatalf("command args = %v, want %v", gotArgs[1:], c.wantArgs)
			}
		})
	}
}

func TestRunMemberProbe(t *testing.T) {
	t.Run("claude success", func(t *testing.T) {
		setRunner(t, fakeCommandRunner{
			combinedOutputHook: func(cmd *exec.Cmd) ([]byte, error) {
				if cmd.Args[1] != "--setting-sources" {
					t.Fatalf("unexpected claude member probe args: %v", cmd.Args)
				}
				return []byte(`{"type":"result","subtype":"success","is_error":false,"num_turns":1,"result":"Skill loaded"}`), nil
			},
		})

		discovered, reason := runMemberProbe(context.Background(), "claude-code", "/fake/claude", "/fake/target", nil, ReceiptMember{Name: "example"})
		if !discovered {
			t.Fatalf("expected discovered, got reason: %s", reason)
		}
		if reason != "" {
			t.Fatalf("unexpected reason: %s", reason)
		}
	})

	t.Run("claude invalid json", func(t *testing.T) {
		setRunner(t, fakeCommandRunner{
			combinedOutputHook: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte("not json"), nil
			},
		})

		discovered, reason := runMemberProbe(context.Background(), "claude-code", "/fake/claude", "/fake/target", nil, ReceiptMember{Name: "example"})
		if discovered {
			t.Fatal("expected not discovered")
		}
		if !strings.Contains(reason, "invalid JSON") {
			t.Fatalf("reason = %q, want invalid JSON mention", reason)
		}
	})

	t.Run("claude unknown command", func(t *testing.T) {
		setRunner(t, fakeCommandRunner{
			combinedOutputHook: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte(`{"type":"result","subtype":"success","is_error":false,"num_turns":0,"result":"Unknown command: /example"}`), nil
			},
		})

		discovered, reason := runMemberProbe(context.Background(), "claude-code", "/fake/claude", "/fake/target", nil, ReceiptMember{Name: "example"})
		if discovered {
			t.Fatal("expected not discovered")
		}
		if !strings.Contains(reason, "did not recognize") {
			t.Fatalf("reason = %q, want recognition failure mention", reason)
		}
	})

	t.Run("codex success", func(t *testing.T) {
		t.Setenv("CODEX_API_KEY", "test")
		target := t.TempDir()
		skillDir := filepath.Join(target, ".agents", "skills", "example")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		skillContents := []byte("# Example skill\n")
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), skillContents, 0o644); err != nil {
			t.Fatal(err)
		}

		root, err := os.OpenRoot(target)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()

		expected, err := codexSkillChallenge(root, "example")
		if err != nil {
			t.Fatal(err)
		}

		setRunner(t, fakeCommandRunner{
			combinedOutputHook: func(cmd *exec.Cmd) ([]byte, error) {
				if cmd.Args[1] != "exec" {
					t.Fatalf("unexpected codex member probe args: %v", cmd.Args)
				}
				return []byte(fmt.Sprintf(`{"type":"item.completed","item":{"type":"agent_message","text":"%s"}}`, expected)), nil
			},
		})

		discovered, reason := runMemberProbe(context.Background(), "codex", "/fake/codex", target, root, ReceiptMember{Name: "example"})
		if !discovered {
			t.Fatalf("expected discovered, got reason: %s", reason)
		}
		if reason != "" {
			t.Fatalf("unexpected reason: %s", reason)
		}
	})

	t.Run("codex wrong digest", func(t *testing.T) {
		t.Setenv("CODEX_API_KEY", "test")
		target := t.TempDir()
		skillDir := filepath.Join(target, ".agents", "skills", "example")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Example skill\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		root, err := os.OpenRoot(target)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()

		expected, err := codexSkillChallenge(root, "example")
		if err != nil {
			t.Fatal(err)
		}

		setRunner(t, fakeCommandRunner{
			combinedOutputHook: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte(`{"type":"item.completed","item":{"type":"agent_message","text":"wrongdigest"}}`), nil
			},
		})

		discovered, reason := runMemberProbe(context.Background(), "codex", "/fake/codex", target, root, ReceiptMember{Name: "example"})
		if discovered {
			t.Fatal("expected not discovered")
		}
		if !strings.Contains(reason, expected) || !strings.Contains(reason, "wrongdigest") {
			t.Fatalf("reason = %q, want expected %q and wrongdigest", reason, expected)
		}
	})

	t.Run("codex command error", func(t *testing.T) {
		t.Setenv("CODEX_API_KEY", "test")
		target := t.TempDir()
		skillDir := filepath.Join(target, ".agents", "skills", "example")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Example skill\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		root, err := os.OpenRoot(target)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()

		setRunner(t, fakeCommandRunner{
			combinedOutputHook: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte("codex crashed"), errors.New("exit status 1")
			},
		})

		discovered, reason := runMemberProbe(context.Background(), "codex", "/fake/codex", target, root, ReceiptMember{Name: "example"})
		if discovered {
			t.Fatal("expected not discovered")
		}
		if reason != "codex crashed" {
			t.Fatalf("reason = %q, want codex crashed", reason)
		}
	})

	t.Run("codex missing skill challenge", func(t *testing.T) {
		t.Setenv("CODEX_API_KEY", "test")
		target := t.TempDir()
		root, err := os.OpenRoot(target)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()

		discovered, reason := runMemberProbe(context.Background(), "codex", "/fake/codex", target, root, ReceiptMember{Name: "missing"})
		if discovered {
			t.Fatal("expected not discovered")
		}
		if !strings.Contains(reason, "SKILL.md") {
			t.Fatalf("reason = %q, want SKILL.md mention", reason)
		}
	})
}

func fakeExecutable(t *testing.T, name string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte{}, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestCommandProberProbe(t *testing.T) {
	t.Setenv("CODEX_API_KEY", "test")
	fakeExecutable(t, "claude")
	fakeExecutable(t, "codex")

	target := t.TempDir()

	claudeViewDir := filepath.Join(target, "claude-view")
	if err := os.MkdirAll(claudeViewDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeViewDir, "SKILL.md"), []byte("# Claude skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	claudeFiles, err := readBoundary(claudeViewDir)
	if err != nil {
		t.Fatal(err)
	}
	claudeHash := hashFiles(claudeFiles)

	codexViewDir := filepath.Join(target, "codex-view", "agents")
	if err := os.MkdirAll(codexViewDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexViewDir, "openai.yaml"), []byte("policy:\n  allow_implicit_invocation: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	codexFiles, err := readBoundary(filepath.Dir(codexViewDir))
	if err != nil {
		t.Fatal(err)
	}
	codexHash := hashFiles(codexFiles)

	skillDir := filepath.Join(target, ".agents", "skills", "example")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Example skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root, err := os.OpenRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	expectedChallenge, err := codexSkillChallenge(root, "example")
	if err != nil {
		t.Fatal(err)
	}

	setRunner(t, fakeCommandRunner{
		runHook: func(cmd *exec.Cmd) error {
			if len(cmd.Args) >= 3 && cmd.Args[1] == "auth" && cmd.Args[2] == "status" {
				return nil
			}
			if len(cmd.Args) >= 3 && cmd.Args[1] == "login" && cmd.Args[2] == "status" {
				return nil
			}
			return fmt.Errorf("unexpected auth command: %v", cmd.Args)
		},
		combinedOutputHook: func(cmd *exec.Cmd) ([]byte, error) {
			if len(cmd.Args) < 2 {
				return nil, fmt.Errorf("unexpected empty command: %v", cmd.Args)
			}
			switch cmd.Args[1] {
			case "--version":
				return []byte("1.0.0\n"), nil
			case "--setting-sources":
				return []byte(`{"type":"result","subtype":"success","is_error":false,"num_turns":1,"result":"Skill loaded"}`), nil
			case "exec":
				return []byte(fmt.Sprintf(`{"type":"item.completed","item":{"type":"agent_message","text":"%s"}}`, expectedChallenge)), nil
			}
			return nil, fmt.Errorf("unexpected command: %v", cmd.Args)
		},
	})

	receipt := WorkspaceReceipt{
		Members: []ReceiptMember{
			{
				Name: "example",
				Views: []RenderedView{
					{Tool: "claude-code", RelativeDestination: "claude-view", RenderedContentSHA256: claudeHash},
					{Tool: "codex", RelativeDestination: "codex-view", RenderedContentSHA256: codexHash},
				},
			},
		},
	}

	prober := CommandProber{Timeout: 5 * time.Second, Concurrency: 2}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	results := prober.Probe(ctx, target, root, receipt)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	for _, result := range results {
		t.Run(result.Tool, func(t *testing.T) {
			if result.Executable == "" {
				t.Fatal("expected executable path")
			}
			if result.Version != "1.0.0" {
				t.Fatalf("version = %q, want 1.0.0", result.Version)
			}
			if !result.Authenticated {
				t.Fatalf("expected authenticated, reason: %s", result.Reason)
			}
			if !result.StaticVerified {
				t.Fatalf("expected static verified, reason: %s", result.Reason)
			}
			if !result.RuntimeVerified {
				t.Fatalf("expected runtime verified, reason: %s, next action: %s", result.Reason, result.NextAction)
			}
			if len(result.MemberProbes) != 1 {
				t.Fatalf("member probes = %v", result.MemberProbes)
			}
			if !result.MemberProbes[0].Discovered {
				t.Fatalf("member probe not discovered: %s", result.MemberProbes[0].Reason)
			}
			if result.Reason != "" {
				t.Fatalf("unexpected reason: %s", result.Reason)
			}
			if result.NextAction != "" {
				t.Fatalf("unexpected next action: %s", result.NextAction)
			}
		})
	}
}
