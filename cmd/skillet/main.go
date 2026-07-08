package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"skillet/internal/engine"
	"skillet/internal/tui"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	e := engine.New(engine.Roots{
		ClaudeHome:         filepath.Join(home, ".claude"),
		CodexHome:          filepath.Join(home, ".codex"),
		AgentsHome:         filepath.Join(home, ".agents"),
		DataDir:            filepath.Join(home, ".skillet"),
		ProjectRoots:       engine.FindProjectRoots(cwd),
		ClaudeProjectRoots: engine.FindClaudeProjectRoots(cwd),
	})
	p := tea.NewProgram(tui.NewModel(e), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
