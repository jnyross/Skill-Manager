package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

// skillDelegate renders one inventory row. It holds the Model's markSet by
// pointer rather than a copy of it: marks change without the item list being
// rebuilt, so a snapshot would render a stale selection.
type skillDelegate struct {
	marks *markSet
}

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
		fmt.Fprint(w, renderSkillItem(item.skill, index == m.Index(), d.marks.has(item.skill), m.Width()))
	}
}

func renderGroupHeader(source engine.Source, width int) string {
	label := " " + string(source) + " "
	if width > lipgloss.Width(label) {
		label += strings.Repeat("-", width-lipgloss.Width(label))
	}
	return sourceHeaderStyle(source).Width(width).Render(label)
}

// markGlyph is the marked-row marker. It sits in its own fixed column, which is
// always present (a space when the row is unmarked) so marking never shifts the
// rest of the row sideways and a column of marks reads as a block.
const markGlyph = "✓"

func renderSkillItem(skill engine.Skill, selected, marked bool, width int) string {
	cursor := " "
	if selected {
		cursor = ">"
	}
	mark := " "
	if marked {
		mark = markedStyle.Render(markGlyph)
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
	styledPluginText := skillMetaStyle.Render(pluginText)

	cursorPrefix := cursor + mark + " "
	sep := " | "
	fixed := lipgloss.Width(cursorPrefix) + lipgloss.Width(sep) +
		lipgloss.Width(activation) + lipgloss.Width(styledPluginText)
	nameMax := width - fixed
	if nameMax < 0 {
		// Drop plugin metadata on very narrow terminals so the Activation label survives.
		styledPluginText = ""
		fixed = lipgloss.Width(cursorPrefix) + lipgloss.Width(sep) + lipgloss.Width(activation)
		nameMax = width - fixed
		if nameMax < 0 {
			nameMax = 0
		}
	}
	if nameMax >= 0 && lipgloss.Width(label) > nameMax {
		label = ansi.Truncate(label, nameMax, "…")
	}

	line := fmt.Sprintf("%s%s%s%s%s",
		cursorPrefix,
		skillNameText(label, selected, marked),
		sep,
		activation,
		styledPluginText,
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

// skillNameText colours the name for the two orthogonal states a row can be
// in. Marked wins over selected: the cursor is already drawn by the ">" and
// moves away, while a mark persists and is what the next bulk action will act
// on, so it has to be the louder of the two.
func skillNameText(name string, selected, marked bool) string {
	switch {
	case marked:
		return markedStyle.Render(name)
	case selected:
		return selectedSkillRowStyle.Render(name)
	default:
		return name
	}
}
