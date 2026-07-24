package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

type bundleItem struct {
	bundle engine.Bundle
	member *engine.BundleMember
	name   string
}

// FilterValue covers the Bundle name and, for a member row, the Library entry
// name and its remembered Activation.
func (i bundleItem) FilterValue() string {
	parts := []string{i.bundle.Name, i.name}
	if i.member != nil {
		parts = append(parts, string(i.member.Activation))
	}
	return strings.Join(parts, " ")
}

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
	// See newLibraryList: filtering on, the list's own filter bar off.
	m.SetShowFilter(false)
	m.SetFilteringEnabled(true)
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
	fmt.Fprint(w, renderBundleItem(row, index == m.Index(), m.Width()))
}

func renderBundleItem(row bundleItem, selected bool, width int) string {
	cursor := " "
	if selected {
		cursor = ">"
	}
	text := row.bundle.Name
	if row.member == nil {
		text = fmt.Sprintf("%s (%d members)", text, len(row.bundle.Members))
	} else {
		text = fmt.Sprintf("  %s [%s]", row.name, row.member.Activation)
	}

	style := skillRowStyle
	if selected {
		style = selectedSkillRowStyle
	}
	// MaxWidth (not Width) for the same reason as renderSkillItem: Width pads
	// and wraps, which turns an over-length row into 2+ terminal lines and
	// desyncs the delegate's fixed Height()==1 from what is actually drawn.
	return style.MaxWidth(width).Render(fmt.Sprintf("%s %s", cursor, text))
}
