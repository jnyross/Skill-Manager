package engine

// Safety tests for the shared write primitives: the cross-process mutation lock
// (lock.go), the plugin cache-path guard (plugin_uninstall.go), and
// writeFileAtomic's symlink handling (atomic.go). These sit next to
// write_fault_test.go's crash simulations and follow the same rule: they mutate
// package-global state (engineLockWait, writeFaultHook) and must never run in
// parallel.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func shortenLockWait(t *testing.T, d time.Duration) {
	t.Helper()
	previous := engineLockWait
	engineLockWait = d
	t.Cleanup(func() { engineLockWait = previous })
}

// --- Mutation lock ---

// Two concurrent suppressions of *different* skills each read config.toml,
// compute their own version of it, and write it back. Without a lock the later
// write silently discards the earlier one's block — a lost update no error ever
// reports.
func TestMutationLockKeepsConcurrentSuppressionsFromLosingEachOther(t *testing.T) {
	f := newFaultFixture(t)
	f.writeCodexSkillWithoutConfig(t, "skill-one")
	f.writeCodexSkillWithoutConfig(t, "skill-two")
	one := f.codexSkill(t, "skill-one")
	two := f.codexSkill(t, "skill-two")

	// Widen the read-modify-write window so the lost update is deterministic
	// rather than a timing accident: without the lock both goroutines read the
	// same config.toml before either writes.
	useWriteFault(t, func(op, path string) error {
		if op == "write" && strings.Contains(path, "config.toml") {
			time.Sleep(100 * time.Millisecond)
		}
		return nil
	})

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, skill := range []Skill{one, two} {
		wg.Add(1)
		go func(i int, skill Skill) {
			defer wg.Done()
			errs[i] = f.e.Suppress(skill)
		}(i, skill)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent Suppress %d: %v", i, err)
		}
	}

	config, err := os.ReadFile(filepath.Join(f.roots.CodexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}
	for _, name := range []string{"skill-one", "skill-two"} {
		if !strings.Contains(string(config), filepath.Join("skills", name, "SKILL.md")) {
			t.Fatalf("%s's disable block was lost to the other suppression:\n%s", name, config)
		}
	}
}

// A lock held by another *live* process is respected: the mutation fails with an
// actionable message instead of racing it, and nothing is written.
func TestMutationLockRefusesToRaceALiveHolder(t *testing.T) {
	f := newFaultFixture(t)
	f.writeCodexSkillWithoutConfig(t, "codex-skill")
	skill := f.codexSkill(t, "codex-skill")
	shortenLockWait(t, 50*time.Millisecond)

	// The parent of the test binary is a different, running process, which is
	// exactly what a second Skillet instance looks like from here.
	lockPath := filepath.Join(f.roots.DataDir, engineLockFileName)
	holder := os.Getppid()
	contents := fmt.Sprintf("pid: %d\nstarted: %s\n", holder, time.Now().UTC().Format(time.RFC3339))
	if err := os.WriteFile(lockPath, []byte(contents), 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	err := f.e.Suppress(skill)
	if err == nil {
		t.Fatalf("Suppress should have refused to run while another process holds the lock")
	}
	for _, want := range []string{fmt.Sprint(holder), lockPath} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
	if exists(t, filepath.Join(f.roots.CodexHome, "config.toml")) {
		t.Fatalf("a blocked mutation must not have written config.toml")
	}
	if !exists(t, lockPath) {
		t.Fatalf("a live holder's lock must not be removed")
	}
}

// A lock left behind by a process that died mid-write must not wedge Skillet
// forever, and a completed mutation must leave no lock behind.
func TestMutationLockClearsAnAbandonedHolderAndReleasesAfterwards(t *testing.T) {
	f := newFaultFixture(t)
	f.writeCodexSkillWithoutConfig(t, "codex-skill")
	skill := f.codexSkill(t, "codex-skill")
	shortenLockWait(t, 50*time.Millisecond)

	lockPath := filepath.Join(f.roots.DataDir, engineLockFileName)
	if err := os.WriteFile(lockPath, []byte("pid: not-a-number\n"), 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	if err := f.e.Suppress(skill); err != nil {
		t.Fatalf("Suppress should have taken over the abandoned lock: %v", err)
	}
	if exists(t, lockPath) {
		t.Fatalf("the lock must be released once the mutation finishes")
	}
}

// The lock is released even when the locked work panics, or one wedged mutation
// would lock every later one out.
func TestMutationLockIsReleasedOnPanic(t *testing.T) {
	dataDir := t.TempDir()
	lockPath := filepath.Join(dataDir, engineLockFileName)

	func() {
		defer func() { _ = recover() }()
		_ = withEngineLock(dataDir, func() error { panic("boom") })
	}()

	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("lock file survived a panicking mutation (stat err = %v)", err)
	}
	if err := withEngineLock(dataDir, func() error { return nil }); err != nil {
		t.Fatalf("the lock must be takeable again after a panic: %v", err)
	}
}

// --- Plugin cache-path guard ---

// The guard's whole purpose is to stop a malformed or malicious manifest entry
// from deleting arbitrary directories. A lexical check alone passes an entry
// whose intermediate component is a symlink out of the cache, and the later
// os.RemoveAll follows it.
func TestValidatePluginCachePathRejectsASymlinkEscapingTheCache(t *testing.T) {
	root := t.TempDir()
	claudeHome := filepath.Join(root, "claude")
	cacheRoot := filepath.Join(claudeHome, "plugins", "cache")
	outside := filepath.Join(root, "important-user-data")
	if err := os.MkdirAll(filepath.Join(outside, "v1"), 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(cacheRoot, "marketplace-x"), 0o755); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	// An intermediate component inside the cache points out of it.
	if err := os.Symlink(outside, filepath.Join(cacheRoot, "marketplace-x", "plugin-x")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	escaping := filepath.Join(cacheRoot, "marketplace-x", "plugin-x", "v1")
	err := validatePluginCachePath(claudeHome, escaping)
	if err == nil {
		t.Fatalf("a path resolving outside the cache root must be rejected")
	}
	if !strings.Contains(err.Error(), "outside cache root") {
		t.Fatalf("error %q does not say the path escapes the cache root", err)
	}
	if _, statErr := os.Stat(filepath.Join(outside, "v1")); statErr != nil {
		t.Fatalf("validation must not touch the external target: %v", statErr)
	}
}

// Real cache paths still validate, including one whose directory a user already
// deleted by hand — an ordinary, non-error case for uninstall.
func TestValidatePluginCachePathAcceptsRealAndAlreadyDeletedCachePaths(t *testing.T) {
	root := t.TempDir()
	claudeHome := filepath.Join(root, "claude")
	cacheRoot := filepath.Join(claudeHome, "plugins", "cache")
	present := filepath.Join(cacheRoot, "marketplace-x", "plugin-x", "v1")
	if err := os.MkdirAll(present, 0o755); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}

	if err := validatePluginCachePath(claudeHome, present); err != nil {
		t.Fatalf("an ordinary cache path must validate: %v", err)
	}
	deleted := filepath.Join(cacheRoot, "marketplace-y", "plugin-y", "v1")
	if err := validatePluginCachePath(claudeHome, deleted); err != nil {
		t.Fatalf("an already-deleted cache path must still validate: %v", err)
	}
}

// --- Atomic write: dangling symlink ---

// A symlink whose target file does not exist yet (a dotfiles repo checked out
// without that file) must be written *through*, not replaced: silently turning
// the link into a regular file detaches the user's config from the repo.
func TestWriteFileAtomicWritesThroughADanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	targetDir := filepath.Join(dir, "dotfiles")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	target := filepath.Join(targetDir, "settings.json")
	link := filepath.Join(dir, "settings.json")
	if err := os.Symlink(target, link); err != nil {
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
		t.Fatalf("the dangling symlink was replaced by a regular file")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read symlink target: %v", err)
	}
	if string(got) != "{\"a\":1}\n" {
		t.Fatalf("target = %q, want the new content", got)
	}
}

// When the dangling link's target directory is gone too there is no sane
// destination: fail with a message naming the link rather than clobbering it.
func TestWriteFileAtomicRefusesASymlinkIntoAMissingDirectory(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "settings.json")
	target := filepath.Join(dir, "gone", "settings.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	err := writeFilePreservingMode(link, "{}\n")
	if err == nil {
		t.Fatalf("writing through a symlink into a missing directory must fail")
	}
	if !strings.Contains(err.Error(), link) {
		t.Fatalf("error %q does not name the symlink %s", err, link)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat link: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the symlink was clobbered by a failed write")
	}
}
