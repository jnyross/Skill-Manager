package engine

import (
	"sort"
)

func (e *Engine) Inventory() Inventory {
	// ~/.codex/config.toml is read and decoded exactly once per Inventory()
	// and threaded to both consumers (the Codex scan and the Codex half of
	// the Project scan). Both used to read it independently, which meant two
	// reads plus two TOML decodes per refresh — and, when the file was
	// unreadable, the same notice reported twice.
	codexDisabled, codexConfigNotices := readCodexDisabledConfig(e.roots.CodexHome)

	personalSkills, personalNotices := scanPersonal(e.roots.ClaudeHome)
	pluginSkills, pluginNotices := scanPlugins(e.roots.ClaudeHome, e.roots.DataDir)
	codexSkills, codexNotices := scanCodex(e.roots.CodexHome, e.roots.AgentsHome, codexDisabled)
	projectSkills, projectNotices := scanProject(e.roots.ClaudeProjectRoots, e.roots.ProjectRoots, codexDisabled)

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
	// Config notices keep their old position: first in the Codex group.
	notices = append(notices, codexConfigNotices...)
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
