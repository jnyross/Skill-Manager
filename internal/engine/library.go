package engine

// Library — Skillet-owned personal catalog of install-source pointers
// (CONTEXT.md Library; ADR 0004). Persistence mirrors Suppress records:
// one JSON file per entry under <DataDir>/library/<id>.json.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ListLibrary returns every Library entry, oldest AddedAt first. A missing
// library directory is an empty catalog, not an error.
func (e *Engine) ListLibrary() ([]LibraryEntry, error) {
	return loadLibraryEntries(e.roots.DataDir)
}

// AddLibraryEntry assigns ID and AddedAt, validates the source descriptor for
// kinds this phase supports writing, and persists the entry. It never copies
// or mutates skill trees on disk.
func (e *Engine) AddLibraryEntry(entry LibraryEntry) (LibraryEntry, error) {
	if err := validateLibraryEntry(entry); err != nil {
		return LibraryEntry{}, err
	}
	entry.ID = e.newLibraryID(entry.Name)
	entry.AddedAt = time.Now().UTC()
	if err := writeLibraryEntry(e.roots.DataDir, entry); err != nil {
		return LibraryEntry{}, err
	}
	return entry, nil
}

// RemoveLibraryEntry deletes the catalog record only — never the installed
// skill at LocalPath or any other source. It refuses to leave dangling Bundle
// references and names every Bundle the user must update first.
func (e *Engine) RemoveLibraryEntry(id string) error {
	if err := validateSinglePathSegment(id, "remove library entry"); err != nil {
		return err
	}
	bundles, err := e.ListBundles()
	if err != nil {
		return fmt.Errorf("remove library entry: check bundle references: %w", err)
	}
	var references []string
	for _, bundle := range bundles {
		for _, member := range bundle.Members {
			if member.LibraryEntryID == id {
				references = append(references, bundle.Name)
				break
			}
		}
	}
	if len(references) > 0 {
		sort.Strings(references)
		return fmt.Errorf("remove library entry: still referenced by bundle(s): %s", strings.Join(references, ", "))
	}
	path := libraryEntryPath(e.roots.DataDir, id)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("remove library entry: %q not found", id)
		}
		return fmt.Errorf("remove library entry: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove library entry: %w", err)
	}
	return nil
}

// FindLibraryEntryByLocalPath returns the Library entry whose local-path
// source matches path, if any. Used by the main-view Library toggle.
func (e *Engine) FindLibraryEntryByLocalPath(path string) (LibraryEntry, bool) {
	entries, err := e.ListLibrary()
	if err != nil {
		return LibraryEntry{}, false
	}
	for _, entry := range entries {
		if entry.Source.Kind == LibrarySourceLocalPath && entry.Source.LocalPath == path {
			return entry, true
		}
	}
	return LibraryEntry{}, false
}

func validateLibraryEntry(entry LibraryEntry) error {
	if strings.TrimSpace(entry.Name) == "" {
		return fmt.Errorf("library entry: name is required")
	}
	switch entry.Source.Kind {
	case LibrarySourceLocalPath:
		if strings.TrimSpace(entry.Source.LocalPath) == "" {
			return fmt.Errorf("library entry: local-path source requires LocalPath")
		}
	case LibrarySourceGit:
		if strings.TrimSpace(entry.Source.GitURL) == "" {
			return fmt.Errorf("library entry: git source requires GitURL")
		}
	case LibrarySourceSkillsSh:
		if strings.TrimSpace(entry.Source.SkillsShRepo) == "" {
			return fmt.Errorf("library entry: skills.sh source requires SkillsShRepo")
		}
	case LibrarySourceMarketplace:
		if strings.TrimSpace(entry.Source.Marketplace) == "" || strings.TrimSpace(entry.Source.PluginName) == "" {
			return fmt.Errorf("library entry: marketplace source requires Marketplace and PluginName")
		}
	default:
		return fmt.Errorf("library entry: unknown source kind %q", entry.Source.Kind)
	}
	return nil
}

func libraryDir(dataDir string) string {
	return filepath.Join(dataDir, "library")
}

func libraryEntryPath(dataDir, id string) string {
	return filepath.Join(libraryDir(dataDir), id+".json")
}

func (e *Engine) newLibraryID(name string) string {
	safeName := sanitizeIDPart(name)
	if safeName == "" {
		safeName = "entry"
	}
	for {
		id := fmt.Sprintf("%d-%s", time.Now().UnixNano(), safeName)
		if _, err := os.Stat(libraryEntryPath(e.roots.DataDir, id)); os.IsNotExist(err) {
			return id
		}
	}
}

func loadLibraryEntries(dataDir string) ([]LibraryEntry, error) {
	dir := libraryDir(dataDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read library: %w", err)
	}

	var out []LibraryEntry
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, ent.Name()))
		if err != nil {
			return nil, fmt.Errorf("read library entry %s: %w", ent.Name(), err)
		}
		var entry LibraryEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			return nil, fmt.Errorf("parse library entry %s: %w", ent.Name(), err)
		}
		// Filename is the durable identity (same as archive folder names /
		// suppression record paths); body ID must not diverge.
		entry.ID = strings.TrimSuffix(ent.Name(), ".json")
		out = append(out, entry)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].AddedAt.Equal(out[j].AddedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].AddedAt.Before(out[j].AddedAt)
	})
	return out, nil
}

func writeLibraryEntry(dataDir string, entry LibraryEntry) error {
	dir := libraryDir(dataDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create library directory: %w", err)
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal library entry: %w", err)
	}
	path := libraryEntryPath(dataDir, entry.ID)
	if err := writeFileAtomic(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write library entry: %w", err)
	}
	return nil
}
