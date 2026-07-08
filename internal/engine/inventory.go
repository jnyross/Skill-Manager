package engine

import (
	"sort"
)

func (e *Engine) Inventory() Inventory {
	personalSkills, personalNotices := scanPersonal(e.roots.ClaudeHome)
	pluginSkills, pluginNotices := scanPlugins(e.roots.ClaudeHome)
	codexSkills, codexNotices := scanCodex(e.roots.CodexHome, e.roots.AgentsHome)

	skills := append([]Skill{}, personalSkills...)
	skills = append(skills, pluginSkills...)
	skills = append(skills, codexSkills...)
	sort.SliceStable(skills, func(i, j int) bool {
		leftSource := sourceSortOrder(skills[i].Source)
		rightSource := sourceSortOrder(skills[j].Source)
		if leftSource != rightSource {
			return leftSource < rightSource
		}
		return skills[i].Name < skills[j].Name
	})

	notices := append([]Notice{}, personalNotices...)
	notices = append(notices, pluginNotices...)
	notices = append(notices, codexNotices...)

	return Inventory{
		Skills:  skills,
		Notices: notices,
	}
}

func sourceSortOrder(source Source) int {
	switch source {
	case SourcePersonal:
		return 0
	case SourcePlugin:
		return 1
	case SourceCodex:
		return 2
	default:
		return 3
	}
}
