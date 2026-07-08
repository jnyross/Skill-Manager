package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"skillet/internal/engine"
)

type viewState int

const (
	mainView viewState = iota
	archiveView
)

type pendingAction int

const (
	pendingUninstall pendingAction = iota
	pendingRestore
	pendingPurge
)

type pendingConfirm struct {
	description string
	action      pendingAction
	location    string
	id          string
}

type Model struct {
	engine  *engine.Engine
	view    viewState
	cursor  int
	inv     engine.Inventory
	archive []engine.ArchiveEntry
	pending *pendingConfirm
	status  string
}

func NewModel(e *engine.Engine) *Model {
	m := &Model{engine: e}
	m.refreshInventory()
	return m
}

func (m *Model) Init() tea.Cmd {
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if m.pending != nil {
		if strings.ToLower(key.String()) == "y" {
			m.executePending()
		} else {
			m.status = "Canceled."
		}
		m.pending = nil
		return m, nil
	}

	switch key.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	default:
		if m.view == mainView {
			m.updateMain(key.String())
		} else {
			m.updateArchive(key.String())
		}
	}

	return m, nil
}

func (m *Model) updateMain(key string) {
	switch key {
	case "u":
		if len(m.inv.Skills) == 0 {
			m.status = "No skill selected."
			return
		}
		selected := m.inv.Skills[m.cursor]
		if selected.Source != engine.SourcePersonal {
			m.status = "Only Personal skills can be archived in this version."
			return
		}
		m.pending = &pendingConfirm{
			description: fmt.Sprintf("Archive Personal skill %q? y to confirm, any other key to cancel.", selected.Name),
			action:      pendingUninstall,
			location:    selected.Location,
		}
	case "a":
		m.view = archiveView
		m.cursor = 0
		m.refreshArchive()
	}
}

func (m *Model) updateArchive(key string) {
	switch key {
	case "a", "esc":
		m.view = mainView
		m.cursor = 0
		m.refreshInventory()
	case "r":
		if len(m.archive) == 0 {
			m.status = "No archive entry selected."
			return
		}
		selected := m.archive[m.cursor]
		m.pending = &pendingConfirm{
			description: fmt.Sprintf("Restore %q to %s? y to confirm, any other key to cancel.", selected.Name, selected.OriginalLocation),
			action:      pendingRestore,
			id:          selected.ID,
		}
	case "p":
		if len(m.archive) == 0 {
			m.status = "No archive entry selected."
			return
		}
		selected := m.archive[m.cursor]
		m.pending = &pendingConfirm{
			description: fmt.Sprintf("Purge %q permanently? y to confirm, any other key to cancel.", selected.Name),
			action:      pendingPurge,
			id:          selected.ID,
		}
	}
}

func (m *Model) executePending() {
	switch m.pending.action {
	case pendingUninstall:
		entry, err := m.engine.Uninstall(m.pending.location)
		if err != nil {
			m.status = "Archive failed: " + err.Error()
			return
		}
		m.status = "Archived " + entry.Name + "."
		m.refreshInventory()
	case pendingRestore:
		if err := m.engine.Restore(m.pending.id); err != nil {
			m.status = "Restore failed: " + err.Error()
			return
		}
		m.status = "Restored archive entry."
		m.refreshArchive()
	case pendingPurge:
		if err := m.engine.Purge(m.pending.id); err != nil {
			m.status = "Purge failed: " + err.Error()
			return
		}
		m.status = "Purged archive entry."
		m.refreshArchive()
	}
}

func (m *Model) moveCursor(delta int) {
	limit := len(m.inv.Skills)
	if m.view == archiveView {
		limit = len(m.archive)
	}
	if limit == 0 {
		m.cursor = 0
		return
	}

	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= limit {
		m.cursor = limit - 1
	}
}

func (m *Model) refreshInventory() {
	m.inv = m.engine.Inventory()
	if m.cursor >= len(m.inv.Skills) {
		m.cursor = max(0, len(m.inv.Skills)-1)
	}
}

func (m *Model) refreshArchive() {
	entries, err := m.engine.ListArchive()
	if err != nil {
		m.archive = nil
		m.status = "Archive read failed: " + err.Error()
		return
	}
	m.archive = entries
	if m.cursor >= len(m.archive) {
		m.cursor = max(0, len(m.archive)-1)
	}
}

func (m *Model) View() string {
	var b strings.Builder
	if m.pending != nil {
		b.WriteString(m.pending.description)
		b.WriteString("\n")
		return b.String()
	}

	if m.view == archiveView {
		m.renderArchive(&b)
	} else {
		m.renderMain(&b)
	}

	if m.status != "" {
		b.WriteString("\n")
		b.WriteString(m.status)
		b.WriteString("\n")
	}
	return b.String()
}

func (m *Model) renderMain(b *strings.Builder) {
	b.WriteString("Skillet\n")
	b.WriteString("up/k down/j move  u archive Personal  a archive view  q quit\n\n")

	if len(m.inv.Skills) == 0 {
		b.WriteString("No skills found.\n")
	} else {
		var current engine.Source
		for i, skill := range m.inv.Skills {
			if skill.Source != current {
				current = skill.Source
				b.WriteString(string(current))
				b.WriteString("\n")
			}
			cursor := " "
			if i == m.cursor {
				cursor = ">"
			}
			label := skill.Name
			if skill.Kind == engine.KindPrompt {
				label = "[prompt] " + label
			}
			pluginText := ""
			if skill.Source == engine.SourcePlugin && skill.Plugin != nil {
				pluginText = fmt.Sprintf(" | one of %d in %s", skill.Plugin.SkillCount, skill.Plugin.Plugin)
			}
			fmt.Fprintf(b, "%s %s | %s | %s%s\n", cursor, label, truncate(skill.Description, 72), skill.Activation, pluginText)
		}
	}

	if len(m.inv.Notices) > 0 {
		b.WriteString("\nNotices\n")
		for _, notice := range m.inv.Notices {
			b.WriteString("- ")
			b.WriteString(notice.Message)
			b.WriteString("\n")
		}
	}
}

func (m *Model) renderArchive(b *strings.Builder) {
	b.WriteString("Skillet Archive\n")
	b.WriteString("up/k down/j move  r restore  p purge  a/esc main view  q quit\n\n")

	if len(m.archive) == 0 {
		b.WriteString("Archive is empty.\n")
		return
	}
	for i, entry := range m.archive {
		cursor := " "
		if i == m.cursor {
			cursor = ">"
		}
		fmt.Fprintf(b, "%s %s | %s | %s | %s\n", cursor, entry.Name, entry.Source, entry.OriginalLocation, entry.ArchivedAt.Format(time.RFC3339))
	}
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}
