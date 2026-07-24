# Product Improvement Implementation Plan — 2026-07-24

Implements the five improvements plus the small-fix batch from the 2026-07-24
product review. Six work packages (WP1–WP6) across three waves. Each WP is
executed by one **Opus 5 subagent at medium reasoning effort** in an **isolated
git worktree**, with verification independent of the implementer.

## Orchestration contract

- **Parent:** Fable — frames each WP packet, sets acceptance criteria, merges,
  performs final acceptance. Fable does not implement.
- **Implementers:** one Opus 5 (medium effort) general-purpose subagent per WP.
  Dispatch each wave as a Workflow with
  `agent(packet, {model: 'opus', effort: 'medium', isolation: 'worktree'})`,
  or equivalently one Agent-tool call per WP with `model: "opus"`.
- **Each packet is self-contained:** the WP section below plus the repo's
  CLAUDE.md conventions. Agents assume no conversation context.
- **Deliverable per WP:** a branch in its worktree with focused commits, full
  `go test ./...` + `go vet ./...` output, the changed-file list, and a short
  self-review noting known limitations. Raw file dumps are not evidence.
- **Independent verification per WP (all three, in order):**
  1. Deterministic: `go test ./...`, `go vet ./...`, formatting via a
     go/format-based checker binary (the sandbox SIGKILLs `gofmt` directly),
     `CGO_ENABLED=0 go build ./cmd/skillet`.
  2. Cross-model review: `codex-review` (GPT-5.6 Sol) over the WP branch diff
     against the WP's acceptance criteria.
  3. Fable acceptance: diff inspection against criteria before merge.
- **Escalation:** one evidence-based corrective retry per WP. A second failure
  of the same verifier, scope growth, or architecture ambiguity returns the WP
  to Fable. Never silently downgrade the model.
- **Merges:** within a wave, merge in the order listed; later WPs in the same
  wave rebase before verification re-runs. Worktree commits are part of
  execution; **merging to `main` and any push waits for John's explicit
  authorization.**
- **Final gate (after Wave 3):** the project's default acceptance path —
  `go test ./...`, `go vet ./...`, `CGO_ENABLED=0` build to a temp candidate,
  install to `~/go/bin/skillet`, byte-identical check, real start-and-quit
  smoke test. Preserve `~/.skillet` and managed skills throughout.

## Dependency graph

```
Wave 1:  WP1 (engine safety)        WP2 (TUI search & discoverability)
              │                          │
Wave 2:  WP4 (scan perf, rebases    WP3 (scriptable CLI)
              on WP1)                    │
              │                          │
Wave 3:  WP5 (token cost — needs    WP6 (responsiveness & first-run —
         WP3 JSON + WP4 scan)       needs WP1 timeouts + WP2 model.go)
```

WP1/WP4 both touch `internal/engine/suppress.go`; WP2/WP6 both touch
`internal/tui/model.go` — hence the sequencing. WPs within a wave touch
disjoint files and run in parallel.

---

## WP1 — Engine write-safety and rollback (Wave 1)

**Objective:** every Skillet write to files it does not own is atomic, and no
multi-step mutation can strand state with no recovery path. This protects the
product's core "reversible and safe" promise.

**Scope:** `internal/engine/` only. No TUI changes.

**Required changes:**

1. Promote the temp-file+rename writer at `internal/engine/archive.go:379`
   (`writeFileAtomic`) into a shared engine helper preserving file mode, and
   use it for **every** config write: `suppress.go:432`
   (`writeFilePreservingMode`), `codex_config.go:68` (`writeCodexConfig`),
   the `settings.json` writers (`install.go:342`, `plugin_uninstall.go:238`),
   and `installed_plugins.json` (`plugin_uninstall.go:177`).
2. **Archive ordering:** eliminate the window where a skill is moved out of
   its source but `provenance.json` doesn't exist (`archive.go:100-109`).
   Either write provenance to a staging path before the move, or roll the move
   back on any post-move failure. Acceptance: a fault-injection test proves no
   reachable state where a skill is absent from both the tool and
   `ListArchive`.
3. **Restore repair:** a failure in `reinstateCodexConfigEntries` after the
   rename-back (`archive.go:171-185`) must not permanently wedge the entry
   ("restore destination already exists"). Detect the half-restored state and
   complete or cleanly report it; a second Restore of the same ID must succeed
   or explain exactly what to do.
4. **UninstallPlugin ordering** (`plugin_uninstall.go:63`): reorder or add
   rollback so no failure leaves cache files deleted while `settings.json`
   still lists the plugin enabled.
5. **Symlink guard for Codex writes:** `setCodexManualOnly`
   (`manual_only.go:135`) writes `agents/openai.yaml` unguarded; apply the
   same protection as `guardSkillMDPath` (`suppress.go:277`).
6. **Codex suppress ownership:** record which `config.toml` entries Skillet
   wrote (extend the `~/.skillet/suppressed/` record). Un-suppress removes
   only Skillet-authored entries and never deletes a `config.toml` containing
   user-authored content (`codex_suppress.go:108`). Same principle for the
   empty-file deletion in `manual_only.go:170`.
7. **Subprocess timeouts:** switch `execCommandRunner.Run` (`engine.go:55`)
   to `exec.CommandContext` with per-operation timeouts (default 120s for
   installs/clones, 30s for probes) and context plumbed from callers so a
   future UI cancel works (consumed by WP6).
8. **Stale-location revalidation:** before executing a suppress/manual-only
   write, re-stat the captured `Skill.Location`; if it vanished (plugin
   updated between confirm and act), fail with a clear message instead of
   writing into a dead directory.

**Out of scope:** JSONC tolerance for `settings.json` (fail with a clear
notice, as today); inventory caching; any TUI rendering.

**Tests:** a fault-injecting writer seam; crash-simulation tests for archive,
restore, and plugin uninstall; ownership tests for codex un-suppress;
timeout test with a hanging fake runner.

**Acceptance criteria:** all required changes present with tests; no direct
`os.WriteFile` remains for any user-owned config file; `go test ./...` and
`go vet ./...` clean.

---

## WP2 — TUI search, filtering, and discoverability (Wave 1)

**Objective:** finding a skill in a realistic inventory (30+ entries) takes
one or two keystrokes, and the keys that exist are discoverable and match the
documentation.

**Scope:** `internal/tui/`, `README.md`. No engine behavior changes.

**Required changes:**

1. **Real filtering, all four views:** enable Bubbles' built-in fuzzy
   filtering (currently explicitly disabled at `model.go:1289`,
   `library.go:36`, `archive.go:38`, `bundle.go:47`). Extend `FilterValue()`
   to name + description + source + plugin name. Replace the hidden
   first-match `/` jump (`model.go:1072-1085`) with the list filter; show a
   zero-match state.
2. **Help:** the default short help hides `s`, `m`, `x`, `l`, `L`, `B`, `/`
   (`keys.go:130-141`). Make every mode-changing key discoverable — a second
   help line or a rotating hint — and keep `?` for full help.
3. **README/key truth:** fix the three wrong rows in the key table
   (`README.md:73-74`): `enter`/`space` (currently no-op in main view) and
   `pgup`/`pgdown` (page the list, not the detail pane). Either implement the
   documented behavior or correct the docs — prefer docs correction unless
   trivial. Document the filter key.
4. **Confirmation consistency:** README promises every disk change confirms
   (`README.md:105-112`), but library toggle (`model.go:765-821`), bundle
   add-member, remove-member, and activation toggle mutate immediately. Add
   the same one-line `y` confirm to all four.
5. **Modal fixes:** viewport-scroll the member picker (`forms.go:87-99`) and
   install picker (`install_picker.go:42-54`); clamp the confirm overlay
   instead of returning an unclipped box (`confirm.go:25-29`); width-clamp
   bundle rows like the other delegates (`bundle.go:61-77` vs
   `delegate.go:99-101`).
6. **Key consistency:** `ctrl+c` quits from every state except the confirm
   modal (where it cancels); `esc` cancels any overlay; forms get shift+tab
   back-navigation and `esc` confirmation if fields are non-empty
   (`forms.go:37-58`).
7. **Status line:** replace the string-sniffing error heuristic
   (`model.go:1466-1471`) with an explicit status level set at the call site;
   errors persist until the next action rather than clearing on cursor move
   (`model.go:961`).
8. Remove the unreachable "No Install targets available" branch
   (`model.go:500-503`).

**Acceptance criteria:** filtering works in all four views with a test per
view; every disk-mutating key confirms; README table matches behavior
(verified row by row); existing TUI tests still pass.

---

## WP3 — Scriptable CLI with JSON output (Wave 2)

**Objective:** everything the TUI can do is scriptable, so agents and CI can
manage skills. Today the whole surface is `skillet [--version|version|setup]`
(`cmd/skillet/main.go:38-57`).

**Scope:** `cmd/skillet/`, thin additive engine entry points, `README.md`.
No TUI changes. Rebase on Wave 1.

**Required changes:**

1. Subcommands, all reusing the engine directly (no TUI imports):
   - `skillet list [--json] [--source S] [--tool T]` — the full inventory,
     including notices.
   - `skillet show <name> [--json]`
   - `skillet archive <name>`, `skillet restore <id|name>`,
     `skillet purge <id> --yes` (refuses without `--yes`)
   - `skillet suppress|unsuppress <name>`, `skillet manual-only|auto <name>`
   - `skillet library list|add|remove [--json]`,
     `skillet bundle list|install <bundle> --target <personal|path> [--json]`
   - `skillet install <library-entry> --target <personal|path>`
2. **Name resolution:** accept a bare name when unambiguous; on ambiguity,
   exit 1 listing qualified candidates (`source:name`); accept the qualified
   form everywhere.
3. **Exit codes:** 0 success, 1 operation error, 2 usage. All mutations print
   a one-line summary of what changed; `--yes` is required for anything the
   TUI would confirm.
4. **JSON schema:** stable field names derived from `engine` types, including
   activation state, source, tool, and location; documented in
   `docs/agents/` or README. Golden-file tests for `list --json`.
5. `skillet` with no args still launches the TUI; `--help` documents the full
   tree.

**Out of scope:** cost fields (WP5 adds them), setup changes, shell
completion.

**Acceptance criteria:** every listed subcommand works against a fixture home
directory in tests (extend `main_test.go`'s pattern); golden JSON tests;
`--help` accurate; README updated.

---

## WP4 — Scan and setup performance (Wave 2)

**Objective:** each `Inventory()` does the minimum IO, and setup never stalls
silently on large directories.

**Scope:** `internal/engine/`, `internal/catalog/`, `internal/setup/`
(read paths only). Rebase on WP1 (shares `suppress.go`).

**Required changes:**

1. Parse `~/.codex/config.toml` **once** per `Inventory()` and thread the
   result to both consumers (`codex.go:36` and `project.go:13`).
2. Single frontmatter parse per suppressed plugin skill: pass the
   already-parsed data from `scanPluginInstall` (`plugin.go:120`) into
   `applySuppressions` (`suppress.go:139-147`); drop the duplicate
   `EvalSymlinks` pair.
3. Scope the plugin scan (`plugin.go:107`): walk only `skills/*/SKILL.md`
   instead of the full recursive `WalkDir` of every plugin tree.
4. Merge the two near-identical ancestor walks (`project_root.go:13` and
   `:37`) into one pass with one `.git` stat per level.
5. `catalog.RecommendedBundleIDs` (`catalog.go:248`): skip `node_modules`,
   `vendor`, `venv`/`.venv`, `target`, `dist`, `build`, `.next` and cap
   traversal (depth and file budget) — it exists only to spot
   `*.csproj`/`*.sln`/`global.json`.
6. Memoize `readBoundaryWithModes` (`adapter.go:157`) within a single setup
   run — it currently slurps every member file three-plus times
   (`service.go:231`, `adapter.go:92` ×2 tools, `verify.go:183`,
   `verify.go:208`).

**Explicitly deferred (do not build):** inventory mtime caching and fs
watching — event-driven refresh is already correct; revisit only with a
measured startup problem.

**Acceptance criteria:** unit tests proving single-parse behavior (counting
seams where they already exist, targeted tests elsewhere); a before/after
timing note on a realistic fixture (50 skills, one large plugin tree, one
deep project dir); behavior-identical inventory output on the existing test
suite.

---

## WP5 — Token and context-cost accounting (Wave 3)

**Objective:** every skill shows what it costs, because context bloat is the
reason a skills manager exists. Today nothing in the codebase models size or
tokens; the scanner never reads a skill body.

**Scope:** `internal/engine/` (new fields + read pass), `internal/tui/`
(detail, sort, aggregate), `cmd/skillet/` (JSON fields, `cost` command).
Depends on WP3 and WP4.

**Required changes:**

1. **Engine fields** on `Skill` (`types.go:55-70`): `DescriptionTokens`
   (estimate of what auto-activation injects per session), `BodyBytes`,
   `BodyTokens` (full SKILL.md), `FileCount` and `TotalBytes` for the skill
   directory. One shared estimator (`bytes/4`, clearly labeled an estimate),
   unit-tested. Body read capped (2 MB) with a notice past the cap; the walk
   for counts scoped to the skill directory.
2. **TUI:** detail pane adds a Cost section (description cost, invoked cost,
   files/size). Main list gains sort-by-cost toggle (pick an unused key,
   document it in help and README). Main header shows the aggregate:
   per-tool total of auto-activated description tokens (Manual-only and
   Suppressed excluded, stated in the header).
3. **CLI:** cost fields in `list --json` and `show --json`;
   `skillet cost [--json]` prints the aggregate and top-10 by description
   cost.
4. Perf guard: the extra read pass must not regress WP4 — costs computed in
   the same scan pass, one bounded read per skill.

**Acceptance criteria:** estimator unit tests; scan populates fields for all
four sources (fixture test); sort and aggregate tested; JSON golden files
updated; WP4's timing note re-run with no material regression.

---

## WP6 — Responsiveness and first-run experience (Wave 3)

**Objective:** slow operations look alive and are cancellable; a new user's
first minute is welcoming; setup returns you to the TUI.

**Scope:** `internal/tui/`, `cmd/skillet/main.go`, `internal/setup/`
(progress + notices), `internal/engine/` (only consuming WP1's contexts).
Depends on WP1 (timeouts/contexts) and WP2 (model.go merged).

**Required changes:**

1. **Spinner:** render a `bubbles/spinner` while `m.installing` is set (the
   flag exists at `model.go:126` but is never drawn); show per-step text for
   multi-step installs (clone → copy → configure).
2. **Cancel:** `esc` during an install cancels via WP1's context, cleans up
   temp dirs, reports "cancelled". Quitting mid-install no longer orphans
   the child process. Destructive keys (`u`, `p`, `x`) are gated while an
   install runs.
3. **Setup round-trip:** `S` currently quits the TUI permanently
   (`model.go:262-265`, `main.go:84-98`). After the wizard completes or is
   cancelled, relaunch the TUI with a status line summarizing the outcome.
4. **Setup progress:** the sequential blocking clones in
   `resolver.go:129-173` emit one progress line per repository (name +
   step); no silent multi-second stalls.
5. **First-run:** align `personal.go:18-20` and `codex.go:69-70` with the
   plugin/prompt scanners — a merely-absent standard directory is quiet, not
   a "not found" notice. A fresh machine shows the friendly empty state plus
   a one-line pointer at `S` and `L`, with zero error-styled text. Truly
   anomalous states (unreadable dir, dangling plugin path) still notice.
6. Surface WP1's timeout errors in the status line with the operation name
   and elapsed time.

**Acceptance criteria:** TUI test that installing renders a spinner frame and
gates destructive keys; cancel test with a fake slow runner; fresh-home
fixture shows no notices; setup-return flow covered by an integration-style
test at the `main.go` seam; manual smoke test on the real terminal recorded
in the evidence packet.

---

## Estimates and risk

| WP | Size | Main risk | Mitigation |
|---|---|---|---|
| WP1 | L | Behavior change in config writers breaks restore round-trip | Byte-identical restore tests already exist; extend them |
| WP2 | M | Bubbles filter interacts badly with custom delegates | Keep delegate `Height()==1` contract; per-view tests |
| WP3 | M | Name-resolution ambiguity UX | Qualified-name escape hatch; golden tests |
| WP4 | S–M | Scoped plugin walk misses nonstandard layouts | Fixture from a real marketplace plugin tree |
| WP5 | M | Cost pass regresses scan time | Same-pass computation; timing gate |
| WP6 | M | Setup round-trip touches the raw-terminal seam | Keep wizard line-based; only wrap the relaunch |

Rough wall-clock per wave: 30–60 min including reviews; ~2.5–4 h total.
Agent budget: 6 Opus implementers + up to 6 retries; codex-review runs are
subscription-side, not subagents.

## Execution checklist

- [ ] Wave 1: dispatch WP1 + WP2 (parallel, worktrees) → verify → merge WP1 then WP2
- [ ] Wave 2: dispatch WP3 + WP4 (WP4 rebased on WP1) → verify → merge WP4 then WP3
- [ ] Wave 3: dispatch WP5 + WP6 → verify → merge WP5 then WP6
- [ ] Final: full local install acceptance path + smoke test
- [ ] John authorizes merge to `main` / release train
