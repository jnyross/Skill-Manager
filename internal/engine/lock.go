package engine

// Cross-process mutation lock. Every config file Skillet edits — Codex's
// config.toml, Claude Code's settings.json and installed_plugins.json — is
// changed by a read-modify-write: the whole file is read, a new version is
// computed in memory, and that version is written back. atomic.go makes each
// of those writes indivisible, so no reader ever sees a torn file, but atomicity
// says nothing about the gap between the read and the write. Two Skillet
// processes (a second TUI instance, or a TUI and a CLI invocation) suppressing
// different skills can each read the same config.toml, each compute a new
// version containing only their own block, and the later rename silently
// discards the other's — a lost update, with no error anywhere and a skill that
// quietly re-enables itself.
//
// The fix is one advisory lock file, <DataDir>/.lock, held across a complete
// read-plan-write-record transaction and released on every exit path including
// a panic. It follows the convention internal/setup/service.go already
// established for its per-project staging lock — a PID plus an RFC3339 start
// timestamp, with a holder considered stale when its process is gone or it has
// held the lock past a timeout — rather than introducing a second lock format.
//
// Scope is deliberately narrow. The lock is taken per mutating transaction, not
// for the lifetime of the TUI: holding it across a session would make a single
// open Skillet window block every other invocation on the machine. Read-only
// scanning does not take it either, since a scan that races a mutation sees the
// old or the new file — both consistent, thanks to atomic.go — and the TUI
// re-scans after every action anyway. Install paths are left unlocked on
// purpose: they call SetManualOnly internally, and the lock is not reentrant.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	engineLockFileName = ".lock"
	// engineLockStaleAfter matches internal/setup's stagingLockTimeout: a lock
	// held this long by a process that still exists is treated as abandoned.
	// Engine mutations take milliseconds, so this only ever fires for a wedged
	// process or a recycled PID.
	engineLockStaleAfter = 15 * time.Minute
	engineLockPoll       = 20 * time.Millisecond
)

// engineLockWait bounds how long a mutation waits for another process to
// release the lock before failing with an actionable message. A variable, not a
// constant, so tests can shorten it; production never changes it.
var engineLockWait = 5 * time.Second

// engineLockInProcess serialises mutations inside one process. The lock file
// alone cannot do that job: it records only a PID, so two goroutines in the
// same process would each see their own PID in the file and take it in turn as
// a stale lock. Holding this mutex first also means a lock file bearing our own
// PID can only be a leftover, which is why engineLockStale treats it as stale.
var engineLockInProcess sync.Mutex

// engineLockAcquired, when set, is called once per successful lock
// acquisition. It is nil in production and exists for the one property that is
// otherwise invisible from outside the package: that a bulk mutation
// (bulk.go's SetManualOnlyBulk) takes the lock once for the whole sweep rather
// than once per Skill — the lock is not reentrant, so looping over the public
// single-Skill method would deadlock or, worse, quietly serialise 44 separate
// transactions. Tests setting it must not run in parallel, the same rule
// writeFaultHook carries.
var engineLockAcquired func()

// withEngineLock runs fn holding the engine's mutation lock. The lock is
// released on every path, including a panic, because both releases are
// deferred.
//
// An empty dataDir (an Engine constructed for a read-only or test purpose with
// no data directory) runs fn unlocked rather than creating a lock file at the
// filesystem root.
func withEngineLock(dataDir string, fn func() error) error {
	if strings.TrimSpace(dataDir) == "" {
		return fn()
	}

	engineLockInProcess.Lock()
	defer engineLockInProcess.Unlock()

	release, err := acquireEngineLock(dataDir)
	if err != nil {
		return err
	}
	defer release()
	return fn()
}

// acquireEngineLock creates <dataDir>/.lock exclusively, waiting up to
// engineLockWait for an existing holder to finish and stealing the lock when
// its holder is demonstrably gone.
func acquireEngineLock(dataDir string) (func(), error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	lockPath := filepath.Join(dataDir, engineLockFileName)
	deadline := time.Now().Add(engineLockWait)

	for {
		file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			contents := fmt.Sprintf("pid: %d\nstarted: %s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339))
			_, writeErr := file.WriteString(contents)
			closeErr := file.Close()
			if err := errors.Join(writeErr, closeErr); err != nil {
				_ = os.Remove(lockPath)
				return nil, fmt.Errorf("write lock %s: %w", lockPath, err)
			}
			if engineLockAcquired != nil {
				engineLockAcquired()
			}
			return func() { _ = os.Remove(lockPath) }, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("acquire lock %s: %w", lockPath, err)
		}

		pid, stale := engineLockStale(lockPath)
		if stale {
			// The holder is gone (or is a leftover from this process). Clearing
			// it and retrying is safe; if another process clears it first, the
			// next attempt simply creates it.
			if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("clear stale lock %s: %w", lockPath, err)
			}
			// The deadline bounds stale-lock churn too: another process
			// recreating the lock as fast as we clear it must not spin forever.
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("could not take %s within %s: another Skillet process keeps re-creating it — close any other running Skillet and try again", lockPath, engineLockWait)
			}
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("another Skillet process (PID %d) is still changing skills; it has held %s for longer than %s — close that process, or delete the lock file if it is no longer running, and try again", pid, lockPath, engineLockWait)
		}
		time.Sleep(engineLockPoll)
	}
}

// engineLockStale reports whether the lock file's holder can be disregarded,
// returning the recorded PID for the error message when it cannot. An
// unreadable or half-written lock counts as stale: it can only come from a
// process that died mid-write.
func engineLockStale(lockPath string) (int, bool) {
	pid, started, err := readEngineLock(lockPath)
	switch {
	case err != nil:
		return 0, true
	case pid == os.Getpid():
		return pid, true
	case !processRunning(pid):
		return pid, true
	case time.Since(started) > engineLockStaleAfter:
		return pid, true
	default:
		return pid, false
	}
}

func readEngineLock(lockPath string) (int, time.Time, error) {
	contents, err := os.ReadFile(lockPath)
	if err != nil {
		return 0, time.Time{}, err
	}
	var pid int
	var started time.Time
	for _, line := range strings.Split(string(contents), "\n") {
		if value, ok := strings.CutPrefix(line, "pid:"); ok {
			if parsed, parseErr := strconv.Atoi(strings.TrimSpace(value)); parseErr == nil {
				pid = parsed
			}
		}
		if value, ok := strings.CutPrefix(line, "started:"); ok {
			if parsed, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(value)); parseErr == nil {
				started = parsed
			}
		}
	}
	if pid == 0 || started.IsZero() {
		return 0, time.Time{}, fmt.Errorf("incomplete lock at %s", lockPath)
	}
	return pid, started, nil
}

// processRunning mirrors internal/setup's isProcessRunning: signal 0 reports
// existence without disturbing the process, and EPERM means it exists but
// belongs to another user.
func processRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}
