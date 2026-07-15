package engine_test

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

type fixture struct {
	root  string
	roots engine.Roots
}

type snapshotEntry struct {
	Mode   fs.FileMode
	Data   string
	Target string
}

type treeSnapshot map[string]snapshotEntry

func newFixture(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	roots := engine.Roots{
		ClaudeHome: filepath.Join(root, "claude"),
		CodexHome:  filepath.Join(root, "codex"),
		AgentsHome: filepath.Join(root, "agents"),
		DataDir:    filepath.Join(root, "data"),
	}
	mkdirAll(t,
		filepath.Join(roots.ClaudeHome, "skills"),
		filepath.Join(roots.ClaudeHome, "plugins"),
		filepath.Join(roots.CodexHome, "skills"),
		filepath.Join(roots.CodexHome, "prompts"),
		filepath.Join(roots.AgentsHome, "skills"),
		roots.DataDir,
	)
	return fixture{root: root, roots: roots}
}

func mkdirAll(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}
}

func writeSkill(t *testing.T, folder, name, description string, extraFrontmatter string) string {
	t.Helper()
	mkdirAll(t, folder)
	content := fmt.Sprintf("---\nname: %s\ndescription: %s\n%s---\nBody\n", strconv.Quote(name), strconv.Quote(description), extraFrontmatter)
	path := filepath.Join(folder, "SKILL.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write skill %s: %v", path, err)
	}
	return folder
}

func writePrompt(t *testing.T, path, description string) {
	t.Helper()
	content := fmt.Sprintf("---\ndescription: %s\nargument-hint: \"[value]\"\n---\nPrompt body\n", strconv.Quote(description))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write prompt %s: %v", path, err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	mkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func writePluginManifest(t *testing.T, claudeHome string, plugins map[string][]map[string]string) {
	t.Helper()
	data, err := json.MarshalIndent(map[string]any{
		"version": 2,
		"plugins": plugins,
	}, "", "  ")
	if err != nil {
		t.Fatalf("marshal plugin manifest: %v", err)
	}
	writeFile(t, filepath.Join(claudeHome, "plugins", "installed_plugins.json"), string(data))
}

// writeSettingsJSON writes a fixture <claudeHome>/settings.json with
// arbitrary top-level content (e.g. "enabledPlugins" alongside unrelated
// keys like "model"), mirroring the real user-level settings.json shape
// verified in docs/research/skill-mechanisms.md ("Enabled/disabled").
func writeSettingsJSON(t *testing.T, claudeHome string, contents map[string]any) {
	t.Helper()
	data, err := json.MarshalIndent(contents, "", "  ")
	if err != nil {
		t.Fatalf("marshal settings.json fixture: %v", err)
	}
	writeFile(t, filepath.Join(claudeHome, "settings.json"), string(data)+"\n")
}

// readJSONFile reads and unmarshals a JSON fixture file into a generic
// map, for tests that need to assert on the raw resulting file content
// (e.g. that unrelated keys/entries were left untouched) rather than going
// back through the engine's own read path.
func readJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return result
}

func sourceSkills(inv engine.Inventory, source engine.Source) []engine.Skill {
	var skills []engine.Skill
	for _, skill := range inv.Skills {
		if skill.Source == source {
			skills = append(skills, skill)
		}
	}
	return skills
}

func findSkill(inv engine.Inventory, source engine.Source, name string) (engine.Skill, bool) {
	for _, skill := range inv.Skills {
		if skill.Source == source && skill.Name == name {
			return skill, true
		}
	}
	return engine.Skill{}, false
}

func noticesContain(inv engine.Inventory, fragment string) bool {
	for _, notice := range inv.Notices {
		if strings.Contains(notice.Message, fragment) {
			return true
		}
	}
	return false
}

func archiveContains(entries []engine.ArchiveEntry, id string) bool {
	for _, entry := range entries {
		if entry.ID == id {
			return true
		}
	}
	return false
}

func snapshotTree(t *testing.T, root string) treeSnapshot {
	t.Helper()
	result := treeSnapshot{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		entry := snapshotEntry{Mode: info.Mode()}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entry.Target = target
		} else if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			entry.Data = string(data)
		}
		result[rel] = entry
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return result
}

func assertSnapshotsEqual(t *testing.T, before, after treeSnapshot) {
	t.Helper()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("tree changed:\nbefore=%#v\nafter=%#v", before, after)
	}
}
