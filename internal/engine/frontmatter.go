package engine

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type skillFrontmatter struct {
	Name                   string `yaml:"name"`
	Description            string `yaml:"description"`
	DisableModelInvocation *bool  `yaml:"disable-model-invocation"`
	// UserInvocable is only read/written by Suppress (internal/engine/suppress.go);
	// Personal/Codex scans don't use it (see docs/research/skill-mechanisms.md).
	UserInvocable *bool `yaml:"user-invocable"`
}

type promptFrontmatter struct {
	Description string `yaml:"description"`
}

// frontmatterParseHook is a test-only seam: when non-nil it is called with
// the path of every SKILL.md/prompt frontmatter actually opened and parsed,
// so a test can assert a scan parses each file exactly once. Tests that
// install it must not run in parallel.
var frontmatterParseHook func(path string)

func parseFrontmatter(path string, out any) error {
	if frontmatterParseHook != nil {
		frontmatterParseHook(path)
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("read frontmatter: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("read frontmatter: %w", err)
		}
		return fmt.Errorf("missing frontmatter opening delimiter")
	}
	if trimLine(scanner.Text()) != "---" {
		return fmt.Errorf("missing frontmatter opening delimiter")
	}

	var yamlLines []string
	foundClosing := false
	for scanner.Scan() {
		line := scanner.Text()
		if trimLine(line) == "---" {
			foundClosing = true
			break
		}
		yamlLines = append(yamlLines, line)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read frontmatter: %w", err)
	}
	if !foundClosing {
		return fmt.Errorf("missing frontmatter closing delimiter")
	}
	if err := yaml.Unmarshal([]byte(strings.Join(yamlLines, "\n")), out); err != nil {
		return fmt.Errorf("parse frontmatter YAML: %w", err)
	}
	return nil
}

func parseSkillFrontmatter(path string) (skillFrontmatter, error) {
	var fm skillFrontmatter
	if err := parseFrontmatter(path, &fm); err != nil {
		return fm, err
	}
	if fm.Name == "" {
		return fm, fmt.Errorf("missing name in frontmatter")
	}
	if fm.Description == "" {
		return fm, fmt.Errorf("missing description in frontmatter")
	}
	return fm, nil
}

func parsePromptFrontmatter(path string) (promptFrontmatter, error) {
	var fm promptFrontmatter
	if err := parseFrontmatter(path, &fm); err != nil {
		return fm, err
	}
	if fm.Description == "" {
		return fm, fmt.Errorf("missing description in frontmatter")
	}
	return fm, nil
}

func trimLine(line string) string {
	return strings.TrimSuffix(line, "\r")
}
