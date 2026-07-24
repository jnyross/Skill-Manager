package main

// Tests for the scriptable command surface. Every test runs against a fixture
// home directory and a fixture working directory, so nothing here can read or
// write the real ~/.claude, ~/.codex, ~/.agents, or ~/.skillet.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// packageDir is the package's own directory, captured before any test changes
// the working directory, so golden files resolve no matter where a test runs.
var packageDir = func() string {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return dir
}()

// fixture is one throwaway machine: a home directory holding Personal, Plugin
// and Codex Sources, plus a project directory holding Project Sources.
type fixture struct {
	home    string
	project string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	project := filepath.Join(root, "project")

	// Personal Skills (Claude Code, user level).
	writeSkillFile(t, filepath.Join(home, ".claude", "skills", "writing"), "writing", "Write clear prose.", "")
	writeSkillFile(t, filepath.Join(home, ".claude", "skills", "review"), "review", "Review a diff.", "disable-model-invocation: true\n")

	// Plugin Skill (Claude Code, bundled inside an installed plugin).
	pluginInstall := filepath.Join(home, ".claude", "plugins", "cache", "demo")
	writeSkillFile(t, filepath.Join(pluginInstall, "skills", "lint"), "lint", "Lint the tree.", "")
	writeFile(t, filepath.Join(home, ".claude", "plugins", "installed_plugins.json"),
		`{"plugins":{"demo@market":[{"scope":"user","installPath":`+quote(pluginInstall)+`}]}}`+"\n")

	// Codex Skill (user level) and a Codex custom prompt.
	writeSkillFile(t, filepath.Join(home, ".agents", "skills", "refactor"), "refactor", "Refactor safely.", "")
	writeFile(t, filepath.Join(home, ".codex", "prompts", "deploy.md"), "---\ndescription: Deploy the service.\n---\n\nDeploy.\n")

	// Project Skills, one per Tool, deliberately sharing the name "review"
	// with the Personal Skill above so ambiguity is exercised.
	writeSkillFile(t, filepath.Join(project, ".claude", "skills", "review"), "review", "Review this repo's diff.", "")
	writeSkillFile(t, filepath.Join(project, ".agents", "skills", "review"), "review", "Review this repo, Codex side.", "")

	t.Setenv("HOME", home)
	t.Chdir(project)
	return fixture{home: home, project: project}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeSkillFile(t *testing.T, dir, name, description, extraFrontmatter string) {
	t.Helper()
	content := "---\nname: " + name + "\ndescription: " + description + "\n" + extraFrontmatter + "---\n\nBody.\n"
	writeFile(t, filepath.Join(dir, "SKILL.md"), content)
}

func quote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

// runCLI invokes the command tree exactly as main does, with an empty stdin.
func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runWithInput(args, strings.NewReader(""), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// normalize replaces machine-specific absolute paths with stable placeholders
// so golden files survive a different temp directory.
func normalize(t *testing.T, f fixture, output string) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	replacements := []struct{ from, to string }{
		{cwd, "$PROJECT"},
		{f.project, "$PROJECT"},
		{f.home, "$HOME"},
	}
	for _, replacement := range replacements {
		output = strings.ReplaceAll(output, replacement.from, replacement.to)
	}
	// The home fixture lives beside the project fixture, so also normalize the
	// resolved (symlink-free) form macOS reports for the working directory.
	return output
}

func TestListJSONMatchesGoldenFile(t *testing.T) {
	f := newFixture(t)
	code, stdout, stderr := runCLI(t, "list", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	got := normalize(t, f, stdout)

	golden := filepath.Join(packageDir, "testdata", "list.json")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		writeFile(t, golden, got)
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("list --json output differs from %s\n--- got ---\n%s\n--- want ---\n%s", golden, got, want)
	}
}

func TestListJSONIsValidAndCarriesTheDocumentedFields(t *testing.T) {
	newFixture(t)
	code, stdout, stderr := runCLI(t, "list", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	var document listJSON
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("list --json is not valid JSON: %v", err)
	}
	if document.SchemaVersion != jsonSchemaVersion {
		t.Fatalf("schemaVersion = %d, want %d", document.SchemaVersion, jsonSchemaVersion)
	}
	if len(document.Notices) == 0 {
		t.Fatal("notices are not represented in list --json")
	}
	byQualified := make(map[string]skillJSON, len(document.Skills))
	for _, skill := range document.Skills {
		byQualified[skill.QualifiedName] = skill
	}
	personal, ok := byQualified["Personal:review"]
	if !ok {
		t.Fatalf("Personal:review missing from %v", byQualified)
	}
	if personal.Activation != "Manual-only" || personal.Tool != "Claude Code" || personal.Kind != "skill" {
		t.Fatalf("Personal:review = %+v", personal)
	}
	plugin, ok := byQualified["Plugin:lint"]
	if !ok {
		t.Fatal("Plugin:lint missing")
	}
	if plugin.Plugin == nil || plugin.Plugin.Plugin != "demo" || plugin.Plugin.Marketplace != "market" || plugin.Plugin.SkillCount != 1 {
		t.Fatalf("Plugin:lint plugin block = %+v", plugin.Plugin)
	}
	if prompt, ok := byQualified["Codex:deploy"]; !ok || prompt.Kind != "prompt" {
		t.Fatalf("Codex:deploy = %+v (present %v)", prompt, ok)
	}
}

func TestListFiltersBySourceAndTool(t *testing.T) {
	newFixture(t)
	code, stdout, _ := runCLI(t, "list", "--source", "personal")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stdout, "writing") || strings.Contains(stdout, "refactor") {
		t.Fatalf("--source personal listed the wrong Skills:\n%s", stdout)
	}
	code, stdout, _ = runCLI(t, "list", "--tool", "codex", "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var document listJSON
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatal(err)
	}
	for _, skill := range document.Skills {
		if skill.Tool != "Codex" {
			t.Fatalf("--tool codex returned %s (%s)", skill.QualifiedName, skill.Tool)
		}
	}
	if len(document.Skills) == 0 {
		t.Fatal("--tool codex returned nothing")
	}
}

func TestListRejectsAnUnknownSourceAsAUsageError(t *testing.T) {
	newFixture(t)
	code, _, stderr := runCLI(t, "list", "--source", "nonsense")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "unknown Source") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestShowResolvesABareNameAndPrintsDetail(t *testing.T) {
	newFixture(t)
	code, stdout, stderr := runCLI(t, "show", "writing")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	for _, want := range []string{"Personal", "Claude Code", "Auto", "Write clear prose."} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("show output missing %q:\n%s", want, stdout)
		}
	}
}

func TestShowJSONCarriesTheSkillObject(t *testing.T) {
	newFixture(t)
	code, stdout, stderr := runCLI(t, "show", "Personal:writing", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	var document showJSON
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != jsonSchemaVersion || document.Skill.Name != "writing" || document.Skill.Source != "Personal" {
		t.Fatalf("show --json = %+v", document)
	}
}

func TestAmbiguousNameFailsWithQualifiedCandidates(t *testing.T) {
	newFixture(t)
	code, _, stderr := runCLI(t, "show", "review")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	for _, want := range []string{"Personal:review", "Project:claude-code:review", "Project:codex:review"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("candidate %q missing from:\n%s", want, stderr)
		}
	}
}

func TestQualifiedNamesFromTheAmbiguityMessageResolve(t *testing.T) {
	newFixture(t)
	for _, name := range []string{"Personal:review", "Project:claude-code:review", "Project:codex:review"} {
		code, stdout, stderr := runCLI(t, "show", name, "--json")
		if code != 0 {
			t.Fatalf("%s: exit = %d, stderr = %q", name, code, stderr)
		}
		var document showJSON
		if err := json.Unmarshal([]byte(stdout), &document); err != nil {
			t.Fatal(err)
		}
		if document.Skill.Name != "review" {
			t.Fatalf("%s resolved to %q", name, document.Skill.Name)
		}
	}
}

func TestUnknownNameIsAnOperationError(t *testing.T) {
	newFixture(t)
	code, _, stderr := runCLI(t, "show", "nope")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "no Skill matches") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestArchiveThenRestoreRoundTripsAPersonalSkill(t *testing.T) {
	f := newFixture(t)
	location := filepath.Join(f.home, ".claude", "skills", "writing")

	code, stdout, stderr := runCLI(t, "archive", "writing", "--yes")
	if code != 0 {
		t.Fatalf("archive exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "Archived Personal skill \"writing\"") {
		t.Fatalf("archive summary = %q", stdout)
	}
	if _, err := os.Stat(location); !os.IsNotExist(err) {
		t.Fatalf("archive left the Skill in place: %v", err)
	}

	id := archiveID(t, "writing")
	code, stdout, stderr = runCLI(t, "restore", id, "--yes")
	if code != 0 {
		t.Fatalf("restore exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "Restored Personal skill \"writing\"") {
		t.Fatalf("restore summary = %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(location, "SKILL.md")); err != nil {
		t.Fatalf("restore did not put the Skill back: %v", err)
	}
}

func TestArchiveRefusesWithoutYes(t *testing.T) {
	f := newFixture(t)
	code, _, stderr := runCLI(t, "archive", "writing")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--yes") {
		t.Fatalf("stderr = %q", stderr)
	}
	if _, err := os.Stat(filepath.Join(f.home, ".claude", "skills", "writing")); err != nil {
		t.Fatalf("refused archive still mutated the Skill: %v", err)
	}
}

func TestPurgeRefusesWithoutYesAndKeepsTheArchiveEntry(t *testing.T) {
	f := newFixture(t)
	if code, _, stderr := runCLI(t, "archive", "writing", "--yes"); code != 0 {
		t.Fatalf("archive exit = %d, stderr = %q", code, stderr)
	}
	id := archiveID(t, "writing")

	code, _, stderr := runCLI(t, "purge", id)
	if code != 2 {
		t.Fatalf("purge without --yes exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--yes") {
		t.Fatalf("stderr = %q", stderr)
	}
	if _, err := os.Stat(filepath.Join(f.home, ".skillet", "archive", id)); err != nil {
		t.Fatalf("refused purge removed the archive entry: %v", err)
	}

	code, stdout, stderr := runCLI(t, "purge", id, "--yes")
	if code != 0 {
		t.Fatalf("purge exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "permanent") {
		t.Fatalf("purge summary = %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(f.home, ".skillet", "archive", id)); !os.IsNotExist(err) {
		t.Fatalf("purge left the archive entry: %v", err)
	}
}

func TestListShowsArchiveEntriesSoPurgeIsScriptable(t *testing.T) {
	newFixture(t)
	if code, _, stderr := runCLI(t, "archive", "writing", "--yes"); code != 0 {
		t.Fatalf("archive exit = %d, stderr = %q", code, stderr)
	}
	code, stdout, stderr := runCLI(t, "list", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	var document listJSON
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Archive) != 1 || document.Archive[0].Name != "writing" || document.Archive[0].ID == "" {
		t.Fatalf("archive block = %+v", document.Archive)
	}
	code, stdout, _ = runCLI(t, "list")
	if code != 0 || !strings.Contains(stdout, "Archive:") {
		t.Fatalf("text list has no Archive section:\n%s", stdout)
	}
}

// archiveID returns the id of the single archive entry whose folder name ends
// with the given Skill folder name.
func archiveID(t *testing.T, name string) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(home, ".skillet", "archive"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), "-"+name) {
			return entry.Name()
		}
	}
	t.Fatalf("no archive entry for %q in %v", name, entries)
	return ""
}

func TestManualOnlyThenAutoRoundTripsAPersonalSkill(t *testing.T) {
	f := newFixture(t)
	skillMD := filepath.Join(f.home, ".claude", "skills", "writing", "SKILL.md")

	code, stdout, stderr := runCLI(t, "manual-only", "writing", "--yes")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "Manual-only") {
		t.Fatalf("summary = %q", stdout)
	}
	if content := readFile(t, skillMD); !strings.Contains(content, "disable-model-invocation: true") {
		t.Fatalf("SKILL.md not updated:\n%s", content)
	}

	code, stdout, stderr = runCLI(t, "auto", "writing", "--yes")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "Auto-activation") {
		t.Fatalf("summary = %q", stdout)
	}
	if content := readFile(t, skillMD); strings.Contains(content, "disable-model-invocation") {
		t.Fatalf("auto did not remove the field:\n%s", content)
	}
}

func TestSuppressThenUnsuppressRoundTripsAPluginSkill(t *testing.T) {
	f := newFixture(t)
	skillMD := filepath.Join(f.home, ".claude", "plugins", "cache", "demo", "skills", "lint", "SKILL.md")

	code, stdout, stderr := runCLI(t, "suppress", "lint", "--yes")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "Suppressed Plugin skill \"lint\"") || !strings.Contains(stdout, "plugin demo@market") {
		t.Fatalf("summary = %q", stdout)
	}
	if content := readFile(t, skillMD); !strings.Contains(content, "disable-model-invocation: true") || !strings.Contains(content, "user-invocable: false") {
		t.Fatalf("suppress did not edit the plugin SKILL.md:\n%s", content)
	}
	code, stdout, _ = runCLI(t, "list", "--json")
	if code != 0 {
		t.Fatal("list failed after suppress")
	}
	if !strings.Contains(stdout, `"activation": "Suppressed"`) {
		t.Fatalf("list does not report the Suppressed state:\n%s", stdout)
	}

	code, stdout, stderr = runCLI(t, "unsuppress", "lint", "--yes")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "Unsuppressed") {
		t.Fatalf("summary = %q", stdout)
	}
	if content := readFile(t, skillMD); strings.Contains(content, "user-invocable: false") {
		t.Fatalf("unsuppress did not restore the plugin SKILL.md:\n%s", content)
	}
}

func TestSuppressRejectsASkillWhoseSourceHasNoSuppressMechanism(t *testing.T) {
	newFixture(t)
	code, _, stderr := runCLI(t, "suppress", "Personal:writing", "--yes")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "not supported") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestLibraryAddListRemoveRoundTrip(t *testing.T) {
	f := newFixture(t)
	source := filepath.Join(f.project, "source-skill")
	writeSkillFile(t, source, "helper", "A helper Skill.", "")

	code, stdout, stderr := runCLI(t, "library", "add", "--name", "helper", "--local-path", source, "--yes", "--json")
	if code != 0 {
		t.Fatalf("library add exit = %d, stderr = %q", code, stderr)
	}
	var added libraryEntryJSON
	if err := json.Unmarshal([]byte(stdout), &added); err != nil {
		t.Fatal(err)
	}
	if added.Entry.ID == "" || added.Entry.Source.Kind != "local-path" || added.Entry.Source.LocalPath != source {
		t.Fatalf("added entry = %+v", added.Entry)
	}

	code, stdout, stderr = runCLI(t, "library", "list", "--json")
	if code != 0 {
		t.Fatalf("library list exit = %d, stderr = %q", code, stderr)
	}
	var listed libraryListJSON
	if err := json.Unmarshal([]byte(stdout), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].ID != added.Entry.ID {
		t.Fatalf("library list = %+v", listed.Entries)
	}

	code, stdout, stderr = runCLI(t, "library", "remove", "helper", "--yes")
	if code != 0 {
		t.Fatalf("library remove exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "Removed Library entry \"helper\"") {
		t.Fatalf("summary = %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(source, "SKILL.md")); err != nil {
		t.Fatalf("library remove deleted the source: %v", err)
	}
	code, stdout, _ = runCLI(t, "library", "list")
	if code != 0 || !strings.Contains(stdout, "The Library is empty.") {
		t.Fatalf("library still has entries:\n%s", stdout)
	}
}

func TestLibraryAddRequiresExactlyOneSource(t *testing.T) {
	newFixture(t)
	code, _, stderr := runCLI(t, "library", "add", "--name", "x", "--yes")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "requires one source") {
		t.Fatalf("stderr = %q", stderr)
	}
	code, _, stderr = runCLI(t, "library", "add", "--name", "x", "--local-path", "/tmp/a", "--git-url", "https://example.com/a.git", "--yes")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "exactly one source") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestInstallPlacesALibraryEntryAtAProjectTarget(t *testing.T) {
	f := newFixture(t)
	source := filepath.Join(f.home, "source-skill")
	writeSkillFile(t, source, "helper", "A helper Skill.", "")
	if code, _, stderr := runCLI(t, "library", "add", "--name", "helper", "--local-path", source, "--yes"); code != 0 {
		t.Fatalf("library add exit = %d, stderr = %q", code, stderr)
	}

	target := filepath.Join(f.project, "repo")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCLI(t, "install", "helper", "--target", target, "--activation", "manual-only", "--yes")
	if code != 0 {
		t.Fatalf("install exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "Installed Library entry \"helper\"") || !strings.Contains(stdout, "Manual-only") {
		t.Fatalf("summary = %q", stdout)
	}
	installed := filepath.Join(target, ".claude", "skills", "helper", "SKILL.md")
	content := readFile(t, installed)
	if !strings.Contains(content, "disable-model-invocation: true") {
		t.Fatalf("--activation manual-only was not applied:\n%s", content)
	}
}

func TestInstallRequiresATarget(t *testing.T) {
	newFixture(t)
	code, _, stderr := runCLI(t, "install", "helper", "--yes")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--target is required") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestBundleListIsEmptyOnAFreshMachine(t *testing.T) {
	newFixture(t)
	code, stdout, stderr := runCLI(t, "bundle", "list")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "No Bundles.") {
		t.Fatalf("stdout = %q", stdout)
	}
	code, stdout, _ = runCLI(t, "bundle", "list", "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var document bundleListJSON
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != jsonSchemaVersion || len(document.Bundles) != 0 {
		t.Fatalf("bundle list --json = %+v", document)
	}
}

func TestBundleInstallReportsAnUnknownBundle(t *testing.T) {
	f := newFixture(t)
	code, _, stderr := runCLI(t, "bundle", "install", "nope", "--target", f.project, "--yes")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "no Bundle matches") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestEverySubcommandDocumentsItselfWithHelp(t *testing.T) {
	newFixture(t)
	commands := [][]string{
		{"list"}, {"show"}, {"archive"}, {"restore"}, {"purge"},
		{"suppress"}, {"unsuppress"}, {"manual-only"}, {"auto"},
		{"library"}, {"library", "list"}, {"library", "add"}, {"library", "remove"},
		{"bundle"}, {"bundle", "list"}, {"bundle", "install"}, {"install"},
	}
	for _, command := range commands {
		args := append(append([]string{}, command...), "--help")
		code, stdout, stderr := runCLI(t, args...)
		if code != 0 {
			t.Fatalf("%v --help exit = %d, stderr = %q", command, code, stderr)
		}
		if !strings.Contains(stdout, "usage: skillet "+command[0]) {
			t.Fatalf("%v --help stdout = %q", command, stdout)
		}
		if stderr != "" {
			t.Fatalf("%v --help wrote to stderr: %q", command, stderr)
		}
	}
}

func TestTopLevelHelpDocumentsTheWholeCommandTree(t *testing.T) {
	code, stdout, stderr := runCLI(t, "--help")
	if code != 0 || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	for _, command := range []string{"list", "show", "archive", "restore", "purge", "suppress", "unsuppress", "manual-only", "auto", "library", "bundle", "install", "setup", "version"} {
		if !strings.Contains(stdout, "  "+command) {
			t.Fatalf("top-level help does not document %q:\n%s", command, stdout)
		}
	}
}

func TestUnknownFlagIsAUsageError(t *testing.T) {
	newFixture(t)
	code, _, stderr := runCLI(t, "list", "--nonsense")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "usage: skillet list") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestVersionRejectsExtraArguments(t *testing.T) {
	code, _, stderr := runCLI(t, "version", "extra")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "usage:") {
		t.Fatalf("stderr = %q", stderr)
	}
}
