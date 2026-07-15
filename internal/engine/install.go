package engine

// Install — resolve a Library entry's install-source descriptor and place a
// disconnected copy at a chosen target (CONTEXT.md Install; ADR 0004).
import (
	"encoding/json"
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
	if entry.Source.Kind == LibrarySourceMarketplace {
		return e.installMarketplace(entry, target)
	}

	dest, _, err := e.InstallDestination(entry, target)
	if err != nil {
		return err
	}

	switch entry.Source.Kind {
	case LibrarySourceLocalPath:
		if err := installLocalPath(entry.Source.LocalPath, dest); err != nil {
			return err
		}
	case LibrarySourceGit:
		if err := e.installGit(entry.Source, dest); err != nil {
			return err
		}
	case LibrarySourceSkillsSh:
		if err := e.installSkillsSh(entry, target); err != nil {
			return err
		}
	default:
		return fmt.Errorf("install: source kind %q is not supported yet", entry.Source.Kind)
	}

	if activation == ActivationManualOnly {
		if entry.Source.Kind == LibrarySourceSkillsSh && strings.TrimSpace(entry.Source.SkillsShSkill) == "" {
			return e.setSkillsShSourceManualOnly(entry, target)
		}
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
	if entry.Source.Kind == LibrarySourceMarketplace {
		if err := e.validateInstallTarget(target); err != nil {
			return "", false, err
		}
		return "", false, nil
	}
	installName := entry.Name
	if entry.Source.Kind == LibrarySourceSkillsSh {
		if skill := strings.TrimSpace(entry.Source.SkillsShSkill); skill != "" {
			installName = skill
		} else {
			if err := e.validateInstallTarget(target); err != nil {
				return "", false, err
			}
			return "", false, nil
		}
	}
	if strings.TrimSpace(installName) == "" {
		return "", false, fmt.Errorf("install: entry name is required")
	}
	if installName != filepath.Base(installName) || installName == "." || installName == ".." {
		return "", false, fmt.Errorf("install: entry name %q is not a single path segment", installName)
	}
	if entry.Kind != "" && entry.Kind != KindSkill {
		return "", false, fmt.Errorf("install: only skill entries are supported (got %q)", entry.Kind)
	}

	skillsDir, err := e.skillsDirForTarget(entry.Tool, target)
	if err != nil {
		return "", false, err
	}
	dest = filepath.Join(skillsDir, installName)
	if _, statErr := os.Lstat(dest); statErr == nil {
		return dest, true, nil
	} else if !os.IsNotExist(statErr) {
		return "", false, fmt.Errorf("install: inspect destination: %w", statErr)
	}
	return dest, false, nil
}

func (e *Engine) validateInstallTarget(target InstallTarget) error {
	switch target.Kind {
	case InstallTargetPersonal:
		return nil
	case InstallTargetProject:
		if strings.TrimSpace(target.RepoRoot) == "" {
			return fmt.Errorf("install: project target requires RepoRoot")
		}
		if !e.isResolvedProjectRoot(absolutePath(target.RepoRoot)) {
			return fmt.Errorf("install: %q is not a resolved project root", target.RepoRoot)
		}
		return nil
	default:
		return fmt.Errorf("install: unknown target kind %q", target.Kind)
	}
}

func (e *Engine) installGit(source LibrarySource, dest string) error {
	tmp, err := os.MkdirTemp("", "skillet-git-install-")
	if err != nil {
		return fmt.Errorf("install git: create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmp)

	cloneDir := filepath.Join(tmp, "repo")
	args := []string{"clone", "--depth", "1"}
	if strings.TrimSpace(source.GitRef) != "" {
		args = append(args, "--branch", source.GitRef)
	}
	args = append(args, source.GitURL, cloneDir)
	if err := e.runCommand(Command{Name: "git", Args: args}, "clone git source"); err != nil {
		return err
	}

	src := cloneDir
	if strings.TrimSpace(source.GitSubPath) != "" {
		clean := filepath.Clean(source.GitSubPath)
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("install git: subpath %q escapes repository", source.GitSubPath)
		}
		src = filepath.Join(cloneDir, clean)
	}
	if err := installLocalPath(src, dest); err != nil {
		return fmt.Errorf("install git: %w", err)
	}
	return nil
}

func (e *Engine) installSkillsSh(entry LibraryEntry, target InstallTarget) error {
	if err := e.validateInstallTarget(target); err != nil {
		return err
	}
	agent := ""
	switch entry.Tool {
	case ToolClaudeCode:
		agent = "claude-code"
	case ToolCodex:
		agent = "codex"
	default:
		return fmt.Errorf("install skills.sh: tool is required (Claude Code or Codex), got %q", entry.Tool)
	}
	args := []string{"skills", "add", entry.Source.SkillsShRepo, "-a", agent, "-y", "--copy"}
	skill := strings.TrimSpace(entry.Source.SkillsShSkill)
	if skill == "" {
		skill = "*"
	}
	args = append(args, "--skill", skill)
	command := Command{Name: "npx", Args: args}
	if target.Kind == InstallTargetPersonal {
		command.Args = append(command.Args, "-g")
	} else {
		command.Dir = absolutePath(target.RepoRoot)
	}
	return e.runCommand(command, "install skills.sh source")
}

func (e *Engine) setSkillsShSourceManualOnly(entry LibraryEntry, target InstallTarget) error {
	lockPath := filepath.Join(absolutePath(target.RepoRoot), "skills-lock.json")
	if target.Kind == InstallTargetPersonal {
		if xdg := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); xdg != "" {
			lockPath = filepath.Join(xdg, "skills", ".skill-lock.json")
		} else {
			lockPath = filepath.Join(e.roots.AgentsHome, ".skill-lock.json")
		}
	}
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return fmt.Errorf("install: apply Manual-only to skills.sh source: read lock: %w", err)
	}
	var lock struct {
		Skills map[string]struct {
			Source string `json:"source"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(data, &lock); err != nil {
		return fmt.Errorf("install: apply Manual-only to skills.sh source: parse lock: %w", err)
	}
	matched := 0
	for name, record := range lock.Skills {
		if record.Source != entry.Source.SkillsShRepo {
			continue
		}
		copyEntry := entry
		copyEntry.Name = name
		copyEntry.Source.SkillsShSkill = name
		dest, _, err := e.InstallDestination(copyEntry, target)
		if err != nil {
			return err
		}
		skill, err := e.skillAtInstallDestination(copyEntry, target, dest)
		if err != nil {
			return err
		}
		if err := e.SetManualOnly(skill, true); err != nil {
			return fmt.Errorf("install: apply Manual-only to %q: %w", name, err)
		}
		matched++
	}
	if matched == 0 {
		return fmt.Errorf("install: apply Manual-only: skills.sh lock has no entries for %q", entry.Source.SkillsShRepo)
	}
	return nil
}

func (e *Engine) installMarketplace(entry LibraryEntry, target InstallTarget) error {
	if err := e.validateInstallTarget(target); err != nil {
		return err
	}
	if !e.marketplaceKnown(entry.Source.Marketplace) {
		if strings.TrimSpace(entry.Source.MarketplaceSource) == "" {
			return fmt.Errorf("install marketplace: marketplace %q is unknown; provide a marketplace source or add it in Claude Code first", entry.Source.Marketplace)
		}
		if err := e.runClaudePluginCommand(Command{Name: "claude", Args: []string{"plugin", "marketplace", "add", entry.Source.MarketplaceSource, "--scope", "user"}}, "add marketplace"); err != nil {
			return err
		}
	}
	scope := "user"
	command := Command{Name: "claude"}
	if target.Kind == InstallTargetProject {
		scope = "project"
		command.Dir = absolutePath(target.RepoRoot)
	}
	command.Args = []string{"plugin", "install", entry.Source.PluginName + "@" + entry.Source.Marketplace, "--scope", scope}
	return e.runClaudePluginCommand(command, "install marketplace plugin")
}

func (e *Engine) runClaudePluginCommand(command Command, action string) error {
	result, err := e.runner.Run(command)
	if err != nil {
		return commandError(action, result, err)
	}
	// Claude Code has emitted plugin errors with exit status 0, so its success
	// marker is the completion boundary rather than the exit code alone.
	output := result.Stdout + "\n" + result.Stderr
	if !strings.Contains(output, "Successfully") {
		return fmt.Errorf("install: %s: Claude CLI did not report successful completion: %s", action, strings.TrimSpace(output))
	}
	return nil
}

func (e *Engine) marketplaceKnown(name string) bool {
	data, err := os.ReadFile(filepath.Join(e.roots.ClaudeHome, "plugins", "known_marketplaces.json"))
	if err != nil {
		return false
	}
	var known map[string]json.RawMessage
	return json.Unmarshal(data, &known) == nil && known[name] != nil
}

func (e *Engine) runCommand(command Command, action string) error {
	result, err := e.runner.Run(command)
	if err == nil {
		return nil
	}
	return commandError(action, result, err)
}

func commandError(action string, result CommandResult, err error) error {
	detail := strings.TrimSpace(result.Stderr)
	if detail == "" {
		detail = strings.TrimSpace(result.Stdout)
	}
	if detail != "" {
		return fmt.Errorf("install: %s: %w: %s", action, err, detail)
	}
	return fmt.Errorf("install: %s: %w", action, err)
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
		backup, err = os.MkdirTemp(parent, ".skillet-backup-")
		if err != nil {
			_ = os.RemoveAll(tmp)
			return fmt.Errorf("install: reserve backup path: %w", err)
		}
		if err := os.Remove(backup); err != nil {
			_ = os.RemoveAll(tmp)
			return fmt.Errorf("install: prepare backup path: %w", err)
		}
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
