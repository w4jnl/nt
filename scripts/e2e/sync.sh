#!/usr/bin/env bash
# nt sync between two machines through a bare repository standing in for GitHub: concurrent
# appends on both while both sync in loops lose nothing; a note changed on both sides leaves one
# conflict copy and no markers; an edit beats a delete; an open editor holds the merge back.
source "$(dirname "$0")/lib.sh"

git init --quiet --bare -b main "$T/remote.git"
machine() { mkdir -p "$T/$1"; printf 'repo = "%s"\nhost = "%s"\n[sync]\nauto = false\n' "$T/$1/notes" "$1" > "$T/$1/config.toml"; }
on() { local m=$1; shift; NT_CONFIG="$T/$m/config.toml" NT_STATE="$T/$m/state" "$BIN" "$@"; }
state() { on "$1" sync --json | python3 -c 'import json,sys; s=json.load(sys.stdin); print(s["state"], len(s.get("new_conflicts") or []), len(s.get("conflicts") or []))'; }
markers() { if grep -rlE '^(<<<<<<<|>>>>>>>)' "$T/$1/notes" --exclude-dir=.git; then echo "markers"; else echo "clean"; fi; }
head_of() { git -C "$T/$1/notes" rev-parse HEAD; }

machine alpha; machine beta
on alpha init >/dev/null
git -C "$T/alpha/notes" remote add origin "$T/remote.git"
expect "alpha pushes the new repository" '^synced 0 0$' "$(state alpha)"
on beta init --clone "$T/remote.git" "$T/beta/notes" >/dev/null 2>&1
expect "beta clones it with nt's files" 'merge=union' "$(cat "$T/beta/notes/.gitattributes" 2>/dev/null)"

# Concurrent writers: three per machine append to today's journal while each machine syncs in
# a loop, as agents and the timer will.
writers() { local m=$1 w
  for w in 1 2 3; do ( for i in $(seq -w 1 15); do on "$m" a "$m writer $w line $i" >/dev/null; done ) & done; wait; }
syncloop() { while [ ! -e "$T/stop" ]; do on "$1" sync >/dev/null 2>&1; done; }
syncloop alpha & SA=$!
syncloop beta & SB=$!
writers alpha & WA=$!
writers beta & WB=$!
wait $WA $WB
touch "$T/stop"; wait $SA $SB
for m in alpha beta alpha beta; do on "$m" sync >/dev/null 2>&1; done
J=$(on alpha t --json | python3 -c 'import json,sys; print(json.load(sys.stdin)["rel"])')
count() { grep -cE "^- [0-9]{2}:[0-9]{2} (alpha|beta) writer" "$T/$1/notes/$J"; }
expect "alpha's journal has all 90 lines" '^90$' "$(count alpha)"
expect "beta's journal has all 90 lines" '^90$' "$(count beta)"
expect "both machines end on the same commit" "^$(head_of alpha)\$" "$(head_of beta)"
expect "the journal has one Log section" '^1$' "$(grep -c '^## Log' "$T/alpha/notes/$J")"
expect "no conflict copies from journals" '^$' "$(cd "$T/alpha/notes" && git ls-files | grep conflict- || true)"
expect "no conflict markers" '^clean$' "$(markers alpha)"

# The same note changed on both machines: the remote version stays, the local one is copied.
NOTE=$(NT_CONFIG="$T/alpha/config.toml" "$BIN" n "Shared plan" tech </dev/null)
REL=${NOTE#"$T/alpha/notes/"}
on alpha sync >/dev/null 2>&1; on beta sync >/dev/null 2>&1
echo "alpha's line" >> "$T/alpha/notes/$REL"
echo "beta's line" >> "$T/beta/notes/$REL"
on alpha sync >/dev/null 2>&1
expect "beta: one new conflict copy" '^conflict 1 1$' "$(state beta)"
expect "beta keeps alpha's version in place" "alpha's line" "$(cat "$T/beta/notes/$REL")"
expect "beta's version is in the copy" "beta's line" "$(cat "$T/beta/notes/${REL%.md}".conflict-beta-*.md)"
expect "alpha sees the conflict copy too" '^conflict 0 1$' "$(state alpha)"
expect "no conflict markers after a conflict" '^clean$' "$(markers beta)"
rm "$T/alpha/notes/${REL%.md}".conflict-beta-*.md
expect "deleting the copy clears the conflict" '^synced 0 0$' "$(state alpha)"
on beta sync >/dev/null 2>&1

# An edit beats a delete.
rm "$T/alpha/notes/$REL"
echo "edited after the delete" >> "$T/beta/notes/$REL"
on alpha sync >/dev/null 2>&1; on beta sync >/dev/null 2>&1; on alpha sync >/dev/null 2>&1
expect "the edited note survives on alpha" 'edited after the delete' "$(cat "$T/alpha/notes/$REL" 2>/dev/null)"

# An editor open through nt holds beta's merge back until it closes.
cat > "$T/editor.sh" <<ED
#!/bin/sh
touch "$T/editor-open"
while [ ! -e "$T/editor-close" ]; do sleep 0.1; done
for f; do :; done   # the file is the last argument
echo "- 23:59 written in the editor" >> "\$f"
ED
chmod +x "$T/editor.sh"
S new-session -d -s e2e -x 100 -y 30 "env VISUAL= EDITOR='$T/editor.sh' NT_CONFIG='$T/beta/config.toml' NT_STATE='$T/beta/state' '$BIN' t"
expect_soon "the editor is open" '^open$' sh -c "[ -e '$T/editor-open' ] && echo open"
on alpha a "alpha while beta edits" >/dev/null; on alpha sync >/dev/null 2>&1
beta_sync() { on beta sync --json | python3 -c 'import json,sys; s=json.load(sys.stdin); print(s["state"], s.get("message",""))'; }
expect "beta does not merge while its editor is open" 'waiting for the editor' "$(beta_sync)"
expect "beta's journal is untouched meanwhile" '^0$' "$(grep -c 'alpha while beta edits' "$T/beta/notes/$J")"
touch "$T/editor-close"
expect_soon "the editor closes" '^closed$' sh -c "tmux -L nt-e2e -f /dev/null has-session -t e2e 2>/dev/null && echo open || echo closed"
on beta sync >/dev/null 2>&1
expect "after the editor, beta has alpha's line" 'alpha while beta edits' "$(cat "$T/beta/notes/$J")"
expect "and the editor's line" 'written in the editor' "$(cat "$T/beta/notes/$J")"
finish
