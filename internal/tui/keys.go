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
	search          key.Binding
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
	m.page = key.NewBinding(key.WithKeys("pgup", "pgdown"), key.WithHelp("pgup/pgdn", "page"))
	m.jump = key.NewBinding(key.WithKeys("home", "end"), key.WithHelp("home/end", "jump"))
	m.search = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search"))
	m.detailScroll = key.NewBinding(key.WithKeys("ctrl+pgup", "ctrl+pgdown", "ctrl+u", "ctrl+d"), key.WithHelp("ctrl+pgup/dn", "scroll detail"))
	m.archive = key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "archive"))
	m.suppress = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "suppress/un-suppress"))
	m.manualOnly = key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "manual-only/auto-activate"))
	m.uninstallPlugin = key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "uninstall plugin"))
	m.libraryToggle = key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "add/remove library"))
	m.switchView = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "archive view"))
	m.libraryView = key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "library view"))
	m.bundleView = key.NewBinding(key.WithKeys("B"), key.WithHelp("B", "bundle view"))
	m.setup = key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "setup workspace"))

	m.move.SetEnabled(ok)
	m.page.SetEnabled(ok)
	m.jump.SetEnabled(ok)
	m.search.SetEnabled(ok)
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
	m.restore = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "restore"))
	m.purge = key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "purge"))
	m.switchView = key.NewBinding(key.WithKeys("a", "esc"), key.WithHelp("a/esc", "main view"))

	m.move.SetEnabled(hasSelection)
	m.restore.SetEnabled(hasSelection)
	m.purge.SetEnabled(hasSelection)
	return m
}

func libraryKeyMap(hasSelection bool, showAll bool) keyMap {
	m := baseKeyMap(showAll)
	m.library = true
	m.libraryInstall = key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "install"))
	m.create = key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new entry"))
	m.libraryRemove = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "remove from library"))
	m.switchView = key.NewBinding(key.WithKeys("L", "esc"), key.WithHelp("L/esc", "main view"))

	m.move.SetEnabled(hasSelection)
	m.libraryInstall.SetEnabled(hasSelection)
	m.libraryRemove.SetEnabled(hasSelection)
	return m
}

func bundleKeyMap(hasSelection bool, showAll bool) keyMap {
	m := baseKeyMap(showAll)
	m.bundle = true
	m.create = key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new bundle"))
	m.addMember = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add member"))
	m.removeMember = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "remove member"))
	m.expand = key.NewBinding(key.WithKeys("enter", " "), key.WithHelp("enter/space", "expand/collapse"))
	m.manualOnly = key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "cycle activation"))
	m.libraryInstall = key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "install bundle"))
	m.libraryRemove = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete bundle"))
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
		move:         key.NewBinding(key.WithKeys("up", "down", "k", "j"), key.WithHelp("up/k down/j", "move")),
		showFullHelp: showHelp,
		quit:         key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q/ctrl+c", "quit")),
	}
}

func (m keyMap) ShortHelp() []key.Binding {
	if m.main {
		// Keep the short help to 5–6 bindings so it does not truncate on an
		// 80-column terminal. Full help (press ?) contains the rest.
		return []key.Binding{
			m.move,
			m.archive,
			m.switchView,
			m.setup,
			m.showFullHelp,
			m.quit,
		}
	}
	if m.library {
		return []key.Binding{
			m.move,
			m.libraryInstall,
			m.create,
			m.libraryRemove,
			m.switchView,
			m.showFullHelp,
			m.quit,
		}
	}
	if m.bundle {
		return []key.Binding{m.move, m.expand, m.create, m.addMember, m.removeMember, m.manualOnly, m.libraryInstall, m.libraryRemove, m.switchView, m.showFullHelp, m.quit}
	}

	return []key.Binding{
		m.move,
		m.restore,
		m.purge,
		m.switchView,
		m.showFullHelp,
		m.quit,
	}
}

func (m keyMap) FullHelp() [][]key.Binding {
	if m.main {
		return [][]key.Binding{
			{m.move, m.switchView, m.libraryView, m.bundleView, m.setup, m.showFullHelp, m.quit},
			{m.archive, m.suppress, m.manualOnly, m.uninstallPlugin, m.libraryToggle, m.detailScroll},
			{m.page, m.jump, m.search},
		}
	}
	if m.library {
		return [][]key.Binding{
			{m.move, m.switchView, m.showFullHelp, m.quit},
			{m.libraryInstall, m.libraryRemove, m.create},
		}
	}
	if m.bundle {
		return [][]key.Binding{
			{m.move, m.switchView, m.showFullHelp, m.quit},
			{m.create, m.expand, m.addMember, m.removeMember, m.manualOnly, m.libraryInstall, m.libraryRemove},
		}
	}

	return [][]key.Binding{
		{m.move, m.switchView, m.showFullHelp, m.quit},
		{m.restore, m.purge},
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
