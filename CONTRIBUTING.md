# Contributing

nt is a personal tool, published because the approach may be useful to others. Read
[Scope and status](README.md#scope-and-status) first: features are the ones I need, and requests
that do not fit that workflow will probably be declined, politely.

What is welcome:

- Bug reports with a reproduction and the output of `nt doctor` (the issue form asks for it).
  Leave your notes out: a few lines that show the problem in a fresh `nt init` repository are
  better than a real one.
- Fixes with a test. Unit tests are table-driven and run in temp folders and temp git
  repositories; git is never mocked. The end-to-end suites in `scripts/e2e/` drive a bare
  repository standing in for GitHub and the browser in isolated tmux servers
  (`tmux -L <name> -f /dev/null`), so they never touch your own. `gofmt -l ./cmd ./internal
  ./skills` must be empty and `make vet`, `make test` and `make e2e` must pass; CI runs them on
  macOS and Linux. In a suite, assert a change with `expect_soon` (it polls until the pattern
  holds), never with `sleep` and then `expect`.
- Small, focused pull requests against `main`, which takes pull requests only. Describe what
  changed and what you verified, in plain words.

Before a larger change, open an issue and ask; it saves both of us a rewrite. `docs/design.md`
is the spec and `docs/format.md` the repository format every client follows: a change to how
notes are stored starts there. `CLAUDE.md` lists the conventions in short: one cgo-free Go
binary, `--json` on every command that prints data, every write a temp file renamed into place
under the repository lock, colours only from `internal/theme`, and the voice of the docs and
messages.
