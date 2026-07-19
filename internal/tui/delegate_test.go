package tui

import (
	"strings"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

func TestRenderSkillItemPreservesActivationOnNarrowTerminal(t *testing.T) {
	skill := engine.Skill{
		Name:       "a-very-long-skill-name-that-would-normally-push-activation-off-screen",
		Source:     engine.SourcePersonal,
		Kind:       engine.KindSkill,
		Activation: engine.ActivationManualOnly,
	}

	rendered := renderSkillItem(skill, false, 40)
	if !strings.Contains(rendered, "Manual") {
		t.Fatalf("rendered row %q missing Activation label", rendered)
	}
}

func TestRenderSkillItemTruncatesLongNameWithEllipsis(t *testing.T) {
	skill := engine.Skill{
		Name:       "this-name-is-longer-than-the-available-space-for-the-row",
		Source:     engine.SourcePersonal,
		Kind:       engine.KindSkill,
		Activation: engine.ActivationAuto,
	}

	rendered := renderSkillItem(skill, false, 30)
	if !strings.Contains(rendered, "…") {
		t.Fatalf("rendered row %q should truncate the name with an ellipsis", rendered)
	}
}
