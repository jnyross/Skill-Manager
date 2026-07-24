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
// "write", "move", "rename", or "remove"; path is the destination being
// operated on. A hook returning syscall.EXDEV for "rename" forces the
// cross-device copy path a real archive directory on another filesystem takes.
var writeFaultHook func(op, path string) error

func injectedFault(op, path string) error {
	if writeFaultHook == nil {
		return nil
	}
	return writeFaultHook(op, path)
}

// writeFileAtomic writes data to path via a temporary file in path's directory
// followed by a rename, giving the result perm. The temporary file is fsynced
// before the rename and the containing directory afterwards: the rename alone
// is atomic against a process crash, but on some filesystems a power loss can
// otherwise lose the whole write. These are small config files, so the cost of
// two fsyncs per write is proportionate to the durability they buy.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	if err := injectedFault("write", path); err != nil {
		return err
	}

	target, err := resolveWriteTarget(path)
	if err != nil {
		return err
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
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("sync temp file: %w", err)
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
	syncDir(dir)
	return nil
}

// syncDir flushes a directory entry so a completed rename survives power loss.
// A failure is deliberately not fatal: the rename has already happened and the
// new content is visible to every reader, so only durability — not
// correctness — is at stake, and some filesystems refuse to open a directory
// for sync at all.
func syncDir(dir string) {
	handle, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = handle.Sync()
	_ = handle.Close()
}

// resolveWriteTarget decides which file writeFileAtomic's rename should land
// on. Users commonly symlink ~/.claude/settings.json into a dotfiles repo, so
// a live symlink resolves to its target and the link survives the write.
//
// A *dangling* symlink is handled explicitly rather than silently: EvalSymlinks
// fails on one, and treating that failure as "write to path itself" would
// replace the user's symlink with a regular file, quietly detaching it from the
// dotfiles repo it pointed into. The link is followed by hand instead when its
// target's directory exists (the file is simply missing, so writing through
// creates it); when even that directory is gone there is no sane destination
// and the caller is told what to repair.
func resolveWriteTarget(path string) (string, error) {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved, nil
	}

	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		// Nothing at path (the ordinary new-file case), or a real file whose
		// resolution failed for some other reason: write to path itself, which
		// is exactly what the caller asked for.
		return path, nil
	}

	linkTarget, err := os.Readlink(path)
	if err != nil {
		return "", fmt.Errorf("read symlink %s: %w", path, err)
	}
	if !filepath.IsAbs(linkTarget) {
		linkTarget = filepath.Join(filepath.Dir(path), linkTarget)
	}
	parent := filepath.Dir(linkTarget)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", fmt.Errorf("write %s: it is a symlink to %s, but %s does not exist — repair or remove the symlink and try again", path, linkTarget, parent)
	}
	parentInfo, err := os.Stat(resolvedParent)
	if err != nil || !parentInfo.IsDir() {
		return "", fmt.Errorf("write %s: it is a symlink to %s, but %s is not a directory — repair or remove the symlink and try again", path, linkTarget, parent)
	}
	return filepath.Join(resolvedParent, filepath.Base(linkTarget)), nil
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

// renameChecked is os.Rename behind the fault-injection seam. A test hook
// returning syscall.EXDEV forces the cross-device copy-then-delete path that a
// real archive directory on another filesystem takes — the interleaving
// archive.go's cross-device handling exists to survive.
func renameChecked(oldPath, newPath string) error {
	if err := injectedFault("rename", newPath); err != nil {
		return err
	}
	return os.Rename(oldPath, newPath)
}
