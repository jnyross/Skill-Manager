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

// scanPlugins scans every user-scoped installed plugin for skills, then
// reconciles the result against Skillet's own Suppress state (dataDir): see
// applySuppressions in suppress.go for the self-healing loop that keeps
// Suppressed plugin skills hidden across plugin updates. Unlike the rest of
// this file, that reconciliation step can write to disk (re-applying a
// suppression edit to a freshly updated SKILL.md) — a deliberate exception to
// "scans are pure reads", required by issue #9's self-healing-on-every-run
// design.
func scanPlugins(claudeHome, dataDir string) ([]Skill, []Notice) {
	manifestPath := filepath.Join(claudeHome, "plugins", "installed_plugins.json")
	data, err := os.ReadFile(manifestPath)

	var skills []Skill
	var notices []Notice
	var manifest installedPluginsManifest

	switch {
	case err != nil && os.IsNotExist(err):
		// No plugins installed at all; manifest stays zero-value (empty map).
		// Still fall through to suppression reconciliation below: any
		// recorded suppressions are legitimately stale in this state.
	case err != nil:
		notices = append(notices, Notice{Message: "Plugin manifest unreadable: " + manifestPath + ": " + err.Error()})
	default:
		if jsonErr := json.Unmarshal(data, &manifest); jsonErr != nil {
			notices = append(notices, Notice{Message: "Plugin manifest malformed: " + manifestPath + ": " + jsonErr.Error()})
		} else {
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
					if _, statErr := os.Stat(install.InstallPath); statErr != nil {
						if os.IsNotExist(statErr) {
							notices = append(notices, Notice{Message: fmt.Sprintf("Plugin %s: install path not found: %s", key, install.InstallPath)})
						} else {
							notices = append(notices, Notice{Message: fmt.Sprintf("Plugin %s: install path unreadable: %s: %v", key, install.InstallPath, statErr)})
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
		}
	}

	records, recErr := loadSuppressionRecords(dataDir)
	if recErr != nil {
		notices = append(notices, Notice{Message: "Suppressed skills unreadable: " + recErr.Error()})
	} else {
		notices = append(notices, applySuppressions(skills, records)...)
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
			Tool:        ToolClaudeCode,
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
