package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/w4jnl/nt/internal/vault"
)

// A made-up vault with the shapes a real one has: dated and compact-dated names, folders with
// spaces, Hugo frontmatter, wikilinks with headings and labels, embeds by path and by name,
// relative and root-relative markdown links, code, Obsidian's own files and empty notes.
var fixture = map[string]string{
	".obsidian/app.json":                  "{}",
	".trash/Old.md":                       "# old\n",
	"Shell/.trash/x.md":                   "trash\n",
	"templates/hugo.md":                   "---\ntitle: {{title}}\n---\n",
	"Untitled.base":                       "x",
	"Untitled.canvas":                     "{}",
	"Untitled.md":                         "",
	"Untitled 1.md":                       "Has content.\n",
	"go.mod":                              "module x\n",
	"img/Pasted image 20230209.png":       "PNG",
	"img/Before.png":                      "PNG",
	"Tech Notes/Home NAS/iptables.png":    "PNG",
	"Tech Notes/Home NAS/Docker issue.md": "# Docker\n\n![[iptables.png]]\n",
	"Tech Notes/202301151524-Remapping Keys Mac.md": "Keys.\n",
	"Tech Notes/2023-01-20-CleanupStorscanDB.md":    "---\ntitle: CleanupStorscanDB\n---\n# Clean\n",
	"notes/2023-04-11.md":                           "# Testing\ncreated daily note\n",
	"notes/2026-10-26.md":                           "---\ntitle: 2026-10-26\ndate:  2026-10-26\ndraft: false\ntags: []\n---\n\n",
	"notes/2023-07-28 122105.md":                    "---\ntitle: 2023-07-28 122105\ndate:  2023-07-28\ndraft: false\n---\nA quick thought.\n",
	"notes/2023-07-31 Project Report.md": "---\ntitle: 2023-07-31 Project Report\ndate:  2023-07-31\ndraft: false\ntags: [done]\n---\n\n" +
		"# Report\n\n## build_report.ksh\n\n![[img/Before.png]]\n\n![[img/Pasted image 20230209.png|300]]\n",
	"notes/2023-08-01 Plan.md": "See [[notes/2023-07-31 Project Report#build_report.ksh|the report]] and [[202301151524-Remapping Keys Mac]].\n" +
		"Also [[Docker issue]] and [[missing note]] and [[#Local heading]].\n\n" +
		"```sh\necho '[[not a link]]'\n```\n\nInline `[[code]]` stays. ![Before](../img/Before.png) ![root](img/Before.png) " +
		"[web](https://example.com/a.md) [file](file://server/share)\n",
	"Shell/TODO List.md": "---\ndate: 2024-01-31\n---\n- [ ] item\n",
}

func setup(t *testing.T) (string, *vault.Vault) {
	t.Helper()
	src := filepath.Join(t.TempDir(), "vault")
	for p, s := range fixture {
		full := filepath.Join(src, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// not a git repository, so an undated note is dated by its modification time
	day := time.Date(2023, 8, 2, 12, 0, 0, 0, time.Local)
	if err := os.Chtimes(filepath.Join(src, "Tech Notes", "Home NAS", "Docker issue.md"), day, day); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "notes")
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	v, err := vault.Open(root, "mbp")
	if err != nil {
		t.Fatal(err)
	}
	return src, v
}

func dests(ms []Moved) map[string]string {
	m := map[string]string{}
	for _, x := range ms {
		m[x.From] = x.To
	}
	return m
}

func TestImportPlacesFiles(t *testing.T) {
	src, v := setup(t)
	rep, err := Obsidian(src, v, Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"notes/2023-07-31 Project Report.md":            "notes/2023-07-31-project-report.md",
		"notes/2023-07-28 122105.md":                    "notes/2023-07-28-122105.md",
		"Tech Notes/202301151524-Remapping Keys Mac.md": "tech-notes/2023-01-15-remapping-keys-mac.md",
		"Tech Notes/2023-01-20-CleanupStorscanDB.md":    "tech-notes/2023-01-20-cleanup-storscan-db.md",
		"Shell/TODO List.md":                            "shell/2024-01-31-todo-list.md",
		"img/Pasted image 20230209.png":                 "assets/pasted-image-20230209.png",
		"Tech Notes/Home NAS/iptables.png":              "tech-notes/home-nas/assets/iptables.png",
		"notes/2023-04-11.md":                           "daily/2023/2023-04-11.md",
	}
	got := dests(append(append(rep.Notes, rep.Journals...), rep.Assets...))
	for from, to := range want {
		if got[from] != to {
			t.Errorf("%s → %s, want %s", from, got[from], to)
		}
		if _, err := os.Stat(v.Abs(to)); err != nil {
			t.Errorf("%s was not written", to)
		}
	}
	if u := got["Untitled 1.md"]; !strings.HasPrefix(u, "inbox/") || !strings.HasSuffix(u, "-untitled-1.md") {
		t.Errorf("a root note with content should go to the inbox: %s", u)
	}
	skipped := map[string]bool{}
	for _, s := range rep.Skipped {
		skipped[s.Path] = true
	}
	for _, p := range []string{".obsidian/", ".trash/", "Shell/.trash/", "templates/", "Untitled.base", "Untitled.canvas", "Untitled.md", "go.mod", "notes/2026-10-26.md"} {
		if !skipped[p] {
			t.Errorf("%s should be left behind; skipped: %v", p, rep.Skipped)
		}
	}
}

func TestImportConvertsLinks(t *testing.T) {
	src, v := setup(t)
	rep, err := Obsidian(src, v, Options{})
	if err != nil {
		t.Fatal(err)
	}
	read := func(rel string) string {
		b, err := os.ReadFile(v.Abs(rel))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	plan := read("notes/2023-08-01-plan.md")
	for _, want := range []string{
		"[the report](2023-07-31-project-report.md#build_reportksh)",
		"[Remapping Keys Mac](../tech-notes/2023-01-15-remapping-keys-mac.md)",
		"[Docker issue](../tech-notes/home-nas/2023-08-02-docker-issue.md)",
		"[[missing note]]",
		"[Local heading](#local-heading)",
		"echo '[[not a link]]'",
		"Inline `[[code]]` stays.",
		"![Before](../assets/before.png) ![root](../assets/before.png)",
		"[web](https://example.com/a.md) [file](file://server/share)",
	} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan lacks %q:\n%s", want, plan)
		}
	}
	report := read("notes/2023-07-31-project-report.md")
	for _, want := range []string{
		"---\ntitle: Project Report\ndate: 2023-07-31\ntags: [done]\n---\n",
		"![Before](../assets/before.png)",
		"![Pasted image 20230209](../assets/pasted-image-20230209.png)",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report lacks %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "draft") {
		t.Errorf("draft: false should be dropped:\n%s", report)
	}
	if got := read("tech-notes/home-nas/2023-08-02-docker-issue.md"); !strings.Contains(got, "![iptables](assets/iptables.png)") {
		t.Errorf("an embed by name should find the image beside the note:\n%s", got)
	}
	if got := read("daily/2023/2023-04-11.md"); !strings.HasPrefix(got, "---\ndate: 2023-04-11\ntype: daily\n---\n# Testing") {
		t.Errorf("journal:\n%s", got)
	}
	if got := read("notes/2023-07-28-122105.md"); !strings.Contains(got, "title: 2023-07-28 122105\n") {
		t.Errorf("an untitled note keeps its time stamp as title:\n%s", got)
	}
	if len(rep.Unresolved) != 1 || rep.Unresolved[0].Text != "[[missing note]]" {
		t.Errorf("unresolved: %+v", rep.Unresolved)
	}
	if rep.Converted != 7 {
		t.Errorf("converted %d wikilinks and embeds, want 7", rep.Converted)
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	src, v := setup(t)
	rep, err := Obsidian(src, v, Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Notes) == 0 {
		t.Fatal("the dry run planned nothing")
	}
	entries, _ := os.ReadDir(v.Root)
	if len(entries) != 1 { // .git only
		t.Errorf("the dry run wrote %v", entries)
	}
}

func TestJournalClashBecomesANote(t *testing.T) {
	src, v := setup(t)
	if err := vault.WriteFile(v.Abs("daily/2023/2023-04-11.md"), []byte("# mine\n")); err != nil {
		t.Fatal(err)
	}
	rep, err := Obsidian(src, v, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := dests(rep.Notes)["notes/2023-04-11.md"]; got != "notes/2023-04-11-daily.md" {
		t.Errorf("clashing journal went to %q", got)
	}
	if b, _ := os.ReadFile(v.Abs("daily/2023/2023-04-11.md")); string(b) != "# mine\n" {
		t.Errorf("the existing journal was overwritten: %q", b)
	}
}

func TestSplitCamelAndAnchor(t *testing.T) {
	for in, want := range map[string]string{"CleanupStorscanDB": "Cleanup Storscan DB", "DockerServiceOverride": "Docker Service Override", "LDAP": "LDAP", "already spaced": "already spaced"} {
		if got := splitCamel(in); got != want {
			t.Errorf("splitCamel(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"build_report.ksh": "build_reportksh", "Setup & Use!": "setup--use", "Ünïcode Heading": "ünïcode-heading"} {
		if got := anchor(in); got != want {
			t.Errorf("anchor(%q) = %q, want %q", in, got, want)
		}
	}
}
