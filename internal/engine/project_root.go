package engine

import (
	"os"
	"path/filepath"
)

// FindProjectRoots returns Codex's inclusive ancestor chain for
// .agents/skills discovery, from cwd through the nearest git repo root. cwd
// and its parent remain the explicit non-repository fallback so an unbounded
// walk cannot unexpectedly inventory skills all the way to the filesystem
// root.
func FindProjectRoots(cwd string) []string {
	absCWD := absolutePath(cwd)
	repoRoot, ok := findGitRepoRoot(absCWD)
	if !ok {
		return dedupePaths([]string{absCWD, filepath.Dir(absCWD)})
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

// FindClaudeProjectRoots returns every directory from cwd up to and
// including the git repo root, for .claude/skills discovery (see
// docs/research/skill-mechanisms.md's "Claude Code project-skill discovery
// and Codex ancestor-discovery section). Deliberately asymmetric with
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
