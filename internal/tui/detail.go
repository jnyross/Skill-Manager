package tui

import (
	"fmt"
	"strings"

	"skillet/internal/engine"
)

type detailPane struct {
	width  int
	height int
}

func (p detailPane) render(skill engine.Skill, ok bool) string {
	width := p.width
	if width < 1 {
		width = 40
	}
	height := p.height
	if height < 1 {
		height = 1
	}

	contentWidth := width - detailPaneStyle.GetHorizontalFrameSize()
	if contentWidth < 1 {
		contentWidth = 1
	}
	contentHeight := height - detailPaneStyle.GetVerticalFrameSize()
	if contentHeight < 1 {
		contentHeight = 1
	}

	if !ok {
		return detailPaneStyle.
			Width(contentWidth).
			Height(contentHeight).
			Render(skillMetaStyle.Render("No skill selected."))
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

	return detailPaneStyle.
		Width(contentWidth).
		Height(contentHeight).
		Render(strings.TrimRight(b.String(), "\n"))
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
