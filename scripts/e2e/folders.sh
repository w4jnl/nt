#!/usr/bin/env bash
# Folders: nt mkdir, mv and rename on one machine, every link still pointing at a file after each;
# the moves reach a second clone through sync; the browser's N, m and R in an isolated tmux server.
source "$(dirname "$0")/lib.sh"

git init --quiet --bare -b main "$T/remote.git"
setup() { # setup NAME: a machine with its own config, state and clone
  printf 'repo = "%s"\nhost = "%s"\n[sync]\nauto = false\n[theme]\nmode = "dark"\n' "$T/$1" "$1" > "$T/$1.toml"; }
A() { NT_CONFIG=$T/a.toml NT_STATE=$T/a.state "$BIN" "$@"; }
B() { NT_CONFIG=$T/b.toml NT_STATE=$T/b.state "$BIN" "$@"; }
setup a; A init "$T/a" >/dev/null
git -C "$T/a" remote add origin "$T/remote.git"
json() { python3 -c 'import sys, json; d = json.load(sys.stdin); print(eval(sys.argv[1]))' "$1"; }
# broken DIR: the relative links in DIR's notes that point at no file, one per line, sorted
broken() { python3 - "$1" <<'PY'
import os, re, sys, urllib.parse
root, out = sys.argv[1], []
for d, dirs, files in os.walk(root):
    dirs[:] = [x for x in dirs if not x.startswith(".")]
    for f in files:
        if not f.endswith(".md"):
            continue
        text = open(os.path.join(d, f)).read()
        for dest in re.findall(r"\]\(([^)\s]+)\)", text):
            p = urllib.parse.unquote(dest.split("#")[0])
            if p and "://" not in p and not os.path.exists(os.path.normpath(os.path.join(d, p))):
                out.append(os.path.relpath(os.path.join(d, f), root) + " " + dest)
print("\n".join(sorted(out)))
PY
}

# two notes in tech/ from today's journal, one linking the other, an image only one of them uses
A n "NAS setup" tech >/dev/null
A n "Backups" tech >/dev/null
nas=$(cd "$T/a" && ls tech/*-nas-setup.md); backups=$(cd "$T/a" && ls tech/*-backups.md); day=$(cd "$T/a" && ls daily/*/*.md)
mkdir -p "$T/a/tech/assets"; printf 'png' > "$T/a/tech/assets/2026-10-20-rack.png"
printf '![rack](assets/2026-10-20-rack.png)\n' >> "$T/a/$nas"
printf 'See [NAS setup](%s#setup).\n' "${nas#tech/}" >> "$T/a/$backups"
sed -i.bak 's/^pins = \[\]/pins = ["tech"]/' "$T/a/.nt/config.toml" && rm "$T/a/.nt/config.toml.bak"
A sync >/dev/null 2>&1
git clone --quiet "$T/remote.git" "$T/b"; setup b
# links that point at nothing from the start: the journal's next day, AGENTS.md's examples
base=$(broken "$T/a")
# fine DIR: "ok" when DIR's notes have no broken link beyond those, else the new ones
fine() { local now; now=$(broken "$1"); if [ "$now" = "$base" ]; then echo ok; else comm -13 <(echo "$base" | sort) <(echo "$now" | sort); fi; }

# --- the commands
expect "mkdir makes a folder, its name a slug" '^home-lab/$' "$(A mkdir "Home Lab")"
expect "with a .gitkeep, so git keeps it empty" '^$' "$(cat "$T/a/home-lab/.gitkeep")"
expect "mv --dry-run says what would change" 'would move tech/.*-nas-setup.md → home-lab/' "$(A mv "$nas" home-lab/ --dry-run)"
expect "and changes nothing" "^$nas\$" "$(cd "$T/a" && ls tech/*-nas-setup.md)"
expect "mv moves the note into the folder" '^home-lab/[0-9-]+-nas-setup.md 2$' \
  "$(A mv "$nas" home-lab/ --json | json 'd["to"], len(d["files"])' | tr -d "(),'")"
expect "its image goes with it" '^png$' "$(cat "$T/a/home-lab/assets/2026-10-20-rack.png")"
expect "the journal links it at its new place" '\(\.\./\.\./home-lab/[0-9-]+-nas-setup.md\)' "$(cat "$T/a/$day")"
expect "the other note too, its anchor kept" '\(\.\./home-lab/[0-9-]+-nas-setup.md#setup\)' "$(cat "$T/a/$backups")"
expect "no link is broken" '^ok$' "$(fine "$T/a")"
nas=$(cd "$T/a" && ls home-lab/*-nas-setup.md)
expect "rename gives the note a new title and file name" 'renamed home-lab/.*-nas-setup.md → home-lab/[0-9-]+-nas-at-home.md, title "NAS at home"' \
  "$(A rename "$nas" "NAS at home")"
expect "the journal's line shows the new title" '^- \[NAS at home\]\(\.\./\.\./home-lab/[0-9-]+-nas-at-home.md\)$' "$(grep 'nas-at-home' "$T/a/$day")"
expect "the heading too" '^# NAS at home$' "$(grep '^# ' "$T/a"/home-lab/*-nas-at-home.md)"
expect "mv renames a folder when the new name is no folder yet" 'moved tech/ → projects/, 1 file' "$(A mv tech projects)"
expect "the pins follow it" '^pins = \["projects"\]' "$(cat "$T/a/.nt/config.toml")"
expect "no link is broken" '^ok$' "$(fine "$T/a")"
expect "a journal stays where its date puts it" 'stay where their date puts them' "$(A mv "$day" projects/ 2>&1)"
expect "nothing goes into daily/" 'holds only the journals' "$(A mv projects daily/ 2>&1)"

# --- the other machine
A sync >/dev/null 2>&1; B sync >/dev/null 2>&1
expect "the moves reach the other clone" '^home-lab/[0-9-]+-nas-at-home.md projects/[0-9-]+-backups.md$' \
  "$(cd "$T/b" && echo home-lab/*.md projects/*.md)"
expect "the old folder is gone there" '^gone$' "$( [ -e "$T/b/tech" ] && echo there || echo gone)"
expect "and every link works there too" '^ok$' "$(fine "$T/b")"

# --- both machines at once
nas=$(cd "$T/a" && ls home-lab/*-nas-at-home.md)
A mv "$nas" lab/ >/dev/null
printf 'Edited on b.\n' >> "$T/b/$nas"
A sync >/dev/null 2>&1; B sync >/dev/null 2>&1; A sync >/dev/null 2>&1
expect "an edit made on the other machine follows the move" '^Edited on b.$' "$(tail -1 "$T"/a/lab/*-nas-at-home.md)"
expect "the old path does not come back" '^gone$' "$(ls "$T"/a/home-lab/*.md >/dev/null 2>&1 && echo there || echo gone)"
expect "no conflict copy" '^0$' "$(find "$T/a" "$T/b" -name '*.conflict-*' | wc -l | tr -d ' ')"
nas=$(cd "$T/a" && ls lab/*-nas-at-home.md)
A mv "$nas" x/ >/dev/null; B mv "$nas" y/ >/dev/null
A sync >/dev/null 2>&1; B sync >/dev/null 2>&1; A sync >/dev/null 2>&1
expect "one note moved two ways at once is kept in both places" '^x/[0-9-]+-nas-at-home.md y/[0-9-]+-nas-at-home.md$' "$(cd "$T/a" && echo x/*.md y/*.md)"
expect "and the journal links both" '^2$' "$(grep -c 'nas-at-home.md)' "$T/a/$day")"
expect "with every link working" '^ok$' "$(fine "$T/a")"

# --- the browser: N, m, R
export NT_CONFIG=$T/a.toml NT_STATE=$T/a.state
S new-session -d -s e2e -x 140 -y 30 -c "$T" "env VISUAL= EDITOR=true NT_CONFIG='$NT_CONFIG' NT_STATE='$NT_STATE' '$BIN' projects"
expect_soon "the browser starts in projects/" '\[nt\]' capture
key() { S send-keys -t e2e "$@"; }
footer() { capture | tail -1; }
key N
expect_soon "N asks for a folder's name, saying where" 'new folder in projects:' footer
key -l '/Archive'; key Enter
expect_soon "a name from / goes in the root" 'made archive/' footer
key 1
expect_soon "the pin follows the renamed folder" 'Backups' capture
key m
expect_soon "m asks where to" 'move to:' footer
expect_soon "listing the folders it can go to" 'home-lab/' capture
key -l 'arch'; key Enter
expect_soon "and moves the note there" 'moved to archive/[0-9-]+-backups.md' footer
expect "the file is there" '^archive/[0-9-]+-backups.md$' "$(cd "$T/a" && ls archive/*.md)"
key R
expect_soon "R starts from the title" 'new title: Backups' footer
key C-u; key -l 'Old backups'; key Enter
expect_soon "and renames the note" 'renamed: Old backups' footer
expect "with the file name" '^archive/[0-9-]+-old-backups.md$' "$(cd "$T/a" && ls archive/*.md)"
expect "and no link is broken" '^ok$' "$(fine "$T/a")"
finish
