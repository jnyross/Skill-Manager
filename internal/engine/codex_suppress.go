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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// suppressCodex writes a minimal `[[skills.config]]` block disabling skill
// in <codexHome>/config.toml, creating the file if it doesn't exist yet. A
// no-op if a matching entry (by path or name, per readCodexDisabledConfig)
// is already present and already disabled — calling Suppress twice does not
// write a second, duplicate block.
//
// Every block it writes is recorded as Skillet-authored (ownership.go), along
// with whether config.toml had to be created, so unsuppressCodex can remove
// exactly what Skillet added and nothing else.
func (e *Engine) suppressCodex(skill Skill) error {
	codexHome := e.roots.CodexHome
	skillMDPath := absolutePath(filepath.Join(skill.Location, "SKILL.md"))

	disabled, _ := readCodexDisabledConfig(codexHome)
	if disabled.matches(skillMDPath, skill.Name) {
		return e.repairCodexConfigRecord(skill, skillMDPath)
	}

	content, err := readCodexConfigOrEmpty(codexHome)
	if err != nil {
		return fmt.Errorf("suppress skill: %w", err)
	}
	configExisted := true
	if _, statErr := os.Stat(filepath.Join(codexHome, "config.toml")); statErr != nil && os.IsNotExist(statErr) {
		configExisted = false
	}

	block := codexSuppressBlock(skillMDPath)
	newContent := content
	if newContent != "" && !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	newContent += block

	// The ownership record is written *before* config.toml, and this ordering is
	// the difference between a repairable failure and a permanent one. Writing
	// the live disable block first left a window where the block existed with no
	// record: a retry hit the "already disabled" early return above and no-op'd,
	// so the record was never repaired, and a later Unsuppress took the
	// no-record path — which is allowed to remove hand-written blocks too. In
	// this order the last step is the only one with an externally visible
	// effect, and every failure state a retry can reach is self-correcting.
	record := codexConfigRecord{
		SkillName:     skill.Name,
		SkillMDPath:   skillMDPath,
		Block:         block,
		CreatedConfig: !configExisted,
		SuppressedAt:  time.Now().UTC(),
	}
	if err := saveCodexConfigRecord(e.roots.DataDir, record); err != nil {
		return fmt.Errorf("suppress skill: record config ownership: %w", err)
	}
	if err := writeCodexConfig(codexHome, newContent); err != nil {
		// The record now claims a block config.toml does not contain. Dropping
		// it keeps the two in step; if even that fails the leftover record is
		// still harmless, because unsuppressCodex's owned path removes only a
		// block matching it byte for byte and simply deletes the stale record
		// when it finds none.
		_ = deleteCodexConfigRecord(e.roots.DataDir, skill.Name, skillMDPath)
		return fmt.Errorf("suppress skill: %w", err)
	}
	return nil
}

// codexSuppressBlock is the exact `[[skills.config]]` text Skillet writes to
// disable one skill. It is a single function so the block Suppress appends and
// the block repairCodexConfigRecord recognises can never drift apart.
func codexSuppressBlock(skillMDPath string) string {
	return "[[skills.config]]\npath = " + strconv.Quote(skillMDPath) + "\nenabled = false\n"
}

// repairCodexConfigRecord is the retry half of the ordering above. A skill can
// legitimately already be disabled with no ownership record: config.toml was
// written by a Skillet old enough to have written the block before the record,
// or the record file was lost. When the live block is byte-identical to the one
// Skillet writes — the same predicate that later authorizes its removal — the
// missing record is written rather than left absent, so a retry of Suppress
// repairs the state instead of no-op'ing forever.
//
// CreatedConfig is deliberately false: Skillet cannot know it created a
// config.toml it is only now adopting a block in, and the conservative answer
// (never delete the user's file) is the safe one. Anything that is not a
// byte-identical Skillet block is left unclaimed, since it may carry hand-tuned
// keys whose author alone should decide their fate.
func (e *Engine) repairCodexConfigRecord(skill Skill, skillMDPath string) error {
	_, found, err := loadCodexConfigRecord(e.roots.DataDir, skill.Name, skillMDPath)
	if err != nil {
		return fmt.Errorf("suppress skill: %w", err)
	}
	if found {
		return nil
	}
	_, matched, _, err := planCodexConfigOwnedRemoval(e.roots.CodexHome, skillMDPath, skill.Name, codexSuppressBlock(skillMDPath))
	if err != nil {
		return fmt.Errorf("suppress skill: %w", err)
	}
	if !matched {
		return nil
	}
	record := codexConfigRecord{
		SkillName:    skill.Name,
		SkillMDPath:  skillMDPath,
		Block:        codexSuppressBlock(skillMDPath),
		SuppressedAt: time.Now().UTC(),
	}
	if err := saveCodexConfigRecord(e.roots.DataDir, record); err != nil {
		return fmt.Errorf("suppress skill: record config ownership: %w", err)
	}
	return nil
}

// unsuppressCodex removes the `[[skills.config]]` block(s) disabling skill
// from <codexHome>/config.toml, reusing codex_config.go's span-finding — the
// same machinery Archive's stale-entry cleanup uses — rather than hand-rolling
// new text surgery. A no-op if config.toml doesn't exist or has no matching
// entry.
//
// Ownership (ownership.go) decides both what is removed and whether the file
// survives:
//
//   - With a Skillet record for this skill, only the exact block Skillet wrote
//     is removed. Any other entry naming the same skill was authored by a
//     human, by Codex, or by a plugin — possibly with hand-tuned extra keys —
//     and is left untouched; the returned error says so, because the skill is
//     then still disabled in Codex and only its author can decide what to do
//     with it.
//   - With no record (an entry predating ownership tracking, or one written
//     outside Skillet), matching entries are still removed: an explicit
//     un-suppress has to be able to re-enable the skill, and nothing else in
//     Skillet can.
//   - config.toml is deleted only when the record says Skillet created it and
//     removing Skillet's block leaves nothing behind. That keeps a
//     Suppress-then-Unsuppress round trip tree-identical when Suppress created
//     the file from nothing, while a config.toml the user already had is left
//     in place — empty, if that is all that remains — because Skillet does not
//     delete files it did not create.
func (e *Engine) unsuppressCodex(skill Skill) error {
	codexHome := e.roots.CodexHome
	skillMDPath := absolutePath(filepath.Join(skill.Location, "SKILL.md"))

	record, owned, err := loadCodexConfigRecord(e.roots.DataDir, skill.Name, skillMDPath)
	if err != nil {
		return fmt.Errorf("unsuppress skill: %w", err)
	}

	var newContent string
	var changed bool
	remaining := 0
	if owned {
		// Precise path: remove exactly the block Skillet appended, leaving any
		// other entry for this skill (which a human or Codex wrote, possibly
		// with extra hand-tuned keys) in place.
		newContent, changed, remaining, err = planCodexConfigOwnedRemoval(codexHome, skillMDPath, skill.Name, record.Block)
	} else {
		// No record: either the entry predates Skillet's ownership tracking or
		// it was written outside Skillet. Removing it is the only way to honour
		// an explicit un-suppress, so it is still removed — but config.toml is
		// never deleted below, because Skillet did not create it.
		newContent, _, changed, err = planCodexConfigRemoval(codexHome, skillMDPath, skill.Name)
	}
	if err != nil {
		return fmt.Errorf("unsuppress skill: %w", err)
	}
	if !changed {
		// Nothing matching this skill is left in config.toml; drop any stale
		// ownership record so it can't authorize a future deletion.
		if owned {
			return deleteCodexConfigRecord(e.roots.DataDir, skill.Name, skillMDPath)
		}
		return nil
	}

	// config.toml is deleted only when Skillet created it. A file the user
	// already had is left in place even if removing Skillet's block empties
	// it: Skillet does not delete files it did not create.
	if strings.TrimSpace(newContent) == "" && owned && record.CreatedConfig {
		if err := removeCodexConfigFile(codexHome); err != nil {
			return fmt.Errorf("unsuppress skill: %w", err)
		}
		return deleteCodexConfigRecord(e.roots.DataDir, skill.Name, skillMDPath)
	}
	if err := writeCodexConfig(codexHome, newContent); err != nil {
		return fmt.Errorf("unsuppress skill: %w", err)
	}
	if owned {
		if err := deleteCodexConfigRecord(e.roots.DataDir, skill.Name, skillMDPath); err != nil {
			return fmt.Errorf("unsuppress skill: %w", err)
		}
	}
	if remaining > 0 {
		return fmt.Errorf("unsuppress skill: removed Skillet's disable entry, but %s still has %d hand-written [[skills.config]] entr(y/ies) for %q — edit that file to re-enable the skill in Codex", filepath.Join(codexHome, "config.toml"), remaining, skill.Name)
	}
	return nil
}
