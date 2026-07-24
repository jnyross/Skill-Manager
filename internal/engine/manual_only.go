package engine

// Manual-only toggle for Personal and Codex skills (CONTEXT.md: "Manual-only"
// — never "disabled" — is the state of a skill whose Auto-activation is off).
// One public method covers both hosts, branching on Skill.Source/Kind, so the
// TUI has a single action to offer rather than choosing between two
// separately-named methods — mirroring Suppress/Unsuppress's shape (skill.go
// suppress.go): callers pass a Skill from a recent Inventory() call, never a
// raw path string.
//
// Claude Code (Personal skills): edits the skill's own SKILL.md frontmatter
// field `disable-model-invocation` (docs/research/skill-mechanisms.md,
// "Making a skill manual-only") — research confirmed there is no
// non-invasive alternative. Reuses suppress.go's byte-level frontmatter
// helpers (setFrontmatterFields / removeFrontmatterFields) directly rather
// than reimplementing frontmatter editing, so every other frontmatter field
// and the body stay byte-identical.
//
// Codex skills: edits a separate policy metadata file,
// <skill-folder>/agents/openai.yaml, field `policy.allow_implicit_invocation`
// (docs/research/skill-mechanisms.md, "Manual-only for Codex" — this is the
// write-side counterpart of codex.go's codexOpenAIActivation), creating the
// file (and its agents/ directory) if absent.
//
// Not offered on Codex custom prompts (Kind == KindPrompt; already strictly
// user-invoked with no toggle mechanism — skill-mechanisms.md) or Plugin
// skills (Suppress covers those instead, see suppress.go); SetManualOnly
// rejects both defensively, matching how Suppress/Unsuppress reject
// non-Plugin skills — the TUI is responsible for not *offering* the action
// there, but the engine never silently no-ops.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	codexPolicyKey        = "policy"
	codexInterfaceKey     = "interface"
	codexAllowImplicitKey = "allow_implicit_invocation"
)

// SetManualOnly turns skill's Auto-activation off (manualOnly = true), so it
// only runs on explicit invocation, or back on (manualOnly = false).
func (e *Engine) SetManualOnly(skill Skill, manualOnly bool) error {
	return withEngineLock(e.roots.DataDir, func() error { return e.setManualOnlyLocked(skill, manualOnly) })
}

func (e *Engine) setManualOnlyLocked(skill Skill, manualOnly bool) error {
	action := "set manual-only"
	if !manualOnly {
		action = "unset manual-only"
	}
	switch {
	case skill.Source == SourcePersonal && skill.Kind == KindSkill,
		skill.Source == SourceProject && skill.Tool == ToolClaudeCode && skill.Kind == KindSkill:
		if err := revalidateSkillLocation(skill, action); err != nil {
			return err
		}
		return setPersonalManualOnly(skill, manualOnly)
	case skill.Source == SourceCodex && skill.Kind == KindSkill,
		skill.Source == SourceProject && skill.Tool == ToolCodex && skill.Kind == KindSkill:
		if err := revalidateSkillLocation(skill, action); err != nil {
			return err
		}
		return e.setCodexManualOnlyForSkill(skill, manualOnly)
	default:
		return fmt.Errorf("manual-only toggle: not supported for %s %s %q", skill.Source, skill.Kind, skill.Name)
	}
}

func setPersonalManualOnly(skill Skill, manualOnly bool) error {
	path := filepath.Join(skill.Location, "SKILL.md")
	if err := guardSkillMDPath(skill.Location, path); err != nil {
		if manualOnly {
			return fmt.Errorf("set manual-only: %w", err)
		}
		return fmt.Errorf("unset manual-only: %w", err)
	}
	if manualOnly {
		if err := setFrontmatterFields(path, []frontmatterField{{Key: "disable-model-invocation", Value: "true"}}); err != nil {
			return fmt.Errorf("set manual-only: %w", err)
		}
		return nil
	}
	if err := removeFrontmatterFields(path, []string{"disable-model-invocation"}); err != nil {
		return fmt.Errorf("unset manual-only: %w", err)
	}
	return nil
}

func (e *Engine) setCodexManualOnlyForSkill(skill Skill, manualOnly bool) error {
	path := filepath.Join(skill.Location, "agents", "openai.yaml")
	// The Codex policy file gets the same symlink protection SKILL.md edits
	// have always had (guardSkillFilePath, suppress.go): a symlinked
	// agents/openai.yaml — or a symlinked agents/ directory, when the file
	// does not exist yet — must not be followed out of the skill tree.
	if err := guardSkillFilePath(skill.Location, path); err != nil {
		if manualOnly {
			return fmt.Errorf("set manual-only: %w", err)
		}
		return fmt.Errorf("unset manual-only: %w", err)
	}
	if manualOnly {
		if err := e.setCodexManualOnly(skill, path); err != nil {
			return fmt.Errorf("set manual-only: %w", err)
		}
		return nil
	}
	if err := e.unsetCodexManualOnly(skill, path); err != nil {
		return fmt.Errorf("unset manual-only: %w", err)
	}
	return nil
}

// --- Codex agents/openai.yaml editing ---
//
// Design decisions (docs/research/skill-mechanisms.md's "Manual-only for
// Codex" section documents two on-disk shapes for the policy block: a flat
// top-level `policy:` mapping, or `policy:` nested one level under
// `interface:` — the same two shapes codexOpenAIActivation in codex.go
// already reads):
//
//   - Setting Manual-only edits whichever shape is already present in the
//     file (preserving it exactly), or, if neither is present, appends a new
//     flat top-level `policy:` block — this is also what happens when the
//     file doesn't exist yet (it's created from scratch with just that
//     block).
//   - Setting Manual-only never invents an `interface:` wrapper; that shape
//     is only preserved, never introduced.
//   - Unsetting Manual-only removes the `allow_implicit_invocation` line
//     (matching the read side's treatment of a missing key as
//     ActivationAuto, and mirroring how Unsuppress removes
//     disable-model-invocation rather than writing it back to `false`) and
//     then collapses any parent block left with no remaining children — an
//     emptied `policy:` line is removed, and if that was nested under
//     `interface:`, an now-empty `interface:` line is removed too. If
//     nothing is left in the file at all, the file itself is deleted — but
//     only when Skillet created it, which is recorded at set time (see
//     ownership.go); a policy file the user already had is left in place,
//     empty if that is all that remains, because Skillet does not delete
//     files it did not create. This
//     is what makes the round trip exact for the two cases the acceptance
//     criteria calls out: a file Skillet created from scratch disappears
//     again; a file that already had unrelated content (or other keys
//     alongside the policy block) keeps exactly that content, byte for
//     byte. If the field already had an explicit value before Skillet ever
//     touched it (rather than being absent), that explicit value is not
//     restored — it's removed like any other Skillet-added field, the same
//     tolerance Unsuppress already has for reverting an edit unconditionally
//     rather than tracking prior state.
//
// Narrower than the read side: only block-mapping YAML (`policy:` /
// `interface:` each on their own line, children indented under them) is
// recognized; a flow-style or scalar `policy: ...` value is not, and would
// result in a second `policy:` key being appended rather than edited. This
// matches every shape verified locally in skill-mechanisms.md.

func (e *Engine) setCodexManualOnly(skill Skill, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read policy file: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create policy directory: %w", err)
		}
		if err := writeFilePreservingMode(path, "policy:\n  allow_implicit_invocation: false\n"); err != nil {
			return err
		}
		// Record that this file is Skillet's own creation, so unsetting
		// Manual-only later may delete it again — and so a policy file the
		// user already had never can be.
		return saveCodexPolicyRecord(e.roots.DataDir, codexPolicyRecord{
			SkillName:   skill.Name,
			PolicyPath:  absolutePath(path),
			CreatedFile: true,
			AppliedAt:   time.Now().UTC(),
		})
	}

	newContent, err := setYAMLBoolField(string(content), codexAllowImplicitKey, false)
	if err != nil {
		return err
	}
	return writeFilePreservingMode(path, newContent)
}

func (e *Engine) unsetCodexManualOnly(skill Skill, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read policy file: %w", err)
	}

	newContent, changed, err := removeYAMLKeyCollapsingEmptyParents(string(content), codexAllowImplicitKey)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	if strings.TrimSpace(newContent) == "" {
		// Same ownership rule as Codex config.toml (codex_suppress.go): an
		// emptied policy file is deleted only when Skillet created it. One the
		// user already had is left in place, empty, rather than deleted.
		record, owned, recErr := loadCodexPolicyRecord(e.roots.DataDir, skill.Name, absolutePath(path))
		if recErr != nil {
			return recErr
		}
		if owned && record.CreatedFile {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove empty policy file: %w", err)
			}
			removeEmptyAgentsDir(filepath.Dir(path))
			return deleteCodexPolicyRecord(e.roots.DataDir, skill.Name, absolutePath(path))
		}
		return writeFilePreservingMode(path, newContent)
	}
	if err := writeFilePreservingMode(path, newContent); err != nil {
		return err
	}
	return deleteCodexPolicyRecord(e.roots.DataDir, skill.Name, absolutePath(path))
}

// removeEmptyAgentsDir removes the agents/ directory Skillet created for a
// policy file it has just deleted. os.Remove only succeeds on an empty
// directory, so a user's own agents/ content is never touched.
func removeEmptyAgentsDir(dir string) {
	if filepath.Base(dir) == "agents" {
		_ = os.Remove(dir)
	}
}

// findYAMLPolicyBlock locates the line opening the `policy:` mapping within
// lines, handling the two documented shapes: flat top-level `policy:`, or
// `policy:` as a direct child of a top-level `interface:` block (not an
// arbitrarily-nested descendant — only the shapes skill-mechanisms.md
// documents and codexOpenAIActivation's read side actually looks at). found
// is false if neither shape is present.
func findYAMLPolicyBlock(lines []string) (index, indent int, found bool) {
	if i, ok := findTopLevelKeyLine(lines, codexPolicyKey); ok {
		return i, 0, true
	}
	if i, ok := findTopLevelKeyLine(lines, codexInterfaceKey); ok {
		blockEnd := blockEndIndex(lines, i, 0)
		childIndent := -1
		for j := i + 1; j < blockEnd; j++ {
			if strings.TrimSpace(lineText(lines[j])) == "" {
				continue
			}
			if childIndent == -1 {
				childIndent = lineIndent(lines[j])
			}
			if lineIndent(lines[j]) == childIndent && strings.TrimSpace(lineText(lines[j])) == codexPolicyKey+":" {
				return j, childIndent, true
			}
		}
	}
	return 0, 0, false
}

func findTopLevelKeyLine(lines []string, key string) (int, bool) {
	for i, line := range lines {
		if lineIndent(line) == 0 && strings.TrimSpace(lineText(line)) == key+":" {
			return i, true
		}
	}
	return 0, false
}

// topLevelKeyHasInlineValue reports whether lines has a top-level line for
// key whose value is inline (e.g. flow-style `policy: {}` or a scalar
// `policy: null`) rather than the block-mapping form (`policy:` alone, with
// children on following indented lines). setYAMLBoolField and
// removeYAMLKeyCollapsingEmptyParents only understand the block form; an
// inline value at the same key would otherwise go undetected by
// findYAMLPolicyBlock and — for setYAMLBoolField's "append a new block"
// fallback — result in a second, colliding top-level `policy:` key being
// written, corrupting the file. Callers use this to refuse the edit instead.
func topLevelKeyHasInlineValue(lines []string, key string) bool {
	for _, line := range lines {
		if lineIndent(line) != 0 {
			continue
		}
		text := strings.TrimSpace(lineText(line))
		if text == key+":" {
			continue
		}
		if strings.HasPrefix(text, key+":") {
			return true
		}
	}
	return false
}

func lineIndent(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

// blockEndIndex returns the index one past the last line belonging to the
// block opened at lines[start] (children indented deeper than parentIndent),
// stopping at the first line at or above parentIndent, or EOF. Blank lines
// are treated as belonging to the block rather than as a dedent, so a block
// removal (see removeYAMLKeyCollapsingEmptyParents) also cleans up any blank
// separator lines left within it.
func blockEndIndex(lines []string, start, parentIndent int) int {
	i := start + 1
	for i < len(lines) {
		if strings.TrimSpace(lineText(lines[i])) == "" {
			i++
			continue
		}
		if lineIndent(lines[i]) <= parentIndent {
			break
		}
		i++
	}
	return i
}

// setYAMLBoolField sets key to value within the policy block located by
// findYAMLPolicyBlock, replacing an existing "key: ..." line in place or
// inserting one as the block's first child line (at the indentation of the
// block's existing children, if any). If no policy block is present at all,
// a new flat top-level `policy:` block is appended to the end of the file —
// unless an existing `policy:` or `interface:` key is present in the
// unsupported inline form (see topLevelKeyHasInlineValue), in which case an
// error is returned rather than risking a colliding second `policy:` key.
func setYAMLBoolField(content, key string, value bool) (string, error) {
	lines := splitLinesPreserveOffsets(content)
	valueText := "false"
	if value {
		valueText = "true"
	}

	polIndex, polIndent, found := findYAMLPolicyBlock(lines)
	if !found {
		if topLevelKeyHasInlineValue(lines, codexPolicyKey) || topLevelKeyHasInlineValue(lines, codexInterfaceKey) {
			return "", fmt.Errorf("policy file has %q/%q as an inline value rather than a block mapping; refusing to edit", codexPolicyKey, codexInterfaceKey)
		}
		if n := len(lines); n > 0 && !strings.HasSuffix(lines[n-1], "\n") {
			lines[n-1] += "\n"
		}
		lines = append(lines, codexPolicyKey+":\n", "  "+key+": "+valueText+"\n")
		return strings.Join(lines, ""), nil
	}

	blockEnd := blockEndIndex(lines, polIndex, polIndent)
	childIndent := polIndent + 2
	for j := polIndex + 1; j < blockEnd; j++ {
		if strings.TrimSpace(lineText(lines[j])) == "" {
			continue
		}
		childIndent = lineIndent(lines[j])
		break
	}

	for j := polIndex + 1; j < blockEnd; j++ {
		if strings.HasPrefix(strings.TrimSpace(lineText(lines[j])), key+":") {
			lines[j] = strings.Repeat(" ", lineIndent(lines[j])) + key + ": " + valueText + "\n"
			return strings.Join(lines, ""), nil
		}
	}

	newLine := strings.Repeat(" ", childIndent) + key + ": " + valueText + "\n"
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:polIndex+1]...)
	out = append(out, newLine)
	out = append(out, lines[polIndex+1:]...)
	return strings.Join(out, ""), nil
}

// removeYAMLKeyCollapsingEmptyParents deletes key's line from within the
// policy block located by findYAMLPolicyBlock, then removes the `policy:`
// line itself if that leaves the block with no remaining (non-blank)
// children, and — if policy was nested under `interface:` — removes the
// interface: line too if it is now childless. changed reports whether key
// was found and removed at all; a no-op (key not present, or no policy block
// at all) returns changed = false so callers can treat unsetting a field
// that was never set as a safe no-op, not an error — except when a `policy:`
// or `interface:` key is present but in the unsupported inline form (see
// topLevelKeyHasInlineValue), where silently reporting success would lie
// about having reverted anything, so an error is returned instead.
func removeYAMLKeyCollapsingEmptyParents(content, key string) (string, bool, error) {
	lines := splitLinesPreserveOffsets(content)
	polIndex, polIndent, found := findYAMLPolicyBlock(lines)
	if !found {
		if topLevelKeyHasInlineValue(lines, codexPolicyKey) || topLevelKeyHasInlineValue(lines, codexInterfaceKey) {
			return content, false, fmt.Errorf("policy file has %q/%q as an inline value rather than a block mapping; refusing to edit", codexPolicyKey, codexInterfaceKey)
		}
		return content, false, nil
	}

	blockEnd := blockEndIndex(lines, polIndex, polIndent)
	keyLine := -1
	for j := polIndex + 1; j < blockEnd; j++ {
		if strings.HasPrefix(strings.TrimSpace(lineText(lines[j])), key+":") {
			keyLine = j
			break
		}
	}
	if keyLine == -1 {
		return content, false, nil
	}

	lines = append(lines[:keyLine], lines[keyLine+1:]...)
	blockEnd--

	if !blockHasNonBlankLine(lines, polIndex+1, blockEnd) {
		lines = append(lines[:polIndex], lines[blockEnd:]...)

		if polIndent > 0 {
			if ifaceIndex, ok := findTopLevelKeyLine(lines, codexInterfaceKey); ok {
				ifaceEnd := blockEndIndex(lines, ifaceIndex, 0)
				if !blockHasNonBlankLine(lines, ifaceIndex+1, ifaceEnd) {
					lines = append(lines[:ifaceIndex], lines[ifaceEnd:]...)
				}
			}
		}
	}

	return strings.Join(lines, ""), true, nil
}

func blockHasNonBlankLine(lines []string, start, end int) bool {
	for j := start; j < end; j++ {
		if strings.TrimSpace(lineText(lines[j])) != "" {
			return true
		}
	}
	return false
}
