package engine_test

// A realistic-scale scan benchmark: ~50 skills across all four sources, one
// large plugin tree whose skills carry the reference/script payload real
// marketplace plugins ship, and a deep project directory containing a
// node_modules-shaped subtree. Run with:
//
//	go test ./internal/engine/ -run XXX -bench InventoryRealisticFixture -benchtime 20x
//
// It uses only the exported engine API so the same file can be dropped into an
// older checkout to measure a before/after.

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

func benchWriteFile(b *testing.B, path, contents string) {
	b.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		b.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		b.Fatalf("write %s: %v", path, err)
	}
}

func benchWriteSkill(b *testing.B, folder, name string) {
	b.Helper()
	benchWriteFile(b, filepath.Join(folder, "SKILL.md"),
		fmt.Sprintf("---\nname: %q\ndescription: %q\n---\n%s\n", name, name+" description", longBody))
}

// benchWritePayload gives a skill directory the shape a real plugin skill has:
// reference documents, scripts, and assets that the old full-tree walk visited
// on every single refresh.
func benchWritePayload(b *testing.B, folder string, files int) {
	b.Helper()
	for i := 0; i < files; i++ {
		sub := []string{"references", "scripts", "assets"}[i%3]
		benchWriteFile(b, filepath.Join(folder, sub, fmt.Sprintf("file-%02d.md", i)), longBody)
	}
}

var longBody = func() string {
	line := "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod.\n"
	out := make([]byte, 0, len(line)*40)
	for i := 0; i < 40; i++ {
		out = append(out, line...)
	}
	return string(out)
}()

func buildRealisticScanFixture(b *testing.B) engine.Roots {
	b.Helper()
	root := b.TempDir()
	claudeHome := filepath.Join(root, "claude")
	codexHome := filepath.Join(root, "codex")
	agentsHome := filepath.Join(root, "agents")
	dataDir := filepath.Join(root, "data")

	// 18 personal skills.
	for i := 0; i < 18; i++ {
		benchWriteSkill(b, filepath.Join(claudeHome, "skills", fmt.Sprintf("personal-%02d", i)), fmt.Sprintf("personal-%02d", i))
	}
	// 10 Codex skills plus a config.toml both Codex consumers need.
	for i := 0; i < 10; i++ {
		benchWriteSkill(b, filepath.Join(codexHome, "skills", fmt.Sprintf("codex-%02d", i)), fmt.Sprintf("codex-%02d", i))
	}
	benchWriteFile(b, filepath.Join(codexHome, "config.toml"), "[[skills.config]]\nname = \"codex-03\"\nenabled = false\n")
	benchWriteSkill(b, filepath.Join(agentsHome, "skills", "agents-shared"), "agents-shared")

	// One large plugin tree: 15 skills, each with 30 payload files, in the
	// nested category layout real marketplaces use.
	installPath := filepath.Join(root, "plugin-cache", "market", "big-plugin", "1.0.0")
	for i := 0; i < 15; i++ {
		category := []string{"engineering", "productivity", "misc"}[i%3]
		folder := filepath.Join(installPath, "skills", category, fmt.Sprintf("plugin-%02d", i))
		benchWriteSkill(b, folder, fmt.Sprintf("plugin-%02d", i))
		benchWritePayload(b, folder, 30)
	}
	benchWriteFile(b, filepath.Join(claudeHome, "plugins", "installed_plugins.json"),
		fmt.Sprintf(`{"version":2,"plugins":{"big-plugin@market":[{"scope":"user","installPath":%q,"version":"1.0.0"}]}}`, installPath))

	// A deep project directory with a node_modules-shaped subtree.
	repo := filepath.Join(root, "workspace", "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		b.Fatalf("mkdir .git: %v", err)
	}
	for i := 0; i < 3; i++ {
		benchWriteSkill(b, filepath.Join(repo, ".claude", "skills", fmt.Sprintf("proj-claude-%02d", i)), fmt.Sprintf("proj-claude-%02d", i))
		benchWriteSkill(b, filepath.Join(repo, ".agents", "skills", fmt.Sprintf("proj-codex-%02d", i)), fmt.Sprintf("proj-codex-%02d", i))
	}
	for i := 0; i < 60; i++ {
		pkg := filepath.Join(repo, "node_modules", fmt.Sprintf("pkg-%03d", i))
		for j := 0; j < 20; j++ {
			benchWriteFile(b, filepath.Join(pkg, "lib", fmt.Sprintf("mod-%02d.js", j)), longBody)
		}
	}
	cwd := filepath.Join(repo, "services", "api", "internal", "handlers", "v2")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		b.Fatalf("mkdir cwd: %v", err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		b.Fatalf("mkdir dataDir: %v", err)
	}

	return engine.Roots{
		ClaudeHome:         claudeHome,
		CodexHome:          codexHome,
		AgentsHome:         agentsHome,
		DataDir:            dataDir,
		ProjectRoots:       engine.FindProjectRoots(cwd),
		ClaudeProjectRoots: engine.FindClaudeProjectRoots(cwd),
	}
}

func BenchmarkInventoryRealisticFixture(b *testing.B) {
	roots := buildRealisticScanFixture(b)
	e := engine.New(roots)
	if got := len(e.Inventory().Skills); got != 50 {
		b.Fatalf("fixture yielded %d skills, want 50", got)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if len(e.Inventory().Skills) != 50 {
			b.Fatal("inventory changed shape mid-benchmark")
		}
	}
}
