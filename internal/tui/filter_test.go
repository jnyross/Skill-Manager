package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

// visibleNames flattens whatever the active list currently shows into
// comparable names, so each view's filter test can assert on results rather
// than on Bubbles internals.
func visibleNames(t *testing.T, m *Model) []string {
	t.Helper()
	var names []string
	for _, item := range m.activeList().VisibleItems() {
		switch row := item.(type) {
		case skillItem:
			names = append(names, row.skill.Name)
		case libraryItem:
			names = append(names, row.entry.Name)
		case archiveItem:
			names = append(names, row.entry.Name)
		case bundleItem:
			if row.member != nil {
				names = append(names, row.name)
			} else {
				names = append(names, row.bundle.Name)
			}
		case groupHeaderItem:
			names = append(names, "header:"+string(row.source))
		}
	}
	return names
}

func applyFilter(t *testing.T, m *Model, query string) {
	t.Helper()
	pressTUIKey(m, "/")
	if !m.filtering {
		t.Fatalf("`/` did not start filtering (status %q)", m.status)
	}
	typeTUIText(m, query)
}

func TestMainViewFilterMatchesDescriptionAndSource(t *testing.T) {
	m := NewModel(engine.New(engine.Roots{}))
	m.inv = engine.Inventory{Skills: []engine.Skill{
		{Name: "alpha", Description: "writes unit tests", Source: engine.SourcePersonal, Location: "/a"},
		{Name: "bravo", Description: "reviews pull requests", Source: engine.SourcePersonal, Location: "/b"},
		{Name: "charlie", Description: "deploys services", Source: engine.SourceCodex, Location: "/c"},
	}}
	drainTUICmd(m, m.list.SetItems(buildListItems(m.inv)))
	m.selectMainCursor()
	m.refreshDetail()

	// Description match: "reviews" appears in no name.
	applyFilter(t, m, "reviews")
	if got := visibleNames(t, m); len(got) != 1 || got[0] != "bravo" {
		t.Fatalf("filter %q visible = %v, want [bravo]", "reviews", got)
	}
	// Source headers are chrome, not results.
	for _, name := range visibleNames(t, m) {
		if strings.HasPrefix(name, "header:") {
			t.Fatalf("filtered list still contains a Source header: %v", visibleNames(t, m))
		}
	}
	// The main view's derived cursor follows the filtered selection.
	selected, ok := m.selectedMainSkill()
	if !ok || selected.Name != "bravo" {
		t.Fatalf("selected skill = %#v (ok=%t), want bravo", selected, ok)
	}

	// enter applies the filter and leaves input mode.
	pressTUIKey(m, "enter")
	if m.filtering {
		t.Fatal("enter did not leave filter input mode")
	}
	if m.activeList().FilterState() != list.FilterApplied {
		t.Fatalf("filter state = %v, want FilterApplied", m.activeList().FilterState())
	}

	// esc clears the applied filter rather than doing anything view-specific.
	pressTUIKey(m, "esc")
	if m.filterActive() {
		t.Fatal("esc did not clear the applied filter")
	}
	if got := len(m.activeList().VisibleItems()); got != len(m.list.Items()) {
		t.Fatalf("after clearing, visible = %d, want all %d items", got, len(m.list.Items()))
	}

	// Source match.
	applyFilter(t, m, "codex")
	if got := visibleNames(t, m); len(got) != 1 || got[0] != "charlie" {
		t.Fatalf("filter %q visible = %v, want [charlie]", "codex", got)
	}
}

func TestMainViewFilterShowsExplicitZeroMatchState(t *testing.T) {
	m := NewModel(engine.New(engine.Roots{}))
	m.inv = engine.Inventory{Skills: []engine.Skill{
		{Name: "alpha", Source: engine.SourcePersonal, Location: "/a"},
	}}
	drainTUICmd(m, m.list.SetItems(buildListItems(m.inv)))
	m.selectMainCursor()

	applyFilter(t, m, "zzzznomatch")
	if !m.filterFoundNothing() {
		t.Fatalf("expected a zero-match state, visible = %v", visibleNames(t, m))
	}
	view := m.renderView()
	if !strings.Contains(view, "No matches for") {
		t.Fatalf("view does not report the zero-match state:\n%s", view)
	}
	if strings.Contains(view, "No skills found") {
		t.Fatalf("zero-match state confused with an empty inventory:\n%s", view)
	}
}

func TestArchiveViewFilterMatchesOriginalLocation(t *testing.T) {
	m := NewModel(engine.New(engine.Roots{}))
	m.view = archiveView
	m.archive = []engine.ArchiveEntry{
		{ID: "1", Name: "alpha", Source: engine.SourcePersonal, OriginalLocation: "/home/me/.claude/skills/alpha", ArchivedAt: time.Now()},
		{ID: "2", Name: "bravo", Source: engine.SourceCodex, OriginalLocation: "/home/me/.codex/skills/bravo", ArchivedAt: time.Now()},
	}
	drainTUICmd(m, m.archiveList.SetItems(buildArchiveItems(m.archive)))

	applyFilter(t, m, "codex")
	if got := visibleNames(t, m); len(got) != 1 || got[0] != "bravo" {
		t.Fatalf("archive filter visible = %v, want [bravo]", got)
	}
	entry, ok := m.selectedArchiveEntry()
	if !ok || entry.ID != "2" {
		t.Fatalf("selected archive entry = %#v (ok=%t), want id 2", entry, ok)
	}
}

func TestLibraryViewFilterMatchesSourceLocation(t *testing.T) {
	m := NewModel(engine.New(engine.Roots{}))
	m.view = libraryView
	m.library = []engine.LibraryEntry{
		{ID: "1", Name: "alpha", Tool: engine.ToolClaudeCode, Source: engine.LibrarySource{Kind: engine.LibrarySourceLocalPath, LocalPath: "/srv/local/alpha"}},
		{ID: "2", Name: "bravo", Tool: engine.ToolCodex, Source: engine.LibrarySource{Kind: engine.LibrarySourceGit, GitURL: "https://example.test/reviewer.git"}},
	}
	drainTUICmd(m, m.libraryList.SetItems(buildLibraryItems(m.library)))

	applyFilter(t, m, "reviewer")
	if got := visibleNames(t, m); len(got) != 1 || got[0] != "bravo" {
		t.Fatalf("library filter visible = %v, want [bravo]", got)
	}
	entry, ok := m.selectedLibraryEntry()
	if !ok || entry.ID != "2" {
		t.Fatalf("selected library entry = %#v (ok=%t), want id 2", entry, ok)
	}
}

func TestBundleViewFilterMatchesMemberName(t *testing.T) {
	m := NewModel(engine.New(engine.Roots{}))
	m.view = bundleView
	bundles := []engine.Bundle{
		{ID: "b1", Name: "review loop", Members: []engine.BundleMember{{LibraryEntryID: "l1", Activation: engine.ActivationAuto}}},
		{ID: "b2", Name: "deploy kit", Members: []engine.BundleMember{{LibraryEntryID: "l2", Activation: engine.ActivationManualOnly}}},
	}
	library := []engine.LibraryEntry{{ID: "l1", Name: "reviewer"}, {ID: "l2", Name: "shipper"}}
	m.bundles = bundles
	expanded := map[string]bool{"b1": true, "b2": true}
	m.bundleExpanded = expanded
	drainTUICmd(m, m.bundleList.SetItems(buildBundleItems(bundles, library, expanded)))

	applyFilter(t, m, "shipper")
	if got := visibleNames(t, m); len(got) != 1 || got[0] != "shipper" {
		t.Fatalf("bundle filter visible = %v, want [shipper]", got)
	}
	row, ok := m.selectedBundleItem()
	if !ok || row.member == nil || row.member.LibraryEntryID != "l2" {
		t.Fatalf("selected bundle row = %#v (ok=%t), want member l2", row, ok)
	}
}

func TestSwitchingViewsDropsTheFilter(t *testing.T) {
	e, _, _, _ := newPhase3TUIFixture(t)
	m := NewModel(e)
	m.view = libraryView
	m.library = []engine.LibraryEntry{
		{ID: "1", Name: "alpha", Source: engine.LibrarySource{Kind: engine.LibrarySourceLocalPath, LocalPath: "/a"}},
	}
	drainTUICmd(m, m.libraryList.SetItems(buildLibraryItems(m.library)))
	applyFilter(t, m, "alpha")
	pressTUIKey(m, "enter")

	pressTUIKey(m, "L") // back to the main view
	if m.view != mainView {
		t.Fatalf("view = %v, want mainView", m.view)
	}
	if m.libraryList.FilterState() != list.Unfiltered {
		t.Fatalf("library filter survived the view switch: %v", m.libraryList.FilterState())
	}
}
