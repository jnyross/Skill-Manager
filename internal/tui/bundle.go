package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"skillet/internal/engine"
)

type bundleItem struct {
	bundle engine.Bundle
	member *engine.BundleMember
	name   string
}

func (i bundleItem) FilterValue() string { return i.bundle.Name + " " + i.name }

func buildBundleItems(bundles []engine.Bundle, library []engine.LibraryEntry, expansion ...map[string]bool) []list.Item {
	names := make(map[string]string, len(library))
	for _, entry := range library {
		names[entry.ID] = entry.Name
	}
	var items []list.Item
	for _, bundle := range bundles {
		items = append(items, bundleItem{bundle: bundle})
		if len(expansion) > 0 && !expansion[0][bundle.ID] {
			continue
		}
		for i := range bundle.Members {
			member := bundle.Members[i]
			name := names[member.LibraryEntryID]
			if name == "" {
				name = "missing: " + member.LibraryEntryID
			}
			items = append(items, bundleItem{bundle: bundle, member: &member, name: name})
		}
	}
	return items
}

func newBundleList(items []list.Item) list.Model {
	m := list.New(items, bundleDelegate{}, 0, 0)
	m.SetShowTitle(false)
	m.SetShowFilter(false)
	m.SetFilteringEnabled(false)
	m.SetShowStatusBar(false)
	m.SetShowPagination(false)
	m.SetShowHelp(false)
	m.DisableQuitKeybindings()
	return m
}

type bundleDelegate struct{}

func (bundleDelegate) Height() int                         { return 1 }
func (bundleDelegate) Spacing() int                        { return 0 }
func (bundleDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (bundleDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	row, ok := item.(bundleItem)
	if !ok {
		return
	}
	cursor := " "
	if index == m.Index() {
		cursor = ">"
	}
	text := row.bundle.Name
	if row.member == nil {
		text = fmt.Sprintf("%s (%d members)", text, len(row.bundle.Members))
	} else {
		text = fmt.Sprintf("  %s [%s]", row.name, row.member.Activation)
	}
	fmt.Fprintf(w, "%s %s", cursor, text)
}
