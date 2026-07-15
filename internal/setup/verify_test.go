package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
