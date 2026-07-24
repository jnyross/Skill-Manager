package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

func TestConfirmOverlayClampsToTheCanvasWhenTooTall(t *testing.T) {
	background := strings.Repeat("row\n", 5)
	description := strings.Repeat("a very long confirmation sentence that wraps. ", 20)

	const width, height = 40, 6
	out := renderConfirmOverlay(background, description, width, height)
	lines := strings.Split(out, "\n")
	if len(lines) != height {
		t.Fatalf("overlay has %d lines, want exactly %d", len(lines), height)
	}
	for i, line := range lines {
		if got := lipgloss.Width(line); got > width {
			t.Errorf("line %d is %d columns wide, want <= %d: %q", i, got, width, line)
		}
	}
}

func TestInstallPickerScrollsRatherThanPrintingEveryTarget(t *testing.T) {
	options := make([]installTargetOption, 40)
	for i := range options {
		options[i] = installTargetOption{label: "Project: /repo/" + string(rune('a'+i%26)) + string(rune('0'+i/26))}
	}
	desc := renderInstallPickerDescription("my-skill", options, 30)
	if got := strings.Count(desc, "\n"); got > pickerVisibleRows+6 {
		t.Fatalf("install picker rendered %d lines for 40 targets", got)
	}
	if !strings.Contains(desc, "more above") {
		t.Fatalf("install picker does not show it is scrolled: %s", desc)
	}
	if !strings.Contains(desc, options[30].label) {
		t.Fatalf("install picker does not show the cursor row: %s", desc)
	}
}

func TestMemberPickerScrollsRatherThanPrintingEveryLibraryEntry(t *testing.T) {
	entries := make([]engine.LibraryEntry, 40)
	for i := range entries {
		entries[i] = engine.LibraryEntry{Name: "entry-" + string(rune('a'+i%26)) + string(rune('0'+i/26))}
	}
	p := &memberPicker{bundle: engine.Bundle{Name: "review loop"}, entries: entries, cursor: 0}
	out := renderMemberPicker(p)
	if got := strings.Count(out, "\n"); got > pickerVisibleRows+6 {
		t.Fatalf("member picker rendered %d lines for a 40-entry Library", got)
	}
	if !strings.Contains(out, "more below") {
		t.Fatalf("member picker does not show it is scrolled: %s", out)
	}

	p.cursor = 39
	out = renderMemberPicker(p)
	if !strings.Contains(out, entries[39].Name) {
		t.Fatalf("member picker does not follow the cursor to the last entry: %s", out)
	}
}

func TestBundleRowIsWidthClampedLikeTheOtherDelegates(t *testing.T) {
	row := bundleItem{bundle: engine.Bundle{Name: strings.Repeat("long-bundle-name-", 10)}}
	rendered := renderBundleItem(row, false, 30)
	if got := lipgloss.Width(rendered); got > 30 {
		t.Fatalf("bundle row is %d columns wide, want <= 30 (delegate Height() is 1)", got)
	}
}
