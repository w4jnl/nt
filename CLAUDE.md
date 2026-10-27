# nt

Plain markdown notes, synced by git. A TUI-first note tool: daily journals, dated topic notes and
agent reports in a plain folder of markdown files that git keeps in sync. One cgo-free Go binary.
Sibling of github.com/w4jnl/flok and github.com/w4jnl/vink; this repository is the tool, and the
person's notes live in their own repository.

## Read first
- `docs/design.md` is the spec: repository contract, sync, agents, CLI and TUI, images and
  platforms, design language, code layout, phases and verification. It is the source of truth;
  ask before deviating from it.
- `docs/format.md` is the repository format every client follows (nt, agents, the later Mac and
  iOS apps). A change to it that older clients would misread raises `format`.
- `docs/design-system/README.md` is the brand and terminal UI rules; `tokens.json` holds every
  colour. `internal/theme` mirrors it and `TestTokensMatchDesignSystem` keeps the two equal.
- `docs/research.md` is the survey behind the design: what exists, what was borrowed, what to avoid.
- `assets/brand/` holds the marks, lockups, tiles and banners; `gen.py` regenerates the SVGs from
  one geometry.

## House conventions
- Go 1.27, cgo-free, `internal/` packages, stdlib first. Subcommands are dispatched by hand in
  `internal/cli/root.go`, as in flok; `--json` on every command that prints data.
- TUI: Bubble Tea v2, Lip Gloss v2, Bubbles v2 and Glamour v2 (`charm.land/...`). Colours come
  only from `internal/theme`; never write a hex value or an ANSI colour number elsewhere.
- Reuse the OS: git for sync, `rg` for search, `fzf` for pickers outside the TUI, `$VISUAL` /
  `$EDITOR` for editing, ssh for remote hosts, Quick Look / `xdg-open` / `chafa` for images.
  Exec argv directly, never through a shell.
- Every write to a notes repository is a temp file `.nt-tmp-*` renamed into place, under the
  repository lock `.git/nt.lock` (flock). Only `nt sync` runs git commands that change the
  repository, and it merges; it never rebases and never leaves conflict markers.
- Voice in docs, help text and messages: technical, direct, short sentences; no emoji, no
  exclamation marks; `nt` always lowercase. States are lowercase words with their glyph, never
  colour alone. Errors say what is wrong and how to fix it.
- Tests: table-driven, temp directories and temp git repositories; no mocks of git. `NT_CONFIG`,
  `NT_STATE` and `NT_REPO` point nt at a test's own machine config, state and repository. End-to-end
  suites in `scripts/e2e/` drive a bare repository and isolated tmux servers (`tmux -L <name>
  -f /dev/null`), in flok's style: poll for a transition, never sleep and assert.

## Rules
- Build one phase at a time (docs/design.md, Phases) and stop at its gate for review.
- Do not add views, modals, colours or keys beyond docs/design.md and the design system without
  asking. Simplicity is the main requirement.
- Never commit an index, a cache or UI state into a notes repository.
- Never make outbound network calls other than git and ssh to the configured remotes and hosts.
- Before calling a task done: `gofmt -l ./cmd ./internal ./skills` is empty, `make vet` and `make test` pass.

## Commands
- `make build`: `bin/nt` with the version stamped from git.
- `make install`: build and copy to `~/.local/bin` (`PREFIX` overridable).
- `make test`, `make vet`, `make fmt`.
- `make brand`: regenerate `assets/brand` (needs fontTools and brotli, headless Chrome, rsvg-convert).
- `make e2e`: end-to-end suites in `scripts/e2e/` (isolated tmux servers).
- `scripts/release.sh <version>`: tag a release and bump `Formula/nt.rb` in w4jnl/homebrew-tap.
  It needs a `## <version> (YYYY-MM-DD)` section in `CHANGELOG.md`, the release notes.
