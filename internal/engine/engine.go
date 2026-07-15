package engine

import (
	"bytes"
	"os/exec"
)

type Roots struct {
	ClaudeHome         string
	CodexHome          string
	AgentsHome         string
	DataDir            string
	ProjectRoots       []string
	ClaudeProjectRoots []string
}

type Engine struct {
	roots  Roots
	runner CommandRunner
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
type CommandRunner interface {
	Run(Command) (CommandResult, error)
}

func NewWithCommandRunner(roots Roots, runner CommandRunner) *Engine {
	if runner == nil {
		runner = execCommandRunner{}
	}
	return &Engine{roots: roots, runner: runner}
}

type execCommandRunner struct{}

func (execCommandRunner) Run(command Command) (CommandResult, error) {
	cmd := exec.Command(command.Name, command.Args...)
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
