package mdparse

import (
	"strings"
	"testing"
)

func TestParseReferenceDefinitions(t *testing.T) {
	d := Parse([]byte("# A\n\nSee [the NAS][nas].\n\n[nas]: ../tech/nas.md \"NAS\"\n[^1]: a footnote\n"))
	if len(d.Links) != 1 || d.Links[0].Dest != "../tech/nas.md" || d.Links[0].Text != "nas" || d.Links[0].Line != 5 {
		t.Fatalf("links = %+v, want the definition of nas on line 5", d.Links)
	}
}

func TestRewrite(t *testing.T) {
	note := strings.Join([]string{
		"---",
		"title: [a](x.md)", // frontmatter: left alone
		"---",
		"- [Old](x.md) and ![pic](assets/p.png \"Title\") and [web](https://x.y)",
		"`[code](x.md)` but [x](<x.md#setup>)",
		"```",
		"[fenced](x.md)",
		"```",
		"[[x|Label]] and ![[x#Part]]",
		`<p align="center"><img src="assets/p.png" width="200"></p>`,
		"[ref]: x.md",
		"[^1]: x.md",
	}, "\n")
	out, n := Rewrite([]byte(note), func(l *Link) bool {
		switch {
		case l.Dest == "x.md" && l.Text == "Old":
			l.Text, l.Dest = "New [1]", "../y.md"
		case strings.HasPrefix(l.Dest, "x") || l.Dest == "assets/p.png":
			l.Dest = "../" + l.Dest
		default:
			return false
		}
		return true
	})
	want := strings.Join([]string{
		"---",
		"title: [a](x.md)",
		"---",
		`- [New \[1\]](../y.md) and ![pic](../assets/p.png "Title") and [web](https://x.y)`,
		"`[code](x.md)` but [x](../x.md#setup)",
		"```",
		"[fenced](x.md)",
		"```",
		"[Label](../x) and ![x#Part](../x#Part)",
		`<p align="center"><img src="../assets/p.png" width="200"></p>`,
		"[ref]: ../x.md",
		"[^1]: x.md",
	}, "\n")
	if string(out) != want {
		t.Errorf("Rewrite:\n%s\nwant:\n%s", out, want)
	}
	if n != 7 {
		t.Errorf("n = %d, want 7", n)
	}
	if same, n := Rewrite([]byte(note), func(*Link) bool { return false }); string(same) != note || n != 0 {
		t.Errorf("a rewrite that changes nothing changed %d links:\n%s", n, same)
	}
}

func TestAnchor(t *testing.T) {
	for in, want := range map[string]string{"build_report.ksh": "build_reportksh", "Setup & Use!": "setup--use", "Ünïcode Heading": "ünïcode-heading"} {
		if got := Anchor(in); got != want {
			t.Errorf("Anchor(%q) = %q, want %q", in, got, want)
		}
	}
}
