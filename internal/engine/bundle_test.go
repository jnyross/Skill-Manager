package engine_test

import (
	"strings"
	"testing"

	"skillet/internal/engine"
)

func TestCreateListDeleteBundle(t *testing.T) {
	f := newFixture(t)
	e := engine.New(f.roots)

	bundle, err := e.CreateBundle("Daily tools")
	if err != nil {
		t.Fatalf("CreateBundle: %v", err)
	}
	if bundle.ID == "" || bundle.Name != "Daily tools" || len(bundle.Members) != 0 {
		t.Fatalf("created bundle: %#v", bundle)
	}

	bundles, err := e.ListBundles()
	if err != nil {
		t.Fatalf("ListBundles: %v", err)
	}
	if len(bundles) != 1 || bundles[0].ID != bundle.ID || bundles[0].Name != bundle.Name {
		t.Fatalf("listed bundles: %#v", bundles)
	}

	if err := e.DeleteBundle(bundle.ID); err != nil {
		t.Fatalf("DeleteBundle: %v", err)
	}
	bundles, err = e.ListBundles()
	if err != nil {
		t.Fatalf("ListBundles after delete: %v", err)
	}
	if len(bundles) != 0 {
		t.Fatalf("bundles after delete: %#v", bundles)
	}
}

func TestBundleMembershipAndActivation(t *testing.T) {
	f := newFixture(t)
	e := engine.New(f.roots)
	entry, err := e.AddLibraryEntry(engine.LibraryEntry{
		Name:   "review",
		Kind:   engine.KindSkill,
		Tool:   engine.ToolClaudeCode,
		Source: engine.LibrarySource{Kind: engine.LibrarySourceLocalPath, LocalPath: "/skills/review"},
	})
	if err != nil {
		t.Fatalf("AddLibraryEntry: %v", err)
	}
	bundle, err := e.CreateBundle("Quality")
	if err != nil {
		t.Fatalf("CreateBundle: %v", err)
	}

	if err := e.AddBundleMember(bundle.ID, entry.ID, engine.ActivationAuto); err != nil {
		t.Fatalf("AddBundleMember: %v", err)
	}
	if err := e.SetBundleMemberActivation(bundle.ID, entry.ID, engine.ActivationManualOnly); err != nil {
		t.Fatalf("SetBundleMemberActivation: %v", err)
	}
	bundles, err := e.ListBundles()
	if err != nil {
		t.Fatalf("ListBundles: %v", err)
	}
	if len(bundles) != 1 || len(bundles[0].Members) != 1 {
		t.Fatalf("bundle membership: %#v", bundles)
	}
	member := bundles[0].Members[0]
	if member.LibraryEntryID != entry.ID || member.Activation != engine.ActivationManualOnly {
		t.Fatalf("member after activation update: %#v", member)
	}

	if err := e.RemoveBundleMember(bundle.ID, entry.ID); err != nil {
		t.Fatalf("RemoveBundleMember: %v", err)
	}
	bundles, err = e.ListBundles()
	if err != nil {
		t.Fatalf("ListBundles after remove: %v", err)
	}
	if len(bundles[0].Members) != 0 {
		t.Fatalf("members after remove: %#v", bundles[0].Members)
	}
}

func TestBundleMemberRequiresExistingLibraryEntryAndInstallActivation(t *testing.T) {
	f := newFixture(t)
	e := engine.New(f.roots)
	bundle, err := e.CreateBundle("Safe")
	if err != nil {
		t.Fatalf("CreateBundle: %v", err)
	}

	if err := e.AddBundleMember(bundle.ID, "missing", engine.ActivationAuto); err == nil {
		t.Fatal("expected missing Library entry error")
	}
	entry, err := e.AddLibraryEntry(engine.LibraryEntry{
		Name:   "safe",
		Kind:   engine.KindSkill,
		Tool:   engine.ToolCodex,
		Source: engine.LibrarySource{Kind: engine.LibrarySourceLocalPath, LocalPath: "/skills/safe"},
	})
	if err != nil {
		t.Fatalf("AddLibraryEntry: %v", err)
	}
	if err := e.AddBundleMember(bundle.ID, entry.ID, engine.ActivationSuppressed); err == nil {
		t.Fatal("expected invalid Bundle activation error")
	}
}

func TestRemoveLibraryEntryRefusesBundleReference(t *testing.T) {
	f := newFixture(t)
	e := engine.New(f.roots)
	entry, err := e.AddLibraryEntry(engine.LibraryEntry{
		Name:   "kept",
		Kind:   engine.KindSkill,
		Tool:   engine.ToolClaudeCode,
		Source: engine.LibrarySource{Kind: engine.LibrarySourceLocalPath, LocalPath: "/skills/kept"},
	})
	if err != nil {
		t.Fatalf("AddLibraryEntry: %v", err)
	}
	bundle, err := e.CreateBundle("Referenced here")
	if err != nil {
		t.Fatalf("CreateBundle: %v", err)
	}
	if err := e.AddBundleMember(bundle.ID, entry.ID, engine.ActivationAuto); err != nil {
		t.Fatalf("AddBundleMember: %v", err)
	}

	err = e.RemoveLibraryEntry(entry.ID)
	if err == nil {
		t.Fatal("expected referenced Library entry removal to fail")
	}
	if !strings.Contains(err.Error(), bundle.Name) {
		t.Fatalf("error %q does not name referencing bundle %q", err, bundle.Name)
	}
	entries, listErr := e.ListLibrary()
	if listErr != nil {
		t.Fatalf("ListLibrary: %v", listErr)
	}
	if len(entries) != 1 || entries[0].ID != entry.ID {
		t.Fatalf("Library entry was removed despite reference: %#v", entries)
	}
}
