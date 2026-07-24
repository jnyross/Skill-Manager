package tui

import (
	"github.com/charmbracelet/bubbles/key"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

type keyMap struct {
	main            bool
	library         bool
	bundle          bool
	move            key.Binding
	page            key.Binding
	jump            key.Binding
	filter          key.Binding
	clearFilter     key.Binding
	detailScroll    key.Binding
	archive         key.Binding
	suppress        key.Binding
	manualOnly      key.Binding
	uninstallPlugin key.Binding
	libraryToggle   key.Binding
	switchView      key.Binding
	libraryView     key.Binding
	bundleView      key.Binding
	setup           key.Binding
	create          key.Binding
	addMember       key.Binding
	removeMember    key.Binding
	expand          key.Binding
	restore         key.Binding
	purge           key.Binding
	libraryRemove   key.Binding
	libraryInstall  key.Binding
	showFullHelp    key.Binding
	quit            key.Binding
}

func mainKeyMap(selected engine.Skill, ok bool, showAll bool) keyMap {
	m := baseKeyMap(showAll)
	m.main = true
	m.page = key.NewBinding(key.WithKeys("pgup", "pgdown"), key.WithHelp("pgup/pgdn", "page the list"))
	m.jump = key.NewBinding(key.WithKeys("home", "end"), key.WithHelp("home/end", "first/last"))
	m.detailScroll = key.NewBinding(key.WithKeys("ctrl+pgup", "ctrl+pgdown", "ctrl+u", "ctrl+d"), key.WithHelp("ctrl+u/ctrl+d", "scroll detail"))
	m.archive = key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "Archive"))
	m.suppress = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "Suppress"))
	m.manualOnly = key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "Manual-only"))
	m.uninstallPlugin = key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "Uninstall plugin"))
	m.libraryToggle = key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "Library"))
	m.switchView = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "Archive view"))
	m.libraryView = key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "Library view"))
	m.bundleView = key.NewBinding(key.WithKeys("B"), key.WithHelp("B", "Bundle view"))
	m.setup = key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "Setup workspace"))

	m.move.SetEnabled(ok)
	m.page.SetEnabled(ok)
	m.jump.SetEnabled(ok)
	m.detailScroll.SetEnabled(ok)
	m.archive.SetEnabled(ok && canArchiveSkill(selected))
	m.suppress.SetEnabled(ok && canSuppressSkill(selected))
	m.manualOnly.SetEnabled(ok && canToggleManualOnly(selected))
	m.uninstallPlugin.SetEnabled(ok && canUninstallPlugin(selected))
	m.libraryToggle.SetEnabled(ok && canToggleLibraryMembership(selected))
	return m
}

func archiveKeyMap(hasSelection bool, showAll bool) keyMap {
	m := baseKeyMap(showAll)
	m.restore = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "Restore"))
	m.purge = key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "Purge"))
	m.switchView = key.NewBinding(key.WithKeys("a", "esc"), key.WithHelp("a/esc", "main view"))

	m.move.SetEnabled(hasSelection)
	m.restore.SetEnabled(hasSelection)
	m.purge.SetEnabled(hasSelection)
	return m
}

func libraryKeyMap(hasSelection bool, showAll bool) keyMap {
	m := baseKeyMap(showAll)
	m.library = true
	m.libraryInstall = key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "Install"))
	m.create = key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new entry"))
	m.libraryRemove = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "remove entry"))
	m.switchView = key.NewBinding(key.WithKeys("L", "esc"), key.WithHelp("L/esc", "main view"))

	m.move.SetEnabled(hasSelection)
	m.libraryInstall.SetEnabled(hasSelection)
	m.libraryRemove.SetEnabled(hasSelection)
	return m
}

func bundleKeyMap(hasSelection bool, showAll bool) keyMap {
	m := baseKeyMap(showAll)
	m.bundle = true
	m.create = key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new Bundle"))
	m.addMember = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add member"))
	m.removeMember = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "remove member"))
	m.expand = key.NewBinding(key.WithKeys("enter", " "), key.WithHelp("enter/space", "expand/collapse"))
	// `m` is a binary Auto <-> Manual-only toggle, not a cycle through more
	// than two states — the help text says exactly what the handler does.
	m.manualOnly = key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "toggle Activation"))
	m.libraryInstall = key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "Install Bundle"))
	m.libraryRemove = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete Bundle"))
	m.switchView = key.NewBinding(key.WithKeys("B", "esc"), key.WithHelp("B/esc", "main view"))
	m.move.SetEnabled(hasSelection)
	m.addMember.SetEnabled(hasSelection)
	m.removeMember.SetEnabled(hasSelection)
	m.expand.SetEnabled(hasSelection)
	m.manualOnly.SetEnabled(hasSelection)
	m.libraryInstall.SetEnabled(hasSelection)
	m.libraryRemove.SetEnabled(hasSelection)
	return m
}

func baseKeyMap(showAll bool) keyMap {
	showHelp := key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more"))
	if showAll {
		showHelp.SetHelp("?", "less")
	}

	return keyMap{
		move:         key.NewBinding(key.WithKeys("up", "down", "k", "j"), key.WithHelp("↑↓/kj", "move")),
		filter:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		clearFilter:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear filter")),
		showFullHelp: showHelp,
		quit:         key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q/ctrl+c", "quit")),
	}
}

// ShortHelpRows is the compact help, split across as many lines as it takes to
// show every mode-changing key without truncating at 80 columns. Model.helpView
// renders one line per row. `?` still opens the grouped full help.
//
// Each row is kept under 80 columns including the " • " separators; adding a
// binding to a row means re-checking that budget (TestShortHelpRowsFit80Columns
// enforces it).
func (m keyMap) ShortHelpRows() [][]key.Binding {
	if m.main {
		return [][]key.Binding{
			{m.move, m.filter, m.showFullHelp, m.quit},
			{m.archive, m.suppress, m.manualOnly, m.uninstallPlugin, m.libraryToggle},
			{m.switchView, m.libraryView, m.bundleView, m.setup},
		}
	}
	if m.library {
		return [][]key.Binding{
			{m.move, m.filter, m.showFullHelp, m.quit},
			{m.libraryInstall, m.create, m.libraryRemove, m.switchView},
		}
	}
	if m.bundle {
		return [][]key.Binding{
			{m.move, m.filter, m.showFullHelp, m.quit},
			{m.expand, m.create, m.addMember, m.removeMember},
			{m.manualOnly, m.libraryInstall, m.libraryRemove, m.switchView},
		}
	}
	return [][]key.Binding{
		{m.move, m.filter, m.showFullHelp, m.quit},
		{m.restore, m.purge, m.switchView},
	}
}

// ShortHelp satisfies help.KeyMap. Model.helpView renders ShortHelpRows line by
// line instead; this flattened form exists for the interface and for callers
// that want the whole compact set.
func (m keyMap) ShortHelp() []key.Binding {
	var flat []key.Binding
	for _, row := range m.ShortHelpRows() {
		flat = append(flat, row...)
	}
	return flat
}

func (m keyMap) FullHelp() [][]key.Binding {
	if m.main {
		return [][]key.Binding{
			{m.move, m.switchView, m.libraryView, m.bundleView, m.setup, m.showFullHelp, m.quit},
			{m.archive, m.suppress, m.manualOnly, m.uninstallPlugin, m.libraryToggle, m.detailScroll},
			{m.page, m.jump, m.filter, m.clearFilter},
		}
	}
	if m.library {
		return [][]key.Binding{
			{m.move, m.switchView, m.showFullHelp, m.quit},
			{m.libraryInstall, m.libraryRemove, m.create},
			{m.filter, m.clearFilter},
		}
	}
	if m.bundle {
		return [][]key.Binding{
			{m.move, m.switchView, m.showFullHelp, m.quit},
			{m.create, m.expand, m.addMember, m.removeMember, m.manualOnly, m.libraryInstall, m.libraryRemove},
			{m.filter, m.clearFilter},
		}
	}

	return [][]key.Binding{
		{m.move, m.switchView, m.showFullHelp, m.quit},
		{m.restore, m.purge},
		{m.filter, m.clearFilter},
	}
}

func canArchiveSkill(skill engine.Skill) bool {
	return skill.Source == engine.SourcePersonal || skill.Source == engine.SourceCodex || skill.Source == engine.SourceProject
}

func canSuppressSkill(skill engine.Skill) bool {
	isCodexSkill := skill.Source == engine.SourceCodex && skill.Kind == engine.KindSkill
	isProjectCodexSkill := skill.Source == engine.SourceProject && skill.Tool == engine.ToolCodex && skill.Kind == engine.KindSkill
	return skill.Source == engine.SourcePlugin || isCodexSkill || isProjectCodexSkill
}

func canUninstallPlugin(skill engine.Skill) bool {
	return skill.Source == engine.SourcePlugin && skill.Plugin != nil
}

func canToggleManualOnly(skill engine.Skill) bool {
	return skill.Kind == engine.KindSkill && (skill.Source == engine.SourcePersonal || skill.Source == engine.SourceCodex || skill.Source == engine.SourceProject)
}

// archiveUnavailableReason returns empty when the skill can be archived;
// otherwise a user-facing reject reason that matches the gate.
func archiveUnavailableReason(skill engine.Skill) string {
	if canArchiveSkill(skill) {
		return ""
	}
	return "Only Personal, Codex, and Project skills can be archived."
}

// suppressUnavailableReason returns empty when Suppress applies; otherwise a
// user-facing reject reason that matches the gate (Plugin + Codex-mechanism).
func suppressUnavailableReason(skill engine.Skill) string {
	if canSuppressSkill(skill) {
		return ""
	}
	return "Suppress is only available for Plugin and Codex-mechanism skills."
}

// manualOnlyUnavailableReason returns empty when Manual-only applies;
// otherwise a user-facing reject reason that matches the gate.
func manualOnlyUnavailableReason(skill engine.Skill) string {
	if canToggleManualOnly(skill) {
		return ""
	}
	return "Manual-only is only available for Personal, Codex, and Project skills."
}

// needsCodexRestartHint is true when Suppress/Unsuppress writes Codex
// config.toml (user-level Codex or Project + Codex Tool).
func needsCodexRestartHint(skill engine.Skill) bool {
	return skill.Tool == engine.ToolCodex
}
