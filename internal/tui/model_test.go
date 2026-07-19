package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

func TestPendingInstallReturnsCmdAndSetsFlag(t *testing.T) {
	m := NewModel(engine.New(engine.Roots{}))
	m.pending = &pendingConfirm{
		action: pendingInstall,
		entry:  engine.LibraryEntry{Name: "my-skill"},
		target: engine.InstallTarget{Kind: engine.InstallTargetPersonal},
	}

	cmd := m.executePending()
	if cmd == nil {
		t.Fatal("executePending returned nil cmd for install")
	}
	if m.installing != "my-skill" {
		t.Fatalf("installing = %q, want my-skill", m.installing)
	}

	msg := cmd()
	if _, ok := msg.(installStartedMsg); !ok {
		t.Fatalf("first message = %T, want installStartedMsg", msg)
	}
}

func TestPendingInstallBundleReturnsCmdAndSetsFlag(t *testing.T) {
	m := NewModel(engine.New(engine.Roots{}))
	m.pending = &pendingConfirm{
		action: pendingInstallBundle,
		bundle: engine.Bundle{Name: "my-bundle"},
		target: engine.InstallTarget{Kind: engine.InstallTargetPersonal},
	}

	cmd := m.executePending()
	if cmd == nil {
		t.Fatal("executePending returned nil cmd for bundle install")
	}
	if m.installing != "my-bundle" {
		t.Fatalf("installing = %q, want my-bundle", m.installing)
	}
}

func TestInstallStartedMsgSetsStatus(t *testing.T) {
	m := NewModel(engine.New(engine.Roots{}))
	m.installWork = func() tea.Msg { return installFinishedMsg{} }

	_, cmd := m.Update(installStartedMsg{desc: "my-skill"})
	if m.installing != "my-skill" {
		t.Fatalf("installing = %q, want my-skill", m.installing)
	}
	if !strings.Contains(m.status, "Installing my-skill") {
		t.Fatalf("status = %q, want Installing my-skill", m.status)
	}
	if cmd == nil {
		t.Fatal("Update did not return work cmd")
	}
}

func TestInstallFinishedMsgClearsFlagAndSetsStatus(t *testing.T) {
	m := NewModel(engine.New(engine.Roots{}))
	m.installing = "my-skill"

	m.Update(installFinishedMsg{
		desc:   "my-skill",
		target: engine.InstallTarget{Kind: engine.InstallTargetPersonal},
	})
	if m.installing != "" {
		t.Fatalf("installing = %q, want empty", m.installing)
	}
	if !strings.Contains(m.status, "Installed \"my-skill\"") {
		t.Fatalf("status = %q, want Installed my-skill", m.status)
	}
}

func TestInstallFinishedMsgErrorSetsStatus(t *testing.T) {
	m := NewModel(engine.New(engine.Roots{}))
	m.installing = "my-skill"

	m.Update(installFinishedMsg{
		err:    errors.New("boom"),
		desc:   "my-skill",
		target: engine.InstallTarget{Kind: engine.InstallTargetPersonal},
	})
	if m.installing != "" {
		t.Fatalf("installing = %q, want empty", m.installing)
	}
	if !strings.Contains(m.status, "failed") {
		t.Fatalf("status = %q, want failed", m.status)
	}
}

func TestStartInstallRejectsConcurrentInstall(t *testing.T) {
	m := NewModel(engine.New(engine.Roots{}))
	m.installing = "other"
	m.pending = &pendingConfirm{
		action: pendingInstall,
		entry:  engine.LibraryEntry{Name: "my-skill"},
		target: engine.InstallTarget{Kind: engine.InstallTargetPersonal},
	}

	cmd := m.executePending()
	if cmd != nil {
		t.Fatal("executePending should return nil when install already in progress")
	}
	if !strings.Contains(m.status, "already in progress") {
		t.Fatalf("status = %q, want already in progress", m.status)
	}
}

func TestIsStatusError(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"Install failed: something broke", true},
		{"No skill selected.", true},
		{"Select a Bundle first.", true},
		{"Uninstall plugin is only available for Plugin skills.", true},
		{"Installed \"foo\" → Personal.", false},
		{"Canceled.", false},
		{"Opening Setup…", false},
		{"Library is empty.", false},
	}

	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			if got := isStatusError(tc.status); got != tc.want {
				t.Fatalf("isStatusError(%q) = %t, want %t", tc.status, got, tc.want)
			}
		})
	}
}

func TestPluginUninstallConfirmationDisclosesPermanentDeletion(t *testing.T) {
	m := NewModel(engine.New(engine.Roots{}))
	m.inv = engine.Inventory{Skills: []engine.Skill{
		{Name: "alpha", Source: engine.SourcePlugin, Plugin: &engine.PluginInfo{Plugin: "example", Marketplace: "marketplace", SkillCount: 2}},
		{Name: "beta", Source: engine.SourcePlugin, Plugin: &engine.PluginInfo{Plugin: "example", Marketplace: "marketplace", SkillCount: 2}},
	}}
	_ = m.list.SetItems(buildListItems(m.inv))
	m.selectMainCursor()
	m.refreshDetail()

	pressTUIKey(m, "x")
	if m.pending == nil {
		t.Fatal("expected pending confirmation")
	}
	desc := m.pending.description
	if !strings.Contains(desc, "permanently deletes") {
		t.Fatalf("description missing permanent deletion disclosure: %q", desc)
	}
	if !strings.Contains(desc, "not an Archive") {
		t.Fatalf("description missing archive clarification: %q", desc)
	}
}

func TestMainListNavigationPagesAndJumps(t *testing.T) {
	m := NewModel(engine.New(engine.Roots{}))
	skills := make([]engine.Skill, 25)
	for i := 0; i < 25; i++ {
		skills[i] = engine.Skill{Name: fmt.Sprintf("skill-%02d", i), Source: engine.SourcePersonal}
	}
	m.inv = engine.Inventory{Skills: skills}
	_ = m.list.SetItems(buildListItems(m.inv))
	m.selectMainCursor()
	m.refreshDetail()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	pageSize := m.mainPageSize()
	if pageSize <= 0 {
		t.Fatalf("page size = %d", pageSize)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.cursor != pageSize {
		t.Fatalf("after pgdown cursor = %d, want %d", m.cursor, pageSize)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if m.cursor != len(m.inv.Skills)-1 {
		t.Fatalf("after end cursor = %d, want %d", m.cursor, len(m.inv.Skills)-1)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyHome})
	if m.cursor != 0 {
		t.Fatalf("after home cursor = %d, want 0", m.cursor)
	}

	m.cursor = len(m.inv.Skills) - 1
	m.selectMainCursor()
	m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	want := len(m.inv.Skills) - 1 - pageSize
	if want < 0 {
		want = 0
	}
	if m.cursor != want {
		t.Fatalf("after pgup from end cursor = %d, want %d", m.cursor, want)
	}
}

func TestMainSearchJumpsToMatchingSkill(t *testing.T) {
	m := NewModel(engine.New(engine.Roots{}))
	m.inv = engine.Inventory{Skills: []engine.Skill{
		{Name: "alpha", Source: engine.SourcePersonal},
		{Name: "beta", Source: engine.SourcePersonal},
		{Name: "gamma", Source: engine.SourcePersonal},
	}}
	_ = m.list.SetItems(buildListItems(m.inv))
	m.selectMainCursor()
	m.refreshDetail()

	pressTUIKey(m, "/")
	if !m.searching {
		t.Fatal("expected search mode")
	}

	typeTUIText(m, "e")
	if m.cursor != 1 {
		t.Fatalf("after 'e' cursor = %d, want 1 (beta)", m.cursor)
	}

	typeTUIText(m, "T")
	if m.cursor != 1 {
		t.Fatalf("after 'eT' cursor = %d, want 1 (beta)", m.cursor)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.searching {
		t.Fatal("expected search canceled")
	}
}
