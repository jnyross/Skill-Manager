package setup_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/catalog"
	"github.com/jnyross/Skill-Manager/internal/setup"
)

func TestServiceCreatesAgentReadyWorkspaceAndConfiguredUnverifiedReceipt(t *testing.T) {
	target := filepath.Join(t.TempDir(), "new project ü")
	service := setup.NewService()
	plan, err := service.Plan(context.Background(), setup.Request{
		TargetPath: target, CatalogVersion: "test.1", BundleIDs: []string{"probe-bundle"},
		Members: []setup.ResolvedMember{resolvedProbe(t)},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	result, err := service.Apply(context.Background(), plan)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Outcome != setup.OutcomeConfiguredUnverified {
		t.Fatalf("outcome = %q, want Configured-unverified", result.Outcome)
	}
	for _, relative := range []string{
		".git", "AGENTS.md", "CLAUDE.md", ".gitignore", ".skillet/workspace.json",
		".skillet/workspace.local.json", ".skillet/managed/skills/probe/SKILL.md",
		".claude/skills/probe/SKILL.md", ".agents/skills/probe/SKILL.md",
	} {
		if _, err := os.Lstat(filepath.Join(target, relative)); err != nil {
			t.Errorf("%s missing: %v", relative, err)
		}
	}
	if _, err := os.Stat(filepath.Join(target, ".git", "index")); !os.IsNotExist(err) {
		t.Fatalf("setup staged files: .git/index stat = %v", err)
	}
}

func TestServiceBlocksUnmanagedConflictWithoutMutation(t *testing.T) {
	target := t.TempDir()
	writeFile(t, filepath.Join(target, "AGENTS.md"), "user instructions\n")
	service := setup.NewService()
	plan, err := service.Plan(context.Background(), setup.Request{
		TargetPath: target, CatalogVersion: "test.1", BundleIDs: []string{"probe-bundle"}, Members: []setup.ResolvedMember{resolvedProbe(t)},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Blockers) == 0 {
		t.Fatal("conflicting AGENTS.md did not block")
	}
	result, err := service.Apply(context.Background(), plan)
	if err != nil {
		t.Fatalf("Apply blocked plan: %v", err)
	}
	if result.Outcome != setup.OutcomeBlocked {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	contents, _ := os.ReadFile(filepath.Join(target, "AGENTS.md"))
	if string(contents) != "user instructions\n" {
		t.Fatal("conflicting user instructions changed")
	}
	if _, err := os.Stat(filepath.Join(target, ".skillet")); !os.IsNotExist(err) {
		t.Fatalf("blocked setup mutated workspace: %v", err)
	}
}

func TestServiceBacksUpAuthorizedConflictAndNoOpsOnRepeat(t *testing.T) {
	target := t.TempDir()
	writeFile(t, filepath.Join(target, "AGENTS.md"), "user instructions\n")
	service := setup.NewService()
	request := setup.Request{
		TargetPath: target, CatalogVersion: "test.1", BundleIDs: []string{"probe-bundle"},
		Members: []setup.ResolvedMember{resolvedProbe(t)}, ReplaceConflicts: true,
	}
	plan, err := service.Plan(context.Background(), request)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	result, err := service.Apply(context.Background(), plan)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(result.Backups) == 0 {
		t.Fatal("authorized replacement made no backup")
	}
	backupContents, err := os.ReadFile(filepath.Join(target, result.Backups[0]))
	if err != nil || string(backupContents) != "user instructions\n" {
		t.Fatalf("backup = %q, %v", backupContents, err)
	}

	repeat, err := service.Plan(context.Background(), request)
	if err != nil {
		t.Fatalf("repeat Plan: %v", err)
	}
	if !repeat.NoOp {
		t.Fatalf("unchanged repeat is not a no-op: %#v", repeat.Changes)
	}
	repeatResult, err := service.Apply(context.Background(), repeat)
	if err != nil || repeatResult.Outcome != setup.OutcomeConfiguredUnverified {
		t.Fatalf("repeat result = %#v, %v", repeatResult, err)
	}
}

func TestServiceDeclinedMaterialDriftBlocksBeforeWrites(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	member := resolvedProbe(t)
	member.Drift = setup.DriftReview{Material: true, Changes: []setup.DriftChange{{Class: setup.DriftContent}}}
	service := setup.NewService()
	plan, err := service.Plan(context.Background(), setup.Request{TargetPath: target, CatalogVersion: "test.1", Members: []setup.ResolvedMember{member}})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Blockers) == 0 {
		t.Fatal("declined material drift did not block")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("planning drift created target: %v", err)
	}
}

func TestServicePlansRecoverableMemberRemovalAndActivationOnlyUpdate(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	probe := resolvedNamed(t, "probe")
	second := resolvedNamed(t, "second")
	service := setup.NewService()
	initial := setup.Request{TargetPath: target, CatalogVersion: "test.1", BundleIDs: []string{"both"}, Members: []setup.ResolvedMember{probe, second}}
	plan, err := service.Plan(context.Background(), initial)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Apply(context.Background(), plan); err != nil {
		t.Fatal(err)
	}

	updated := setup.Request{
		TargetPath: target, CatalogVersion: "test.1", BundleIDs: []string{"probe-only"}, Members: []setup.ResolvedMember{probe},
		Activation: map[string]setup.Activation{"probe": setup.ActivationManualOnly},
	}
	plan, err = service.Plan(context.Background(), updated)
	if err != nil {
		t.Fatal(err)
	}
	var sawRemoval, sawActivation bool
	for _, change := range plan.Changes {
		if change.State == setup.ChangeManagedRemove && strings.Contains(change.Path, "second") {
			sawRemoval = true
		}
		if change.State == setup.ChangeManagedUpdate && strings.Contains(change.Path, "probe") {
			sawActivation = true
		}
	}
	if !sawRemoval || !sawActivation {
		t.Fatalf("plan missed removal/activation update: %#v", plan.Changes)
	}
	result, err := service.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Backups) == 0 {
		t.Fatal("removed managed member had no recovery backup")
	}
	for _, relative := range []string{".claude/skills/second/SKILL.md", ".agents/skills/second/SKILL.md", ".skillet/managed/skills/second/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(target, relative)); !os.IsNotExist(err) {
			t.Errorf("removed path %s still exists: %v", relative, err)
		}
	}
	claudeSkill, _ := os.ReadFile(filepath.Join(target, ".claude", "skills", "probe", "SKILL.md"))
	if !strings.Contains(string(claudeSkill), "disable-model-invocation: true") {
		t.Fatal("activation-only update did not render Claude overlay")
	}
}

func TestServiceRejectsEscapingManagedPathFromPriorReceipt(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "project")
	if err := os.MkdirAll(filepath.Join(target, ".skillet"), 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(parent, "sentinel")
	writeFile(t, sentinel, "keep\n")
	writeFile(t, filepath.Join(target, ".skillet", "workspace.json"), `{"schemaVersion":1,"catalogVersion":"old","managedPaths":[{"path":"../sentinel","sha256":"forged"}]}`+"\n")
	_, err := setup.NewService().Plan(context.Background(), setup.Request{TargetPath: target, CatalogVersion: "test.1", Members: []setup.ResolvedMember{resolvedProbe(t)}})
	if err == nil || !strings.Contains(err.Error(), "unsafe managed path") {
		t.Fatalf("Plan error = %v, want unsafe managed path", err)
	}
	contents, _ := os.ReadFile(sentinel)
	if string(contents) != "keep\n" {
		t.Fatal("escaping receipt changed sentinel")
	}
}

func TestServiceBlocksSymlinkedManagedDestinationAncestor(t *testing.T) {
	target := t.TempDir()
	external := t.TempDir()
	if err := os.Symlink(external, filepath.Join(target, ".agents")); err != nil {
		t.Fatal(err)
	}
	plan, err := setup.NewService().Plan(context.Background(), setup.Request{TargetPath: target, CatalogVersion: "test.1", Members: []setup.ResolvedMember{resolvedProbe(t)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Blockers) == 0 || !strings.Contains(strings.Join(plan.Blockers, " "), "crosses symlink") {
		t.Fatalf("blockers = %v, want symlink blocker", plan.Blockers)
	}
	if _, err := os.Stat(filepath.Join(external, "skills", "probe", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("external destination changed: %v", err)
	}
}

func TestServiceAnchoredApplyCannotBeRedirectedByPromotionHook(t *testing.T) {
	target := t.TempDir()
	external := t.TempDir()
	service := setup.NewServiceWith(nil, setup.ApplyHooks{BeforePromote: func(relative string) error {
		if relative == ".agents/skills/probe/SKILL.md" {
			if err := os.RemoveAll(filepath.Join(target, ".agents")); err != nil {
				return err
			}
			return os.Symlink(external, filepath.Join(target, ".agents"))
		}
		return nil
	}})
	plan, err := service.Plan(context.Background(), setup.Request{TargetPath: target, CatalogVersion: "test.1", Members: []setup.ResolvedMember{resolvedProbe(t)}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Apply(context.Background(), plan)
	if err == nil || result.Outcome != setup.OutcomeBlocked {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	if _, statErr := os.Stat(filepath.Join(external, "skills", "probe", "SKILL.md")); !os.IsNotExist(statErr) {
		t.Fatalf("anchored apply wrote outside target: %v", statErr)
	}
}

func TestServiceReportsMissingRequiredRollbackSnapshotAndRetainsBackup(t *testing.T) {
	target := t.TempDir()
	writeFile(t, filepath.Join(target, ".gitignore"), "user rule\n")
	service := setup.NewServiceWith(nil, setup.ApplyHooks{BeforePromote: func(relative string) error {
		if relative != ".skillet/managed/skills/probe/SKILL.md" {
			return nil
		}
		matches, _ := filepath.Glob(filepath.Join(target, ".skillet", "staging", "*", "rollback", ".gitignore"))
		for _, match := range matches {
			if err := os.Remove(match); err != nil {
				return err
			}
		}
		return errors.New("trigger rollback after snapshot removal")
	}})
	plan, err := service.Plan(context.Background(), setup.Request{TargetPath: target, CatalogVersion: "test.1", Members: []setup.ResolvedMember{resolvedProbe(t)}, ReplaceConflicts: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Apply(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "restore prior .gitignore") || !strings.Contains(result.NextAction, "retained backups") {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	if len(result.Backups) == 0 {
		t.Fatal("incomplete rollback did not report retained backup")
	}
	contents, backupErr := os.ReadFile(filepath.Join(target, filepath.FromSlash(result.Backups[0])))
	if backupErr != nil || string(contents) != "user rule\n" {
		t.Fatalf("retained backup = %q, %v", contents, backupErr)
	}
}

func TestServicePreservesExecutableMemberFiles(t *testing.T) {
	priorUmask := syscall.Umask(0o077)
	t.Cleanup(func() { syscall.Umask(priorUmask) })
	target := filepath.Join(t.TempDir(), "target")
	member := resolvedProbe(t)
	script := filepath.Join(member.SourceDir, "scripts", "run.sh")
	writeFile(t, script, "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	plan, err := setup.NewService().Plan(context.Background(), setup.Request{TargetPath: target, CatalogVersion: "test.1", Members: []setup.ResolvedMember{member}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := setup.NewService().Apply(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{".skillet/managed/skills/probe/scripts/run.sh", ".claude/skills/probe/scripts/run.sh", ".agents/skills/probe/scripts/run.sh"} {
		info, err := os.Stat(filepath.Join(target, filepath.FromSlash(relative)))
		if err != nil || info.Mode().Perm() != 0o755 {
			t.Fatalf("%s mode = %v, %v; want 0755", relative, info, err)
		}
	}
	for _, relative := range []string{".claude/skills/probe/scripts/run.sh", ".agents/skills/probe/scripts/run.sh"} {
		if err := os.Chmod(filepath.Join(target, filepath.FromSlash(relative)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	repeat, err := setup.NewService().Plan(context.Background(), setup.Request{TargetPath: target, CatalogVersion: "test.1", Members: []setup.ResolvedMember{member}})
	if err != nil {
		t.Fatal(err)
	}
	if len(repeat.Blockers) == 0 || !strings.Contains(strings.Join(repeat.Blockers, " "), "edited-managed") {
		t.Fatalf("chmod drift blockers = %v", repeat.Blockers)
	}
}

func TestServiceRehydratesMissingLocalReceiptFromCommittedPortableReceipt(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	request := setup.Request{TargetPath: target, CatalogVersion: "test.1", Members: []setup.ResolvedMember{resolvedProbe(t)}}
	service := setup.NewService()
	plan, err := service.Plan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Apply(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-C", target, "add", "."},
		{"-C", target, "-c", "user.name=Skillet Test", "-c", "user.email=skillet@example.invalid", "commit", "-m", "managed workspace"},
	} {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	localPath := filepath.Join(target, ".skillet", "workspace.local.json")
	if err := os.Remove(localPath); err != nil {
		t.Fatal(err)
	}
	rehydrate, err := service.Plan(context.Background(), request)
	if err != nil {
		t.Fatalf("fresh-clone Plan: %v", err)
	}
	if _, err := service.Apply(context.Background(), rehydrate); err != nil {
		t.Fatalf("fresh-clone Apply: %v", err)
	}
	if _, err := os.Stat(localPath); err != nil {
		t.Fatalf("local receipt was not rehydrated: %v", err)
	}
	if err := exec.Command("git", "-C", target, "diff", "--quiet", "HEAD", "--", ".skillet/workspace.json").Run(); err != nil {
		t.Fatal("rehydrating local state dirtied the portable receipt")
	}
}

func TestServiceRuntimeDetailRefreshDoesNotChangePortableReceipt(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	request := setup.Request{TargetPath: target, CatalogVersion: "test.1", Members: []setup.ResolvedMember{resolvedProbe(t)}}
	results := func(version string) []setup.ToolResult {
		return []setup.ToolResult{
			{Tool: "claude-code", StaticVerified: true, Version: version},
			{Tool: "codex", StaticVerified: true, Version: version},
		}
	}
	first := setup.NewServiceWith(proberFunc(func(context.Context, string, setup.WorkspaceReceipt) []setup.ToolResult { return results("one") }), setup.ApplyHooks{})
	plan, err := first.Plan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Apply(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	portablePath := filepath.Join(target, ".skillet", "workspace.json")
	before, _ := os.ReadFile(portablePath)
	second := setup.NewServiceWith(proberFunc(func(context.Context, string, setup.WorkspaceReceipt) []setup.ToolResult { return results("two") }), setup.ApplyHooks{})
	repeat, err := second.Plan(context.Background(), request)
	if err != nil || !repeat.NoOp {
		t.Fatalf("repeat=%#v err=%v", repeat, err)
	}
	if _, err := second.Apply(context.Background(), repeat); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(portablePath)
	if string(after) != string(before) {
		t.Fatal("machine-local runtime detail refresh changed portable receipt")
	}
}

func TestServiceBindsExistingTargetBeforeGitInitialization(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	service := setup.NewService()
	plan, err := service.Plan(context.Background(), setup.Request{TargetPath: target, CatalogVersion: "test.1", Members: []setup.ResolvedMember{resolvedProbe(t)}})
	if err != nil || !plan.NeedGitInit {
		t.Fatalf("plan=%#v err=%v", plan, err)
	}
	original := filepath.Join(parent, "original")
	if err := os.Rename(target, original); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(parent, "external")
	if err := os.Mkdir(external, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, target); err != nil {
		t.Fatal(err)
	}
	result, err := service.Apply(context.Background(), plan)
	if err == nil || result.Outcome != setup.OutcomeBlocked {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if _, err := os.Stat(filepath.Join(external, ".git")); !os.IsNotExist(err) {
		t.Fatalf("git init escaped into replacement target: %v", err)
	}
}

func TestServiceDoesNotDeleteConcurrentlyCreatedGitMetadata(t *testing.T) {
	target := t.TempDir()
	service := setup.NewService()
	plan, err := service.Plan(context.Background(), setup.Request{TargetPath: target, CatalogVersion: "test.1", Members: []setup.ResolvedMember{resolvedProbe(t)}})
	if err != nil || !plan.NeedGitInit {
		t.Fatalf("plan=%#v err=%v", plan, err)
	}
	if err := os.Mkdir(filepath.Join(target, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(target, ".git", "sentinel")
	writeFile(t, sentinel, "preserve\n")
	result, err := service.Apply(context.Background(), plan)
	if err == nil || result.Outcome != setup.OutcomeBlocked {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	contents, readErr := os.ReadFile(sentinel)
	if readErr != nil || string(contents) != "preserve\n" {
		t.Fatalf("concurrent Git metadata was removed: %q, %v", contents, readErr)
	}
}

func TestServiceRejectsTargetSwapDuringVerification(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(parent, "external")
	if err := os.Mkdir(external, 0o755); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(parent, "moved")
	service := setup.NewServiceWith(proberFunc(func(_ context.Context, _ string, _ setup.WorkspaceReceipt) []setup.ToolResult {
		if err := os.Rename(target, moved); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(external, target); err != nil {
			t.Fatal(err)
		}
		return []setup.ToolResult{{Tool: "claude-code", StaticVerified: true, RuntimeVerified: true}, {Tool: "codex", StaticVerified: true, RuntimeVerified: true}}
	}), setup.ApplyHooks{})
	plan, err := service.Plan(context.Background(), setup.Request{TargetPath: target, CatalogVersion: "test.1", Members: []setup.ResolvedMember{resolvedProbe(t)}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Apply(context.Background(), plan)
	if err == nil || result.Outcome != setup.OutcomeBlocked || !strings.Contains(err.Error(), "target identity changed during verification") {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if _, err := os.Stat(filepath.Join(external, ".skillet")); !os.IsNotExist(err) {
		t.Fatalf("replacement target was modified: %v", err)
	}
}

func TestServiceDetectsEditedPortableAndLocalReceipts(t *testing.T) {
	for _, receiptName := range []string{"workspace.json", "workspace.local.json"} {
		t.Run(receiptName, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "target")
			member := resolvedProbe(t)
			service := setup.NewService()
			plan, err := service.Plan(context.Background(), setup.Request{TargetPath: target, CatalogVersion: "test.1", Members: []setup.ResolvedMember{member}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.Apply(context.Background(), plan); err != nil {
				t.Fatal(err)
			}
			filename := filepath.Join(target, ".skillet", receiptName)
			contents, _ := os.ReadFile(filename)
			edited := strings.Replace(string(contents), "Configured-unverified", "Verified", 1)
			if receiptName == "workspace.json" {
				edited = strings.Replace(string(contents), "test.1", "tampered", 1)
			}
			if err := os.WriteFile(filename, []byte(edited), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err = service.Plan(context.Background(), setup.Request{TargetPath: target, CatalogVersion: "test.1", Members: []setup.ResolvedMember{member}})
			if err == nil || !strings.Contains(err.Error(), "receipt was edited") {
				t.Fatalf("Plan error = %v, want edited receipt", err)
			}
		})
	}
}

func TestServiceRetainsConsecutiveReceiptTransitionHistory(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	probe := resolvedNamed(t, "probe")
	second := resolvedNamed(t, "second")
	service := setup.NewService()
	requests := []setup.Request{
		{TargetPath: target, CatalogVersion: "test.1", BundleIDs: []string{"both"}, Members: []setup.ResolvedMember{probe, second}},
		{TargetPath: target, CatalogVersion: "test.2", BundleIDs: []string{"probe"}, Members: []setup.ResolvedMember{probe}, Activation: map[string]setup.Activation{"probe": setup.ActivationManualOnly}},
		{TargetPath: target, CatalogVersion: "test.3", BundleIDs: []string{"probe"}, Members: []setup.ResolvedMember{probe}, Activation: map[string]setup.Activation{"probe": setup.ActivationAuto}},
	}
	for _, request := range requests {
		plan, err := service.Plan(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.Apply(context.Background(), plan); err != nil {
			t.Fatal(err)
		}
	}
	var receipt setup.WorkspaceReceipt
	contents, _ := os.ReadFile(filepath.Join(target, ".skillet", "workspace.json"))
	if err := json.Unmarshal(contents, &receipt); err != nil {
		t.Fatal(err)
	}
	if len(receipt.History) != 2 || len(receipt.History[0].Members) != 2 || receipt.History[1].Members[0].Activation != setup.ActivationManualOnly {
		t.Fatalf("history = %#v", receipt.History)
	}
}

func TestServiceRollsBackFinalReceiptWriteFailure(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	service := setup.NewServiceWith(proberFunc(func(_ context.Context, target string, _ setup.WorkspaceReceipt) []setup.ToolResult {
		local := filepath.Join(target, ".skillet", "workspace.local.json")
		if err := os.Remove(local); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(local, 0o755); err != nil {
			t.Fatal(err)
		}
		return []setup.ToolResult{{Tool: "claude-code", StaticVerified: true}, {Tool: "codex", StaticVerified: true}}
	}), setup.ApplyHooks{})
	plan, err := service.Plan(context.Background(), setup.Request{TargetPath: target, CatalogVersion: "test.1", Members: []setup.ResolvedMember{resolvedProbe(t)}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Apply(context.Background(), plan)
	if err == nil || result.Outcome != setup.OutcomeBlocked {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("final receipt failure left target: %v", statErr)
	}
}

func TestServiceRestoresReceiptPairWhenNoOpLocalWriteFails(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	request := setup.Request{TargetPath: target, CatalogVersion: "test.1", Members: []setup.ResolvedMember{resolvedProbe(t)}}
	service := setup.NewService()
	plan, err := service.Plan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Apply(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	portablePath := filepath.Join(target, ".skillet", "workspace.json")
	localPath := filepath.Join(target, ".skillet", "workspace.local.json")
	portableBefore, _ := os.ReadFile(portablePath)
	localBefore, _ := os.ReadFile(localPath)

	failing := setup.NewServiceWith(proberFunc(func(_ context.Context, _ string, _ setup.WorkspaceReceipt) []setup.ToolResult {
		if err := os.Remove(localPath); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(localPath, 0o755); err != nil {
			t.Fatal(err)
		}
		return []setup.ToolResult{{Tool: "claude-code", StaticVerified: true}, {Tool: "codex", StaticVerified: true}}
	}), setup.ApplyHooks{})
	repeat, err := failing.Plan(context.Background(), request)
	if err != nil || !repeat.NoOp {
		t.Fatalf("repeat plan = %#v, err = %v", repeat, err)
	}
	result, err := failing.Apply(context.Background(), repeat)
	if err == nil || result.Outcome != setup.OutcomeBlocked {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	portableAfter, portableErr := os.ReadFile(portablePath)
	localAfter, localErr := os.ReadFile(localPath)
	if portableErr != nil || localErr != nil || string(portableAfter) != string(portableBefore) || string(localAfter) != string(localBefore) {
		t.Fatalf("receipt pair not restored: portableErr=%v localErr=%v portableEqual=%t localEqual=%t", portableErr, localErr, string(portableAfter) == string(portableBefore), string(localAfter) == string(localBefore))
	}
}

func TestServiceRollsBackWhenStaticVerificationFails(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	service := setup.NewServiceWith(proberFunc(func(context.Context, string, setup.WorkspaceReceipt) []setup.ToolResult {
		return []setup.ToolResult{{Tool: "claude-code", StaticVerified: false, Reason: "bad view"}, {Tool: "codex", StaticVerified: true}}
	}), setup.ApplyHooks{})
	plan, err := service.Plan(context.Background(), setup.Request{TargetPath: target, CatalogVersion: "test.1", Members: []setup.ResolvedMember{resolvedProbe(t)}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Apply(context.Background(), plan)
	if err != nil || result.Outcome != setup.OutcomeBlocked {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("static verification failure left target: %v", statErr)
	}
}

func TestServiceRollsBackEveryManagedWriteAfterInjectedPromotionFailure(t *testing.T) {
	target := filepath.Join(t.TempDir(), "new-target")
	service := setup.NewServiceWith(nil, setup.ApplyHooks{BeforePromote: func(relative string) error {
		if relative == "CLAUDE.md" {
			return errors.New("injected promotion failure")
		}
		return nil
	}})
	plan, err := service.Plan(context.Background(), setup.Request{TargetPath: target, CatalogVersion: "test.1", Members: []setup.ResolvedMember{resolvedProbe(t)}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Apply(context.Background(), plan)
	if err == nil || result.Outcome != setup.OutcomeBlocked {
		t.Fatalf("result = %#v, err=%v", result, err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("failed new-workspace transaction left target behind: %v", statErr)
	}
}

func TestServiceReportsPartialOnlyForUnreversedExternalChange(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "target")
	external := filepath.Join(parent, "external-side-effect")
	service := setup.NewServiceWith(nil, setup.ApplyHooks{ExternalAction: func(context.Context, string) (func() error, string, error) {
		if err := os.WriteFile(external, []byte("changed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return func() error { return errors.New("cannot undo external state") }, "Remove external-side-effect manually", errors.New("external action failed")
	}})
	plan, err := service.Plan(context.Background(), setup.Request{TargetPath: target, CatalogVersion: "test.1", Members: []setup.ResolvedMember{resolvedProbe(t)}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Apply(context.Background(), plan)
	if err != nil || result.Outcome != setup.OutcomePartial || result.NextAction != "Remove external-side-effect manually" {
		t.Fatalf("result = %#v, err=%v", result, err)
	}
	if _, err := os.Stat(external); err != nil {
		t.Fatalf("simulated unreversed external state missing: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("managed workspace was not rolled back: %v", err)
	}
}

func TestServiceReportsVerifiedOnlyWhenBothToolProbesSucceed(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	service := setup.NewServiceWith(proberFunc(func(context.Context, string, setup.WorkspaceReceipt) []setup.ToolResult {
		return []setup.ToolResult{
			{Tool: "claude-code", StaticVerified: true, Authenticated: true, RuntimeVerified: true},
			{Tool: "codex", StaticVerified: true, Authenticated: true, RuntimeVerified: true},
		}
	}), setup.ApplyHooks{})
	plan, err := service.Plan(context.Background(), setup.Request{TargetPath: target, CatalogVersion: "test.1", Members: []setup.ResolvedMember{resolvedProbe(t)}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Apply(context.Background(), plan)
	if err != nil || result.Outcome != setup.OutcomeVerified {
		t.Fatalf("result = %#v, err=%v", result, err)
	}
}

type proberFunc func(context.Context, string, setup.WorkspaceReceipt) []setup.ToolResult

func (function proberFunc) Probe(ctx context.Context, target string, _ *os.Root, receipt setup.WorkspaceReceipt) []setup.ToolResult {
	return function(ctx, target, receipt)
}

func resolvedProbe(t *testing.T) setup.ResolvedMember {
	return resolvedNamed(t, "probe")
}

func resolvedNamed(t *testing.T, name string) setup.ResolvedMember {
	t.Helper()
	source := t.TempDir()
	writeFile(t, filepath.Join(source, "SKILL.md"), "---\nname: "+name+"\ndescription: Probe\n---\nBody\n")
	return setup.ResolvedMember{
		Member: catalog.Member{
			Name: name, Family: "fixture", UpstreamActivation: "auto", VerificationPrompt: "Return only SKILLET_DISCOVERED_" + name + ".",
			Source: catalog.Source{
				Repository: "fixture", Subpath: "skills/" + name, ReviewedRevision: digest('a')[:40],
				ContentSHA256: digest('a'), MetadataSHA256: digest('b'), DependencyEvidenceSHA256: digest('c'), ExternalActionEvidenceSHA256: digest('d'),
			},
			License: catalog.License{SPDX: "MIT", Notice: "LICENSE", NoticeSHA256: digest('b'), Evidence: "license-text"},
			Recipes: []catalog.Recipe{{Tool: "claude-code", Scope: "project", Artifact: "direct-skill"}, {Tool: "codex", Scope: "project", Artifact: "direct-skill"}},
		},
		SourceDir: source,
		Evidence:  setup.BoundaryEvidence{Revision: digest('a')[:40], Subpath: "skills/" + name, ContentSHA256: digest('a'), LicenseSHA256: digest('b')},
	}
}
