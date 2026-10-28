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
- Mac app (`app/macos`, docs/design.md §8) and iOS app (`app/ios`, §9), sharing `app/shared`
  (`#if os` where the platforms differ): Swift 6 with complete strict concurrency, SwiftUI,
  `@MainActor @Observable` state, Apple frameworks only; the editor page's CodeMirror is the one
  third-party component, and on iOS libgit2 with libssh2 and mbedTLS. Colours come only from
  `Brand.swift` and `editor.css`, which mirror the tokens. The apps write only through nt; the Mac
  app never runs git, and the iOS app's git (libgit2) runs only when nt's sync asks for it.
- Tests: table-driven, temp directories and temp git repositories; no mocks of git. `NT_CONFIG`,
  `NT_STATE` and `NT_REPO` point nt at a test's own machine config, state and repository. End-to-end
  suites in `scripts/e2e/` drive a bare repository and isolated tmux servers (`tmux -L <name>
  -f /dev/null`), in flok's style: poll for a transition, never sleep and assert.

## Rules
- Build one phase at a time (docs/design.md, Phases) and stop at its gate for review.
- Do not add views, modals, colours or keys beyond docs/design.md and the design system without
  asking. Simplicity is the main requirement.
- Never commit an index, a cache or UI state into a notes repository.
- `main` takes pull requests only (a ruleset): work on a branch, open a PR, and CI (`test` on
  macOS and Linux) must pass before it merges. Nobody pushes to `main` directly.
- Never make outbound network calls other than git and ssh to the configured remotes and hosts.
- Before calling a task done: `gofmt -l ./cmd ./internal ./skills ./mobile` is empty, `make vet` and `make test` pass;
  after a change under `app/macos` or `app/shared`, `make mac-test` and `make mac-editor` pass too;
  after a change under `app/ios`, `app/shared` or `mobile`, `make ios-core` and `make ios-test`.

## Commands
- `make build`: `bin/nt` with the version stamped from git.
- `make install`: build and copy to `~/.local/bin` (`PREFIX` overridable).
- `make test`, `make vet`, `make fmt`.
- `make brand`: regenerate `assets/brand` (needs fontTools and brotli, headless Chrome, rsvg-convert).
- `assets/demo/ghostty.sh`: record the README's browser GIF from a real, full-screen Ghostty
  window, so the note's image shows (macOS, at an unlocked screen, hands off for about 40 s).
  `assets/demo/record.sh` records the command-line GIF with vhs. Both need ffmpeg and build the
  demo notes with `assets/demo/scene.sh`.
- `make mac-run`, `make mac-test`: build and open, or test, the Mac app (Xcode 26+, XcodeGen);
  `make mac-editor`: test and rebuild its editor page after a change under `app/macos/editor/src` (Node).
- `make ios-build`, `make ios-test`, `make ios-device`: the iOS app (app/ios) in the simulator, its
  tests, or on a connected iPhone; `make ios-core` rebuilds nt for the phone (gomobile) after a Go
  change, `make ios-libgit2` its libgit2 (cmake), `make ios-ssh-test` its ssh against a test sshd.
- `make e2e`: end-to-end suites in `scripts/e2e/` (isolated tmux servers).
- `scripts/release.sh <version>`: tag a release and bump `Formula/nt.rb` in w4jnl/homebrew-tap.
  Run it on `main` after the PR with the CHANGELOG section has merged.
  It needs a `## <version> (YYYY-MM-DD)` section in `CHANGELOG.md`, the release notes.
