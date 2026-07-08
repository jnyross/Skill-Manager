package tui

import (
	"github.com/charmbracelet/bubbles/key"

	"skillet/internal/engine"
)

type keyMap struct {
	main            bool
	move            key.Binding
	archive         key.Binding
	suppress        key.Binding
	manualOnly      key.Binding
	uninstallPlugin key.Binding
	switchView      key.Binding
	restore         key.Binding
	purge           key.Binding
	showFullHelp    key.Binding
	quit            key.Binding
}

func mainKeyMap(selected engine.Skill, ok bool, showAll bool) keyMap {
	m := baseKeyMap(showAll)
	m.main = true
	m.archive = key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "archive"))
	m.suppress = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "suppress/un-suppress"))
	m.manualOnly = key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "manual-only/auto-activate"))
	m.uninstallPlugin = key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "uninstall plugin"))
	m.switchView = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "archive view"))

	m.move.SetEnabled(ok)
	m.archive.SetEnabled(ok && canArchiveSkill(selected))
	m.suppress.SetEnabled(ok && canSuppressSkill(selected))
	m.manualOnly.SetEnabled(ok && canToggleManualOnly(selected))
	m.uninstallPlugin.SetEnabled(ok && canUninstallPlugin(selected))
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
		return []key.Binding{
			m.move,
			m.archive,
			m.suppress,
			m.manualOnly,
			m.uninstallPlugin,
			m.switchView,
			m.showFullHelp,
			m.quit,
		}
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
			{m.move, m.switchView, m.showFullHelp, m.quit},
			{m.archive, m.suppress, m.manualOnly, m.uninstallPlugin},
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
