package cli

import (
	"strconv"
	"strings"
	"testing"

	"github.com/w4jnl/nt/internal/todo"
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
		it, err := matchTodo(items, tc.match, tc.want)
		got := it.Path + ":" + strconv.Itoa(it.Line)
		if err != nil {
			got = err.Error()
		}
		if !strings.HasPrefix(got, tc.at) {
			t.Errorf("matchTodo(%q) = %q, want %q", tc.match, got, tc.at)
		}
	}
}
