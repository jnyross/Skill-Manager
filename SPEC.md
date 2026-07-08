# SPEC: Skillet walking skeleton + Plugin/Codex inventory + Personal Archive (issues #2-#5)

This spec covers four GitHub issues in this repo, implemented together as one
Go project because they share a single engine and a single test seam. Read
`CONTEXT.md` (vocabulary), `docs/adr/0001-go-bubbletea-tui.md` (stack), and
`docs/research/skill-mechanisms.md` (on-disk mechanisms) first — they are the
source of truth and this spec quotes/grounds against them. Full issue text is
in GitHub Issues #2, #3, #4, #5 (`gh issue view <n>`) if anything here is
ambiguous, but this spec should be sufficient on its own.

Do not implement Suppress, Manual-only toggling, plugin uninstall, or any
Codex mutation (issues #6-#11). This pass is inventory (Personal, Plugin,
Codex skills + Codex custom prompts) plus the Archive lifecycle for Personal
skills only (Uninstall/Restore/Purge/archive view).

## Go module

Run at repo root:

```
go mod init skillet
```

Module name is `skillet` (no domain prefix — this is an application, not an
importable library). Add dependencies as needed (see below). Go version:
whatever `go version` reports locally (1.25.x) — set `go 1.23` or similar in
go.mod, don't pin to the exact patch version.

Add Go build artifacts to `.gitignore` (it's currently language-neutral, per
issue #2's acceptance criteria: "Go rules added to the currently
language-neutral `.gitignore`"). Add: `/skillet`, `*.exe`, `/dist/`.

Suggested dependencies (all fine to add via `go get`):
- `github.com/charmbracelet/bubbletea` — TUI runtime (per ADR 0001)
- `gopkg.in/yaml.v3` — SKILL.md / openai.yaml frontmatter parsing
- `github.com/BurntSushi/toml` — Codex `config.toml` parsing

Do not add `github.com/charmbracelet/bubbles` or `lipgloss` unless you judge
them clearly necessary — a hand-rolled `tea.Model` with a plain slice + cursor
index is sufficient for this v1 list UI and avoids unfamiliar-library risk.

## Package layout

```
go.mod
cmd/skillet/main.go        — entrypoint: resolve real Roots from $HOME, run TUI
internal/engine/
  types.go                  — Source, ActivationState, Kind, Skill, Notice, Inventory, PluginInfo, ArchiveEntry
  engine.go                  — Roots struct, Engine struct, New(Roots) *Engine
  frontmatter.go               — shared "---\nYAML\n---\nbody" splitter + YAML unmarshal helper
  personal.go                   — scanPersonal(claudeHome) ([]Skill, []Notice)
  plugin.go                      — scanPlugins(claudeHome) ([]Skill, []Notice)
  codex.go                        — scanCodex(codexHome, agentsHome) ([]Skill, []Notice)
  inventory.go                     — Engine.Inventory() Inventory (aggregates all three scans)
  archive.go                        — Engine.Uninstall/Restore/Purge/ListArchive (Personal only)
internal/tui/
  model.go                           — tea.Model: list view + archive view + confirmation state
README.md                             — update per issue #2/#11 expectations (brief is fine for now)
```

Every scan function and archive operation takes its root(s) as explicit
parameters (via the `Engine` struct's stored `Roots`, set once at
construction) — **never** call `os.UserHomeDir()` or read `$HOME` anywhere
under `internal/engine`. Only `cmd/skillet/main.go` resolves the real
filesystem defaults. This is the project's single testing seam (PRD
"Implementation Decisions" and issue #2 acceptance criteria are explicit
about this).

## Types (`internal/engine/types.go`)

```go
package engine

type Source string

const (
    SourcePersonal Source = "Personal"
    SourcePlugin   Source = "Plugin"
    SourceCodex    Source = "Codex"
)

type ActivationState string

const (
    ActivationAuto       ActivationState = "Auto"
    ActivationManualOnly ActivationState = "Manual-only"
    ActivationDisabled   ActivationState = "Disabled" // Codex native config.toml disable only
)

type Kind string

const (
    KindSkill  Kind = "skill"
    KindPrompt Kind = "prompt" // Codex custom prompts only
)

// PluginInfo is set only when Source == SourcePlugin.
type PluginInfo struct {
    Plugin      string // e.g. "last30days"
    Marketplace string // e.g. "last30days-skill"
    SkillCount  int    // N in "one of N in plugin-x"
}

type Skill struct {
    Name        string
    Description string
    Source      Source
    Kind        Kind
    Location    string // absolute path to the skill's folder (or the .md file, for prompts)
    Activation  ActivationState
    Plugin      *PluginInfo // non-nil only for Source == SourcePlugin
}

type Notice struct {
    Message string
}

type Inventory struct {
    Skills  []Skill
    Notices []Notice
}

// ArchiveEntry describes one archived item and how to restore it.
type ArchiveEntry struct {
    ID               string
    Name             string
    Source           Source
    OriginalLocation string
    ArchivedAt       time.Time
    IsSymlink        bool
    SymlinkTarget    string // set only if IsSymlink
}
```

Sort order for `Inventory.Skills`: stable, grouped by `Source` in the order
Personal, Plugin, Codex, then alphabetically by `Name` within each group. The
TUI relies on this order for its grouped display rather than re-sorting
itself.

## `internal/engine/engine.go`

```go
package engine

type Roots struct {
    ClaudeHome string // e.g. ~/.claude
    CodexHome  string // e.g. ~/.codex
    AgentsHome string // e.g. ~/.agents (shared, Codex also scans this)
    DataDir    string // Skillet's own data dir, e.g. ~/.skillet
}

type Engine struct {
    roots Roots
}

func New(roots Roots) *Engine {
    return &Engine{roots: roots}
}
```

`cmd/skillet/main.go` builds the real `Roots` from `os.UserHomeDir()`:
`ClaudeHome = home/.claude`, `CodexHome = home/.codex`, `AgentsHome =
home/.agents`, `DataDir = home/.skillet`.

## Frontmatter parsing (`internal/engine/frontmatter.go`)

SKILL.md and Codex prompt `.md` files share the same shape:

```
---
<yaml>
---
<markdown body, ignored>
```

Write one helper, e.g. `parseFrontmatter(path string, out any) error` that
reads the file, requires it to start with a `---` line, finds the closing
`---` line, and `yaml.Unmarshal`s the YAML block into `out`. Return a
descriptive error (missing file, missing frontmatter delimiters, YAML parse
error) — callers turn errors into `Notice`s and skip the item; they must
never panic or abort the whole scan.

Verified-locally frontmatter shape for both Personal and Codex SKILL.md:

```yaml
---
name: onepassword-login
description: Use when John asks Codex to log in to a chosen website or app, ...
---
```

Personal skills only ever use plain `name`/`description` plus, per
`docs/research/skill-mechanisms.md`, an optional `disable-model-invocation:
true` boolean (not present in current real skills, but must be handled). If
`disable-model-invocation` is `true`, `Activation = ActivationManualOnly`,
else `ActivationAuto`.

Plugin skill SKILL.md frontmatter is the same shape and may have extra
fields Skillet doesn't use yet (`version`, `argument-hint`,
`allowed-tools`, `homepage`, `repository`, `author`, `license` were observed
locally) — unmarshal into a struct with just `Name`, `Description`, and
`DisableModelInvocation *bool` (yaml tag `disable-model-invocation`); extra
YAML keys are ignored automatically by `yaml.Unmarshal` into a struct.

## Personal skill scan (`internal/engine/personal.go`)

Source: `<ClaudeHome>/skills/<folder>/SKILL.md`.

- `os.ReadDir(claudeHome + "/skills")`. If the directory doesn't exist,
  return zero skills plus one `Notice{"Personal skills directory not found: <path>"}`
  — this is graceful degradation per issue #2 acceptance criteria, not an
  error.
- For each entry (skip anything starting with `.`, e.g. `.DS_Store`): if it's
  a directory (use `os.Stat`, which follows symlinks, so a symlinked skill
  folder like `~/.claude/skills/codex-computer-use ->
  /Users/.../claude-shared-config/skills/codex-computer-use` is treated as a
  directory), look for `SKILL.md` inside it.
- If `SKILL.md` is missing or fails to parse (missing `name` or
  `description`), skip the entry and add a `Notice{"Skipped <folder>: <reason>"}`.
  Do not let one bad skill stop the scan.
- Emit a `Skill{Source: SourcePersonal, Kind: KindSkill, Location:
  <folder path>, ...}`.

## Plugin skill scan (`internal/engine/plugin.go`)

Two files, both under `<ClaudeHome>/plugins/`:

`installed_plugins.json` (verified shape, real file on this machine):

```json
{
  "version": 2,
  "plugins": {
    "<plugin-name>@<marketplace-name>": [
      {
        "scope": "user",
        "installPath": "/abs/path/to/plugins/cache/<marketplace>/<plugin>/<version>",
        "version": "<version>",
        "installedAt": "2026-06-09T23:28:34.988Z",
        "lastUpdated": "2026-06-20T15:23:08.460Z",
        "gitCommitSha": "..."
      }
    ]
  }
}
```

The map key is `"<plugin>@<marketplace>"` — split on the last `@` to get
`Plugin` and `Marketplace` for `PluginInfo`. The value is an array (multiple
install scopes are possible); v1 is user-level only, so filter to entries
where `scope == "user"`. `installPath` is already the plugin's cache
directory — do not reconstruct it from marketplace/plugin/version yourself.

You do **not** need `known_marketplaces.json` for this — the marketplace
name is already in the map key.

For each user-scoped plugin install:
- If `installPath` doesn't exist on disk, skip the whole plugin with a
  `Notice{"Plugin <plugin>@<marketplace>: install path not found: <path>"}`
  (graceful degradation — this is explicitly required by issue #3's
  acceptance criteria: "a manifest entry whose cache directory is missing").
- Otherwise, recursively walk `installPath + "/skills"` (may not exist —
  treat as zero skills, not an error) with `filepath.WalkDir`, finding every
  `SKILL.md` at **any depth** (verified locally:
  `.../mattpocock-skills/1445797da5ee/skills/engineering/implement/SKILL.md`
  — skills nest under category subdirectories). Each `SKILL.md` found is one
  skill folder (its containing directory).
- Parse each SKILL.md the same way as Personal (missing/malformed → skip
  with a `Notice`, don't abort the plugin).
- `PluginInfo.SkillCount` = the total count of valid skills found for that
  plugin (i.e. `N` in "one of N in plugin-x" is the count *after* skipping
  malformed ones — the number of skills Skillet will actually list).
- Plugin skills currently have no manual-only/suppress mechanism wired up in
  this pass (that's issues #7/#9) — always set `Activation: ActivationAuto`.

`Location` for a plugin skill is its full path inside the cache dir.

## Codex scan (`internal/engine/codex.go`)

Two skill-scan locations, both user-level, verified locally:
- `<AgentsHome>/skills` (e.g. `~/.agents/skills`) — **higher priority**
- `<CodexHome>/skills` (e.g. `~/.codex/skills`) — lower priority; also
  contains a `.system/` subdirectory of Codex's own bundled skills
  (`skill-creator`, `plugin-creator`, etc.) — **exclude any entry whose
  folder name starts with `.`** (this naturally excludes `.system`, which is
  out of scope: "Admin/system-level Codex skill locations" per the PRD's Out
  of Scope list).

Per `docs/research/skill-mechanisms.md` ("Multi-scope discovery, scanned in
priority order"), on a name collision between the two locations, the
higher-priority one (`AgentsHome`) wins and the lower-priority one does not
appear in the inventory at all (shadowed, matching what Codex itself would
actually invoke). Merge algorithm: scan `AgentsHome` first into a map keyed
by skill name, then scan `CodexHome`, only adding entries whose name isn't
already present.

Skill folder scan mechanics (both locations) are otherwise identical to
Personal: subdirectory containing `SKILL.md`, same frontmatter shape
(`name`, `description`). Missing/malformed → skip with `Notice`. Missing
root directory itself → `Notice`, zero skills, not an error.

**Manual-only for Codex skills**: optional file
`<skill-folder>/agents/openai.yaml`. Verified shapes on this machine (the
`interface:` wrapper is sometimes present, sometimes absent, and there may be
no `policy:` key at all — treat missing file, missing `policy` key, or
missing `allow_implicit_invocation` key all as `ActivationAuto`):

```yaml
policy:
  allow_implicit_invocation: false
```

Parse just enough to read `policy.allow_implicit_invocation` (a struct with
`Policy *struct{ AllowImplicitInvocation *bool }` is enough — ignore
`interface:` and everything else in the file). If `allow_implicit_invocation
== false`, `Activation = ActivationManualOnly`.

**Native disable via config.toml** (`<CodexHome>/config.toml`) overrides
whatever the above produced, per issue #4 acceptance criteria ("Skills
natively disabled in Codex's config show that state in the list"). Verified
real shape (array of tables, mixed keying):

```toml
[[skills.config]]
name = "render:render-debug"
enabled = false

[[skills.config]]
path = "/Users/johnross/.codex/skills/world-class-skill-creator/SKILL.md"
enabled = false
```

Define:

```go
type codexConfig struct {
    Skills struct {
        Config []struct {
            Path    string `toml:"path"`
            Name    string `toml:"name"`
            Enabled *bool  `toml:"enabled"`
        } `toml:"config"`
    } `toml:"skills"`
}
```

If `<CodexHome>/config.toml` doesn't exist, treat as zero entries (not an
error — config.toml is optional; many installs won't have this section).
For each scanned Codex skill, look for a `skills.config` entry that matches
either `Path == <skill's SKILL.md absolute path>` or `Name == <skill name>`;
if found and `Enabled != nil && *Enabled == false`, set `Activation =
ActivationDisabled` (this takes priority over the openai.yaml
manual-only check — Disabled is a stronger state than Manual-only).

**Custom prompts**: `<CodexHome>/prompts/*.md`. Verified shape:

```yaml
---
description: Generate and critically evaluate grounded improvement ideas...
argument-hint: "[feature, focus area, or constraint]"
---
```

Frontmatter has `description` (required) and optional `argument-hint`
(ignore it — not modeled in `Skill` v1). `Name` = filename without the
`.md` extension. `Kind = KindPrompt`, `Source = SourceCodex`. Custom prompts
have no toggle mechanism (per the research doc: "strictly user-invoked, no
per-prompt enable/disable found") — set `Activation: ActivationManualOnly`
always (accurately describes that they only ever run via explicit
invocation; do not treat this as a real toggle in the TUI — Kind == KindPrompt
means "don't offer an activation toggle here" for future issues, not
relevant to this pass since no toggle actions exist yet).
Missing/malformed prompt file → skip with `Notice`, same pattern as skills.
Missing `<CodexHome>/prompts` directory → zero prompts, not an error.

## `internal/engine/inventory.go`

```go
func (e *Engine) Inventory() Inventory
```

Calls all three scanners, concatenates `Skills` and `Notices`, sorts
`Skills` per the ordering rule in the Types section above, and returns the
result. Must never return an error and must never write to disk — this
method is pure read.

## Archive lifecycle (`internal/engine/archive.go`) — Personal skills only

Archive layout under `<DataDir>` (Skillet's own data directory, distinct
from any scanned source root):

```
<DataDir>/archive/<id>/provenance.json
<DataDir>/archive/<id>/<original-folder-name>   (the moved skill folder or symlink)
```

`<id>` must be unique and filesystem-safe — e.g.
`fmt.Sprintf("%d-%s", time.Now().UnixNano(), sanitize(folderName))`. Exact
scheme is your call as long as it's unique per archive operation and
collision-free within a test run.

`provenance.json` (matches `ArchiveEntry` minus `ID`, which is the directory
name):

```json
{
  "name": "codex-computer-use",
  "source": "Personal",
  "originalLocation": "/Users/johnross/.claude/skills/codex-computer-use",
  "archivedAt": "2026-07-08T12:00:00Z",
  "isSymlink": true,
  "symlinkTarget": "/Users/johnross/Projects/claude-shared-config/skills/codex-computer-use"
}
```

### `Engine.Uninstall(location string) (ArchiveEntry, error)`

`location` is a Personal skill's `Skill.Location` (the folder path under
`<ClaudeHome>/skills/`) as returned by `Inventory()`.

1. `os.Lstat(location)` to detect whether it's a symlink (`Mode() &
   os.ModeSymlink != 0`) without following it.
2. Generate `<id>`, create `<DataDir>/archive/<id>/`.
3. If it's a symlink: `os.Readlink` to capture the target, then move the
   symlink itself into the archive dir (`os.Rename` first; if that fails
   with a cross-device error, fall back to `os.Symlink(target, newPath)` +
   `os.Remove(oldPath)`). Record `IsSymlink: true, SymlinkTarget: target`.
4. If it's a real directory: `os.Rename(location, archiveDir + "/" +
   folderName)` (same-filesystem move preserves the tree exactly — this
   satisfies the "identical file tree" round-trip requirement without
   needing to copy).
5. Write `provenance.json`.
6. Return the resulting `ArchiveEntry`.

No confirmation logic here — confirmation is a TUI-level concern (the TUI
must prompt before calling this; the engine itself always executes
immediately when called). Engine-level tests call this directly.

### `Engine.ListArchive() ([]ArchiveEntry, error)`

Read `<DataDir>/archive/*/provenance.json`, unmarshal each, set `ID` from
the directory name, return sorted by `ArchivedAt` descending (most recent
first). Missing `<DataDir>/archive` directory → empty slice, not an error.

### `Engine.Restore(id string) error`

1. Read `<DataDir>/archive/<id>/provenance.json`.
2. Ensure the parent directory of `OriginalLocation` exists (it should,
   since that's `<ClaudeHome>/skills`, but don't assume — `os.MkdirAll` is
   cheap insurance).
3. If `IsSymlink`, recreate the symlink at `OriginalLocation` pointing at
   `SymlinkTarget`, then remove the archived symlink.
4. Else, `os.Rename` the archived folder back to `OriginalLocation`.
5. Remove `<DataDir>/archive/<id>/` (including the now-empty provenance
   file).
6. If `OriginalLocation` already has something at it (e.g. a new skill was
   installed at the same path after archiving), return an error rather than
   overwriting — do not silently clobber.

### `Engine.Purge(id string) error`

`os.RemoveAll(<DataDir>/archive/<id>)`. That's it — this is the only method
in the whole engine allowed to permanently delete anything. No confirmation
logic here either (TUI-level, same as Uninstall).

## `internal/tui/model.go`

A single hand-rolled `tea.Model`. Keep it simple — v1 gets no automated
tests (per the PRD's Testing Decisions: "The TUI gets no automated tests in
v1; it is verified by running it against a real install"), so favor
straightforward, readable state over cleverness.

State needed:
- Current `Inventory` (re-fetched from the engine after any mutating
  action)
- Current view: `main` or `archive`
- Cursor index into whichever list is showing
- Pending confirmation, if any (what action, on what item) — e.g. a
  `*pendingConfirm` struct with a description string and a callback/enum of
  which action to run on 'y'
- Any transient status/error message from the last action

Main view:
- List every skill from `Inventory.Skills`, grouped visually by `Source`
  (a header line per group is fine — "Personal", "Plugin", "Codex"). Each
  line shows `Name`, truncated `Description`, `Activation`, and for
  `Source == SourcePlugin`, `"one of N in plugin-x"` using `PluginInfo`.
  For `Kind == KindPrompt`, label it distinctly (e.g. prefix "[prompt]").
- Show `Inventory.Notices` in a footer/status area if non-empty (issue #2's
  "visible notice" requirement).
- Keys: `↑/k` `↓/j` move cursor, `u` uninstall the selected item **only if
  it's `Source == SourcePersonal`** (no-op / status message otherwise —
  Plugin/Codex mutation isn't in scope this pass), `a` switch to archive
  view, `q`/`ctrl+c` quit.
- `u` on a valid target sets `pendingConfirm`; the next keypress must be `y`
  (execute `Uninstall`, refresh inventory, clear confirm) or anything else
  (cancel, clear confirm). Do not execute on any key other than `y`.

Archive view:
- List `ListArchive()` results: name, original source, original location,
  archived-at.
- Keys: `↑/k` `↓/j` move, `r` restore selected (confirm same as above,
  `y`/cancel), `p` purge selected (confirm same as above), `a`/`esc` back to
  main view (re-fetching inventory), `q`/`ctrl+c` quit.

`cmd/skillet/main.go`:

```go
func main() {
    home, err := os.UserHomeDir()
    if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
    e := engine.New(engine.Roots{
        ClaudeHome: filepath.Join(home, ".claude"),
        CodexHome:  filepath.Join(home, ".codex"),
        AgentsHome: filepath.Join(home, ".agents"),
        DataDir:    filepath.Join(home, ".skillet"),
    })
    p := tea.NewProgram(tui.NewModel(e), tea.WithAltScreen())
    if _, err := p.Run(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

## Testing

Go standard `testing` package, `t.TempDir()` fixtures — no mocks. Build a
fake `<tmp>/claude`, `<tmp>/codex`, `<tmp>/agents`, `<tmp>/data` tree per
test, construct `engine.New(engine.Roots{...})` pointed at it, call engine
methods, and assert only on:
- The returned `Inventory` / `ArchiveEntry` / `[]ArchiveEntry` values
  (external behavior).
- The resulting file tree (`filepath.Walk` + compare, or targeted
  `os.Stat`/`os.ReadFile` checks) for mutating operations.

Never assert on unexported engine internals.

Required coverage (mirrors each issue's acceptance criteria — write these as
real `_test.go` files, one per scanner/concern is fine):

**Personal (issue #2)**
- A fixture with 2-3 valid Personal skills → `Inventory()` lists them with
  correct name/description/Source/Location.
- A missing Personal skills root directory → zero skills, one `Notice`, no
  panic.
- A malformed `SKILL.md` (no frontmatter, or missing `name`) alongside valid
  ones → the malformed one is skipped with a `Notice`; the valid ones still
  appear.

**Plugin (issue #3)**
- A fixture `installed_plugins.json` + cache tree with nested skills (at
  least one nested 2+ levels deep, e.g. `skills/engineering/foo/SKILL.md`) →
  all discovered, `PluginInfo.SkillCount` correct, `"one of N in plugin-x"`
  data correct.
- A manifest entry whose `installPath` doesn't exist on disk → skipped with
  a `Notice`, other plugins still scan fine.

**Codex (issue #4)**
- Skills in `AgentsHome/skills` and `CodexHome/skills` both appear with
  `Source = Codex`.
- Custom prompts in `CodexHome/prompts` appear alongside skills.
- A `config.toml` `[[skills.config]]` entry with `enabled = false` matching
  a scanned skill's path → that skill shows `ActivationDisabled`.
- Same skill name present in both `AgentsHome/skills` and `CodexHome/skills`
  → only the `AgentsHome` one appears in the inventory (priority/merge
  test).

**Archive (issue #5)**
- Uninstall a Personal skill → it's gone from `Inventory()`, present in
  `ListArchive()` with correct provenance, and the fixture's `<claude>/skills/<name>`
  path no longer exists while `<data>/archive/<id>/<name>` does.
- Uninstall a **symlinked** Personal skill folder, then Restore it → the
  restored entry at the original location is again a symlink pointing at
  the original target (not a copy of the target's contents).
- Restore round-trip: capture a full byte-for-byte snapshot of the fixture
  tree before Uninstall, Uninstall, then Restore, then assert the tree is
  identical to the snapshot.
- Purge removes the archive entry permanently; a subsequent `ListArchive()`
  no longer includes it.
- A session that only calls read-only methods (`Inventory()`,
  `ListArchive()`) leaves the entire fixture tree byte-identical (snapshot
  before/after, assert equal) — this is issue #5's "no explicit action
  leaves the fixture tree byte-identical" criterion.

Run `go vet ./...` and `go test ./...` and make sure both pass with no
warnings before considering this done.

## Explicitly out of scope for this pass

- Suppress, Manual-only *toggling* (writing frontmatter/openai.yaml/config.toml
  back out), plugin uninstall, Codex archive/restore, project skills, usage
  data, seeding/shortlist. These are later issues (#6-#11). If you find
  yourself writing a mechanism to *change* activation state or archive
  anything other than Personal skills, stop — that's out of scope here.
- Don't build a `Suppress` self-healing loop. Don't add a
  `disableModelInvocation`/`allow_implicit_invocation` *writer*. Read-only
  for Plugin and Codex activation state in this pass; only Personal gets a
  mutating operation (Archive), and only Uninstall/Restore/Purge, not
  Manual-only.

## Verification

```
go build ./...
go vet ./...
go test ./... -v
```

All must pass. Do not commit, push, or modify anything outside this repo.
Report back: files changed, a summary of what each scanner/operation does,
the verification command output, and anything you were uncertain about or
had to make a judgment call on.
