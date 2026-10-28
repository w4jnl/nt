#!/usr/bin/env bash
# Todos: nt todo add, list, done, undo, due and rm with --json; two clones adding to todo.md at
# the same time merge line by line, with no conflict copy; the browser's todo list (x) in an
# isolated tmux server: the list, checking a todo, a due date, the done ones; the calendar.
source "$(dirname "$0")/lib.sh"

git init --quiet --bare -b main "$T/remote.git"
setup() { # setup NAME: a machine with its own config, state and clone
  printf 'repo = "%s"\nhost = "%s"\n[sync]\nauto = false\n[theme]\nmode = "dark"\n' "$T/$1" "$1" > "$T/$1.toml"; }
A() { NT_CONFIG=$T/a.toml NT_STATE=$T/a.state "$BIN" "$@"; }
B() { NT_CONFIG=$T/b.toml NT_STATE=$T/b.state "$BIN" "$@"; }
setup a; A init "$T/a" >/dev/null
git -C "$T/a" remote add origin "$T/remote.git"
A sync >/dev/null 2>&1
git clone --quiet "$T/remote.git" "$T/b"; setup b
json() { python3 -c 'import sys, json; d = json.load(sys.stdin); print(eval(sys.argv[1]))' "$1"; }

# --- the commands
expect "the repository merges todo.md line by line" '^todo.md merge=union$' "$(cat "$T/a/.gitattributes")"
expect "add puts the todo in todo.md and says where" '^todo.md 5 2[0-9]{3}-[0-9]{2}-[0-9]{2}$' \
  "$(A todo add "Renew passport" --due fri --json | json 'd["rel"], d["line"], d["due"]' | tr -d "(),'")"
A todo add "Call the plumber due:tomorrow" >/dev/null
printf 'Read the RFC' | A todo add >/dev/null
mkdir -p "$T/a/work"
printf '# Meeting\n\n- [ ] Send the slides due:2020-01-06\n```\n- [ ] not a todo\n```\n' > "$T/a/work/2026-10-20-meeting.md"
expect "todo.md is plain markdown" '^- \[ \] Renew passport due:[0-9-]{10}$' "$(cat "$T/a/todo.md")"
expect "nt todo lists the open ones, the overdue first" '^○ Send the slides +[0-9]+d late +work/2026-10-20-meeting.md:3$' "$(A todo | head -1)"
expect "and leaves code blocks out" '^4$' "$(A todo --json | json 'len(d["todos"])')"
expect "done checks one by its words" '^done ✓ Call the plumber' "$(A todo done plumber)"
expect "with the day it was done" '^- \[x\] Call the plumber due:[0-9-]{10} done:[0-9-]{10}$' "$(cat "$T/a/todo.md")"
expect "--done lists the done ones" '^Call the plumber$' "$(A todo --done --json | json 'd["todos"][0]["text"]')"
expect "a word in two todos asks which" 'match "e"; name one by path:line' "$(A todo done e 2>&1)"
expect "due sets a date by path:line" 'due:2030-01-02$' "$(A todo due todo.md:7 2030-01-02 >/dev/null; sed -n 7p "$T/a/todo.md")"
expect "due none clears it" '^- \[ \] Read the RFC$' "$(A todo due RFC none >/dev/null; sed -n 7p "$T/a/todo.md")"
expect "undo opens it again" '^- \[ \] Call the plumber due:[0-9-]{10}$' "$(A todo undo plumber >/dev/null; sed -n 6p "$T/a/todo.md")"
expect "rm deletes the line" '^0$' "$(A todo rm 'the RFC' >/dev/null; grep -c RFC "$T/a/todo.md")"
expect "a bad date says what works" 'is not a date: use YYYY-MM-DD' "$(A todo add x --due soon 2>&1)"
expect "--in adds to a folder's own list" '^work/todo.md 5$' "$(A todo add 'Order the cables' --in work --json | json 'd["rel"], d["line"]' | tr -d "(),'")"
expect "nt todo <folder> lists the todos under it" '^Send the slides, Order the cables$' "$(A todo work --json | json '", ".join(t["text"] for t in d["todos"])')"
expect "the agents folder keeps no list" 'holds reports, not todos' "$(A todo add x --in agents 2>&1)"

# --- two machines add at the same time
A sync >/dev/null 2>&1; B sync >/dev/null 2>&1
B todo add "from B" >/dev/null
A todo add "from A" >/dev/null
A sync >/dev/null 2>&1; B sync >/dev/null 2>&1; A sync >/dev/null 2>&1
for side in a b; do
  expect "both todos are in todo.md on $side" '^1 1$' "$(echo "$(grep -c 'from A' "$T/$side/todo.md") $(grep -c 'from B' "$T/$side/todo.md")")"
done
expect "with no conflict copy" '^0$' "$(find "$T/a" "$T/b" -name '*.conflict-*' | wc -l | tr -d ' ')"

# --- the browser's todo list
export NT_CONFIG=$T/a.toml NT_STATE=$T/a.state
S new-session -d -s e2e -x 140 -y 30 -c "$T" "env VISUAL= EDITOR=true NT_CONFIG='$NT_CONFIG' NT_STATE='$NT_STATE' '$BIN'"
expect_soon "the browser starts" '\[nt\]' capture
key() { S send-keys -t e2e "$@"; }
footer() { capture | tail -1; }
expect_soon "the footer offers the todos" 'x todos' footer
key x
expect_soon "x lists the open todos" 'todo · 6 open · 1 late' capture
expect_soon "the overdue one first, late in words" '○ Send the slides +[0-9]+d late' capture
expect_soon "with the note it is in beside it" 'Meeting' capture
expect_soon "and the todo keys in the footer" 'x done +d due +a add +X done ones' footer
key x
expect_soon "x checks it" 'done: Send the slides' footer
expect_soon "it leaves the open ones" 'todo · 5 open' capture
expect "in the file" '^- \[x\] Send the slides due:2020-01-06 done:[0-9-]{10}$' "$(sed -n 3p "$T/a/work/2026-10-20-meeting.md")"
key d
expect_soon "d asks for the due date" 'due \(fri, \+3d' footer
key -l '+3d'; key Enter
expect_soon "and sets it" 'due [a-z0-9 ]+: ' footer
key X
expect_soon "X shows the done ones" 'done · 1' capture
expect_soon "with when they were done" '✓ Send the slides +today' capture
key a
expect_soon "a asks for a todo, saying where it goes" 'todo in todo.md:' footer
key -l 'From the browser'; key Enter
expect_soon "and adds it to the open ones" '○ From the browser' capture
expect "at the end of the root's list" '^- \[ \] From the browser$' "$(tail -1 "$T/a/todo.md")"
key Escape
expect_soon "esc goes back to the folders" 't today.*x todos' footer

# --- the calendar
# eight days on: never this week's Friday, when the passport is due
A todo add "Dentist" --due +8d >/dev/null
key c
expect_soon "c shows this month" "$(LC_ALL=C date '+%B %Y')" capture
expect_soon "with its keys in the footer" 'h l day +j k week +\[ \] month' footer
key j; key l
expect_soon "the day eight days on has the dentist" '○ Dentist' capture
key x
expect_soon "x lists that day's todos" '· todo · 1 open' capture
key Escape
expect_soon "esc goes back to the calendar" 'h l day' footer
key Escape
expect_soon "and esc again to the folders" 't today.*x todos' footer
finish
