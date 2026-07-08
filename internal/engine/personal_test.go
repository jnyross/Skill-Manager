package engine_test

import (
	"os"
	"path/filepath"
	"testing"

	"skillet/internal/engine"
)

func TestPersonalInventoryValidSkills(t *testing.T) {
	f := newFixture(t)
	alpha := writeSkill(t, filepath.Join(f.roots.ClaudeHome, "skills", "alpha"), "alpha", "Alpha description", "")
	bravo := writeSkill(t, filepath.Join(f.roots.ClaudeHome, "skills", "bravo"), "bravo", "Bravo description", "disable-model-invocation: true\n")
	charlie := writeSkill(t, filepath.Join(f.roots.ClaudeHome, "skills", "charlie"), "charlie", "Charlie description", "")

	inv := engine.New(f.roots).Inventory()
	skills := sourceSkills(inv, engine.SourcePersonal)
	if len(skills) != 3 {
		t.Fatalf("got %d personal skills, want 3: %#v", len(skills), skills)
	}

	assertPersonalSkill(t, inv, "alpha", "Alpha description", alpha, engine.ActivationAuto)
	assertPersonalSkill(t, inv, "bravo", "Bravo description", bravo, engine.ActivationManualOnly)
	assertPersonalSkill(t, inv, "charlie", "Charlie description", charlie, engine.ActivationAuto)
}

func TestPersonalMissingRootReturnsNotice(t *testing.T) {
	f := newFixture(t)
	if err := os.RemoveAll(filepath.Join(f.roots.ClaudeHome, "skills")); err != nil {
		t.Fatalf("remove personal skills root: %v", err)
	}

	inv := engine.New(f.roots).Inventory()
	if len(sourceSkills(inv, engine.SourcePersonal)) != 0 {
		t.Fatalf("expected no personal skills, got %#v", sourceSkills(inv, engine.SourcePersonal))
	}
	if len(inv.Notices) != 1 {
		t.Fatalf("got %d notices, want 1: %#v", len(inv.Notices), inv.Notices)
	}
	if !noticesContain(inv, "Personal skills directory not found") {
		t.Fatalf("missing personal root notice: %#v", inv.Notices)
	}
}

func TestPersonalMalformedSkillSkipped(t *testing.T) {
	f := newFixture(t)
	writeSkill(t, filepath.Join(f.roots.ClaudeHome, "skills", "valid"), "valid", "Valid description", "")
	writeFile(t, filepath.Join(f.roots.ClaudeHome, "skills", "bad", "SKILL.md"), "no frontmatter\n")
	writeFile(t, filepath.Join(f.roots.ClaudeHome, "skills", "missing-name", "SKILL.md"), "---\ndescription: \"No name\"\n---\n")

	inv := engine.New(f.roots).Inventory()
	if _, ok := findSkill(inv, engine.SourcePersonal, "valid"); !ok {
		t.Fatalf("valid skill missing from inventory: %#v", inv.Skills)
	}
	if _, ok := findSkill(inv, engine.SourcePersonal, "bad"); ok {
		t.Fatalf("malformed skill appeared in inventory: %#v", inv.Skills)
	}
	if !noticesContain(inv, "Skipped bad") || !noticesContain(inv, "Skipped missing-name") {
		t.Fatalf("missing malformed skill notices: %#v", inv.Notices)
	}
}

func assertPersonalSkill(t *testing.T, inv engine.Inventory, name, description, location string, activation engine.ActivationState) {
	t.Helper()
	skill, ok := findSkill(inv, engine.SourcePersonal, name)
	if !ok {
		t.Fatalf("missing personal skill %q in %#v", name, inv.Skills)
	}
	if skill.Description != description {
		t.Fatalf("%s description = %q, want %q", name, skill.Description, description)
	}
	if skill.Location != location {
		t.Fatalf("%s location = %q, want %q", name, skill.Location, location)
	}
	if skill.Activation != activation {
		t.Fatalf("%s activation = %q, want %q", name, skill.Activation, activation)
	}
	if skill.Kind != engine.KindSkill {
		t.Fatalf("%s kind = %q, want skill", name, skill.Kind)
	}
}
