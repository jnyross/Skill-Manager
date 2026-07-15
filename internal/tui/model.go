package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"skillet/internal/engine"
)

type viewState int

const (
	mainView viewState = iota
	archiveView
	libraryView
	bundleView
)

type pendingAction int

const (
	pendingUninstall pendingAction = iota
	pendingRestore
	pendingPurge
	pendingSuppress
	pendingUnsuppress
	pendingManualOnly
	pendingAutoActivate
	pendingUninstallPlugin
	pendingInstall
	pendingInstallBundle
)

type pendingConfirm struct {
	description string
	action      pendingAction
	location    string
	id          string
	skill       engine.Skill
	plugin      engine.PluginInfo
	entry       engine.LibraryEntry
	target      engine.InstallTarget
	bundle      engine.Bundle
}

type Model struct {
	engine         *engine.Engine
	view           viewState
	cursor         int // inventory skill index for main view
	inv            engine.Inventory
	list           list.Model
	archiveList    list.Model
	libraryList    list.Model
	bundleList     list.Model
	help           help.Model
	archive        []engine.ArchiveEntry
	library        []engine.LibraryEntry
	bundles        []engine.Bundle
	bundleExpanded map[string]bool
	pending        *pendingConfirm
	installPicker  *installPicker
	sourcePicker   *librarySourcePicker
	form           *textForm
	memberPicker   *memberPicker
	status         string
	width          int
	height         int
	detail         detailPane
}

func NewModel(e *engine.Engine) *Model {
	m := &Model{
		engine:         e,
		list:           newSkillList(nil),
		archiveList:    newArchiveList(nil),
		libraryList:    newLibraryList(nil),
		bundleList:     newBundleList(nil),
		help:           help.New(),
		detail:         newDetailPane(),
		bundleExpanded: make(map[string]bool),
	}
	m.refreshInventory()
	return m
}

func (m *Model) Init() tea.Cmd {
	return tea.EnterAltScreen
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = size.Width
		m.height = size.Height
		m.resizeList()
		return m, nil
	}

	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if m.pending != nil {
		if strings.ToLower(key.String()) == "y" {
			m.executePending()
		} else {
			m.status = "Canceled."
		}
		m.pending = nil
		m.resizeList()
		return m, nil
	}

	if m.installPicker != nil {
		if key.String() == "ctrl+c" {
			return m, tea.Quit
		}
		m.updateInstallPicker(key.String())
		m.resizeList()
		return m, nil
	}
	if m.sourcePicker != nil {
		m.updateSourcePicker(key)
		m.resizeList()
		return m, nil
	}
	if m.form != nil {
		m.updateForm(key)
		m.resizeList()
		return m, nil
	}
	if m.memberPicker != nil {
		m.updateMemberPicker(key)
		m.resizeList()
		return m, nil
	}

	switch key.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "?":
		m.help.ShowAll = !m.help.ShowAll
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "pgup", "ctrl+u":
		if m.view == mainView {
			m.detail.scrollHalf(-1)
		}
	case "pgdown", "ctrl+d":
		if m.view == mainView {
			m.detail.scrollHalf(1)
		}
	default:
		switch m.view {
		case mainView:
			m.updateMain(key.String())
		case archiveView:
			m.updateArchive(key.String())
		case libraryView:
			m.updateLibrary(key.String())
		case bundleView:
			m.updateBundle(key.String())
		}
	}

	m.resizeList()
	return m, nil
}

func (m *Model) updateMain(key string) {
	switch key {
	case "u":
		selected, ok := m.selectedMainSkill()
		if !ok {
			m.status = "No skill selected."
			return
		}
		if reason := archiveUnavailableReason(selected); reason != "" {
			m.status = reason
			return
		}
		m.pending = &pendingConfirm{
			description: fmt.Sprintf("Archive %s %q? y to confirm, any other key to cancel.", selected.Source, selected.Name),
			action:      pendingUninstall,
			location:    selected.Location,
		}
	case "s":
		selected, ok := m.selectedMainSkill()
		if !ok {
			m.status = "No skill selected."
			return
		}
		if reason := suppressUnavailableReason(selected); reason != "" {
			m.status = reason
			return
		}
		if selected.Activation == engine.ActivationSuppressed || selected.Activation == engine.ActivationDisabled {
			m.pending = &pendingConfirm{
				description: fmt.Sprintf("Un-suppress %q? y to confirm, any other key to cancel.", selected.Name),
				action:      pendingUnsuppress,
				skill:       selected,
			}
		} else {
			m.pending = &pendingConfirm{
				description: fmt.Sprintf("Suppress %q? Hides it from the model and slash menu; plugin stays installed. y to confirm, any other key to cancel.", selected.Name),
				action:      pendingSuppress,
				skill:       selected,
			}
		}
	case "x":
		selected, ok := m.selectedMainSkill()
		if !ok {
			m.status = "No skill selected."
			return
		}
		if !canUninstallPlugin(selected) {
			m.status = "Uninstall plugin is only available for Plugin skills."
			return
		}
		names := pluginSkillNames(m.inv.Skills, *selected.Plugin)
		m.pending = &pendingConfirm{
			description: fmt.Sprintf("Uninstall plugin %q (%s@%s)? This removes all %d skills: %s. y to confirm, any other key to cancel.",
				selected.Plugin.Plugin, selected.Plugin.Plugin, selected.Plugin.Marketplace, len(names), strings.Join(names, ", ")),
			action: pendingUninstallPlugin,
			plugin: *selected.Plugin,
		}
	case "m":
		selected, ok := m.selectedMainSkill()
		if !ok {
			m.status = "No skill selected."
			return
		}
		if reason := manualOnlyUnavailableReason(selected); reason != "" {
			m.status = reason
			return
		}
		if selected.Activation == engine.ActivationManualOnly {
			m.pending = &pendingConfirm{
				description: fmt.Sprintf("Turn Auto-activation back on for %q? y to confirm, any other key to cancel.", selected.Name),
				action:      pendingAutoActivate,
				skill:       selected,
			}
		} else {
			m.pending = &pendingConfirm{
				description: fmt.Sprintf("Make %q Manual-only? It will only run when explicitly invoked. y to confirm, any other key to cancel.", selected.Name),
				action:      pendingManualOnly,
				skill:       selected,
			}
		}
	case "a":
		m.view = archiveView
		m.refreshArchive()
	case "L":
		m.view = libraryView
		m.refreshLibrary()
	case "B":
		m.view = bundleView
		m.refreshBundles()
	case "l":
		m.toggleLibraryMembership()
	}
}

func (m *Model) updateArchive(key string) {
	switch key {
	case "a", "esc":
		m.view = mainView
		m.cursor = 0
		m.refreshInventory()
	case "r":
		selected, ok := m.selectedArchiveEntry()
		if !ok {
			m.status = "No archive entry selected."
			return
		}
		m.pending = &pendingConfirm{
			description: fmt.Sprintf("Restore %q to %s? y to confirm, any other key to cancel.", selected.Name, selected.OriginalLocation),
			action:      pendingRestore,
			id:          selected.ID,
		}
	case "p":
		selected, ok := m.selectedArchiveEntry()
		if !ok {
			m.status = "No archive entry selected."
			return
		}
		m.pending = &pendingConfirm{
			description: fmt.Sprintf("Purge %q permanently? y to confirm, any other key to cancel.", selected.Name),
			action:      pendingPurge,
			id:          selected.ID,
		}
	}
}

func (m *Model) updateLibrary(key string) {
	switch key {
	case "L", "esc":
		m.view = mainView
		m.cursor = 0
		m.refreshInventory()
	case "d":
		selected, ok := m.selectedLibraryEntry()
		if !ok {
			m.status = "No Library entry selected."
			return
		}
		if err := m.engine.RemoveLibraryEntry(selected.ID); err != nil {
			m.status = "Remove from Library failed: " + err.Error()
			return
		}
		m.status = "Removed " + selected.Name + " from Library."
		m.refreshLibrary()
	case "i":
		m.beginLibraryInstall()
	case "n":
		m.sourcePicker = &librarySourcePicker{}
	}
}

func (m *Model) updateBundle(key string) {
	row, selected := m.selectedBundleItem()
	switch key {
	case "B", "esc":
		m.view = mainView
		m.refreshInventory()
	case "n":
		m.form = newTextForm(formBundleName, []string{"Bundle name"})
	case "enter", " ":
		if !selected || row.member != nil {
			return
		}
		m.bundleExpanded[row.bundle.ID] = !m.bundleExpanded[row.bundle.ID]
		m.refreshBundles()
	case "d":
		if !selected || row.member != nil {
			m.status = "Select a Bundle to delete."
			return
		}
		if err := m.engine.DeleteBundle(row.bundle.ID); err != nil {
			m.status = "Delete Bundle failed: " + err.Error()
		} else {
			m.status = "Deleted Bundle " + row.bundle.Name + "."
			m.refreshBundles()
		}
	case "a":
		if !selected {
			m.status = "Select a Bundle or member first."
			return
		}
		entries, err := m.engine.ListLibrary()
		if err != nil {
			m.status = "Library read failed: " + err.Error()
			return
		}
		if len(entries) == 0 {
			m.status = "Library is empty."
			return
		}
		m.memberPicker = &memberPicker{bundle: row.bundle, entries: entries}
	case "r":
		if !selected || row.member == nil {
			m.status = "Select a Bundle member to remove."
			return
		}
		if err := m.engine.RemoveBundleMember(row.bundle.ID, row.member.LibraryEntryID); err != nil {
			m.status = "Remove member failed: " + err.Error()
		} else {
			m.status = "Removed member from Bundle."
			m.refreshBundles()
		}
	case "m":
		if !selected || row.member == nil {
			m.status = "Select a Bundle member to change Activation."
			return
		}
		next := engine.ActivationManualOnly
		if row.member.Activation == engine.ActivationManualOnly {
			next = engine.ActivationAuto
		}
		if err := m.engine.SetBundleMemberActivation(row.bundle.ID, row.member.LibraryEntryID, next); err != nil {
			m.status = "Activation failed: " + err.Error()
		} else {
			m.status = "Bundle member Activation: " + string(next)
			m.refreshBundles()
		}
	case "i":
		if !selected {
			m.status = "Select a Bundle first."
			return
		}
		m.beginBundleInstall(row.bundle)
	}
}

func (m *Model) beginLibraryInstall() {
	selected, ok := m.selectedLibraryEntry()
	if !ok {
		m.status = "No Library entry selected."
		return
	}
	options := buildInstallTargetOptions(m.engine)
	if len(options) == 0 {
		m.status = "No Install targets available."
		return
	}
	m.installPicker = &installPicker{
		entry:   selected,
		options: options,
		cursor:  0,
	}
	m.status = ""
}

func (m *Model) beginBundleInstall(bundle engine.Bundle) {
	options := buildInstallTargetOptions(m.engine)
	m.installPicker = &installPicker{bundle: &bundle, options: options}
	m.status = ""
}

func (m *Model) updateInstallPicker(key string) {
	picker := m.installPicker
	if picker == nil {
		return
	}
	switch key {
	case "esc", "q":
		m.installPicker = nil
		m.status = "Canceled."
	case "up", "k":
		if picker.cursor > 0 {
			picker.cursor--
		}
	case "down", "j":
		if picker.cursor < len(picker.options)-1 {
			picker.cursor++
		}
	case "enter":
		opt := picker.options[picker.cursor]
		if picker.bundle != nil {
			bundle := *picker.bundle
			m.installPicker = nil
			m.confirmOrRunBundleInstall(bundle, opt.target)
			return
		}
		entry := picker.entry
		m.installPicker = nil
		m.confirmOrRunInstall(entry, opt.target)
	}
}

func (m *Model) confirmOrRunBundleInstall(bundle engine.Bundle, target engine.InstallTarget) {
	entries, err := m.engine.ListLibrary()
	if err != nil {
		m.status = "Install Bundle failed: " + err.Error()
		return
	}
	byID := make(map[string]engine.LibraryEntry, len(entries))
	for _, entry := range entries {
		byID[entry.ID] = entry
	}
	var collisions []string
	for _, member := range bundle.Members {
		entry, ok := byID[member.LibraryEntryID]
		if !ok || entry.Source.Kind == engine.LibrarySourceMarketplace {
			continue
		}
		if entry.Source.Kind == engine.LibrarySourceSkillsSh && entry.Source.SkillsShSkill == "" {
			collisions = append(collisions, "all skills from "+entry.Source.SkillsShRepo+" (matching names)")
			continue
		}
		dest, exists, err := m.engine.InstallDestination(entry, target)
		if err != nil {
			m.status = "Install Bundle failed: " + err.Error()
			return
		}
		if exists {
			collisions = append(collisions, dest)
		}
	}
	if len(collisions) > 0 {
		m.pending = &pendingConfirm{description: fmt.Sprintf("Install Bundle %q will replace: %s. y to confirm, any other key to cancel.", bundle.Name, strings.Join(collisions, ", ")), action: pendingInstallBundle, bundle: bundle, target: target}
		return
	}
	m.runBundleInstall(bundle, target)
}

func (m *Model) runBundleInstall(bundle engine.Bundle, target engine.InstallTarget) {
	if err := m.engine.InstallBundle(bundle.ID, target); err != nil {
		m.status = "Install Bundle failed: " + err.Error()
		return
	}
	m.status = fmt.Sprintf("Installed Bundle %q.", bundle.Name)
}

func (m *Model) confirmOrRunInstall(entry engine.LibraryEntry, target engine.InstallTarget) {
	if entry.Source.Kind == engine.LibrarySourceMarketplace {
		m.runInstall(entry, target)
		return
	}
	dest, exists, err := m.engine.InstallDestination(entry, target)
	if err != nil {
		m.status = "Install failed: " + err.Error()
		return
	}
	if entry.Source.Kind == engine.LibrarySourceSkillsSh && entry.Source.SkillsShSkill == "" {
		m.pending = &pendingConfirm{description: fmt.Sprintf("Install every skill from %q? Existing skills with matching names may be replaced. y to confirm, any other key to cancel.", entry.Source.SkillsShRepo), action: pendingInstall, entry: entry, target: target}
		return
	}
	if exists {
		m.pending = &pendingConfirm{
			description: fmt.Sprintf("Replace existing skill at %s with Library entry %q? y to confirm, any other key to cancel.", dest, entry.Name),
			action:      pendingInstall,
			entry:       entry,
			target:      target,
		}
		return
	}
	m.runInstall(entry, target)
}

func (m *Model) updateSourcePicker(key tea.KeyMsg) {
	p := m.sourcePicker
	switch key.String() {
	case "esc":
		m.sourcePicker = nil
		m.status = "Canceled."
	case "up", "k":
		if p.cursor > 0 {
			p.cursor--
		}
	case "down", "j":
		if p.cursor < len(librarySourceChoices)-1 {
			p.cursor++
		}
	case "enter":
		source := librarySourceChoices[p.cursor]
		m.sourcePicker = nil
		var fields []string
		switch source {
		case engine.LibrarySourceLocalPath:
			fields = []string{"Name", "Tool (claude-code or codex)", "Local path"}
		case engine.LibrarySourceGit:
			fields = []string{"Name", "Tool (claude-code or codex)", "Git URL", "Git ref (optional)", "Git subpath (optional)"}
		case engine.LibrarySourceSkillsSh:
			fields = []string{"Name", "Tool (claude-code or codex)", "owner/repo", "Skill name (optional; blank means all)"}
		case engine.LibrarySourceMarketplace:
			fields = []string{"Name", "Marketplace name", "Plugin name", "Marketplace source (optional)"}
		}
		m.form = newTextForm(formLibraryEntry, fields)
		m.form.source = source
	}
}

func (m *Model) updateForm(key tea.KeyMsg) {
	done, canceled := m.form.update(key)
	if canceled {
		m.form = nil
		m.status = "Canceled."
		return
	}
	if !done {
		return
	}
	f := m.form
	m.form = nil
	if f.kind == formBundleName {
		if _, err := m.engine.CreateBundle(f.values[0]); err != nil {
			m.status = "Create Bundle failed: " + err.Error()
		} else {
			m.status = "Created Bundle."
			m.refreshBundles()
		}
		return
	}
	entry := engine.LibraryEntry{Name: f.values[0], Kind: engine.KindSkill}
	switch f.source {
	case engine.LibrarySourceLocalPath:
		entry.Tool = parseTool(f.values[1])
		entry.Source = engine.LibrarySource{Kind: f.source, LocalPath: f.values[2]}
	case engine.LibrarySourceGit:
		entry.Tool = parseTool(f.values[1])
		entry.Source = engine.LibrarySource{Kind: f.source, GitURL: f.values[2], GitRef: f.values[3], GitSubPath: f.values[4]}
	case engine.LibrarySourceSkillsSh:
		entry.Tool = parseTool(f.values[1])
		entry.Source = engine.LibrarySource{Kind: f.source, SkillsShRepo: f.values[2], SkillsShSkill: f.values[3]}
	case engine.LibrarySourceMarketplace:
		entry.Kind = ""
		entry.Source = engine.LibrarySource{Kind: f.source, Marketplace: f.values[1], PluginName: f.values[2], MarketplaceSource: f.values[3]}
	}
	if _, err := m.engine.AddLibraryEntry(entry); err != nil {
		m.status = "Add Library entry failed: " + err.Error()
	} else {
		m.status = "Added " + entry.Name + " to Library."
		m.refreshLibrary()
	}
}

func parseTool(value string) engine.Tool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "claude", "claude-code", "claude code":
		return engine.ToolClaudeCode
	case "codex":
		return engine.ToolCodex
	default:
		return engine.Tool(value)
	}
}

func (m *Model) updateMemberPicker(key tea.KeyMsg) {
	p := m.memberPicker
	switch key.String() {
	case "esc":
		m.memberPicker = nil
		m.status = "Canceled."
	case "up", "k":
		if p.cursor > 0 {
			p.cursor--
		}
	case "down", "j":
		if p.cursor < len(p.entries)-1 {
			p.cursor++
		}
	case "enter":
		entry := p.entries[p.cursor]
		m.memberPicker = nil
		if err := m.engine.AddBundleMember(p.bundle.ID, entry.ID, engine.ActivationAuto); err != nil {
			m.status = "Add Bundle member failed: " + err.Error()
		} else {
			m.status = "Added " + entry.Name + " to Bundle."
			m.refreshBundles()
		}
	}
}

func (m *Model) runInstall(entry engine.LibraryEntry, target engine.InstallTarget) {
	// Library view Install defaults to Auto; Bundle Install passes each
	// member's remembered Activation through Engine.InstallBundle.
	if err := m.engine.InstallLibraryEntry(entry, target, engine.ActivationAuto); err != nil {
		m.status = "Install failed: " + err.Error()
		return
	}
	where := "Personal"
	if target.Kind == engine.InstallTargetProject {
		where = target.RepoRoot
	}
	m.status = fmt.Sprintf("Installed %q → %s.", entry.Name, where)
}

func (m *Model) toggleLibraryMembership() {
	selected, ok := m.selectedMainSkill()
	if !ok {
		m.status = "No skill selected."
		return
	}
	if reason := libraryToggleUnavailableReason(selected); reason != "" {
		m.status = reason
		return
	}
	if selected.Source == engine.SourcePlugin && selected.Plugin != nil {
		entries, err := m.engine.ListLibrary()
		if err != nil {
			m.status = "Library read failed: " + err.Error()
			return
		}
		for _, entry := range entries {
			if entry.Source.Kind == engine.LibrarySourceMarketplace && entry.Source.Marketplace == selected.Plugin.Marketplace && entry.Source.PluginName == selected.Plugin.Plugin {
				if err := m.engine.RemoveLibraryEntry(entry.ID); err != nil {
					m.status = "Remove from Library failed: " + err.Error()
				} else {
					m.status = "Removed plugin " + selected.Plugin.Plugin + " from Library."
				}
				return
			}
		}
		entry, err := m.engine.AddLibraryEntry(engine.LibraryEntry{Name: selected.Plugin.Plugin, Source: engine.LibrarySource{Kind: engine.LibrarySourceMarketplace, Marketplace: selected.Plugin.Marketplace, PluginName: selected.Plugin.Plugin}})
		if err != nil {
			m.status = "Add to Library failed: " + err.Error()
		} else {
			m.status = "Added plugin " + entry.Name + " to Library."
		}
		return
	}
	if existing, found := m.engine.FindLibraryEntryByLocalPath(selected.Location); found {
		if err := m.engine.RemoveLibraryEntry(existing.ID); err != nil {
			m.status = "Remove from Library failed: " + err.Error()
			return
		}
		m.status = "Removed " + selected.Name + " from Library."
		return
	}
	entry, err := m.engine.AddLibraryEntry(engine.LibraryEntry{
		Name: selected.Name,
		Kind: selected.Kind,
		Tool: selected.Tool,
		Source: engine.LibrarySource{
			Kind:      engine.LibrarySourceLocalPath,
			LocalPath: selected.Location,
		},
	})
	if err != nil {
		m.status = "Add to Library failed: " + err.Error()
		return
	}
	m.status = "Added " + entry.Name + " to Library."
}

func (m *Model) executePending() {
	switch m.pending.action {
	case pendingUninstall:
		entry, err := m.engine.Uninstall(m.pending.location)
		if err != nil {
			m.status = "Archive failed: " + err.Error()
			return
		}
		m.status = "Archived " + entry.Name + "."
		m.refreshInventory()
	case pendingRestore:
		if err := m.engine.Restore(m.pending.id); err != nil {
			m.status = "Restore failed: " + err.Error()
			return
		}
		m.status = "Restored archive entry."
		m.refreshArchive()
	case pendingPurge:
		if err := m.engine.Purge(m.pending.id); err != nil {
			m.status = "Purge failed: " + err.Error()
			return
		}
		m.status = "Purged archive entry."
		m.refreshArchive()
	case pendingSuppress:
		if err := m.engine.Suppress(m.pending.skill); err != nil {
			m.status = "Suppress failed: " + err.Error()
			return
		}
		m.status = "Suppressed " + m.pending.skill.Name + "."
		if needsCodexRestartHint(m.pending.skill) {
			m.status += " Restart Codex to pick up the change."
		}
		m.refreshInventory()
	case pendingUnsuppress:
		if err := m.engine.Unsuppress(m.pending.skill); err != nil {
			m.status = "Un-suppress failed: " + err.Error()
			return
		}
		m.status = "Un-suppressed " + m.pending.skill.Name + "."
		if needsCodexRestartHint(m.pending.skill) {
			m.status += " Restart Codex to pick up the change."
		}
		m.refreshInventory()
	case pendingManualOnly:
		if err := m.engine.SetManualOnly(m.pending.skill, true); err != nil {
			m.status = "Manual-only failed: " + err.Error()
			return
		}
		m.status = "Made " + m.pending.skill.Name + " Manual-only."
		m.refreshInventory()
	case pendingAutoActivate:
		if err := m.engine.SetManualOnly(m.pending.skill, false); err != nil {
			m.status = "Auto-activation failed: " + err.Error()
			return
		}
		m.status = "Restored Auto-activation for " + m.pending.skill.Name + "."
		m.refreshInventory()
	case pendingUninstallPlugin:
		plugin := m.pending.plugin
		if err := m.engine.UninstallPlugin(plugin); err != nil {
			m.status = "Uninstall plugin failed: " + err.Error()
			return
		}
		m.status = "Uninstalled plugin " + plugin.Plugin + "."
		m.refreshInventory()
	case pendingInstall:
		m.runInstall(m.pending.entry, m.pending.target)
	case pendingInstallBundle:
		m.runBundleInstall(m.pending.bundle, m.pending.target)
	}
}

// pluginSkillNames returns the names of every skill in skills belonging to
// plugin (matched by Marketplace+Plugin), for the Uninstall-plugin
// confirmation to list every skill about to be removed (issue #10's
// acceptance criterion: "the confirmation lists all N skills in the plugin
// before proceeding"). Built client-side from the Inventory() result
// already held by the model, rather than a new engine listing method.
func pluginSkillNames(skills []engine.Skill, plugin engine.PluginInfo) []string {
	var names []string
	for _, skill := range skills {
		if skill.Source == engine.SourcePlugin && skill.Plugin != nil &&
			skill.Plugin.Marketplace == plugin.Marketplace && skill.Plugin.Plugin == plugin.Plugin {
			names = append(names, skill.Name)
		}
	}
	return names
}

func (m *Model) moveCursor(delta int) {
	switch m.view {
	case mainView:
		m.moveMainCursor(delta)
	case archiveView:
		m.moveListCursor(&m.archiveList, delta)
	case libraryView:
		m.moveListCursor(&m.libraryList, delta)
	case bundleView:
		m.moveListCursor(&m.bundleList, delta)
	}
}

func (m *Model) moveListCursor(l *list.Model, delta int) {
	items := l.Items()
	if len(items) == 0 {
		return
	}
	index := l.Index() + delta
	if index < 0 {
		index = 0
	}
	if index >= len(items) {
		index = len(items) - 1
	}
	l.Select(index)
}

func (m *Model) moveMainCursor(delta int) {
	items := m.list.Items()
	if len(items) == 0 {
		m.cursor = 0
		return
	}

	index := m.list.Index()
	for {
		index += delta
		if index < 0 || index >= len(items) {
			break
		}
		if _, ok := items[index].(skillItem); ok {
			m.list.Select(index)
			break
		}
	}
	m.syncMainCursor()
	m.refreshDetail()
}

func (m *Model) refreshInventory() {
	m.inv = m.engine.Inventory()
	if m.cursor >= len(m.inv.Skills) {
		m.cursor = max(0, len(m.inv.Skills)-1)
	}
	_ = m.list.SetItems(buildListItems(m.inv))
	m.selectMainCursor()
	m.refreshDetail()
	m.resizeList()
}

func (m *Model) refreshArchive() {
	entries, err := m.engine.ListArchive()
	if err != nil {
		m.archive = nil
		_ = m.archiveList.SetItems(nil)
		m.status = "Archive read failed: " + err.Error()
		return
	}
	m.archive = entries
	_ = m.archiveList.SetItems(buildArchiveItems(m.archive))
	if len(m.archive) == 0 {
		m.archiveList.Select(0)
		return
	}
	index := m.archiveList.Index()
	if index >= len(m.archive) {
		index = len(m.archive) - 1
	}
	m.archiveList.Select(index)
}

func (m *Model) refreshLibrary() {
	entries, err := m.engine.ListLibrary()
	if err != nil {
		m.library = nil
		_ = m.libraryList.SetItems(nil)
		m.status = "Library read failed: " + err.Error()
		return
	}
	m.library = entries
	_ = m.libraryList.SetItems(buildLibraryItems(m.library))
	if len(m.library) == 0 {
		m.libraryList.Select(0)
		return
	}
	index := m.libraryList.Index()
	if index >= len(m.library) {
		index = len(m.library) - 1
	}
	m.libraryList.Select(index)
}

func (m *Model) refreshBundles() {
	bundles, err := m.engine.ListBundles()
	if err != nil {
		m.bundles = nil
		_ = m.bundleList.SetItems(nil)
		m.status = "Bundle read failed: " + err.Error()
		return
	}
	m.bundles = bundles
	library, err := m.engine.ListLibrary()
	if err != nil {
		m.status = "Library read failed: " + err.Error()
		return
	}
	_ = m.bundleList.SetItems(buildBundleItems(bundles, library, m.bundleExpanded))
}

func (m *Model) View() string {
	view := m.renderView()
	if m.pending != nil {
		return renderConfirmOverlay(view, m.pending.description, m.width, m.height)
	}
	if m.installPicker != nil {
		name := m.installPicker.entry.Name
		if m.installPicker.bundle != nil {
			name = m.installPicker.bundle.Name
		}
		desc := renderInstallPickerDescription(name, m.installPicker.options, m.installPicker.cursor)
		return renderConfirmOverlay(view, desc, m.width, m.height)
	}
	if m.sourcePicker != nil {
		return renderConfirmOverlay(view, renderLibrarySourcePicker(m.sourcePicker.cursor), m.width, m.height)
	}
	if m.form != nil {
		return renderConfirmOverlay(view, m.form.render(), m.width, m.height)
	}
	if m.memberPicker != nil {
		return renderConfirmOverlay(view, renderMemberPicker(m.memberPicker), m.width, m.height)
	}
	return view
}

func (m *Model) renderView() string {
	var b strings.Builder

	switch m.view {
	case archiveView:
		m.renderArchive(&b)
	case libraryView:
		m.renderLibrary(&b)
	case bundleView:
		m.renderBundles(&b)
	default:
		m.renderMain(&b)
	}

	if m.status != "" {
		b.WriteString("\n")
		b.WriteString(m.status)
		b.WriteString("\n")
	}
	return b.String()
}

func (m *Model) renderMain(b *strings.Builder) {
	b.WriteString("Skillet\n")
	b.WriteString(m.helpView())
	b.WriteString("\n\n")

	if len(m.inv.Skills) == 0 {
		b.WriteString("No skills found.\n")
	} else {
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, m.list.View(), " ", m.detail.render()))
		b.WriteString("\n")
	}

	if len(m.inv.Notices) > 0 {
		b.WriteString("\nNotices\n")
		for _, notice := range m.inv.Notices {
			b.WriteString("- ")
			b.WriteString(notice.Message)
			b.WriteString("\n")
		}
	}
}

func (m *Model) renderArchive(b *strings.Builder) {
	b.WriteString("Skillet Archive\n")
	b.WriteString(m.helpView())
	b.WriteString("\n\n")

	if len(m.archive) == 0 {
		b.WriteString("Archive is empty.\n")
		return
	}
	b.WriteString(m.archiveList.View())
	b.WriteString("\n")
}

func (m *Model) renderLibrary(b *strings.Builder) {
	b.WriteString("Skillet Library\n")
	b.WriteString(m.helpView())
	b.WriteString("\n\n")

	if len(m.library) == 0 {
		b.WriteString("Library is empty.\n")
		return
	}
	b.WriteString(m.libraryList.View())
	b.WriteString("\n")
}

func (m *Model) renderBundles(b *strings.Builder) {
	b.WriteString("Skillet Bundles\n")
	b.WriteString(m.helpView())
	b.WriteString("\n\n")
	if len(m.bundles) == 0 {
		b.WriteString("No Bundles yet. Press n to create one.\n")
		return
	}
	b.WriteString(m.bundleList.View())
	b.WriteString("\n")
}

func newSkillList(items []list.Item) list.Model {
	model := list.New(items, skillDelegate{}, 0, 0)
	model.SetShowTitle(false)
	model.SetShowFilter(false)
	model.SetFilteringEnabled(false)
	model.SetShowStatusBar(false)
	model.SetShowPagination(false)
	model.SetShowHelp(false)
	model.DisableQuitKeybindings()
	return model
}

func (m *Model) selectedMainSkill() (engine.Skill, bool) {
	item, ok := m.list.SelectedItem().(skillItem)
	if ok {
		return item.skill, true
	}
	if len(m.inv.Skills) == 0 {
		return engine.Skill{}, false
	}
	return m.inv.Skills[m.cursor], true
}

func (m *Model) selectedArchiveEntry() (engine.ArchiveEntry, bool) {
	item, ok := m.archiveList.SelectedItem().(archiveItem)
	if !ok {
		return engine.ArchiveEntry{}, false
	}
	return item.entry, true
}

func (m *Model) selectedLibraryEntry() (engine.LibraryEntry, bool) {
	item, ok := m.libraryList.SelectedItem().(libraryItem)
	if !ok {
		return engine.LibraryEntry{}, false
	}
	return item.entry, true
}

func (m *Model) selectedBundleItem() (bundleItem, bool) {
	item, ok := m.bundleList.SelectedItem().(bundleItem)
	return item, ok
}

func (m *Model) syncMainCursor() {
	selected, ok := m.list.SelectedItem().(skillItem)
	if !ok {
		m.cursor = 0
		return
	}
	for i, skill := range m.inv.Skills {
		if skill.Location == selected.skill.Location {
			m.cursor = i
			return
		}
	}
	m.cursor = 0
}

func (m *Model) selectMainCursor() {
	if len(m.inv.Skills) == 0 {
		m.cursor = 0
		m.list.Select(0)
		return
	}

	items := m.list.Items()
	skillIndex := 0
	for i, item := range items {
		if _, ok := item.(skillItem); !ok {
			continue
		}
		if skillIndex == m.cursor {
			m.list.Select(i)
			return
		}
		skillIndex++
	}
	m.list.Select(0)
	m.syncMainCursor()
}

func (m *Model) refreshDetail() {
	skill, ok := m.selectedMainSkill()
	m.detail.setSkill(skill, ok)
}

func (m *Model) resizeList() {
	width := m.width
	if width < 1 {
		width = 100
	}
	m.help.Width = width

	reserved := 3 + renderedLineCount(m.helpView()) // title, help, blank line, trailing newline after the list
	if m.view == mainView && len(m.inv.Notices) > 0 {
		reserved += 2 + len(m.inv.Notices) // blank line, "Notices" line, one per notice
	}
	if m.status != "" {
		reserved += 2 // blank line, status line
	}

	height := m.height - reserved
	if height < 1 {
		switch m.view {
		case archiveView:
			height = max(1, len(m.archiveList.Items()))
		case libraryView:
			height = max(1, len(m.libraryList.Items()))
		case bundleView:
			height = max(1, len(m.bundleList.Items()))
		default:
			height = max(1, len(m.list.Items()))
		}
	}

	switch m.view {
	case archiveView:
		m.archiveList.SetSize(width, height)
		return
	case libraryView:
		m.libraryList.SetSize(width, height)
		return
	case bundleView:
		m.bundleList.SetSize(width, height)
		return
	}

	listWidth, detailWidth := splitPaneWidths(width)
	m.list.SetSize(listWidth, height)
	m.detail.setSize(detailWidth, height)
	// Size change reclamps scroll; selection content is only replaced via
	// refreshDetail when the selected skill changes.
}

func (m *Model) helpView() string {
	switch m.view {
	case archiveView:
		return m.help.View(archiveKeyMap(len(m.archive) > 0, m.help.ShowAll))
	case libraryView:
		return m.help.View(libraryKeyMap(len(m.library) > 0, m.help.ShowAll))
	case bundleView:
		return m.help.View(bundleKeyMap(len(m.bundles) > 0, m.help.ShowAll))
	default:
		selected, ok := m.selectedMainSkill()
		return m.help.View(mainKeyMap(selected, ok, m.help.ShowAll))
	}
}

func renderedLineCount(s string) int {
	if s == "" {
		return 0
	}
	return len(strings.Split(s, "\n"))
}

func splitPaneWidths(width int) (int, int) {
	if width < 2 {
		return width, 0
	}

	gap := 1
	available := width - gap
	listWidth := available * 3 / 5
	if listWidth < 1 {
		listWidth = 1
	}
	detailWidth := available - listWidth
	if detailWidth < 1 {
		detailWidth = 1
		listWidth = width - gap - detailWidth
	}
	return listWidth, detailWidth
}
