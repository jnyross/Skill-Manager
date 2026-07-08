package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
	pendingSuppress
	pendingUnsuppress
	pendingManualOnly
	pendingAutoActivate
	pendingUninstallPlugin
)

type pendingConfirm struct {
	description string
	action      pendingAction
	location    string
	id          string
	skill       engine.Skill
	plugin      engine.PluginInfo
}

type Model struct {
	engine  *engine.Engine
	view    viewState
	cursor  int
	inv     engine.Inventory
	list    list.Model
	archive []engine.ArchiveEntry
	pending *pendingConfirm
	status  string
	width   int
	height  int
	detail  detailPane
}

func NewModel(e *engine.Engine) *Model {
	m := &Model{
		engine: e,
		list:   newSkillList(nil),
	}
	m.refreshInventory()
	return m
}

func (m *Model) Init() tea.Cmd {
	return tea.EnterAltScreen
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = size.Width
		m.height = size.Height
		m.resizeList()
		return m, nil
	}

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
		m.resizeList()
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

	m.resizeList()
	return m, nil
}

func (m *Model) updateMain(key string) {
	switch key {
	case "u":
		selected, ok := m.selectedMainSkill()
		if !ok {
			m.status = "No skill selected."
			return
		}
		if selected.Source != engine.SourcePersonal && selected.Source != engine.SourceCodex {
			m.status = "Only Personal and Codex skills can be archived in this version."
			return
		}
		m.pending = &pendingConfirm{
			description: fmt.Sprintf("Archive %s %q? y to confirm, any other key to cancel.", selected.Source, selected.Name),
			action:      pendingUninstall,
			location:    selected.Location,
		}
	case "s":
		selected, ok := m.selectedMainSkill()
		if !ok {
			m.status = "No skill selected."
			return
		}
		isCodexSkill := selected.Source == engine.SourceCodex && selected.Kind == engine.KindSkill
		if selected.Source != engine.SourcePlugin && !isCodexSkill {
			m.status = "Suppress is only available for Plugin and Codex skills."
			return
		}
		if selected.Activation == engine.ActivationSuppressed || selected.Activation == engine.ActivationDisabled {
			m.pending = &pendingConfirm{
				description: fmt.Sprintf("Un-suppress %q? y to confirm, any other key to cancel.", selected.Name),
				action:      pendingUnsuppress,
				skill:       selected,
			}
		} else {
			m.pending = &pendingConfirm{
				description: fmt.Sprintf("Suppress %q? Hides it from the model and slash menu; plugin stays installed. y to confirm, any other key to cancel.", selected.Name),
				action:      pendingSuppress,
				skill:       selected,
			}
		}
	case "x":
		selected, ok := m.selectedMainSkill()
		if !ok {
			m.status = "No skill selected."
			return
		}
		if selected.Source != engine.SourcePlugin || selected.Plugin == nil {
			m.status = "Uninstall plugin is only available for Plugin skills."
			return
		}
		names := pluginSkillNames(m.inv.Skills, *selected.Plugin)
		m.pending = &pendingConfirm{
			description: fmt.Sprintf("Uninstall plugin %q (%s@%s)? This removes all %d skills: %s. y to confirm, any other key to cancel.",
				selected.Plugin.Plugin, selected.Plugin.Plugin, selected.Plugin.Marketplace, len(names), strings.Join(names, ", ")),
			action: pendingUninstallPlugin,
			plugin: *selected.Plugin,
		}
	case "m":
		selected, ok := m.selectedMainSkill()
		if !ok {
			m.status = "No skill selected."
			return
		}
		if selected.Kind != engine.KindSkill || (selected.Source != engine.SourcePersonal && selected.Source != engine.SourceCodex) {
			m.status = "Manual-only is only available for Personal and Codex skills."
			return
		}
		if selected.Activation == engine.ActivationManualOnly {
			m.pending = &pendingConfirm{
				description: fmt.Sprintf("Turn Auto-activation back on for %q? y to confirm, any other key to cancel.", selected.Name),
				action:      pendingAutoActivate,
				skill:       selected,
			}
		} else {
			m.pending = &pendingConfirm{
				description: fmt.Sprintf("Make %q Manual-only? It will only run when explicitly invoked. y to confirm, any other key to cancel.", selected.Name),
				action:      pendingManualOnly,
				skill:       selected,
			}
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
	case pendingSuppress:
		if err := m.engine.Suppress(m.pending.skill); err != nil {
			m.status = "Suppress failed: " + err.Error()
			return
		}
		m.status = "Suppressed " + m.pending.skill.Name + "."
		if m.pending.skill.Source == engine.SourceCodex {
			m.status += " Restart Codex to pick up the change."
		}
		m.refreshInventory()
	case pendingUnsuppress:
		if err := m.engine.Unsuppress(m.pending.skill); err != nil {
			m.status = "Un-suppress failed: " + err.Error()
			return
		}
		m.status = "Un-suppressed " + m.pending.skill.Name + "."
		if m.pending.skill.Source == engine.SourceCodex {
			m.status += " Restart Codex to pick up the change."
		}
		m.refreshInventory()
	case pendingManualOnly:
		if err := m.engine.SetManualOnly(m.pending.skill, true); err != nil {
			m.status = "Manual-only failed: " + err.Error()
			return
		}
		m.status = "Made " + m.pending.skill.Name + " Manual-only."
		m.refreshInventory()
	case pendingAutoActivate:
		if err := m.engine.SetManualOnly(m.pending.skill, false); err != nil {
			m.status = "Auto-activation failed: " + err.Error()
			return
		}
		m.status = "Restored Auto-activation for " + m.pending.skill.Name + "."
		m.refreshInventory()
	case pendingUninstallPlugin:
		plugin := m.pending.plugin
		if err := m.engine.UninstallPlugin(plugin); err != nil {
			m.status = "Uninstall plugin failed: " + err.Error()
			return
		}
		m.status = "Uninstalled plugin " + plugin.Plugin + "."
		m.refreshInventory()
	}
}

// pluginSkillNames returns the names of every skill in skills belonging to
// plugin (matched by Marketplace+Plugin), for the Uninstall-plugin
// confirmation to list every skill about to be removed (issue #10's
// acceptance criterion: "the confirmation lists all N skills in the plugin
// before proceeding"). Built client-side from the Inventory() result
// already held by the model, rather than a new engine listing method.
func pluginSkillNames(skills []engine.Skill, plugin engine.PluginInfo) []string {
	var names []string
	for _, skill := range skills {
		if skill.Source == engine.SourcePlugin && skill.Plugin != nil &&
			skill.Plugin.Marketplace == plugin.Marketplace && skill.Plugin.Plugin == plugin.Plugin {
			names = append(names, skill.Name)
		}
	}
	return names
}

func (m *Model) moveCursor(delta int) {
	if m.view == mainView {
		m.moveMainCursor(delta)
		return
	}

	limit := len(m.archive)
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

func (m *Model) moveMainCursor(delta int) {
	items := m.list.Items()
	if len(items) == 0 {
		m.cursor = 0
		return
	}

	index := m.list.Index()
	for {
		index += delta
		if index < 0 || index >= len(items) {
			break
		}
		if _, ok := items[index].(skillItem); ok {
			m.list.Select(index)
			break
		}
	}
	m.syncMainCursor()
}

func (m *Model) refreshInventory() {
	m.inv = m.engine.Inventory()
	if m.cursor >= len(m.inv.Skills) {
		m.cursor = max(0, len(m.inv.Skills)-1)
	}
	_ = m.list.SetItems(buildListItems(m.inv))
	m.selectMainCursor()
	m.resizeList()
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
	b.WriteString("up/k down/j move  u archive Personal/Codex  s suppress/un-suppress Plugin/Codex skill  m manual-only/auto-activate Personal/Codex skill  x uninstall whole Plugin  a archive view  q quit\n\n")

	if len(m.inv.Skills) == 0 {
		b.WriteString("No skills found.\n")
	} else {
		detail := m.detail.render(m.selectedMainSkill())
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, m.list.View(), " ", detail))
		b.WriteString("\n")
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

func newSkillList(items []list.Item) list.Model {
	model := list.New(items, skillDelegate{}, 0, 0)
	model.SetShowTitle(false)
	model.SetShowFilter(false)
	model.SetFilteringEnabled(false)
	model.SetShowStatusBar(false)
	model.SetShowPagination(false)
	model.SetShowHelp(false)
	model.DisableQuitKeybindings()
	return model
}

func (m *Model) selectedMainSkill() (engine.Skill, bool) {
	item, ok := m.list.SelectedItem().(skillItem)
	if ok {
		return item.skill, true
	}
	if len(m.inv.Skills) == 0 {
		return engine.Skill{}, false
	}
	return m.inv.Skills[m.cursor], true
}


func (m *Model) syncMainCursor() {
	selected, ok := m.list.SelectedItem().(skillItem)
	if !ok {
		m.cursor = 0
		return
	}
	for i, skill := range m.inv.Skills {
		if skill.Location == selected.skill.Location {
			m.cursor = i
			return
		}
	}
	m.cursor = 0
}

func (m *Model) selectMainCursor() {
	if len(m.inv.Skills) == 0 {
		m.cursor = 0
		m.list.Select(0)
		return
	}

	items := m.list.Items()
	skillIndex := 0
	for i, item := range items {
		if _, ok := item.(skillItem); !ok {
			continue
		}
		if skillIndex == m.cursor {
			m.list.Select(i)
			return
		}
		skillIndex++
	}
	m.list.Select(0)
	m.syncMainCursor()
}

func (m *Model) resizeList() {
	reserved := 4 // title, help, blank line, trailing newline after the list
	if len(m.inv.Notices) > 0 {
		reserved += 2 + len(m.inv.Notices) // blank line, "Notices" line, one per notice
	}
	if m.status != "" {
		reserved += 2 // blank line, status line
	}

	height := m.height - reserved
	if height < 1 {
		height = len(m.list.Items())
	}

	width := m.width
	if width < 1 {
		width = 100
	}
	listWidth, detailWidth := splitPaneWidths(width)
	m.list.SetSize(listWidth, height)
	m.detail.width = detailWidth
	m.detail.height = height
}

func splitPaneWidths(width int) (int, int) {
	if width < 2 {
		return width, 0
	}

	gap := 1
	available := width - gap
	listWidth := available * 3 / 5
	if listWidth < 1 {
		listWidth = 1
	}
	detailWidth := available - listWidth
	if detailWidth < 1 {
		detailWidth = 1
		listWidth = width - gap - detailWidth
	}
	return listWidth, detailWidth
}
