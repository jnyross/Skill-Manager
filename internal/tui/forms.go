package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"skillet/internal/engine"
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
}

func newTextForm(kind formKind, fields []string) *textForm {
	in := textinput.New()
	in.Focus()
	in.CharLimit = 512
	in.Width = 60
	return &textForm{kind: kind, fields: fields, values: make([]string, len(fields)), input: in}
}

func (f *textForm) update(msg tea.KeyMsg) (done, canceled bool) {
	switch msg.String() {
	case "esc":
		return false, true
	case "enter":
		f.values[f.index] = strings.TrimSpace(f.input.Value())
		if f.index == len(f.fields)-1 {
			return true, false
		}
		f.index++
		f.input.SetValue(f.values[f.index])
		return false, false
	}
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	_ = cmd
	return false, false
}

func (f *textForm) render() string {
	return fmt.Sprintf("%s\n\n%s\n\nenter next/save · esc cancel", f.fields[f.index], f.input.View())
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

func renderMemberPicker(p *memberPicker) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Add to Bundle %q:\n", p.bundle.Name)
	for i, entry := range p.entries {
		marker := " "
		if i == p.cursor {
			marker = ">"
		}
		fmt.Fprintf(&b, "%s %s\n", marker, entry.Name)
	}
	b.WriteString("\nenter add · esc cancel")
	return b.String()
}
