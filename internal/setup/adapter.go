package setup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jnyross/Skill-Manager/internal/catalog"
	"gopkg.in/yaml.v3"
)

type Activation string

const (
	ActivationAuto       Activation = "auto"
	ActivationManualOnly Activation = "manual-only"
)

type Capabilities struct {
	DirectSkillProject   bool `json:"directSkillProject"`
	ActivationAuto       bool `json:"activationAuto"`
	ActivationManualOnly bool `json:"activationManualOnly"`
	StaticValidation     bool `json:"staticValidation"`
	RuntimeExplicitProbe bool `json:"runtimeExplicitProbe"`
	ExecutableHealth     bool `json:"executableHealth"`
	AuthenticationHealth bool `json:"authenticationHealth"`
	RollbackManagedFiles bool `json:"rollbackManagedFiles"`
	ProjectPlugin        bool `json:"projectPlugin"`
	Hooks                bool `json:"hooks"`
	Commands             bool `json:"commands"`
	Agents               bool `json:"agents"`
	MCP                  bool `json:"mcp"`
	ExternalRequirements bool `json:"externalRequirements"`
}

type RenderRequest struct {
	Member             catalog.Member
	SourceDir          string
	Activation         Activation
	ActivationOverride bool

	// boundaries, when set, supplies SourceDir's already-read contents from
	// the current setup run instead of walking and slurping the directory
	// again. Unexported on purpose: an externally constructed RenderRequest
	// leaves it nil and reads through, which is always correct.
	boundaries *boundaryCache
}

type RenderedView struct {
	Tool                   string            `json:"tool"`
	RelativeDestination    string            `json:"relativeDestination"`
	Files                  map[string][]byte `json:"-"`
	FileModes              map[string]uint32 `json:"fileModes"`
	CanonicalContentSHA256 string            `json:"canonicalContentSHA256"`
	RenderedContentSHA256  string            `json:"renderedContentSHA256"`
	RequestedActivation    Activation        `json:"requestedActivation"`
	Warnings               []string          `json:"warnings,omitempty"`
}

type ToolAdapter interface {
	Tool() string
	Capabilities() Capabilities
	Render(RenderRequest) (RenderedView, error)
}

type directSkillAdapter struct {
	tool            string
	destinationRoot string
}

func NewClaudeAdapter() ToolAdapter {
	return directSkillAdapter{tool: "claude-code", destinationRoot: ".claude/skills"}
}

func NewCodexAdapter() ToolAdapter {
	return directSkillAdapter{tool: "codex", destinationRoot: ".agents/skills"}
}

func (a directSkillAdapter) Tool() string { return a.tool }

func (a directSkillAdapter) Capabilities() Capabilities {
	return Capabilities{
		DirectSkillProject: true, ActivationAuto: true, ActivationManualOnly: true,
		StaticValidation: true, RuntimeExplicitProbe: true, ExecutableHealth: true,
		AuthenticationHealth: true, RollbackManagedFiles: true, ProjectPlugin: false,
		Hooks: false, Commands: false, Agents: false, MCP: false, ExternalRequirements: true,
	}
}

func (a directSkillAdapter) Render(request RenderRequest) (RenderedView, error) {
	if err := validateRecipe(request.Member, a.tool); err != nil {
		return RenderedView{}, err
	}
	files, modes, err := request.boundaries.read(request.SourceDir)
	if err != nil {
		return RenderedView{}, fmt.Errorf("read %s source boundary: %w", request.Member.Name, err)
	}
	if _, ok := files["SKILL.md"]; !ok {
		return RenderedView{}, fmt.Errorf("%s source boundary has no SKILL.md", request.Member.Name)
	}
	canonicalHash := hashFiles(files)
	rendered := cloneFiles(files)
	if request.ActivationOverride || (a.tool == "codex" && request.Activation == ActivationManualOnly) {
		switch request.Activation {
		case ActivationAuto, ActivationManualOnly:
		default:
			return RenderedView{}, fmt.Errorf("%s activation %q is unsupported", a.tool, request.Activation)
		}
		manualOnly := request.Activation == ActivationManualOnly
		if a.tool == "claude-code" {
			rendered["SKILL.md"], err = setFrontmatterBool(rendered["SKILL.md"], "disable-model-invocation", manualOnly)
		} else {
			rendered["agents/openai.yaml"], err = setCodexActivation(rendered["agents/openai.yaml"], !manualOnly)
		}
		if err != nil {
			return RenderedView{}, fmt.Errorf("render %s activation for %s: %w", a.tool, request.Member.Name, err)
		}
	}

	view := RenderedView{
		Tool: a.tool, RelativeDestination: filepath.ToSlash(filepath.Join(a.destinationRoot, request.Member.Name)),
		Files: rendered, FileModes: modes, CanonicalContentSHA256: canonicalHash, RenderedContentSHA256: hashFiles(rendered),
		RequestedActivation: request.Activation,
	}
	if a.tool == "codex" && hasClaudeSpecificFrontmatter(rendered["SKILL.md"]) {
		view.Warnings = append(view.Warnings, "Claude-specific frontmatter is retained as canonical source evidence but is not a Codex activation mechanism")
	}
	return view, nil
}

func validateRecipe(member catalog.Member, tool string) error {
	for _, recipe := range member.Recipes {
		if recipe.Tool != tool {
			continue
		}
		if recipe.Scope != "project" || recipe.Artifact != "direct-skill" {
			return fmt.Errorf("%s requires unsupported %s/%s recipe for %s", member.Name, recipe.Scope, recipe.Artifact, tool)
		}
		capabilities := (directSkillAdapter{tool: tool}).Capabilities()
		for _, requirement := range recipe.Requires {
			supported := map[string]bool{
				"hooks": capabilities.Hooks, "commands": capabilities.Commands, "agents": capabilities.Agents,
				"mcp": capabilities.MCP, "external-requirements": capabilities.ExternalRequirements,
			}[requirement]
			if !supported {
				return fmt.Errorf("%s requires unsupported %s capability for %s", member.Name, requirement, tool)
			}
		}
		return nil
	}
	return fmt.Errorf("%s has no recipe for %s", member.Name, tool)
}

func readBoundary(root string) (map[string][]byte, error) {
	files, _, err := readBoundaryWithModes(root)
	return files, err
}

// boundaryReadHook is a test-only seam: when non-nil it is called with the
// root of every boundary actually read from disk, so a test can assert how
// many reads a setup run performs. Tests that install it must not run in
// parallel.
var boundaryReadHook func(root string)

func readBoundaryWithModes(root string) (map[string][]byte, map[string]uint32, error) {
	if boundaryReadHook != nil {
		boundaryReadHook(root)
	}
	files := make(map[string][]byte)
	modes := make(map[string]uint32)
	err := filepath.WalkDir(root, func(filename string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filename == root {
			return nil
		}
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("source boundary contains unsupported symlink %s", filepath.ToSlash(relative))
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("source boundary contains unsupported file %s", filepath.ToSlash(relative))
		}
		contents, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		files[name] = contents
		info, err := entry.Info()
		if err != nil {
			return err
		}
		modes[name] = uint32(info.Mode().Perm())
		return nil
	})
	return files, modes, err
}

func hashFiles(files map[string][]byte) string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	hash := sha256.New()
	for _, name := range names {
		hash.Write([]byte(name))
		hash.Write([]byte{0})
		hash.Write(files[name])
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func cloneFiles(files map[string][]byte) map[string][]byte {
	cloned := make(map[string][]byte, len(files))
	for name, contents := range files {
		cloned[name] = append([]byte(nil), contents...)
	}
	return cloned
}

func setFrontmatterBool(source []byte, key string, value bool) ([]byte, error) {
	text := string(source)
	if !strings.HasPrefix(text, "---\n") {
		return nil, fmt.Errorf("SKILL.md has no YAML frontmatter")
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		return nil, fmt.Errorf("SKILL.md frontmatter is not closed")
	}
	end += 4
	frontmatter := text[4:end]
	lines := strings.Split(frontmatter, "\n")
	replacement := fmt.Sprintf("%s: %t", key, value)
	found := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), key+":") {
			lines[i] = replacement
			found = true
		}
	}
	if !found {
		lines = append(lines, replacement)
	}
	return []byte("---\n" + strings.Join(lines, "\n") + text[end:]), nil
}

func setCodexActivation(existing []byte, allowImplicit bool) ([]byte, error) {
	document := make(map[string]any)
	if len(existing) != 0 {
		if err := yaml.Unmarshal(existing, &document); err != nil {
			return nil, fmt.Errorf("parse agents/openai.yaml: %w", err)
		}
	}
	policy, ok := document["policy"].(map[string]any)
	if !ok {
		policy = make(map[string]any)
		document["policy"] = policy
	}
	policy["allow_implicit_invocation"] = allowImplicit
	return yaml.Marshal(document)
}

func hasClaudeSpecificFrontmatter(source []byte) bool {
	text := string(source)
	return strings.Contains(text, "disable-model-invocation:") || strings.Contains(text, "allowed-tools:") || strings.Contains(text, "user-invocable:")
}
