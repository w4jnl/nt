package ops

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/w4jnl/nt/internal/todo"
	"github.com/w4jnl/nt/internal/vault"
)

func TestMatchTodo(t *testing.T) {
	items := []todo.Item{
		{Path: "todo.md", Line: 5, Text: "Renew passport"},
		{Path: "todo.md", Line: 6, Text: "Pay", Done: true},
		{Path: "work/a.md", Line: 3, Text: "Pay the invoice"},
		{Path: "work/a.md", Line: 4, Text: "Pay the plumber"},
		{Path: "work/a.md", Line: 7, Text: "Pay"},
	}
	open := func(it todo.Item) bool { return !it.Done }
	done := func(it todo.Item) bool { return it.Done }
	for _, tc := range []struct {
		match string
		want  func(todo.Item) bool
		at    string // path:line, or the start of the error
	}{
		{"passport", open, "todo.md:5"},
		{"PAY THE INVOICE", open, "work/a.md:3"},
		{"todo.md:6", open, "todo.md:6"}, // path:line names any todo
		{"pay", open, "work/a.md:7"},     // three hold it, one is it
		{"pay", done, "todo.md:6"},
		{"pay the", open, "2 todos match"},
		{"rent", open, "no todo matches"},
		{"", open, "name the todo"},
		{"x.md:9", open, "no todo at x.md:9"},
	} {
		it, err := MatchTodo(items, tc.match, tc.want)
		got := it.Path + ":" + strconv.Itoa(it.Line)
		if err != nil {
			got = err.Error()
		}
		if !strings.HasPrefix(got, tc.at) {
			t.Errorf("MatchTodo(%q) = %q, want %q", tc.match, got, tc.at)
		}
	}
}

func TestTodoJSONSaysWhen(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	v, err := vault.Open(root, "mbp")
	if err != nil {
		t.Fatal(err)
	}
	v.Now = func() time.Time { return time.Date(2026, 10, 28, 12, 0, 0, 0, time.Local) } // a Wednesday
	for _, tc := range []struct {
		it   todo.Item
		when string
		late bool
	}{
		{todo.Item{Text: "UPS battery", Due: "2026-10-26"}, "2d late", true},
		{todo.Item{Text: "Domain", Due: "2026-10-30"}, "fri", false},
		{todo.Item{Text: "Cables"}, "", false},
		{todo.Item{Text: "Router", Done: true, DoneOn: "2026-10-27"}, "yesterday", false},
	} {
		got := TodoOf(v, tc.it)
		if got.When != tc.when || got.Late != tc.late {
			t.Errorf("%s: when %q late %v, want %q %v", tc.it.Text, got.When, got.Late, tc.when, tc.late)
		}
	}
}
