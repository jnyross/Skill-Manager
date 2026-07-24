package tui

import (
	"strings"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

// markSet is the main view's multi-selection: the Skills the next bulk action
// applies to. It is keyed by identity rather than by list index so a mark
// survives everything that reorders or hides rows — filtering, the cost sort,
// and cursor movement — which is the whole point of marking: filter, mark,
// clear the filter, filter again, mark more, then act once.
//
// The Model holds one markSet and hands the same pointer to the list delegate,
// so the rendered row and the pending action can never disagree about what is
// marked.
type markSet struct {
	keys map[string]bool
}

func newMarkSet() *markSet {
	return &markSet{keys: make(map[string]bool)}
}

// skillMarkKey identifies a Skill across an inventory re-read. Location alone is
// not enough — a Codex prompt and a Claude skill can share a directory, and a
// Skill with no Location would collide with every other one — so the key is the
// whole coordinate: which Source and Tool govern it, what it is called, and
// where it lives.
func skillMarkKey(skill engine.Skill) string {
	return strings.Join([]string{
		string(skill.Source),
		string(skill.Tool),
		string(skill.Kind),
		skill.Name,
		skill.Location,
	}, "\x00")
}

func (s *markSet) has(skill engine.Skill) bool {
	if s == nil || len(s.keys) == 0 {
		return false
	}
	return s.keys[skillMarkKey(skill)]
}

// toggle marks or unmarks skill and reports the state it left behind.
func (s *markSet) toggle(skill engine.Skill) bool {
	key := skillMarkKey(skill)
	if s.keys[key] {
		delete(s.keys, key)
		return false
	}
	s.keys[key] = true
	return true
}

func (s *markSet) add(skill engine.Skill) {
	s.keys[skillMarkKey(skill)] = true
}

func (s *markSet) len() int {
	if s == nil {
		return 0
	}
	return len(s.keys)
}

// clear drops every mark and reports how many there were, so the caller can say
// what it just did rather than silently emptying the selection.
func (s *markSet) clear() int {
	if s == nil {
		return 0
	}
	count := len(s.keys)
	s.keys = make(map[string]bool)
	return count
}

// markedSkills is the marked Skills in inventory order. It reads from the
// inventory rather than storing Skill values, so the action always operates on
// the freshest scan of each marked Skill, and a mark whose Skill has vanished
// from disk simply drops out instead of failing later.
func (m *Model) markedSkills() []engine.Skill {
	if m.marks.len() == 0 {
		return nil
	}
	marked := make([]engine.Skill, 0, m.marks.len())
	for _, skill := range m.inv.Skills {
		if m.marks.has(skill) {
			marked = append(marked, skill)
		}
	}
	return marked
}

// toggleMark is the `space` action. Marking is gated by exactly the same rule
// as the single-skill `m` toggle: a Skill that cannot be made Manual-only is
// refused at mark time with the reason, rather than being accepted here and
// reported as a failure after the bulk action has already run.
func (m *Model) toggleMark() {
	selected, ok := m.selectedMainSkill()
	if !ok {
		m.setError("No skill selected.")
		return
	}
	if reason := manualOnlyUnavailableReason(selected); reason != "" {
		m.setError("Cannot mark " + selected.Name + ". " + reason)
		return
	}
	m.clearTransientStatus()
	if !m.marks.toggle(selected) && m.marks.len() == 0 {
		// The marked-count line disappears with the last mark, so this is the
		// one unmark that would otherwise leave no trace on screen.
		m.setStatus("No Skills marked.")
	}
}
