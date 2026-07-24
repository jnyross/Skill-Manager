package setup

import "sync"

// boundaryCache memoizes readBoundaryWithModes for the duration of a single
// setup run. A catalog member's source boundary is read once by
// InspectBoundary during resolution and then again by Service.Plan and by
// every ToolAdapter.Render — three full directory walks plus three complete
// slurps of every file, per member, per Plan call, for bytes that cannot
// change while the run is in flight. RunTerminal creates one cache and
// threads it through Request so the plan-twice path (a conflict prompt
// re-plans) also reuses it.
//
// The cache is deliberately not a package-level singleton: source directories
// are frequently temporary git clones whose paths get reused, so a cache that
// outlived its run could serve one run's bytes to the next. A nil
// *boundaryCache is valid and reads through on every call, which is what
// callers that construct a Request themselves (tests, scripts) get.
//
// Destination boundaries — the ones verify.go re-reads after placement — are
// never cached: those files are written during the run, and re-reading them
// from disk is the entire point of verification.
type boundaryCache struct {
	mu      sync.Mutex
	entries map[string]boundaryEntry
}

type boundaryEntry struct {
	files map[string][]byte
	modes map[string]uint32
	err   error
}

func newBoundaryCache() *boundaryCache {
	return &boundaryCache{entries: make(map[string]boundaryEntry)}
}

// read returns root's boundary, reading it from disk at most once per cache.
// Callers get their own map values (the file contents themselves are shared,
// and every mutating caller already clones before editing).
func (cache *boundaryCache) read(root string) (map[string][]byte, map[string]uint32, error) {
	if cache == nil {
		return readBoundaryWithModes(root)
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.entries == nil {
		cache.entries = make(map[string]boundaryEntry)
	}
	entry, ok := cache.entries[root]
	if !ok {
		files, modes, err := readBoundaryWithModes(root)
		entry = boundaryEntry{files: files, modes: modes, err: err}
		cache.entries[root] = entry
	}
	if entry.err != nil {
		return entry.files, entry.modes, entry.err
	}
	return copyFileMap(entry.files), copyModeMap(entry.modes), nil
}

func copyFileMap(source map[string][]byte) map[string][]byte {
	copied := make(map[string][]byte, len(source))
	for name, contents := range source {
		copied[name] = contents
	}
	return copied
}

func copyModeMap(source map[string]uint32) map[string]uint32 {
	copied := make(map[string]uint32, len(source))
	for name, mode := range source {
		copied[name] = mode
	}
	return copied
}
