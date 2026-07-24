package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

// These tests cover the four actions that used to write to disk with no
// confirmation, against the README's "every action that changes something on
// disk asks for confirmation first" promise.

func TestLibraryToggleConfirmsBeforeWriting(t *testing.T) {
	e, _, root, _ := newPhase3TUIFixture(t)
	writeTUISkill(t, filepath.Join(root, "claude", "skills", "alpha"), "alpha", "does alpha things")

	m := NewModel(e)
	pressTUIKey(m, "l")

	if m.pending == nil {
		t.Fatalf("`l` did not ask for confirmation (status %q)", m.status)
	}
	if !strings.Contains(m.pending.description, "Library") || !strings.Contains(m.pending.description, "alpha") {
		t.Fatalf("confirmation description = %q", m.pending.description)
	}
	entries, err := e.ListLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("`l` wrote to the Library before confirmation: %#v", entries)
	}

	pressTUIKey(m, "y")
	entries, err = e.ListLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "alpha" {
		t.Fatalf("Library after confirmation = %#v", entries)
	}

	// And the reverse direction confirms too.
	pressTUIKey(m, "l")
	if m.pending == nil || !strings.Contains(m.pending.description, "Remove") {
		t.Fatalf("un-toggling did not ask to remove: %#v", m.pending)
	}
	pressTUIKey(m, "n") // any key other than y cancels
	entries, err = e.ListLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("cancelling still changed the Library: %#v", entries)
	}
}

func newBundleFixture(t *testing.T) (*engine.Engine, engine.Bundle, engine.LibraryEntry) {
	t.Helper()
	e, _, root, _ := newPhase3TUIFixture(t)
	source := writeTUISkill(t, filepath.Join(root, "sources", "reviewer"), "reviewer", "reviews code")
	entry, err := e.AddLibraryEntry(engine.LibraryEntry{
		Name:   "reviewer",
		Kind:   engine.KindSkill,
		Tool:   engine.ToolClaudeCode,
		Source: engine.LibrarySource{Kind: engine.LibrarySourceLocalPath, LocalPath: source},
	})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := e.CreateBundle("review loop")
	if err != nil {
		t.Fatal(err)
	}
	return e, bundle, entry
}

func TestBundleAddMemberConfirmsBeforeWriting(t *testing.T) {
	e, bundle, entry := newBundleFixture(t)

	m := NewModel(e)
	pressTUIKey(m, "B")
	pressTUIKey(m, "a")
	if m.memberPicker == nil {
		t.Fatalf("`a` did not open the member picker (status %q)", m.status)
	}
	pressTUIKey(m, "enter")

	if m.pending == nil {
		t.Fatalf("choosing a member did not ask for confirmation (status %q)", m.status)
	}
	if !strings.Contains(m.pending.description, entry.Name) || !strings.Contains(m.pending.description, bundle.Name) {
		t.Fatalf("confirmation description = %q", m.pending.description)
	}
	if members := bundleMembers(t, e, bundle.ID); len(members) != 0 {
		t.Fatalf("member was added before confirmation: %#v", members)
	}

	pressTUIKey(m, "y")
	if members := bundleMembers(t, e, bundle.ID); len(members) != 1 || members[0].LibraryEntryID != entry.ID {
		t.Fatalf("members after confirmation = %#v", members)
	}
}

func TestBundleRemoveMemberAndActivationConfirmBeforeWriting(t *testing.T) {
	e, bundle, entry := newBundleFixture(t)
	if err := e.AddBundleMember(bundle.ID, entry.ID, engine.ActivationAuto); err != nil {
		t.Fatal(err)
	}

	m := NewModel(e)
	pressTUIKey(m, "B")
	pressTUIKey(m, "enter") // expand the Bundle so the member row is selectable
	pressTUIKey(m, "down")  // move onto the member row

	// Activation toggle.
	pressTUIKey(m, "m")
	if m.pending == nil {
		t.Fatalf("`m` did not ask for confirmation (status %q)", m.status)
	}
	if !strings.Contains(m.pending.description, string(engine.ActivationManualOnly)) {
		t.Fatalf("confirmation description = %q", m.pending.description)
	}
	if members := bundleMembers(t, e, bundle.ID); members[0].Activation != engine.ActivationAuto {
		t.Fatalf("Activation changed before confirmation: %#v", members)
	}
	pressTUIKey(m, "y")
	if members := bundleMembers(t, e, bundle.ID); members[0].Activation != engine.ActivationManualOnly {
		t.Fatalf("Activation after confirmation = %#v", members)
	}

	// Remove member.
	pressTUIKey(m, "r")
	if m.pending == nil {
		t.Fatalf("`r` did not ask for confirmation (status %q)", m.status)
	}
	if members := bundleMembers(t, e, bundle.ID); len(members) != 1 {
		t.Fatalf("member removed before confirmation: %#v", members)
	}
	pressTUIKey(m, "y")
	if members := bundleMembers(t, e, bundle.ID); len(members) != 0 {
		t.Fatalf("members after confirmed removal = %#v", members)
	}
}

func bundleMembers(t *testing.T, e *engine.Engine, id string) []engine.BundleMember {
	t.Helper()
	bundles, err := e.ListBundles()
	if err != nil {
		t.Fatal(err)
	}
	for _, bundle := range bundles {
		if bundle.ID == id {
			return bundle.Members
		}
	}
	t.Fatalf("bundle %s not found", id)
	return nil
}

func TestCtrlCQuitsFromOverlaysButCancelsAConfirmation(t *testing.T) {
	e, _, root, _ := newPhase3TUIFixture(t)
	writeTUISkill(t, filepath.Join(root, "claude", "skills", "alpha"), "alpha", "does alpha things")

	// In the text form ctrl+c used to do nothing at all.
	m := NewModel(e)
	pressTUIKey(m, "L")
	pressTUIKey(m, "n")
	pressTUIKey(m, "enter") // choose the local-path source, opening the form
	if m.form == nil {
		t.Fatal("expected the Library entry form")
	}
	if _, cmd := m.Update(tuiKeyMsg("ctrl+c")); cmd == nil {
		t.Fatal("ctrl+c in the text form did not quit")
	}

	// In a confirmation ctrl+c cancels instead of quitting.
	m2 := NewModel(e)
	pressTUIKey(m2, "l")
	if m2.pending == nil {
		t.Fatal("expected a pending confirmation")
	}
	updated, cmd := m2.Update(tuiKeyMsg("ctrl+c"))
	if cmd != nil {
		t.Fatal("ctrl+c quit out of a confirmation instead of cancelling it")
	}
	if updated.(*Model).pending != nil {
		t.Fatal("ctrl+c left the confirmation open")
	}
	entries, err := e.ListLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("cancelled confirmation still wrote: %#v", entries)
	}
}

func TestFormEscConfirmsDiscardAndShiftTabGoesBack(t *testing.T) {
	f := newTextForm(formLibraryEntry, []string{"Name", "Tool", "Local path"})

	// esc with nothing typed cancels straight away.
	if _, canceled := f.update(tuiKeyMsg("esc")); !canceled {
		t.Fatal("esc on an empty form did not cancel")
	}

	f = newTextForm(formLibraryEntry, []string{"Name", "Tool", "Local path"})
	f.input.SetValue("alpha")
	f.update(tuiKeyMsg("enter"))
	if f.index != 1 {
		t.Fatalf("field index = %d, want 1", f.index)
	}

	// esc with typed values asks first.
	if _, canceled := f.update(tuiKeyMsg("esc")); canceled {
		t.Fatal("esc discarded typed values without asking")
	}
	if !f.discarding {
		t.Fatal("esc did not enter the discard confirmation")
	}
	if !strings.Contains(f.render(), "Discard") {
		t.Fatalf("discard prompt not rendered: %q", f.render())
	}
	if _, canceled := f.update(tuiKeyMsg("n")); canceled {
		t.Fatal("any key other than y should keep editing")
	}
	if f.discarding {
		t.Fatal("declining the discard left the prompt up")
	}

	// shift+tab steps back and restores the earlier value.
	if _, canceled := f.update(tuiKeyMsg("shift+tab")); canceled {
		t.Fatal("shift+tab cancelled the form")
	}
	if f.index != 0 {
		t.Fatalf("after shift+tab index = %d, want 0", f.index)
	}
	if f.input.Value() != "alpha" {
		t.Fatalf("back-navigation lost the value: %q", f.input.Value())
	}

	// esc then y discards.
	f.update(tuiKeyMsg("esc"))
	if _, canceled := f.update(tuiKeyMsg("y")); !canceled {
		t.Fatal("y at the discard prompt did not cancel the form")
	}
}
