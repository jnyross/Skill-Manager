package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

type Roots struct {
	ClaudeHome         string
	CodexHome          string
	AgentsHome         string
	DataDir            string
	ProjectRoots       []string
	ClaudeProjectRoots []string
}

// Default per-operation subprocess timeouts. Every external program Skillet
// runs is bounded: a hung `git clone` (an unreachable host, a credential
// prompt on a non-tty) or a stalled `npx` must surface as an error the TUI can
// show, not as a UI that never comes back.
const (
	// DefaultInstallTimeout bounds a network-backed install or clone.
	DefaultInstallTimeout = 120 * time.Second
	// DefaultProbeTimeout bounds a short informational subprocess (a version
	// or availability check) run through RunProbe.
	DefaultProbeTimeout = 30 * time.Second
)

type Engine struct {
	roots          Roots
	runner         CommandRunner
	installTimeout time.Duration
	probeTimeout   time.Duration
}

func New(roots Roots) *Engine {
	return NewWithCommandRunner(roots, execCommandRunner{})
}

// Command describes one external program invocation owned by an install
// mechanism. Dir is empty when the caller's working directory is appropriate.
type Command struct {
	Name string
	Args []string
	Dir  string
	Env  []string
}

type CommandResult struct {
	Stdout string
	Stderr string
}

// CommandRunner is the process boundary used by network-backed Library
// sources. Injecting it keeps engine tests offline and makes argv/cwd explicit.
// Run must honour ctx: the engine always passes a context carrying the
// operation's deadline, and cancelling it must terminate the subprocess.
type CommandRunner interface {
	Run(ctx context.Context, command Command) (CommandResult, error)
}

func NewWithCommandRunner(roots Roots, runner CommandRunner) *Engine {
	if runner == nil {
		runner = execCommandRunner{}
	}
	return &Engine{
		roots:          roots,
		runner:         runner,
		installTimeout: DefaultInstallTimeout,
		probeTimeout:   DefaultProbeTimeout,
	}
}

// SetInstallTimeout overrides the per-subprocess timeout applied to installs
// and clones. A non-positive value restores DefaultInstallTimeout.
func (e *Engine) SetInstallTimeout(d time.Duration) {
	if d <= 0 {
		d = DefaultInstallTimeout
	}
	e.installTimeout = d
}

// SetProbeTimeout overrides the per-subprocess timeout applied to RunProbe. A
// non-positive value restores DefaultProbeTimeout.
func (e *Engine) SetProbeTimeout(d time.Duration) {
	if d <= 0 {
		d = DefaultProbeTimeout
	}
	e.probeTimeout = d
}

// RunProbe runs a short-lived informational subprocess through the engine's
// CommandRunner seam, bounded by the probe timeout and by ctx. It is the
// read-only counterpart of the install entry points: callers that need to ask
// whether a tool exists or what version it is get the same cancellation and
// timeout behaviour as an install without reaching for os/exec themselves.
func (e *Engine) RunProbe(ctx context.Context, command Command) (CommandResult, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout(e.probeTimeout))
	defer cancel()
	result, err := e.runner.Run(ctx, command)
	if err != nil {
		return result, timeoutAwareError(ctx, command, e.timeout(e.probeTimeout), err)
	}
	return result, nil
}

func (e *Engine) timeout(configured time.Duration) time.Duration {
	if configured <= 0 {
		return DefaultInstallTimeout
	}
	return configured
}

// timeoutAwareError replaces an opaque subprocess failure with an explicit
// timeout or cancellation message when the context is what ended it, so the
// status line can say which operation stalled and for how long.
func timeoutAwareError(ctx context.Context, command Command, limit time.Duration, err error) error {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return fmt.Errorf("%s timed out after %s: %w", command.Name, limit, err)
	case errors.Is(ctx.Err(), context.Canceled):
		return fmt.Errorf("%s cancelled: %w", command.Name, err)
	default:
		return err
	}
}

type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, command Command) (CommandResult, error) {
	cmd := exec.CommandContext(ctx, command.Name, command.Args...)
	cmd.Dir = command.Dir
	if command.Env != nil {
		cmd.Env = command.Env
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := CommandResult{Stdout: stdout.String(), Stderr: stderr.String()}
	return result, err
}
