package engine

// Crash-simulation tests. Every multi-step mutation in the engine is exercised
// with one step forced to fail, asserting the invariant that matters for a
// tool whose promise is "reversible and safe": no failure may leave a skill
// absent from both the tool and the archive, wedge a restore permanently, or
// delete plugin cache files while settings.json still enables the plugin.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// useWriteFault installs a fault-injection hook for the duration of one test.
// Tests using it must not run in parallel — the hook is package-global, which
// is the price of keeping the seam invisible in production code.
func useWriteFault(t *testing.T, hook func(op, path string) error) {
	t.Helper()
	writeFaultHook = hook
	t.Cleanup(func() { writeFaultHook = nil })
}

// faultOn returns a hook failing every op whose path contains any of
// substrings.
func faultOn(op string, substrings ...string) func(string, string) error {
	return func(gotOp, path string) error {
		if gotOp != op {
			return nil
		}
		for _, substring := range substrings {
			if strings.Contains(path, substring) {
				return fmt.Errorf("injected %s fault at %s", op, path)
			}
		}
		return nil
	}
}

type faultFixture struct {
	root  string
	roots Roots
	e     *Engine
}

func newFaultFixture(t *testing.T) faultFixture {
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
	return faultFixture{root: root, roots: roots, e: New(roots)}
}

// writeCodexSkill creates a Codex skill folder plus a config.toml entry for
// it, the shape Uninstall has to clean up in more than one step.
func (f faultFixture) writeCodexSkill(t *testing.T, name string) string {
	t.Helper()
	folder := filepath.Join(f.roots.CodexHome, "skills", name)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	skillMD := filepath.Join(folder, "SKILL.md")
	content := fmt.Sprintf("---\nname: %q\ndescription: \"A skill\"\n---\nBody\n", name)
	if err := os.WriteFile(skillMD, []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	config := "[profile]\nname = \"default\"\n\n[[skills.config]]\npath = " + fmt.Sprintf("%q", skillMD) + "\nenabled = false\n"
	if err := os.WriteFile(filepath.Join(f.roots.CodexHome, "config.toml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}
	return folder
}

func (f faultFixture) archiveIDs(t *testing.T) []string {
	t.Helper()
	entries, _, err := f.e.ListArchive()
	if err != nil {
		t.Fatalf("ListArchive: %v", err)
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ID)
	}
	return ids
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	present, err := pathExists(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return present
}

// The core invariant: whichever step of Uninstall fails, the skill is either
// still installed or visible in the archive — never neither.
func TestUninstallNeverStrandsSkillOutsideToolAndArchive(t *testing.T) {
	for _, tc := range []struct {
		name        string
		hook        func(string, string) error
		wantInstall bool
		wantArchive bool
	}{
		{
			name:        "provenance write fails before the move",
			hook:        faultOn("write", "provenance.json"),
			wantInstall: true,
		},
		{
			name:        "the move itself fails",
			hook:        faultOn("move", filepath.Join("data", "archive")),
			wantInstall: true,
		},
		{
			name:        "config rewrite after the move fails and the move is rolled back",
			hook:        faultOn("write", "config.toml"),
			wantInstall: true,
		},
		{
			name: "config rewrite fails and the rollback fails too",
			hook: func(op, path string) error {
				if op == "write" && strings.Contains(path, "config.toml") {
					return fmt.Errorf("injected config write fault")
				}
				if op == "move" && strings.Contains(path, filepath.Join("codex", "skills")) {
					return fmt.Errorf("injected rollback fault")
				}
				return nil
			},
			wantArchive: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFaultFixture(t)
			folder := f.writeCodexSkill(t, "codex-skill")
			useWriteFault(t, tc.hook)

			if _, err := f.e.Uninstall(folder); err == nil {
				t.Fatalf("Uninstall should have failed")
			}

			writeFaultHook = nil
			installed := exists(t, folder)
			archived := f.archiveIDs(t)
			if !installed && len(archived) == 0 {
				t.Fatalf("skill is gone from the tool and absent from ListArchive — unrecoverable state")
			}
			if tc.wantInstall && !installed {
				t.Fatalf("skill should still be installed at %s", folder)
			}
			if tc.wantInstall && len(archived) != 0 {
				t.Fatalf("failed Uninstall left archive entries %v", archived)
			}
			if tc.wantArchive {
				if len(archived) != 1 {
					t.Fatalf("archive entries = %v, want exactly one recoverable entry", archived)
				}
				if err := f.e.Restore(archived[0]); err != nil {
					t.Fatalf("the preserved archive entry must be restorable: %v", err)
				}
				if !exists(t, folder) {
					t.Fatalf("Restore did not put the skill back at %s", folder)
				}
			}
		})
	}
}

// A failure reinstating config.toml used to wedge the entry forever: the
// archive directory survived with the payload already moved back, so every
// later Restore reported "restore destination already exists".
func TestRestoreResumesAfterConfigReinstateFailure(t *testing.T) {
	f := newFaultFixture(t)
	folder := f.writeCodexSkill(t, "codex-skill")
	configPath := filepath.Join(f.roots.CodexHome, "config.toml")

	entry, err := f.e.Uninstall(folder)
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if len(entry.RemovedConfigEntries) == 0 {
		t.Fatalf("test setup invalid: no config entries were removed")
	}

	useWriteFault(t, faultOn("write", "config.toml"))
	if err := f.e.Restore(entry.ID); err == nil {
		t.Fatalf("Restore should have failed while config.toml writes fail")
	}
	if !exists(t, folder) {
		t.Fatalf("the skill directory should already be back at %s", folder)
	}
	writeFaultHook = nil

	if err := f.e.Restore(entry.ID); err != nil {
		t.Fatalf("second Restore must complete the half-restored entry: %v", err)
	}
	if exists(t, filepath.Join(f.roots.DataDir, "archive", entry.ID)) {
		t.Fatalf("archive directory should be gone after a completed Restore")
	}

	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}
	if got := strings.Count(string(config), "[[skills.config]]"); got != 1 {
		t.Fatalf("config.toml has %d skills.config blocks, want exactly 1 (reinstatement must be idempotent):\n%s", got, config)
	}
}

func TestRestoreReportsActionablyWhenDestinationIsOccupied(t *testing.T) {
	f := newFaultFixture(t)
	folder := f.writeCodexSkill(t, "codex-skill")
	entry, err := f.e.Uninstall(folder)
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatalf("recreate skill folder: %v", err)
	}

	err = f.e.Restore(entry.ID)
	if err == nil {
		t.Fatalf("Restore should refuse to overwrite an occupied destination")
	}
	for _, want := range []string{"already exists", "archived copy is still safe", entry.ID} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
}

// --- Plugin uninstall ordering ---

func (f faultFixture) writePluginFixture(t *testing.T) (cacheDir string, settingsPath string, manifestPath string) {
	t.Helper()
	cacheDir = filepath.Join(f.roots.ClaudeHome, "plugins", "cache", "catalog", "demo", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "marker"), []byte("cached"), 0o644); err != nil {
		t.Fatalf("write cache marker: %v", err)
	}
	manifestPath = filepath.Join(f.roots.ClaudeHome, "plugins", "installed_plugins.json")
	manifest := fmt.Sprintf(`{
  "version": 2,
  "plugins": {
    "demo@catalog": [{"scope": "user", "installPath": %q}]
  }
}
`, cacheDir)
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	settingsPath = filepath.Join(f.roots.ClaudeHome, "settings.json")
	settings := "{\n  \"model\": \"opus\",\n  \"enabledPlugins\": {\n    \"demo@catalog\": true,\n    \"other@catalog\": true\n  }\n}\n"
	if err := os.WriteFile(settingsPath, []byte(settings), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	return cacheDir, settingsPath, manifestPath
}

func TestUninstallPluginNeverDeletesCacheWhileSettingsStillEnablesIt(t *testing.T) {
	for _, tc := range []struct {
		name              string
		hook              func(string, string) error
		wantStillEnabled  bool
		wantCachePresent  bool
		wantManifestEntry bool
	}{
		{
			name:              "manifest write fails: settings.json is rolled back and the cache survives",
			hook:              faultOn("write", "installed_plugins.json"),
			wantStillEnabled:  true,
			wantCachePresent:  true,
			wantManifestEntry: true,
		},
		{
			name:             "cache deletion fails: both config files already agree the plugin is gone",
			hook:             faultOn("remove", filepath.Join("plugins", "cache")),
			wantCachePresent: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFaultFixture(t)
			cacheDir, settingsPath, manifestPath := f.writePluginFixture(t)
			useWriteFault(t, tc.hook)

			if err := f.e.UninstallPlugin(PluginInfo{Plugin: "demo", Marketplace: "catalog"}); err == nil {
				t.Fatalf("UninstallPlugin should have failed")
			}
			writeFaultHook = nil

			settings, err := os.ReadFile(settingsPath)
			if err != nil {
				t.Fatalf("read settings.json: %v", err)
			}
			stillEnabled := strings.Contains(string(settings), "demo@catalog")
			cachePresent := exists(t, cacheDir)
			manifest, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatalf("read manifest: %v", err)
			}
			manifestEntry := strings.Contains(string(manifest), "demo@catalog")

			if stillEnabled && !cachePresent {
				t.Fatalf("cache files were deleted while settings.json still enables the plugin")
			}
			if stillEnabled != tc.wantStillEnabled {
				t.Fatalf("settings.json still enables plugin = %v, want %v (content: %s)", stillEnabled, tc.wantStillEnabled, settings)
			}
			if cachePresent != tc.wantCachePresent {
				t.Fatalf("cache present = %v, want %v", cachePresent, tc.wantCachePresent)
			}
			if manifestEntry != tc.wantManifestEntry {
				t.Fatalf("manifest entry present = %v, want %v", manifestEntry, tc.wantManifestEntry)
			}
			if !strings.Contains(string(settings), "other@catalog") {
				t.Fatalf("unrelated enabledPlugins entry was lost: %s", settings)
			}
		})
	}
}

// --- Atomic write behaviour ---

func TestWriteFileAtomicLeavesOriginalIntactWhenTheWriteFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	original := "[profile]\nname = \"default\"\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	useWriteFault(t, faultOn("write", "config.toml"))
	if err := writeFileAtomic(path, []byte("replacement"), 0o600); err == nil {
		t.Fatalf("expected the injected fault to fail the write")
	}
	writeFaultHook = nil

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) != original {
		t.Fatalf("file = %q, want the untouched original %q", got, original)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("temp file left behind: %d entries", len(entries))
	}
}

// A user who symlinks ~/.claude/settings.json into a dotfiles repo must keep
// the symlink: the atomic rename has to land on the real file.
func TestWriteFileAtomicWritesThroughSymlinkedDestination(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.json")
	link := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(real, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("seed real file: %v", err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if err := writeFilePreservingMode(link, "{\"a\":1}\n"); err != nil {
		t.Fatalf("writeFilePreservingMode: %v", err)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat link: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the symlink was replaced by a regular file")
	}
	got, err := os.ReadFile(real)
	if err != nil {
		t.Fatalf("read real file: %v", err)
	}
	if string(got) != "{\"a\":1}\n" {
		t.Fatalf("real file = %q, want the new content", got)
	}
}
