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
	// frontmatter carries each scanned skill's already-parsed SKILL.md
	// frontmatter, keyed by the skill's absolute Location, into
	// applySuppressions — which used to re-open and re-parse the same file.
	frontmatter := make(map[string]skillFrontmatter)

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

					pluginSkills, pluginFrontmatter, pluginNotices := scanPluginInstall(pluginName, marketplace, install.InstallPath)
					notices = append(notices, pluginNotices...)
					for i := range pluginSkills {
						pluginSkills[i].Plugin.SkillCount = len(pluginSkills)
					}
					skills = append(skills, pluginSkills...)
					for location, fm := range pluginFrontmatter {
						frontmatter[location] = fm
					}
				}
			}
		}
	}

	records, recErr := loadSuppressionRecords(dataDir)
	if recErr != nil {
		notices = append(notices, Notice{Message: "Suppressed skills unreadable: " + recErr.Error()})
	} else {
		notices = append(notices, applySuppressions(skills, records, frontmatter)...)
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

// pluginScanSkipDirs are directory names that never contain a plugin skill
// but are expensive to walk when a plugin ships them (a vendored dependency
// tree, or a plugin distributed as a git checkout).
var pluginScanSkipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
}

// scanPluginInstall finds every skill under installPath/skills, returning the
// skills, their parsed frontmatter keyed by absolute Location (so
// applySuppressions need not re-read the same files), and any notices.
//
// The traversal is deliberately narrower than a full recursive walk of the
// skills tree. A directory containing a SKILL.md *is* a skill, so its subtree
// is that skill's own payload — references/, scripts/, assets/ — and is never
// descended into. Everything else is descended, because real marketplace
// plugins do nest: as of this writing mattpocock/mattpocock-skills groups its
// skills one extra level deep (skills/<category>/<name>/SKILL.md) while every
// other plugin in the same cache uses skills/<name>/SKILL.md, so narrowing to
// a fixed skills/*/SKILL.md glob would silently drop 41 real skills.
func scanPluginInstall(pluginName, marketplace, installPath string) ([]Skill, map[string]skillFrontmatter, []Notice) {
	skillsRoot := filepath.Join(installPath, "skills")
	if _, err := os.Stat(skillsRoot); err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, []Notice{{Message: fmt.Sprintf("Plugin %s@%s: skills directory unreadable: %s: %v", pluginName, marketplace, skillsRoot, err)}}
	}

	var skills []Skill
	var notices []Notice
	frontmatter := make(map[string]skillFrontmatter)

	var visit func(dir string, depth int)
	visit = func(dir string, depth int) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			notices = append(notices, Notice{Message: fmt.Sprintf("Plugin %s@%s: skipped %s: %v", pluginName, marketplace, dir, err)})
			return
		}

		for _, entry := range entries {
			if entry.IsDir() || entry.Name() != "SKILL.md" {
				continue
			}
			path := filepath.Join(dir, "SKILL.md")
			fm, parseErr := parseSkillFrontmatter(path)
			if parseErr != nil {
				notices = append(notices, Notice{Message: fmt.Sprintf("Skipped %s: %v", dir, parseErr)})
				// A directory whose SKILL.md is unparseable is still a skill
				// directory, not a container of nested skills — except at the
				// skills root, which always holds skill directories.
				if depth == 0 {
					break
				}
				return
			}
			location := absolutePath(dir)
			skills = append(skills, Skill{
				Name:        fm.Name,
				Description: fm.Description,
				Source:      SourcePlugin,
				Tool:        ToolClaudeCode,
				Kind:        KindSkill,
				Location:    location,
				Activation:  ActivationAuto,
				Plugin: &PluginInfo{
					Plugin:      pluginName,
					Marketplace: marketplace,
				},
			})
			frontmatter[location] = fm
			if depth == 0 {
				break
			}
			return
		}

		for _, entry := range entries {
			if !entry.IsDir() || pluginScanSkipDirs[entry.Name()] {
				continue
			}
			visit(filepath.Join(dir, entry.Name()), depth+1)
		}
	}
	visit(skillsRoot, 0)

	return skills, frontmatter, notices
}
