package tui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

type libraryItem struct {
	entry engine.LibraryEntry
}

// FilterValue covers name, Tool, source kind, and resolved location so "/"
// finds a Library entry by where it comes from as well as by name.
func (i libraryItem) FilterValue() string {
	return strings.Join([]string{
		i.entry.Name,
		string(i.entry.Tool),
		string(i.entry.Source.Kind),
		librarySourceLocation(i.entry.Source),
	}, " ")
}

func buildLibraryItems(entries []engine.LibraryEntry) []list.Item {
	if len(entries) == 0 {
		return nil
	}
	items := make([]list.Item, 0, len(entries))
	for _, entry := range entries {
		items = append(items, libraryItem{entry: entry})
	}
	return items
}

func newLibraryList(items []list.Item) list.Model {
	model := list.New(items, libraryDelegate{}, 0, 0)
	model.SetShowTitle(false)
	// Filtering is enabled but the list's own filter bar stays hidden: help
	// renders as line 2 (a header, not a footer), so the Model draws the
	// filter prompt itself in renderFilterLine to keep the layout stable.
	model.SetShowFilter(false)
	model.SetFilteringEnabled(true)
	model.SetShowStatusBar(false)
	model.SetShowPagination(false)
	model.SetShowHelp(false)
	model.DisableQuitKeybindings()
	return model
}

type libraryDelegate struct{}

func (d libraryDelegate) Height() int  { return 1 }
func (d libraryDelegate) Spacing() int { return 0 }

func (d libraryDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

func (d libraryDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	entryItem, ok := item.(libraryItem)
	if !ok {
		return
	}
	fmt.Fprint(w, renderLibraryItem(entryItem.entry, index == m.Index(), m.Width()))
}

func renderLibraryItem(entry engine.LibraryEntry, selected bool, width int) string {
	cursor := " "
	if selected {
		cursor = ">"
	}

	label := entry.Name
	if entry.Tool != "" {
		label += skillMetaStyle.Render(" [" + string(entry.Tool) + "]")
	}

	meta := skillMetaStyle.Render(fmt.Sprintf(" | %s | %s | %s",
		entry.Source.Kind,
		librarySourceLocation(entry.Source),
		entry.AddedAt.Format(time.RFC3339),
	))
	line := fmt.Sprintf("%s %s%s", cursor, skillNameText(label, selected, false), meta)

	style := skillRowStyle
	if selected {
		style = selectedSkillRowStyle
	}
	return style.MaxWidth(width).Render(line)
}

func librarySourceLocation(src engine.LibrarySource) string {
	switch src.Kind {
	case engine.LibrarySourceLocalPath:
		return src.LocalPath
	case engine.LibrarySourceGit:
		if src.GitSubPath != "" {
			return src.GitURL + ":" + src.GitSubPath
		}
		return src.GitURL
	case engine.LibrarySourceSkillsSh:
		if src.SkillsShSkill != "" {
			return src.SkillsShRepo + "/" + src.SkillsShSkill
		}
		return src.SkillsShRepo
	case engine.LibrarySourceMarketplace:
		return src.PluginName + "@" + src.Marketplace
	default:
		return string(src.Kind)
	}
}

// canToggleLibraryMembership is true for user-level directory skills that
// derive a local-path Library source (Personal or Codex skill, not prompts).
func canToggleLibraryMembership(skill engine.Skill) bool {
	if skill.Source == engine.SourcePlugin && skill.Plugin != nil {
		return true
	}
	return skill.Kind == engine.KindSkill &&
		(skill.Source == engine.SourcePersonal || skill.Source == engine.SourceCodex)
}

func libraryToggleUnavailableReason(skill engine.Skill) string {
	if canToggleLibraryMembership(skill) {
		return ""
	}
	if skill.Source == engine.SourceProject {
		return "Project skills are not added to the Library (Library is a personal catalog)."
	}
	if skill.Kind == engine.KindPrompt {
		return "Only skills (not prompts) can be added to the Library from the main list."
	}
	return "Only Personal and Codex skills can be added to the Library from the main list."
}
