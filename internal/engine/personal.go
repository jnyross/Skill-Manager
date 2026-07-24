package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func scanPersonal(claudeHome string) ([]Skill, []Notice) {
	root := filepath.Join(claudeHome, "skills")
	return scanClaudeSkillFolder(root, SourcePersonal, ToolClaudeCode)
}

func scanClaudeSkillFolder(root string, source Source, tool Tool) ([]Skill, []Notice) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			// A standard directory that simply does not exist is the normal
			// state on a fresh machine, not something to report. The plugin
			// and Codex prompt scanners already treat absence this way.
			return nil, nil
		}
		return nil, []Notice{{Message: claudeSkillFolderNoticePrefix(source) + " directory unreadable: " + root + ": " + err.Error()}}
	}

	var skills []Skill
	var notices []Notice
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		folder := filepath.Join(root, entry.Name())
		info, err := os.Stat(folder)
		if err != nil {
			notices = append(notices, Notice{Message: fmt.Sprintf("Skipped %s: %v", entry.Name(), err)})
			continue
		}
		if !info.IsDir() {
			continue
		}

		skillPath := filepath.Join(folder, "SKILL.md")
		fm, err := parseSkillFrontmatter(skillPath)
		if err != nil {
			notices = append(notices, Notice{Message: fmt.Sprintf("Skipped %s: %v", entry.Name(), err)})
			continue
		}

		activation := ActivationAuto
		if fm.DisableModelInvocation != nil && *fm.DisableModelInvocation {
			activation = ActivationManualOnly
		}
		skill := Skill{
			Name:        fm.Name,
			Description: fm.Description,
			Source:      source,
			Tool:        tool,
			Kind:        KindSkill,
			Location:    absolutePath(folder),
			Activation:  activation,
		}
		// Cost accounting rides along on the frontmatter read this scan has
		// already done (internal/engine/cost.go).
		notices = append(notices, applyBodyCost(&skill, fm.bodyBytes)...)
		skills = append(skills, skill)
	}

	return skills, notices
}

func claudeSkillFolderNoticePrefix(source Source) string {
	if source == SourceProject {
		return "Project Claude skills"
	}
	return "Personal skills"
}
