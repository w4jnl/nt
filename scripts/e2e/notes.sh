#!/usr/bin/env bash
# The note commands at a terminal: nt n and nt t open the editor on the right file, nt f picks a
# note with fzf; ls, s and doctor answer in JSON.
source "$(dirname "$0")/lib.sh"

printf 'repo = "%s"\nhost = "e2e"\n[sync]\nauto = false\n' "$T/notes" > "$T/config.toml"
export NT_CONFIG=$T/config.toml NT_STATE=$T/state
"$BIN" init "$T/notes" >/dev/null
cat > "$T/editor.sh" <<ED
#!/bin/sh
printf '%s\n' "\$@" > "$T/edited"
ED
chmod +x "$T/editor.sh"
run() { rm -f "$T/edited"; S new-session -d -s e2e -x 100 -y 30 -c "$T" "env VISUAL= EDITOR='$T/editor.sh' NT_CONFIG='$NT_CONFIG' NT_STATE='$NT_STATE' '$BIN' $*"; }
edited() { cat "$T/edited" 2>/dev/null; }
session() { if S has-session -t e2e 2>/dev/null; then echo running; else echo gone; fi; }

run n "'Plan A'" tech
expect_soon "nt n opens the new note in the editor" "/notes/tech/[0-9]{4}-[0-9]{2}-[0-9]{2}-plan-a\.md$" edited
expect_soon "nt n exits with the editor" '^gone$' session
expect "the note has its title" '^# Plan A$' "$(cat "$T"/notes/tech/*-plan-a.md)"
run t
expect_soon "nt t opens today's journal" "/notes/daily/[0-9]{4}/[0-9]{4}-[0-9]{2}-[0-9]{2}\.md$" edited
expect "the journal links the note" '\[Plan A\]\(\.\./\.\./tech/' "$(cat "$T"/notes/daily/*/*.md)"
expect_soon "nt t exits with the editor" '^gone$' session

"$BIN" n "Other topic" work >/dev/null
run f
expect_soon "nt f shows fzf" 'note>' capture
expect_soon "nt f previews the note in nt's style" 'Other topic|Plan A' capture
S send-keys -t e2e plan-a
expect_soon "fzf narrows to the typed note" '1/[0-9]+' capture
S send-keys -t e2e Enter
expect_soon "nt f opens the pick" "/notes/tech/[0-9-]+-plan-a\.md$" edited

S new-session -d -s e2e -x 100 -y 30 -c "$T" "env NT_CONFIG='$NT_CONFIG' NT_STATE='$NT_STATE' '$BIN' view --theme dark work/$(basename "$T"/notes/work/*-other-topic.md)"
expect_soon "nt view finds a note by its repository path from anywhere" 'Other topic' capture
S kill-server
rc=0; "$BIN" view work/no-such-note.md 2>"$T/err" || rc=$?
expect "nt view says where it looked" 'no such file here or in' "$(cat "$T/err")"

expect "nt ls --json lists the notes" '"title": "Other topic"' "$("$BIN" ls work --json)"
expect "nt s --json finds the text" '"path": "tech/[0-9-]+-plan-a.md"' "$("$BIN" s 'Plan A' --json)"
rc=0; "$BIN" s no-such-words --json >/dev/null || rc=$?
expect "nt s exits 1 when nothing matches" '^1$' "$rc"
expect "nt doctor --json reports its checks" '"check": "repository"' "$("$BIN" doctor --json)"
expect "nt config set changes one setting, in its file" '^summary.model = haiku' "$("$BIN" config set summary.model haiku)"
expect "nt config get reads it back" '^haiku$' "$("$BIN" config get summary.model)"
expect "a value that is not one is refused" 'one of auto, dark, light' "$("$BIN" config set theme.mode blue 2>&1)"
expect "a repository setting goes to .nt/config.toml" '^pins = \["tech", "work"\]' "$("$BIN" config set pins tech work >/dev/null; grep '^pins' "$T/notes/.nt/config.toml")"
expect "nt config --json lists every setting" '"key": "day_starts"' "$("$BIN" config --json)"
finish
