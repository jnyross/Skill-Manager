package engine

import (
	"context"
	"fmt"
)

// InstallBundle installs members in stored order and stops at the first hard
// error. Successful earlier members remain installed; the error names the
// member that failed so partial progress is explicit.
func (e *Engine) InstallBundle(bundleID string, target InstallTarget) error {
	return e.InstallBundleContext(context.Background(), bundleID, target)
}

// InstallBundleContext is InstallBundle with a caller-supplied context;
// cancelling ctx stops the run at the member currently installing.
func (e *Engine) InstallBundleContext(ctx context.Context, bundleID string, target InstallTarget) error {
	bundles, err := e.ListBundles()
	if err != nil {
		return fmt.Errorf("install bundle: %w", err)
	}
	var bundle *Bundle
	for i := range bundles {
		if bundles[i].ID == bundleID {
			bundle = &bundles[i]
			break
		}
	}
	if bundle == nil {
		return fmt.Errorf("install bundle: %q not found", bundleID)
	}
	entries, err := e.ListLibrary()
	if err != nil {
		return fmt.Errorf("install bundle %q: %w", bundle.Name, err)
	}
	byID := make(map[string]LibraryEntry, len(entries))
	for _, entry := range entries {
		byID[entry.ID] = entry
	}
	for _, member := range bundle.Members {
		entry, ok := byID[member.LibraryEntryID]
		if !ok {
			return fmt.Errorf("install bundle %q: member %q is missing from Library", bundle.Name, member.LibraryEntryID)
		}
		if err := e.InstallLibraryEntryContext(ctx, entry, target, member.Activation); err != nil {
			return fmt.Errorf("install bundle %q: member %q: %w", bundle.Name, entry.Name, err)
		}
	}
	return nil
}
