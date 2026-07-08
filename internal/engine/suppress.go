package engine

// Suppress for Claude Code Plugin skills — the highest-risk mechanism in
// Skillet's design (see docs/research/skill-mechanisms.md, "Plugin skills:
// no supported per-skill control"). Claude Code has no supported way to
// disable a single skill inside a plugin that survives a plugin update: each
// version is a fresh cache directory, so any direct edit is abandoned on
// update. Skillet works around this by owning the suppression decision
// itself (SuppressionRecord, keyed by marketplace+plugin+skill name, in
// types.go) and re-applying the frontmatter edit to whichever cache
// directory is current every time it scans (applySuppressions, called from
// scanPlugins in plugin.go on every Inventory()) — the "self-healing loop".
//
// The edit itself sets two SKILL.md frontmatter fields documented in
// skill-mechanisms.md: disable-model-invocation (blocks the model from
// auto-invoking) and user-invocable (hides it from the slash menu). Either
// alone only gets you Manual-only or a model-only skill; CONTEXT.md defines
// Suppress as hiding the skill from *both* the model and slash commands, so
// both fields are set together.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var suppressionFieldKeys = []string{"disable-model-invocation", "user-invocable"}

// Suppress hides a Plugin skill from the model and slash menu by editing its
// currently-cached SKILL.md and recording the decision in Skillet's own data
// directory so it survives future plugin updates (see applySuppressions).
// skill must be a Plugin skill as returned by a recent Inventory() call (its
// Location is the current cache directory).
func (e *Engine) Suppress(skill Skill) error {
	if skill.Source != SourcePlugin || skill.Plugin == nil {
		return fmt.Errorf("suppress skill: not a Plugin skill: %s", skill.Name)
	}

	skillMDPath := filepath.Join(skill.Location, "SKILL.md")
	if err := applySuppressionEdit(skillMDPath); err != nil {
		return fmt.Errorf("suppress skill: %w", err)
	}

	record := SuppressionRecord{
		Marketplace:  skill.Plugin.Marketplace,
		Plugin:       skill.Plugin.Plugin,
		SkillName:    skill.Name,
		SuppressedAt: time.Now().UTC(),
	}
	if err := writeSuppressionRecord(e.roots.DataDir, record); err != nil {
		return fmt.Errorf("suppress skill: %w", err)
	}
	return nil
}

// Unsuppress removes the Skillet-owned suppression record and reverts the
// frontmatter edit on skill's currently-cached SKILL.md. It is safe to call
// even if the cached copy no longer carries the edit (e.g. it was never
// successfully applied) — reverting is a no-op in that case, not an error.
func (e *Engine) Unsuppress(skill Skill) error {
	if skill.Source != SourcePlugin || skill.Plugin == nil {
		return fmt.Errorf("unsuppress skill: not a Plugin skill: %s", skill.Name)
	}

	skillMDPath := filepath.Join(skill.Location, "SKILL.md")
	if err := revertSuppressionEdit(skillMDPath); err != nil {
		return fmt.Errorf("unsuppress skill: %w", err)
	}
	if err := removeSuppressionRecord(e.roots.DataDir, skill.Plugin.Marketplace, skill.Plugin.Plugin, skill.Name); err != nil {
		return fmt.Errorf("unsuppress skill: %w", err)
	}
	return nil
}

// applySuppressions reconciles already-scanned Plugin skills (mutated in
// place) against the recorded suppressions. For every skill with a matching
// record it ensures the cached SKILL.md carries the suppression edit —
// re-applying it if a plugin update has replaced the cache directory with an
// unedited copy — and sets Activation to ActivationSuppressed. This is the
// self-healing loop: it runs as a side effect of every scanPlugins call
// (i.e. every Inventory()), with no separate "heal" step required.
//
// Records with no matching skill among the scanned plugins are stale (most
// likely the plugin was uninstalled, or the specific skill removed from it,
// outside of Skillet). Rather than silently doing nothing or silently
// deleting Skillet's own state, each stale record is surfaced as a Notice;
// the record itself is left in place so re-suppression resumes automatically
// if the plugin reappears (e.g. reinstalled at the same marketplace/plugin
// name).
func applySuppressions(skills []Skill, records []SuppressionRecord) []Notice {
	var notices []Notice
	matched := make(map[string]bool, len(records))

	for i := range skills {
		skill := &skills[i]
		if skill.Source != SourcePlugin || skill.Plugin == nil {
			continue
		}

		for _, record := range records {
			if record.Marketplace != skill.Plugin.Marketplace || record.Plugin != skill.Plugin.Plugin || record.SkillName != skill.Name {
				continue
			}
			matched[suppressionID(record.Marketplace, record.Plugin, record.SkillName)] = true

			skillMDPath := filepath.Join(skill.Location, "SKILL.md")
			fm, err := parseSkillFrontmatter(skillMDPath)
			if err != nil {
				notices = append(notices, Notice{Message: fmt.Sprintf("Suppressed skill %s (%s@%s): could not re-check frontmatter: %v", skill.Name, record.Plugin, record.Marketplace, err)})
				break
			}
			if !isSuppressionApplied(fm) {
				if err := applySuppressionEdit(skillMDPath); err != nil {
					notices = append(notices, Notice{Message: fmt.Sprintf("Suppressed skill %s (%s@%s): could not re-apply suppression after update: %v", skill.Name, record.Plugin, record.Marketplace, err)})
					break
				}
			}
			skill.Activation = ActivationSuppressed
			break
		}
	}

	for _, record := range records {
		if !matched[suppressionID(record.Marketplace, record.Plugin, record.SkillName)] {
			notices = append(notices, Notice{Message: fmt.Sprintf("Suppressed skill %s (%s@%s) no longer found — plugin may have been uninstalled or updated; suppression record kept in case it reappears", record.SkillName, record.Plugin, record.Marketplace)})
		}
	}

	return notices
}

// --- Suppression record storage (<DataDir>/suppressed/<id>.json) ---

func suppressionDir(dataDir string) string {
	return filepath.Join(dataDir, "suppressed")
}

// suppressionID is a deterministic, filesystem-safe id for a
// marketplace+plugin+skill triple, so a record's filename alone is enough to
// look it up or overwrite it — no index file needed.
func suppressionID(marketplace, plugin, skillName string) string {
	return sanitizeIDPart(marketplace) + "__" + sanitizeIDPart(plugin) + "__" + sanitizeIDPart(skillName)
}

func suppressionPath(dataDir, marketplace, plugin, skillName string) string {
	return filepath.Join(suppressionDir(dataDir), suppressionID(marketplace, plugin, skillName)+".json")
}

// loadSuppressionRecords reads every recorded suppression under dataDir. A
// missing directory means no suppressions exist yet — zero records, not an
// error (same graceful-degradation pattern as ListArchive).
func loadSuppressionRecords(dataDir string) ([]SuppressionRecord, error) {
	dir := suppressionDir(dataDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read suppressions: %w", err)
	}

	var records []SuppressionRecord
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read suppression %s: %w", entry.Name(), err)
		}
		var record SuppressionRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, fmt.Errorf("parse suppression %s: %w", entry.Name(), err)
		}
		records = append(records, record)
	}
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].SuppressedAt.Before(records[j].SuppressedAt)
	})
	return records, nil
}

func writeSuppressionRecord(dataDir string, record SuppressionRecord) error {
	dir := suppressionDir(dataDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create suppressions directory: %w", err)
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal suppression: %w", err)
	}
	path := suppressionPath(dataDir, record.Marketplace, record.Plugin, record.SkillName)
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write suppression: %w", err)
	}
	return nil
}

func removeSuppressionRecord(dataDir, marketplace, plugin, skillName string) error {
	path := suppressionPath(dataDir, marketplace, plugin, skillName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove suppression: %w", err)
	}
	return nil
}

// --- SKILL.md frontmatter editing ---
//
// These operate at the byte level, in the same spirit as
// codex_config.go's TOML block editing: locate exactly the span that needs
// to change and rewrite only that, so every other frontmatter field, the
// delimiters, and the body are preserved byte-for-byte.

type frontmatterField struct {
	Key   string
	Value string
}

func applySuppressionEdit(path string) error {
	return setFrontmatterFields(path, []frontmatterField{
		{Key: "disable-model-invocation", Value: "true"},
		{Key: "user-invocable", Value: "false"},
	})
}

func revertSuppressionEdit(path string) error {
	return removeFrontmatterFields(path, suppressionFieldKeys)
}

func isSuppressionApplied(fm skillFrontmatter) bool {
	return fm.DisableModelInvocation != nil && *fm.DisableModelInvocation &&
		fm.UserInvocable != nil && !*fm.UserInvocable
}

// splitFrontmatterSpan locates the YAML block within a SKILL.md's raw bytes
// (the "---\n<yaml>\n---\n<body>" shape parseFrontmatter also reads),
// returning the byte offsets of the YAML block itself, excluding both "---"
// delimiter lines, so callers can edit individual fields while leaving the
// delimiters, unrelated fields, and the body untouched.
func splitFrontmatterSpan(content string) (yamlStart, yamlEnd int, err error) {
	lines := splitLinesPreserveOffsets(content)
	if len(lines) == 0 || lineText(lines[0]) != "---" {
		return 0, 0, fmt.Errorf("missing frontmatter opening delimiter")
	}

	offset := len(lines[0])
	yamlStart = offset
	for i := 1; i < len(lines); i++ {
		if lineText(lines[i]) == "---" {
			return yamlStart, offset, nil
		}
		offset += len(lines[i])
	}
	return 0, 0, fmt.Errorf("missing frontmatter closing delimiter")
}

func lineText(line string) string {
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
}

// setFrontmatterFields rewrites path's YAML frontmatter block so each of
// fields is present with the given value: an existing "key: ..." line is
// replaced in place, or a new line is appended to the block if the key isn't
// present yet.
func setFrontmatterFields(path string, fields []frontmatterField) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read frontmatter: %w", err)
	}
	yamlStart, yamlEnd, err := splitFrontmatterSpan(string(content))
	if err != nil {
		return fmt.Errorf("read frontmatter: %w", err)
	}

	lines := splitLinesPreserveOffsets(string(content)[yamlStart:yamlEnd])
	for _, field := range fields {
		newLine := field.Key + ": " + field.Value + "\n"
		replaced := false
		for i, line := range lines {
			if strings.HasPrefix(lineText(line), field.Key+":") {
				lines[i] = newLine
				replaced = true
				break
			}
		}
		if !replaced {
			lines = append(lines, newLine)
		}
	}

	newContent := string(content)[:yamlStart] + strings.Join(lines, "") + string(content)[yamlEnd:]
	return writeFilePreservingMode(path, newContent)
}

// removeFrontmatterFields is the inverse of setFrontmatterFields: it deletes
// any "key: ..." lines whose key is in keys, leaving every other line
// untouched. A no-op (not an error) if none of the keys are present, so
// callers (Unsuppress) can call it unconditionally.
func removeFrontmatterFields(path string, keys []string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read frontmatter: %w", err)
	}
	yamlStart, yamlEnd, err := splitFrontmatterSpan(string(content))
	if err != nil {
		return fmt.Errorf("read frontmatter: %w", err)
	}

	lines := splitLinesPreserveOffsets(string(content)[yamlStart:yamlEnd])
	kept := lines[:0:0]
	for _, line := range lines {
		text := lineText(line)
		remove := false
		for _, key := range keys {
			if strings.HasPrefix(text, key+":") {
				remove = true
				break
			}
		}
		if !remove {
			kept = append(kept, line)
		}
	}

	newContent := string(content)[:yamlStart] + strings.Join(kept, "") + string(content)[yamlEnd:]
	return writeFilePreservingMode(path, newContent)
}

func writeFilePreservingMode(path, content string) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode()
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		return fmt.Errorf("write frontmatter: %w", err)
	}
	return nil
}
