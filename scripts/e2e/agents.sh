#!/usr/bin/env bash
# Agent reports: flok's records of a session into its report (nt agent import), nt agent write,
# the digest, and 5 sessions reporting at once on two clones that sync all the while: every
# report arrives whole on both sides, with no conflict copy. Then nt's old hooks, which do
# nothing now and come out with nt uninstall --hooks, and the skill's installer.
source "$(dirname "$0")/lib.sh"

git init --quiet --bare -b main "$T/remote.git"
# clone A writes the digests, clone B does not; each machine has its own config and state
setup() { # setup NAME DIGEST
  printf 'repo = "%s"\nhost = "%s"\ndigest = %s\n[sync]\nauto = false\n' "$T/$1" "$1" "$2" > "$T/$1.toml"
  NT_CONFIG=$T/$1.toml NT_STATE=$T/$1.state "$BIN" init "$T/$1" >/dev/null; }
setup a true
git -C "$T/a" remote add origin "$T/remote.git"
A() { NT_CONFIG=$T/a.toml NT_STATE=$T/a.state "$BIN" "$@"; }
B() { NT_CONFIG=$T/b.toml NT_STATE=$T/b.state "$BIN" "$@"; }
A sync >/dev/null 2>&1
git clone --quiet "$T/remote.git" "$T/b"
printf 'repo = "%s"\nhost = "b"\ndigest = false\n[sync]\nauto = false\n' "$T/b" > "$T/b.toml"

# a project the sessions run in: a git repository on a branch
P=$T/work/noteTaking
mkdir -p "$P/sub" && git -C "$P" init --quiet -b feature && git -C "$P" commit --quiet --allow-empty -m first
at() { [ -n "$1" ] && printf '2026-10-29T%s:00+01:00' "$1"; }
turn() { # turn HH:MM TEXT: a turn that ended at HH:MM
  printf '{"started":"%s","ended":"%s","text":"%s","tools":{"Bash":3,"Read":1}}' "$(at "$1")" "$(at "$1")" "$2"; }
rec() { # rec AGENT SESSION UPDATED ENDED STATE TURNS LOG: flok's record, the whole session
  printf '{"version":1,"agent":"%s","session":"%s","cwd":"%s","branch":"feature","model":"claude-fable-5-1","created":"2026-10-29T13:47:44+01:00","updated":"%s","ended":"%s","end_reason":"%s","state":"%s","turns":%s,"tools":4,"waited_s":710,"log":[%s],"later":{"x":1}}' \
    "$1" "$2" "$P" "$(at "$3")" "$(at "$4")" "${4:+exit}" "$5" "$6" "$7"; }

# --- one Claude Code session, as flok sends it
expect "agent import --probe says the record version" '^1$' "$(A agent import --probe)"
rc=0; out=$(rec claude-code 00893aaf-19fa 13:47 "" working 0 "" | A agent import 2>&1) || rc=$?
expect "an import prints nothing and exits 0" '^0:$' "$rc:$out"
R=$(find "$T/a/agents/2026/2026-10-29" -name 'a--claude-code--notetaking--00893aaf.md' 2>/dev/null)
expect "it makes the report in the folder of the day it was created" 'notetaking--00893aaf\.md$' "$R"
expect "the report carries the session" 'status: in-progress' "$(cat "$R")"
expect "and the project's branch" 'git_branch: feature' "$(cat "$R")"
t1=$(turn 13:52 '## Result\n\nThe digest is built.\nDetails.')
blocked='{"started":"2026-10-29T14:00:00+01:00","events":[{"at":"2026-10-29T14:00:30+01:00","kind":"blocked","text":"permission: Bash"}]}'
rec claude-code 00893aaf-19fa 14:01 "" blocked 2 "$t1,$blocked" | A agent import
expect "a turn logs the first line of the answer" '^- 13:52 Result$' "$(cat "$R")"
expect "with its tools under it" '^  - Bash ×3, Read ×1$' "$(cat "$R")"
expect "a block in the running turn shows" '^  - 14:00 blocked: permission: Bash$' "$(cat "$R")"
expect "and the live status" 'status: blocked' "$(cat "$R")"
(cd "$P/sub" && printf '## Done\n- the digest\n## Open\n- the hooks\n' | A agent write --summary "Digest done" >/dev/null)
expect "agent write from a subfolder fills the session's report" '## Open' "$(cat "$R")"
expect "and sets the summary in the title" '^# noteTaking: Digest done$' "$(cat "$R")"
rec claude-code 00893aaf-19fa 14:06 14:06 done 2 "$t1,$(turn 14:05 Shipped.)" | A agent import
expect "the end ends it done" 'status: done' "$(cat "$R")"
expect "with the counts in the frontmatter" '^waited_minutes: 12$' "$(cat "$R")"
expect "and the agent's own sections kept" '## Open' "$(cat "$R")"
rec claude-code 00893aaf-19fa 14:09 14:06 working 3 "$t1,$(turn 14:05 Shipped.),$(turn 14:08 Late.)" | A agent import
expect "a late record does not reopen it" 'status: done' "$(cat "$R")"

# --- a Copilot CLI session closed in its first turn
rec copilot-cli 7c1d0e2a 13:50 13:50 done 1 '{"started":"2026-10-29T13:48:00+01:00"}' | A agent import
C=$(find "$T/a/agents" -name '*--copilot-cli--*7c1d0e2a.md')
expect "a Copilot session gets its report" 'status: abandoned' "$(cat "$C" 2>/dev/null)"

rc=0; printf '{"version":1,"agent":"claude-code","session":"tmp1","cwd":"/tmp/x"}' | NT_REPORT_TEMP= A agent import || rc=$?
expect "a throwaway session in /tmp is skipped, exit 0" '^0:$' "$rc:$(find "$T/a/agents" -name '*tmp1*')"
rc=0; out=$(printf '{"version":9,"agent":"a","session":"s"}' | A agent import 2>&1) || rc=$?
expect "a newer record is refused with exit 2 and one line" '^2:nt: agent import: the record is newer' "$rc:$out"

# --- nt's old hooks: on a machine that still has them they do nothing, without a word
rc=0; out=$(printf '{"session_id":"9a0b1c2d","cwd":"%s","last_assistant_message":"from the hook"}' "$P" | A agent turn 2>&1) || rc=$?
expect "an old hook exits 0 and says nothing" '^0:$' "$rc:$out"
expect "and writes no report" '^$' "$(find "$T/a/agents" -name '*9a0b1c2d*')"

A digest 2026-10-29 >/dev/null
D=$(dirname "$R")/README.md
expect "nt digest writes the day's README" '✓ done · claude-code · a · Digest done · \[report\]' "$(cat "$D")"
expect "it links the day's journal" '^\[← journal\]\(\.\./\.\./\.\./daily/' "$(cat "$D")"
before=$(shasum "$D")
A digest 2026-10-29 >/dev/null
expect "the same reports give the same digest" "^$before$" "$(shasum "$D")"

# --- 5 sessions report at once, 3 on A and 2 on B, while both clones sync
A sync >/dev/null 2>&1; B sync >/dev/null 2>&1
loop() { while [ ! -e "$T/stop" ]; do "$@" sync >/dev/null 2>&1 || true; done; }
loop A & la=$!
loop B & lb=$!
session() { # session CLONE ID: ten turns, a record after each, then the end
  local run=$1 id=$2 i log=""
  for i in $(seq 1 10); do
    log="${log:+$log,}$(turn "14:$(printf %02d "$i")" "turn $i of $id")"
    rec claude-code "$id" "14:$(printf %02d "$i")" "" working "$i" "$log" | $run agent import
  done
  rec claude-code "$id" 14:30 14:30 done 10 "$log" | $run agent import; }
session A s1aaaaaa & p1=$!; session A s2aaaaaa & p2=$!; session A s3aaaaaa & p3=$!
session B s4bbbbbb & p4=$!; session B s5bbbbbb & p5=$!
wait $p1 $p2 $p3 $p4 $p5
touch "$T/stop"; wait $la $lb
for i in 1 2; do A sync >/dev/null 2>&1; B sync >/dev/null 2>&1; done
for id in s1aaaaaa s2aaaaaa s3aaaaaa s4bbbbbb s5bbbbbb; do
  for side in a b; do
    f=$(find "$T/$side/agents" -name "*--$id.md")
    expect "$id is whole on $side" '^12 done$' "$( [ -n "$f" ] && echo "$(grep -c '^- [0-9:]\{5\} ' "$f") $(sed -n 's/^status: //p' "$f")")"
  done
done
expect "no conflict copies" '^0$' "$(find "$T/a" "$T/b" -name '*.conflict-*' | wc -l | tr -d ' ')"
expect "valid frontmatter everywhere" '^0$' "$(for f in $(find "$T/a/agents" -name '*--*.md'); do head -1 "$f" | grep -qx -- '---' && grep -q '^type: agent-report$' "$f" || echo "$f"; done | wc -l | tr -d ' ')"
A sync >/dev/null 2>&1
expect "the digest machine lists B's sessions too" 's5bbbbbb' "$(cat "$D")"
B sync >/dev/null 2>&1
expect "and B gets the digest by sync" 's5bbbbbb' "$(cat "$T/b/${D#"$T/a/"}")"
expect "doctor says flok imports the reports" '^ok$' "$(A doctor --json 2>/dev/null | python3 -c 'import sys, json
print(*[c["state"] for c in json.load(sys.stdin)["checks"] if c["check"] == "reports"])')"

# --- nt's old hooks come out of a home of their own; the skill goes in and out
H=$T/home; mkdir -p "$H/.claude" "$H/.copilot/hooks"
printf '{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"/opt/flok hook claude"},{"type":"command","command":"/usr/local/bin/nt agent turn","timeout":10}]}],"SessionStart":[{"hooks":[{"type":"command","command":"/usr/local/bin/nt agent start","timeout":10}]}]}}' > "$H/.claude/settings.json"
printf '{"version":1,"hooks":{"agentStop":[{"type":"command","bash":"/usr/local/bin/nt agent turn --agent copilot-cli"}]}}' > "$H/.copilot/hooks/nt.json"
I() { HOME=$H XDG_DATA_HOME=$H/.local/share NT_CONFIG=$T/a.toml NT_STATE=$T/i.state "$BIN" "$@"; }
check() { I doctor --json 2>/dev/null | python3 -c 'import sys, json
print(*[c["state"] for c in json.load(sys.stdin)["checks"] if c["check"] == sys.argv[1]])' "$1"; }
expect "doctor warns about nt's old hooks" '^warn$' "$(check hooks)"
expect "and that no session gets a report here" '^warn$' "$(check reports)"
rc=0; out=$(I install --hooks 2>&1) || rc=$?
expect "install --hooks says flok writes the reports" '^2:nt: install: .*flok writes the session reports' "$rc:$out"
I uninstall --hooks >/dev/null
expect "uninstall --hooks takes nt's out and leaves flok's" '^0 1$' "$(grep -c 'nt agent' "$H/.claude/settings.json") $(grep -c 'flok hook' "$H/.claude/settings.json")"
expect "and Copilot's hooks file" '^gone$' "$( [ -e "$H/.copilot/hooks/nt.json" ] && echo there || echo gone)"
expect "doctor finds none left" '^$' "$(check hooks)"
I install --skill >/dev/null
expect "install --skill links it for both agents" '^name: nt name: nt$' "$(sed -n 2p "$H/.claude/skills/nt/SKILL.md") $(sed -n 2p "$H/.agents/skills/nt/SKILL.md")"
expect "doctor finds the skill" '^ok$' "$(check skill)"
I uninstall --skill >/dev/null
expect "uninstall --skill takes it out" '^gone$' "$( [ -e "$H/.claude/skills/nt" ] && echo there || echo gone)"
finish
