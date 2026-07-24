package setup_test

// Setup-path benchmark: Service.Plan over a catalog-sized member list whose
// source boundaries carry realistic reference material. Run with:
//
//	go test ./internal/setup/ -run XXX -bench PlanCatalogSizedSelection -benchtime 20x
//
// Uses only the exported API so the same file can be dropped into an older
// checkout to measure a before/after.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/catalog"
	"github.com/jnyross/Skill-Manager/internal/setup"
)

func benchMember(b *testing.B, parent, name string) setup.ResolvedMember {
	b.Helper()
	source := filepath.Join(parent, name)
	body := make([]byte, 0, 8192)
	for len(body) < 8192 {
		body = append(body, "Reference material for a catalog member boundary.\n"...)
	}
	write := func(relative string, contents []byte) {
		path := filepath.Join(source, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			b.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, contents, 0o644); err != nil {
			b.Fatalf("write: %v", err)
		}
	}
	write("SKILL.md", []byte("---\nname: "+name+"\ndescription: Bench member\n---\n"+string(body)))
	write("agents/openai.yaml", []byte("policy:\n  allow_implicit_invocation: true\n"))
	for i := 0; i < 8; i++ {
		write(fmt.Sprintf("references/doc-%02d.md", i), body)
	}
	return setup.ResolvedMember{
		Member: catalog.Member{
			Name: name, Family: "fixture", UpstreamActivation: "auto",
			Recipes: []catalog.Recipe{
				{Tool: "claude-code", Scope: "project", Artifact: "direct-skill"},
				{Tool: "codex", Scope: "project", Artifact: "direct-skill"},
			},
		},
		SourceDir: source,
	}
}

func BenchmarkPlanCatalogSizedSelection(b *testing.B) {
	parent := b.TempDir()
	members := make([]setup.ResolvedMember, 0, 20)
	for i := 0; i < 20; i++ {
		members = append(members, benchMember(b, parent, fmt.Sprintf("member-%02d", i)))
	}
	service := setup.NewService()
	target := filepath.Join(b.TempDir(), "workspace")
	request := setup.Request{
		TargetPath: target, CatalogVersion: "bench.1", BundleIDs: []string{"bench-bundle"}, Members: members,
	}
	if _, err := service.Plan(context.Background(), request); err != nil {
		b.Fatalf("Plan: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := service.Plan(context.Background(), request); err != nil {
			b.Fatalf("Plan: %v", err)
		}
	}
}
