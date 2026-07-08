package tui

import (
	"github.com/charmbracelet/bubbles/list"

	"skillet/internal/engine"
)

type skillItem struct {
	skill engine.Skill
}

func (i skillItem) FilterValue() string {
	return i.skill.Name
}

type groupHeaderItem struct {
	source engine.Source
}

func (i groupHeaderItem) FilterValue() string {
	return string(i.source)
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

func hasGroupHeader(source engine.Source) bool {
	switch source {
	case engine.SourcePersonal, engine.SourcePlugin, engine.SourceCodex, engine.SourceProject:
		return true
	default:
		return false
	}
}
