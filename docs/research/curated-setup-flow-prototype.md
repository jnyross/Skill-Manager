# Curated setup flow prototype

Prototyped 2026-07-15 for [issue #49](https://github.com/jnyross/Skill-Manager/issues/49).

## Question

Can one guided setup state model make new and existing workspaces, preflight
conflicts, missing Tool executables, rollback, and unavoidable partial external
changes understandable before the production TUI is designed?

## Prototype

A throwaway line-driven Go terminal prototype exposed workspace choice,
unmanaged conflicts, missing Tool executables, an external side-effect toggle,
preflight, successful apply, failed apply, and the resulting Workspace receipt.
It was exercised inside the existing Go module because the local Homebrew Go
toolchain could not resolve its standard library for a standalone file outside
the module. The prototype was deleted after these decisions were captured.

## Scenarios exercised

1. **New folder, both Tools available** — preflight produced one reviewable
   plan; apply ended `Verified` and recorded managed instruction, skill, catalog,
   and Tool-outcome identities.
2. **Existing folder, Tool executable absent** — preflight remained actionable;
   apply ended `Configured-unverified` rather than claiming live readiness.
3. **Existing folder, unmanaged path collision** — preflight ended `Blocked`
   before any write and named the need for an explicit conflict decision.
4. **Failure before an external side effect** — staged Managed files could be
   rolled back completely; the operation remained `Blocked`, not partially
   successful.
5. **Failure after an external Tool changed state** — Managed files rolled back,
   but the honest outcome was `Partial`; the Workspace receipt retained the
   observed external change and required repair action.

## Decision

Use one linear guided flow:

1. choose or create the project folder through a native picker when available,
   with a guarded TUI path fallback;
2. choose an approved Built-in catalog Bundle;
3. run preflight and show the complete plan for both Tool Adapters;
4. resolve every unmanaged conflict before enabling confirmation;
5. confirm the exact Managed files and external Tool actions;
6. stage and apply reversible Managed-file changes, then perform Tool actions;
7. verify discovery where the Tool executable is available; and
8. show one explicit outcome with repair or next steps.

The outcome vocabulary is:

- **Blocked** — setup cannot safely apply; no Managed-file change remains.
- **Configured-unverified** — requested configuration and the Workspace receipt
  exist, but live Tool verification was unavailable.
- **Verified** — both Tool Adapters proved the expected project discovery.
- **Partial** — reversible Managed-file changes were rolled back, but an
  external Tool side effect remains and is recorded with a repair action.

`Partial` is not a generic warning state. It is reserved for evidence that an
external Tool changed state and Skillet could not fully reverse that change.
An ordinary staged-write failure that rolls back cleanly remains `Blocked`.

## TUI implications

- The review step needs a per-Tool plan, not one undifferentiated list.
- Missing executables are warnings when static configuration is possible, not
  automatic blockers and not silent successes.
- Unmanaged collisions disable confirmation until the user chooses a safe
  resolution; merely displaying a warning is insufficient.
- The result view must show the Workspace receipt location, outcome per Tool,
  and exact repair/verification action.
- The folder picker is only a path-selection convenience. The TUI must still
  show the normalized selected path, whether it is new/existing, its Git state,
  and detected managed/unmanaged content before confirmation.

## Remaining decisions

- Whether the portable ownership portion of the Workspace receipt is committed
  to the project or kept as local state.
- Which conflict resolutions are supported for unmanaged paths beyond the safe
  default of blocking.
- Which external Tool actions are sufficiently reversible to avoid `Partial`;
  this depends on the mechanism research for each Tool Adapter.
