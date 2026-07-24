package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

type formKind int

const (
	formBundleName formKind = iota
	formLibraryEntry
)

type textForm struct {
	kind   formKind
	fields []string
	values []string
	index  int
	input  textinput.Model
	source engine.LibrarySourceKind
	// discarding is true while the form is asking the user to confirm that
	// esc should throw away what they have already typed. Without it, esc on
	// field 5 of 5 silently discarded all five values.
	discarding bool
}

func newTextForm(kind formKind, fields []string) *textForm {
	in := textinput.New()
	in.Focus()
	in.CharLimit = 512
	in.Width = 60
	return &textForm{kind: kind, fields: fields, values: make([]string, len(fields)), input: in}
}

// hasInput reports whether the user has typed anything worth protecting from
// an accidental esc.
func (f *textForm) hasInput() bool {
	if strings.TrimSpace(f.input.Value()) != "" {
		return true
	}
	for _, value := range f.values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func (f *textForm) update(msg tea.KeyMsg) (done, canceled bool) {
	if f.discarding {
		if strings.ToLower(msg.String()) == "y" {
			return false, true
		}
		f.discarding = false
		return false, false
	}

	switch msg.String() {
	case "esc":
		if f.hasInput() {
			f.discarding = true
			return false, false
		}
		return false, true
	case "ctrl+c":
		// Handled by the Model as a global quit; never treated as text.
		return false, false
	case "enter":
		f.values[f.index] = strings.TrimSpace(f.input.Value())
		if f.index == len(f.fields)-1 {
			return true, false
		}
		f.index++
		f.input.SetValue(f.values[f.index])
		f.input.CursorEnd()
		return false, false
	case "shift+tab":
		// Back-navigation: keep what is on screen, then step to the previous
		// field so a typo three fields ago does not mean restarting the form.
		f.values[f.index] = strings.TrimSpace(f.input.Value())
		if f.index > 0 {
			f.index--
			f.input.SetValue(f.values[f.index])
			f.input.CursorEnd()
		}
		return false, false
	}
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	_ = cmd
	return false, false
}

func (f *textForm) render() string {
	if f.discarding {
		return "Discard this entry and everything typed so far?\n\ny to discard, any other key to keep editing"
	}
	position := fmt.Sprintf("Field %d of %d", f.index+1, len(f.fields))
	hint := "enter next · shift+tab back · esc cancel"
	if f.index == len(f.fields)-1 {
		hint = "enter save · shift+tab back · esc cancel"
	}
	return fmt.Sprintf("%s — %s\n\n%s\n\n%s", position, f.fields[f.index], f.input.View(), hint)
}

type librarySourcePicker struct{ cursor int }

var librarySourceChoices = []engine.LibrarySourceKind{
	engine.LibrarySourceLocalPath, engine.LibrarySourceGit,
	engine.LibrarySourceSkillsSh, engine.LibrarySourceMarketplace,
}

func renderLibrarySourcePicker(cursor int) string {
	var b strings.Builder
	b.WriteString("New Library entry — choose source:\n")
	for i, choice := range librarySourceChoices {
		marker := " "
		if i == cursor {
			marker = ">"
		}
		fmt.Fprintf(&b, "%s %s\n", marker, choice)
	}
	b.WriteString("\nenter choose · esc cancel")
	return b.String()
}

type memberPicker struct {
	bundle  engine.Bundle
	entries []engine.LibraryEntry
	cursor  int
}

// renderMemberPicker shows a scrolling window over the Library rather than
// every entry: a 40-entry Library printed in full renders taller than the
// terminal and the overlay has no scrollback of its own.
func renderMemberPicker(p *memberPicker) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Add to Bundle %q:\n", p.bundle.Name)
	start, end := pickerWindow(len(p.entries), p.cursor, pickerVisibleRows)
	if start > 0 {
		fmt.Fprintf(&b, "  … %d more above\n", start)
	}
	for i := start; i < end; i++ {
		marker := " "
		if i == p.cursor {
			marker = ">"
		}
		fmt.Fprintf(&b, "%s %s\n", marker, p.entries[i].Name)
	}
	if end < len(p.entries) {
		fmt.Fprintf(&b, "  … %d more below\n", len(p.entries)-end)
	}
	b.WriteString("\nenter add · esc cancel")
	return b.String()
}
