package engine

import (
	"fmt"
	"os"
	"path/filepath"
)

func scanProject(claudeRoots, agentsRoots []string, codexHome string) ([]Skill, []Notice) {
	var skills []Skill
	var notices []Notice

	disabled, configNotices := readCodexDisabledConfig(codexHome)
	notices = append(notices, configNotices...)

	for _, root := range claudeRoots {
		dir := filepath.Join(root, ".claude", "skills")
		if !projectSkillDirExists(dir, &notices) {
			continue
		}
		found, foundNotices := scanClaudeSkillFolder(dir, SourceProject, ToolClaudeCode)
		skills = append(skills, found...)
		notices = append(notices, foundNotices...)
	}

	for _, root := range agentsRoots {
		dir := filepath.Join(root, ".agents", "skills")
		if !projectSkillDirExists(dir, &notices) {
			continue
		}
		found, foundNotices := scanCodexSkillRoot(dir, SourceProject, disabled)
		skills = append(skills, found...)
		notices = append(notices, foundNotices...)
	}

	return skills, notices
}

func projectSkillDirExists(dir string, notices *[]Notice) bool {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		*notices = append(*notices, Notice{Message: fmt.Sprintf("Project skills directory unreadable: %s: %v", dir, err)})
		return false
	}
	if !info.IsDir() {
		*notices = append(*notices, Notice{Message: "Project skills path is not a directory: " + dir})
		return false
	}
	return true
}
