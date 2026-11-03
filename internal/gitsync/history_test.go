package gitsync

import (
	"testing"
	"time"
)

// TestHistoryNamesTheHostOfEachChange: git's history, read through the plumbing, says when a
// note last changed and which host's nt committed it, newest commit first; a merge commit is
// not a change of its own.
func TestHistoryNamesTheHostOfEachChange(t *testing.T) {
	w := newWorld(t)
	vs := w.seeded("mac", "phone")
	mac, phone := vs[0], vs[1]
	if st := sync(t, phone); st.State != "synced" {
		t.Fatalf("phone: %+v", st)
	}
	write(t, phone, "tech/nas.md", "# NAS\n\nfrom the phone\n")
	if st := sync(t, phone); st.State != "synced" {
		t.Fatalf("phone: %+v", st)
	}
	if st := sync(t, mac); st.State != "synced" { // a merge, by fast-forward or not
		t.Fatalf("mac: %+v", st)
	}
	changes, err := Changes(Exec(mac.Root))
	if err != nil {
		t.Fatal(err)
	}
	if c := changes["tech/nas.md"]; c.By != "phone" || time.Since(c.At) > time.Minute {
		t.Errorf("tech/nas.md: %+v, want the phone's change just now", c)
	}
	if c := changes["tech/gone.md"]; c.By != "mac" {
		t.Errorf("tech/gone.md: %+v, want the mac's seed", c)
	}
	entries, err := Exec(mac.Root).History()
	if err != nil || len(entries) != 2 {
		t.Fatalf("history: %d entries, %v; want the two commits, not the merge", len(entries), err)
	}
	if entries[0].Subject != "nt: phone · 1 file (tech)" || entries[1].Files[0] != ".gitattributes" {
		t.Errorf("history: %+v", entries)
	}
	if empty, err := Exec(t.TempDir()).History(); empty != nil || err != nil {
		t.Errorf("no repository: %v %v", empty, err)
	}
}

func TestHostOfAndParseHistory(t *testing.T) {
	for subject, want := range map[string]string{
		"nt: mac · 3 files (daily, agents)": "mac",
		"nt: work-mbp · 1 file (tech)":      "work-mbp",
		"seed":                              "",
		"Merge branch 'main'":               "",
	} {
		if got := HostOf(subject); got != want {
			t.Errorf("HostOf(%q) = %q, want %q", subject, got, want)
		}
	}
	entries := ParseHistory("\x001761900000\tnt: mac · 2 files (tech)\n\ntech/a.md\ntech/b.md\n\x001761800000\tseed\n\nREADME.md\n")
	if len(entries) != 2 || entries[0].At.Unix() != 1761900000 || len(entries[0].Files) != 2 || entries[1].Subject != "seed" || entries[1].Files[0] != "README.md" {
		t.Errorf("parsed %+v", entries)
	}
	if got := ParseHistory(""); got != nil {
		t.Errorf("nothing parses to %+v", got)
	}
}
