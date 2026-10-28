package search

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestGoFindsWhatRgFinds: without rg, as on a phone, the same lines are found: smart case,
// markdown files only, hidden folders and temp files left out.
func TestGoFindsWhatRgFinds(t *testing.T) {
	root := t.TempDir()
	for p, body := range map[string]string{
		"homelab/nas.md":       "# NAS setup\n\nThe NAS is on VLAN 10.\n- [ ] Replace the fan in bay 2\n",
		"work/net.md":          "# Network\n\nvlan 20 is for guests; the nas sits on vlan 10\n",
		"notes.txt":            "vlan in a text file\n",
		".hidden/x.md":         "vlan in a hidden folder\n",
		"homelab/.nt-tmp-1.md": "vlan in a temp file\n",
		"daily/2026/a.md":      "fan in bay 2 is loud again\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, p), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lines := func(ms []Match) string {
		var out []string
		for _, m := range ms {
			out = append(out, m.Path+":"+strconv.Itoa(m.Line)+":"+m.Text)
		}
		sort.Strings(out)
		return strings.Join(out, "\n")
	}
	want := map[string]string{
		"vlan":       "homelab/nas.md:3:The NAS is on VLAN 10.\nwork/net.md:3:vlan 20 is for guests; the nas sits on vlan 10",
		"NAS":        "homelab/nas.md:1:# NAS setup\nhomelab/nas.md:3:The NAS is on VLAN 10.",
		"fan.*bay 2": "daily/2026/a.md:1:fan in bay 2 is loud again\nhomelab/nas.md:4:- [ ] Replace the fan in bay 2",
	}
	_, rgErr := exec.LookPath("rg")
	for q, w := range want {
		got, err := Go(context.Background(), root, q)
		if err != nil || lines(got) != w {
			t.Errorf("Go(%q) = %v\n%s\nwant\n%s", q, err, lines(got), w)
		}
		if rgErr == nil {
			rg, err := Rg(context.Background(), root, q)
			if err != nil || lines(rg) != lines(got) {
				t.Errorf("%q: rg finds\n%s\nGo finds\n%s", q, lines(rg), lines(got))
			}
		}
	}
	if m, err := Go(context.Background(), root, "VLAN"); err != nil || len(m) != 1 || len(m[0].Spans) != 1 {
		t.Errorf("a capital makes the search case-sensitive: %v %+v", err, m)
	}
	if _, err := Go(context.Background(), root, "["); err == nil {
		t.Error("a query that is no regular expression is not refused")
	}
}
