package catalog

import (
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed catalog.json
var builtInJSON []byte

// ErrUnknownBundle is returned by SelectBundles when a requested bundle ID
// does not exist in the catalog.
var ErrUnknownBundle = errors.New("unknown built-in catalog bundle")

type Catalog struct {
	SchemaVersion int      `json:"schemaVersion"`
	Version       string   `json:"version"`
	ReviewedDate  string   `json:"reviewedDate"`
	Members       []Member `json:"members"`
	Bundles       []Bundle `json:"bundles"`
}

type Member struct {
	Name               string       `json:"name"`
	Family             string       `json:"family"`
	Source             Source       `json:"source"`
	License            License      `json:"license"`
	UpstreamActivation string       `json:"upstreamActivation"`
	Dependencies       []Dependency `json:"dependencies"`
	Scripts            []string     `json:"scripts"`
	Executables        []string     `json:"executables"`
	ExternalActions    []string     `json:"externalActions"`
	VerificationPrompt string       `json:"verificationPrompt"`
	Recipes            []Recipe     `json:"recipes"`
}

type Source struct {
	Repository                   string `json:"repository"`
	Subpath                      string `json:"subpath"`
	ReviewedRevision             string `json:"reviewedRevision"`
	ReviewedDate                 string `json:"reviewedDate"`
	ContentSHA256                string `json:"contentSHA256"`
	MetadataSHA256               string `json:"metadataSHA256"`
	DependencyEvidenceSHA256     string `json:"dependencyEvidenceSHA256"`
	ExternalActionEvidenceSHA256 string `json:"externalActionEvidenceSHA256"`
}

type License struct {
	SPDX         string `json:"spdx"`
	Notice       string `json:"notice"`
	NoticeSHA256 string `json:"noticeSHA256"`
	Evidence     string `json:"evidence"`
}

type Dependency struct {
	Name     string `json:"name"`
	Optional bool   `json:"optional"`
	Reason   string `json:"reason"`
}

type Recipe struct {
	Tool     string   `json:"tool"`
	Scope    string   `json:"scope"`
	Artifact string   `json:"artifact"`
	Requires []string `json:"requires"`
}

type Bundle struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Members               []string `json:"members"`
	OverlapWarning        string   `json:"overlapWarning,omitempty"`
	RecommendationGlobs   []string `json:"recommendationGlobs,omitempty"`
	ExplicitSelectionOnly bool     `json:"explicitSelectionOnly,omitempty"`
}

type Selection struct {
	CatalogVersion string   `json:"catalogVersion"`
	Bundles        []Bundle `json:"bundles"`
	Members        []Member `json:"members"`
	Warnings       []string `json:"warnings"`
}

func Load() (Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(builtInJSON, &c); err != nil {
		return Catalog{}, fmt.Errorf("load built-in catalog: %w", err)
	}
	if err := c.Validate(); err != nil {
		return Catalog{}, err
	}
	return c, nil
}

func (c Catalog) Validate() error {
	if c.SchemaVersion != 1 {
		return fmt.Errorf("built-in catalog schema %d is unsupported", c.SchemaVersion)
	}
	if strings.TrimSpace(c.Version) == "" || strings.TrimSpace(c.ReviewedDate) == "" {
		return fmt.Errorf("built-in catalog version and reviewed date are required")
	}
	if len(c.Members) != 48 {
		return fmt.Errorf("built-in catalog has %d members; want exactly 48", len(c.Members))
	}
	members := make(map[string]Member, len(c.Members))
	for _, member := range c.Members {
		if member.Name == "" || member.Family == "" {
			return fmt.Errorf("catalog member name and family are required")
		}
		if _, exists := members[member.Name]; exists {
			return fmt.Errorf("duplicate catalog member %q", member.Name)
		}
		members[member.Name] = member
		if member.Source.Repository == "" || member.Source.Subpath == "" || strings.ContainsAny(member.Source.Subpath, "*?[") {
			return fmt.Errorf("catalog member %q has invalid source boundary", member.Name)
		}
		if !isHexDigest(member.Source.ReviewedRevision, 20) || !isHexDigest(member.Source.ContentSHA256, 32) ||
			!isHexDigest(member.Source.MetadataSHA256, 32) || !isHexDigest(member.Source.DependencyEvidenceSHA256, 32) ||
			!isHexDigest(member.Source.ExternalActionEvidenceSHA256, 32) {
			return fmt.Errorf("catalog member %q has invalid reviewed identity", member.Name)
		}
		if member.License.SPDX == "" || member.License.Notice == "" || !isHexDigest(member.License.NoticeSHA256, 32) || member.License.Evidence == "" {
			return fmt.Errorf("catalog member %q has incomplete license evidence", member.Name)
		}
		if member.UpstreamActivation != "auto" && member.UpstreamActivation != "manual-only" {
			return fmt.Errorf("catalog member %q has invalid upstream activation %q", member.Name, member.UpstreamActivation)
		}
		if strings.TrimSpace(member.VerificationPrompt) == "" {
			return fmt.Errorf("catalog member %q has no safe verification prompt", member.Name)
		}
		for _, executable := range member.Executables {
			if !contains(member.Scripts, executable) {
				return fmt.Errorf("catalog member %q executable %q is not a disclosed script", member.Name, executable)
			}
		}
		if len(member.Recipes) != 2 || !directSkillRecipe(member.Recipes[0], "claude-code") || !directSkillRecipe(member.Recipes[1], "codex") {
			return fmt.Errorf("catalog member %q must have exact Claude Code and Codex Project direct-skill recipes", member.Name)
		}
	}

	seenBundles := make(map[string]bool, len(c.Bundles))
	covered := make(map[string]int, len(c.Members))
	for _, bundle := range c.Bundles {
		if bundle.ID == "" || bundle.Name == "" || len(bundle.Members) == 0 || seenBundles[bundle.ID] {
			return fmt.Errorf("invalid or duplicate catalog bundle %q", bundle.ID)
		}
		seenBundles[bundle.ID] = true
		for _, name := range bundle.Members {
			if _, ok := members[name]; !ok {
				return fmt.Errorf("bundle %q references unknown member %q", bundle.ID, name)
			}
			covered[name]++
		}
	}
	for name, count := range covered {
		if count != 1 {
			return fmt.Errorf("catalog member %q appears in %d bundles; want exactly one", name, count)
		}
	}
	if len(covered) != len(c.Members) {
		return fmt.Errorf("catalog bundles cover %d of %d members", len(covered), len(c.Members))
	}
	return nil
}

func directSkillRecipe(recipe Recipe, tool string) bool {
	if recipe.Tool != tool || recipe.Scope != "project" || recipe.Artifact != "direct-skill" {
		return false
	}
	for _, requirement := range recipe.Requires {
		switch requirement {
		case "hooks", "commands", "agents", "mcp", "external-requirements":
		default:
			return false
		}
	}
	return true
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// GovernanceBlockers reports catalog evidence that is inspectable but not yet
// strong enough for a public readiness claim.
func (c Catalog) GovernanceBlockers() []string {
	var blockers []string
	for _, member := range c.Members {
		if member.License.Evidence != "license-text" {
			blockers = append(blockers, fmt.Sprintf("%s: %s license evidence is %s", member.Name, member.License.SPDX, member.License.Evidence))
		}
	}
	sort.Strings(blockers)
	return blockers
}

func (c Catalog) SelectBundles(ids []string) (Selection, error) {
	bundlesByID := make(map[string]Bundle, len(c.Bundles))
	membersByName := make(map[string]Member, len(c.Members))
	for _, bundle := range c.Bundles {
		bundlesByID[bundle.ID] = bundle
	}
	for _, member := range c.Members {
		membersByName[member.Name] = member
	}

	selection := Selection{CatalogVersion: c.Version}
	seenMember := make(map[string]bool)
	for _, id := range ids {
		bundle, ok := bundlesByID[id]
		if !ok {
			return Selection{}, fmt.Errorf("%w %q", ErrUnknownBundle, id)
		}
		selection.Bundles = append(selection.Bundles, bundle)
		for _, name := range bundle.Members {
			if !seenMember[name] {
				selection.Members = append(selection.Members, membersByName[name])
				seenMember[name] = true
			}
		}
	}
	return selection, nil
}

func (c Catalog) BundleIDs() []string {
	ids := make([]string, 0, len(c.Bundles))
	for _, bundle := range c.Bundles {
		ids = append(ids, bundle.ID)
	}
	sort.Strings(ids)
	return ids
}

// recommendationSkipDirs are directory names never worth traversing to spot a
// project-type marker file: dependency, virtualenv, and build-output trees.
// They routinely hold tens of thousands of files and contain no signal about
// what kind of project this is.
var recommendationSkipDirs = map[string]bool{
	".git": true, ".skillet": true,
	"node_modules": true, "vendor": true, "venv": true, ".venv": true,
	"target": true, "dist": true, "build": true, ".next": true,
}

const (
	// recommendationMaxDepth bounds how deep below the project root a marker
	// file is looked for. Solution and project files live at or very near the
	// top of a repository; anything deeper is a fixture or a vendored copy.
	recommendationMaxDepth = 5
	// recommendationFileBudget caps total work so setup cannot stall silently
	// on a large tree. Reaching it ends the scan with whatever was found.
	recommendationFileBudget = 20000
)

// RecommendedBundleIDs looks for project-type marker files below projectRoot.
// It exists solely to spot a .NET project, so the traversal is bounded three
// ways — skip list, depth cap, file budget — and stops the moment it has an
// answer. Before this it was an unbounded walk of the entire chosen directory,
// node_modules and all, on the setup path with no progress output.
func (c Catalog) RecommendedBundleIDs(projectRoot string) []string {
	recommended := make(map[string]bool)
	rootDepth := strings.Count(filepath.Clean(projectRoot), string(filepath.Separator))
	budget := recommendationFileBudget
	stop := errors.New("recommendation scan complete")

	walkErr := filepath.WalkDir(projectRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			if path == projectRoot {
				return nil
			}
			if recommendationSkipDirs[strings.ToLower(entry.Name())] {
				return filepath.SkipDir
			}
			if strings.Count(filepath.Clean(path), string(filepath.Separator))-rootDepth >= recommendationMaxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		budget--
		if budget <= 0 {
			return stop
		}
		name := strings.ToLower(entry.Name())
		if name == "global.json" || strings.HasSuffix(name, ".csproj") || strings.HasSuffix(name, ".sln") || strings.HasSuffix(name, ".slnx") {
			recommended["dotnet-starter"] = true
			// Every marker feeds the same single recommendation, so there is
			// nothing left to learn from the rest of the tree.
			return stop
		}
		return nil
	})
	_ = walkErr

	ids := make([]string, 0, len(recommended))
	for id := range recommended {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func isHexDigest(value string, bytes int) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == bytes
}
