package engine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

func TestSetManualOnlyPersonalSkillEditsFrontmatterPreservingRest(t *testing.T) {
	f := newFixture(t)
	folder := writeSkill(t, filepath.Join(f.roots.ClaudeHome, "skills", "loop-me"), "loop-me", "Loop description", "version: \"1.0.0\"\n")
	e := engine.New(f.roots)

	skill, ok := findSkill(e.Inventory(), engine.SourcePersonal, "loop-me")
	if !ok {
		t.Fatalf("personal skill not found")
	}
	if skill.Activation != engine.ActivationAuto {
		t.Fatalf("activation before = %q, want Auto", skill.Activation)
	}

	if err := e.SetManualOnly(skill, true); err != nil {
		t.Fatalf("SetManualOnly(true): %v", err)
	}

	skillMDPath := filepath.Join(folder, "SKILL.md")
	data, err := os.ReadFile(skillMDPath)
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "disable-model-invocation: true") {
		t.Fatalf("SKILL.md missing disable-model-invocation edit:\n%s", content)
	}
	if !strings.Contains(content, `name: "loop-me"`) {
		t.Fatalf("SKILL.md lost its name field:\n%s", content)
	}
	if !strings.Contains(content, `description: "Loop description"`) {
		t.Fatalf("SKILL.md lost its description field:\n%s", content)
	}
	if !strings.Contains(content, `version: "1.0.0"`) {
		t.Fatalf("SKILL.md lost an unrelated existing field:\n%s", content)
	}
	if !strings.Contains(content, "Body\n") {
		t.Fatalf("SKILL.md lost its body:\n%s", content)
	}

	inv := e.Inventory()
	manualOnly, ok := findSkill(inv, engine.SourcePersonal, "loop-me")
	if !ok {
		t.Fatalf("personal skill not found after SetManualOnly")
	}
	if manualOnly.Activation != engine.ActivationManualOnly {
		t.Fatalf("activation after SetManualOnly(true) = %q, want Manual-only", manualOnly.Activation)
	}
}

func TestSetManualOnlyPersonalSkillRoundTrip(t *testing.T) {
	f := newFixture(t)
	folder := writeSkill(t, filepath.Join(f.roots.ClaudeHome, "skills", "loop-me"), "loop-me", "Loop description", "version: \"1.0.0\"\n")
	skillMDPath := filepath.Join(folder, "SKILL.md")
	before, err := os.ReadFile(skillMDPath)
	if err != nil {
		t.Fatalf("read SKILL.md before: %v", err)
	}

	e := engine.New(f.roots)
	skill, _ := findSkill(e.Inventory(), engine.SourcePersonal, "loop-me")

	if err := e.SetManualOnly(skill, true); err != nil {
		t.Fatalf("SetManualOnly(true): %v", err)
	}
	manualOnly, _ := findSkill(e.Inventory(), engine.SourcePersonal, "loop-me")
	if err := e.SetManualOnly(manualOnly, false); err != nil {
		t.Fatalf("SetManualOnly(false): %v", err)
	}

	after, err := os.ReadFile(skillMDPath)
	if err != nil {
		t.Fatalf("read SKILL.md after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("SKILL.md not byte-identical after round trip:\nbefore=%q\nafter=%q", before, after)
	}

	reverted, ok := findSkill(e.Inventory(), engine.SourcePersonal, "loop-me")
	if !ok {
		t.Fatalf("personal skill not found after round trip")
	}
	if reverted.Activation != engine.ActivationAuto {
		t.Fatalf("activation after round trip = %q, want Auto", reverted.Activation)
	}
}

func TestSetManualOnlyCodexSkillCreatesPolicyFileWhenAbsent(t *testing.T) {
	f := newFixture(t)
	folder := writeSkill(t, filepath.Join(f.roots.AgentsHome, "skills", "codex-loop"), "codex-loop", "Codex loop description", "")
	e := engine.New(f.roots)

	skill, ok := findSkill(e.Inventory(), engine.SourceCodex, "codex-loop")
	if !ok {
		t.Fatalf("codex skill not found")
	}
	if skill.Activation != engine.ActivationAuto {
		t.Fatalf("activation before = %q, want Auto", skill.Activation)
	}

	policyPath := filepath.Join(folder, "agents", "openai.yaml")
	if _, err := os.Stat(policyPath); !os.IsNotExist(err) {
		t.Fatalf("test setup invalid: policy file already exists")
	}

	if err := e.SetManualOnly(skill, true); err != nil {
		t.Fatalf("SetManualOnly(true): %v", err)
	}

	data, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("read policy file: %v", err)
	}
	if !strings.Contains(string(data), "allow_implicit_invocation: false") {
		t.Fatalf("policy file missing allow_implicit_invocation: false:\n%s", data)
	}

	manualOnly, ok := findSkill(e.Inventory(), engine.SourceCodex, "codex-loop")
	if !ok {
		t.Fatalf("codex skill not found after SetManualOnly")
	}
	if manualOnly.Activation != engine.ActivationManualOnly {
		t.Fatalf("activation after SetManualOnly(true) = %q, want Manual-only", manualOnly.Activation)
	}
}

func TestSetManualOnlyCodexSkillRoundTripDeletesCreatedFile(t *testing.T) {
	f := newFixture(t)
	folder := writeSkill(t, filepath.Join(f.roots.AgentsHome, "skills", "codex-loop"), "codex-loop", "Codex loop description", "")
	policyPath := filepath.Join(folder, "agents", "openai.yaml")
	e := engine.New(f.roots)

	skill, _ := findSkill(e.Inventory(), engine.SourceCodex, "codex-loop")
	if err := e.SetManualOnly(skill, true); err != nil {
		t.Fatalf("SetManualOnly(true): %v", err)
	}
	if _, err := os.Stat(policyPath); err != nil {
		t.Fatalf("policy file should exist after SetManualOnly(true): %v", err)
	}

	manualOnly, _ := findSkill(e.Inventory(), engine.SourceCodex, "codex-loop")
	if err := e.SetManualOnly(manualOnly, false); err != nil {
		t.Fatalf("SetManualOnly(false): %v", err)
	}

	if _, err := os.Stat(policyPath); !os.IsNotExist(err) {
		t.Fatalf("policy file should not exist after round trip (Skillet created it from scratch), stat err = %v", err)
	}

	reverted, ok := findSkill(e.Inventory(), engine.SourceCodex, "codex-loop")
	if !ok {
		t.Fatalf("codex skill not found after round trip")
	}
	if reverted.Activation != engine.ActivationAuto {
		t.Fatalf("activation after round trip = %q, want Auto", reverted.Activation)
	}
}

func TestSetManualOnlyCodexSkillPreservesExistingPolicyFileContent(t *testing.T) {
	f := newFixture(t)
	folder := writeSkill(t, filepath.Join(f.roots.AgentsHome, "skills", "codex-loop"), "codex-loop", "Codex loop description", "")
	policyPath := filepath.Join(folder, "agents", "openai.yaml")
	writeFile(t, policyPath, "policy:\n  something_else: true\n")

	e := engine.New(f.roots)
	skill, _ := findSkill(e.Inventory(), engine.SourceCodex, "codex-loop")

	if err := e.SetManualOnly(skill, true); err != nil {
		t.Fatalf("SetManualOnly(true): %v", err)
	}
	data, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("read policy file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "allow_implicit_invocation: false") {
		t.Fatalf("policy file missing allow_implicit_invocation: false:\n%s", content)
	}
	if !strings.Contains(content, "something_else: true") {
		t.Fatalf("policy file lost unrelated existing key:\n%s", content)
	}

	manualOnly, _ := findSkill(e.Inventory(), engine.SourceCodex, "codex-loop")
	if err := e.SetManualOnly(manualOnly, false); err != nil {
		t.Fatalf("SetManualOnly(false): %v", err)
	}

	after, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("read policy file after round trip: %v", err)
	}
	if string(after) != "policy:\n  something_else: true\n" {
		t.Fatalf("policy file not restored to original content: %q", after)
	}

	reverted, ok := findSkill(e.Inventory(), engine.SourceCodex, "codex-loop")
	if !ok {
		t.Fatalf("codex skill not found after round trip")
	}
	if reverted.Activation != engine.ActivationAuto {
		t.Fatalf("activation after round trip = %q, want Auto", reverted.Activation)
	}
}

func TestSetManualOnlyCodexSkillPreservesInterfaceWrapperShape(t *testing.T) {
	f := newFixture(t)
	folder := writeSkill(t, filepath.Join(f.roots.AgentsHome, "skills", "codex-loop"), "codex-loop", "Codex loop description", "")
	policyPath := filepath.Join(folder, "agents", "openai.yaml")
	writeFile(t, policyPath, "interface:\n  policy:\n    something_else: true\n")

	e := engine.New(f.roots)
	skill, _ := findSkill(e.Inventory(), engine.SourceCodex, "codex-loop")

	if err := e.SetManualOnly(skill, true); err != nil {
		t.Fatalf("SetManualOnly(true): %v", err)
	}
	data, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("read policy file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "interface:") {
		t.Fatalf("policy file lost interface: wrapper:\n%s", content)
	}
	if !strings.Contains(content, "allow_implicit_invocation: false") {
		t.Fatalf("policy file missing allow_implicit_invocation: false:\n%s", content)
	}
	if !strings.Contains(content, "something_else: true") {
		t.Fatalf("policy file lost unrelated existing key:\n%s", content)
	}

	manualOnly, ok := findSkill(e.Inventory(), engine.SourceCodex, "codex-loop")
	if !ok {
		t.Fatalf("codex skill not found")
	}
	if manualOnly.Activation != engine.ActivationManualOnly {
		t.Fatalf("activation after SetManualOnly(true) with interface wrapper = %q, want Manual-only", manualOnly.Activation)
	}

	if err := e.SetManualOnly(manualOnly, false); err != nil {
		t.Fatalf("SetManualOnly(false): %v", err)
	}
	after, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("read policy file after round trip: %v", err)
	}
	if string(after) != "interface:\n  policy:\n    something_else: true\n" {
		t.Fatalf("policy file not restored to original content: %q", after)
	}
}

func TestSetManualOnlyProjectClaudeCodeSkillDispatchesToPersonalMechanism(t *testing.T) {
	f := newFixture(t)
	folder := writeSkill(t, filepath.Join(f.root, "repo", ".claude", "skills", "project-claude"), "project-claude", "Project Claude description", "version: \"1.0.0\"\n")
	skillMDPath := filepath.Join(folder, "SKILL.md")
	before, err := os.ReadFile(skillMDPath)
	if err != nil {
		t.Fatalf("read SKILL.md before: %v", err)
	}

	e := engine.New(f.roots)
	skill := engine.Skill{
		Name:     "project-claude",
		Source:   engine.SourceProject,
		Tool:     engine.ToolClaudeCode,
		Kind:     engine.KindSkill,
		Location: folder,
	}
	if err := e.SetManualOnly(skill, true); err != nil {
		t.Fatalf("SetManualOnly(true): %v", err)
	}
	data, err := os.ReadFile(skillMDPath)
	if err != nil {
		t.Fatalf("read SKILL.md after SetManualOnly(true): %v", err)
	}
	if !strings.Contains(string(data), "disable-model-invocation: true") {
		t.Fatalf("SKILL.md missing disable-model-invocation edit:\n%s", data)
	}

	if err := e.SetManualOnly(skill, false); err != nil {
		t.Fatalf("SetManualOnly(false): %v", err)
	}
	after, err := os.ReadFile(skillMDPath)
	if err != nil {
		t.Fatalf("read SKILL.md after round trip: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("SKILL.md not byte-identical after round trip:\nbefore=%q\nafter=%q", before, after)
	}
}

func TestSetManualOnlyProjectCodexSkillDispatchesToCodexMechanism(t *testing.T) {
	f := newFixture(t)
	folder := writeSkill(t, filepath.Join(f.root, "repo", ".agents", "skills", "project-codex"), "project-codex", "Project Codex description", "")
	policyPath := filepath.Join(folder, "agents", "openai.yaml")

	e := engine.New(f.roots)
	skill := engine.Skill{
		Name:     "project-codex",
		Source:   engine.SourceProject,
		Tool:     engine.ToolCodex,
		Kind:     engine.KindSkill,
		Location: folder,
	}
	if err := e.SetManualOnly(skill, true); err != nil {
		t.Fatalf("SetManualOnly(true): %v", err)
	}
	data, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("read policy file: %v", err)
	}
	if !strings.Contains(string(data), "allow_implicit_invocation: false") {
		t.Fatalf("policy file missing allow_implicit_invocation: false:\n%s", data)
	}

	if err := e.SetManualOnly(skill, false); err != nil {
		t.Fatalf("SetManualOnly(false): %v", err)
	}
	if _, err := os.Stat(policyPath); !os.IsNotExist(err) {
		t.Fatalf("policy file should not exist after round trip, stat err = %v", err)
	}
}

func TestSetManualOnlyRejectsCodexCustomPrompt(t *testing.T) {
	f := newFixture(t)
	writePrompt(t, filepath.Join(f.roots.CodexHome, "prompts", "ideas.md"), "Ideas prompt")
	e := engine.New(f.roots)

	skill, ok := findSkill(e.Inventory(), engine.SourceCodex, "ideas")
	if !ok {
		t.Fatalf("prompt not found")
	}
	if err := e.SetManualOnly(skill, true); err == nil {
		t.Fatalf("SetManualOnly on a Codex custom prompt should return an error")
	}
}

func TestSetManualOnlyRejectsPluginSkill(t *testing.T) {
	f := newFixture(t)
	installPath := filepath.Join(f.root, "plugin-cache", "marketplace-x", "plugin-x", "v1")
	writeSkill(t, filepath.Join(installPath, "skills", "plug-me"), "plug-me", "Plugin description", "")
	writePluginManifest(t, f.roots.ClaudeHome, map[string][]map[string]string{
		"plugin-x@marketplace-x": {
			{"scope": "user", "installPath": installPath, "version": "1.0.0"},
		},
	})
	e := engine.New(f.roots)

	skill, ok := findSkill(e.Inventory(), engine.SourcePlugin, "plug-me")
	if !ok {
		t.Fatalf("plugin skill not found")
	}
	if err := e.SetManualOnly(skill, true); err == nil {
		t.Fatalf("SetManualOnly on a Plugin skill should return an error")
	}
}

// TestManualOnlyScanningNeverCreatesPolicyFilesSpeculatively guards against a
// regression where merely scanning (Inventory()) would create
// agents/openai.yaml for a Codex skill that has never had SetManualOnly
// called on it — reading activation state must stay pure, exactly like every
// other scan in this package (see TestSuppressReadOnlySessionLeavesTreeByteIdenticalWhenNoSuppressions
// for the equivalent guard on Suppress).
func TestManualOnlyScanningNeverCreatesPolicyFilesSpeculatively(t *testing.T) {
	f := newFixture(t)
	writeSkill(t, filepath.Join(f.roots.ClaudeHome, "skills", "alpha"), "alpha", "Alpha description", "")
	writeSkill(t, filepath.Join(f.roots.AgentsHome, "skills", "codex-loop"), "codex-loop", "Codex loop description", "")
	e := engine.New(f.roots)

	before := snapshotTree(t, f.root)
	_ = e.Inventory()
	_ = e.Inventory()
	after := snapshotTree(t, f.root)
	assertSnapshotsEqual(t, before, after)
}

// TestSetManualOnlyRefusesInlinePolicyValueRatherThanCorrupting pins the
// deliberate narrowing documented in manual_only.go: only block-mapping YAML
// (`policy:` alone on its line, children indented beneath) is understood.
// An inline/flow-style `policy:` value is a shape codexOpenAIActivation's
// read side would also fail to fully interpret as manual-only, and blindly
// appending a second top-level `policy:` key to "fix" it would produce a
// YAML file with a duplicate key — a real, if unlikely, corruption. Both
// SetManualOnly(true) and SetManualOnly(false) must refuse instead.
func TestSetManualOnlyRefusesInlinePolicyValueRatherThanCorrupting(t *testing.T) {
	f := newFixture(t)
	folder := writeSkill(t, filepath.Join(f.roots.AgentsHome, "skills", "codex-loop"), "codex-loop", "Codex loop description", "")
	policyPath := filepath.Join(folder, "agents", "openai.yaml")
	writeFile(t, policyPath, "policy: {}\n")

	e := engine.New(f.roots)
	skill, _ := findSkill(e.Inventory(), engine.SourceCodex, "codex-loop")

	if err := e.SetManualOnly(skill, true); err == nil {
		t.Fatalf("SetManualOnly(true) on an inline policy value should return an error, not corrupt the file")
	}
	if err := e.SetManualOnly(skill, false); err == nil {
		t.Fatalf("SetManualOnly(false) on an inline policy value should return an error, not silently no-op")
	}

	after, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("read policy file: %v", err)
	}
	if string(after) != "policy: {}\n" {
		t.Fatalf("policy file was modified despite the refused edit: %q", after)
	}
}
