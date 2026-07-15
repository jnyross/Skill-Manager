package tui

import (
	"testing"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

// buildListItems trusts its input is already ordered by Source then Name
// (matching engine.Inventory()'s own sort) — these fixtures are pre-sorted.

func TestBuildListItemsInsertsGroupHeadersBySourceOrder(t *testing.T) {
	items := buildListItems(engine.Inventory{
		Skills: []engine.Skill{
			{Name: "personal", Source: engine.SourcePersonal},
			{Name: "plugin", Source: engine.SourcePlugin},
			{Name: "codex", Source: engine.SourceCodex},
			{Name: "project", Source: engine.SourceProject},
		},
	})

	if len(items) != 8 {
		t.Fatalf("len(items) = %d, want 8", len(items))
	}

	assertHeader(t, items[0], engine.SourcePersonal)
	assertSkill(t, items[1], "personal")
	assertHeader(t, items[2], engine.SourcePlugin)
	assertSkill(t, items[3], "plugin")
	assertHeader(t, items[4], engine.SourceCodex)
	assertSkill(t, items[5], "codex")
	assertHeader(t, items[6], engine.SourceProject)
	assertSkill(t, items[7], "project")
}

func TestBuildListItemsInsertsHeaderBeforeFirstItemOfEachSource(t *testing.T) {
	items := buildListItems(engine.Inventory{
		Skills: []engine.Skill{
			{Name: "alpha", Source: engine.SourcePersonal},
			{Name: "bravo", Source: engine.SourcePersonal},
			{Name: "charlie", Source: engine.SourceCodex},
			{Name: "delta", Source: engine.SourceCodex},
		},
	})

	if len(items) != 6 {
		t.Fatalf("len(items) = %d, want 6", len(items))
	}

	assertHeader(t, items[0], engine.SourcePersonal)
	assertSkill(t, items[1], "alpha")
	assertSkill(t, items[2], "bravo")
	assertHeader(t, items[3], engine.SourceCodex)
	assertSkill(t, items[4], "charlie")
	assertSkill(t, items[5], "delta")
}

func TestBuildListItemsEmptyInventoryHasNoHeaders(t *testing.T) {
	items := buildListItems(engine.Inventory{})
	if len(items) != 0 {
		t.Fatalf("len(items) = %d, want 0", len(items))
	}
}

func assertHeader(t *testing.T, item any, source engine.Source) {
	t.Helper()

	header, ok := item.(groupHeaderItem)
	if !ok {
		t.Fatalf("item = %#v, want groupHeaderItem", item)
	}
	if header.source != source {
		t.Fatalf("header.source = %q, want %q", header.source, source)
	}
}

func assertSkill(t *testing.T, item any, name string) {
	t.Helper()

	skill, ok := item.(skillItem)
	if !ok {
		t.Fatalf("item = %#v, want skillItem", item)
	}
	if skill.skill.Name != name {
		t.Fatalf("skill.name = %q, want %q", skill.skill.Name, name)
	}
}
