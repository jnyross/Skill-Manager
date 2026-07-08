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
	if err := e.requirePersonalSkillLocation(location); err != nil {
		return ArchiveEntry{}, err
	}

	info, err := os.Lstat(location)
	if err != nil {
		return ArchiveEntry{}, fmt.Errorf("inspect skill location: %w", err)
	}

	folderName := filepath.Base(location)
	archiveRoot := filepath.Join(e.roots.DataDir, "archive")
	id := e.newArchiveID(folderName)
	archiveDir := filepath.Join(archiveRoot, id)
	if err := os.MkdirAll(archiveDir, 0o700); err != nil {
		return ArchiveEntry{}, fmt.Errorf("create archive directory: %w", err)
	}

	archivedPath := filepath.Join(archiveDir, folderName)
	entry := ArchiveEntry{
		ID:               id,
		Name:             folderName,
		Source:           SourcePersonal,
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
	} else {
		if !info.IsDir() {
			return ArchiveEntry{}, fmt.Errorf("skill location is not a directory: %s", location)
		}
		if err := os.Rename(location, archivedPath); err != nil {
			return ArchiveEntry{}, fmt.Errorf("archive skill directory: %w", err)
		}
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

func (e *Engine) requirePersonalSkillLocation(location string) error {
	root := absolutePath(filepath.Join(e.roots.ClaudeHome, "skills"))
	rel, err := filepath.Rel(root, location)
	if err != nil {
		return fmt.Errorf("validate personal skill location: %w", err)
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return fmt.Errorf("archive is only supported for Personal skills under %s", root)
	}
	return nil
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
