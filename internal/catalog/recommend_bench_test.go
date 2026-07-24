package catalog_test

// Setup-path benchmark: how long RecommendedBundleIDs takes on a deep project
// directory containing a node_modules-shaped subtree. Run with:
//
//	go test ./internal/catalog/ -run XXX -bench RecommendedBundleIDs -benchtime 10x
//
// Uses only the exported API so the same file can be dropped into an older
// checkout to measure a before/after.

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/catalog"
)

func buildDeepProjectFixture(b *testing.B) string {
	b.Helper()
	root := b.TempDir()
	write := func(path string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			b.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			b.Fatalf("write: %v", err)
		}
	}
	for i := 0; i < 200; i++ {
		pkg := filepath.Join(root, "node_modules", fmt.Sprintf("pkg-%03d", i))
		for j := 0; j < 25; j++ {
			write(filepath.Join(pkg, "lib", fmt.Sprintf("mod-%02d.js", j)))
		}
	}
	for i := 0; i < 40; i++ {
		write(filepath.Join(root, "src", fmt.Sprintf("dir-%02d", i), "main.go"))
	}
	return root
}

func BenchmarkRecommendedBundleIDsDeepProject(b *testing.B) {
	c, err := catalog.Load()
	if err != nil {
		b.Fatalf("Load: %v", err)
	}
	root := buildDeepProjectFixture(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.RecommendedBundleIDs(root)
	}
}
