package engine

// Atomic file writing — the shared write primitive for every file Skillet
// edits but does not own (Codex's config.toml, Claude Code's settings.json and
// installed_plugins.json, a skill's SKILL.md frontmatter, a Codex skill's
// agents/openai.yaml) plus Skillet's own records. Skillet's brand promise is
// "reversible and safe"; a partially-written config file is neither, so every
// write goes through writeFileAtomic: content is written to a temporary file
// in the destination's own directory, given the destination's mode, and then
// renamed over the destination. A crash or an error leaves either the whole
// old file or the whole new file, never a truncated one.
//
// Symlinked destinations are resolved first (users commonly symlink
// ~/.claude/settings.json into a dotfiles repo), so the rename replaces the
// real file rather than replacing the user's symlink with a regular file.
// Paths that must not be followed at all are refused earlier, by the callers'
// own guards (guardSkillFilePath in suppress.go).

import (
	"fmt"
	"os"
	"path/filepath"
)

// writeFaultHook is a test-only seam for fault injection. It is nil in
// production; crash-simulation tests (see write_fault_test.go) set it to force
// a specific step of a multi-step mutation to fail, so the rollback and
// recovery paths can be exercised without an actual crash. op is one of
// "write", "move", or "remove"; path is the destination being operated on.
var writeFaultHook func(op, path string) error

func injectedFault(op, path string) error {
	if writeFaultHook == nil {
		return nil
	}
	return writeFaultHook(op, path)
}

// writeFileAtomic writes data to path via a temporary file in path's directory
// followed by a rename, giving the result perm.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	if err := injectedFault("write", path); err != nil {
		return err
	}

	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	}

	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

// writeFilePreservingMode atomically writes content to path, keeping path's
// existing file mode if it already exists (falling back to 0o644 for a new
// file). Shared by suppress.go's frontmatter edits, manual_only.go's
// agents/openai.yaml edits, and the JSON config writers in install.go and
// plugin_uninstall.go.
func writeFilePreservingMode(path, content string) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := writeFileAtomic(path, []byte(content), mode); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

// removeAllChecked is os.RemoveAll behind the fault-injection seam, so
// crash-simulation tests can force a mid-uninstall cache-deletion failure.
func removeAllChecked(path string) error {
	if err := injectedFault("remove", path); err != nil {
		return err
	}
	return os.RemoveAll(path)
}
