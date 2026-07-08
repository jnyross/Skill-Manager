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
	source, kind, tool, err := e.classifyLocation(location)
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
	id := e.newArchiveID(base)
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
		OriginalLocation: location,
		ArchivedAt:       time.Now().UTC(),
	}

	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(location)
		if err != nil {
			return ArchiveEntry{}, fmt.Errorf("read symlink target: %w", err)
		}
		if err := moveSymlink(location, archivedPath, target); err != nil {
			return ArchiveEntry{}, fmt.Errorf("archive symlink: %w", err)
		}
		entry.IsSymlink = true
		entry.SymlinkTarget = target
	} else if kind == KindSkill {
		if !info.IsDir() {
			return ArchiveEntry{}, fmt.Errorf("skill location is not a directory: %s", location)
		}
		if err := os.Rename(location, archivedPath); err != nil {
			return ArchiveEntry{}, fmt.Errorf("archive skill directory: %w", err)
		}
	} else {
		if info.IsDir() {
			return ArchiveEntry{}, fmt.Errorf("prompt location is not a file: %s", location)
		}
		if err := os.Rename(location, archivedPath); err != nil {
			return ArchiveEntry{}, fmt.Errorf("archive prompt file: %w", err)
		}
	}

	if configChanged {
		if err := writeCodexConfig(e.roots.CodexHome, newConfig); err != nil {
			return ArchiveEntry{}, err
		}
		entry.RemovedConfigEntries = removedConfig
	}

	if err := writeArchiveEntry(archiveDir, entry); err != nil {
		return ArchiveEntry{}, err
	}
	return entry, nil
}

func (e *Engine) ListArchive() ([]ArchiveEntry, error) {
	archiveRoot := filepath.Join(e.roots.DataDir, "archive")
	entries, err := os.ReadDir(archiveRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read archive: %w", err)
	}

	var archived []ArchiveEntry
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		archiveEntry, err := readArchiveEntry(filepath.Join(archiveRoot, entry.Name()))
		if err != nil {
			return nil, err
		}
		archiveEntry.ID = entry.Name()
		archived = append(archived, archiveEntry)
	}

	sort.SliceStable(archived, func(i, j int) bool {
		return archived[i].ArchivedAt.After(archived[j].ArchivedAt)
	})
	return archived, nil
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

	if _, err := os.Lstat(entry.OriginalLocation); err == nil {
		return fmt.Errorf("restore destination already exists: %s", entry.OriginalLocation)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect restore destination: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(entry.OriginalLocation), 0o700); err != nil {
		return fmt.Errorf("create restore parent: %w", err)
	}

	archivedPath := filepath.Join(archiveDir, filepath.Base(entry.OriginalLocation))
	if entry.IsSymlink {
		if err := os.Symlink(entry.SymlinkTarget, entry.OriginalLocation); err != nil {
			return fmt.Errorf("restore symlink: %w", err)
		}
		if err := os.Remove(archivedPath); err != nil {
			return fmt.Errorf("remove archived symlink: %w", err)
		}
	} else {
		if err := os.Rename(archivedPath, entry.OriginalLocation); err != nil {
			return fmt.Errorf("restore skill directory: %w", err)
		}
	}

	if len(entry.RemovedConfigEntries) > 0 {
		if err := reinstateCodexConfigEntries(e.roots.CodexHome, entry.RemovedConfigEntries); err != nil {
			return err
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

// classifyLocation determines the Source, Kind, and Tool of an archivable
// location by checking which scan root it is an immediate child of. Archive
// is supported for Personal skills, Codex skills (either scan root), Codex
// custom prompts, and Project skills (either Tool); anything else (Plugin
// skills, unknown paths) is rejected. Tool is only meaningful to
// disambiguate a Project location (Personal/Codex sources imply their own
// Tool and Uninstall doesn't need it disambiguated for them).
func (e *Engine) classifyLocation(location string) (Source, Kind, Tool, error) {
	if _, ok := immediateChildOf(filepath.Join(e.roots.ClaudeHome, "skills"), location); ok {
		return SourcePersonal, KindSkill, ToolClaudeCode, nil
	}
	if _, ok := immediateChildOf(filepath.Join(e.roots.CodexHome, "skills"), location); ok {
		return SourceCodex, KindSkill, ToolCodex, nil
	}
	if _, ok := immediateChildOf(filepath.Join(e.roots.AgentsHome, "skills"), location); ok {
		return SourceCodex, KindSkill, ToolCodex, nil
	}
	if _, ok := immediateChildOf(filepath.Join(e.roots.CodexHome, "prompts"), location); ok {
		return SourceCodex, KindPrompt, ToolCodex, nil
	}
	for _, root := range e.roots.ClaudeProjectRoots {
		if _, ok := immediateChildOf(filepath.Join(root, ".claude", "skills"), location); ok {
			return SourceProject, KindSkill, ToolClaudeCode, nil
		}
	}
	for _, root := range e.roots.ProjectRoots {
		if _, ok := immediateChildOf(filepath.Join(root, ".agents", "skills"), location); ok {
			return SourceProject, KindSkill, ToolCodex, nil
		}
	}
	return "", "", "", fmt.Errorf("archive is not supported for this location: %s", location)
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

func (e *Engine) newArchiveID(folderName string) string {
	safeName := sanitizeIDPart(folderName)
	for {
		id := fmt.Sprintf("%d-%s", time.Now().UnixNano(), safeName)
		if _, err := os.Stat(filepath.Join(e.roots.DataDir, "archive", id)); os.IsNotExist(err) {
			return id
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

func moveSymlink(oldPath, newPath, target string) error {
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

func writeArchiveEntry(archiveDir string, entry ArchiveEntry) error {
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal provenance: %w", err)
	}
	if err := os.WriteFile(filepath.Join(archiveDir, "provenance.json"), append(data, '\n'), 0o600); err != nil {
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
