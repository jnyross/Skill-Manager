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
	codexRoots, _ := FindProjectRootsForTools(cwd)
	return codexRoots
}

// FindProjectRootsForTools computes both ancestor chains from a single walk
// up from cwd, stat-ing .git exactly once per ancestor level. Calling
// FindProjectRoots and FindClaudeProjectRoots separately walks — and stats —
// the same chain twice for identical results; callers that need both lists
// should prefer this entry point. The returned lists are exactly what the two
// single-tool functions return, including their deliberate asymmetry outside a
// repository (see each function's doc comment).
func FindProjectRootsForTools(cwd string) (codexRoots, claudeRoots []string) {
	absCWD := absolutePath(cwd)
	chain, repoRootIndex := gitAncestorChain(absCWD)
	if repoRootIndex < 0 {
		return dedupePaths([]string{absCWD, filepath.Dir(absCWD)}), []string{absCWD}
	}
	roots := dedupePaths(chain[:repoRootIndex+1])
	return roots, roots
}

// gitAncestorChain walks up from absCWD to the filesystem root, stat-ing
// .git at each level, and stops at the first level that has one. It returns
// every directory it visited and the index of the repository root within that
// slice, or -1 when no .git was found anywhere on the chain.
// gitProbeHook is a test-only seam: when non-nil it is called for every
// .git stat performed while walking the ancestor chain, so a test can assert
// exactly one stat per ancestor level. Tests that install it must not run in
// parallel.
var gitProbeHook func(path string)

func gitAncestorChain(absCWD string) ([]string, int) {
	var chain []string
	for dir := absCWD; ; dir = filepath.Dir(dir) {
		chain = append(chain, dir)
		gitPath := filepath.Join(dir, ".git")
		if gitProbeHook != nil {
			gitProbeHook(gitPath)
		}
		if _, err := os.Stat(gitPath); err == nil {
			return chain, len(chain) - 1
		}
		if isFilesystemRoot(dir) {
			return chain, -1
		}
	}
}

// FindClaudeProjectRoots returns every directory from cwd up to and
// including the git repo root, for .claude/skills discovery (see
// docs/research/skill-mechanisms.md's "Claude Code project-skill discovery
// and Codex ancestor-discovery section). Deliberately asymmetric with
// FindProjectRoots outside a repo: with no git root to bound the walk-up,
// there's no safe stopping point short of the filesystem root, so this
// returns just cwd rather than walking further.
func FindClaudeProjectRoots(cwd string) []string {
	_, claudeRoots := FindProjectRootsForTools(cwd)
	return claudeRoots
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
