package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func scanPersonal(claudeHome string) ([]Skill, []Notice) {
	root := filepath.Join(claudeHome, "skills")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, []Notice{{Message: "Personal skills directory not found: " + root}}
		}
		return nil, []Notice{{Message: "Personal skills directory unreadable: " + root + ": " + err.Error()}}
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
		skills = append(skills, Skill{
			Name:        fm.Name,
			Description: fm.Description,
			Source:      SourcePersonal,
			Kind:        KindSkill,
			Location:    absolutePath(folder),
			Activation:  activation,
		})
	}

	return skills, notices
}
