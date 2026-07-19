package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

func TestDetailContentPutsSourceToolKindActivationAboveDescription(t *testing.T) {
	skill := engine.Skill{
		Name:        "example",
		Description: "A helpful skill.",
		Source:      engine.SourceProject,
		Tool:        engine.ToolClaudeCode,
		Kind:        engine.KindSkill,
		Location:    "/tmp/example",
		Activation:  engine.ActivationAuto,
	}

	content := detailContent(skill, true, 40)
	sourceIdx := strings.Index(content, "Source")
	descIdx := strings.Index(content, "Description")
	if sourceIdx == -1 || descIdx == -1 || sourceIdx > descIdx {
		t.Fatalf("Source must appear before Description in detail content")
	}
}

func TestDetailContentWrapsLongLocation(t *testing.T) {
	skill := engine.Skill{
		Name:     "example",
		Source:   engine.SourcePersonal,
		Kind:     engine.KindSkill,
		Location: strings.Repeat("a", 100),
	}

	content := detailContent(skill, true, 20)
	lines := strings.Split(content, "\n")
	found := false
	for _, line := range lines {
		if strings.HasPrefix(line, "aaaa") && lipgloss.Width(line) <= 20 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("long Location should wrap within width 20")
	}
}

func TestDetailPaneAddsMoreHintForTallContent(t *testing.T) {
	p := newDetailPane()
	p.setSize(40, 6)
	skill := engine.Skill{
		Name:        "example",
		Description: strings.Repeat("line of description\n", 30),
		Source:      engine.SourcePersonal,
		Kind:        engine.KindSkill,
		Location:    "/tmp/example",
		Activation:  engine.ActivationAuto,
	}
	p.setSkill(skill, true)

	rendered := p.render()
	if !strings.Contains(rendered, "↓ more") {
		t.Fatalf("detail pane should show a '↓ more' hint for tall content")
	}
}

func TestDetailContentNotesCodexManualOnlyClaudeDeclaration(t *testing.T) {
	skill := engine.Skill{
		Name:                        "example",
		Description:                 "A helpful skill.",
		Source:                      engine.SourceCodex,
		Tool:                        engine.ToolCodex,
		Kind:                        engine.KindSkill,
		Location:                    "/tmp/example",
		Activation:                  engine.ActivationAuto,
		DeclaredManualOnlyForClaude: true,
	}

	content := detailContent(skill, true, 40)
	if !strings.Contains(content, "SKILL.md declares manual-only for Claude Code; no effect in Codex") {
		t.Fatalf("detail content missing Codex manual-only Claude note:\n%s", content)
	}
}

func TestDetailContentOmitsCodexNoteWhenNotDeclared(t *testing.T) {
	skill := engine.Skill{
		Name:        "example",
		Description: "A helpful skill.",
		Source:      engine.SourceCodex,
		Tool:        engine.ToolCodex,
		Kind:        engine.KindSkill,
		Location:    "/tmp/example",
		Activation:  engine.ActivationAuto,
	}

	content := detailContent(skill, true, 40)
	if strings.Contains(content, "manual-only for Claude Code") {
		t.Fatalf("detail content should not mention Claude manual-only when not declared:\n%s", content)
	}
}

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
