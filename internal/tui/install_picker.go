package tui

import (
	"fmt"
	"strings"

	"skillet/internal/engine"
)

// installTargetOption is one row in the Library Install target picker
// (Personal, or a resolved Project root). Free-text paths are never offered.
type installTargetOption struct {
	label  string
	target engine.InstallTarget
}

// installPicker is the modal Install target chooser (overlay family, same
// render path as confirmation overlays).
type installPicker struct {
	entry   engine.LibraryEntry
	bundle  *engine.Bundle
	options []installTargetOption
	cursor  int
}

// buildInstallTargetOptions lists Personal plus every resolved project root
// (Codex ∪ Claude roots, deduplicated). Shared by Library and Bundle Install.
func buildInstallTargetOptions(e *engine.Engine) []installTargetOption {
	opts := []installTargetOption{{
		label:  "Personal",
		target: engine.InstallTarget{Kind: engine.InstallTargetPersonal},
	}}
	for _, root := range e.ResolvedProjectRoots() {
		opts = append(opts, installTargetOption{
			label:  "Project: " + root,
			target: engine.InstallTarget{Kind: engine.InstallTargetProject, RepoRoot: root},
		})
	}
	return opts
}

func renderInstallPickerDescription(entryName string, options []installTargetOption, cursor int) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Install %q — choose target:\n", entryName))
	for i, opt := range options {
		mark := " "
		if i == cursor {
			mark = ">"
		}
		b.WriteString(fmt.Sprintf("\n%s %s", mark, opt.label))
	}
	b.WriteString("\n\nenter to select · esc to cancel")
	return b.String()
}
