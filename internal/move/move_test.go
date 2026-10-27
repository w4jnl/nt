package move

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/w4jnl/nt/internal/vault"
)

// repo is a small notes repository: a journal linking two notes, notes linking each other in
// both link styles, attachments one note owns and one two notes share, pins in the config.
func repo(t *testing.T) *vault.Vault {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		".nt/config.toml": "format = 1\ndefault_folder = \"inbox\"   # where nt n puts a note\n" +
			"pins = [\"tech\", \"tech/sub\", \"work\"]   # e.g. [\"tech\"]\n\n[folders]\ndaily = \"daily\"\nagents = \"agents\"\n",
		"daily/2026/2026-10-20.md": "# Day\n\n## Notes\n\n- [NAS setup](../../tech/2026-10-20-nas-setup.md)\n" +
			"- [Rack](../../tech/sub/2026-10-21-rack.md#power)\n",
		"tech/2026-10-20-nas-setup.md": "---\ntitle: NAS setup\ndate: 2026-10-20\n---\n\n# NAS setup\n\n" +
			"![rack](assets/2026-10-20-rack.png)\n![shared](assets/2026-10-20-shared.png)\n" +
			"See [backups](2026-10-20-backups.md#plan), [rack](sub/2026-10-21-rack.md), [web](https://x.y), [top](#nas-setup).\n" +
			"```\n[code](2026-10-20-backups.md)\n```\n[gone](2026-01-01-gone.md)\n[Note]: call Bob\n",
		"tech/2026-10-20-backups.md": "# Backups\n\n[NAS setup](2026-10-20-nas-setup.md) and [[2026-10-20-nas-setup|the NAS]]\n" +
			"![shared](assets/2026-10-20-shared.png)\n",
		"tech/assets/2026-10-20-rack.png":   "png",
		"tech/assets/2026-10-20-shared.png": "png",
		"tech/sub/2026-10-21-rack.md":       "# Rack\n\n## Power\n\n[NAS](../2026-10-20-nas-setup.md)\n",
		"inbox/2026-10-22-x.md":             "# X\n\n[tech](../tech/) and [rack][r]\n\n[r]: ../tech/sub/2026-10-21-rack.md\n",
	}
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	v, err := vault.Open(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func read(t *testing.T, v *vault.Vault, rel string) string {
	t.Helper()
	b, err := os.ReadFile(v.Abs(rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func has(t *testing.T, v *vault.Vault, rel string, want ...string) {
	t.Helper()
	got := read(t, v, rel)
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("%s does not hold %q:\n%s", rel, w, got)
		}
	}
}

func gone(t *testing.T, v *vault.Vault, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		if _, err := os.Stat(v.Abs(rel)); err == nil {
			t.Errorf("%s is still there", rel)
		}
	}
}

func TestMoveNote(t *testing.T) {
	v := repo(t)
	before := read(t, v, "daily/2026/2026-10-20.md")
	dry, err := Move(v, "tech/2026-10-20-nas-setup.md", "Work/", true)
	if err != nil {
		t.Fatal(err)
	}
	if read(t, v, "daily/2026/2026-10-20.md") != before {
		t.Fatal("a dry run wrote")
	}
	p, err := Move(v, "tech/2026-10-20-nas-setup.md", "Work/", false)
	if err != nil {
		t.Fatal(err)
	}
	if p.To != "work/2026-10-20-nas-setup.md" || dry.To != p.To {
		t.Errorf("to = %q (dry %q), want work/2026-10-20-nas-setup.md: a new folder is a slug", p.To, dry.To)
	}
	want := []Step{{"tech/2026-10-20-nas-setup.md", "work/2026-10-20-nas-setup.md"}, {"tech/assets/2026-10-20-rack.png", "work/assets/2026-10-20-rack.png"}}
	if len(p.Files) != 2 || p.Files[0] != want[0] || p.Files[1] != want[1] {
		t.Errorf("files = %v, want the note and the image only it uses: %v", p.Files, want)
	}
	if len(p.Links) != 4 || p.Links[0] != (Edit{"daily/2026/2026-10-20.md", 1}) || p.Links[3] != (Edit{"work/2026-10-20-nas-setup.md", 3}) {
		t.Errorf("links = %v", p.Links)
	}
	gone(t, v, "tech/2026-10-20-nas-setup.md", "tech/assets/2026-10-20-rack.png")
	has(t, v, "work/assets/2026-10-20-rack.png", "png")
	has(t, v, "tech/assets/2026-10-20-shared.png", "png")
	has(t, v, "work/2026-10-20-nas-setup.md",
		"![rack](assets/2026-10-20-rack.png)", "![shared](../tech/assets/2026-10-20-shared.png)",
		"[backups](../tech/2026-10-20-backups.md#plan)", "[rack](../tech/sub/2026-10-21-rack.md)",
		"[web](https://x.y)", "[top](#nas-setup)", "[code](2026-10-20-backups.md)",
		"[gone](2026-01-01-gone.md)", "[Note]: call Bob") // pointed at nothing, left as written
	has(t, v, "daily/2026/2026-10-20.md", "- [NAS setup](../../work/2026-10-20-nas-setup.md)\n", "- [Rack](../../tech/sub/2026-10-21-rack.md#power)\n")
	has(t, v, "tech/2026-10-20-backups.md", "[NAS setup](../work/2026-10-20-nas-setup.md) and [the NAS](../work/2026-10-20-nas-setup.md)")
	has(t, v, "tech/sub/2026-10-21-rack.md", "[NAS](../../work/2026-10-20-nas-setup.md)")

	if p, err := Move(v, "tech/2026-10-20-backups.md", "/", true); err != nil || p.To != "2026-10-20-backups.md" {
		t.Errorf("mv to / = %v, %v; want the root of the notes", p, err)
	}

	// into a folder that holds that name already
	if err := os.WriteFile(v.Abs("tech/2026-10-20-nas-setup.md"), []byte("# Other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if p, err := Move(v, "tech/2026-10-20-nas-setup.md", "work", false); err != nil || p.To != "work/2026-10-20-nas-setup-2.md" {
		t.Errorf("a taken name: to = %v, %v, want -2", p, err)
	}
}

func TestRenameNote(t *testing.T) {
	v := repo(t)
	p, err := Rename(v, "tech/2026-10-20-nas-setup.md", "Home NAS", false)
	if err != nil {
		t.Fatal(err)
	}
	if p.To != "tech/2026-10-20-home-nas.md" || len(p.Files) != 1 {
		t.Errorf("to = %q, files %v: the date stays, the slug follows the title, the images stay", p.To, p.Files)
	}
	has(t, v, "tech/2026-10-20-home-nas.md", "title: Home NAS\n", "\n# Home NAS\n", "![rack](assets/2026-10-20-rack.png)")
	has(t, v, "daily/2026/2026-10-20.md", "- [Home NAS](../../tech/2026-10-20-home-nas.md)\n")
	has(t, v, "tech/2026-10-20-backups.md", "[Home NAS](2026-10-20-home-nas.md) and [the NAS](2026-10-20-home-nas.md)")
	has(t, v, "tech/sub/2026-10-21-rack.md", "[NAS](../2026-10-20-home-nas.md)")

	// the same slug: only the title changes
	if p, err := Rename(v, "tech/2026-10-20-home-nas.md", "Home: NAS", false); err != nil || p.To != p.From {
		t.Fatalf("rename to the same slug: %v, %v", p, err)
	}
	has(t, v, "tech/2026-10-20-home-nas.md", "title: \"Home: NAS\"\n", "\n# Home: NAS\n")
	has(t, v, "daily/2026/2026-10-20.md", "- [Home: NAS](../../tech/2026-10-20-home-nas.md)\n")
}

func TestMoveFolder(t *testing.T) {
	v := repo(t)
	p, err := Move(v, "tech", "areas/", false)
	if err != nil {
		t.Fatal(err)
	}
	if p.To != "areas/tech" || !p.Folder || len(p.Files) != 5 || !p.Config {
		t.Errorf("plan = %+v", p)
	}
	gone(t, v, "tech")
	has(t, v, "areas/tech/assets/2026-10-20-shared.png", "png")
	has(t, v, "daily/2026/2026-10-20.md", "(../../areas/tech/2026-10-20-nas-setup.md)", "(../../areas/tech/sub/2026-10-21-rack.md#power)")
	has(t, v, "inbox/2026-10-22-x.md", "[tech](../areas/tech/)", "[r]: ../areas/tech/sub/2026-10-21-rack.md\n")
	has(t, v, "areas/tech/2026-10-20-nas-setup.md", "[backups](2026-10-20-backups.md#plan)", "[rack](sub/2026-10-21-rack.md)")
	has(t, v, ".nt/config.toml", `pins = ["areas/tech", "areas/tech/sub", "work"]   # e.g. ["tech"]`, `default_folder = "inbox"`)

	if p, err := Rename(v, "areas/tech", "My Tech", false); err != nil || p.To != "areas/my-tech" {
		t.Fatalf("folder rename: %v, %v", p, err)
	}
	has(t, v, "daily/2026/2026-10-20.md", "(../../areas/my-tech/2026-10-20-nas-setup.md)")

	if p, err := Rename(v, "inbox", "Inbox Two", false); err != nil || p.To != "inbox-two" {
		t.Fatalf("rename the default folder: %v, %v", p, err)
	}
	has(t, v, ".nt/config.toml", `default_folder = "inbox-two"   # where nt n puts a note`)
}

func TestRefuse(t *testing.T) {
	v := repo(t)
	for _, f := range []string{"tech/notes.txt", "tech/README.md"} {
		if err := os.WriteFile(v.Abs(f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct{ from, to, want string }{
		{"daily/2026/2026-10-20.md", "tech/", "stay where their date puts them"},
		{"daily", "old", "stay where their date puts them"},
		{"tech/2026-10-20-backups.md", "daily/", "holds only the journals"},
		{"tech/2026-10-20-backups.md", "agents/2026/", "holds only the agent reports"},
		{"tech/2026-10-20-backups.md", ".nt/", "names starting with . are not notes"},
		{"tech/2026-10-20-backups.md", ".hidden/", ""}, // a new name is a slug: hidden/
		{"tech/2026-10-20-backups.md", "tech/assets/", "assets folders hold attachments"},
		{"tech/assets", "work/", "they move with their notes"},
		{"tech/notes.txt", "work/", "is not a note"},
		{"tech", "tech/sub/", "cannot move into itself"},
		{"tech/README.md", "tech/readme", "differ only in case"},
		{"tech/2026-10-20-backups.md", "tech/2026-10-21-rack", ""},
		{"tech/sub/2026-10-21-rack.md", "tech/2026-10-20-backups.md", "is there already"},
		{"tech/2026-10-20-backups.md", "tech", "is in tech/ already"},
		{"tech/2026-10-20-backups.md", "../out/", "outside the notes repository"},
		{"nothing.md", "tech/", "no such note or folder"},
		{"", "tech/", "the root of the notes does not move"},
	} {
		_, err := Move(v, c.from, c.to, true)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("mv %s %s: %v", c.from, c.to, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("mv %s %s: %v, want %q", c.from, c.to, err, c.want)
		}
	}
	if _, err := Rename(v, "tech/2026-10-20-backups.md", "!!!", true); err == nil || !strings.Contains(err.Error(), "no letters or digits") {
		t.Errorf("a title with no slug: %v", err)
	}
}

func TestMkdir(t *testing.T) {
	v := repo(t)
	got, err := Mkdir(v, "TECH/New Stuff")
	if err != nil || got != "tech/new-stuff" {
		t.Fatalf("mkdir = %q, %v; want the folder that is there in its case, the new one a slug", got, err)
	}
	if _, err := os.Stat(v.Abs("tech/new-stuff/.gitkeep")); err != nil {
		t.Error("no .gitkeep: git would not keep the empty folder")
	}
	if !v.FolderEmpty("tech/new-stuff") {
		t.Error("a folder with only .gitkeep is not empty")
	}
	for in, want := range map[string]string{"tech/new-stuff": "there already", "daily/x": "holds only the journals", "": "name the folder", ".nt/x": "names starting with ."} {
		if _, err := Mkdir(v, in); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("mkdir %q: %v, want %q", in, err, want)
		}
	}
}

func TestRetitle(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"---\ntitle: Old\ntags: [a]\n---\n\n# Old\n\ntext\n", "---\ntitle: New\ntags: [a]\n---\n\n# New\n\ntext\n"},
		{"---\ndate: 2026-10-20\n---\n\n# 2026-10-20 Old\n", "---\ntitle: New\ndate: 2026-10-20\n---\n\n# New\n"}, // the title came from the heading
		{"---\ntitle: Old\n---\n\n# Something else\n", "---\ntitle: New\n---\n\n# Something else\n"},
		{"```\n# Old\n```\n# Old\n", "```\n# Old\n```\n# New\n"},
		{"just text\n", "# New\n\njust text\n"},
	} {
		if got := string(retitle([]byte(c.in), "Old", "New")); got != c.want {
			t.Errorf("retitle(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
