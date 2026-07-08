# Bubbles + Lip Gloss for rendering

The v1 TUI (`internal/tui/model.go`) renders everything with `fmt.Sprintf` and `strings.Builder` — no color, no styling, and a hand-rolled cursor/scroll loop. This produces unaligned columns, mid-word-truncated descriptions, a 200+ character unwrapped help line, and confirmation prompts that wipe the whole screen down to one line of plain text. Phase 2 replaces this with `bubbles` (Charm's official `list.Model`, and `help.Model` for contextual key hints) for structure, and `lipgloss` for styling (borders, color, adaptive light/dark, the confirmation overlay). Both are already transitive dependencies of `bubbletea` (see [0001](./0001-go-bubbletea-tui.md)), so this costs nothing new in the binary and stays inside the ecosystem that ADR already chose for its maturity.

## Considered Options

- **Keep hand-rolled rendering, add lipgloss only** — styles the existing string-formatting loop without solving the underlying problems (list scrolling, selection, truncation) that `bubbles` already handles; would mean re-implementing what `list.Model` gives for free, and re-implementing it again later if we ever adopt `bubbles` anyway.
- **Custom component from scratch** — full control, but no benefit over `bubbles` for a fairly standard list+detail TUI, and higher maintenance cost with no upstream fixes/improvements.
- **A different TUI-adjacent library entirely (e.g. `gocui`, raw `termbox`)** — would abandon the `bubbletea` architecture ADR 0001 already committed to; no motivating reason to revisit that choice here.
