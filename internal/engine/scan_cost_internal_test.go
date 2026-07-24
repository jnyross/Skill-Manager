package engine

// Scan-cost tests: they assert how much filesystem work one Inventory() does,
// not just what it returns. Each uses a package-global counting hook, so —
// like the write-fault tests in write_fault_test.go — none of them may be made
// parallel.

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func countingHook(t *testing.T, install func(func(string)), counts *map[string]int) {
	t.Helper()
	seen := make(map[string]int)
	*counts = seen
	install(func(path string) { seen[path]++ })
	t.Cleanup(func() { install(nil) })
}

func writeTestSkill(t *testing.T, folder, name, extraFrontmatter string) {
	t.Helper()
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", folder, err)
	}
	content := "---\nname: " + strconv.Quote(name) + "\ndescription: " + strconv.Quote(name+" description") + "\n" + extraFrontmatter + "---\nBody\n"
	if err := os.WriteFile(filepath.Join(folder, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}

func newCostFixtureRoots(t *testing.T) (string, Roots) {
	t.Helper()
	root := t.TempDir()
	roots := Roots{
		ClaudeHome: filepath.Join(root, "claude"),
		CodexHome:  filepath.Join(root, "codex"),
		AgentsHome: filepath.Join(root, "agents"),
		DataDir:    filepath.Join(root, "data"),
	}
	for _, dir := range []string{
		filepath.Join(roots.ClaudeHome, "skills"),
		filepath.Join(roots.ClaudeHome, "plugins"),
		filepath.Join(roots.CodexHome, "skills"),
		filepath.Join(roots.AgentsHome, "skills"),
		roots.DataDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	return root, roots
}

// TestInventoryReadsCodexConfigOnce pins requirement 1: ~/.codex/config.toml
// used to be read and TOML-decoded twice per refresh, once by the Codex scan
// and once by the Codex half of the Project scan.
func TestInventoryReadsCodexConfigOnce(t *testing.T) {
	root, roots := newCostFixtureRoots(t)

	project := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	writeTestSkill(t, filepath.Join(project, ".agents", "skills", "proj"), "proj", "")
	roots.ProjectRoots = FindProjectRoots(project)
	roots.ClaudeProjectRoots = FindClaudeProjectRoots(project)

	configPath := filepath.Join(roots.CodexHome, "config.toml")
	if err := os.WriteFile(configPath, []byte("[[skills.config]]\nname = \"nope\"\nenabled = false\n"), 0o644); err != nil {
		t.Fatalf("write codex config: %v", err)
	}
	writeTestSkill(t, filepath.Join(roots.CodexHome, "skills", "codexer"), "codexer", "")

	var counts map[string]int
	countingHook(t, func(fn func(string)) { codexConfigReadHook = fn }, &counts)

	inv := New(roots).Inventory()
	if len(inv.Skills) == 0 {
		t.Fatalf("fixture produced no skills")
	}
	if got := counts[configPath]; got != 1 {
		t.Fatalf("config.toml decoded %d times, want exactly 1", got)
	}
}

// TestInventoryReportsUnreadableCodexConfigOnce is the user-visible half of
// requirement 1: two independent readers meant one broken config produced the
// same notice twice.
func TestInventoryReportsUnreadableCodexConfigOnce(t *testing.T) {
	root, roots := newCostFixtureRoots(t)
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	roots.ProjectRoots = FindProjectRoots(project)
	roots.ClaudeProjectRoots = FindClaudeProjectRoots(project)

	if err := os.WriteFile(filepath.Join(roots.CodexHome, "config.toml"), []byte("this = is = not = toml\n"), 0o644); err != nil {
		t.Fatalf("write codex config: %v", err)
	}

	inv := New(roots).Inventory()
	found := 0
	for _, notice := range inv.Notices {
		if strings.HasPrefix(notice.Message, "Codex config unreadable") {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("got %d unreadable-config notices, want exactly 1: %#v", found, inv.Notices)
	}
}

// TestScanParsesEachPluginSkillFrontmatterOnce pins requirement 2: a
// suppressed plugin skill's SKILL.md was parsed by the scan and then parsed
// again by the suppression reconciliation pass.
func TestScanParsesEachPluginSkillFrontmatterOnce(t *testing.T) {
	root, roots := newCostFixtureRoots(t)
	installPath := filepath.Join(root, "plugin-cache", "market", "plugin-x", "v1")
	plain := filepath.Join(installPath, "skills", "plain")
	hidden := filepath.Join(installPath, "skills", "hidden")
	writeTestSkill(t, plain, "plain", "")
	writeTestSkill(t, hidden, "hidden", "disable-model-invocation: true\nuser-invocable: false\n")
	writePluginManifestFile(t, roots.ClaudeHome, installPath)

	if err := writeSuppressionRecord(roots.DataDir, SuppressionRecord{
		Marketplace: "marketplace-x", Plugin: "plugin-x", SkillName: "hidden", SuppressedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("write suppression record: %v", err)
	}

	var counts map[string]int
	countingHook(t, func(fn func(string)) { frontmatterParseHook = fn }, &counts)

	inv := New(roots).Inventory()

	suppressed := false
	for _, skill := range inv.Skills {
		if skill.Name == "hidden" && skill.Activation == ActivationSuppressed {
			suppressed = true
		}
	}
	if !suppressed {
		t.Fatalf("suppressed plugin skill was not marked Suppressed: %#v", inv.Skills)
	}
	for _, folder := range []string{plain, hidden} {
		path := filepath.Join(folder, "SKILL.md")
		if got := counts[path]; got != 1 {
			t.Fatalf("%s parsed %d times, want exactly 1", path, got)
		}
	}
}

// TestSuppressionSelfHealsWithoutARedundantParse keeps WP1's self-healing
// behaviour honest now that the reconciliation pass trusts the scan's parse:
// an unedited SKILL.md (as a plugin update leaves behind) must still be
// re-edited on the next scan.
func TestSuppressionSelfHealsWithoutARedundantParse(t *testing.T) {
	root, roots := newCostFixtureRoots(t)
	installPath := filepath.Join(root, "plugin-cache", "market", "plugin-x", "v2")
	folder := filepath.Join(installPath, "skills", "hidden")
	writeTestSkill(t, folder, "hidden", "")
	writePluginManifestFile(t, roots.ClaudeHome, installPath)
	if err := writeSuppressionRecord(roots.DataDir, SuppressionRecord{
		Marketplace: "marketplace-x", Plugin: "plugin-x", SkillName: "hidden", SuppressedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("write suppression record: %v", err)
	}

	inv := New(roots).Inventory()
	if len(inv.Skills) != 1 || inv.Skills[0].Activation != ActivationSuppressed {
		t.Fatalf("skill not re-suppressed: %#v", inv.Skills)
	}
	data, err := os.ReadFile(filepath.Join(folder, "SKILL.md"))
	if err != nil {
		t.Fatalf("read healed SKILL.md: %v", err)
	}
	fm, err := parseSkillFrontmatter(filepath.Join(folder, "SKILL.md"))
	if err != nil {
		t.Fatalf("parse healed SKILL.md: %v (%s)", err, data)
	}
	if !isSuppressionApplied(fm) {
		t.Fatalf("suppression edit was not re-applied: %s", data)
	}
}

// TestPluginScanDoesNotDescendIntoSkillPayload pins requirement 3: a skill
// directory's own subtree (references/, scripts/, node_modules/) is that
// skill's payload, not a container of further skills, so it is never walked.
func TestPluginScanDoesNotDescendIntoSkillPayload(t *testing.T) {
	root, roots := newCostFixtureRoots(t)
	installPath := filepath.Join(root, "plugin-cache", "market", "plugin-x", "v1")
	skillDir := filepath.Join(installPath, "skills", "outer")
	writeTestSkill(t, skillDir, "outer", "")
	// A SKILL.md quoted inside a skill's own reference material must not be
	// inventoried as a second skill.
	writeTestSkill(t, filepath.Join(skillDir, "references", "example"), "example-from-docs", "")
	writePluginManifestFile(t, roots.ClaudeHome, installPath)

	var counts map[string]int
	countingHook(t, func(fn func(string)) { frontmatterParseHook = fn }, &counts)

	inv := New(roots).Inventory()
	if len(inv.Skills) != 1 || inv.Skills[0].Name != "outer" {
		t.Fatalf("got %#v, want only the outer skill", inv.Skills)
	}
	nested := filepath.Join(skillDir, "references", "example", "SKILL.md")
	if counts[nested] != 0 {
		t.Fatalf("payload SKILL.md was opened %d times, want 0", counts[nested])
	}
}

// TestPluginScanKeepsNestedCategoryLayout guards the discovery semantics that
// requirement 3 must not break. Real marketplace plugins nest: as of the WP4
// survey of ~/.claude/plugins/cache, mattpocock/mattpocock-skills groups all
// 41 of its skills as skills/<category>/<name>/SKILL.md while every other
// installed plugin uses skills/<name>/SKILL.md.
func TestPluginScanKeepsNestedCategoryLayout(t *testing.T) {
	root, roots := newCostFixtureRoots(t)
	installPath := filepath.Join(root, "plugin-cache", "market", "plugin-x", "v1")
	writeTestSkill(t, filepath.Join(installPath, "skills", "flat"), "flat", "")
	writeTestSkill(t, filepath.Join(installPath, "skills", "engineering", "nested"), "nested", "")
	writeTestSkill(t, filepath.Join(installPath, "skills", "a", "b", "c", "deep"), "deep", "")
	writePluginManifestFile(t, roots.ClaudeHome, installPath)

	inv := New(roots).Inventory()
	got := make(map[string]bool)
	for _, skill := range inv.Skills {
		got[skill.Name] = true
	}
	for _, name := range []string{"flat", "nested", "deep"} {
		if !got[name] {
			t.Fatalf("missing %s in %#v", name, inv.Skills)
		}
	}
}

// TestMergedAncestorWalkMatchesSeparateWalks pins requirement 4: the merged
// pass must return exactly what the two separate walks returned, in a repo
// and outside one, and must stat .git once per ancestor level rather than
// twice.
func TestMergedAncestorWalkMatchesSeparateWalks(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	inRepo := filepath.Join(repoRoot, "a", "b")
	if err := os.MkdirAll(inRepo, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	outsideRepo := filepath.Join(t.TempDir(), "x", "y")
	if err := os.MkdirAll(outsideRepo, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}

	for _, cwd := range []string{inRepo, repoRoot, outsideRepo} {
		wantCodex := referenceProjectRoots(cwd)
		wantClaude := referenceClaudeProjectRoots(cwd)
		gotCodex, gotClaude := FindProjectRootsForTools(cwd)
		if !reflect.DeepEqual(gotCodex, wantCodex) {
			t.Fatalf("codex roots for %s = %#v, want %#v", cwd, gotCodex, wantCodex)
		}
		if !reflect.DeepEqual(gotClaude, wantClaude) {
			t.Fatalf("claude roots for %s = %#v, want %#v", cwd, gotClaude, wantClaude)
		}
		if !reflect.DeepEqual(FindProjectRoots(cwd), wantCodex) {
			t.Fatalf("FindProjectRoots(%s) diverged from the merged pass", cwd)
		}
		if !reflect.DeepEqual(FindClaudeProjectRoots(cwd), wantClaude) {
			t.Fatalf("FindClaudeProjectRoots(%s) diverged from the merged pass", cwd)
		}
	}

	var counts map[string]int
	countingHook(t, func(fn func(string)) { gitProbeHook = fn }, &counts)
	FindProjectRootsForTools(inRepo)
	for path, n := range counts {
		if n != 1 {
			t.Fatalf("%s stat'd %d times in one merged pass, want 1", path, n)
		}
	}
	if len(counts) != 3 {
		t.Fatalf("merged pass stat'd %d levels, want 3 (b, a, repo root): %#v", len(counts), counts)
	}
}

// referenceProjectRoots and referenceClaudeProjectRoots reproduce the
// pre-merge two-walk implementations verbatim, so the test compares the
// merged pass against the behaviour it replaced rather than against itself.
func referenceProjectRoots(cwd string) []string {
	absCWD := absolutePath(cwd)
	repoRoot, ok := referenceGitRepoRoot(absCWD)
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

func referenceClaudeProjectRoots(cwd string) []string {
	absCWD := absolutePath(cwd)
	repoRoot, ok := referenceGitRepoRoot(absCWD)
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

func referenceGitRepoRoot(cwd string) (string, bool) {
	for dir := absolutePath(cwd); ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, true
		}
		if isFilesystemRoot(dir) {
			return "", false
		}
	}
}

func writePluginManifestFile(t *testing.T, claudeHome, installPath string) {
	t.Helper()
	manifest := `{"version":2,"plugins":{"plugin-x@marketplace-x":[{"scope":"user","installPath":` + strconv.Quote(installPath) + `,"version":"1.0.0"}]}}`
	path := filepath.Join(claudeHome, "plugins", "installed_plugins.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir plugins: %v", err)
	}
	if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write plugin manifest: %v", err)
	}
}
