package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// This file holds the byte-level editing of Codex's config.toml used by the
// Archive lifecycle (internal/engine/archive.go) to remove and later
// reinstate stale `[[skills.config]]` entries for an archived Codex skill.
// See docs/research/skill-mechanisms.md for the on-disk config.toml shape.

// planCodexConfigRemoval finds any `[[skills.config]]` blocks in
// <codexHome>/config.toml that reference the given skill (by its SKILL.md
// path or its frontmatter name) and computes the config content with those
// blocks removed, without writing anything. changed is false when
// config.toml doesn't exist or has no matching entries, in which case
// newContent and removed are unset.
//
// removed[i].Offset is recorded in "skeleton" coordinates (see buildSkeleton)
// rather than a raw byte offset into newContent, so it stays valid even if
// other Codex skills' config entries are archived or restored in between —
// see reinstateCodexConfigEntries.
func planCodexConfigRemoval(codexHome, skillMDPath, skillName string) (newContent string, removed []RemovedConfigEntry, changed bool, err error) {
	configPath := filepath.Join(codexHome, "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, false, nil
		}
		return "", nil, false, fmt.Errorf("read codex config: %w", err)
	}
	content := string(data)
	skillMDPath = absolutePath(skillMDPath)

	matches := matchingSkillsConfigBlockSpans(content, skillMDPath, skillName)
	if len(matches) == 0 {
		return "", nil, false, nil
	}

	for _, span := range matches {
		removed = append(removed, RemovedConfigEntry{
			Offset: skeletonOffsetForBlockStart(content, span.start),
			Raw:    content[span.start:span.end],
		})
	}

	var kept strings.Builder
	prev := 0
	for _, span := range matches {
		kept.WriteString(content[prev:span.start])
		prev = span.end
	}
	kept.WriteString(content[prev:])
	return kept.String(), removed, true, nil
}

// planCodexConfigOwnedRemoval is planCodexConfigRemoval narrowed to the blocks
// Skillet itself wrote: only a block whose bytes match ownedBlock exactly (the
// text recorded at Suppress time, see ownership.go) is removed. remaining
// counts blocks that reference the same skill but were authored elsewhere — a
// human, Codex, or a plugin — and are therefore left untouched, so hand-tuned
// keys inside them are never silently discarded.
func planCodexConfigOwnedRemoval(codexHome, skillMDPath, skillName, ownedBlock string) (newContent string, changed bool, remaining int, err error) {
	configPath := filepath.Join(codexHome, "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, 0, nil
		}
		return "", false, 0, fmt.Errorf("read codex config: %w", err)
	}
	content := string(data)

	want := strings.TrimRight(ownedBlock, "\n")
	var owned []byteSpan
	for _, span := range matchingSkillsConfigBlockSpans(content, absolutePath(skillMDPath), skillName) {
		if strings.TrimRight(content[span.start:span.end], "\n") == want {
			owned = append(owned, span)
			continue
		}
		remaining++
	}
	if len(owned) == 0 {
		return "", false, remaining, nil
	}

	var kept strings.Builder
	prev := 0
	for _, span := range owned {
		kept.WriteString(content[prev:span.start])
		prev = span.end
	}
	kept.WriteString(content[prev:])
	return kept.String(), true, remaining, nil
}

// writeCodexConfig replaces <codexHome>/config.toml atomically (temp file plus
// rename, see atomic.go), keeping the file's existing mode. config.toml is a
// file Skillet edits but does not own, so a half-written config — which Codex
// would refuse to parse — must not be a reachable state.
func writeCodexConfig(codexHome, content string) error {
	configPath := filepath.Join(codexHome, "config.toml")
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		return fmt.Errorf("write codex config: %w", err)
	}
	if err := writeFilePreservingMode(configPath, content); err != nil {
		return fmt.Errorf("write codex config: %w", err)
	}
	return nil
}

// readCodexConfigOrEmpty reads <codexHome>/config.toml's raw content,
// returning "" (not an error) when the file doesn't exist yet — the signal
// codex_suppress.go's Suppress needs to know it's starting from nothing
// rather than failing to read an existing file.
func readCodexConfigOrEmpty(codexHome string) (string, error) {
	data, err := os.ReadFile(filepath.Join(codexHome, "config.toml"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read codex config: %w", err)
	}
	return string(data), nil
}

// removeCodexConfigFile deletes <codexHome>/config.toml outright. A no-op,
// not an error, if it's already gone. Used by codex_suppress.go's Unsuppress
// when removing the last remaining `[[skills.config]]` block leaves nothing
// else in the file — see that file for why deleting (rather than leaving a
// zero-byte file) is what makes a Suppress-then-Unsuppress round trip exact
// when Suppress created config.toml from nothing.
func removeCodexConfigFile(codexHome string) error {
	if err := os.Remove(filepath.Join(codexHome, "config.toml")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove codex config: %w", err)
	}
	return nil
}

// reinstateCodexConfigEntries splices previously removed blocks back into
// <codexHome>/config.toml, one at a time, converting each entry's skeleton
// offset into the correct position in the config file as it currently
// stands. A missing config.toml is treated as an empty file, so restoring a
// Codex skill works after a prior Unsuppress deleted the config. Recomputing
// against the live file on every insertion (rather than against the state at
// removal time) is what makes this safe when another Codex skill's config
// entries were archived or restored in between: the skeleton is unaffected by
// any of Skillet's own block insertions/removals, so it remains a stable
// coordinate space regardless of interleaving.
func reinstateCodexConfigEntries(codexHome string, entries []RemovedConfigEntry) error {
	content, err := readCodexConfigOrEmpty(codexHome)
	if err != nil {
		return err
	}

	changed := false
	for _, e := range entries {
		// Skip a block that is already present verbatim. Restore is resumable
		// (see archive.go), so this function can legitimately run twice for
		// the same entry after a partial restore; without this check the
		// second run would duplicate every block it had already reinstated.
		if strings.Contains(content, strings.TrimRight(e.Raw, "\n")) {
			continue
		}
		pos, err := filePositionForSkeletonOffset(content, e.Offset)
		if err != nil {
			return err
		}
		content = content[:pos] + e.Raw + content[pos:]
		changed = true
	}
	if !changed {
		return nil
	}

	return writeCodexConfig(codexHome, content)
}

type byteSpan struct {
	start, end int
}

// keptSegment is a run of content preserved by buildSkeleton, annotated with
// where it starts in both the original file and the resulting skeleton.
type keptSegment struct {
	fileStart, fileEnd int
	skelStart          int
}

// skillsConfigBlockSpans scans config.toml's raw text for every
// `[[skills.config]]` array-table block (per
// docs/research/skill-mechanisms.md, the observed shape is
// `path`/`name`/`enabled` keys), regardless of which skill each references,
// and returns their byte spans. Header detection tolerates extra whitespace
// inside the brackets and inline `#` comments on the header line. A block
// runs from its header line through its last non-blank line before the next
// line that opens a table, or EOF. Trailing blank lines are deliberately
// excluded from the span (left as skeleton/kept content, see buildSkeleton)
// rather than absorbed into whichever block happens to precede them:
// attributing a separator blank line to one block would give two adjacent
// blocks the same skeleton offset, making their relative order unrecoverable
// when both are archived and later restored out of order.
func skillsConfigBlockSpans(content string) []byteSpan {
	lines := splitLinesPreserveOffsets(content)

	var spans []byteSpan
	offset := 0
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if !isSkillsConfigHeader(line) {
			offset += len(line)
			continue
		}

		start := offset
		j := i + 1
		for j < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[j]), "[") {
			j++
		}

		lastContentLine := j - 1
		for lastContentLine > i && strings.TrimSpace(lines[lastContentLine]) == "" {
			lastContentLine--
		}
		end := start
		for k := i; k <= lastContentLine; k++ {
			end += len(lines[k])
		}
		spans = append(spans, byteSpan{start, end})

		for k := i; k < j; k++ {
			offset += len(lines[k])
		}
		i = j - 1
	}
	return spans
}

// matchingSkillsConfigBlockSpans filters skillsConfigBlockSpans to the
// blocks whose decoded path or name identifies the given skill. Byte spans
// (not a structural TOML edit) are used deliberately so the rest of the
// file — formatting, comments, unrelated tables — is preserved untouched and
// Restore can splice the exact original bytes back.
func matchingSkillsConfigBlockSpans(content, skillMDPath, skillName string) []byteSpan {
	var matches []byteSpan
	for _, span := range skillsConfigBlockSpans(content) {
		var block codexConfig
		if _, err := toml.Decode(content[span.start:span.end], &block); err != nil || len(block.Skills.Config) != 1 {
			continue
		}
		config := block.Skills.Config[0]
		matchesPath := config.Path != "" && absolutePath(config.Path) == skillMDPath
		matchesName := config.Name != "" && config.Name == skillName
		if matchesPath || matchesName {
			matches = append(matches, span)
		}
	}
	return matches
}

// buildSkeleton strips every `[[skills.config]]` block out of content,
// returning the remaining text (the "skeleton") plus the segments of
// content that were kept, each annotated with its offset in the skeleton.
// Skillet only ever adds or removes whole skills.config blocks, so this
// skeleton is invariant across any sequence of archive/restore operations —
// it gives RemovedConfigEntry.Offset a coordinate space that stays valid no
// matter which other skills' entries are currently archived.
func buildSkeleton(content string) (skeleton string, segments []keptSegment) {
	blocks := skillsConfigBlockSpans(content)
	var b strings.Builder
	prev := 0
	for _, blk := range blocks {
		if blk.start > prev {
			segments = append(segments, keptSegment{fileStart: prev, fileEnd: blk.start, skelStart: b.Len()})
			b.WriteString(content[prev:blk.start])
		}
		prev = blk.end
	}
	if prev < len(content) {
		segments = append(segments, keptSegment{fileStart: prev, fileEnd: len(content), skelStart: b.Len()})
		b.WriteString(content[prev:])
	}
	return b.String(), segments
}

// skeletonOffsetForBlockStart converts blockStart — the position of a
// skills.config block about to be removed from content — into the
// corresponding offset in content's skeleton.
func skeletonOffsetForBlockStart(content string, blockStart int) int {
	_, segments := buildSkeleton(content)
	for _, seg := range segments {
		if blockStart == seg.fileEnd {
			return seg.skelStart + (seg.fileEnd - seg.fileStart)
		}
	}
	return 0
}

// filePositionForSkeletonOffset is the inverse of
// skeletonOffsetForBlockStart: given content as it currently stands (which
// may have a different set of skills.config blocks present than when the
// offset was recorded), it finds the file position that skeleton offset now
// corresponds to.
func filePositionForSkeletonOffset(content string, skeletonOffset int) (int, error) {
	_, segments := buildSkeleton(content)
	if len(segments) == 0 {
		if skeletonOffset != 0 {
			return 0, fmt.Errorf("invalid removed config entry offset %d", skeletonOffset)
		}
		return 0, nil
	}
	for _, seg := range segments {
		segLen := seg.fileEnd - seg.fileStart
		if skeletonOffset >= seg.skelStart && skeletonOffset <= seg.skelStart+segLen {
			return seg.fileStart + (skeletonOffset - seg.skelStart), nil
		}
	}
	return 0, fmt.Errorf("invalid removed config entry offset %d", skeletonOffset)
}

// splitLinesPreserveOffsets splits content into lines, each including its
// trailing "\n" (the last line omits it if content doesn't end in one), so
// that concatenating the result reproduces content exactly and byte offsets
// can be tracked by summing line lengths.
func splitLinesPreserveOffsets(content string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			lines = append(lines, content[start:i+1])
			start = i + 1
		}
	}
	if start < len(content) {
		lines = append(lines, content[start:])
	}
	return lines
}

// isSkillsConfigHeader reports whether line is a `[[skills.config]]` array-table
// header, tolerating hand-edited whitespace inside the brackets and trailing
// `#` comments.
func isSkillsConfigHeader(line string) bool {
	line = stripInlineComment(line)
	return strings.Join(strings.Fields(line), "") == "[[skills.config]]"
}

// stripInlineComment returns the portion of line before the first unquoted `#`
// character. It is intentionally conservative (used only for header detection)
// and does not treat `\#` as escaped outside of quoted strings.
func stripInlineComment(line string) string {
	var inSingle, inDouble bool
	for i, r := range line {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return line[:i]
			}
		}
	}
	return line
}
