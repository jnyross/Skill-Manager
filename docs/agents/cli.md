# Skillet's scriptable command surface

Everything the TUI can do, `skillet` can also do non-interactively, so an agent
session (Claude Code or Codex) or a CI job can manage Skills without a
terminal UI. `skillet` with no arguments still launches the TUI.

The vocabulary here is CONTEXT.md's: **Source**, **Tool**, **Skill**,
**Archive**, **Restore**, **Purge**, **Suppress**, **Manual-only**,
**Library**, **Bundle**, **Install**. JSON field names use the same terms.

## Command tree

```
skillet                                              launch the TUI
skillet --help | -h | help                           the full command tree
skillet version | --version                          release identity

skillet list [--json] [--source SOURCE] [--tool TOOL]
skillet show <name> [--json]
skillet cost [--json]

skillet archive <name> --yes
skillet restore <id|name> --yes
skillet purge <id|name> --yes

skillet suppress <name> --yes
skillet unsuppress <name> --yes
skillet manual-only <name>... --yes
skillet manual-only --all [--except NAME[,NAME...]] --yes
skillet auto <name>... --yes

skillet library list [--json]
skillet library add --name NAME <source flags> [--tool TOOL] [--kind KIND] --yes [--json]
skillet library remove <id|name> --yes

skillet bundle list [--json]
skillet bundle install <id|name> --target <personal|PATH> --yes [--json]

skillet install <id|name> --target <personal|PATH> [--activation auto|manual-only] --yes

skillet setup [--path PATH] [--bundles IDS] [--manual-only MEMBERS] [--auto MEMBERS]
              [--yes] [--accept-drift] [--replace-conflicts] [--static]
```

Every subcommand accepts `--help`. Flags may appear before or after the
positional argument.

`library add` takes exactly one install-source descriptor:

- `--local-path PATH`
- `--git-url URL` with optional `--git-ref REF` and `--git-subpath PATH`
- `--skills-sh OWNER/REPO` with optional `--skills-sh-skill NAME`
- `--marketplace NAME --plugin NAME` with optional `--marketplace-source SOURCE`

## Naming and resolution

A bare Skill name works when exactly one Skill has it. When it does not,
Skillet exits 1 and prints the qualified candidates, each of which is itself
valid input:

```
$ skillet show review
"review" matches 2 Skills — re-run with one of:
  Personal:review  (/Users/me/.claude/skills/review)
  Project:review   (/Users/me/repo/.claude/skills/review)
```

The qualified forms are:

- `Source:Name` — for example `Personal:review`. Sources are `Personal`,
  `Plugin`, `Codex`, `Project` (case-insensitive).
- `Source:Tool:Name` — for example `Project:codex:review`. Needed only when
  one Source holds that name under both Tools. Tools are `claude-code` and
  `codex` (`claude` and `claude code` are also accepted).

`restore`, `purge`, `library remove`, `bundle install`, and `install` accept an
id or, when unambiguous, a name; ambiguity lists the ids.

`manual-only` and `auto` accept **any number** of names. Every name — and
every `--except` name — is resolved before anything is written, so one
ambiguous or unknown name aborts the whole command with exit 1 and the message
`Nothing was changed.` rather than half-applying it. Naming the same Skill
twice changes it once.

## The Activation sweep

Getting a machine down to the bare minimum set of Skills that Auto-activate is
one command:

```
$ skillet manual-only --all --except code-review,handoff --yes
```

`--all` selects every Skill that currently Auto-activates **and** whose
Activation Skillet can actually change, ordered most expensive first.
`--except` keeps the named Skills Auto-activating; it applies only to `--all`.
`--all` cannot be combined with Skill names.

Without `--yes`, both commands print exactly what they would change and the
estimated per-session effect, write nothing, and exit 2:

```
$ skillet manual-only --all --except handoff
Would set 3 Skills to Manual-only:
  SOURCE    TOOL         NAME         PER SESSION (EST.)
  Personal  Claude Code  writing      ~31 tokens
  Personal  Claude Code  review       ~24 tokens
  Codex     Codex        refactor     ~18 tokens

Estimated saving: ~73 tokens per session, across 3 Skills (estimate — Skillet sizes files rather than running a tokenizer).
Nothing has been changed yet.
manual-only changes files on disk and needs confirmation: re-run with --yes
```

With `--yes`, every bulk change prints one line per Skill and then a summary:
how many changed, how many were already in that state, how many could not be
changed, and the estimated per-session saving (or, for `auto`, the added cost).
Reasons for the Skills that could not be changed go to stderr, one per Skill.

Two Sources are never changeable this way, and say so rather than being
silently skipped when named explicitly:

- a **Plugin** Skill — its per-Skill control is `suppress`, which leaves the
  plugin installed and intact;
- a **Codex prompt** — it has no Auto-activation to turn off.

`--all` leaves both out of the sweep entirely, so a sweep never manufactures
failures out of Skills it was never able to change.

A bulk change **applies every Skill it can** and then exits 1 if any Skill
failed, so a non-zero exit means "partially applied, and here is what was
not" — never "nothing happened". Every token figure is an estimate.

## Exit codes and confirmation

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Operation error (no such Skill, ambiguous name, a write failed, or a bulk change that applied some Skills but not all) |
| 2 | Usage error (unknown command or flag, missing argument, missing `--yes`) |

Anything the TUI would confirm requires `--yes` on the CLI, and every mutation
prints a one-line summary of exactly what changed:

```
$ skillet archive writing --yes
Archived Personal skill "writing" from /Users/me/.claude/skills/writing (archive id 1784924950240492000-writing; restore with "skillet restore 1784924950240492000-writing --yes")
```

`purge` is the only destructive command; without `--yes` it refuses and exits 2
without touching the Archive.

## JSON schema

Every JSON document is an object with `schemaVersion` (currently `1`).
`schemaVersion` changes only when an existing field is removed, renamed, or
given a new meaning — **new fields are added without bumping it**, so consumers
must ignore fields they do not know.

### `skillet list --json`

```json
{
  "schemaVersion": 1,
  "skills": [
    {
      "name": "lint",
      "qualifiedName": "Plugin:lint",
      "description": "Lint the tree.",
      "source": "Plugin",
      "tool": "Claude Code",
      "kind": "skill",
      "activation": "Auto",
      "location": "/Users/me/.claude/plugins/cache/demo/skills/lint",
      "declaredManualOnlyForClaude": false,
      "plugin": { "plugin": "demo", "marketplace": "market", "skillCount": 1 },
      "cost": {
        "descriptionTokens": 6,
        "bodyBytes": 4820,
        "bodyTokens": 1205,
        "fileCount": 7,
        "totalBytes": 41230
      }
    }
  ],
  "notices": [
    { "message": "Codex skills directory not found: /Users/me/.codex/skills" }
  ],
  "archive": [
    {
      "id": "1784924950240492000-writing",
      "name": "writing",
      "source": "Personal",
      "tool": "Claude Code",
      "kind": "skill",
      "originalLocation": "/Users/me/.claude/skills/writing",
      "archivedAt": "2026-07-24T20:29:10Z"
    }
  ]
}
```

Skill fields:

| Field | Meaning |
|---|---|
| `name` | The Skill's name as its Tool sees it |
| `qualifiedName` | `Source:Name`, accepted as input by every command |
| `description` | The description the model reads when deciding to auto-activate |
| `source` | `Personal`, `Plugin`, `Codex`, or `Project` |
| `tool` | `Claude Code` or `Codex` |
| `kind` | `skill` or `prompt` |
| `activation` | `Auto`, `Manual-only`, `Suppressed`, or `Disabled` |
| `location` | Absolute path to the Skill directory (or prompt file) |
| `declaredManualOnlyForClaude` | A Codex Skill declaring Claude Code's `disable-model-invocation`, which Codex ignores |
| `plugin` | Present only for Plugin Skills: `plugin`, `marketplace`, `skillCount` |
| `cost` | Context cost, **all values estimated** — see below |

`cost` fields:

| Field | Meaning |
|---|---|
| `descriptionTokens` | Estimated tokens the description injects into **every** session while the Skill Auto-activates. Reported for every Skill whatever its `activation`, so a caller can price turning one back on |
| `bodyBytes` / `bodyTokens` | The whole `SKILL.md` (or prompt file): what invoking the Skill costs |
| `fileCount` / `totalBytes` | The Skill's whole directory — references, scripts, assets |

Every number under `cost` is an **estimate**, not a token count. Skillet sizes
files at roughly four bytes per token rather than running a tokenizer, so the
values are reliable for ranking Skills against each other and should not be
quoted as measurements. A Codex prompt is a single file, so its `fileCount` is
`1` and its `totalBytes` equals `bodyBytes`.


`notices` carries every scan notice, and `archive` every visible Archive entry
— that is where `purge` and `restore` ids come from.

`--source` and `--tool` filter `skills`; `notices` and `archive` are not
filtered.

### `skillet show <name> --json`

```json
{ "schemaVersion": 1, "skill": { "...": "the same Skill object as list" } }
```

### `skillet cost --json`

```json
{
  "schemaVersion": 1,
  "estimate": { "method": "bytes/4", "bytesPerToken": 4, "exact": false },
  "perSession": {
    "descriptionTokens": 1640,
    "skills": 15,
    "excludedSkills": 7,
    "byTool": [
      { "tool": "Claude Code", "skills": 12, "descriptionTokens": 1430 },
      { "tool": "Codex", "skills": 3, "descriptionTokens": 210 }
    ]
  },
  "topByDescriptionCost": [ { "...": "the same Skill object as list" } ],
  "notices": []
}
```

`perSession` is the headline: what Auto-activation costs in **every** session,
per Tool. Only Skills with `activation: "Auto"` are counted — Manual-only,
Disabled, and Suppressed Skills are not offered to the model unprompted, so
they cost nothing per session, and `excludedSkills` says how many were left
out. `byTool` always sums to `descriptionTokens`.

`topByDescriptionCost` is the ten most expensive Skills by `descriptionTokens`,
most expensive first, drawn from the whole inventory (including excluded
Skills, since "this Manual-only Skill would be expensive" is worth knowing).
Only those Skills carry a measured `fileCount`/`totalBytes`.

### `skillet library list --json` / `library add --json`

```json
{
  "schemaVersion": 1,
  "entries": [
    {
      "id": "1784924962760388000-helper",
      "name": "helper",
      "tool": "Claude Code",
      "source": { "kind": "local-path", "localPath": "/Users/me/src/helper" },
      "addedAt": "2026-07-24T20:29:22.760405Z"
    }
  ]
}
```

`library add --json` emits the created entry as `{"schemaVersion":1,"entry":{…}}`.
`source.kind` is one of `local-path`, `git`, `skills.sh`, `marketplace`, and the
descriptor fields present depend on the kind (`gitUrl`/`gitRef`/`gitSubPath`,
`skillsShRepo`/`skillsShSkill`, `marketplace`/`pluginName`).

### `skillet bundle list --json`

```json
{
  "schemaVersion": 1,
  "bundles": [
    {
      "id": "1784924962760388000-starter",
      "name": "starter",
      "members": [
        { "libraryEntryId": "1784924962760388000-helper", "activation": "Manual-only" }
      ]
    }
  ]
}
```

### `skillet bundle install --json`

```json
{
  "schemaVersion": 1,
  "bundle": { "id": "…", "name": "starter", "members": [] },
  "target": { "kind": "project", "repoRoot": "/Users/me/repo" },
  "installed": 1
}
```

## Recipes

```bash
# Everything the model can auto-activate today, by Tool.
skillet list --json | jq -r '.skills[] | select(.activation == "Auto") | "\(.tool)\t\(.qualifiedName)"'

# Make every Personal Skill Manual-only.
skillet list --json --source Personal |
  jq -r '.skills[].qualifiedName' |
  xargs -n1 -I{} skillet manual-only {} --yes

# What Auto-activation costs in every session, per Tool (estimated).
skillet cost --json | jq -r '.perSession.byTool[] | "\(.tool)\t~\(.descriptionTokens) tokens"'

# The five Auto Skills with the most expensive descriptions.
skillet list --json |
  jq -r '[.skills[] | select(.activation == "Auto")] | sort_by(-.cost.descriptionTokens) | .[:5][] | "\(.cost.descriptionTokens)\t\(.qualifiedName)"'

# Fail a CI job when scanning raised any notice.
test "$(skillet list --json | jq '.notices | length')" -eq 0

# Archive a Skill, then put it back.
skillet archive writing --yes
skillet restore "$(skillet list --json | jq -r '.archive[0].id')" --yes
```

## Notes and limits

- Project Sources are discovered from the current working directory, exactly as
  in the TUI, so `list` inside a repository shows that repository's Skills.
- `--target PATH` installs into that repository (`.claude/skills` or
  `.agents/skills` depending on the entry's Tool); `--target personal` installs
  at the user level. An existing Skill of the same name at the target is
  replaced.
- Every `cost` number is an estimate from file size, never a tokenizer result.
  `list --json` and `cost --json` measure each reported Skill's directory;
  `show --json` measures only the Skill it prints.
- `suppress` applies to Plugin and Codex Skills; `manual-only`/`auto` apply to
  Personal, Project, and Codex Skills. Asking for one where the Source has no
  such mechanism is an operation error (exit 1) with an explicit message.
- `archive` covers Personal, Codex, and Project Skills and Codex prompts.
  A Plugin Skill cannot be archived alone — Suppress it instead.
