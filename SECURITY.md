# Security policy

nt runs on your own machine over a folder of markdown. It makes no network calls of its own: git
and ssh reach the remote you configured, and the editor, `rg`, `fzf` and the OS's opener run as
you. The attack surface is local:

- the notes themselves, rendered in the browser (links, images, HTML `<img>` tags) and opened
  with the OS's opener when you follow a link;
- `nt agent`, which Claude Code and Copilot CLI run with their hook payloads, and `nt agent
  write`, which agents run with text they wrote;
- `nt import obsidian`, which reads a vault you point it at;
- the launchd or systemd job that runs `nt sync`, and the hooks and the nvim plugin
  `nt install` writes.

## Supported versions

Only the latest release gets fixes. Update with `brew upgrade nt` or from the
[releases page](https://github.com/w4jnl/nt/releases).

## Reporting a vulnerability

Use GitHub's private vulnerability reporting for this repository:
<https://github.com/w4jnl/nt/security/advisories/new>. Describe what an attacker needs to
control (a note, a hook payload, a vault, a config file) and what they gain. You will get an
answer within a week; fixes ship as a patch release with a note in `CHANGELOG.md`. Please do not
open a public issue for something exploitable before it is fixed.
