package engine

// Suppress for Codex skills (CONTEXT.md's Suppress, applied here via
// Codex's own native per-skill disable rather than the Skillet-owned
// mechanism Plugin Suppress needs — see suppress.go). Per
// docs/research/skill-mechanisms.md ("Settings-level disable exists"),
// Codex already has this built in: a `[[skills.config]]` block in
// <CodexHome>/config.toml with `enabled = false`. Unlike Plugin skills,
// Codex's config.toml is one stable, unversioned file — there is no
// plugin-cache-directory-replacement problem to work around, so unlike
// suppress.go there is no self-healing loop and no separate
// Skillet-owned record: writing the native entry *is* the suppression, and
// codex.go's readCodexDisabledConfig (already written for issue #4) already
// reads it back on every scan. That's also why this reuses ActivationDisabled
// rather than ActivationSuppressed — see the doc comment on those constants
// in types.go.
//
// Keying: a new entry is always written keyed by `path` (the SKILL.md's
// absolute path) — the documented key, per skill-mechanisms.md — never by
// `name`. The `name` form is only observed locally on one namespaced plugin
// skill (`render:render-debug`); its exact semantics (e.g. how a plugin
// namespace prefix is derived) are inferred, not documented, so this code
// does not depend on them for anything it writes. Un-suppress, however, must
// still be able to *remove* a pre-existing name-keyed entry for a skill (one
// Codex, a plugin, or a human wrote directly) — the same either-key matching
// codex.go's readCodexDisabledConfig already does when deciding a skill's
// Activation — so it reuses codex_config.go's matchingSkillsConfigBlockSpans/
// planCodexConfigRemoval, which already match by path OR name, unchanged.
//
// Byte-level editing: like codex_config.go's Archive-lifecycle functions,
// Suppress and Unsuppress edit config.toml's raw text directly rather than
// re-serializing it through the TOML encoder, so every other table,
// formatting choice, and comment in the file survives untouched. Suppress
// always appends its new block at the very end of the file with no blank
// line inserted before it (only a trailing newline is added first if the
// file doesn't already end in one) — not for cosmetic reasons, but so
// Unsuppress's generic block-span removal (which only ever deletes the exact
// bytes of a matched `[[skills.config]]` block, never a separator) restores
// the file to byte-identical original content on the way back out. Inserting
// a blank-line separator would survive removal as a stray blank line and
// break that round trip.
import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// suppressCodex writes a minimal `[[skills.config]]` block disabling skill
// in <codexHome>/config.toml, creating the file if it doesn't exist yet. A
// no-op if a matching entry (by path or name, per readCodexDisabledConfig)
// is already present and already disabled — calling Suppress twice does not
// write a second, duplicate block.
func suppressCodex(codexHome string, skill Skill) error {
	skillMDPath := absolutePath(filepath.Join(skill.Location, "SKILL.md"))

	disabled, _ := readCodexDisabledConfig(codexHome)
	if disabled.matches(skillMDPath, skill.Name) {
		return nil
	}

	content, err := readCodexConfigOrEmpty(codexHome)
	if err != nil {
		return fmt.Errorf("suppress skill: %w", err)
	}

	block := "[[skills.config]]\npath = " + strconv.Quote(skillMDPath) + "\nenabled = false\n"
	newContent := content
	if newContent != "" && !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	newContent += block

	if err := writeCodexConfig(codexHome, newContent); err != nil {
		return fmt.Errorf("suppress skill: %w", err)
	}
	return nil
}

// unsuppressCodex removes every `[[skills.config]]` block matching skill (by
// path or name) from <codexHome>/config.toml, reusing
// codex_config.go's planCodexConfigRemoval — the same span-finding Archive's
// stale-entry cleanup already uses — rather than hand-rolling new text
// surgery. A no-op if config.toml doesn't exist or has no matching entry. If
// removing the entry leaves the file with nothing else in it, the file is
// deleted outright rather than left as an empty file — unconditionally, the
// same delete-on-empty rule manual_only.go's Codex agents/openai.yaml editing
// already uses (it doesn't track whether Skillet itself created the file
// either). This code has no way to tell "config.toml existed before Suppress
// but happened to contain only this one entry" apart from "Suppress created
// it from nothing" — both end up empty after removal, so both are deleted.
// The consequence worth calling out: it's the created-from-nothing case that
// makes Suppress-then-Unsuppress round-trip exact (a tree snapshot would see
// a leftover empty file as a change, not "no change"); a config.toml a human
// authored with only one entry, once unsuppressed, is also removed rather
// than left empty — a deliberate simplification, not a bug.
func unsuppressCodex(codexHome string, skill Skill) error {
	skillMDPath := absolutePath(filepath.Join(skill.Location, "SKILL.md"))

	newContent, _, changed, err := planCodexConfigRemoval(codexHome, skillMDPath, skill.Name)
	if err != nil {
		return fmt.Errorf("unsuppress skill: %w", err)
	}
	if !changed {
		return nil
	}

	if strings.TrimSpace(newContent) == "" {
		if err := removeCodexConfigFile(codexHome); err != nil {
			return fmt.Errorf("unsuppress skill: %w", err)
		}
		return nil
	}
	if err := writeCodexConfig(codexHome, newContent); err != nil {
		return fmt.Errorf("unsuppress skill: %w", err)
	}
	return nil
}
