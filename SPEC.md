# SPEC: Skillet phase 2 — TUI redesign + Project-skill support

This spec covers the two workstreams agreed for phase 2 (grilled and recorded
2026-07-08; see `CONTEXT.md`'s **Source**/**Tool**/**Project skill** entries,
`docs/adr/0002-bubbles-lipgloss-rendering.md`, and
`docs/adr/0003-source-tool-axis-project-scope.md`). Read those three first —
this spec quotes/grounds against them and against
`docs/research/skill-mechanisms.md` (on-disk mechanisms) and the existing v1
code under `internal/engine/` and `internal/tui/`, which this spec extends
rather than replaces. The old `SPEC.md` for v1 (issues #2-#5) is available in
git history (`git log --follow -- SPEC.md`) if you need the original
package-layout rationale.

**Sequencing: implement Part 1 (TUI redesign) before Part 2 (Project-skill
support).** Part 2's fourth Source group is designed to slot into Part 1's
list+detail structure; building it against the old flat-string renderer would
mean redoing the display logic twice.

Do not implement Shortlist or Seeding (CONTEXT.md defines the terms; no code
exists yet) — that's a separate future phase, out of scope here.

---

## Part 1 — TUI redesign (bubbles + lipgloss)

### Problem

`internal/tui/model.go` (427 lines) renders everything through
`fmt.Sprintf`/`strings.Builder` with zero use of `lipgloss` or `bubbles` (both
already pulled in as transitive dependencies of `bubbletea` — see ADR 0002).
Concretely, this produces:
- Descriptions truncated mid-word at a fixed 72 chars (`truncate` in
  `model.go`).
- A single 200+ character help line that doesn't wrap
  (`"up/k down/j move  u archive Personal/Codex  s suppress/..."`).
- Confirmation prompts that replace the *entire* screen with one plain line,
  losing all list context.
- No color anywhere — Source group headers are bare text
  (`b.WriteString(string(current))`), Activation states aren't visually
  distinguished.
- No `tea.WindowSizeMsg` handling at all — the program never learns the
  terminal's dimensions, which also blocks doing a real list+detail split
  (you need a width/height to lay out two panes).

### New dependencies

Promote `github.com/charmbracelet/bubbles` and `github.com/charmbracelet/lipgloss`
from indirect to direct in `go.mod` (`go get github.com/charmbracelet/bubbles`
— lipgloss will already resolve via bubbles' own requirements at a version
compatible with the existing `bubbletea v1.2.4`).

### Package layout (`internal/tui/`)

```
internal/tui/
  model.go      — top-level tea.Model: owns the bubbles list.Model, detail
                  viewport, help.Model, and confirmation overlay state; routes
                  Update() to the right sub-component; Init() now returns
                  tea.EnterAltScreen (unchanged) — no new Cmd needed for size,
                  bubbletea sends an initial tea.WindowSizeMsg automatically
  items.go      — the list.Item implementation(s): a skillItem wrapping
                  engine.Skill (+ engine.Tool, see Part 2) and a groupHeaderItem
                  for the styled, non-selectable Source section dividers
  delegate.go   — a custom list.ItemDelegate: renders skillItem as a single
                  styled line (name, Activation badge, Tool badge for Project
                  rows) and groupHeaderItem as a full-width styled divider;
                  Height()/Spacing() define row geometry
  detail.go     — renders the detail pane for the currently-selected skillItem
                  (full untruncated Description, Location, Activation,
                  Source/Tool, and for Plugin skills the "one of N in X" line)
  styles.go     — every lipgloss.Style as a package-level var: pane borders,
                  the four Source header styles, Activation-state colors
                  (Auto/Manual-only/Suppressed/Disabled), all via
                  lipgloss.AdaptiveColor{Light: "...", Dark: "..."} so it
                  reads correctly in both terminal color schemes
  confirm.go    — renders the confirmation modal: a bordered lipgloss box
                  (styles.go's confirmBoxStyle), composited over a dimmed
                  copy of the current list+detail view. bubbles/lipgloss have
                  no built-in overlay compositor — implement with
                  lipgloss.Place(width, height, lipgloss.Center,
                  lipgloss.Center, box, lipgloss.WithWhitespaceForeground(...))
                  rendered on top of the background string; simplest correct
                  approach is to render background and box separately, then
                  overwrite the background's center lines with the box's
                  lines at the same column offset (a small manual splice
                  function — bubbles has no "dim a view" helper, so approximate
                  dimming by rendering the background through a faint/gray
                  lipgloss style rather than true alpha blending, which
                  terminals can't do anyway)
  keys.go       — key.Binding definitions (upDown, archive, suppress,
                  manualOnly, uninstallPlugin, switchView, quit, showFullHelp)
                  with ShortHelp()/FullHelp() implementing help.KeyMap, so
                  bubbles' help.Model can render both the short hint and the
                  '?'-expanded full list; each binding's Enabled() reflects
                  whether it applies to the currently-selected row (e.g.
                  uninstallPlugin.Enabled() is false when a Personal skill is
                  selected) so the help bar never lists a dead key
  archive.go    — archive view's list.Model + delegate (reuses items.go's
                  patterns; archive rows have no "group header" concept, just
                  a flat list sorted by ArchivedAt descending, matching
                  Engine.ListArchive()'s existing order)
```

### Behavior, decision by decision

1. **List + detail pane.** Split the terminal width roughly 60/40 (exact
   ratio is your call — bias toward the list, since names/activation states
   need to stay scannable) on every `tea.WindowSizeMsg`, calling
   `list.SetSize(...)` and updating the detail viewport's width/height. The
   list rows drop the description entirely (name, Activation badge, and for
   Plugin/Project the count/Tool tag); the detail pane shows the selected
   item's full description and metadata. This fully replaces `truncate()` —
   delete it once nothing calls it.

2. **One list, styled section headers, four Source groups.** `bubbles`'
   `list.Model` has no native "section header" concept — implement it by
   inserting a non-selectable `groupHeaderItem` before the first item of each
   new `Source` as the item slice is built (mirroring the existing
   `current != skill.Source` transition check in `renderMain`), and make the
   delegate's `Update()`/cursor movement skip over header items when the user
   presses up/down (bubbles' list moves index-by-index with no built-in
   "skip this row" — wrap the up/down key handling to loop past header rows).
   Order stays Personal, Plugin, Codex, Project (extending the existing
   `sourceSortOrder` — see Part 2).

3. **Confirmation overlay modal.** On any pending-confirm action, keep
   rendering the list+detail view underneath (dimmed via style, not truly
   removed) and composite the bordered confirmation box centered on top via
   `lipgloss.Place`. The description text inside the box is unchanged from
   today's `pendingConfirm.description` strings — only the presentation
   changes. `y` confirms, any other key cancels — unchanged behavior.

4. **Contextual help bar.** Replace the always-full key line with `bubbles`'
   `help.Model`: short hint by default, `?` toggles `ShowAll` for the full
   list. Bindings' `Enabled()` (see keys.go above) hides keys that don't apply
   to the current selection — e.g. selecting a Codex prompt hides
   `manualOnly` (prompts have no toggle, per `docs/research/skill-mechanisms.md`).

5. **Activation-state color coding.** Auto = default/neutral text color,
   Manual-only = amber/yellow, Suppressed = orange, Disabled = red — all as
   `lipgloss.AdaptiveColor` pairs so they're legible on both light and dark
   terminal backgrounds. This wasn't explicitly asked about in the interview
   but follows directly from "make Activation scannable at a glance," which
   was the stated pain point; treat exact hex values as your call, not a
   fixed requirement.

6. **Window resize handling.** Add a `case tea.WindowSizeMsg:` branch to
   `Update()` that resizes the list, detail viewport, and cached width/height
   used by the confirm overlay's `lipgloss.Place` call. This is a correctness
   fix (the v1 program silently never adapts to terminal size), not
   optional — needed for the list+detail split to render at all.

Existing keybindings (`u`/`s`/`m`/`x`/`a`/`q`) and their gating logic (which
Source/Kind each applies to) are unchanged by Part 1 — only their
presentation (via `keys.go`/`help.Model`) and the confirmation flow change.
Part 2 extends *which* skills those gates accept (Project skills), not how
the keys themselves work.

### Testing

Still no automated tests for the `tea.Model` itself (matches the v1 decision
— "verified by running it against a real install," now also verified via the
`run`/`verify` skills if available). Do add plain Go unit tests for anything
extracted into pure functions with no `tea` dependency — e.g. the group-header
insertion logic (`items.go`, if written as a standalone
`buildListItems(inventory) []list.Item` function) and any key-binding
`Enabled()` logic that takes a `Skill` and returns a bool, since both are
easy to get subtly wrong (off-by-one on header skip, wrong gating) and cheap
to test without spinning up a `tea.Program`.

---

## Part 2 — Project-skill support

### Problem

`README.md`: "Project skills (installed inside a single repository) are not
yet supported." `CONTEXT.md` already defines the term. The v1 engine
resolves everything from `$HOME` only (`cmd/skillet/main.go`) — Project
support requires a working-directory-relative scope that didn't exist before.

### Types (`internal/engine/types.go`)

```go
type Source string

const (
	SourcePersonal Source = "Personal"
	SourcePlugin   Source = "Plugin"
	SourceCodex    Source = "Codex"
	SourceProject  Source = "Project" // new
)

// Tool is orthogonal to Source (see ADR 0003): which underlying system's
// mechanisms govern this skill. Personal and Plugin skills are always
// ToolClaudeCode; Codex-source skills are always ToolCodex; Project-source
// skills vary and this is the only place the distinction is visibly shown.
type Tool string

const (
	ToolClaudeCode Tool = "Claude Code"
	ToolCodex      Tool = "Codex"
)
```

Add `Tool Tool` to `Skill` — set by every scanner, not just the new Project
one (`scanPersonal`/`scanPlugins` always set `ToolClaudeCode`, `scanCodex`
always sets `ToolCodex`).

Add to `ArchiveEntry`:

```go
	Tool       Tool   `json:"tool"`
	OriginRepo string `json:"originRepo,omitempty"` // absolute path to the
	// resolved project directory this entry was archived from; empty for
	// every non-Project source. Used by ListArchive's repo filter (below).
```

### Roots (`internal/engine/engine.go`)

```go
type Roots struct {
	ClaudeHome  string
	CodexHome   string
	AgentsHome  string
	DataDir     string
	ProjectRoots []string // new: resolved candidate project directories for
	                       // this run, in priority order; empty if none found
	                       // (skillet launched outside any repo). See
	                       // FindProjectRoots below — computed once by
	                       // cmd/skillet/main.go before constructing Engine.
}
```

### Resolving Project scope (new `internal/engine/project_root.go`)

Per ADR 0003 and `docs/research/skill-mechanisms.md`'s exact quote —
"REPO: `$CWD/.agents/skills`, `$CWD/../.agents/skills`,
`$REPO_ROOT/.agents/skills`" — Codex's own repo-scope discovery is **three
fixed candidate directories**, not an arbitrary walk-up through every
ancestor: the current directory, its immediate parent, and the git repo root.
Mirror this exactly for consistency between the two tools Skillet manages:

```go
// FindProjectRoots returns the candidate project directories for cwd, in
// priority order: cwd itself, cwd's parent, and the git repo root (found by
// walking up looking for a .git entry — a directory for a normal repo, a
// file for a worktree/submodule, per git's own convention). Duplicates
// (e.g. cwd IS the repo root) are removed, keeping the first occurrence.
// Returns an empty slice if cwd is not inside a git repo AND neither cwd
// nor its parent has anything to offer — the caller still checks cwd/parent
// even without a git root, matching Codex's own three-candidate list.
func FindProjectRoots(cwd string) []string
```

**Verification gap to close before/during implementation:** this exact
three-candidate rule is documented and confirmed for Codex's `.agents/skills`
discovery. It is *not* independently confirmed for Claude Code's own
`.claude/skills` project-scope discovery (no equivalent statement exists in
`docs/research/skill-mechanisms.md` — that research only covers user-level
Claude Code paths). Do a quick check against
`https://code.claude.com/docs/en/skills.md` (or empirically, by creating a
probe `.claude/skills/` at each of the three candidate levels and confirming
which ones Claude Code actually picks up) before assuming it's identical.
If it turns out Claude Code only ever looks at the exact cwd (no parent/root
walk-up), apply `FindProjectRoots`'s three-candidate list only to the
`.agents/skills` (Codex) scan and restrict the `.claude/skills` scan to
`cwd` alone — flag this as a follow-up notice in code comments either way,
the same way `skill-mechanisms.md` tracks its own gaps.

`cmd/skillet/main.go` calls `os.Getwd()` and `FindProjectRoots(cwd)` before
constructing `engine.Roots{...}`, adding the result as `ProjectRoots`.

### Scanning (`internal/engine/project.go`, new)

Reuse the existing per-folder parsing logic rather than duplicating it:

- Extract the innermost per-entry loop body from `scanPersonal`
  (`internal/engine/personal.go`) into a shared helper parameterized by
  `Source`/`Tool`, e.g. `scanClaudeSkillFolder(root string, source Source, tool Tool) ([]Skill, []Notice)`.
  `scanPersonal(claudeHome)` becomes a thin wrapper calling this with
  `SourcePersonal, ToolClaudeCode`. **Preserve `scanPersonal`'s existing
  "directory not found" Notice behavior for the user-level call** (tested in
  `personal_test.go`) — do not change that test's expectations.
- Similarly, add a `source Source` parameter to `scanCodexSkillRoot`
  (`internal/engine/codex.go`), defaulting existing call sites (from
  `scanCodex`) to pass `SourceCodex`, and set `Tool: ToolCodex` unconditionally
  inside it (it's always the Codex mechanism regardless of Source).
- New `scanProject(roots []string, dataDir string) ([]Skill, []Notice)`:
  for each candidate in `roots` (already deduplicated by `FindProjectRoots`),
  check `<candidate>/.claude/skills` and `<candidate>/.agents/skills` with a
  plain `os.Stat` first — **if a candidate's directory doesn't exist, skip it
  silently, no Notice.** Unlike the user-level scans (where a missing
  `~/.claude/skills` is notable), 2 of the 3 project candidates almost always
  won't have skill directories — emitting a Notice for each would make the
  Notices section noisy on every single run. Only emit a Notice for a
  directory that exists but is unreadable, or for a malformed `SKILL.md`
  inside one that does exist (same as every other scanner). Call
  `scanClaudeSkillFolder(dir, SourceProject, ToolClaudeCode)` and
  `scanCodexSkillRoot(dir, SourceProject, disabled)` (Codex's `config.toml`
  disabled-set — read it once via `readCodexDisabledConfig(codexHome)`,
  passed in or re-read; duplicating that one small file read is fine, don't
  over-engineer a shared cache for it) for the respective sub-paths.
- Track which candidate directory produced each skill (needed for
  `OriginRepo` at archive time) — either by setting a not-yet-in-`Skill`
  field, or (simpler, since `Skill.Location` is already the absolute skill
  folder path) deriving `OriginRepo` later from `Location` by checking which
  `ProjectRoots` entry it falls under, in `archive.go`'s `classifyLocation`
  (below) rather than threading a new field through `Skill`.

### Inventory (`internal/engine/inventory.go`)

Add the fourth scan alongside the existing three, extend
`sourceSortOrder` with `SourceProject: 3` (default case becomes 4), and set
`Tool` uniformly — `scanPersonal`/`scanPlugins` results get `ToolClaudeCode`
tagged (if not already set by the scanner itself per the extraction above),
`scanCodex` results get `ToolCodex`.

### Action dispatch — minimal diffs, mechanisms already generalize

The actual mechanism functions operate purely on `Skill.Location` (a folder
path) and don't care which root produced it — confirmed by reading
`setPersonalManualOnly`, `setCodexManualOnlyForSkill`
(`internal/engine/manual_only.go`), and `suppressCodex`
(`internal/engine/codex_suppress.go`): all three take a `Skill` and operate
on `skill.Location`/`skill.Location + "/SKILL.md"` directly, with no
Claude-home or Codex-home path baked in except `suppressCodex`'s writes to
`<CodexHome>/config.toml`, which is correct to keep at the user level even
for a Project skill (the `path`-keyed entry disambiguates by the skill's own
absolute path — there is no evidence Codex supports a *project-scoped*
config.toml, and none is needed since path-keying already works from any
location). **This means only the dispatch `switch` statements need new
`case`s — no mechanism function changes:**

```go
// SetManualOnly (manual_only.go): add
case skill.Source == SourceProject && skill.Tool == ToolClaudeCode && skill.Kind == KindSkill:
	return setPersonalManualOnly(skill, manualOnly)
case skill.Source == SourceProject && skill.Tool == ToolCodex && skill.Kind == KindSkill:
	return setCodexManualOnlyForSkill(skill, manualOnly)

// Suppress / Unsuppress (suppress.go): add
case skill.Source == SourceProject && skill.Tool == ToolCodex && skill.Kind == KindSkill:
	return suppressCodex(e.roots.CodexHome, skill)   // / unsuppressCodex, respectively
```

There is deliberately **no** `Suppress` case for `SourceProject &&
ToolClaudeCode` — matching today's rule that Suppress only applies to Plugin
and Codex-mechanism skills; a Project/Claude-Code skill gets Archive +
Manual-only only, same action set as Personal.

### Archive (`internal/engine/archive.go`)

`classifyLocation` currently checks `immediateChildOf` against
`ClaudeHome/skills`, `CodexHome/skills`, `AgentsHome/skills`,
`CodexHome/prompts` and returns `(Source, Kind, error)`. Extend its return to
`(Source, Kind, Tool, originRepo string, error)` (or an equivalent small
struct — your call) and add, for each entry in `e.roots.ProjectRoots`, two
more checks: `immediateChildOf(<root>/.claude/skills, location)` →
`(SourceProject, KindSkill, ToolClaudeCode, root, nil)`, and
`immediateChildOf(<root>/.agents/skills, location)` →
`(SourceProject, KindSkill, ToolCodex, root, nil)`. `Uninstall` sets the new
`ArchiveEntry.Tool`/`.OriginRepo` fields from this result (empty `OriginRepo`
for every non-Project source, matching the `omitempty` json tag).

`ListArchive` needs a repo filter, per the resolved decision (Question 10 —
"filter to current repo"):

```go
func (e *Engine) ListArchive() ([]ArchiveEntry, error) // unchanged signature —
// filtering happens by comparing each entry's OriginRepo against
// e.roots.ProjectRoots, which is already resolved once per run and stored on
// Engine, so no new parameter is needed. Rule: keep every entry where
// Source != SourceProject (always global — Personal/Codex/Plugin, unchanged
// from v1), plus every entry where OriginRepo is present in e.roots.ProjectRoots.
// A Project entry archived from a different repo than the one skillet is
// currently running in is simply omitted from this run's Archive view (it
// still exists on disk under ~/.skillet and reappears when skillet is next
// run from that repo).
```

### `cmd/skillet/main.go`

```go
func main() {
	home, err := os.UserHomeDir()
	if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
	cwd, err := os.Getwd()
	if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }

	e := engine.New(engine.Roots{
		ClaudeHome:   filepath.Join(home, ".claude"),
		CodexHome:    filepath.Join(home, ".codex"),
		AgentsHome:   filepath.Join(home, ".agents"),
		DataDir:      filepath.Join(home, ".skillet"),
		ProjectRoots: engine.FindProjectRoots(cwd),
	})
	// ... unchanged tea.NewProgram / p.Run()
}
```

### TUI changes needed for Part 2 (on top of Part 1's structure)

- Add a fourth styled group header ("Project") in `items.go`/`delegate.go`.
- Render the `Tool` tag on Project rows only (e.g. a small `[Claude Code]` /
  `[Codex]` badge after the name) — per ADR 0003, Personal/Plugin/Codex rows
  never show a Tool tag since it would be redundant there.
- Extend `keys.go`'s `Enabled()` checks (archive/manualOnly/suppress
  bindings) to accept `SourceProject` alongside the existing
  `SourcePersonal`/`SourceCodex` checks, matching the new engine dispatch
  cases above exactly — the TUI's gating must never offer an action the
  engine would reject, and must never hide one the engine now accepts.

### Testing

New `internal/engine/project_test.go`, `project_root_test.go` (or combined),
following the existing `t.TempDir()` fixture pattern (no mocks) used
throughout `internal/engine/*_test.go`:

- `FindProjectRoots`: a fixture repo with a `.git` directory at the root, a
  nested working directory 2+ levels deep — assert the three candidates
  resolve correctly (cwd, parent, repo root) and are deduplicated when cwd
  equals the repo root (single-level repo) or cwd's parent equals the repo
  root.
- No `.git` anywhere in the fixture tree (or up to a synthetic boundary) —
  assert `FindProjectRoots` still returns cwd and its parent as candidates,
  just no third "repo root" entry, and doesn't walk indefinitely up the real
  filesystem past the fixture's temp directory (bound the walk-up, e.g. stop
  at a passed-in ceiling directory in tests to avoid depending on the real
  machine's directory structure — consider taking an optional ceiling param
  purely for testability if the real implementation would otherwise walk to
  `/`).
- `scanProject`: a fixture with `.claude/skills/<name>/SKILL.md` at the
  resolved repo root only (not at cwd or parent) → the skill appears with
  `Source: SourceProject, Tool: ToolClaudeCode`, and candidates without a
  `.claude/skills`/`.agents/skills` directory produce **zero Notices** (this
  is the specific "don't be noisy" requirement above — assert
  `len(notices) == 0` for the missing-directory-is-normal case, as distinct
  from the existing Personal/Codex tests which assert a Notice *is* present
  for their missing-root case).
- A Codex repo-scoped skill (`.agents/skills/<name>/SKILL.md` at the repo
  root, plus an `agents/openai.yaml` with `allow_implicit_invocation: false`)
  → appears with `Source: SourceProject, Tool: ToolCodex, Activation:
  ActivationManualOnly`.
- `SetManualOnly`/`Suppress` dispatch: call each on a fixture `Skill{Source:
  SourceProject, Tool: ToolClaudeCode, ...}` and `{Source: SourceProject,
  Tool: ToolCodex, ...}` and assert the same file mutations as the existing
  Personal/Codex tests (`manual_only_test.go`, `suppress_test.go`) — these
  should pass with zero changes to the underlying mechanism functions, so a
  failing test here means the dispatch `case` was written wrong, not that the
  mechanism itself needs work.
- Archive round-trip for a Project skill (mirroring `archive_test.go`'s
  existing Personal round-trip): Uninstall → `ArchiveEntry.OriginRepo` set
  correctly → Restore → tree byte-identical.
- `ListArchive` repo filter: archive one Project skill from fixture repo A
  and one from fixture repo B, construct an `Engine` with `ProjectRoots`
  matching only repo A, assert `ListArchive()` returns repo A's entry plus
  any Personal/Codex/Plugin entries, but not repo B's.

Run `go vet ./...` and `go test ./...` for both parts before considering
either done.

---

## Explicitly out of scope for this pass

- Shortlist and Seeding (CONTEXT.md defines both terms; no code exists) —
  a later phase, not phase 2.
- Any change to Plugin-skill scope (plugins remain user-level only; there is
  no "Project plugin" concept — CONTEXT.md's Source/Tool split intentionally
  only adds Project as a peer of Personal/Plugin/Codex, it does not turn
  Plugin into a fourth Tool-bearing group).
- Any settings/config file for Skillet itself (e.g. to override
  `FindProjectRoots`' behavior) — cwd walk-up is the only resolution
  mechanism per ADR 0003; don't add a flag "just in case."

## Verification

```
go build ./...
go vet ./...
go test ./... -v
```

All must pass. For the TUI redesign specifically, also run the built binary
interactively (`go run ./cmd/skillet`) from inside this repo (which now has
no `.claude/skills` or `.agents/skills` of its own — you may need to create a
throwaway fixture skill under one of those to see the Project group render)
and confirm: the list+detail split renders correctly at a real terminal size,
resizing the terminal doesn't break layout, `?` toggles the full help list,
and a confirmation overlay renders on top of a visibly-dimmed list rather
than replacing it. Do not commit, push, or modify anything outside this
repo. Report back: files changed, a summary of what each new/modified
function does, the verification command output, and anything you were
uncertain about or had to make a judgment call on — especially the Claude
Code project-scope discovery verification gap flagged above.
