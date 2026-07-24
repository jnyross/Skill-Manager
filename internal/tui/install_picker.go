package tui

import (
	"fmt"
	"strings"

	"github.com/jnyross/Skill-Manager/internal/engine"
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

// pickerVisibleRows caps how many rows any modal chooser prints at once. The
// overlay has no scrollback of its own, so an uncapped list (a machine with
// many resolved project roots, or a 40-entry Library) would render taller than
// the terminal and get clamped away by renderConfirmOverlay.
const pickerVisibleRows = 10

// pickerWindow returns the [start, end) slice of `count` rows to display so
// that `cursor` stays visible, scrolling by whole rows.
func pickerWindow(count, cursor, visible int) (int, int) {
	if visible < 1 {
		visible = 1
	}
	if count <= visible {
		return 0, count
	}
	start := cursor - visible/2
	if start < 0 {
		start = 0
	}
	if start > count-visible {
		start = count - visible
	}
	return start, start + visible
}

func renderInstallPickerDescription(entryName string, options []installTargetOption, cursor int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Install %q — choose target:\n", entryName)
	start, end := pickerWindow(len(options), cursor, pickerVisibleRows)
	if start > 0 {
		fmt.Fprintf(&b, "\n  … %d more above", start)
	}
	for i := start; i < end; i++ {
		mark := " "
		if i == cursor {
			mark = ">"
		}
		fmt.Fprintf(&b, "\n%s %s", mark, options[i].label)
	}
	if end < len(options) {
		fmt.Fprintf(&b, "\n  … %d more below", len(options)-end)
	}
	b.WriteString("\n\nenter to select · esc to cancel")
	return b.String()
}
