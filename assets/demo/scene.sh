#!/usr/bin/env bash
# Builds the README recordings' scene: a notes repository with a week of journals, notes in a
# few folders that link each other, todos (one late), two agent reports and the week's digest,
# dated around today. Prints the folder; its config.toml is the machine config (NT_CONFIG).
set -euo pipefail
R=$(cd "$(dirname "$0")/../.." && pwd)
BIN=${BIN:-$R/bin/nt}
D=$(mktemp -d "${TMPDIR:-/tmp}/nt-demo.XXXX")
N=$D/notes
printf 'repo = "%s"\nhost = "mbp"\nweekly = true\ninline_images = "off"\n[sync]\nauto = false\n[theme]\nmode = "dark"\n' "$N" > "$D/config.toml"
export NT_CONFIG=$D/config.toml NT_STATE=$D/state
export GIT_AUTHOR_NAME=demo GIT_AUTHOR_EMAIL=demo@example.com GIT_COMMITTER_NAME=demo GIT_COMMITTER_EMAIL=demo@example.com
"$BIN" init "$N" >/dev/null
sed -i.bak 's/^pins = \[\]/pins = ["homelab", "work", "reading"]/' "$N/.nt/config.toml" && rm "$N/.nt/config.toml.bak"

# day N: the date N days from today, YYYY-MM-DD; day N fmt: in another strftime format
day() { python3 -c 'import datetime, sys; d = datetime.date.today() + datetime.timedelta(days=int(sys.argv[1])); print(d.strftime(sys.argv[2] if len(sys.argv) > 2 else "%Y-%m-%d"))' "$@"; }
note() { mkdir -p "$(dirname "$N/$1")"; cat > "$N/$1"; }

nas=homelab/$(day -6)-nas-setup.md
backups=homelab/$(day -5)-backups.md
network=homelab/$(day -3)-home-network.md
q4=work/$(day -4)-q4-planning.md
incident=work/$(day -1)-incident-review.md
ddia=reading/$(day -2)-designing-data-intensive-applications.md

note "$nas" <<EOF
---
title: NAS setup
date: $(day -6)
tags: [homelab, storage]
---

# NAS setup

Four disks in RAID-Z1, shared over NFS. Nightly [backups](../$backups) go to a second
box; the [home network](../$network) note has the VLANs.

| Disk | Size | Role |
|---|---|---|
| sda | 8 TB | data |
| sdb | 8 TB | data |
| sdc | 8 TB | data |
| sdd | 8 TB | parity |

## Mounting

\`\`\`sh
sudo mount -t nfs nas.lan:/tank /mnt/tank
\`\`\`

## Open

- [ ] Replace the fan in bay 2
- [x] Update the firmware
EOF

note "$backups" <<EOF
---
title: Backups
date: $(day -5)
tags: [homelab]
---

# Backups

restic, every night at 02:00, to the old Synology. Restores are tested monthly.

## Plan

1. Snapshot the datasets.
2. \`restic backup /mnt/tank --exclude-caches\`
3. Prune: keep 7 daily, 4 weekly, 12 monthly.

## Restore

\`\`\`sh
restic restore latest --target /tmp/restore --include /mnt/tank/photos
\`\`\`
EOF

note "$network" <<EOF
---
title: Home network
date: $(day -3)
---

# Home network

| VLAN | Name | Subnet |
|---|---|---|
| 10 | trusted | 10.0.10.0/24 |
| 20 | iot | 10.0.20.0/24 |
| 30 | guests | 10.0.30.0/24 |

The [NAS](../$nas) lives on VLAN 10.
EOF

note "$q4" <<EOF
---
title: Q4 planning
date: $(day -4)
tags: [work]
---

# Q4 planning

## Goals

- Ship the billing migration by mid-November.
- Halve the p95 latency of search.
- Retire the old cron host.

## Risks

> The migration window overlaps the holiday freeze.

Follow-up from the [incident review](../$incident).
EOF

note "$incident" <<EOF
---
title: Incident review
date: $(day -1)
tags: [work]
---

# Incident review

A config push took the API down for 11 minutes. Rollback worked; alerting was 4 minutes late.

## Actions

- [ ] Canary config pushes
- [ ] Page on error rate, not on uptime
EOF

note "$ddia" <<EOF
---
title: Designing Data-Intensive Applications
date: $(day -2)
tags: [reading]
---

# Designing Data-Intensive Applications

Chapter 5, replication. Leaderless replication trades read repair for availability; quorums
\`w + r > n\` do not guarantee linearizability.
EOF

note todo.md <<EOF
# Todo

## Inbox

- [ ] Renew the domain due:$(day 3)
- [ ] Replace the UPS battery due:$(day -2)
- [ ] Order patch cables
- [x] Patch the router due:$(day -1) done:$(day -1)
EOF

# journals: the week so far, each with a few lines and the notes made that day
journal() { # journal OFFSET "lines" "notes"
  local d; d=$(day "$1")
  note "daily/$(day "$1" %Y)/$d.md" <<EOF
---
title: $(day "$1" "%A %-d %B %Y")
date: $d
type: daily
---

# $(day "$1" "%A %-d %B %Y")

[← $(day $(($1 - 1)))]($(day $(($1 - 1))).md) · [$(day $(($1 + 1))) →]($(day $(($1 + 1))).md) · [agents](../../agents/$(day "$1" %Y)/$d/README.md)

## Log

$2

## Notes
$3
EOF
}
journal -6 "- 08:40 NAS disks arrived
- 21:15 Pool created, scrub running" "
- [NAS setup](../../$nas)"
journal -5 "- 09:05 First restic run: 1.2 TB in 3 h" "
- [Backups](../../$backups)"
journal -4 "- 10:00 Q4 planning with the team
- 16:30 Billing migration needs a freeze exception" "
- [Q4 planning](../../$q4)"
journal -3 "- 19:20 Moved the cameras to the iot VLAN" "
- [Home network](../../$network)"
journal -2 "- 22:10 Read chapter 5 of DDIA" "
- [Designing Data-Intensive Applications](../../$ddia)"
journal -1 "- 11:02 API down 11 min after a config push
- 14:30 Incident review written" "
- [Incident review](../../$incident)"
"$BIN" a "Stand-up: billing migration on track" >/dev/null
"$BIN" a "Fan in bay 2 is loud again" >/dev/null

# two agent sessions today, with their own accounts, and the day's digest
agents=agents/$(day 0 %Y)/$(day 0)
report() { # report FILE PROJECT AGENT STATUS SUMMARY
  note "$agents/$1" <<EOF
---
type: agent-report
schema: 1
date: $(day 0)
created: $(day 0)T09:12:00+01:00
updated: $(day 0)T10:40:00+01:00
ended: $(day 0)T10:40:00+01:00
agent: $3
host: mbp
session: $1
project: $2
status: $4
summary: $5
---

# $2: $5

## Done

- $5

## Log

- 09:12 started in ~/src/$2
- 10:40 ended
EOF
}
report mbp--claude-code--infra--1a2b3c4d.md infra claude-code done "Terraform for the new cron host"
report mbp--copilot-cli--website--5e6f7a8b.md website copilot-cli blocked "Waiting on the DNS change"
"$BIN" digest >/dev/null
git -C "$N" add -A >/dev/null && git -C "$N" commit -qm "the demo notes" >/dev/null
"$BIN" week >/dev/null
# the brand line's sync state, as a synced machine shows it
mkdir -p "$D/state"
now=$(date -u +%Y-%m-%dT%H:%M:%SZ)
printf '{"repo":"%s","state":"synced","branch":"main","ahead":0,"committed":0,"merged":false,"at":"%s","last_synced":"%s"}\n' "$N" "$now" "$now" > "$D/state/status.json"
echo "$D"
