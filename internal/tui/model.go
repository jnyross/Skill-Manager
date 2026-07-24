package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jnyross/Skill-Manager/internal/engine"
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
	// pendingArchive is the Archive action (key `u`). CONTEXT.md's word for
	// this is Archive, not Uninstall — Uninstall belongs to plugins only.
	pendingArchive pendingAction = iota
	pendingRestore
	pendingPurge
	pendingSuppress
	pendingUnsuppress
	pendingManualOnly
	pendingAutoActivate
	pendingUninstallPlugin
	pendingInstall
	pendingInstallBundle
	pendingRemoveLibraryEntry
	pendingDeleteBundle
	pendingLibraryToggle
	pendingAddBundleMember
	pendingRemoveBundleMember
	pendingBundleMemberActivation
	// pendingBulkManualOnly is the marked-set action (key `M`): one
	// confirmation, one engine call, one report covering every marked Skill.
	pendingBulkManualOnly
)

type pendingConfirm struct {
	description string
	action      pendingAction
	location    string
	id          string
	skill       engine.Skill
	// skills is the marked set a bulk action applies to, captured at
	// confirmation time so what runs is exactly what was described.
	skills []engine.Skill
	plugin engine.PluginInfo
	entry  engine.LibraryEntry
	target engine.InstallTarget
	bundle engine.Bundle
	// memberID is the Library entry ID of the Bundle member an add / remove /
	// Activation confirmation applies to; memberName is its display name.
	memberID   string
	memberName string
	activation engine.ActivationState
}

// statusLevel classifies the status line explicitly at the call site, so
// rendering never has to guess from the message text what colour it should be.
type statusLevel int

const (
	statusNone statusLevel = iota
	statusInfo
	statusError
)

type installStartedMsg struct {
	desc string
	// step names the phase the TUI is about to dispatch. The engine reports no
	// intermediate progress, so every step label is Skillet's own knowledge of
	// what it asked for, never a reading of the child process.
	step string
}

type installFinishedMsg struct {
	err      error
	desc     string
	target   engine.InstallTarget
	isBundle bool
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
	archiveNotices []engine.Notice
	library        []engine.LibraryEntry
	bundles        []engine.Bundle
	bundleExpanded map[string]bool
	setupRequested bool
	pending        *pendingConfirm
	// marks is the main view's multi-selection. The list delegate holds this
	// same pointer, so a marked row and the pending bulk action always agree.
	marks *markSet
	// moreMenu is the secondary entry point (key `o`) for Library, Bundles, and
	// Setup. It renders as an overlay, so it costs the layout nothing.
	moreMenu      *moreMenu
	installPicker *installPicker
	sourcePicker  *librarySourcePicker
	form          *textForm
	memberPicker  *memberPicker
	status        string
	statusLevel   statusLevel
	installing    string
	installWork   func() tea.Msg
	spinner       spinner.Model
	// installStep is the phase label rendered beside the spinner; see
	// installStartedMsg.step for why it is TUI-derived.
	installStep string
	// installStarted is when the work command was dispatched, so a timeout can
	// be reported with the elapsed time the user actually waited.
	installStarted time.Time
	// installCancel cancels the context handed to the engine's *Context install
	// entry points, which is what makes esc and quit stop a running clone.
	installCancel context.CancelFunc
	// installCanceled records that the cancellation was the user's, so the
	// resulting engine error is reported as "cancelled" rather than a failure.
	installCanceled bool
	// quitAfterInstall defers the quit until the cancelled install has actually
	// returned, so Skillet never exits leaving an orphaned child process.
	quitAfterInstall bool
	width            int
	height           int
	detail           detailPane
	// sortByCost flips the main list between its Source grouping and a flat
	// ranking by estimated per-session cost. See toggleCostSort.
	sortByCost bool
	// filtering is true while keystrokes are being routed into the active
	// list's filter input. Once the filter is accepted the flag clears but the
	// list stays in list.FilterApplied until esc.
	filtering bool
	// queued collects commands produced by the Bubbles list (filter recompute,
	// cursor blink) from anywhere in an update so a single Update return can
	// hand them all back to the runtime.
	queued []tea.Cmd
}

func NewModel(e *engine.Engine) *Model {
	install := spinner.New()
	install.Spinner = spinner.Dot
	marks := newMarkSet()
	m := &Model{
		engine:         e,
		marks:          marks,
		list:           newSkillList(nil, marks),
		archiveList:    newArchiveList(nil),
		libraryList:    newLibraryList(nil),
		bundleList:     newBundleList(nil),
		help:           help.New(),
		detail:         newDetailPane(),
		bundleExpanded: make(map[string]bool),
		spinner:        install,
	}
	m.refreshInventory()
	return m
}

func (m *Model) SetupRequested() bool { return m.setupRequested }

// SetInitialStatus seeds the status line the TUI opens with. The setup
// round-trip uses it to report the Setup outcome when the wizard hands control
// back to the inventory.
func (m *Model) SetInitialStatus(text string, isError bool) {
	if text == "" {
		return
	}
	if isError {
		m.setError(text)
		return
	}
	m.setStatus(text)
}

func (m *Model) Init() tea.Cmd {
	return tea.EnterAltScreen
}

// setStatus records an informational outcome. It is cleared by the next cursor
// move.
func (m *Model) setStatus(text string) {
	m.status = text
	m.statusLevel = statusInfo
}

// setError records a failure or a refused action. Unlike an informational
// status it survives cursor movement and stays up until the next action, so a
// message cannot vanish before it has been read.
func (m *Model) setError(text string) {
	m.status = text
	m.statusLevel = statusError
}

func (m *Model) clearStatus() {
	m.status = ""
	m.statusLevel = statusNone
}

// clearTransientStatus clears an informational status but leaves an error in
// place.
func (m *Model) clearTransientStatus() {
	if m.statusLevel != statusError {
		m.clearStatus()
	}
}

func (m *Model) queue(cmd tea.Cmd) {
	if cmd != nil {
		m.queued = append(m.queued, cmd)
	}
}

func (m *Model) flush(cmd tea.Cmd) tea.Cmd {
	if len(m.queued) == 0 {
		return cmd
	}
	cmds := append(m.queued, cmd)
	m.queued = nil
	return tea.Batch(cmds...)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.update(msg)
	return m, m.flush(cmd)
}

func (m *Model) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeList()
		return nil
	case list.FilterMatchesMsg:
		// The fuzzy match ran off the main loop; hand the result to whichever
		// list is filtering and re-sync the selection.
		l := m.activeList()
		updated, cmd := l.Update(msg)
		*l = updated
		m.afterFilterChange()
		m.resizeList()
		return cmd
	case spinner.TickMsg:
		// Ticking stops the moment nothing is in flight, so an idle TUI does no
		// work per frame.
		if m.installing == "" {
			return nil
		}
		updated, cmd := m.spinner.Update(msg)
		m.spinner = updated
		return cmd
	case installStartedMsg:
		m.installing = msg.desc
		m.installStep = msg.step
		m.installStarted = time.Now()
		m.setStatus("Installing " + msg.desc + "…")
		work := m.installWork
		m.installWork = nil
		// The spinner tick and the work run together: the tick is what keeps the
		// screen visibly alive through a multi-second clone.
		return tea.Batch(m.spinner.Tick, work)
	case installFinishedMsg:
		return m.finishInstall(msg)
	}

	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}

	// The confirm modal is the one place ctrl+c must not quit: it is the
	// "get me out of here" key and cancelling is the safe reading of it.
	if m.pending != nil {
		var cmd tea.Cmd
		if strings.ToLower(key.String()) == "y" {
			cmd = m.executePending()
		} else {
			m.setStatus("Canceled.")
		}
		m.pending = nil
		m.resizeList()
		return cmd
	}

	// Everywhere else ctrl+c quits, including inside the pickers and the text
	// form, which previously swallowed it.
	if key.String() == "ctrl+c" {
		return m.quitOrCancelInstall()
	}

	if m.moreMenu != nil {
		m.updateMoreMenu(key.String())
		m.resizeList()
		if m.setupRequested {
			return tea.Quit
		}
		return nil
	}
	if m.installPicker != nil {
		cmd := m.updateInstallPicker(key.String())
		m.resizeList()
		return cmd
	}
	if m.sourcePicker != nil {
		m.updateSourcePicker(key)
		m.resizeList()
		return nil
	}
	if m.form != nil {
		m.updateForm(key)
		m.resizeList()
		return nil
	}
	if m.memberPicker != nil {
		m.updateMemberPicker(key)
		m.resizeList()
		return nil
	}

	if m.filtering {
		cmd := m.updateFilter(key)
		m.resizeList()
		return cmd
	}

	switch key.String() {
	case "q":
		return m.quitOrCancelInstall()
	case "?":
		m.help.ShowAll = !m.help.ShowAll
	case "/":
		m.beginFilter()
	case "esc":
		// While an install is in flight esc is the cancel key; it only falls
		// through to filter-clearing and view navigation once nothing is running.
		if m.installing != "" {
			m.cancelInstall()
			break
		}
		// esc first clears an applied filter; only once the list is unfiltered
		// does it fall through to the per-view meaning (back to main view).
		if m.clearActiveFilter() {
			break
		}
		// Then it drops the marked set. Marks outlive filtering by design, so
		// esc has to be able to let go of them too — otherwise the only way out
		// of a selection is to act on it.
		if m.view == mainView && m.marks.len() > 0 {
			m.setStatus(fmt.Sprintf("Cleared %s.", pluralCount(m.marks.clear(), "mark", "marks")))
			break
		}
		m.dispatchViewKey("esc")
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "pgup":
		if m.view == mainView {
			m.pageMainCursor(-1)
		}
	case "pgdown":
		if m.view == mainView {
			m.pageMainCursor(1)
		}
	case "ctrl+pgup", "ctrl+u":
		if m.view == mainView {
			m.detail.scrollHalf(-1)
		}
	case "ctrl+pgdown", "ctrl+d":
		if m.view == mainView {
			m.detail.scrollHalf(1)
		}
	case "home":
		if m.view == mainView {
			m.jumpMainCursor(false)
		}
	case "end":
		if m.view == mainView {
			m.jumpMainCursor(true)
		}
	default:
		if m.installing != "" && installBlocksKey(m.view, key.String()) {
			m.setError("Install in progress — press esc to cancel it first.")
			break
		}
		m.dispatchViewKey(key.String())
		if m.setupRequested {
			return tea.Quit
		}
	}

	m.resizeList()
	return nil
}

func (m *Model) dispatchViewKey(key string) {
	switch m.view {
	case mainView:
		m.updateMain(key)
	case archiveView:
		m.updateArchive(key)
	case libraryView:
		m.updateLibrary(key)
	case bundleView:
		m.updateBundle(key)
	}
}

// activeList returns the list backing the current view. All four support
// filtering, so filter handling never needs to branch on the view.
func (m *Model) activeList() *list.Model {
	switch m.view {
	case archiveView:
		return &m.archiveList
	case libraryView:
		return &m.libraryList
	case bundleView:
		return &m.bundleList
	default:
		return &m.list
	}
}

func (m *Model) beginFilter() {
	l := m.activeList()
	if len(l.Items()) == 0 {
		m.setError("Nothing to filter in this view.")
		return
	}
	m.filtering = true
	m.clearStatus()
	m.queue(m.forwardToList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}))
}

func (m *Model) updateFilter(key tea.KeyMsg) tea.Cmd {
	cmd := m.forwardToList(key)
	if m.activeList().FilterState() != list.Filtering {
		m.filtering = false
	}
	m.afterFilterChange()
	return cmd
}

func (m *Model) forwardToList(msg tea.Msg) tea.Cmd {
	l := m.activeList()
	updated, cmd := l.Update(msg)
	*l = updated
	return cmd
}

// clearActiveFilter drops an applied filter and reports whether it did
// anything, so esc can fall through to its per-view meaning when there is no
// filter to clear.
func (m *Model) clearActiveFilter() bool {
	l := m.activeList()
	if l.FilterState() == list.Unfiltered {
		return false
	}
	l.ResetFilter()
	m.filtering = false
	m.afterFilterChange()
	m.clearStatus()
	return true
}

// afterFilterChange re-syncs derived state (main-view cursor and detail pane)
// with whatever the filter left visible.
func (m *Model) afterFilterChange() {
	if m.view != mainView {
		return
	}
	if _, ok := m.list.SelectedItem().(skillItem); !ok {
		// The filter left a header (or nothing) under the cursor; step onto
		// the first real row when there is one.
		m.selectNearestMainRow(m.list.Index(), 1)
	}
	m.syncMainCursor()
	m.refreshDetail()
}

// filterQuery is the text currently typed into the active list's filter.
func (m *Model) filterQuery() string {
	return m.activeList().FilterValue()
}

// filterActive is true whenever a filter is being typed or is applied.
func (m *Model) filterActive() bool {
	return m.activeList().FilterState() != list.Unfiltered
}

// filterFoundNothing is the explicit zero-match state: a filter is in play and
// nothing survived it.
func (m *Model) filterFoundNothing() bool {
	l := m.activeList()
	return l.FilterState() != list.Unfiltered && l.FilterValue() != "" && len(l.VisibleItems()) == 0
}

func (m *Model) updateMain(key string) {
	switch key {
	case "S":
		m.setupRequested = true
		m.setStatus("Opening Setup…")
	case "u":
		selected, ok := m.selectedMainSkill()
		if !ok {
			m.setError("No skill selected.")
			return
		}
		if reason := archiveUnavailableReason(selected); reason != "" {
			m.setError(reason)
			return
		}
		m.pending = &pendingConfirm{
			description: fmt.Sprintf("Archive %s %q? y to confirm, any other key to cancel.", selected.Source, selected.Name),
			action:      pendingArchive,
			location:    selected.Location,
		}
	case "s":
		selected, ok := m.selectedMainSkill()
		if !ok {
			m.setError("No skill selected.")
			return
		}
		if reason := suppressUnavailableReason(selected); reason != "" {
			m.setError(reason)
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
			m.setError("No skill selected.")
			return
		}
		if !canUninstallPlugin(selected) {
			m.setError("Uninstall plugin is only available for Plugin skills.")
			return
		}
		names := pluginSkillNames(m.inv.Skills, *selected.Plugin)
		m.pending = &pendingConfirm{
			description: fmt.Sprintf("Uninstall plugin %q (%s@%s)? This permanently deletes the plugin's files and cannot be undone; it is not an Archive operation. This removes all %d skills: %s. y to confirm, any other key to cancel.",
				selected.Plugin.Plugin, selected.Plugin.Plugin, selected.Plugin.Marketplace, len(names), strings.Join(names, ", ")),
			action: pendingUninstallPlugin,
			plugin: *selected.Plugin,
		}
	case "m":
		selected, ok := m.selectedMainSkill()
		if !ok {
			m.setError("No skill selected.")
			return
		}
		if reason := manualOnlyUnavailableReason(selected); reason != "" {
			m.setError(reason)
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
		m.switchView(archiveView)
		m.refreshArchive()
	case "L":
		m.switchView(libraryView)
		m.refreshLibrary()
	case "B":
		m.switchView(bundleView)
		m.refreshBundles()
	case "l":
		m.confirmLibraryToggle()
	case "c":
		m.toggleCostSort()
	case " ":
		m.toggleMark()
	case "M":
		m.confirmBulkManualOnly()
	case "o":
		m.moreMenu = &moreMenu{}
		m.clearStatus()
	}
}

// confirmBulkManualOnly stages the `M` action. It refuses loudly rather than
// silently when there is nothing to do, and the confirmation states both how
// many Skills change and what that is worth per session, because the saving is
// the reason to press the key at all.
func (m *Model) confirmBulkManualOnly() {
	marked := m.markedSkills()
	if len(marked) == 0 {
		m.setError("Nothing marked. Press space to mark Skills, then M to set them all Manual-only.")
		return
	}
	savings := engine.EstimateManualOnlySavings(marked)
	if savings.Skills == 0 {
		m.setStatus(fmt.Sprintf("Nothing to do: all %s already Manual-only.", pluralCount(len(marked), "marked Skill is", "marked Skills are")))
		return
	}

	description := fmt.Sprintf("Set %s to Manual-only?", pluralCount(savings.Skills, "Skill", "Skills"))
	if already := len(marked) - savings.Skills; already > 0 {
		description += fmt.Sprintf(" (%d of the %d marked are already Manual-only.)", already, len(marked))
	}
	description += fmt.Sprintf(" Saves %s tokens per session (estimated). They will still run when explicitly invoked. y to confirm, any other key to cancel.",
		engine.FormatTokenEstimate(savings.Tokens))

	m.pending = &pendingConfirm{
		description: description,
		action:      pendingBulkManualOnly,
		skills:      marked,
	}
}

// applyBulkManualOnly runs the confirmed marked-set change and reports all
// three outcomes it can produce. A partial failure is an error status, not a
// success with a footnote: some of the Skills the user asked about are still
// Auto-activating, and the per-session number on screen will not have dropped
// as far as the confirmation promised.
func (m *Model) applyBulkManualOnly(skills []engine.Skill) {
	// The pre-change Activation decides how each result is reported, and it has
	// to be read here rather than from the post-run inventory. Tokens are only
	// credited for a Skill that was actually Auto — matching
	// EstimateManualOnlySavings, so the confirmation's promise and the report's
	// claim are computed the same way.
	before := make(map[string]engine.ActivationState, len(skills))
	for _, skill := range skills {
		before[skillMarkKey(skill)] = skill.Activation
	}

	changed, already, saved := 0, 0, 0
	var failures []engine.Skill
	var reasons []string
	for _, result := range m.engine.SetManualOnlyBulk(skills, true) {
		if result.Err != nil {
			failures = append(failures, result.Skill)
			reasons = append(reasons, result.Skill.Name+" ("+result.Err.Error()+")")
			continue
		}
		was := before[skillMarkKey(result.Skill)]
		if was == engine.ActivationManualOnly {
			already++
			continue
		}
		changed++
		if was == engine.ActivationAuto {
			saved += result.Skill.DescriptionTokens
		}
	}

	report := fmt.Sprintf("Set %s to Manual-only, saving %s tokens per session (estimated).",
		pluralCount(changed, "Skill", "Skills"), engine.FormatTokenEstimate(saved))
	if already > 0 {
		report += fmt.Sprintf(" %s already Manual-only.", pluralCount(already, "was", "were"))
	}

	// The refresh is what re-reads Activation for every row; it also drops the
	// marks, which is right for the Skills that changed.
	m.refreshInventory()

	if len(failures) > 0 {
		report += fmt.Sprintf(" %s: %s.", pluralCount(len(failures), "failure", "failures"), strings.Join(truncateList(reasons, 3), "; "))
		m.setError(report)
		// Re-mark exactly what did not change, so retrying is one keypress and
		// the failures stay visible in the list instead of disappearing into a
		// status line the next action will overwrite.
		for _, failed := range failures {
			m.marks.add(failed)
		}
		return
	}
	m.setStatus(report)
}

// truncateList caps a list of reasons so one bad batch cannot push everything
// else off the screen, while still saying how much it is not showing.
func truncateList(items []string, limit int) []string {
	if len(items) <= limit {
		return items
	}
	return append(items[:limit:limit], fmt.Sprintf("and %d more", len(items)-limit))
}

// pluralCount renders "1 Skill" / "3 Skills" from a count and the two forms.
func pluralCount(count int, singular, plural string) string {
	if count == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", count, plural)
}

// toggleCostSort switches the inventory between its Source grouping and a flat
// ranking by estimated per-session cost, most expensive first. It is a view
// change only: nothing is written, and the same Skills are shown either way.
func (m *Model) toggleCostSort() {
	// The re-read behind the re-sort clears the marked set. Say so rather than
	// letting a selection the user built up vanish without comment.
	dropped := m.marks.len()
	m.sortByCost = !m.sortByCost
	m.refreshInventory()
	note := ""
	if dropped > 0 {
		note = fmt.Sprintf(" %s cleared.", pluralCount(dropped, "mark", "marks"))
	}
	if m.sortByCost {
		m.setStatus("Sorted by estimated cost per session, most expensive first. c groups by Source again." + note)
		return
	}
	m.setStatus("Grouped by Source again." + note)
}

// switchView leaves the outgoing view unfiltered so a stale filter does not
// silently hide rows the next time the view is opened.
func (m *Model) switchView(next viewState) {
	m.activeList().ResetFilter()
	m.filtering = false
	m.view = next
	m.activeList().ResetFilter()
	m.clearStatus()
}

func (m *Model) updateArchive(key string) {
	switch key {
	case "a", "esc":
		m.switchView(mainView)
		m.refreshInventory()
	case "r":
		selected, ok := m.selectedArchiveEntry()
		if !ok {
			m.setError("No archive entry selected.")
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
			m.setError("No archive entry selected.")
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
		m.switchView(mainView)
		m.refreshInventory()
	case "d":
		selected, ok := m.selectedLibraryEntry()
		if !ok {
			m.setError("No Library entry selected.")
			return
		}
		m.pending = &pendingConfirm{
			description: fmt.Sprintf("Remove %q from Library? y to confirm, any other key to cancel.", selected.Name),
			action:      pendingRemoveLibraryEntry,
			id:          selected.ID,
			entry:       selected,
		}
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
		m.switchView(mainView)
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
			m.setError("Select a Bundle to delete.")
			return
		}
		m.pending = &pendingConfirm{
			description: fmt.Sprintf("Delete Bundle %q? y to confirm, any other key to cancel.", row.bundle.Name),
			action:      pendingDeleteBundle,
			id:          row.bundle.ID,
			bundle:      row.bundle,
		}
	case "a":
		if !selected {
			m.setError("Select a Bundle or member first.")
			return
		}
		entries, err := m.engine.ListLibrary()
		if err != nil {
			m.setError(formatActionError("Library read failed: ", err))
			return
		}
		if len(entries) == 0 {
			m.setError("Library is empty.")
			return
		}
		m.memberPicker = &memberPicker{bundle: row.bundle, entries: entries}
	case "r":
		if !selected || row.member == nil {
			m.setError("Select a Bundle member to remove.")
			return
		}
		m.pending = &pendingConfirm{
			description: fmt.Sprintf("Remove %q from Bundle %q? y to confirm, any other key to cancel.", row.name, row.bundle.Name),
			action:      pendingRemoveBundleMember,
			bundle:      row.bundle,
			memberID:    row.member.LibraryEntryID,
			memberName:  row.name,
		}
	case "m":
		if !selected || row.member == nil {
			m.setError("Select a Bundle member to change Activation.")
			return
		}
		next := engine.ActivationManualOnly
		if row.member.Activation == engine.ActivationManualOnly {
			next = engine.ActivationAuto
		}
		m.pending = &pendingConfirm{
			description: fmt.Sprintf("Set %q in Bundle %q to %s? y to confirm, any other key to cancel.", row.name, row.bundle.Name, next),
			action:      pendingBundleMemberActivation,
			bundle:      row.bundle,
			memberID:    row.member.LibraryEntryID,
			memberName:  row.name,
			activation:  next,
		}
	case "i":
		if !selected {
			m.setError("Select a Bundle first.")
			return
		}
		m.beginBundleInstall(row.bundle)
	}
}

func (m *Model) beginLibraryInstall() {
	selected, ok := m.selectedLibraryEntry()
	if !ok {
		m.setError("No Library entry selected.")
		return
	}
	// buildInstallTargetOptions always seeds "Personal", so the option list is
	// never empty and needs no empty-case branch.
	m.installPicker = &installPicker{
		entry:   selected,
		options: buildInstallTargetOptions(m.engine),
		cursor:  0,
	}
	m.clearStatus()
}

func (m *Model) beginBundleInstall(bundle engine.Bundle) {
	options := buildInstallTargetOptions(m.engine)
	m.installPicker = &installPicker{bundle: &bundle, options: options}
	m.clearStatus()
}

func (m *Model) updateInstallPicker(key string) tea.Cmd {
	picker := m.installPicker
	if picker == nil {
		return nil
	}
	switch key {
	case "esc", "q":
		m.installPicker = nil
		m.setStatus("Canceled.")
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
			return m.confirmOrRunBundleInstall(bundle, opt.target)
		}
		entry := picker.entry
		m.installPicker = nil
		return m.confirmOrRunInstall(entry, opt.target)
	}
	return nil
}

func (m *Model) confirmOrRunBundleInstall(bundle engine.Bundle, target engine.InstallTarget) tea.Cmd {
	entries, err := m.engine.ListLibrary()
	if err != nil {
		m.setError(formatActionError("Install Bundle failed: ", err))
		return nil
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
			m.setError(formatActionError("Install Bundle failed: ", err))
			return nil
		}
		if exists {
			collisions = append(collisions, dest)
		}
	}
	if len(collisions) > 0 {
		m.pending = &pendingConfirm{description: fmt.Sprintf("Install Bundle %q will replace: %s. y to confirm, any other key to cancel.", bundle.Name, strings.Join(collisions, ", ")), action: pendingInstallBundle, bundle: bundle, target: target}
		return nil
	}
	return m.startInstall(bundle.Name, target, true, installStepBundle, func(ctx context.Context) error {
		return m.engine.InstallBundleContext(ctx, bundle.ID, target)
	})
}

func (m *Model) confirmOrRunInstall(entry engine.LibraryEntry, target engine.InstallTarget) tea.Cmd {
	if entry.Source.Kind == engine.LibrarySourceMarketplace {
		return m.startInstall(entry.Name, target, false, installStepFor(entry), func(ctx context.Context) error {
			return m.engine.InstallLibraryEntryContext(ctx, entry, target, engine.ActivationAuto)
		})
	}
	dest, exists, err := m.engine.InstallDestination(entry, target)
	if err != nil {
		m.setError(formatActionError("Install failed: ", err))
		return nil
	}
	if entry.Source.Kind == engine.LibrarySourceSkillsSh && entry.Source.SkillsShSkill == "" {
		m.pending = &pendingConfirm{description: fmt.Sprintf("Install every skill from %q? Existing skills with matching names may be replaced. y to confirm, any other key to cancel.", entry.Source.SkillsShRepo), action: pendingInstall, entry: entry, target: target}
		return nil
	}
	if exists {
		m.pending = &pendingConfirm{
			description: fmt.Sprintf("Replace existing skill at %s with Library entry %q? y to confirm, any other key to cancel.", dest, entry.Name),
			action:      pendingInstall,
			entry:       entry,
			target:      target,
		}
		return nil
	}
	return m.startInstall(entry.Name, target, false, installStepFor(entry), func(ctx context.Context) error {
		return m.engine.InstallLibraryEntryContext(ctx, entry, target, engine.ActivationAuto)
	})
}

func (m *Model) updateSourcePicker(key tea.KeyMsg) {
	p := m.sourcePicker
	switch key.String() {
	case "esc":
		m.sourcePicker = nil
		m.setStatus("Canceled.")
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
		m.setStatus("Canceled.")
		return
	}
	if !done {
		return
	}
	f := m.form
	m.form = nil
	if f.kind == formBundleName {
		if _, err := m.engine.CreateBundle(f.values[0]); err != nil {
			m.setError(formatActionError("Create Bundle failed: ", err))
		} else {
			m.setStatus("Created Bundle.")
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
		m.setError(formatActionError("Add Library entry failed: ", err))
	} else {
		m.setStatus("Added " + entry.Name + " to Library.")
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
		m.setStatus("Canceled.")
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
		bundle := p.bundle
		m.memberPicker = nil
		m.pending = &pendingConfirm{
			description: fmt.Sprintf("Add %q to Bundle %q? y to confirm, any other key to cancel.", entry.Name, bundle.Name),
			action:      pendingAddBundleMember,
			bundle:      bundle,
			memberID:    entry.ID,
			memberName:  entry.Name,
		}
	}
}

// startInstall stages an install: the returned command announces the start (so
// the spinner and status appear on the very next frame) and the work itself
// runs against a cancellable context handed to the engine's *Context entry
// points.
//
// step is the phase the work dispatches. The engine reports no intermediate
// progress, so Skillet shows the two phases it genuinely knows about —
// "resolving" while the target and destination are settled here, then step
// once the child process is running.
func (m *Model) startInstall(desc string, target engine.InstallTarget, isBundle bool, step string, work func(context.Context) error) tea.Cmd {
	if m.installing != "" {
		m.setError("Install already in progress.")
		return nil
	}
	m.installing = desc
	m.installStep = installStepResolving
	m.installCanceled = false
	ctx, cancel := context.WithCancel(context.Background())
	m.installCancel = cancel
	m.installWork = func() tea.Msg {
		err := work(ctx)
		cancel()
		return installFinishedMsg{err: err, desc: desc, target: target, isBundle: isBundle}
	}
	return func() tea.Msg { return installStartedMsg{desc: desc, step: step} }
}

const (
	installStepResolving = "resolving"
	installStepCloning   = "cloning source"
	installStepCopying   = "copying files"
	installStepPlugin    = "configuring plugin"
	installStepBundle    = "installing Bundle members"
)

// installStepFor names the phase a Library entry's Install dispatches, derived
// from the entry's Source kind rather than from engine progress reporting.
func installStepFor(entry engine.LibraryEntry) string {
	switch entry.Source.Kind {
	case engine.LibrarySourceMarketplace:
		return installStepPlugin
	case engine.LibrarySourceGit, engine.LibrarySourceSkillsSh:
		return installStepCloning
	default:
		return installStepCopying
	}
}

// cancelInstall cancels the running install through the engine's context seam.
// The status is only provisional: the outcome is reported once the work
// actually returns, in finishInstall.
func (m *Model) cancelInstall() {
	if m.installing == "" {
		return
	}
	m.installCanceled = true
	if m.installCancel != nil {
		m.installCancel()
	}
	m.setStatus(fmt.Sprintf("Cancelling Install of %q…", m.installing))
}

// quitOrCancelInstall makes quitting safe mid-install: instead of exiting and
// orphaning the child process, it cancels and waits for the work to return.
func (m *Model) quitOrCancelInstall() tea.Cmd {
	if m.installing == "" {
		return tea.Quit
	}
	m.quitAfterInstall = true
	m.cancelInstall()
	return nil
}

func (m *Model) finishInstall(msg installFinishedMsg) tea.Cmd {
	elapsed := time.Since(m.installStarted)
	if m.installStarted.IsZero() {
		elapsed = 0
	}
	canceled := m.installCanceled
	quit := m.quitAfterInstall
	if m.installCancel != nil {
		m.installCancel()
	}
	m.installing = ""
	m.installStep = ""
	m.installCancel = nil
	m.installCanceled = false
	m.installStarted = time.Time{}
	m.quitAfterInstall = false

	prefix := "Install"
	if msg.isBundle {
		prefix = "Install Bundle"
	}
	switch {
	case canceled:
		m.setStatus(fmt.Sprintf("%s of %q cancelled.", prefix, msg.desc))
	case msg.err != nil && isTimeoutError(msg.err):
		m.setError(fmt.Sprintf("%s of %q timed out after %s: %v", prefix, msg.desc, elapsed.Round(time.Second), msg.err))
	case msg.err != nil:
		m.setError(formatActionError(prefix+" failed: ", msg.err))
	case msg.isBundle:
		m.setStatus(fmt.Sprintf("Installed Bundle %q.", msg.desc))
	default:
		where := "Personal"
		if msg.target.Kind == engine.InstallTargetProject {
			where = msg.target.RepoRoot
		}
		m.setStatus(fmt.Sprintf("Installed %q → %s.", msg.desc, where))
	}
	if quit {
		return tea.Quit
	}
	return nil
}

// isTimeoutError recognizes the engine's rewritten deadline message
// ("<program> timed out after <limit>: …") so the status line can name the
// operation and how long the user waited.
func isTimeoutError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "timed out after")
}

// installBlocksKey lists the disk-mutating keys refused while an install is in
// flight. Navigation, filtering, and view switching stay live; anything that
// would archive, purge, delete, or start a second write does not.
func installBlocksKey(view viewState, key string) bool {
	switch view {
	case mainView:
		switch key {
		case "u", "s", "m", "M", "x", "l":
			return true
		}
	case archiveView:
		switch key {
		case "r", "p":
			return true
		}
	case libraryView:
		switch key {
		case "d", "i":
			return true
		}
	case bundleView:
		switch key {
		case "d", "i", "a", "r", "m":
			return true
		}
	}
	return false
}

// libraryMembership describes what pressing `l` on a skill would do, so the
// confirmation can name the exact change before anything is written.
type libraryMembership struct {
	// remove is true when the skill already has a Library record.
	remove bool
	// entryID is the record to remove; empty when adding.
	entryID string
	// label is the user-facing name of the thing being added or removed.
	label string
	// isPlugin distinguishes a marketplace plugin record from a local-path one.
	isPlugin bool
}

func (m *Model) libraryMembershipFor(skill engine.Skill) (libraryMembership, error) {
	if skill.Source == engine.SourcePlugin && skill.Plugin != nil {
		entries, err := m.engine.ListLibrary()
		if err != nil {
			return libraryMembership{}, err
		}
		for _, entry := range entries {
			if entry.Source.Kind == engine.LibrarySourceMarketplace &&
				entry.Source.Marketplace == skill.Plugin.Marketplace &&
				entry.Source.PluginName == skill.Plugin.Plugin {
				return libraryMembership{remove: true, entryID: entry.ID, label: skill.Plugin.Plugin, isPlugin: true}, nil
			}
		}
		return libraryMembership{label: skill.Plugin.Plugin, isPlugin: true}, nil
	}
	if existing, found := m.engine.FindLibraryEntryByLocalPath(skill.Location); found {
		return libraryMembership{remove: true, entryID: existing.ID, label: skill.Name}, nil
	}
	return libraryMembership{label: skill.Name}, nil
}

// confirmLibraryToggle stages the `l` action behind the same one-line y/n
// confirmation as every other disk change — it writes or deletes a record
// under ~/.skillet/library.
func (m *Model) confirmLibraryToggle() {
	selected, ok := m.selectedMainSkill()
	if !ok {
		m.setError("No skill selected.")
		return
	}
	if reason := libraryToggleUnavailableReason(selected); reason != "" {
		m.setError(reason)
		return
	}
	membership, err := m.libraryMembershipFor(selected)
	if err != nil {
		m.setError(formatActionError("Library read failed: ", err))
		return
	}
	noun := "skill"
	if membership.isPlugin {
		noun = "plugin"
	}
	verb := "Add"
	preposition := "to"
	if membership.remove {
		verb = "Remove"
		preposition = "from"
	}
	m.pending = &pendingConfirm{
		description: fmt.Sprintf("%s %s %q %s Library? y to confirm, any other key to cancel.", verb, noun, membership.label, preposition),
		action:      pendingLibraryToggle,
		skill:       selected,
	}
}

// applyLibraryToggle performs the confirmed `l` action. Membership is
// re-derived here rather than carried from the confirmation so the write
// always matches the Library as it stands at confirm time.
func (m *Model) applyLibraryToggle(skill engine.Skill) {
	membership, err := m.libraryMembershipFor(skill)
	if err != nil {
		m.setError(formatActionError("Library read failed: ", err))
		return
	}
	noun := ""
	if membership.isPlugin {
		noun = "plugin "
	}
	if membership.remove {
		if err := m.engine.RemoveLibraryEntry(membership.entryID); err != nil {
			m.setError(formatActionError("Remove from Library failed: ", err))
			return
		}
		m.setStatus("Removed " + noun + membership.label + " from Library.")
		return
	}

	entry := engine.LibraryEntry{
		Name: skill.Name,
		Kind: skill.Kind,
		Tool: skill.Tool,
		Source: engine.LibrarySource{
			Kind:      engine.LibrarySourceLocalPath,
			LocalPath: skill.Location,
		},
	}
	if membership.isPlugin {
		entry = engine.LibraryEntry{
			Name: skill.Plugin.Plugin,
			Source: engine.LibrarySource{
				Kind:        engine.LibrarySourceMarketplace,
				Marketplace: skill.Plugin.Marketplace,
				PluginName:  skill.Plugin.Plugin,
			},
		}
	}
	added, err := m.engine.AddLibraryEntry(entry)
	if err != nil {
		m.setError(formatActionError("Add to Library failed: ", err))
		return
	}
	m.setStatus("Added " + noun + added.Name + " to Library.")
}

func (m *Model) executePending() tea.Cmd {
	switch m.pending.action {
	case pendingArchive:
		entry, err := m.engine.Uninstall(m.pending.location)
		if err != nil {
			m.setError(formatActionError("Archive failed: ", err))
			return nil
		}
		m.setStatus("Archived " + entry.Name + ".")
		m.refreshInventory()
	case pendingRestore:
		if err := m.engine.Restore(m.pending.id); err != nil {
			m.setError(formatActionError("Restore failed: ", err))
			return nil
		}
		m.setStatus("Restored archive entry.")
		m.refreshArchive()
	case pendingPurge:
		if err := m.engine.Purge(m.pending.id); err != nil {
			m.setError(formatActionError("Purge failed: ", err))
			return nil
		}
		m.setStatus("Purged archive entry.")
		m.refreshArchive()
	case pendingSuppress:
		if err := m.engine.Suppress(m.pending.skill); err != nil {
			m.setError(formatActionError("Suppress failed: ", err))
			return nil
		}
		status := "Suppressed " + m.pending.skill.Name + "."
		if needsCodexRestartHint(m.pending.skill) {
			status += " Restart Codex to pick up the change."
		}
		m.setStatus(status)
		m.refreshInventory()
	case pendingUnsuppress:
		if err := m.engine.Unsuppress(m.pending.skill); err != nil {
			m.setError(formatActionError("Un-suppress failed: ", err))
			return nil
		}
		status := "Un-suppressed " + m.pending.skill.Name + "."
		if needsCodexRestartHint(m.pending.skill) {
			status += " Restart Codex to pick up the change."
		}
		m.setStatus(status)
		m.refreshInventory()
	case pendingManualOnly:
		if err := m.engine.SetManualOnly(m.pending.skill, true); err != nil {
			m.setError(formatActionError("Manual-only failed: ", err))
			return nil
		}
		m.setStatus("Made " + m.pending.skill.Name + " Manual-only.")
		m.refreshInventory()
	case pendingAutoActivate:
		if err := m.engine.SetManualOnly(m.pending.skill, false); err != nil {
			m.setError(formatActionError("Auto-activation failed: ", err))
			return nil
		}
		m.setStatus("Restored Auto-activation for " + m.pending.skill.Name + ".")
		m.refreshInventory()
	case pendingBulkManualOnly:
		m.applyBulkManualOnly(m.pending.skills)
	case pendingUninstallPlugin:
		plugin := m.pending.plugin
		if err := m.engine.UninstallPlugin(plugin); err != nil {
			m.setError(formatActionError("Uninstall plugin failed: ", err))
			return nil
		}
		m.setStatus("Uninstalled plugin " + plugin.Plugin + ".")
		m.refreshInventory()
	case pendingInstall:
		entry := m.pending.entry
		target := m.pending.target
		return m.startInstall(entry.Name, target, false, installStepFor(entry), func(ctx context.Context) error {
			return m.engine.InstallLibraryEntryContext(ctx, entry, target, engine.ActivationAuto)
		})
	case pendingInstallBundle:
		bundle := m.pending.bundle
		target := m.pending.target
		return m.startInstall(bundle.Name, target, true, installStepBundle, func(ctx context.Context) error {
			return m.engine.InstallBundleContext(ctx, bundle.ID, target)
		})
	case pendingRemoveLibraryEntry:
		entry := m.pending.entry
		if err := m.engine.RemoveLibraryEntry(entry.ID); err != nil {
			m.setError(formatActionError("Remove from Library failed: ", err))
			return nil
		}
		m.setStatus("Removed " + entry.Name + " from Library.")
		m.refreshLibrary()
	case pendingDeleteBundle:
		bundle := m.pending.bundle
		if err := m.engine.DeleteBundle(bundle.ID); err != nil {
			m.setError(formatActionError("Delete Bundle failed: ", err))
			return nil
		}
		m.setStatus("Deleted Bundle " + bundle.Name + ".")
		m.refreshBundles()
	case pendingLibraryToggle:
		m.applyLibraryToggle(m.pending.skill)
	case pendingAddBundleMember:
		if err := m.engine.AddBundleMember(m.pending.bundle.ID, m.pending.memberID, engine.ActivationAuto); err != nil {
			m.setError(formatActionError("Add Bundle member failed: ", err))
			return nil
		}
		m.setStatus("Added " + m.pending.memberName + " to Bundle.")
		m.refreshBundles()
	case pendingRemoveBundleMember:
		if err := m.engine.RemoveBundleMember(m.pending.bundle.ID, m.pending.memberID); err != nil {
			m.setError(formatActionError("Remove member failed: ", err))
			return nil
		}
		m.setStatus("Removed " + m.pending.memberName + " from Bundle.")
		m.refreshBundles()
	case pendingBundleMemberActivation:
		if err := m.engine.SetBundleMemberActivation(m.pending.bundle.ID, m.pending.memberID, m.pending.activation); err != nil {
			m.setError(formatActionError("Activation failed: ", err))
			return nil
		}
		m.setStatus("Bundle member Activation: " + string(m.pending.activation))
		m.refreshBundles()
	}
	return nil
}

// formatActionError prefixes an engine error with a user-facing action label,
// but avoids stutter when the engine error already begins with the same verb
// (e.g. "install:" or "suppress skill:").
func formatActionError(prefix string, err error) string {
	if err == nil {
		return strings.TrimSuffix(prefix, ": ") + "."
	}
	msg := err.Error()
	loweredMsg := strings.ToLower(strings.TrimSpace(msg))
	loweredPrefix := strings.ToLower(prefix)
	if strings.HasPrefix(loweredMsg, loweredPrefix) {
		return msg
	}
	// Normalize hyphenation so "Un-suppress" matches engine "unsuppress".
	verb := strings.SplitN(loweredPrefix, " ", 2)[0]
	normVerb := strings.ReplaceAll(verb, "-", "")
	if strings.HasPrefix(loweredMsg, verb) || strings.HasPrefix(loweredMsg, normVerb) {
		return msg
	}
	return prefix + msg
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
	m.clearTransientStatus()
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
	items := l.VisibleItems()
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

// Main-view navigation works in the list's *visible* index space, which is
// what list.Index/Select/SelectedItem use. That is the same as the raw item
// space when unfiltered and the filtered subset otherwise, so filtering and
// navigation cannot disagree about which row is selected.
func (m *Model) moveMainCursor(delta int) {
	items := m.list.VisibleItems()
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

// selectNearestMainRow selects the skill row at index, or the nearest one
// searching first in direction dir and then the other way. Source headers are
// not selectable rows.
func (m *Model) selectNearestMainRow(index, dir int) {
	items := m.list.VisibleItems()
	if len(items) == 0 {
		return
	}
	if index < 0 {
		index = 0
	}
	if index >= len(items) {
		index = len(items) - 1
	}
	if dir == 0 {
		dir = 1
	}
	for _, step := range [2]int{dir, -dir} {
		for i := index; i >= 0 && i < len(items); i += step {
			if _, ok := items[i].(skillItem); ok {
				m.list.Select(i)
				return
			}
		}
	}
	m.list.Select(index)
}

func (m *Model) pageMainCursor(pages int) {
	m.clearTransientStatus()
	if len(m.list.VisibleItems()) == 0 {
		return
	}
	dir := 1
	if pages < 0 {
		dir = -1
	}
	m.selectNearestMainRow(m.list.Index()+pages*m.mainPageSize(), dir)
	m.syncMainCursor()
	m.refreshDetail()
}

func (m *Model) jumpMainCursor(toEnd bool) {
	m.clearTransientStatus()
	items := m.list.VisibleItems()
	if len(items) == 0 {
		return
	}
	if toEnd {
		m.selectNearestMainRow(len(items)-1, -1)
	} else {
		m.selectNearestMainRow(0, 1)
	}
	m.syncMainCursor()
	m.refreshDetail()
}

func (m *Model) mainPageSize() int {
	if h := m.list.Height(); h > 1 {
		return h
	}
	return 10
}

func (m *Model) refreshInventory() {
	// A re-read is the one moment the marked set can no longer be trusted: the
	// Skills it points at may have changed Activation, moved, or gone. Marks
	// are cheap to make and expensive to get subtly wrong, so a refresh clears
	// them rather than trying to reconcile them.
	m.marks.clear()
	m.inv = m.engine.Inventory()
	if m.cursor >= len(m.inv.Skills) {
		m.cursor = max(0, len(m.inv.Skills)-1)
	}
	// The item order must match m.inv.Skills (syncMainCursor counts rows), so
	// the cost ranking is applied to the inventory itself, not just to the list.
	items := buildListItems(m.inv)
	if m.sortByCost {
		m.inv.Skills = engine.SortByDescriptionCost(m.inv.Skills)
		items = buildCostSortedListItems(m.inv)
	}
	// A re-read is exactly when a Skill's files may have changed on disk.
	m.detail.forgetMeasurements()
	// SetItems returns a re-filter command when a filter is applied; it has to
	// reach the runtime or the filtered view would show stale matches.
	m.queue(m.list.SetItems(items))
	m.selectMainCursor()
	m.refreshDetail()
	m.resizeList()
}

func (m *Model) refreshArchive() {
	entries, notices, err := m.engine.ListArchive()
	if err != nil {
		m.archive = nil
		m.archiveNotices = nil
		m.queue(m.archiveList.SetItems(nil))
		m.setError(formatActionError("Archive read failed: ", err))
		return
	}
	m.archive = entries
	m.archiveNotices = notices
	m.queue(m.archiveList.SetItems(buildArchiveItems(m.archive)))
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
		m.queue(m.libraryList.SetItems(nil))
		m.setError(formatActionError("Library read failed: ", err))
		return
	}
	m.library = entries
	m.queue(m.libraryList.SetItems(buildLibraryItems(m.library)))
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
		m.queue(m.bundleList.SetItems(nil))
		m.setError(formatActionError("Bundle read failed: ", err))
		return
	}
	m.bundles = bundles
	library, err := m.engine.ListLibrary()
	if err != nil {
		m.setError(formatActionError("Library read failed: ", err))
		return
	}
	m.queue(m.bundleList.SetItems(buildBundleItems(bundles, library, m.bundleExpanded)))
}

func (m *Model) View() string {
	view := m.renderView()
	if m.pending != nil {
		return renderConfirmOverlay(view, m.pending.description, m.width, m.height)
	}
	if m.moreMenu != nil {
		return renderConfirmOverlay(view, renderMoreMenu(m.moreMenu.cursor), m.width, m.height)
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

	m.renderFilterLine(&b)

	// While an install runs the spinner line replaces the informational status
	// (which says the same thing, without the live step, elapsed time, or the
	// cancel key). An error still renders above it — that is where a refused
	// destructive key reports itself.
	if m.installing != "" {
		if m.statusLevel == statusError && m.status != "" {
			b.WriteString("\n")
			b.WriteString(statusErrorStyle.Render(m.status))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(m.spinner.View())
		b.WriteString(m.installLine())
		b.WriteString("\n")
		return b.String()
	}

	if m.status != "" {
		b.WriteString("\n")
		if m.statusLevel == statusError {
			b.WriteString(statusErrorStyle.Render(m.status))
		} else {
			b.WriteString(m.status)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// installLine is the text beside the spinner. spinner.View() already ends in a
// space, so this starts at the message.
func (m *Model) installLine() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Installing %s…", m.installing)
	if m.installStep != "" {
		b.WriteString(" " + m.installStep)
	}
	if !m.installStarted.IsZero() {
		fmt.Fprintf(&b, " (%s)", time.Since(m.installStarted).Round(time.Second))
	}
	b.WriteString("  esc cancels")
	return b.String()
}

// renderFilterLine draws the filter prompt and its match count. The Bubbles
// list's own filter bar is disabled because help renders as line 2 (a header),
// and this keeps the filter next to the list it applies to.
func (m *Model) renderFilterLine(b *strings.Builder) {
	l := m.activeList()
	if !m.filterActive() {
		return
	}
	b.WriteString("\n")
	if m.filtering {
		// FilterInput.View() already carries its own "Filter: " prompt and
		// the text cursor.
		b.WriteString(strings.TrimRight(l.FilterInput.View(), " "))
	} else {
		b.WriteString("Filter: " + l.FilterValue() + "  (esc clears)")
	}
	matches := len(l.VisibleItems())
	b.WriteString(fmt.Sprintf("  %d match", matches))
	if matches != 1 {
		b.WriteString("es")
	}
	b.WriteString("\n")
}

// zeroMatchLine is the explicit "your filter matched nothing" state. Without
// it a filtered-to-empty list is indistinguishable from an empty inventory.
func (m *Model) zeroMatchLine() string {
	return fmt.Sprintf("No matches for %q. Press esc to clear the filter.\n", m.filterQuery())
}

func (m *Model) renderMain(b *strings.Builder) {
	b.WriteString(m.titleLine("Skillet"))
	if line := m.costHeaderLine(); line != "" {
		b.WriteString(line)
		b.WriteString("\n")
	}
	// The marked line sits directly under the cost header, not down by the
	// status: it is a running total of what the current selection would take
	// off the number above it.
	if line := m.markedLine(); line != "" {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(m.helpView())
	b.WriteString("\n\n")

	switch {
	case m.filterFoundNothing():
		b.WriteString(m.zeroMatchLine())
	case len(m.inv.Skills) == 0:
		// The first screen on a fresh machine. It has to read as a starting
		// point, not a failure, so it points at the two ways in: guided Setup
		// and the Library.
		b.WriteString("No skills yet.\n")
		b.WriteString("Press S for guided Setup, or L to open the Library and Install one.\n")
	default:
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, m.list.View(), " ", m.detail.render()))
		b.WriteString("\n")
	}

	notices := m.visibleInventoryNotices()
	if len(notices) > 0 {
		b.WriteString("\nNotices\n")
		for _, notice := range notices {
			b.WriteString("- ")
			b.WriteString(notice.Message)
			b.WriteString("\n")
		}
	}
}

// titleLine is the view's title with the two keys that are always true beside
// it. Putting them here rather than in a help row is what lets the compact help
// fit the main view's job into two lines: the title line was mostly empty
// anyway, and a header line is worth more to the list than to whitespace.
func (m *Model) titleLine(title string) string {
	for _, hint := range []string{"   ? all keys · q quit", "  ? keys · q quit"} {
		if m.width <= 0 || lipgloss.Width(title)+lipgloss.Width(hint) <= m.width {
			return title + skillMetaStyle.Render(hint) + "\n"
		}
	}
	return title + "\n"
}

// markedLine is the persistent marked-count line: how many Skills are marked,
// what M would do to them, and what that is worth. It is only rendered while a
// mark is set, and resizeList reserves a line for it on exactly the same
// condition.
func (m *Model) markedLine() string {
	marked := m.markedSkills()
	if len(marked) == 0 {
		return ""
	}
	count := pluralCount(len(marked), "Skill marked", "Skills marked")
	savings := engine.EstimateManualOnlySavings(marked)
	if savings.Skills == 0 {
		return markedStyle.Render(fitToWidth([]string{
			count + " · already Manual-only · esc clears",
			count + " · already Manual-only",
			count,
		}, m.width))
	}
	tokens := engine.FormatTokenEstimate(savings.Tokens)
	return markedStyle.Render(fitToWidth([]string{
		fmt.Sprintf("%s · M sets them Manual-only, saving %s tokens per session (est.) · esc clears", count, tokens),
		fmt.Sprintf("%s · M → Manual-only, saves %s tokens/session · esc clears", count, tokens),
		fmt.Sprintf("%s · M → Manual-only, saves %s", count, tokens),
		count,
	}, m.width))
}

// costHeaderLine is the number this whole feature exists for: what
// Auto-activation is costing the user in every single session, per Tool.
//
// It is deliberately one line, and it deliberately states its own exclusions.
// Only Auto-activating Skills are counted, because only their descriptions are
// offered to the model unprompted; a bare total with silently dropped Skills
// would be worse than showing nothing, since the user would read it as "all my
// Skills cost this much" and conclude their Manual-only ones are free.
//
// It returns "" for an empty inventory, where the friendly empty state says
// everything there is to say.
func (m *Model) costHeaderLine() string {
	if len(m.inv.Skills) == 0 {
		return ""
	}
	summary := engine.SummarizeContextCost(m.inv.Skills)

	return skillMetaStyle.Render(fitToWidth(costHeaderVariants(summary), m.width))
}

// costHeaderVariants is the same statement at three lengths, longest first. Even
// the shortest keeps the two things that stop the number being misread: that it
// is an estimate, and that it counts Auto-activation only.
func costHeaderVariants(summary engine.ContextCost) []string {
	if len(summary.ByTool) == 0 {
		return []string{
			fmt.Sprintf("Every session (est.): nothing Auto-activates — all %d Skills excluded", summary.Excluded),
			"Every session (est.): nothing Auto-activates",
			"Auto: none",
		}
	}

	parts := make([]string, 0, len(summary.ByTool))
	for _, tool := range summary.ByTool {
		parts = append(parts, fmt.Sprintf("%s %s", tool.Tool, engine.FormatTokenEstimate(tool.DescriptionTokens)))
	}
	byTool := strings.Join(parts, " · ")
	total := engine.FormatTokenEstimate(summary.DescriptionTokens)

	excluded := ""
	if summary.Excluded > 0 {
		excluded = fmt.Sprintf(", %d excluded", summary.Excluded)
	}
	return []string{
		fmt.Sprintf("Every session (est.): %s tokens — Auto descriptions only%s", byTool, excluded),
		fmt.Sprintf("Every session (est.): %s tokens, Auto only%s", total, excluded),
		fmt.Sprintf("Auto/session: %s", total),
	}
}

// fitToWidth picks the first variant that fits, and truncates the last one if
// even that is too wide. width <= 0 means the terminal size is not known yet, in
// which case the fullest form is right.
func fitToWidth(variants []string, width int) string {
	if len(variants) == 0 {
		return ""
	}
	if width <= 0 {
		return variants[0]
	}
	for _, variant := range variants {
		if lipgloss.Width(variant) <= width {
			return variant
		}
	}
	runes := []rune(variants[len(variants)-1])
	for len(runes) > 0 && lipgloss.Width(string(runes)) > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes)
}

// visibleInventoryNotices is the inventory's notices as rendered. The scanners
// stay quiet about a standard directory that simply does not exist, so
// everything reaching here is a real anomaly worth showing.
func (m *Model) visibleInventoryNotices() []engine.Notice {
	return m.inv.Notices
}

func (m *Model) renderArchive(b *strings.Builder) {
	b.WriteString(m.titleLine("Skillet Archive"))
	b.WriteString(m.helpView())
	b.WriteString("\n\n")

	switch {
	case m.filterFoundNothing():
		b.WriteString(m.zeroMatchLine())
	case len(m.archive) == 0:
		b.WriteString("Archive is empty. Press a to return to the inventory.\n")
	default:
		b.WriteString(m.archiveList.View())
		b.WriteString("\n")
	}

	if len(m.archiveNotices) > 0 {
		b.WriteString("\nNotices\n")
		for _, notice := range m.archiveNotices {
			b.WriteString("- ")
			b.WriteString(notice.Message)
			b.WriteString("\n")
		}
	}
}

func (m *Model) renderLibrary(b *strings.Builder) {
	b.WriteString(m.titleLine("Skillet Library"))
	b.WriteString(m.helpView())
	b.WriteString("\n\n")

	switch {
	case m.filterFoundNothing():
		b.WriteString(m.zeroMatchLine())
	case len(m.library) == 0:
		b.WriteString("Library is empty. Press n to add a new entry.\n")
	default:
		b.WriteString(m.libraryList.View())
		b.WriteString("\n")
	}
}

func (m *Model) renderBundles(b *strings.Builder) {
	b.WriteString(m.titleLine("Skillet Bundles"))
	b.WriteString(m.helpView())
	b.WriteString("\n\n")

	switch {
	case m.filterFoundNothing():
		b.WriteString(m.zeroMatchLine())
	case len(m.bundles) == 0:
		b.WriteString("No Bundles yet. Press n to create one.\n")
	default:
		b.WriteString(m.bundleList.View())
		b.WriteString("\n")
	}
}

func newSkillList(items []list.Item, marks *markSet) list.Model {
	model := list.New(items, skillDelegate{marks: marks}, 0, 0)
	model.SetShowTitle(false)
	// See newLibraryList: filtering on, the list's own filter bar off — the
	// Model renders the filter prompt itself in renderFilterLine.
	model.SetShowFilter(false)
	model.SetFilteringEnabled(true)
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
	if len(m.inv.Skills) == 0 || m.filterActive() {
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

// syncMainCursor maps the list's selection back onto an index into
// inv.Skills. It counts skill rows in the *unfiltered* item list rather than
// matching on Location: buildListItems preserves inventory order, and a
// Location match is ambiguous whenever two rows share one (or have none).
func (m *Model) syncMainCursor() {
	selected, ok := m.list.SelectedItem().(skillItem)
	if !ok {
		m.cursor = 0
		return
	}
	skillIndex := 0
	for _, item := range m.list.Items() {
		row, isSkill := item.(skillItem)
		if !isSkill {
			continue
		}
		if row == selected {
			m.cursor = skillIndex
			return
		}
		skillIndex++
	}
	m.cursor = 0
}

func (m *Model) selectMainCursor() {
	items := m.list.VisibleItems()
	if len(m.inv.Skills) == 0 || len(items) == 0 {
		m.cursor = 0
		m.list.Select(0)
		return
	}

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
	if m.view == mainView && m.costHeaderLine() != "" {
		reserved++ // the per-session cost line under the title
	}
	if m.view == mainView && m.markedLine() != "" {
		reserved++ // the marked-count line under the cost header
	}
	if notices := m.visibleInventoryNotices(); m.view == mainView && len(notices) > 0 {
		reserved += 2 + len(notices) // blank line, "Notices" line, one per notice
	}
	if m.status != "" {
		reserved += 2 // blank line, status line
	}
	if m.installing != "" {
		reserved += 2 // blank line, install-progress line (below any error)
	}
	if m.filterActive() {
		reserved += 2 // blank line, filter line
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

func (m *Model) currentKeyMap() keyMap {
	switch m.view {
	case archiveView:
		return archiveKeyMap(len(m.archive) > 0, m.help.ShowAll)
	case libraryView:
		return libraryKeyMap(len(m.library) > 0, m.help.ShowAll)
	case bundleView:
		return bundleKeyMap(len(m.bundles) > 0, m.help.ShowAll)
	default:
		selected, ok := m.selectedMainSkill()
		return mainKeyMap(selected, ok, m.help.ShowAll)
	}
}

// helpView renders the compact help across as many lines as ShortHelpRows
// needs, so every mode-changing key is visible without pressing `?`. `?` still
// swaps in the grouped full help.
func (m *Model) helpView() string {
	km := m.currentKeyMap()
	if m.help.ShowAll {
		return m.help.FullHelpView(km.FullHelp())
	}
	var lines []string
	for _, row := range km.ShortHelpRows() {
		if rendered := m.help.ShortHelpView(row); strings.TrimSpace(rendered) != "" {
			lines = append(lines, rendered)
		}
	}
	return strings.Join(lines, "\n")
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
