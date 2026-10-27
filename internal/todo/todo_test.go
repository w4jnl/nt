package todo

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/w4jnl/nt/internal/vault"
)

func newVault(t *testing.T) *vault.Vault {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	v, err := vault.Open(root, "mbp")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func write(t *testing.T, v *vault.Vault, rel, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(v.Abs(rel)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(v.Abs(rel), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, v *vault.Vault, rel string) string {
	t.Helper()
	b, err := os.ReadFile(v.Abs(rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func day(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

func TestParse(t *testing.T) {
	doc := strings.Join([]string{
		"---",
		"title: - [ ] not a todo",
		"---",
		"# Todo",
		"- [ ] Renew passport due:2026-11-03",
		"  * [x] nested, done done:2026-10-27",
		"+ [X] capital x",
		"1. [ ] numbered",
		"## Work",
		"- [ ] due:2026-10-30 date first",
		"- [ ]",
		"- [ ]no space is not a task",
		"-[ ] no space after the bullet",
		"- [ ] due:soon stays in the text",
		"```",
		"- [ ] in a code block",
		"```",
		"~~~md",
		"- [ ] in another",
		"~~~",
		"- [x]  [link](x.md) after two spaces\r",
	}, "\n")
	got := Parse("todo.md", []byte(doc))
	want := []Item{
		{Line: 5, Text: "Renew passport", Due: "2026-11-03", Section: "Todo"},
		{Line: 6, Text: "nested, done", Done: true, DoneOn: "2026-10-27", Section: "Todo"},
		{Line: 7, Text: "capital x", Done: true, Section: "Todo"},
		{Line: 8, Text: "numbered", Section: "Todo"},
		{Line: 10, Text: "date first", Due: "2026-10-30", Section: "Work"},
		{Line: 14, Text: "due:soon stays in the text", Section: "Work"},
		{Line: 21, Text: "[link](x.md) after two spaces", Done: true, Section: "Work"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d todos, want %d: %+v", len(got), len(want), got)
	}
	lines := strings.Split(doc, "\n")
	for i, w := range want {
		w.Path, w.Raw = "todo.md", lines[w.Line-1]
		if !reflect.DeepEqual(got[i], w) {
			t.Errorf("todo %d:\n got %+v\nwant %+v", i, got[i], w)
		}
	}
}

func TestParseWhen(t *testing.T) {
	today := day("2026-10-27") // a Tuesday
	for _, tc := range []struct{ in, want string }{
		{"2026-11-03", "2026-11-03"},
		{"today", "2026-10-27"},
		{"Tomorrow", "2026-10-28"},
		{"fri", "2026-10-30"},
		{"friday", "2026-10-30"},
		{"tue", "2026-11-03"}, // today is Tuesday: the next one
		{"mon", "2026-11-02"},
		{"+3d", "2026-10-30"},
		{"+2w", "2026-11-10"},
		{"+1m", "2026-11-27"},
		{"none", ""},
		{"", ""},
	} {
		got, err := ParseWhen(tc.in, today)
		if err != nil || got != tc.want {
			t.Errorf("ParseWhen(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	for _, bad := range []string{"soon", "2026-13-01", "+3y", "3d", "next friday"} {
		if _, err := ParseWhen(bad, today); err == nil {
			t.Errorf("ParseWhen(%q) gave no error", bad)
		}
	}
}

func TestWhen(t *testing.T) {
	today := day("2026-10-27")
	for _, tc := range []struct {
		due, want string
		late      bool
	}{
		{"2026-10-25", "2d late", true},
		{"2026-10-27", "today", false},
		{"2026-10-28", "tomorrow", false},
		{"2026-10-30", "fri", false},
		{"2026-11-03", "3 nov", false},
		{"2027-01-05", "5 jan 2027", false},
	} {
		if got, late := When(tc.due, today); got != tc.want || late != tc.late {
			t.Errorf("When(%s) = %q %v, want %q %v", tc.due, got, late, tc.want, tc.late)
		}
	}
	if got := Ago("2026-10-26", today); got != "yesterday" {
		t.Errorf("Ago: %q", got)
	}
}

func TestAdd(t *testing.T) {
	v := newVault(t)
	now := day("2026-10-27").Add(10 * time.Hour)
	it, err := Add(v, "", "Renew  passport", "fri", now)
	if err != nil {
		t.Fatal(err)
	}
	if it.Line != 5 || it.Due != "2026-10-30" {
		t.Errorf("added %+v", it)
	}
	if _, err := Add(v, "", "Call the plumber due:tomorrow", "", now); err != nil {
		t.Fatal(err)
	}
	want := "# Todo\n\n## Inbox\n\n- [ ] Renew passport due:2026-10-30\n- [ ] Call the plumber due:2026-10-28\n"
	if got := read(t, v, File); got != want {
		t.Errorf("todo.md:\n%s\nwant:\n%s", got, want)
	}

	// other sections are the person's; new todos still go to the end of Inbox
	write(t, v, File, "# Todo\n\n## Inbox\n\n- [ ] one\n\n## Work\n\n- [ ] two\n")
	if it, err = Add(v, "/", "three", "", now); err != nil || it.Line != 6 {
		t.Fatalf("add: %+v %v", it, err)
	}
	want = "# Todo\n\n## Inbox\n\n- [ ] one\n- [ ] three\n\n## Work\n\n- [ ] two\n"
	if got := read(t, v, File); got != want {
		t.Errorf("todo.md:\n%s\nwant:\n%s", got, want)
	}

	// a folder's own list
	if it, err = Add(v, "work/../work/", "four", "", now); err != nil || it.Path != "work/todo.md" || it.Line != 5 {
		t.Fatalf("add in a folder: %+v %v", it, err)
	}
	if got := read(t, v, "work/todo.md"); got != "# Todo\n\n## Inbox\n\n- [ ] four\n" {
		t.Errorf("work/todo.md:\n%s", got)
	}
	for _, folder := range []string{"agents", "agents/2026", ".nt", "work/.hidden"} {
		if _, err := Add(v, folder, "x", "", now); err == nil {
			t.Errorf("Add in %s gave no error", folder)
		}
	}
	for _, bad := range []struct{ text, due string }{{"", ""}, {"due:fri", ""}, {"x", "soon"}, {"x due:soon", ""}} {
		if _, err := Add(v, "", bad.text, bad.due, now); err == nil {
			t.Errorf("Add(%q, %q) gave no error", bad.text, bad.due)
		}
	}
}

func TestChange(t *testing.T) {
	v := newVault(t)
	write(t, v, "work/a.md", "# A\n\n  * [ ] Review the report due:2026-10-30\n- [ ] other\n")
	items := Parse("work/a.md", []byte(read(t, v, "work/a.md")))
	it := items[0]

	done, err := Check(v, it, day("2026-10-27"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "  * [x] Review the report due:2026-10-30 done:2026-10-27"; done.Raw != want || !done.Done || done.DoneOn != "2026-10-27" {
		t.Errorf("checked: %+v", done)
	}
	open, err := Uncheck(v, done)
	if err != nil {
		t.Fatal(err)
	}
	if want := "  * [ ] Review the report due:2026-10-30"; open.Raw != want {
		t.Errorf("unchecked: %q", open.Raw)
	}
	moved, err := SetDue(v, open, "2026-11-02")
	if err != nil || moved.Raw != "  * [ ] Review the report due:2026-11-02" {
		t.Errorf("due: %q %v", moved.Raw, err)
	}

	// a done todo keeps done: last when its due date changes
	d, _ := Check(v, moved, day("2026-10-28"))
	d, err = SetDue(v, d, "")
	if err != nil || d.Raw != "  * [x] Review the report done:2026-10-28" {
		t.Errorf("due cleared on a done todo: %q %v", d.Raw, err)
	}

	// a line added above: the todo is found by its text
	write(t, v, "work/a.md", "# A\n\nnew line\n"+strings.TrimPrefix(read(t, v, "work/a.md"), "# A\n\n"))
	if _, err := Uncheck(v, d); err != nil {
		t.Errorf("after a line was added above: %v", err)
	}
	// the line changed: nothing is written
	before := read(t, v, "work/a.md")
	if _, err := Check(v, d, day("2026-10-29")); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Errorf("a changed line: %v", err)
	}
	if read(t, v, "work/a.md") != before {
		t.Error("the file changed although the todo was not found")
	}

	items = Parse("work/a.md", []byte(before))
	if err := Remove(v, items[1]); err != nil {
		t.Fatal(err)
	}
	if got, want := read(t, v, "work/a.md"), "# A\n\nnew line\n  * [ ] Review the report\n"; got != want {
		t.Errorf("after remove:\n%q\nwant\n%q", got, want)
	}
}

func TestCollectAndSort(t *testing.T) {
	v := newVault(t)
	write(t, v, File, "# Todo\n\n## Inbox\n\n- [ ] undated a\n- [ ] late due:2026-10-20\n- [x] done early done:2026-10-01\n- [ ] undated b\n")
	write(t, v, "work/2026-10-01-x.md", "- [ ] old undated\n- [ ] soon due:2026-10-28\n")
	write(t, v, "work/todo.md", "- [ ] work list\n")
	write(t, v, "home/todo.md", "- [ ] home list\n")
	write(t, v, "work/2026-10-20-y.md", "- [ ] newer undated\n- [x] done late done:2026-10-26\n- [x] done, no date\n")
	write(t, v, "agents/2026/2026-10-27/r.md", "- [ ] an agent's open item\n")
	write(t, v, "work/2026-10-20-y.conflict-mbp-20261027.md", "- [ ] a conflict copy\n")
	write(t, v, ".nt/templates/note.md", "- [ ] a template\n")
	items := Collect(v)
	Sort(items)
	var got []string
	for _, it := range items {
		got = append(got, it.Text)
	}
	want := []string{"late", "soon", "undated a", "undated b", "home list", "work list", "newer undated", "old undated", "done late", "done early", "done, no date"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order:\n got %q\nwant %q", got, want)
	}
}
