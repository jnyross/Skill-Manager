package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type installedPluginsManifest struct {
	Plugins map[string][]pluginInstall `json:"plugins"`
}

type pluginInstall struct {
	Scope       string `json:"scope"`
	InstallPath string `json:"installPath"`
}

func scanPlugins(claudeHome string) ([]Skill, []Notice) {
	manifestPath := filepath.Join(claudeHome, "plugins", "installed_plugins.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []Notice{{Message: "Plugin manifest unreadable: " + manifestPath + ": " + err.Error()}}
	}

	var manifest installedPluginsManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, []Notice{{Message: "Plugin manifest malformed: " + manifestPath + ": " + err.Error()}}
	}

	var skills []Skill
	var notices []Notice
	for key, installs := range manifest.Plugins {
		pluginName, marketplace, ok := splitPluginKey(key)
		if !ok {
			notices = append(notices, Notice{Message: "Plugin manifest entry has invalid key: " + key})
			continue
		}

		for _, install := range installs {
			if install.Scope != "user" {
				continue
			}
			if _, err := os.Stat(install.InstallPath); err != nil {
				if os.IsNotExist(err) {
					notices = append(notices, Notice{Message: fmt.Sprintf("Plugin %s: install path not found: %s", key, install.InstallPath)})
				} else {
					notices = append(notices, Notice{Message: fmt.Sprintf("Plugin %s: install path unreadable: %s: %v", key, install.InstallPath, err)})
				}
				continue
			}

			pluginSkills, pluginNotices := scanPluginInstall(pluginName, marketplace, install.InstallPath)
			notices = append(notices, pluginNotices...)
			for i := range pluginSkills {
				pluginSkills[i].Plugin.SkillCount = len(pluginSkills)
			}
			skills = append(skills, pluginSkills...)
		}
	}

	return skills, notices
}

func splitPluginKey(key string) (string, string, bool) {
	index := strings.LastIndex(key, "@")
	if index <= 0 || index == len(key)-1 {
		return "", "", false
	}
	return key[:index], key[index+1:], true
}

func scanPluginInstall(pluginName, marketplace, installPath string) ([]Skill, []Notice) {
	skillsRoot := filepath.Join(installPath, "skills")
	if _, err := os.Stat(skillsRoot); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []Notice{{Message: fmt.Sprintf("Plugin %s@%s: skills directory unreadable: %s: %v", pluginName, marketplace, skillsRoot, err)}}
	}

	var skills []Skill
	var notices []Notice
	err := filepath.WalkDir(skillsRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			notices = append(notices, Notice{Message: fmt.Sprintf("Plugin %s@%s: skipped %s: %v", pluginName, marketplace, path, err)})
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != "SKILL.md" {
			return nil
		}

		folder := filepath.Dir(path)
		fm, err := parseSkillFrontmatter(path)
		if err != nil {
			notices = append(notices, Notice{Message: fmt.Sprintf("Skipped %s: %v", folder, err)})
			return nil
		}
		skills = append(skills, Skill{
			Name:        fm.Name,
			Description: fm.Description,
			Source:      SourcePlugin,
			Kind:        KindSkill,
			Location:    absolutePath(folder),
			Activation:  ActivationAuto,
			Plugin: &PluginInfo{
				Plugin:      pluginName,
				Marketplace: marketplace,
			},
		})
		return nil
	})
	if err != nil {
		notices = append(notices, Notice{Message: fmt.Sprintf("Plugin %s@%s: walk failed: %v", pluginName, marketplace, err)})
	}

	return skills, notices
}
