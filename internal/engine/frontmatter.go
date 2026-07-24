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
	// bodyBytes is the size of the whole file this frontmatter came from — the
	// Skill's invoked cost (internal/engine/cost.go). It is unexported and has
	// no yaml tag on purpose: it is measured, not declared, and a SKILL.md
	// cannot claim its own size.
	bodyBytes int64
}

func (f *skillFrontmatter) setBodyBytes(size int64) { f.bodyBytes = size }

type promptFrontmatter struct {
	Description string `yaml:"description"`
	bodyBytes   int64
}

func (f *promptFrontmatter) setBodyBytes(size int64) { f.bodyBytes = size }

// bodySizer is implemented by the frontmatter types that want the size of the
// file they were parsed from. parseFrontmatter fills it from the handle it has
// already opened, so knowing a Skill's size costs one fstat and never a second
// open — this is what keeps cost accounting inside the existing scan pass.
type bodySizer interface {
	setBodyBytes(size int64)
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

	if sizer, ok := out.(bodySizer); ok {
		if info, statErr := file.Stat(); statErr == nil {
			sizer.setBodyBytes(info.Size())
		}
	}

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
