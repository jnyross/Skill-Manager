package engine

// Install — resolve a Library entry's install-source descriptor and place a
// disconnected copy at a chosen target (CONTEXT.md Install; ADR 0004).
// This ticket implements LibrarySourceLocalPath only; other kinds return a
// clear "not yet supported" error so later source tickets share the same
// entry point and target rules.

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// InstallLibraryEntry places entry at target, applying activation afterward
// when Manual-only. The engine is non-interactive: callers (TUI) must already
// have confirmed overwrite when the destination name collides.
func (e *Engine) InstallLibraryEntry(entry LibraryEntry, target InstallTarget, activation ActivationState) error {
	dest, _, err := e.InstallDestination(entry, target)
	if err != nil {
		return err
	}

	switch entry.Source.Kind {
	case LibrarySourceLocalPath:
		if err := installLocalPath(entry.Source.LocalPath, dest); err != nil {
			return err
		}
	default:
		return fmt.Errorf("install: source kind %q is not supported yet", entry.Source.Kind)
	}

	if activation == ActivationManualOnly {
		skill, err := e.skillAtInstallDestination(entry, target, dest)
		if err != nil {
			return err
		}
		if err := e.SetManualOnly(skill, true); err != nil {
			return fmt.Errorf("install: apply Manual-only: %w", err)
		}
	}
	return nil
}

// InstallDestination returns the absolute path where entry would be placed for
// target, and whether that path already exists. Used by the TUI for
// confirm-and-overwrite before calling InstallLibraryEntry.
func (e *Engine) InstallDestination(entry LibraryEntry, target InstallTarget) (dest string, exists bool, err error) {
	if strings.TrimSpace(entry.Name) == "" {
		return "", false, fmt.Errorf("install: entry name is required")
	}
	if entry.Name != filepath.Base(entry.Name) || entry.Name == "." || entry.Name == ".." {
		return "", false, fmt.Errorf("install: entry name %q is not a single path segment", entry.Name)
	}
	if entry.Kind != "" && entry.Kind != KindSkill {
		return "", false, fmt.Errorf("install: only skill entries are supported (got %q)", entry.Kind)
	}

	skillsDir, err := e.skillsDirForTarget(entry.Tool, target)
	if err != nil {
		return "", false, err
	}
	dest = filepath.Join(skillsDir, entry.Name)
	if _, statErr := os.Lstat(dest); statErr == nil {
		return dest, true, nil
	} else if !os.IsNotExist(statErr) {
		return "", false, fmt.Errorf("install: inspect destination: %w", statErr)
	}
	return dest, false, nil
}

// ResolvedProjectRoots returns the deduplicated union of Codex and Claude
// project roots the engine was constructed with — the only allowed Install
// Project targets (no free-text paths).
func (e *Engine) ResolvedProjectRoots() []string {
	combined := make([]string, 0, len(e.roots.ProjectRoots)+len(e.roots.ClaudeProjectRoots))
	combined = append(combined, e.roots.ProjectRoots...)
	combined = append(combined, e.roots.ClaudeProjectRoots...)
	return dedupePaths(combined)
}

func (e *Engine) skillsDirForTarget(tool Tool, target InstallTarget) (string, error) {
	switch target.Kind {
	case InstallTargetPersonal:
		switch tool {
		case ToolClaudeCode:
			return filepath.Join(e.roots.ClaudeHome, "skills"), nil
		case ToolCodex:
			// Official USER Codex skills path (skill-mechanisms.md).
			return filepath.Join(e.roots.AgentsHome, "skills"), nil
		default:
			return "", fmt.Errorf("install: tool is required (Claude Code or Codex), got %q", tool)
		}
	case InstallTargetProject:
		if strings.TrimSpace(target.RepoRoot) == "" {
			return "", fmt.Errorf("install: project target requires RepoRoot")
		}
		root := absolutePath(target.RepoRoot)
		if !e.isResolvedProjectRoot(root) {
			return "", fmt.Errorf("install: %q is not a resolved project root", target.RepoRoot)
		}
		switch tool {
		case ToolClaudeCode:
			return filepath.Join(root, ".claude", "skills"), nil
		case ToolCodex:
			return filepath.Join(root, ".agents", "skills"), nil
		default:
			return "", fmt.Errorf("install: tool is required (Claude Code or Codex), got %q", tool)
		}
	default:
		return "", fmt.Errorf("install: unknown target kind %q", target.Kind)
	}
}

func (e *Engine) isResolvedProjectRoot(root string) bool {
	for _, allowed := range e.ResolvedProjectRoots() {
		if samePath(allowed, root) {
			return true
		}
	}
	return false
}

func (e *Engine) skillAtInstallDestination(entry LibraryEntry, target InstallTarget, dest string) (Skill, error) {
	source, tool, err := sourceAndToolForInstall(entry.Tool, target)
	if err != nil {
		return Skill{}, err
	}
	return Skill{
		Name:     entry.Name,
		Source:   source,
		Tool:     tool,
		Kind:     KindSkill,
		Location: absolutePath(dest),
	}, nil
}

func sourceAndToolForInstall(tool Tool, target InstallTarget) (Source, Tool, error) {
	switch target.Kind {
	case InstallTargetPersonal:
		switch tool {
		case ToolClaudeCode:
			return SourcePersonal, ToolClaudeCode, nil
		case ToolCodex:
			return SourceCodex, ToolCodex, nil
		default:
			return "", "", fmt.Errorf("install: tool is required (Claude Code or Codex), got %q", tool)
		}
	case InstallTargetProject:
		switch tool {
		case ToolClaudeCode, ToolCodex:
			return SourceProject, tool, nil
		default:
			return "", "", fmt.Errorf("install: tool is required (Claude Code or Codex), got %q", tool)
		}
	default:
		return "", "", fmt.Errorf("install: unknown target kind %q", target.Kind)
	}
}

func installLocalPath(src, dest string) error {
	src = absolutePath(src)
	// Resolve directory symlinks so Library local-paths that point at a skill
	// folder (common for Personal skills) copy the real tree, not the link.
	resolved, err := filepath.EvalSymlinks(src)
	if err != nil {
		return fmt.Errorf("install: source path: %w", err)
	}
	src = resolved
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("install: source path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("install: local-path source must be a directory: %s", src)
	}
	if samePath(src, dest) {
		return fmt.Errorf("install: source and destination are the same path: %s", src)
	}

	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("install: create skills directory: %w", err)
	}

	// Stage into a sibling path (not MkdirTemp — that would own the root mode),
	// then swap into place so a failed copy never leaves dest half-removed.
	tmp := filepath.Join(parent, fmt.Sprintf(".skillet-install-%d", time.Now().UnixNano()))
	if err := copyDir(src, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("install: copy skill: %w", err)
	}

	backup := ""
	if _, err := os.Lstat(dest); err == nil {
		backup = dest + ".skillet-old"
		_ = os.RemoveAll(backup)
		if err := os.Rename(dest, backup); err != nil {
			_ = os.RemoveAll(tmp)
			return fmt.Errorf("install: move existing destination aside: %w", err)
		}
	}

	if err := os.Rename(tmp, dest); err != nil {
		_ = os.RemoveAll(tmp)
		if backup != "" {
			_ = os.Rename(backup, dest)
		}
		return fmt.Errorf("install: place skill at destination: %w", err)
	}
	if backup != "" {
		_ = os.RemoveAll(backup)
	}
	return nil
}

func copyDir(src, dest string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)

		info, err := d.Info()
		if err != nil {
			return err
		}

		if d.Type()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			return os.Symlink(linkTarget, target)
		}

		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}

		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported file type at %s", path)
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(src, dest string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
