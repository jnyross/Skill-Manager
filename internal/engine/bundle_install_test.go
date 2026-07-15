package engine_test

import (
	"os"
	"path/filepath"
	"testing"

	"skillet/internal/engine"
)

func TestInstallBundlePlacesEveryMemberWithRememberedActivation(t *testing.T) {
	f := newFixture(t)
	e := engine.New(f.roots)
	sourceA := filepath.Join(f.root, "sources", "auto-skill")
	sourceB := filepath.Join(f.root, "sources", "manual-skill")
	writeSkill(t, sourceA, "auto-skill", "auto", "")
	writeSkill(t, sourceB, "manual-skill", "manual", "")

	a, err := e.AddLibraryEntry(engine.LibraryEntry{Name: "auto-skill", Kind: engine.KindSkill, Tool: engine.ToolClaudeCode, Source: engine.LibrarySource{Kind: engine.LibrarySourceLocalPath, LocalPath: sourceA}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.AddLibraryEntry(engine.LibraryEntry{Name: "manual-skill", Kind: engine.KindSkill, Tool: engine.ToolClaudeCode, Source: engine.LibrarySource{Kind: engine.LibrarySourceLocalPath, LocalPath: sourceB}})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := e.CreateBundle("starter")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.AddBundleMember(bundle.ID, a.ID, engine.ActivationAuto); err != nil {
		t.Fatal(err)
	}
	if err := e.AddBundleMember(bundle.ID, b.ID, engine.ActivationManualOnly); err != nil {
		t.Fatal(err)
	}

	if err := e.InstallBundle(bundle.ID, engine.InstallTarget{Kind: engine.InstallTargetPersonal}); err != nil {
		t.Fatalf("InstallBundle: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.roots.ClaudeHome, "skills", "auto-skill", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	manual, err := os.ReadFile(filepath.Join(f.roots.ClaudeHome, "skills", "manual-skill", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(manual) == "" || !contains(string(manual), "disable-model-invocation: true") {
		t.Fatalf("manual-only frontmatter not applied: %s", manual)
	}
}

func TestInstallBundleStopsAtFirstHardErrorAndNamesMember(t *testing.T) {
	f := newFixture(t)
	e := engine.New(f.roots)
	missing, err := e.AddLibraryEntry(engine.LibraryEntry{Name: "missing", Kind: engine.KindSkill, Tool: engine.ToolClaudeCode, Source: engine.LibrarySource{Kind: engine.LibrarySourceLocalPath, LocalPath: filepath.Join(f.root, "missing")}})
	if err != nil {
		t.Fatal(err)
	}
	goodSource := filepath.Join(f.root, "sources", "good")
	writeSkill(t, goodSource, "good", "good", "")
	good, err := e.AddLibraryEntry(engine.LibraryEntry{Name: "good", Kind: engine.KindSkill, Tool: engine.ToolClaudeCode, Source: engine.LibrarySource{Kind: engine.LibrarySourceLocalPath, LocalPath: goodSource}})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := e.CreateBundle("ordered")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.AddBundleMember(bundle.ID, missing.ID, engine.ActivationAuto); err != nil {
		t.Fatal(err)
	}
	if err := e.AddBundleMember(bundle.ID, good.ID, engine.ActivationAuto); err != nil {
		t.Fatal(err)
	}

	err = e.InstallBundle(bundle.ID, engine.InstallTarget{Kind: engine.InstallTargetPersonal})
	if err == nil || !contains(err.Error(), `member "missing"`) {
		t.Fatalf("InstallBundle error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.roots.ClaudeHome, "skills", "good")); !os.IsNotExist(err) {
		t.Fatalf("later member installed after hard error: %v", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
