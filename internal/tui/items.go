package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

type skillItem struct {
	skill engine.Skill
}

// FilterValue is the haystack the list's fuzzy filter matches against. It
// covers everything a row shows plus the description, so "/" finds a skill by
// what it does, by its Source, or by the plugin it came from — not just by
// name.
func (i skillItem) FilterValue() string {
	parts := []string{
		i.skill.Name,
		i.skill.Description,
		string(i.skill.Source),
		string(i.skill.Tool),
	}
	if i.skill.Plugin != nil {
		parts = append(parts, i.skill.Plugin.Plugin, i.skill.Plugin.Marketplace)
	}
	return strings.Join(parts, " ")
}

type groupHeaderItem struct {
	source engine.Source
}

// FilterValue is deliberately empty: a Source header is chrome, not a result.
// An empty haystack never matches a non-empty query, so headers drop out while
// a filter is active and the filtered view is a flat list of matches.
func (i groupHeaderItem) FilterValue() string {
	return ""
}

// buildListItems assumes inventory.Skills is already ordered by Source then
// Name, matching engine.Inventory()'s own sort — it does not re-sort.
func buildListItems(inventory engine.Inventory) []list.Item {
	if len(inventory.Skills) == 0 {
		return nil
	}

	items := make([]list.Item, 0, len(inventory.Skills)+4)
	var current engine.Source
	for _, skill := range inventory.Skills {
		if skill.Source != current {
			current = skill.Source
			if hasGroupHeader(current) {
				items = append(items, groupHeaderItem{source: current})
			}
		}
		items = append(items, skillItem{skill: skill})
	}
	return items
}

// buildCostSortedListItems is the same list ranked by per-session cost instead
// of grouped by Source. It carries no Source headers on purpose: the whole
// point of the ranking is to put the most expensive Skills at the top wherever
// they came from, and headers would break that order into groups again. Each
// row still shows its own Source (see skillDelegate), so nothing is lost.
//
// It assumes inventory.Skills is already ordered by cost — Model.refreshInventory
// applies engine.SortByDescriptionCost — so the item order and the inventory
// order stay identical, which is what syncMainCursor relies on.
func buildCostSortedListItems(inventory engine.Inventory) []list.Item {
	if len(inventory.Skills) == 0 {
		return nil
	}
	items := make([]list.Item, 0, len(inventory.Skills))
	for _, skill := range inventory.Skills {
		items = append(items, skillItem{skill: skill})
	}
	return items
}

func hasGroupHeader(source engine.Source) bool {
	switch source {
	case engine.SourcePersonal, engine.SourcePlugin, engine.SourceCodex, engine.SourceProject:
		return true
	default:
		return false
	}
}
