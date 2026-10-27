#!/usr/bin/env bash
# The weekly digest: the one machine with weekly = true compiles this week's digest on each sync,
# from the journals, the notes made, the todos and the agent sessions; it goes out with the notes,
# follows what the other machine writes, and a sync with nothing new makes no commit. The weekly
# summary: each machine has its own tool summarize what was written on it, and the digest shows
# them all.
source "$(dirname "$0")/lib.sh"

git init --quiet --bare -b main "$T/remote.git"
setup() { # setup NAME [weekly]: a machine with its own config, state and clone
  printf 'repo = "%s"\nhost = "%s"\nweekly = %s\n[sync]\nauto = false\n' "$T/$1" "$1" "${2:-false}" > "$T/$1.toml"; }
A() { NT_CONFIG=$T/a.toml NT_STATE=$T/a.state "$BIN" "$@"; }
B() { NT_CONFIG=$T/b.toml NT_STATE=$T/b.state "$BIN" "$@"; }
setup a true; A init "$T/a" >/dev/null
git -C "$T/a" remote add origin "$T/remote.git"
json() { python3 -c 'import sys, json; d = json.load(sys.stdin); print(eval(sys.argv[1]))' "$1"; }

A a "Called the bank" >/dev/null
A n "NAS setup" tech >/dev/null
A todo add "Renew passport" --due today >/dev/null
A sync >/dev/null 2>&1
week=$(cd "$T/a" && ls weekly/*/*.md 2>/dev/null)
expect "the weekly machine compiles this week's digest on sync" '^weekly/[0-9]{4}/[0-9]{4}-W[0-9]{2}\.md$' "$week"
expect "with today's journal line" '^- [0-9:]{5} Called the bank$' "$(cat "$T/a/$week")"
expect "the note made this week" '^- \[NAS setup\]\(\.\./\.\./tech/[0-9-]+-nas-setup\.md\) · tech · new$' "$(cat "$T/a/$week")"
expect "and the todo due" '^- ○ Renew passport · due ' "$(cat "$T/a/$week")"
expect "it went out with the notes" "^$week\$" "$(git -C "$T/a" show --name-only --format= origin/main | grep weekly/)"
before=$(git -C "$T/a" rev-list --count HEAD)
A sync >/dev/null 2>&1
expect "a sync with nothing new makes no commit" "^$before\$" "$(git -C "$T/a" rev-list --count HEAD)"

git clone --quiet "$T/remote.git" "$T/b"; setup b
B a "From the other machine" >/dev/null
B sync >/dev/null 2>&1
expect "the other machine does not compile it" '^0$' "$(grep -c 'From the other machine' "$T/b/$week")"
A sync >/dev/null 2>&1 # it compiles before the merge brings the line in: the next run has it
A sync >/dev/null 2>&1
expect "the weekly machine takes in the other's line on its next run" '^- [0-9:]{5} From the other machine$' "$(cat "$T/a/$week")"
B sync >/dev/null 2>&1
expect "and the other machine gets the digest back" '^1$' "$(grep -c 'From the other machine' "$T/b/$week")"

# --- the weekly summary: each machine summarizes what was written on it, with its own tool
mkdir -p "$T/tools"
for tool in claude copilot; do # stand-ins that keep what they were given
  printf '#!/bin/sh\ncat > "%s/%s.in"; printf "%%s\\n" "$@" > "%s/%s.args"\necho "Summary by %s."\n' "$T/tools" $tool "$T/tools" $tool $tool > "$T/tools/$tool"
  chmod +x "$T/tools/$tool"
done
printf '[summary]\ntool = "claude"\n' >> "$T/a.toml"
printf '[summary]\ntool = "copilot"\n' >> "$T/b.toml"
B a "Billing migration needs a freeze exception" >/dev/null
B sync >/dev/null 2>&1; A sync >/dev/null 2>&1
expect "nt week --summarize has this machine's tool write its summary" "^wrote $(dirname "$week")/[0-9]{4}-W[0-9]{2}-a\.md" \
  "$(PATH="$T/tools:$PATH" A week --summarize)"
expect "from what was written on this machine" 'Called the bank' "$(cat "$T/tools/claude.in")"
expect "and nothing written on the other" '^0$' "$(grep -c 'Billing migration' "$T/tools/claude.in")"
expect "the other machine's summary is its own, with its own tool" "^wrote $(dirname "$week")/[0-9]{4}-W[0-9]{2}-b\.md" \
  "$(PATH="$T/tools:$PATH" B week --summarize)"
expect "with its own week" 'Billing migration' "$(cat "$T/tools/copilot.args")"
B sync >/dev/null 2>&1; A sync >/dev/null 2>&1; A sync >/dev/null 2>&1
expect "the weekly digest shows both summaries" '### a · claude.*### b · copilot' "$(tr '\n' ' ' < "$T/a/$week")"
reports() { find "$T/a/agents" -name "*--$1*" 2>/dev/null | wc -l | tr -d ' '; }
expect "nt's own call to an agent is not reported as a session" '^0$' \
  "$(printf '{"session_id":"s1abcdef","cwd":"/tmp"}' | NT_NO_REPORT=1 A agent start; reports s1abcdef)"
expect "while the same hook without it is" '^1$' \
  "$(printf '{"session_id":"s2abcdef","cwd":"/tmp"}' | A agent start; reports s2abcdef)"
expect "nt doctor names the tool" 'summary +claude writes' "$(PATH="$T/tools:$PATH" A doctor 2>&1)"

expect "nt week compiles a week by hand; an empty one gets none" '^$' "$(B week last --json | json 'd["rel"]')"
expect "a weekly digest does not move" 'weekly digests' "$(A mv "$week" tech/ 2>&1)"
expect "its links to the weeks around it are not broken links" '^ok$' \
  "$(A doctor --json | json 'next(c for c in d["checks"] if c["check"] == "links")["state"]')"
finish
