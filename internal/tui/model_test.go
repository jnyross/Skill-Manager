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

func TestStatusLevelComesFromTheCallSiteNotTheText(t *testing.T) {
	m := NewModel(engine.New(engine.Roots{}))

	// A message beginning "No " used to be sniffed as an error by its text.
	// The level is now declared where the message is produced.
	m.setStatus("No changes were needed.")
	if m.statusLevel != statusInfo {
		t.Fatalf("setStatus level = %v, want statusInfo", m.statusLevel)
	}
	if !strings.Contains(m.renderView(), "No changes were needed.") {
		t.Fatal("informational status missing from the rendered view")
	}

	m.setError("Archive failed: boom")
	if m.statusLevel != statusError {
		t.Fatalf("setError level = %v, want statusError", m.statusLevel)
	}

	m.clearStatus()
	if m.status != "" || m.statusLevel != statusNone {
		t.Fatalf("clearStatus left %q at level %v", m.status, m.statusLevel)
	}
}

func TestErrorStatusSurvivesCursorMovementAndInfoDoesNot(t *testing.T) {
	m := NewModel(engine.New(engine.Roots{}))
	m.inv = engine.Inventory{Skills: []engine.Skill{
		{Name: "alpha", Source: engine.SourcePersonal, Location: "/a"},
		{Name: "beta", Source: engine.SourcePersonal, Location: "/b"},
	}}
	_ = m.list.SetItems(buildListItems(m.inv))
	m.selectMainCursor()

	m.setStatus("Archived alpha.")
	m.moveCursor(1)
	if m.status != "" {
		t.Fatalf("informational status survived a cursor move: %q", m.status)
	}

	m.setError("Archive failed: boom")
	m.moveCursor(1)
	m.moveCursor(-1)
	if m.status != "Archive failed: boom" {
		t.Fatalf("error status = %q, want it to persist until the next action", m.status)
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
