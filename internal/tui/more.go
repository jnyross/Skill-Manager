package tui

import "strings"

// moreMenu is the single secondary entry point for everything that is not the
// main view's job. Library, Bundles, and Setup all still work from their own
// keys — this exists so the compact help does not have to advertise three more
// destinations while the user is trying to see and reduce what Auto-activates.
//
// It renders through renderConfirmOverlay, which always returns exactly one
// screen of lines, so opening it costs the layout budget nothing.
type moreMenu struct {
	cursor int
}

type moreEntry struct {
	key   string
	label string
	// detail says what the destination is for, in CONTEXT.md's words, because
	// this menu is where a user who does not know what a Bundle is will look.
	detail string
}

var moreEntries = []moreEntry{
	{key: "L", label: "Library", detail: "your own catalog of Skills and plugins, and Install from it"},
	{key: "B", label: "Bundles", detail: "named groups of Library entries for repeatable setup"},
	{key: "S", label: "Setup", detail: "guided agent-ready workspace setup for a repo"},
}

// updateMoreMenu drives the menu. Every destination answers to its own letter
// as well as to enter, so the menu teaches the shortcut it replaces.
func (m *Model) updateMoreMenu(key string) {
	menu := m.moreMenu
	if menu == nil {
		return
	}
	switch key {
	case "esc", "q", "o":
		m.moreMenu = nil
		m.setStatus("Canceled.")
	case "up", "k":
		if menu.cursor > 0 {
			menu.cursor--
		}
	case "down", "j":
		if menu.cursor < len(moreEntries)-1 {
			menu.cursor++
		}
	case "enter":
		m.chooseMoreEntry(moreEntries[menu.cursor].key)
	default:
		for _, entry := range moreEntries {
			if key == entry.key {
				m.chooseMoreEntry(entry.key)
				return
			}
		}
	}
}

// chooseMoreEntry closes the menu and hands the key to the main view's own
// handler, so a destination behaves identically whether it was reached through
// the menu or by its shortcut.
func (m *Model) chooseMoreEntry(key string) {
	m.moreMenu = nil
	m.updateMain(key)
}

func renderMoreMenu(cursor int) string {
	var b strings.Builder
	b.WriteString("More\n\n")
	for index, entry := range moreEntries {
		marker := "  "
		label := entry.label
		if index == cursor {
			marker = "> "
			label = selectedSkillRowStyle.Render(label)
		}
		b.WriteString(marker + entry.key + "  " + label + "\n")
		b.WriteString("     " + skillMetaStyle.Render(entry.detail) + "\n")
	}
	b.WriteString("\n" + skillMetaStyle.Render("enter or the letter opens · esc closes"))
	return b.String()
}
