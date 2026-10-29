package mdparse

import (
	"fmt"
	"strings"
	"testing"
)

// TestTags: the frontmatter's tags in each form notes write them, then the #tag words of the
// text; not a heading's #, a link's #anchor, a number, or a word in code.
func TestTags(t *testing.T) {
	for _, c := range []struct{ note, want string }{
		{"---\ntags: [Docker, \"homelab\"]\n---\n# A\n", "docker:2 homelab:2"},
		{"---\ntitle: A\ntags:\n  - work/q3\n  - '#nas'\ndate: 2026-10-28\n---\n", "work/q3:4 nas:5"},
		{"---\ntags:\n- one\n---\n", "one:3"},
		{"---\ntags: docker, nas\n---\n", "docker:2 nas:2"},
		{"# Title\n\nRestic runs at 02:00 #backup #homelab/nas.\n## Notes #docker\n", "backup:3 homelab/nas:3 docker:4"},
		{"See [x](a.md#setup), issue #12, `#code` and a#b.\n```\n#fenced\n```\n#Work/\n", "work:5"},
		{"tags: [not, frontmatter]\n", ""},
	} {
		var got []string
		for _, tag := range Tags([]byte(c.note)) {
			got = append(got, fmt.Sprintf("%s:%d", tag.Name, tag.Line))
		}
		if strings.Join(got, " ") != c.want {
			t.Errorf("Tags(%q) = %q, want %q", c.note, strings.Join(got, " "), c.want)
		}
	}
	for _, c := range []struct {
		name, q string
		want    bool
	}{{"work", "work", true}, {"work/q3", "work", true}, {"workshop", "work", false}, {"work", "work/q3", false}} {
		if HasTag(c.name, c.q) != c.want {
			t.Errorf("HasTag(%q, %q) = %v", c.name, c.q, !c.want)
		}
	}
}
