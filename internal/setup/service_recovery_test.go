package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jnyross/Skill-Manager/internal/catalog"
)

func TestApplyRecoversStaleStagingLock(t *testing.T) {
	target := t.TempDir()
	svc := NewService()
	plan, err := svc.Plan(context.Background(), Request{
		TargetPath: target, CatalogVersion: "test.1", Members: []ResolvedMember{testResolvedMember(t, "probe")},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	opID := operationIdentity(plan.desired)
	stageRoot := filepath.Join(target, ".skillet", "staging", opID)
	lockPath := filepath.Join(stageRoot, "staging.lock")
	if err := os.MkdirAll(stageRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	lockContents := fmt.Sprintf("pid: %d\nstarted: %s\n", 999999, time.Now().UTC().Add(-30*time.Minute).Format(time.RFC3339))
	if err := os.WriteFile(lockPath, []byte(lockContents), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := svc.Apply(context.Background(), plan)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Outcome != OutcomeConfiguredUnverified {
		t.Fatalf("outcome = %q, want Configured-unverified", result.Outcome)
	}
	if _, statErr := os.Stat(stageRoot); !os.IsNotExist(statErr) {
		t.Fatalf("stale staging was not removed: %v", statErr)
	}
}

func TestApplyRecoversMissingStagingLock(t *testing.T) {
	target := t.TempDir()
	svc := NewService()
	plan, err := svc.Plan(context.Background(), Request{
		TargetPath: target, CatalogVersion: "test.1", Members: []ResolvedMember{testResolvedMember(t, "probe")},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	opID := operationIdentity(plan.desired)
	stageRoot := filepath.Join(target, ".skillet", "staging", opID)
	if err := os.MkdirAll(stageRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stageRoot, "leftover.txt"), []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := svc.Apply(context.Background(), plan)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Outcome != OutcomeConfiguredUnverified {
		t.Fatalf("outcome = %q, want Configured-unverified", result.Outcome)
	}
	if _, statErr := os.Stat(stageRoot); !os.IsNotExist(statErr) {
		t.Fatalf("stale staging was not removed: %v", statErr)
	}
}

func TestApplyBlocksLiveStagingLock(t *testing.T) {
	target := t.TempDir()
	svc := NewService()
	plan, err := svc.Plan(context.Background(), Request{
		TargetPath: target, CatalogVersion: "test.1", Members: []ResolvedMember{testResolvedMember(t, "probe")},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	opID := operationIdentity(plan.desired)
	stageRoot := filepath.Join(target, ".skillet", "staging", opID)
	lockPath := filepath.Join(stageRoot, "staging.lock")
	if err := os.MkdirAll(stageRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	lockContents := fmt.Sprintf("pid: %d\nstarted: %s\n", cmd.Process.Pid, time.Now().UTC().Format(time.RFC3339))
	if err := os.WriteFile(lockPath, []byte(lockContents), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := svc.Apply(context.Background(), plan)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Outcome != OutcomeBlocked {
		t.Fatalf("outcome = %q, want Blocked", result.Outcome)
	}
	if !strings.Contains(result.NextAction, "locked by another running Skillet process") {
		t.Fatalf("nextAction = %q, want lock message", result.NextAction)
	}
	if _, statErr := os.Stat(lockPath); err != nil {
		t.Fatalf("live lock staging was unexpectedly removed: %v", statErr)
	}
}

func TestApplyCancelsBetweenPromotionSteps(t *testing.T) {
	target := filepath.Join(t.TempDir(), "notyet")
	ctx, cancel := context.WithCancel(context.Background())
	var cancelled bool
	svc := NewServiceWith(nil, ApplyHooks{BeforePromote: func(relative string) error {
		if relative == ".agents/skills/probe/SKILL.md" && !cancelled {
			cancelled = true
			cancel()
		}
		return nil
	}})

	plan, err := svc.Plan(ctx, Request{
		TargetPath: target, CatalogVersion: "test.1", Members: []ResolvedMember{testResolvedMember(t, "probe")},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	result, err := svc.Apply(ctx, plan)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if result.Outcome != OutcomeBlocked {
		t.Fatalf("outcome = %q, want Blocked", result.Outcome)
	}
	if _, statErr := os.Stat(filepath.Join(target, ".skillet")); !os.IsNotExist(statErr) {
		t.Fatalf("cancelled apply did not roll back workspace: %v", statErr)
	}
}

func testResolvedMember(t *testing.T, name string) ResolvedMember {
	t.Helper()
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: Probe\n---\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return ResolvedMember{
		Member: catalog.Member{
			Name:               name,
			UpstreamActivation: "auto",
			Source:             catalog.Source{Repository: "fixture", Subpath: "skills/" + name, ReviewedRevision: strings.Repeat("a", 40), ContentSHA256: strings.Repeat("a", 64)},
			Recipes: []catalog.Recipe{
				{Tool: "claude-code", Scope: "project", Artifact: "direct-skill"},
				{Tool: "codex", Scope: "project", Artifact: "direct-skill"},
			},
		},
		SourceDir: source,
		Evidence:  BoundaryEvidence{Revision: strings.Repeat("a", 40), Subpath: "skills/" + name, ContentSHA256: strings.Repeat("a", 64)},
	}
}
