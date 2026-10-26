#!/usr/bin/env bash
# Runs every suite in scripts/e2e and fails when any of them does.
set -u
cd "$(dirname "$0")"
rc=0
for s in *.sh; do
  case $s in lib.sh | run-all.sh) continue ;; esac
  echo "== $s"
  bash "$s" || rc=1
done
exit $rc
