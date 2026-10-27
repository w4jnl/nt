package weekly

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/w4jnl/nt/internal/vault"
)

// summaryRepo is a repository where the machines home and work each wrote in the week of 26
// October 2026 (journal lines, a note, todos) and each ran an agent session.
func summaryRepo(t *testing.T) *summaryWorld {
	t.Helper()
	v := repo(t, map[string]string{
		"daily/2026/2026-10-26.md": "# Monday\n\n## Log\n\n## Notes\n",
		"todo.md":                  "# Todo\n\n## Inbox\n\n- [ ] Renew the domain due:2026-10-30\n",
		"work/2026-09-01-old.md":   "# Old work note\n",
	}, "2026-09-01 12:00")
	w := &summaryWorld{t: t, root: v.Root}
	lastWorld = w
	// home writes on Monday
	write(t, v.Root, map[string]string{
		"daily/2026/2026-10-26.md": "# Monday\n\n## Log\n\n- 08:40 NAS disks arrived\n\n## Notes\n",
		"home/2026-10-26-nas.md":   "---\ntitle: NAS setup\n---\n\n# NAS setup\n\nFour disks in RAID-Z1.\n",
		"todo.md":                  "# Todo\n\n## Inbox\n\n- [x] Renew the domain due:2026-10-30 done:2026-10-26\n",
	})
	git(t, v.Root, "2026-10-26 20:00", "add", "-A")
	git(t, v.Root, "2026-10-26 20:00", "commit", "-q", "-m", "nt: home · 3 files (daily, home)")
	// work writes on Tuesday, in the same journal and in an old note
	write(t, v.Root, map[string]string{
		"daily/2026/2026-10-26.md": "# Monday\n\n## Log\n\n- 08:40 NAS disks arrived\n- 16:00 Billing migration needs a freeze exception\n\n## Notes\n",
		"work/2026-09-01-old.md":   "# Old work note\n\nThe customer signed.\n",
	})
	git(t, v.Root, "2026-10-27 17:00", "commit", "-q", "-am", "nt: work · 2 files (daily, work)")
	for host, project := range map[string]string{"home": "homelab", "work": "billing"} {
		write(t, v.Root, map[string]string{"agents/2026/2026-10-27/" + host + "--claude-code--" + project + "--aaaa1111.md": "---\ntype: agent-report\nproject: " + project +
			"\nagent: claude-code\nhost: " + host + "\nstatus: done\nsummary: " + project + " work done\ncreated: 2026-10-27T09:00:00+01:00\n---\n\n# x\n\n## Done\n\n- " + project + " shipped\n\n## Decisions\n\n## Open\n\n## Log\n"})
	}
	return w
}

type summaryWorld struct {
	t    *testing.T
	root string
}

var lastWorld *summaryWorld

// openVault opens the repository the last summaryRepo made.
func openVault(t *testing.T) *vault.Vault {
	t.Helper()
	v, err := vault.Open(lastWorld.root, "test")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// fakeTool puts a stand-in for claude and copilot on PATH: it keeps its stdin, its arguments
// and NT_NO_REPORT in dir, and answers with a summary over a few lines, or fails when told to.
func fakeTool(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\ncat > \"" + dir + "/$(basename \"$0\").in\"\nprintf '%s\\n' \"$@\" > \"" + dir + "/$(basename \"$0\").args\"\n" +
		"echo \"$NT_NO_REPORT\" > \"" + dir + "/$(basename \"$0\").noreport\"\n" +
		"[ -f \"" + dir + "/fail\" ] && { echo 'not logged in' >&2; exit 1; }\n" +
		"printf '# Summary\\n\\nThe NAS got its disks.\\n- And the domain was renewed.\\n'\n"
	for _, name := range []string{"claude", "copilot"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func TestSummaryInputIsOneMachines(t *testing.T) {
	summaryRepo(t)
	v := openVault(t)
	home := Input(v, "home", at("2026-10-26 10:00"))
	for _, want := range []string{"written on the machine home", "Mon 08:40 NAS disks arrived", "done: Renew the domain",
		"NAS setup (home/2026-10-26-nas.md, new)", "Four disks in RAID-Z1.", "homelab, done with claude-code: homelab work done", "homelab shipped"} {
		if !strings.Contains(home, want) {
			t.Errorf("home's week does not say %q:\n%s", want, home)
		}
	}
	for _, not := range []string{"Billing", "billing", "customer", "title: NAS setup", "(todo.md"} {
		if strings.Contains(home, not) {
			t.Errorf("home's week holds %q, which is work's, or frontmatter:\n%s", not, home)
		}
	}
	work := Input(v, "work", at("2026-10-26 10:00"))
	for _, want := range []string{"Billing migration needs a freeze exception", "Old work note (work/2026-09-01-old.md, changed)", "The customer signed.", "billing shipped"} {
		if !strings.Contains(work, want) {
			t.Errorf("work's week does not say %q:\n%s", want, work)
		}
	}
	if strings.Contains(work, "NAS") {
		t.Errorf("work's week holds home's lines:\n%s", work)
	}
	if Input(v, "nobody", at("2026-10-26 10:00")) != "" || Input(v, "home", at("2026-10-19 10:00")) != "" {
		t.Error("a machine that wrote nothing, or a week with nothing, has an input")
	}
}

func TestSummarize(t *testing.T) {
	summaryRepo(t)
	v := openVault(t)
	v.Host = "home"
	tools := fakeTool(t)
	rel, err := Summarize(context.Background(), v, Tool{Name: "claude", Model: "haiku"}, at("2026-10-28 10:00"))
	if err != nil || rel != "weekly/2026/2026-W44-home.md" {
		t.Fatalf("Summarize = %q, %v", rel, err)
	}
	b, _ := os.ReadFile(v.Abs(rel))
	want := "---\ntitle: \"Week 44, 2026 · home\"\ndate: 2026-10-26\ntype: weekly-summary\nhost: home\ntool: claude\nmodel: haiku\n---\n\nSummary The NAS got its disks. And the domain was renewed.\n"
	if string(b) != want {
		t.Errorf("the summary file:\n%s\nwant:\n%s", b, want)
	}
	in, _ := os.ReadFile(filepath.Join(tools, "claude.in"))
	args, _ := os.ReadFile(filepath.Join(tools, "claude.args"))
	noReport, _ := os.ReadFile(filepath.Join(tools, "claude.noreport"))
	if !strings.Contains(string(in), "NAS disks arrived") || strings.Contains(string(in), "Billing") {
		t.Errorf("claude read on stdin:\n%s", in)
	}
	for _, a := range []string{"-p", "--tools", "--no-session-persistence", `{"disableAllHooks":true}`, "--model\nhaiku", "You write the weekly summary"} {
		if !strings.Contains(string(args), a) {
			t.Errorf("claude's arguments lack %q:\n%s", a, args)
		}
	}
	if strings.TrimSpace(string(noReport)) != "1" {
		t.Error("the tool runs without NT_NO_REPORT: nt's own hooks would report it as a session")
	}

	digest, _ := Text(v, at("2026-10-28 10:00"))
	if !strings.Contains(string(digest), "## Summary\n\n### home · claude\n\nSummary The NAS got its disks.") {
		t.Errorf("the digest does not show the summary:\n%s", digest)
	}

	// the work machine, with Copilot: the week goes in the prompt, and only work's
	v.Host = "work"
	if _, err := Summarize(context.Background(), v, Tool{Name: "copilot"}, at("2026-10-28 10:00")); err != nil {
		t.Fatal(err)
	}
	args, _ = os.ReadFile(filepath.Join(tools, "copilot.args"))
	if !strings.Contains(string(args), "Billing migration") || strings.Contains(string(args), "NAS disks") || !strings.Contains(string(args), "--silent") {
		t.Errorf("copilot's arguments:\n%s", args)
	}
	digest, _ = Text(v, at("2026-10-28 10:00"))
	if !strings.Contains(string(digest), "### home · claude") || !strings.Contains(string(digest), "### work · copilot") {
		t.Errorf("the digest does not show both summaries:\n%s", digest)
	}
}

func TestRefreshSummaries(t *testing.T) {
	summaryRepo(t)
	v := openVault(t)
	v.Host = "home"
	tools := fakeTool(t)
	state := t.TempDir()
	tuesday := at("2026-10-27 10:00")
	if got := Due(v, tuesday); len(got) != 0 {
		t.Errorf("a week not over yet is due: %v", got)
	}
	nextWeek := at("2026-11-03 10:00")
	if got := Due(v, nextWeek); len(got) != 1 || got[0].Format("2006-01-02") != "2026-10-26" {
		t.Fatalf("Due = %v, want the week of 26 October", got)
	}
	if err := os.WriteFile(filepath.Join(tools, "fail"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := RefreshSummaries(context.Background(), v, Tool{Name: "claude"}, state, nextWeek); err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("a failing tool: %v", err)
	}
	os.Remove(filepath.Join(tools, "fail"))
	os.Remove(filepath.Join(tools, "claude.in"))
	if got, err := RefreshSummaries(context.Background(), v, Tool{Name: "claude"}, state, nextWeek.Add(10*time.Minute)); err != nil || len(got) != 0 {
		t.Fatalf("tried again within the hour: %v, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(tools, "claude.in")); err == nil {
		t.Error("the tool ran again within the hour")
	}
	marks, _ := filepath.Glob(filepath.Join(state, "summary-*.tried"))
	for _, m := range marks { // an hour on
		old := time.Now().Add(-2 * time.Hour)
		os.Chtimes(m, old, old)
	}
	got, err := RefreshSummaries(context.Background(), v, Tool{Name: "claude"}, state, nextWeek.Add(2*time.Hour))
	if err != nil || strings.Join(got, " ") != "weekly/2026/2026-W44-home.md" {
		t.Fatalf("RefreshSummaries an hour later = %v, %v", got, err)
	}
	if got := Due(v, nextWeek); len(got) != 0 {
		t.Errorf("a week with its summary is still due: %v", got)
	}
}

func TestParagraph(t *testing.T) {
	if got := paragraph("## Week\n\n- one.\n* two\n> three\n- [ ] four"); got != "Week one. two three [ ] four" {
		t.Errorf("paragraph = %q", got)
	}
	long := strings.Repeat("A sentence. ", 200)
	if got := paragraph(long); len(got) > summaryMax || !strings.HasSuffix(got, ".") {
		t.Errorf("a long answer is cut to %d characters, ending %q", len(got), got[len(got)-5:])
	}
}
