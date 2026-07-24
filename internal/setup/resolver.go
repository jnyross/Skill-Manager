package setup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/jnyross/Skill-Manager/internal/catalog"
)

type DriftClass string

const (
	DriftContent        DriftClass = "content"
	DriftLicense        DriftClass = "license"
	DriftScripts        DriftClass = "scripts"
	DriftExecutableMode DriftClass = "executable-modes"
	DriftDependencies   DriftClass = "dependencies"
	DriftExternalAction DriftClass = "external-actions"
	DriftMetadata       DriftClass = "metadata"
)

type DriftChange struct {
	Class    DriftClass `json:"class"`
	Reviewed string     `json:"reviewed"`
	Resolved string     `json:"resolved"`
}

type DriftReview struct {
	Material         bool          `json:"material"`
	ReviewedRevision string        `json:"reviewedRevision"`
	ResolvedRevision string        `json:"resolvedRevision"`
	Changes          []DriftChange `json:"changes"`
}

func (review DriftReview) Has(class DriftClass) bool {
	for _, change := range review.Changes {
		if change.Class == class {
			return true
		}
	}
	return false
}

type BoundaryEvidence struct {
	Revision                     string   `json:"revision"`
	Subpath                      string   `json:"subpath"`
	ContentSHA256                string   `json:"contentSHA256"`
	LicenseSHA256                string   `json:"licenseSHA256"`
	Scripts                      []string `json:"scripts"`
	Executables                  []string `json:"executables"`
	MetadataSHA256               string   `json:"metadataSHA256"`
	DependencyEvidenceSHA256     string   `json:"dependencyEvidenceSHA256"`
	ExternalActionEvidenceSHA256 string   `json:"externalActionEvidenceSHA256"`
}

type ResolvedMember struct {
	Member    catalog.Member   `json:"member"`
	SourceDir string           `json:"-"`
	Evidence  BoundaryEvidence `json:"evidence"`
	Drift     DriftReview      `json:"drift"`
}

func CompareDrift(member catalog.Member, resolved BoundaryEvidence) DriftReview {
	review := DriftReview{ReviewedRevision: member.Source.ReviewedRevision, ResolvedRevision: resolved.Revision}
	add := func(class DriftClass, reviewed, actual string) {
		review.Changes = append(review.Changes, DriftChange{Class: class, Reviewed: reviewed, Resolved: actual})
	}
	if resolved.ContentSHA256 != member.Source.ContentSHA256 {
		add(DriftContent, member.Source.ContentSHA256, resolved.ContentSHA256)
	}
	if resolved.LicenseSHA256 != member.License.NoticeSHA256 {
		add(DriftLicense, member.License.NoticeSHA256, resolved.LicenseSHA256)
	}
	reviewedScripts := append([]string(nil), member.Scripts...)
	resolvedScripts := append([]string(nil), resolved.Scripts...)
	sort.Strings(reviewedScripts)
	sort.Strings(resolvedScripts)
	if !reflect.DeepEqual(reviewedScripts, resolvedScripts) {
		add(DriftScripts, strings.Join(reviewedScripts, ","), strings.Join(resolvedScripts, ","))
	}
	reviewedExecutables := append([]string(nil), member.Executables...)
	resolvedExecutables := append([]string(nil), resolved.Executables...)
	sort.Strings(reviewedExecutables)
	sort.Strings(resolvedExecutables)
	if strings.Join(reviewedExecutables, "\x00") != strings.Join(resolvedExecutables, "\x00") {
		add(DriftExecutableMode, strings.Join(reviewedExecutables, ","), strings.Join(resolvedExecutables, ","))
	}
	if member.Source.MetadataSHA256 != resolved.MetadataSHA256 {
		add(DriftMetadata, member.Source.MetadataSHA256, resolved.MetadataSHA256)
	}
	if member.Source.DependencyEvidenceSHA256 != resolved.DependencyEvidenceSHA256 {
		add(DriftDependencies, member.Source.DependencyEvidenceSHA256, resolved.DependencyEvidenceSHA256)
	}
	if member.Source.ExternalActionEvidenceSHA256 != resolved.ExternalActionEvidenceSHA256 {
		add(DriftExternalAction, member.Source.ExternalActionEvidenceSHA256, resolved.ExternalActionEvidenceSHA256)
	}
	review.Material = len(review.Changes) != 0
	return review
}

// Progress receives one user-facing line per setup step. It is the narrow seam
// that lets a resolver report what it is doing without knowing where the
// wizard's output goes; RunTerminal wires it to the wizard's output writer.
type Progress func(line string)

// ProgressReceiver is implemented by a MemberResolver that can report progress.
// RunTerminal offers every resolver a Progress and uses whatever it returns, so
// a resolver that does not report stays unchanged.
type ProgressReceiver interface {
	WithProgress(Progress) MemberResolver
}

type GitResolver struct {
	TempParent string
	progress   Progress
}

// WithProgress returns a copy of the resolver that reports one line per Source
// as it clones and inspects it, so a multi-second sequence of clones is never
// silent.
func (resolver GitResolver) WithProgress(progress Progress) MemberResolver {
	resolver.progress = progress
	return resolver
}

func (resolver GitResolver) report(format string, args ...any) {
	if resolver.progress == nil {
		return
	}
	resolver.progress(fmt.Sprintf(format, args...))
}

func (resolver GitResolver) ResolveMembers(ctx context.Context, members []catalog.Member) ([]ResolvedMember, func(), error) {
	groups := make(map[string][]catalog.Member)
	order := make([]string, 0)
	for _, member := range members {
		if _, exists := groups[member.Source.Repository]; !exists {
			order = append(order, member.Source.Repository)
		}
		groups[member.Source.Repository] = append(groups[member.Source.Repository], member)
	}
	var resolved []ResolvedMember
	var cleanups []func()
	cleanupAll := func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}
	for index, repository := range order {
		resolver.report("Source %d/%d %s — cloning", index+1, len(order), repository)
		repoDir, revision, cleanup, err := resolver.cloneLatest(ctx, repository)
		if err != nil {
			cleanupAll()
			return nil, func() {}, err
		}
		cleanups = append(cleanups, cleanup)
		resolver.report("Source %d/%d %s — inspecting %d member(s) at %s", index+1, len(order), repository, len(groups[repository]), shortRevision(revision))
		for _, member := range groups[repository] {
			item, err := InspectBoundary(member, repoDir, revision)
			if err != nil {
				cleanupAll()
				return nil, func() {}, err
			}
			item.Drift = CompareDrift(member, item.Evidence)
			resolved = append(resolved, item)
		}
	}
	return resolved, cleanupAll, nil
}

func (resolver GitResolver) Resolve(ctx context.Context, member catalog.Member) (ResolvedMember, func(), error) {
	repoDir, revision, cleanup, err := resolver.cloneLatest(ctx, member.Source.Repository)
	if err != nil {
		return ResolvedMember{}, func() {}, err
	}
	resolved, err := InspectBoundary(member, repoDir, revision)
	if err != nil {
		cleanup()
		return ResolvedMember{}, func() {}, err
	}
	resolved.Drift = CompareDrift(member, resolved.Evidence)
	return resolved, cleanup, nil
}

func (resolver GitResolver) cloneLatest(ctx context.Context, repository string) (string, string, func(), error) {
	tempParent := resolver.TempParent
	if tempParent == "" {
		tempParent = os.TempDir()
	}
	repoDir, err := os.MkdirTemp(tempParent, "skillet-catalog-resolve-")
	if err != nil {
		return "", "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(repoDir) }
	command := exec.CommandContext(ctx, "git", "clone", "--filter=blob:none", "--depth=1", "--no-tags", repository, repoDir)
	if output, cloneErr := command.CombinedOutput(); cloneErr != nil {
		cleanup()
		return "", "", func() {}, fmt.Errorf("resolve latest %s: %w: %s", repository, cloneErr, strings.TrimSpace(string(output)))
	}
	revisionOutput, err := exec.CommandContext(ctx, "git", "-C", repoDir, "rev-parse", "HEAD").Output()
	if err != nil {
		cleanup()
		return "", "", func() {}, fmt.Errorf("read resolved revision: %w", err)
	}
	return repoDir, strings.TrimSpace(string(revisionOutput)), cleanup, nil
}

func InspectBoundary(member catalog.Member, repositoryRoot, revision string) (ResolvedMember, error) {
	sourceDir := filepath.Join(repositoryRoot, filepath.FromSlash(member.Source.Subpath))
	relative, err := filepath.Rel(repositoryRoot, sourceDir)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return ResolvedMember{}, fmt.Errorf("source boundary %q escapes repository", member.Source.Subpath)
	}
	files, modes, err := readBoundaryWithModes(sourceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return ResolvedMember{}, fmt.Errorf("source boundary %q was deleted or renamed", member.Source.Subpath)
		}
		return ResolvedMember{}, err
	}
	licenseFile := filepath.Join(repositoryRoot, filepath.FromSlash(member.License.Notice))
	licenseBytes, err := os.ReadFile(licenseFile)
	if err != nil {
		return ResolvedMember{}, fmt.Errorf("read license evidence %q: %w", member.License.Notice, err)
	}
	scripts := make([]string, 0)
	executables := make([]string, 0)
	for name := range files {
		if isScriptSurface(name) {
			scripts = append(scripts, name)
			if modes[name]&0o111 != 0 {
				executables = append(executables, name)
			}
		}
	}
	sort.Strings(scripts)
	sort.Strings(executables)
	evidence := BoundaryEvidence{
		Revision: revision, Subpath: member.Source.Subpath, ContentSHA256: hashFiles(files),
		LicenseSHA256: hashBytes(licenseBytes), Scripts: scripts, Executables: executables,
		MetadataSHA256:               operationalEvidenceHash(files, []string{"name:", "description:", "disable-model-invocation:", "allowed-tools:", "user-invocable:"}),
		DependencyEvidenceSHA256:     operationalEvidenceHash(files, dependencyEvidenceTokens),
		ExternalActionEvidenceSHA256: operationalEvidenceHash(files, externalActionEvidenceTokens),
	}
	return ResolvedMember{Member: member, SourceDir: sourceDir, Evidence: evidence}, nil
}

var dependencyEvidenceTokens = []string{"git ", "gh ", "npm ", "npx ", "python", "playwright", "dotnet", "curl", "wget", "download", "network", "browser"}
var externalActionEvidenceTokens = []string{"issue create", "publish", "git commit", "git push", "create a branch", "write", "edit", "create", "save", "generate", "download", "install", "browser", "screenshot"}

func operationalEvidenceHash(files map[string][]byte, tokens []string) string {
	var evidence []string
	for name, contents := range files {
		if !isOperationalText(name) {
			continue
		}
		for lineNumber, line := range strings.Split(strings.ToLower(string(contents)), "\n") {
			trimmed := strings.TrimSpace(line)
			for _, token := range tokens {
				if strings.Contains(trimmed, token) {
					evidence = append(evidence, fmt.Sprintf("%s:%d:%s", name, lineNumber+1, trimmed))
					break
				}
			}
		}
	}
	sort.Strings(evidence)
	return hashBytes([]byte(strings.Join(evidence, "\n")))
}

func isOperationalText(name string) bool {
	extension := strings.ToLower(filepath.Ext(name))
	if extension == "" {
		return true
	}
	for _, candidate := range []string{".md", ".txt", ".yaml", ".yml", ".json", ".py", ".sh", ".js", ".mjs", ".cjs", ".ts", ".rb", ".ps1"} {
		if extension == candidate {
			return true
		}
	}
	return false
}

func isScriptSurface(name string) bool {
	lower := strings.ToLower(filepath.ToSlash(name))
	if strings.Contains("/"+lower, "/script/") || strings.Contains("/"+lower, "/scripts/") || strings.Contains("/"+lower, "/bin/") {
		return true
	}
	for _, extension := range []string{".py", ".sh", ".js", ".mjs", ".cjs", ".ts", ".rb", ".ps1"} {
		if strings.HasSuffix(lower, extension) {
			return true
		}
	}
	return false
}

func hashBytes(contents []byte) string {
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}
