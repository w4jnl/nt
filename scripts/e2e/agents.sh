#!/usr/bin/env bash
# Agent reports: Claude Code and Copilot CLI hook payloads into reports, nt agent write, the
# digest, and 5 sessions reporting at once on two clones that sync all the while: every report
# arrives whole on both sides, with no conflict copy. Then the hook and skill installers.
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
claude() { printf '{"hook_event_name":"%s","session_id":"%s","cwd":"%s"%s}' "$1" "$2" "$P" "${3:-}"; }

# --- one Claude Code session, as its hooks call nt
claude SessionStart 00893aaf-19fa-41d2 | A agent start
claude Stop 00893aaf-19fa-41d2 ',"last_assistant_message":"## Result\n\nThe digest is built.\nDetails."' | A agent turn
R=$(find "$T/a/agents" -name 'a--claude-code--notetaking--00893aaf.md')
expect "a session start makes its report" 'notetaking--00893aaf\.md$' "$R"
expect "the report carries the session" 'status: in-progress' "$(cat "$R")"
expect "and the project's branch" 'git_branch: feature' "$(cat "$R")"
expect "a turn logs the first line of the answer" '^- [0-9:]{5} Result$' "$(cat "$R")"
(cd "$P/sub" && printf '## Done\n- the digest\n## Open\n- the hooks\n' | A agent write --summary "Digest done" >/dev/null)
expect "agent write from a subfolder fills the session's report" '## Open' "$(cat "$R")"
expect "and sets the summary in the title" '^# noteTaking: Digest done$' "$(cat "$R")"
claude SessionEnd 00893aaf-19fa-41d2 ',"reason":"prompt_input_exit"' | A agent end
expect "the end marks it done" 'status: done' "$(cat "$R")"
claude Stop 00893aaf-19fa-41d2 ',"last_assistant_message":"late"' | A agent turn
expect "a late turn does not reopen it" 'status: done' "$(cat "$R")"

# --- a Copilot CLI session: camelCase, no agent name in the payload
printf '{"sessionId":"7c1d0e2a","cwd":"%s"}' "$P" | A agent start --agent copilot-cli
printf '{"sessionId":"7c1d0e2a","cwd":"%s","reason":"error"}' "$P" | A agent end --agent copilot-cli
C=$(find "$T/a/agents" -name '*--copilot-cli--*7c1d0e2a.md')
expect "a Copilot session gets its report" 'status: abandoned' "$(cat "$C" 2>/dev/null)"

printf '{"session_id":"9a0b1c2d","cwd":"/tmp/probe"}' | NT_REPORT_TEMP= A agent start
expect "a throwaway session in /tmp gets no report" '^$' "$(find "$T/a/agents" -name '*9a0b1c2d*')"

rc=0; printf 'not json' | A agent turn || rc=$?
expect "a broken payload still exits 0" '^0$' "$rc"
expect "and is noted in agent.log" 'not JSON' "$(cat "$T/a.state/agent.log")"

A digest >/dev/null
D=$(dirname "$R")/README.md
expect "nt digest writes the day's README" '✓ done · claude-code · a · Digest done · \[report\]' "$(cat "$D")"
expect "it links the day's journal" '^\[← journal\]\(\.\./\.\./\.\./daily/' "$(cat "$D")"
before=$(shasum "$D")
A digest >/dev/null
expect "the same reports give the same digest" "^$before$" "$(shasum "$D")"

# --- 5 sessions report at once, 3 on A and 2 on B, while both clones sync
A sync >/dev/null 2>&1; B sync >/dev/null 2>&1
loop() { while [ ! -e "$T/stop" ]; do "$@" sync >/dev/null 2>&1 || true; done; }
loop A & la=$!
loop B & lb=$!
session() { # session CLONE ID
  local run=$1 id=$2 i
  claude SessionStart "$id" | $run agent start
  for i in $(seq 1 10); do claude Stop "$id" ",\"last_assistant_message\":\"turn $i of $id\"" | $run agent turn; done
  claude SessionEnd "$id" | $run agent end; }
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

# --- the installers write into a home of their own
H=$T/home; mkdir -p "$H/.claude"
printf '{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"/opt/flok hook claude"}]}]}}' > "$H/.claude/settings.json"
I() { HOME=$H XDG_DATA_HOME=$H/.local/share NT_CONFIG=$T/a.toml NT_STATE=$T/a.state "$BIN" "$@"; }
check() { I doctor --json 2>/dev/null | python3 -c 'import sys, json
print(*[c["state"] for c in json.load(sys.stdin)["checks"] if c["check"] == sys.argv[1]])' "$1"; }
expect "doctor asks for the hooks" '^warn$' "$(check hooks)"
I install --hooks --skill >/dev/null
expect "install --hooks adds the three hooks" '^3$' "$(grep -c 'nt agent \(start\|turn\|end\)' "$H/.claude/settings.json")"
expect "and keeps flok's" 'flok hook claude' "$(cat "$H/.claude/settings.json")"
expect "and writes Copilot's hooks file" 'agent turn --agent copilot-cli' "$(cat "$H/.copilot/hooks/nt.json")"
expect "install --skill links it for both agents" '^name: nt name: nt$' "$(sed -n 2p "$H/.claude/skills/nt/SKILL.md") $(sed -n 2p "$H/.agents/skills/nt/SKILL.md")"
expect "doctor finds the hooks" '^ok$' "$(check hooks)"
expect "and the skill" '^ok$' "$(check skill)"
I uninstall --hooks --skill >/dev/null
expect "uninstall takes nt's hooks out and leaves flok's" '^0 1$' "$(grep -c 'agent turn' "$H/.claude/settings.json") $(grep -c 'flok hook' "$H/.claude/settings.json")"
expect "and the skill" '^gone$' "$( [ -e "$H/.claude/skills/nt" ] && echo there || echo gone)"
finish
