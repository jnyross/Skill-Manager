package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

func (e *Engine) Uninstall(location string) (ArchiveEntry, error) {
	location = absolutePath(location)
	source, kind, tool, originRepo, err := e.classifyLocation(location)
	if err != nil {
		return ArchiveEntry{}, err
	}

	info, err := os.Lstat(location)
	if err != nil {
		return ArchiveEntry{}, fmt.Errorf("inspect skill location: %w", err)
	}

	base := filepath.Base(location)
	name := base
	if kind == KindPrompt {
		name = strings.TrimSuffix(base, filepath.Ext(base))
	}

	var newConfig string
	var removedConfig []RemovedConfigEntry
	var configChanged bool
	isCodexMechanismSkill := kind == KindSkill && (source == SourceCodex || (source == SourceProject && tool == ToolCodex))
	if isCodexMechanismSkill {
		skillName := name
		if fm, err := parseSkillFrontmatter(filepath.Join(location, "SKILL.md")); err == nil {
			skillName = fm.Name
		}
		newConfig, removedConfig, configChanged, err = planCodexConfigRemoval(e.roots.CodexHome, filepath.Join(location, "SKILL.md"), skillName)
		if err != nil {
			return ArchiveEntry{}, err
		}
	}

	archiveRoot := filepath.Join(e.roots.DataDir, "archive")
	id, err := e.newArchiveID(base)
	if err != nil {
		return ArchiveEntry{}, fmt.Errorf("generate archive id: %w", err)
	}
	archiveDir := filepath.Join(archiveRoot, id)
	if err := os.MkdirAll(archiveDir, 0o700); err != nil {
		return ArchiveEntry{}, fmt.Errorf("create archive directory: %w", err)
	}

	archivedPath := filepath.Join(archiveDir, base)
	entry := ArchiveEntry{
		ID:               id,
		Name:             name,
		Source:           source,
		Kind:             kind,
		Tool:             tool,
		OriginRepo:       originRepo,
		OriginalLocation: location,
		ArchivedAt:       time.Now().UTC(),
	}
	isSymlink := info.Mode()&os.ModeSymlink != 0
	if isSymlink {
		target, err := os.Readlink(location)
		if err != nil {
			_ = os.RemoveAll(archiveDir)
			return ArchiveEntry{}, fmt.Errorf("read symlink target: %w", err)
		}
		entry.IsSymlink = true
		entry.SymlinkTarget = target
	} else if kind == KindSkill && !info.IsDir() {
		_ = os.RemoveAll(archiveDir)
		return ArchiveEntry{}, fmt.Errorf("skill location is not a directory: %s", location)
	} else if kind == KindPrompt && info.IsDir() {
		_ = os.RemoveAll(archiveDir)
		return ArchiveEntry{}, fmt.Errorf("prompt location is not a file: %s", location)
	}
	if configChanged {
		entry.RemovedConfigEntries = removedConfig
	}

	// Provenance is written *before* the skill leaves its source directory.
	// ListArchive skips entries with no provenance.json, so writing it after
	// the move would leave a window where a failure (or a crash) makes the
	// skill absent from the tool *and* invisible in the archive — recoverable
	// only by hand. The reverse window this ordering creates is benign and
	// self-describing: an archive entry whose payload has not arrived yet
	// means the skill is still installed exactly where it was, and Restore
	// reports that state explicitly instead of wedging.
	if err := writeArchiveEntry(archiveDir, entry); err != nil {
		_ = os.RemoveAll(archiveDir)
		return ArchiveEntry{}, err
	}

	switch {
	case isSymlink:
		if err := moveSymlink(location, archivedPath, entry.SymlinkTarget); err != nil {
			_ = os.RemoveAll(archiveDir)
			return ArchiveEntry{}, fmt.Errorf("archive symlink: %w", err)
		}
	case kind == KindSkill:
		if err := moveDirectory(location, archivedPath); err != nil {
			_ = os.RemoveAll(archiveDir)
			return ArchiveEntry{}, fmt.Errorf("archive skill directory: %w", err)
		}
	default:
		if err := moveFile(location, archivedPath, info.Mode().Perm()); err != nil {
			_ = os.RemoveAll(archiveDir)
			return ArchiveEntry{}, fmt.Errorf("archive prompt file: %w", err)
		}
	}

	if configChanged {
		if err := writeCodexConfig(e.roots.CodexHome, newConfig); err != nil {
			// Undo the move so the skill is back where the user left it. If
			// even that fails, the archive entry (payload plus provenance) is
			// deliberately kept: the skill is then recoverable with Restore
			// rather than lost, which is the invariant this ordering exists
			// to protect.
			if rollbackErr := rollbackArchiveMove(entry, archivedPath, info.Mode().Perm()); rollbackErr != nil {
				return ArchiveEntry{}, fmt.Errorf("%w (rollback also failed: %v; the skill is preserved in the archive as %s — restore it from the Archive view)", err, rollbackErr, id)
			}
			_ = os.RemoveAll(archiveDir)
			return ArchiveEntry{}, err
		}
	}
	return entry, nil
}

// rollbackArchiveMove moves an already-archived payload back to the location
// it came from, undoing the move step of Uninstall when a later step fails.
func rollbackArchiveMove(entry ArchiveEntry, archivedPath string, perm os.FileMode) error {
	switch {
	case entry.IsSymlink:
		return moveSymlink(archivedPath, entry.OriginalLocation, entry.SymlinkTarget)
	case entry.Kind == KindSkill:
		return moveDirectory(archivedPath, entry.OriginalLocation)
	default:
		return moveFile(archivedPath, entry.OriginalLocation, perm)
	}
}

func (e *Engine) ListArchive() ([]ArchiveEntry, []Notice, error) {
	archiveRoot := filepath.Join(e.roots.DataDir, "archive")
	entries, err := os.ReadDir(archiveRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("read archive: %w", err)
	}

	var archived []ArchiveEntry
	var notices []Notice
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		archiveEntry, err := readArchiveEntry(filepath.Join(archiveRoot, entry.Name()))
		if err != nil {
			if os.IsNotExist(err) {
				notices = append(notices, Notice{Message: fmt.Sprintf("Archive entry missing provenance.json, skipping: %s", entry.Name())})
			} else {
				notices = append(notices, Notice{Message: fmt.Sprintf("Archive entry malformed, skipping: %s: %v", entry.Name(), err)})
			}
			continue
		}
		archiveEntry.ID = entry.Name()
		if !e.archiveEntryVisible(archiveEntry) {
			continue
		}
		archived = append(archived, archiveEntry)
	}

	sort.SliceStable(archived, func(i, j int) bool {
		return archived[i].ArchivedAt.After(archived[j].ArchivedAt)
	})
	return archived, notices, nil
}

func (e *Engine) archiveEntryVisible(entry ArchiveEntry) bool {
	if entry.Source != SourceProject {
		return true
	}
	return e.hasProjectRoot(entry.OriginRepo)
}

func (e *Engine) hasProjectRoot(originRepo string) bool {
	return containsPath(e.roots.ClaudeProjectRoots, originRepo) || containsPath(e.roots.ProjectRoots, originRepo)
}

func containsPath(paths []string, target string) bool {
	for _, path := range paths {
		if path == target {
			return true
		}
	}
	return false
}

func (e *Engine) Restore(id string) error {
	if err := validateArchiveID(id); err != nil {
		return err
	}

	archiveRoot := filepath.Join(e.roots.DataDir, "archive")
	archiveDir := filepath.Join(archiveRoot, id)
	entry, err := readArchiveEntry(archiveDir)
	if err != nil {
		return err
	}
	entry.ID = id

	archivedPath := filepath.Join(archiveDir, filepath.Base(entry.OriginalLocation))
	payloadPresent, err := pathExists(archivedPath)
	if err != nil {
		return fmt.Errorf("inspect archived copy: %w", err)
	}
	destPresent, err := pathExists(entry.OriginalLocation)
	if err != nil {
		return fmt.Errorf("inspect restore destination: %w", err)
	}

	// Restore is resumable. A previous attempt that moved the payload back but
	// then failed (most commonly in reinstateCodexConfigEntries) leaves the
	// destination populated and the archived copy gone; re-running must finish
	// that work rather than reporting "restore destination already exists"
	// forever. The four states are distinguished structurally, so recovery
	// does not depend on a marker that could itself have failed to be written.
	switch {
	case payloadPresent && destPresent:
		return fmt.Errorf("restore destination already exists: %s — the archived copy is still safe at %s; move or delete the existing skill and restore again, or purge archive entry %s", entry.OriginalLocation, archivedPath, id)
	case !payloadPresent && !destPresent:
		return fmt.Errorf("archive entry %s has no stored copy at %s and nothing at its original location %s — the archive record is incomplete; purge it from the Archive view", id, archivedPath, entry.OriginalLocation)
	case payloadPresent && !destPresent:
		if err := os.MkdirAll(filepath.Dir(entry.OriginalLocation), 0o700); err != nil {
			return fmt.Errorf("create restore parent: %w", err)
		}
		if entry.IsSymlink {
			if err := os.Symlink(entry.SymlinkTarget, entry.OriginalLocation); err != nil {
				return fmt.Errorf("restore symlink: %w", err)
			}
			if err := os.Remove(archivedPath); err != nil {
				return fmt.Errorf("remove archived symlink: %w", err)
			}
		} else {
			if err := moveArchivedPayload(archivedPath, entry.OriginalLocation); err != nil {
				return fmt.Errorf("restore skill directory: %w", err)
			}
		}
	default:
		// !payloadPresent && destPresent: a previous Restore already put the
		// skill back; only the config reinstatement and cleanup remain. Both
		// remaining steps are idempotent, so simply falling through completes
		// the restore.
	}

	if len(entry.RemovedConfigEntries) > 0 {
		if err := reinstateCodexConfigEntries(e.roots.CodexHome, entry.RemovedConfigEntries); err != nil {
			return fmt.Errorf("%w — the skill itself is restored at %s; fix the Codex config problem and run Restore on %s again to finish reinstating its config.toml entries", err, entry.OriginalLocation, id)
		}
	}

	if err := os.RemoveAll(archiveDir); err != nil {
		return fmt.Errorf("remove archive entry: %w", err)
	}
	removeEmptyArchiveRoot(archiveRoot)
	return nil
}

func (e *Engine) Purge(id string) error {
	if err := validateArchiveID(id); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(e.roots.DataDir, "archive", id))
}

// classifyLocation determines the Source, Kind, Tool, and Project origin of an
// archivable location by checking which scan root it is an immediate child of.
// Archive is supported for Personal skills, Codex skills (either scan root),
// Codex custom prompts, and Project skills (either Tool); anything else
// (Plugin skills, unknown paths) is rejected.
func (e *Engine) classifyLocation(location string) (Source, Kind, Tool, string, error) {
	if _, ok := immediateChildOf(filepath.Join(e.roots.ClaudeHome, "skills"), location); ok {
		return SourcePersonal, KindSkill, ToolClaudeCode, "", nil
	}
	if _, ok := immediateChildOf(filepath.Join(e.roots.CodexHome, "skills"), location); ok {
		return SourceCodex, KindSkill, ToolCodex, "", nil
	}
	if _, ok := immediateChildOf(filepath.Join(e.roots.AgentsHome, "skills"), location); ok {
		return SourceCodex, KindSkill, ToolCodex, "", nil
	}
	if _, ok := immediateChildOf(filepath.Join(e.roots.CodexHome, "prompts"), location); ok {
		return SourceCodex, KindPrompt, ToolCodex, "", nil
	}
	for _, root := range e.roots.ClaudeProjectRoots {
		if _, ok := immediateChildOf(filepath.Join(root, ".claude", "skills"), location); ok {
			return SourceProject, KindSkill, ToolClaudeCode, root, nil
		}
	}
	for _, root := range e.roots.ProjectRoots {
		if _, ok := immediateChildOf(filepath.Join(root, ".agents", "skills"), location); ok {
			return SourceProject, KindSkill, ToolCodex, root, nil
		}
	}
	return "", "", "", "", fmt.Errorf("archive is not supported for this location: %s", location)
}

// immediateChildOf reports whether location is exactly one path segment
// below root, returning that segment.
func immediateChildOf(root, location string) (string, bool) {
	root = absolutePath(root)
	location = absolutePath(location)
	rel, err := filepath.Rel(root, location)
	if err != nil {
		return "", false
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	if strings.Contains(rel, string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

func (e *Engine) newArchiveID(folderName string) (string, error) {
	safeName := sanitizeIDPart(folderName)
	for {
		id := fmt.Sprintf("%d-%s", time.Now().UnixNano(), safeName)
		_, err := os.Stat(filepath.Join(e.roots.DataDir, "archive", id))
		if err != nil {
			if os.IsNotExist(err) {
				return id, nil
			}
			return "", fmt.Errorf("check archive id: %w", err)
		}
	}
}

func sanitizeIDPart(value string) string {
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteByte('-')
		}
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "skill"
	}
	return result
}

// pathExists reports whether path exists, without following a symlink at the
// final component. A non-IsNotExist error is returned rather than swallowed so
// callers can distinguish "absent" from "unreadable".
func pathExists(path string) (bool, error) {
	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// moveArchivedPayload moves an archived skill directory or prompt file back to
// dest, tolerating a cross-device archive directory the way moveDirectory and
// moveFile already do on the way in.
func moveArchivedPayload(src, dest string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return moveDirectory(src, dest)
	}
	return moveFile(src, dest, info.Mode().Perm())
}

func moveSymlink(oldPath, newPath, target string) error {
	if err := injectedFault("move", newPath); err != nil {
		return err
	}
	if err := os.Rename(oldPath, newPath); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}

	if err := os.Symlink(target, newPath); err != nil {
		return err
	}
	if err := os.Remove(oldPath); err != nil {
		_ = os.Remove(newPath)
		return err
	}
	return nil
}

func moveDirectory(oldPath, newPath string) error {
	if err := injectedFault("move", newPath); err != nil {
		return err
	}
	if err := os.Rename(oldPath, newPath); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}

	if err := copyDir(oldPath, newPath); err != nil {
		return err
	}
	return os.RemoveAll(oldPath)
}

func moveFile(oldPath, newPath string, perm os.FileMode) error {
	if err := injectedFault("move", newPath); err != nil {
		return err
	}
	if err := os.Rename(oldPath, newPath); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}

	if err := copyFile(oldPath, newPath, perm); err != nil {
		return err
	}
	return os.Remove(oldPath)
}

func writeArchiveEntry(archiveDir string, entry ArchiveEntry) error {
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal provenance: %w", err)
	}
	if err := writeFileAtomic(filepath.Join(archiveDir, "provenance.json"), append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write provenance: %w", err)
	}
	return nil
}

func readArchiveEntry(archiveDir string) (ArchiveEntry, error) {
	data, err := os.ReadFile(filepath.Join(archiveDir, "provenance.json"))
	if err != nil {
		return ArchiveEntry{}, fmt.Errorf("read provenance: %w", err)
	}
	var entry ArchiveEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return ArchiveEntry{}, fmt.Errorf("parse provenance: %w", err)
	}
	return entry, nil
}

func validateArchiveID(id string) error {
	if id == "" || id == "." || id == ".." || filepath.Base(id) != id {
		return fmt.Errorf("invalid archive id: %q", id)
	}
	return nil
}

func removeEmptyArchiveRoot(archiveRoot string) {
	_ = os.Remove(archiveRoot)
}
