package engine

import (
	"os"
	"path/filepath"
)

// FindProjectRoots returns Codex's own fixed three-candidate list for
// .agents/skills discovery: cwd, cwd's parent, and the git repo root (if
// any), deduplicated. cwd and its parent are always included even without a
// git repo — mirroring Codex's own behavior, which doesn't require a repo at
// all to check those two directories.
func FindProjectRoots(cwd string) []string {
	absCWD := absolutePath(cwd)
	roots := []string{absCWD, filepath.Dir(absCWD)}
	if repoRoot, ok := findGitRepoRoot(absCWD); ok {
		roots = append(roots, repoRoot)
	}
	return dedupePaths(roots)
}

// FindClaudeProjectRoots returns every directory from cwd up to and
// including the git repo root, for .claude/skills discovery (see
// docs/research/skill-mechanisms.md's "Claude Code project-skill discovery
// vs. Codex's three-candidate rule" section). Deliberately asymmetric with
// FindProjectRoots outside a repo: with no git root to bound the walk-up,
// there's no safe stopping point short of the filesystem root, so this
// returns just cwd rather than walking further.
func FindClaudeProjectRoots(cwd string) []string {
	absCWD := absolutePath(cwd)
	repoRoot, ok := findGitRepoRoot(absCWD)
	if !ok {
		return []string{absCWD}
	}

	var roots []string
	for dir := absCWD; ; dir = filepath.Dir(dir) {
		roots = append(roots, dir)
		if samePath(dir, repoRoot) || isFilesystemRoot(dir) {
			break
		}
	}
	return dedupePaths(roots)
}

func findGitRepoRoot(cwd string) (string, bool) {
	for dir := absolutePath(cwd); ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, true
		}
		if isFilesystemRoot(dir) {
			return "", false
		}
	}
}

func dedupePaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	var result []string
	for _, path := range paths {
		abs := absolutePath(path)
		key := filepath.Clean(abs)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, abs)
	}
	return result
}

func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

func isFilesystemRoot(path string) bool {
	return samePath(path, filepath.Dir(path))
}
