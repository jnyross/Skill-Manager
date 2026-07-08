package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

type codexConfig struct {
	Skills struct {
		Config []struct {
			Path    string `toml:"path"`
			Name    string `toml:"name"`
			Enabled *bool  `toml:"enabled"`
		} `toml:"config"`
	} `toml:"skills"`
}

type openAIConfig struct {
	Policy    *openAIPolicy `yaml:"policy"`
	Interface *struct {
		Policy *openAIPolicy `yaml:"policy"`
	} `yaml:"interface"`
}

type openAIPolicy struct {
	AllowImplicitInvocation *bool `yaml:"allow_implicit_invocation"`
}

func scanCodex(codexHome, agentsHome string) ([]Skill, []Notice) {
	var notices []Notice
	disabled, configNotices := readCodexDisabledConfig(codexHome)
	notices = append(notices, configNotices...)

	byName := make(map[string]Skill)
	agentsSkills, agentsNotices := scanCodexSkillRoot(filepath.Join(agentsHome, "skills"), disabled)
	notices = append(notices, agentsNotices...)
	for _, skill := range agentsSkills {
		byName[skill.Name] = skill
	}

	codexSkills, codexNotices := scanCodexSkillRoot(filepath.Join(codexHome, "skills"), disabled)
	notices = append(notices, codexNotices...)
	for _, skill := range codexSkills {
		if _, exists := byName[skill.Name]; !exists {
			byName[skill.Name] = skill
		}
	}

	var skills []Skill
	for _, skill := range byName {
		skills = append(skills, skill)
	}

	prompts, promptNotices := scanCodexPrompts(filepath.Join(codexHome, "prompts"))
	notices = append(notices, promptNotices...)
	skills = append(skills, prompts...)

	return skills, notices
}

func scanCodexSkillRoot(root string, disabled codexDisabledConfig) ([]Skill, []Notice) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, []Notice{{Message: "Codex skills directory not found: " + root}}
		}
		return nil, []Notice{{Message: "Codex skills directory unreadable: " + root + ": " + err.Error()}}
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

		activation := codexOpenAIActivation(filepath.Join(folder, "agents", "openai.yaml"))
		if disabled.matches(skillPath, fm.Name) {
			activation = ActivationDisabled
		}

		skills = append(skills, Skill{
			Name:        fm.Name,
			Description: fm.Description,
			Source:      SourceCodex,
			Kind:        KindSkill,
			Location:    absolutePath(folder),
			Activation:  activation,
		})
	}

	return skills, notices
}

func codexOpenAIActivation(path string) ActivationState {
	data, err := os.ReadFile(path)
	if err != nil {
		return ActivationAuto
	}

	var cfg openAIConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ActivationAuto
	}

	policy := cfg.Policy
	if policy == nil && cfg.Interface != nil {
		policy = cfg.Interface.Policy
	}
	if policy == nil || policy.AllowImplicitInvocation == nil {
		return ActivationAuto
	}
	if !*policy.AllowImplicitInvocation {
		return ActivationManualOnly
	}
	return ActivationAuto
}

type codexDisabledConfig struct {
	paths map[string]bool
	names map[string]bool
}

func readCodexDisabledConfig(codexHome string) (codexDisabledConfig, []Notice) {
	result := codexDisabledConfig{
		paths: make(map[string]bool),
		names: make(map[string]bool),
	}
	configPath := filepath.Join(codexHome, "config.toml")
	var cfg codexConfig
	if _, err := toml.DecodeFile(configPath, &cfg); err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return result, []Notice{{Message: "Codex config unreadable: " + configPath + ": " + err.Error()}}
	}

	for _, entry := range cfg.Skills.Config {
		if entry.Enabled == nil || *entry.Enabled {
			continue
		}
		// Codex's real runtime validator (core-skills/src/config_rules.rs,
		// per docs/research/skill-mechanisms.md's "Re-verification against
		// codex-cli 0.143.0" section) ignores any entry that sets both
		// `path` and `name`, or neither — it never disables the skill in
		// either case. Skip such entries here so this reader never disagrees
		// with what Codex itself actually does with them.
		hasPath := entry.Path != ""
		hasName := entry.Name != ""
		if hasPath == hasName {
			continue
		}
		if hasPath {
			result.paths[absolutePath(entry.Path)] = true
		}
		if hasName {
			result.names[entry.Name] = true
		}
	}
	return result, nil
}

func (c codexDisabledConfig) matches(skillPath, name string) bool {
	return c.paths[absolutePath(skillPath)] || c.names[name]
}

func scanCodexPrompts(root string) ([]Skill, []Notice) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []Notice{{Message: "Codex prompts directory unreadable: " + root + ": " + err.Error()}}
	}

	var skills []Skill
	var notices []Notice
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || filepath.Ext(entry.Name()) != ".md" {
			continue
		}

		path := filepath.Join(root, entry.Name())
		fm, err := parsePromptFrontmatter(path)
		if err != nil {
			notices = append(notices, Notice{Message: fmt.Sprintf("Skipped %s: %v", entry.Name(), err)})
			continue
		}

		skills = append(skills, Skill{
			Name:        strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())),
			Description: fm.Description,
			Source:      SourceCodex,
			Kind:        KindPrompt,
			Location:    absolutePath(path),
			Activation:  ActivationManualOnly,
		})
	}
	return skills, notices
}

func absolutePath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
