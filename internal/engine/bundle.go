package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BundleMember pairs a Library entry with the activation preference this
// Bundle remembers for installation.
type BundleMember struct {
	LibraryEntryID string          `json:"libraryEntryId"`
	Activation     ActivationState `json:"activation"`
}

// Bundle is a named, reusable set of Library entries.
type Bundle struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Members []BundleMember `json:"members"`
}

func (e *Engine) ListBundles() ([]Bundle, error) {
	dir := bundleDir(e.roots.DataDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read bundles: %w", err)
	}

	var bundles []Bundle
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read bundle %s: %w", entry.Name(), err)
		}
		var bundle Bundle
		if err := json.Unmarshal(data, &bundle); err != nil {
			return nil, fmt.Errorf("parse bundle %s: %w", entry.Name(), err)
		}
		bundle.ID = strings.TrimSuffix(entry.Name(), ".json")
		bundles = append(bundles, bundle)
	}
	sort.SliceStable(bundles, func(i, j int) bool {
		if bundles[i].Name == bundles[j].Name {
			return bundles[i].ID < bundles[j].ID
		}
		return bundles[i].Name < bundles[j].Name
	})
	return bundles, nil
}

func (e *Engine) CreateBundle(name string) (Bundle, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Bundle{}, fmt.Errorf("create bundle: name is required")
	}
	bundle := Bundle{ID: e.newBundleID(name), Name: name, Members: []BundleMember{}}
	if err := writeBundle(e.roots.DataDir, bundle); err != nil {
		return Bundle{}, err
	}
	return bundle, nil
}

func (e *Engine) DeleteBundle(id string) error {
	if _, err := e.loadBundle(id); err != nil {
		return err
	}
	if err := os.Remove(bundlePath(e.roots.DataDir, id)); err != nil {
		return fmt.Errorf("delete bundle %q: %w", id, err)
	}
	return nil
}

func (e *Engine) AddBundleMember(bundleID, libraryEntryID string, activation ActivationState) error {
	if err := validateBundleActivation(activation); err != nil {
		return err
	}
	if _, err := e.loadLibraryEntry(libraryEntryID); err != nil {
		return err
	}
	bundle, err := e.loadBundle(bundleID)
	if err != nil {
		return err
	}
	for _, member := range bundle.Members {
		if member.LibraryEntryID == libraryEntryID {
			return fmt.Errorf("add bundle member: Library entry %q is already a member", libraryEntryID)
		}
	}
	bundle.Members = append(bundle.Members, BundleMember{LibraryEntryID: libraryEntryID, Activation: activation})
	return writeBundle(e.roots.DataDir, bundle)
}

func (e *Engine) RemoveBundleMember(bundleID, libraryEntryID string) error {
	bundle, err := e.loadBundle(bundleID)
	if err != nil {
		return err
	}
	for i, member := range bundle.Members {
		if member.LibraryEntryID == libraryEntryID {
			bundle.Members = append(bundle.Members[:i], bundle.Members[i+1:]...)
			return writeBundle(e.roots.DataDir, bundle)
		}
	}
	return fmt.Errorf("remove bundle member: Library entry %q is not a member", libraryEntryID)
}

func (e *Engine) SetBundleMemberActivation(bundleID, libraryEntryID string, activation ActivationState) error {
	if err := validateBundleActivation(activation); err != nil {
		return err
	}
	bundle, err := e.loadBundle(bundleID)
	if err != nil {
		return err
	}
	for i := range bundle.Members {
		if bundle.Members[i].LibraryEntryID == libraryEntryID {
			bundle.Members[i].Activation = activation
			return writeBundle(e.roots.DataDir, bundle)
		}
	}
	return fmt.Errorf("set bundle member activation: Library entry %q is not a member", libraryEntryID)
}

func validateBundleActivation(activation ActivationState) error {
	if activation != ActivationAuto && activation != ActivationManualOnly {
		return fmt.Errorf("bundle member: activation must be %q or %q", ActivationAuto, ActivationManualOnly)
	}
	return nil
}

func bundleDir(dataDir string) string { return filepath.Join(dataDir, "bundles") }

func bundlePath(dataDir, id string) string { return filepath.Join(bundleDir(dataDir), id+".json") }

func (e *Engine) newBundleID(name string) string {
	safeName := sanitizeIDPart(name)
	if safeName == "" {
		safeName = "bundle"
	}
	for {
		id := fmt.Sprintf("%d-%s", time.Now().UnixNano(), safeName)
		if _, err := os.Stat(bundlePath(e.roots.DataDir, id)); os.IsNotExist(err) {
			return id
		}
	}
}

func (e *Engine) loadBundle(id string) (Bundle, error) {
	if strings.TrimSpace(id) == "" {
		return Bundle{}, fmt.Errorf("bundle: empty id")
	}
	data, err := os.ReadFile(bundlePath(e.roots.DataDir, id))
	if err != nil {
		if os.IsNotExist(err) {
			return Bundle{}, fmt.Errorf("bundle %q not found", id)
		}
		return Bundle{}, fmt.Errorf("read bundle %q: %w", id, err)
	}
	var bundle Bundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return Bundle{}, fmt.Errorf("parse bundle %q: %w", id, err)
	}
	bundle.ID = id
	return bundle, nil
}

func (e *Engine) loadLibraryEntry(id string) (LibraryEntry, error) {
	if strings.TrimSpace(id) == "" {
		return LibraryEntry{}, fmt.Errorf("Library entry: empty id")
	}
	entries, err := e.ListLibrary()
	if err != nil {
		return LibraryEntry{}, err
	}
	for _, entry := range entries {
		if entry.ID == id {
			return entry, nil
		}
	}
	return LibraryEntry{}, fmt.Errorf("Library entry %q not found", id)
}

func writeBundle(dataDir string, bundle Bundle) error {
	dir := bundleDir(dataDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create bundle directory: %w", err)
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal bundle: %w", err)
	}
	if err := os.WriteFile(bundlePath(dataDir, bundle.ID), append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write bundle: %w", err)
	}
	return nil
}
