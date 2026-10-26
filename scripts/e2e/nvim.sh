#!/usr/bin/env bash
# nt's nvim plugin in a real nvim on an isolated tmux server: install --nvim and doctor, the paste
# key only in notes, the edit hold while a note has unsaved changes (sync does not merge under
# it), a note changed on disk reloading, and a sync when nvim exits after a write.
source "$(dirname "$0")/lib.sh"
if ! command -v nvim >/dev/null; then echo "SKIP  nvim is not installed"; finish; exit 0; fi

git init --quiet --bare -b main "$T/remote.git"
printf 'repo = "%s"\nhost = "e2e"\n[sync]\nauto = true\n' "$T/notes" > "$T/config.toml"
export NT_CONFIG=$T/config.toml NT_STATE=$T/state XDG_DATA_HOME=$T/data
"$BIN" init "$T/notes" >/dev/null
git -C "$T/notes" remote add origin "$T/remote.git"
N=$T/notes
mkdir -p "$N/tech"
printf '# NAS\n\nFirst line.\n' > "$N/tech/nas.md"
"$BIN" sync >/dev/null 2>&1

plugin() { "$BIN" doctor --json 2>/dev/null | python3 -c 'import sys, json
print(*[c["state"] + " " + c["detail"] for c in json.load(sys.stdin)["checks"] if c["check"] == "nvim plugin"])'; }
expect "doctor asks for the plugin" '^warn not installed' "$(plugin)"
expect "install --nvim writes the plugin" 'installed the nvim plugin' "$("$BIN" install --nvim)"
expect "doctor finds it current" '^ok ' "$(plugin)"

# nt on nvim's PATH is the binary under test, except that paste answers without a clipboard.
mkdir -p "$T/bin"
cat > "$T/bin/nt" <<SH
#!/bin/sh
if [ "\$1" = paste ]; then printf '%s\n' "\$*" > "$T/pasted"; echo '![shot](assets/2026-10-26-shot.png)'; exit 0; fi
exec "$BIN" "\$@"
SH
chmod +x "$T/bin/nt"
S new-session -d -s e2e -x 100 -y 30 -c "$T" "env PATH='$T/bin:$PATH' NT_CONFIG='$NT_CONFIG' NT_STATE='$NT_STATE' nvim --clean --cmd 'set rtp^=$T/data/nt/nvim' '$N/tech/nas.md'"
expect_soon "nvim opens the note" 'First line' capture
key() { S send-keys -t e2e "$@"; }
ex() { S send-keys -t e2e -l ":$1"; S send-keys -t e2e Enter; }
mapped() { rm -f "$T/map"; ex "lua vim.fn.writefile({vim.fn.maparg('<leader>ip', 'n') ~= '' and 'mapped' or 'none'}, '$T/map')"; sleep 0.3; cat "$T/map" 2>/dev/null; }
edit_lock() { python3 -c 'import fcntl, sys
f = open(sys.argv[1], "a")
try:
    fcntl.flock(f, fcntl.LOCK_EX | fcntl.LOCK_NB); print("free")
except OSError:
    print("held")' "$N/.git/nt-edit"; }

expect_soon "a note gets the paste key" '^mapped$' mapped
expect "an unchanged note holds nothing" '^free$' "$(edit_lock)"
key '\' i p
expect_soon "the key asks for a name" 'image name' capture
key -l shot
key Enter
expect_soon "the image line goes below the cursor" '!\[shot\]\(assets/2026-10-26-shot\.png\)' capture
expect "paste gets the note and the name" '^paste /.*/notes/tech/nas\.md --name shot$' "$(cat "$T/pasted")"
expect_soon "unsaved changes hold the merge back" '^held$' edit_lock

git clone --quiet "$T/remote.git" "$T/other"
printf '# Other\n' > "$T/other/tech/other.md"
git -C "$T/other" add -A && git -C "$T/other" commit --quiet -m other && git -C "$T/other" push --quiet origin main
"$BIN" sync >/dev/null 2>&1 || true
expect "sync does not merge under unsaved changes" '^absent$' "$([ -e "$N/tech/other.md" ] && echo present || echo absent)"

ex w
expect_soon "writing lets the hold go" '^free$' edit_lock
"$BIN" sync >/dev/null 2>&1 || true
expect "then sync merges" '^present$' "$([ -e "$N/tech/other.md" ] && echo present || echo absent)"

printf 'Changed on disk.\n' >> "$N/tech/nas.md"
expect_soon -t 8 "a note changed on disk reloads" 'Changed on disk' capture

printf '# Elsewhere\n' > "$T/outside.md"
ex "e $T/outside.md"
expect_soon "a file outside the repository gets no key" '^none$' mapped

ex "e $N/tech/nas.md"
key G o
key -l 'Third line.'
key Escape
ex w
ex qa
remote_note() { git -C "$T/remote.git" show main:tech/nas.md; }
expect_soon -t 10 "leaving nvim after a write syncs" 'Third line' remote_note

printf -- '-- older\n' > "$T/data/nt/nvim/lua/nt/init.lua"
expect "doctor notices an older plugin" '^warn older than this nt' "$(plugin)"
expect "uninstall --nvim removes it" 'removed the nvim plugin' "$("$BIN" uninstall --nvim)"
expect "and doctor asks for it again" '^warn not installed' "$(plugin)"
finish
