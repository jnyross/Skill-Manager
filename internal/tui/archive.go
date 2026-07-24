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

type archiveItem struct {
	entry engine.ArchiveEntry
}

// FilterValue covers name, original Source, Tool, and original location so
// "/" finds an archived entry by where it used to live.
func (i archiveItem) FilterValue() string {
	return strings.Join([]string{
		i.entry.Name,
		string(i.entry.Source),
		string(i.entry.Tool),
		i.entry.OriginalLocation,
	}, " ")
}

// buildArchiveItems preserves engine.ListArchive order (ArchivedAt descending).
func buildArchiveItems(entries []engine.ArchiveEntry) []list.Item {
	if len(entries) == 0 {
		return nil
	}
	items := make([]list.Item, 0, len(entries))
	for _, entry := range entries {
		items = append(items, archiveItem{entry: entry})
	}
	return items
}

func newArchiveList(items []list.Item) list.Model {
	model := list.New(items, archiveDelegate{}, 0, 0)
	model.SetShowTitle(false)
	// See newLibraryList: filtering on, the list's own filter bar off.
	model.SetShowFilter(false)
	model.SetFilteringEnabled(true)
	model.SetShowStatusBar(false)
	model.SetShowPagination(false)
	model.SetShowHelp(false)
	model.DisableQuitKeybindings()
	return model
}

type archiveDelegate struct{}

func (d archiveDelegate) Height() int  { return 1 }
func (d archiveDelegate) Spacing() int { return 0 }

func (d archiveDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

func (d archiveDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	entryItem, ok := item.(archiveItem)
	if !ok {
		return
	}
	fmt.Fprint(w, renderArchiveItem(entryItem.entry, index == m.Index(), m.Width()))
}

func renderArchiveItem(entry engine.ArchiveEntry, selected bool, width int) string {
	cursor := " "
	if selected {
		cursor = ">"
	}

	label := entry.Name
	if entry.Kind == engine.KindPrompt {
		label = "[prompt] " + label
	}
	if entry.Source == engine.SourceProject && entry.Tool != "" {
		label += skillMetaStyle.Render(" [" + string(entry.Tool) + "]")
	}

	meta := skillMetaStyle.Render(fmt.Sprintf(" | %s | %s | %s",
		entry.Source,
		entry.OriginalLocation,
		entry.ArchivedAt.Format(time.RFC3339),
	))
	line := fmt.Sprintf("%s %s%s", cursor, selectedSkillName(label, selected), meta)

	style := skillRowStyle
	if selected {
		style = selectedSkillRowStyle
	}
	return style.MaxWidth(width).Render(line)
}
