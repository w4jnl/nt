# Shared setup for the end-to-end suites, in flok's style: everything runs on an ISOLATED tmux
# server (tmux -L nt-e2e -f /dev/null) in a temp directory; the real tmux server is untouched.
set -Eeuo pipefail
unset TMUX TMUX_PANE NT_REPO NT_CONFIG NT_STATE
# commits work the same on a machine without a git identity (CI)
export GIT_AUTHOR_NAME=nt-e2e GIT_AUTHOR_EMAIL=e2e@example.com GIT_COMMITTER_NAME=nt-e2e GIT_COMMITTER_EMAIL=e2e@example.com
R=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
BIN=$R/bin/nt
go build -C "$R" -o "$BIN" ./cmd/nt
T=$(mktemp -d)
cd "$T"
# the suites' agent sessions run in $T, a temporary folder, whose sessions nt does not report
export NT_REPORT_TEMP=1
S() { tmux -L nt-e2e -f /dev/null "$@"; }
O() { tmux -L nt-e2e-outer -f /dev/null "$@"; }   # a second server around S, as flok's outer tmux
cleanup() { O kill-server 2>/dev/null || true; S kill-server 2>/dev/null || true; rm -rf "$T"; }
trap cleanup EXIT
S kill-server 2>/dev/null || true; O kill-server 2>/dev/null || true

pass=0; fail=0
ok()  { printf 'PASS  %s\n' "$1"; pass=$((pass+1)); }
bad() { printf 'FAIL  %s\n' "$1"; fail=$((fail+1)); }
# expect NAME PATTERN TEXT: PASS when TEXT matches PATTERN (grep -E), else FAIL with the text.
# The text goes through a file: a pipe into grep -q can end in SIGPIPE on large text.
expect() { printf '%s\n' "$3" > "$T/expect.txt"
  if grep -qE "$2" "$T/expect.txt"; then ok "$1"; else bad "$1 (pattern: $2)"; sed '/^ *$/d; s/^/      | /' "$T/expect.txt" | head -20; fi; }
capture() { S capture-pane -p -t "${1:-e2e}"; }
# expect_soon [-t SECS] NAME PATTERN CMD [ARGS…]: run CMD every 0.1 s until its output matches
# PATTERN (grep -E), then PASS; FAIL with the last output after SECS (default 5). A render that
# is late never fails a check; a state that never arrives does.
expect_soon() { local secs=5; if [ "$1" = -t ]; then secs=$2; shift 2; fi
  local name=$1 pat=$2 i; shift 2
  for i in $(seq 1 $(( secs * 10 ))); do
    "$@" > "$T/expect.txt" 2>/dev/null || true
    if grep -qE "$pat" "$T/expect.txt"; then ok "$name"; return 0; fi
    sleep 0.1
  done
  bad "$name (pattern: $pat, after ${secs}s)"; sed '/^ *$/d; s/^/      | /' "$T/expect.txt"; return 0; }
finish() { echo "== $pass passed, $fail failed =="; [ "$fail" -eq 0 ]; }
