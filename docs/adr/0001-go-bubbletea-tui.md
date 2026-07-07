# Go + Bubble Tea for the TUI

The Skills Manager is an interactive TUI ("see every installed skill, act on items"), for John now and possibly public later. We chose Go with Bubble Tea over TypeScript/Ink, Rust/Ratatui, and Python/Textual because Bubble Tea is the most mature framework for full-screen list-driven TUIs, and Go ships as a single dependency-free binary — the cleanest distribution story if the tool goes public.

## Considered Options

- **TypeScript + Ink** — the Claude Code ecosystem's language and npm reaches the exact audience, but Ink is weaker for full-screen list UIs and a global npm install is a worse end-user story than a binary.
- **Rust + Ratatui** — also a single binary, but slower to iterate in for no benefit in a config-file-reading tool.
- **Python + Textual** — fastest to prototype, weakest distribution story.
