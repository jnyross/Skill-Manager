# SPEC: Skillet phase 3 — Library + Bundle (Install)

This spec covers the phase agreed via `/grill-with-docs` (grilled and
recorded 2026-07-08; see `CONTEXT.md`'s **Library**, **Bundle**, and
**Install** entries, and `docs/adr/0004-library-install-source-pointers.md`).
Read those first — this spec quotes/grounds against them and against
`docs/research/skill-mechanisms.md` and the existing code under
`internal/engine/` and `internal/tui/`, which this spec extends rather than
replaces. Phase 2's `SPEC.md` (issues #14-#21) is available in git history
(`git log --follow -- SPEC.md`) if you need that rationale.

Phase 2 closed every open issue (#2-#21); this is genuinely new scope, not a
fix-up pass. `CONTEXT.md`'s old **Shortlist**/**Seeding** entries — written
before this session — are superseded by **Library**/**Bundle**/**Install**;
do not resurrect the old "preauthorised, one-time copy, one item at a time"
model anywhere in this work.

**Sequencing: Part 0 (research) before anything else.** Two of the four
install-source types this spec requires (skills.sh, and installing a
brand-new Claude Code plugin) have no prior code in this repo and are not
fully confirmed by public docs (see Part 0). Do not guess at their mechanics
and start writing `Install` against an assumption — verify first, the same
way `docs/research/skill-mechanisms.md` preceded Suppress (the previous
highest-risk mechanism in this codebase). Part 1 (data model) can proceed in
parallel with Part 0 since it doesn't depend on the research's outcome.
Parts 2 (TUI) and 3 (Install engine) depend on both.

---

## Part 0 — Research spike: install mechanics

Write findings to `docs/research/library-install-mechanisms.md`, matching
`skill-mechanisms.md`'s style (cite sources, mark what's verified locally vs.
inferred, flag gaps explicitly rather than silently resolving them).

### skills.sh CLI targeting

The documented install command is `npx skills add <owner>/<repo>` (see
https://www.skills.sh/ and https://www.skills.sh/docs — both fetched
2026-07-08, neither specifies install scope or version semantics in the
publicly visible content). Before Part 3 can implement the skills.sh source
type, verify locally, in a scratch directory:

- Does `npx skills add owner/repo` install relative to the **current working
  directory** (e.g. into `./.claude/skills/`), matching the `npm install`
  convention of operating on cwd? If so, Personal-target installs mean
  running it with cwd set to `~/.claude` (or wherever it actually resolves
  under), and Project-target installs mean running it with cwd set to the
  repo root.
- Is there a global/user-level flag, or is every install project-relative by
  design (i.e. does "Personal" even make sense for a skills.sh source, or
  does Skillet need to run it into a scratch dir and copy the result into
  `~/.claude/skills` itself)?
- Does it pin a version/commit anywhere Skillet could read back (for the
  Library entry's Location/version display), or is "latest" implicit and
  unqueryable — i.e. does Skillet just trust whatever the command produced?
- Confirm what `owner/repo` actually resolves to — a GitHub repo path
  directly, or an indirect registry lookup (the site's own copy suggests a
  registry/leaderboard sits in front of raw GitHub repos).

### Installing a new Claude Code plugin programmatically

Skillet has never installed a plugin before — only uninstalled
(`plugin_uninstall.go`) or suppressed a skill within one (`suppress.go`).
`skill-mechanisms.md` confirms the on-disk shape of an *already-installed*
plugin (`installed_plugins.json`, `known_marketplaces.json`,
`enabledPlugins` map in `settings.json` at user/project/project-local scope —
see that doc's "Where things live on disk" section) but does not cover how a
**new** plugin gets added. Verify:

- Does `claude plugin install <marketplace>/<plugin> --scope project|user`
  (referenced in `skill-mechanisms.md` line 42) actually exist as a
  documented CLI command Skillet can shell out to? Confirm the exact syntax
  against https://code.claude.com/docs/en/plugins-reference.md.
- Does it require the marketplace to already be known
  (`known_marketplaces.json`), or does it add one inline given a
  `github:owner/repo` reference?
- What does it do if the target scope's `settings.json` doesn't exist yet
  (a repo with no `.claude/` directory at all)?
- Confirm whether shelling out to `claude plugin install` is preferable to
  writing `installed_plugins.json`/`known_marketplaces.json`/`settings.json`
  directly — the CLI is very likely the safer choice (matches how the tool
  itself keeps those three files consistent), but confirm it's scriptable
  non-interactively before committing to it.

If either mechanism turns out to be materially different from what's
sketched in Parts 1-3 below, update this spec's Part 3 before implementing —
don't silently reconcile the mismatch in code with no record of why.

---

## Part 1 — Data model

### `internal/engine/types.go` additions

```go
type LibrarySourceKind string

const (
	LibrarySourceSkillsSh     LibrarySourceKind = "skills.sh"
	LibrarySourceGit          LibrarySourceKind = "git"
	LibrarySourceMarketplace  LibrarySourceKind = "marketplace"
	LibrarySourceLocalPath    LibrarySourceKind = "local-path"
)

// LibrarySource is an install-source descriptor (CONTEXT.md's Library entry:
// "a skills.sh owner/repo reference, a git URL, a Claude/Codex marketplace
// pointer, or a local filesystem path"). Exactly one field group is set,
// selected by Kind — this is Go's usual tagged-union-by-string-field
// approach, matching PluginInfo's existing shape rather than introducing an
// interface hierarchy for four fixed cases.
type LibrarySource struct {
	Kind LibrarySourceKind

	// Kind == LibrarySourceSkillsSh
	SkillsShRepo string // "owner/repo"

	// Kind == LibrarySourceGit
	GitURL     string
	GitRef     string // optional branch/tag/commit; empty = default branch
	GitSubPath string // optional path within the repo to the skill folder; empty = repo root

	// Kind == LibrarySourceMarketplace
	Marketplace string
	PluginName  string

	// Kind == LibrarySourceLocalPath
	LocalPath string
}

// LibraryEntry is one item in the user's Library (CONTEXT.md: "the user's
// own catalog of skills and plugins they maintain... never Project"). Tool
// only applies when Kind == KindSkill (a Plugin entry has no Tool — plugins
// are a Claude-Code-only concept, see Source's existing doc comment).
type LibraryEntry struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Kind        Kind          `json:"kind"` // KindSkill or a plugin (see note below)
	Tool        Tool          `json:"tool,omitempty"`
	Source      LibrarySource `json:"source"`
	AddedAt     time.Time     `json:"addedAt"`
}

// BundleMember pairs a Library entry with this Bundle's own remembered
// Activation preference (CONTEXT.md's Bundle: "the same Library skill can be
// Auto in one Bundle and Manual-only in another"). ActivationSuppressed and
// ActivationDisabled are not valid here — a Bundle only ever expresses the
// two activation states Install can actually set on first placement
// (Manual-only toggle or its absence); Suppress is a separate, later action
// on an already-installed skill, never a Bundle-time concern.
type BundleMember struct {
	LibraryEntryID string          `json:"libraryEntryId"`
	Activation     ActivationState `json:"activation"` // ActivationAuto or ActivationManualOnly
}

type Bundle struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Members []BundleMember `json:"members"`
}

// InstallTarget is where an Install places its result — Personal (the
// user-level Claude Code/Codex skill directories) or a specific repo
// (Project-scoped, same two-tool split Source/Project already has).
type InstallTargetKind string

const (
	InstallTargetPersonal InstallTargetKind = "personal"
	InstallTargetProject  InstallTargetKind = "project"
)

type InstallTarget struct {
	Kind InstallTargetKind
	// RepoRoot is set only when Kind == InstallTargetProject — an absolute
	// path from e.roots.ProjectRoots/ClaudeProjectRoots, never arbitrary
	// user input, so Install never writes outside a resolved project root.
	RepoRoot string
}
```

A `LibraryEntry` needs a `Kind` distinguishing a skill entry from a plugin
entry; unlike `Skill`, a plugin *is* the entry (there's no per-skill-within-
plugin granularity in the Library — CONTEXT.md's Bundle preview always shows
whole-plugin entries, e.g. "plugin C (Auto)"). Confirm during implementation
whether `Kind` needs a third value or whether `Source.Kind ==
LibrarySourceMarketplace` is itself sufficient to mean "this entry is a
plugin" (making a separate `Kind` field redundant for Library entries
specifically) — don't carry two ways of saying the same thing.

### Persistence — `internal/engine/library.go`, `internal/engine/bundle.go`

Follow `suppress.go`'s established pattern for Skillet-owned records: one
JSON file per entry under `<DataDir>/library/<id>.json` and
`<DataDir>/bundles/<id>.json` (see `suppress.go`'s
"Suppression record storage" section and `loadSuppressionRecords`/
`writeSuppressionRecord`/`removeSuppressionRecord` for the load-all/write-one/
remove-one shape to mirror) — not the archive's move-a-directory model
(nothing here is a file being relocated) and not a single flat list file
(one-file-per-id keeps concurrent-write risk and diffing behavior consistent
with how suppression records already work).

```go
func (e *Engine) ListLibrary() ([]LibraryEntry, error)
func (e *Engine) AddLibraryEntry(entry LibraryEntry) (LibraryEntry, error) // assigns ID, AddedAt
func (e *Engine) RemoveLibraryEntry(id string) error // bookkeeping only — never touches any installed copy

func (e *Engine) ListBundles() ([]Bundle, error)
func (e *Engine) CreateBundle(name string) (Bundle, error)
func (e *Engine) DeleteBundle(id string) error
func (e *Engine) AddBundleMember(bundleID, libraryEntryID string, activation ActivationState) error
func (e *Engine) RemoveBundleMember(bundleID, libraryEntryID string) error
func (e *Engine) SetBundleMemberActivation(bundleID, libraryEntryID string, activation ActivationState) error
```

`RemoveLibraryEntry` on an entry still referenced by one or more Bundles: per
the "one-way copy, no ongoing sync" decision (ADR 0004), a Bundle only
remembers `LibraryEntryID` + Activation, so removing the Library entry leaves
a dangling reference. Resolve during implementation whether
`RemoveLibraryEntry` should refuse (return an error naming the referencing
Bundles) or cascade (silently drop the member from every Bundle) — this
wasn't grilled explicitly; refusing is the safer default (matches Skillet's
existing philosophy of never silently mutating something the user didn't
directly act on) unless testing reveals it's more annoying than protective.

---

## Part 2 — TUI: two new views

Adds `libraryView` and `bundleView` to `viewState` in `model.go`, alongside
the existing `mainView`/`archiveView`. Follow `archive.go`'s
list+delegate+render pattern (`archiveItem`, `archiveDelegate`,
`buildArchiveItems`, `newArchiveList`) for both new views rather than
inventing a new list-rendering approach.

### Keybindings (proposed — not separately grilled; keep if they read
naturally during implementation, adjust if they collide or confuse)

Main view additions:
- `l` — toggle "add to/remove from Library" for the selected row. Adding
  auto-derives the `LibrarySource` from what Skillet already knows about
  that row: a Personal/Codex skill's `Skill.Location` becomes
  `LibrarySourceLocalPath`; a Plugin skill's `Skill.Plugin.{Marketplace,
  Plugin}` becomes `LibrarySourceMarketplace`. No manual descriptor entry
  needed for this path (confirmed in-session: "Both" — row-toggle handles
  the already-installed case; the Library view's own "new entry" flow, below,
  handles the not-yet-installed case).
- `L` — switch to Library view (mirrors existing `a` for Archive).
- `B` — switch to Bundle view.

Library view:
- `n` — new entry from scratch: prompts for `LibrarySourceKind` then the
  corresponding field(s) (skills.sh `owner/repo`, git URL [+ optional ref/
  subpath], marketplace + plugin name, or a local path) and a display name.
- `d` — remove the selected entry from the Library (bookkeeping only, per
  `RemoveLibraryEntry` above — never deletes anything on disk).
- `i` — Install the selected entry: opens a target picker (Personal, or one
  entry per resolved project root) then confirm-and-overwrite (existing
  `pendingConfirm` pattern) if something already exists at that name/target.
- `L`/`esc` — back to main view.

Bundle view:
- `n` — new Bundle (prompts for a name).
- `enter`/`space` on a Bundle — expand to show/edit its members: add from a
  Library-entry picker, remove a member, cycle a member's Activation
  (`a`/`m`, matching main view's existing manual-only-toggle key choice for
  consistency).
- `i` on a Bundle — Install every member to a chosen target (same target
  picker as the Library view's `i`), applying each member's stored
  Activation as part of placement.
- `d` — delete the selected Bundle (members' underlying Library entries are
  untouched — only the Bundle's grouping/prefs are deleted).
- `B`/`esc` — back to main view.

### Target picker

A small new prompt component (not a full view) offering "Personal" plus one
row per `e.roots.ProjectRoots ∪ e.roots.ClaudeProjectRoots` (deduplicated,
labeled by path) — reuse `renderConfirmOverlay`'s overlay styling
(`confirm.go`) rather than building a second, differently-styled popup.

---

## Part 3 — Install engine

`internal/engine/install.go`:

```go
func (e *Engine) InstallLibraryEntry(entry LibraryEntry, target InstallTarget, activation ActivationState) error
func (e *Engine) InstallBundle(bundleID string, target InstallTarget) error // resolves + installs every member, applying each member's own Activation
```

Dispatch on `entry.Source.Kind`:

- **`LibrarySourceLocalPath`**: straightforward directory copy from
  `entry.Source.LocalPath` to the target's skills directory (`~/.claude/
  skills/<name>` or `<repo>/.claude/skills/<name>` for a Claude Code entry;
  the Codex equivalents for a Codex-tool entry). Lowest-risk source type —
  no external process, no unresolved research from Part 0.
- **`LibrarySourceGit`**: `git clone` (shallow, at `GitRef` if set) into a
  scratch temp directory, then copy `GitSubPath` (or the repo root if unset)
  into the target, same as the local-path case.
- **`LibrarySourceSkillsSh`**: shells out to the CLI verified in Part 0,
  with cwd/flags set per that verification's findings. Do not implement this
  branch until Part 0 confirms the targeting mechanism — a wrong guess here
  risks writing into the wrong directory silently.
- **`LibrarySourceMarketplace`**: for a Plugin entry, "install" means
  ensuring the plugin is installed at all (shell out to `claude plugin
  install`, per Part 0) *and* enabled at the chosen target's scope
  (`enabledPlugins` map — user `settings.json` for Personal, project
  `settings.json` for a repo target, per `skill-mechanisms.md` line 42).
  Also gated on Part 0.

After placement, if `activation == ActivationManualOnly`, call the existing
`SetManualOnly(skill, true)` (reusing `manual_only.go` — do not reimplement
frontmatter/`agents/openai.yaml` editing here) constructed from the freshly
placed skill's location. `ActivationAuto` needs no follow-up call (Auto is
the default state for anything just placed).

Conflict handling: confirm-and-overwrite (per the in-session decision),
reusing the existing `pendingConfirm`/`executePending` pattern in `model.go`
— show what's about to be replaced, wait for `y`. If a name collision exists
at the target, the TUI raises the confirmation before `Install*` is called;
the engine methods themselves can assume the caller already resolved that
(matching how `Uninstall`/`Restore` don't re-confirm internally either).

---

## Testing

New `internal/engine/library_test.go`, `bundle_test.go`, `install_test.go`
following the existing `t.TempDir()` fixture pattern (no mocks):

- Library CRUD round-trip (add/list/remove) for each `LibrarySourceKind`.
- Bundle CRUD, including `SetBundleMemberActivation` changing an existing
  member without disturbing others.
- `RemoveLibraryEntry` behavior against a Bundle-referencing entry (whichever
  of refuse/cascade is chosen in Part 1 — assert that specific behavior).
- `InstallLibraryEntry` for `LibrarySourceLocalPath` and `LibrarySourceGit`
  (fixture repos, no network) to both `InstallTargetPersonal` and
  `InstallTargetProject` fixtures — assert file contents byte-identical to
  source, and (for `ActivationManualOnly`) that the placed skill's frontmatter
  reflects it via the existing `manual_only.go` mechanism.
- `LibrarySourceSkillsSh`/`LibrarySourceMarketplace` installs: only what Part
  0's findings make testable without live network/plugin-install side
  effects — likely dependency-injecting the shell-out command rather than
  actually invoking `npx`/`claude` in tests, mirroring how no existing test
  in this repo invokes a real external process.

Run `go build ./...`, `go vet ./...`, and `go test ./... -v` — all must pass.

## Verification

Same as phase 2's closing verification: run the built binary interactively
(`go run ./cmd/skillet`) and confirm the full Install loop for at least one
`LibrarySourceLocalPath` entry end-to-end (add a Personal skill to the
Library via `l`, Install it to a throwaway project fixture, confirm the copy
appears and its Activation preference took effect), plus Library/Bundle view
navigation, before considering this phase done. Do not commit, push, or
modify anything outside this repo. Report back: files changed, what Part 0's
research actually found (especially if it changes Part 3's dispatch), the
verification command output, and any judgment calls made against this spec's
explicitly-flagged open questions (the `RemoveLibraryEntry` dangling-Bundle-
reference behavior, and whether `LibraryEntry.Kind` is redundant with
`Source.Kind`).
