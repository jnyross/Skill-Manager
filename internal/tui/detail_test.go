package tui

import (
	"strings"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

func TestSetSkillPreservesScrollWhenSelectionUnchanged(t *testing.T) {
	p := newDetailPane()
	p.setSize(40, 4)
	skill := engine.Skill{
		Name:        "long",
		Description: strings.Repeat("line of description\n", 30),
		Source:      engine.SourcePersonal,
		Tool:        engine.ToolClaudeCode,
		Kind:        engine.KindSkill,
		Location:    "/tmp/long",
		Activation:  engine.ActivationAuto,
	}
	p.setSkill(skill, true)
	p.scrollHalf(1)
	y := p.vp.YOffset
	if y == 0 {
		t.Fatal("expected scroll offset > 0 after HalfViewDown on tall content")
	}

	p.setSkill(skill, true)
	if p.vp.YOffset != y {
		t.Fatalf("YOffset after identical setSkill = %d, want %d (scroll must survive)", p.vp.YOffset, y)
	}

	other := skill
	other.Name = "other"
	p.setSkill(other, true)
	if p.vp.YOffset != 0 {
		t.Fatalf("YOffset after selection change = %d, want 0", p.vp.YOffset)
	}
}
