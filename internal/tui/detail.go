package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

type detailPane struct {
	vp      viewport.Model
	content string
}

func newDetailPane() detailPane {
	return detailPane{vp: viewport.New(0, 0)}
}

func (p *detailPane) setSize(width, height int) {
	if width < 1 {
		width = 40
	}
	if height < 1 {
		height = 1
	}

	// Viewport fills the inside of the bordered pane.
	contentWidth := width - detailPaneStyle.GetHorizontalFrameSize()
	if contentWidth < 1 {
		contentWidth = 1
	}
	contentHeight := height - detailPaneStyle.GetVerticalFrameSize()
	if contentHeight < 1 {
		contentHeight = 1
	}
	p.vp.Width = contentWidth
	p.vp.Height = contentHeight
	// Re-apply content after size change so the viewport reclamps YOffset
	// without jumping to top when the selection is unchanged.
	if p.content != "" {
		y := p.vp.YOffset
		p.applyContent()
		p.vp.SetYOffset(y)
	}
}

// setSkill replaces detail content when the selected skill changes. Unchanged
// content is a no-op so scroll position survives unrelated Update cycles.
func (p *detailPane) setSkill(skill engine.Skill, ok bool) {
	content := detailContent(skill, ok, p.vp.Width)
	if content == p.content {
		return
	}
	p.content = content
	p.applyContent()
	p.vp.GotoTop()
}

// applyContent sets the viewport content.
func (p *detailPane) applyContent() {
	p.vp.SetContent(p.content)
}

func (p *detailPane) scrollHalf(delta int) {
	if delta > 0 {
		p.vp.HalfViewDown()
		return
	}
	if delta < 0 {
		p.vp.HalfViewUp()
	}
}

func (p detailPane) render() string {
	width := p.vp.Width
	if width < 1 {
		width = 1
	}
	height := p.vp.Height
	if height < 1 {
		height = 1
	}

	view := p.vp.View()
	if p.vp.TotalLineCount() > p.vp.Height {
		// Overlay a "more" hint on the last visible line so the user knows
		// the pane is scrollable.
		lines := strings.Split(view, "\n")
		if len(lines) > 0 {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, skillMetaStyle.Render("↓ more"))
		view = strings.Join(lines, "\n")
	}

	return detailPaneStyle.
		Width(width).
		Height(height).
		Render(view)
}

func detailContent(skill engine.Skill, ok bool, width int) string {
	if !ok {
		return skillMetaStyle.Render("No skill selected.")
	}

	if width < 1 {
		width = 1
	}

	var b strings.Builder
	b.WriteString(detailTitleStyle.Render(skill.Name))
	b.WriteString("\n\n")
	writeDetailField(&b, "Source", string(skill.Source))
	if skill.Source == engine.SourceProject {
		writeDetailField(&b, "Tool", string(skill.Tool))
	}
	writeDetailField(&b, "Kind", string(skill.Kind))
	writeDetailField(&b, "Activation", activationStyle(skill.Activation).Render(string(skill.Activation)))
	if skill.Tool == engine.ToolCodex && skill.DeclaredManualOnlyForClaude && skill.Activation == engine.ActivationAuto {
		writeDetailField(&b, "Note", "SKILL.md declares manual-only for Claude Code; no effect in Codex")
	}
	writeDetailField(&b, "Description", skill.Description)
	writeDetailWrappedField(&b, "Location", skill.Location, width)
	if skill.Source == engine.SourcePlugin && skill.Plugin != nil {
		writeDetailField(&b, "Plugin", fmt.Sprintf("one of %d in %s", skill.Plugin.SkillCount, skill.Plugin.Plugin))
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeDetailField(b *strings.Builder, label, value string) {
	if value == "" {
		value = "-"
	}
	b.WriteString(detailLabelStyle.Render(label))
	b.WriteString("\n")
	b.WriteString(value)
	b.WriteString("\n\n")
}

func writeDetailWrappedField(b *strings.Builder, label, value string, width int) {
	if value == "" {
		value = "-"
	}
	b.WriteString(detailLabelStyle.Render(label))
	b.WriteString("\n")
	wrapped := lipgloss.NewStyle().Width(width).Render(value)
	b.WriteString(wrapped)
	b.WriteString("\n\n")
}
