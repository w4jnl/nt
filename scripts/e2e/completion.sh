#!/usr/bin/env bash
# Shell completion in real interactive shells on an isolated tmux server: zsh (compinit, then
# eval "$(nt completion zsh)") and bash, completing commands, folders, notes and flag values.
source "$(dirname "$0")/lib.sh"

printf 'repo = "%s"\nhost = "e2e"\n[sync]\nauto = false\n' "$T/notes" > "$T/config.toml"
export NT_CONFIG=$T/config.toml NT_STATE=$T/state
"$BIN" init "$T/notes" >/dev/null
mkdir -p "$T/notes/tech" "$T/notes/work"
printf '# NAS\n' > "$T/notes/tech/2026-10-20-nas.md"
ENV="env PATH='$(dirname "$BIN"):$PATH' NT_CONFIG='$NT_CONFIG' NT_STATE='$NT_STATE' HOME='$T'"
key() { S send-keys -t e2e "$@"; }
line() { capture | grep -E '^[^ ]*[%$] ' | tail -1; }
try() { key C-u; key -l "$1"; key Tab; }
prompts() { capture | grep -cE "^$1( |$)" || true; }
# start SHELL-COMMAND PROMPT LOAD: a shell in the pane; type only once it shows its prompt, then
# load the completion and wait for the prompt after it.
start() { S new-session -d -s e2e -x 100 -y 20 -c "$T" "$ENV $1"
  expect_soon "$2 starts" '^1$' prompts "$3"
  key -l "$4"; key Enter
  expect_soon "$2 loads the completion" '^2$' prompts "$3"; }

if command -v zsh >/dev/null; then
  start "PS1='z%# ' zsh -f" zsh 'z%' 'autoload -Uz compinit && compinit -u && eval "$(nt completion zsh)"'
  try 'nt l'
  expect_soon "zsh: a command" 'nt ls *$' line
  try 'nt ls t'
  expect_soon "zsh: ls takes a folder" 'nt ls tech' line
  try 'nt te'
  expect_soon "zsh: a folder in place of a command opens the browser there" 'nt tech' line
  try 'nt view tech/'
  expect_soon "zsh: view takes a note" 'nt view tech/2026-10-20-nas\.md' line
  try 'nt view --theme '
  expect_soon "zsh: a flag's values" 'auto +dark +light' capture
  try 'nt n "a title" w'
  expect_soon "zsh: n's folder after the title" 'nt n "a title" work' line
  try 'nt install --n'
  expect_soon "zsh: a command's flags" 'nt install --nvim' line
  S kill-session -t e2e
else
  echo "SKIP  zsh is not installed"
fi

start "PS1='b\$ ' bash --norc --noprofile" bash 'b\$' 'eval "$(nt completion bash)"'
try 'nt ls w'
expect_soon "bash: ls takes a folder" 'nt ls work' line
try 'nt paste tech/2'
expect_soon "bash: paste takes a note" 'nt paste tech/2026-10-20-nas\.md' line
try 'nt completion z'
expect_soon "bash: completion takes a shell" 'nt completion zsh' line
finish
