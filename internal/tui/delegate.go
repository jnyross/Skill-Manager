package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"skillet/internal/engine"
)

type skillDelegate struct{}

func (d skillDelegate) Height() int {
	return 1
}

func (d skillDelegate) Spacing() int {
	return 0
}

func (d skillDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

func (d skillDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	switch item := item.(type) {
	case groupHeaderItem:
		fmt.Fprint(w, renderGroupHeader(item.source, m.Width()))
	case skillItem:
		fmt.Fprint(w, renderSkillItem(item.skill, index == m.Index(), m.Width()))
	}
}

func renderGroupHeader(source engine.Source, width int) string {
	label := " " + string(source) + " "
	if width > lipgloss.Width(label) {
		label += strings.Repeat("-", width-lipgloss.Width(label))
	}
	return sourceHeaderStyle(source).Width(width).Render(label)
}

func renderSkillItem(skill engine.Skill, selected bool, width int) string {
	cursor := " "
	if selected {
		cursor = ">"
	}

	label := skill.Name
	if skill.Kind == engine.KindPrompt {
		label = "[prompt] " + label
	}
	if skill.Source == engine.SourceProject {
		label += skillMetaStyle.Render(" [" + string(skill.Tool) + "]")
	}

	pluginText := ""
	if skill.Source == engine.SourcePlugin && skill.Plugin != nil {
		pluginText = fmt.Sprintf(" | one of %d in %s", skill.Plugin.SkillCount, skill.Plugin.Plugin)
	}

	activation := activationStyle(skill.Activation).Render(string(skill.Activation))
	line := fmt.Sprintf("%s %s | %s%s",
		cursor,
		selectedSkillName(label, selected),
		activation,
		skillMetaStyle.Render(pluginText),
	)

	style := skillRowStyle
	if selected {
		style = selectedSkillRowStyle
	}
	// MaxWidth (not Width) — Width pads/wraps to fill the block, which turns
	// an over-length row into 2+ terminal lines and desyncs the delegate's
	// fixed Height()==1 from what's actually rendered.
	return style.MaxWidth(width).Render(line)
}

func selectedSkillName(name string, selected bool) string {
	if selected {
		return selectedSkillRowStyle.Render(name)
	}
	return name
}
