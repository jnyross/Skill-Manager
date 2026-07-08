package engine

import (
	"sort"
)

func (e *Engine) Inventory() Inventory {
	personalSkills, personalNotices := scanPersonal(e.roots.ClaudeHome)
	pluginSkills, pluginNotices := scanPlugins(e.roots.ClaudeHome, e.roots.DataDir)
	codexSkills, codexNotices := scanCodex(e.roots.CodexHome, e.roots.AgentsHome)
	projectSkills, projectNotices := scanProject(e.roots.ClaudeProjectRoots, e.roots.ProjectRoots, e.roots.CodexHome)

	skills := append([]Skill{}, personalSkills...)
	skills = append(skills, pluginSkills...)
	skills = append(skills, codexSkills...)
	skills = append(skills, projectSkills...)
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
	notices = append(notices, projectNotices...)

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
	case SourceProject:
		return 3
	default:
		return 4
	}
}
