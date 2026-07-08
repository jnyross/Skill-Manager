package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"

	"skillet/internal/engine"
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
		p.vp.SetContent(p.content)
		p.vp.SetYOffset(y)
	}
}

// setSkill replaces detail content when the selected skill changes. Unchanged
// content is a no-op so scroll position survives unrelated Update cycles.
func (p *detailPane) setSkill(skill engine.Skill, ok bool) {
	content := detailContent(skill, ok)
	if content == p.content {
		return
	}
	p.content = content
	p.vp.SetContent(content)
	p.vp.GotoTop()
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
	return detailPaneStyle.
		Width(width).
		Height(height).
		Render(p.vp.View())
}

func detailContent(skill engine.Skill, ok bool) string {
	if !ok {
		return skillMetaStyle.Render("No skill selected.")
	}

	var b strings.Builder
	b.WriteString(detailTitleStyle.Render(skill.Name))
	b.WriteString("\n\n")
	writeDetailField(&b, "Description", skill.Description)
	writeDetailField(&b, "Location", skill.Location)
	writeDetailField(&b, "Source", string(skill.Source))
	if skill.Source == engine.SourceProject {
		writeDetailField(&b, "Tool", string(skill.Tool))
	}
	writeDetailField(&b, "Kind", string(skill.Kind))
	writeDetailField(&b, "Activation", activationStyle(skill.Activation).Render(string(skill.Activation)))
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
