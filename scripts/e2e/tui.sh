#!/usr/bin/env bash
# The browser (nt with no arguments) in an isolated tmux server: folders and notes, the preview,
# today, add, new note through the editor, live search, titles, links and "Linked from", the
# outline, days, pins, help, sync, the mouse and a narrow screen.
source "$(dirname "$0")/lib.sh"

git init --quiet --bare -b main "$T/remote.git"
printf 'repo = "%s"\nhost = "e2e"\n[sync]\nauto = false\n[theme]\nmode = "dark"\n' "$T/notes" > "$T/config.toml"
export NT_CONFIG=$T/config.toml NT_STATE=$T/state
"$BIN" init "$T/notes" >/dev/null
git -C "$T/notes" remote add origin "$T/remote.git"
N=$T/notes
mkdir -p "$N/tech" "$N/work" "$N/daily/2025"
printf -- '---\ntitle: NAS setup\n---\n\n# NAS setup\n\nThe unicorn mount lives here.\n\n## Mounting\n\nSteps.\n\n## Unmounting\n\nMore steps.\n' > "$N/tech/2026-10-20-nas-setup.md"
printf -- '---\ntitle: Project plan\n---\n\n# Project plan\n\nSee [the NAS](../tech/2026-10-20-nas-setup.md) first.\n' > "$N/work/2026-10-21-project-plan.md"
printf -- '---\ntitle: Old day\ndate: 2025-12-31\ntype: daily\n---\n\n# Old day\n\n## Log\n\n- 10:00 last year\n' > "$N/daily/2025/2025-12-31.md"
sed -i.bak 's/^pins = \[\]/pins = ["work", "tech"]/' "$N/.nt/config.toml" && rm "$N/.nt/config.toml.bak"
"$BIN" sync >/dev/null 2>&1

cat > "$T/editor.sh" <<ED
#!/bin/sh
for f; do :; done
printf '%s\n' "\$f" > "$T/edited"
printf 'Written in the editor.\n' >> "\$f"
ED
chmod +x "$T/editor.sh"
start() { S kill-server 2>/dev/null || true
  S new-session -d -s e2e -x "${1:-120}" -y 30 -c "$T" "env VISUAL= EDITOR='$T/editor.sh' NT_CONFIG='$NT_CONFIG' NT_STATE='$NT_STATE' '$BIN'"
  expect_soon "the browser starts" '\[nt\]' capture; }
key() { S send-keys -t e2e "$@"; }
footer() { capture | tail -1; }
session() { if S has-session -t e2e 2>/dev/null; then echo running; else echo gone; fi; }

start
expect_soon "the root lists the folders" 'tech/' capture
expect_soon "the brand line shows the sync state" '● synced' sh -c "tmux -L nt-e2e -f /dev/null capture-pane -p -t e2e | head -1"
expect_soon "the footer shows the key hints" 't today.*\? help' footer

key 1
expect_soon "1 opens the first pinned folder" '\[nt\]  work' capture
expect_soon "the folder's note shows in the preview" 'See the NAS' capture
key l
expect_soon "l lists the note's links" '→ the NAS' capture
key Enter
expect_soon "following a link opens the note" 'The unicorn mount lives here' capture
expect_soon "the linked note shows what links to it" '← Project plan' capture
key o
expect_soon "o lists the headings" 'outline · 3 headings' capture
key j j Enter
expect_soon "the outline jumps to a heading" 'More steps' capture

key t
expect_soon "t opens today's journal" '  today' capture
key a
expect_soon "a opens the add prompt" 'add to today:' footer
key -l 'from the browser'
key Enter
expect_soon "the line lands in today's journal" 'from the browser' capture
key '['
expect_soon "[ goes to the day before that exists" 'last year' capture
key ']'
expect_soon "] comes back to today" 'from the browser' capture

key 2
key n
expect_soon "n asks for a title in the current folder" 'new note in tech' footer
key -l 'Browser note'
key Enter
expect_soon "the new note opens in the editor" '/tech/[0-9-]+-browser-note\.md$' cat "$T/edited"
expect_soon "the list shows and selects the new note" '▎.*Browser note' capture
expect_soon "the preview shows what the editor wrote" 'Written in the editor' capture
expect "the journal links the new note" 'Browser note\]\(\.\./\.\./tech/' "$(cat "$N"/daily/*/"$(date +%F)".md)"

key /
key -l unicorn
expect_soon "/ searches as you type" 'search · 1 line' capture
expect_soon "the preview follows the result" 'The unicorn mount lives here' capture
key Enter
expect_soon "enter goes to the result's note" '▎.*NAS setup' capture
key C-p
key -l plan
expect_soon "ctrl-p finds notes by title" 'Project plan' capture
key Enter
expect_soon "enter goes to the note" '▎.*Project plan' capture
key '?'
expect_soon "? shows the keys" 'this help' capture
key '?'

remote_files() { git -C "$T/remote.git" ls-tree -r --name-only main; }
key s
expect_soon "s syncs the new note to the remote" 'browser-note' remote_files
expect_soon "the badge says synced after it" '● synced' sh -c "tmux -L nt-e2e -f /dev/null capture-pane -p -t e2e | head -1"
key q
expect_soon "q quits" '^gone$' session

# The divider: the column of the first │ on the first row of the panes. Mouse events go in as
# the SGR reports a terminal sends (1-based columns; 32 is motion with the button held).
divider() { capture | sed -n 3p | python3 -c 'import sys; print(sys.stdin.read().index("│"))'; }
mouse() { key -l $'\e[<'"$1"'M'; }
start
expect_soon "the list starts at list-width columns" '^44$' divider
mouse '0;45;5' && mouse '32;55;5' && mouse '32;61;5' && key -l $'\e[<0;61;5m'
expect_soon "dragging the divider resizes the list" '^60$' divider
key '<'
expect_soon "< makes the list narrower" '^56$' divider
mouse '0;10;5'
expect_soon "a click selects a row" '▎ work/' capture
key q
expect_soon "q quits" '^gone$' session
start
expect_soon "the width is remembered" '^56$' divider
key q
expect_soon "q quits" '^gone$' session

start 60
expect_soon "a narrow screen shows the list alone" 'tech/' capture
key 2
key Enter
expect_soon "enter shows the note on its own" 'Written in the editor' capture
key Escape
expect_soon "escape goes back to the list" '▎' capture
key q
finish
