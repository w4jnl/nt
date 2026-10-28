#!/usr/bin/env bash
# The iOS app's ssh, end to end in the simulator: a throwaway sshd on 127.0.0.1 run as you, with
# its own host key, serving a bare repository; the test makes a key as the phone does, puts it in
# the sshd's authorized_keys, then clones, pushes and fetches over ssh, and checks that a key the
# server does not know and a host key that changed are both refused.
#   scripts/ios-ssh-test.sh          (make ios-ssh-test)
# Requires: what make ios-test does, and /usr/sbin/sshd (no Remote Login needed).
set -euo pipefail
R=$(cd "$(dirname "$0")/.." && pwd)
D=$(mktemp -d "${TMPDIR:-/tmp}/nt-ssh.XXXXXX")
PORT=${NT_SSH_PORT:-22722}
trap '[ -f "$D/sshd.pid" ] && kill "$(cat "$D/sshd.pid")" 2>/dev/null; rm -rf "$D"' EXIT

ssh-keygen -q -t ecdsa -N '' -f "$D/host_ecdsa"
touch "$D/authorized_keys"
cat >"$D/sshd_config" <<CONF
Port $PORT
ListenAddress 127.0.0.1
HostKey $D/host_ecdsa
AuthorizedKeysFile $D/authorized_keys
PasswordAuthentication no
KbdInteractiveAuthentication no
UsePAM no
StrictModes no
PidFile $D/sshd.pid
CONF

# the remote: one note, pushed
git init -q --bare -b main "$D/remote.git"
git clone -q "$D/remote.git" "$D/seed" 2>/dev/null
mkdir -p "$D/seed/tech"
printf '# NAS\n\nbase\n' >"$D/seed/tech/nas.md"
git -C "$D/seed" add -A
git -C "$D/seed" -c user.name=mac -c user.email=mac@example.com -c commit.gpgsign=false commit -qm seed
git -C "$D/seed" push -q origin main

/usr/sbin/sshd -f "$D/sshd_config" -E "$D/sshd.log"
for _ in $(seq 50); do nc -z 127.0.0.1 "$PORT" 2>/dev/null && break; perl -e 'select(undef,undef,undef,0.1)'; done
nc -z 127.0.0.1 "$PORT" 2>/dev/null || { echo "ios-ssh-test.sh: sshd did not start:" >&2; cat "$D/sshd.log" >&2; exit 1; }

cd "$R/app/ios"
TEST_RUNNER_NT_SSH_URL="ssh://$(id -un)@127.0.0.1:$PORT$D/remote.git" TEST_RUNNER_NT_SSH_KEYS="$D/authorized_keys" \
  xcodebuild -project Nt.xcodeproj -scheme Nt -configuration Debug -derivedDataPath build/DerivedData \
  -destination "${IOS_SIM:-platform=iOS Simulator,name=iPhone 17}" test CODE_SIGN_IDENTITY=- \
  '-only-testing:NtTests/LibGitTests/overSSHWithThePhonesKey()' -collect-test-diagnostics never -quiet
[ "$(git -C "$D/remote.git" log -1 --format=%s)" = phone ] || { echo "ios-ssh-test.sh: the phone's commit did not reach the remote" >&2; exit 1; }
echo "ios-ssh-test.sh: cloned, pushed and fetched over ssh with the phone's key"
