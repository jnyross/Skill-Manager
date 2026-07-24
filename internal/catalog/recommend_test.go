package catalog_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jnyross/Skill-Manager/internal/catalog"
)

func mkfile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestRecommendedBundleIDsFindsTopLevelDotnetMarkers(t *testing.T) {
	c, err := catalog.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, marker := range []string{"App.sln", "App.slnx", "src/App.csproj", "global.json"} {
		root := t.TempDir()
		mkfile(t, filepath.Join(root, filepath.FromSlash(marker)))
		got := c.RecommendedBundleIDs(root)
		if !reflect.DeepEqual(got, []string{"dotnet-starter"}) {
			t.Fatalf("marker %s: got %#v, want [dotnet-starter]", marker, got)
		}
	}
}

// TestRecommendedBundleIDsSkipsDependencyTrees pins the bounded traversal:
// this scan exists only to spot a .NET project at the top of the chosen
// folder, so a marker vendored inside a dependency or build-output tree is
// neither a signal nor worth the walk.
func TestRecommendedBundleIDsSkipsDependencyTrees(t *testing.T) {
	c, err := catalog.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, skipped := range []string{"node_modules", "vendor", "venv", ".venv", "target", "dist", "build", ".next", ".git", ".skillet"} {
		root := t.TempDir()
		mkfile(t, filepath.Join(root, skipped, "pkg", "Vendored.csproj"))
		if got := c.RecommendedBundleIDs(root); len(got) != 0 {
			t.Fatalf("marker under %s produced %#v, want no recommendation", skipped, got)
		}
	}
}

func TestRecommendedBundleIDsStopsAtDepthCap(t *testing.T) {
	c, err := catalog.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c", "d", "e", "f", "g")
	mkfile(t, filepath.Join(deep, "Buried.csproj"))
	if got := c.RecommendedBundleIDs(root); len(got) != 0 {
		t.Fatalf("marker below the depth cap produced %#v, want no recommendation", got)
	}
	mkfile(t, filepath.Join(root, "a", "b", "Near.csproj"))
	if got := c.RecommendedBundleIDs(root); !reflect.DeepEqual(got, []string{"dotnet-starter"}) {
		t.Fatalf("marker within the depth cap: got %#v, want [dotnet-starter]", got)
	}
}

// TestRecommendedBundleIDsDoesNotWalkLargeDependencyTree is the "setup never
// stalls silently" guard: a chosen folder with a node_modules-shaped subtree
// must be classified from its top level, not by reading the subtree.
func TestRecommendedBundleIDsDoesNotWalkLargeDependencyTree(t *testing.T) {
	c, err := catalog.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	root := t.TempDir()
	for i := 0; i < 200; i++ {
		pkg := filepath.Join(root, "node_modules", "pkg"+string(rune('a'+i%26))+string(rune('a'+i/26)))
		for j := 0; j < 25; j++ {
			mkfile(t, filepath.Join(pkg, "lib", "file"+string(rune('a'+j))+".js"))
		}
	}
	start := time.Now()
	got := c.RecommendedBundleIDs(root)
	elapsed := time.Since(start)
	if len(got) != 0 {
		t.Fatalf("got %#v, want no recommendation", got)
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("scan of a node_modules-shaped tree took %s; the skip list is not being applied", elapsed)
	}
}
