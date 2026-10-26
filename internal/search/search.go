// Package search runs ripgrep over a notes repository: smart case, markdown files only, temp
// files left out. nt s and the browser's live search share it.
package search

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Match is one line rg found.
type Match struct {
	Path  string   `json:"path"`
	Line  int      `json:"line"`
	Text  string   `json:"text"`
	Spans [][2]int `json:"-"` // byte ranges of Text that matched
}

// ErrNoRg says ripgrep is missing, with the fix.
var ErrNoRg = errors.New("ripgrep (rg) is not installed: brew install ripgrep")

// Rg searches the repository at root for query. No match is no error.
func Rg(ctx context.Context, root, query string) ([]Match, error) {
	if _, err := exec.LookPath("rg"); err != nil {
		return nil, ErrNoRg
	}
	cmd := exec.CommandContext(ctx, "rg", "--json", "--smart-case", "--max-columns", "400",
		"--glob", "*.md", "--glob", "!.nt-tmp-*", "--", query, ".")
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	var ee *exec.ExitError
	if err != nil && !(errors.As(err, &ee) && ee.ExitCode() == 1) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("rg: %s", strings.TrimSpace(stderr.String()))
	}
	return Parse(out), nil
}

// Parse reads rg --json output: one object per line; the "match" ones carry a path, a line and
// the byte spans that matched.
func Parse(out []byte) []Match {
	var res []Match
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var ev struct {
			Type string `json:"type"`
			Data struct {
				Path       struct{ Text string } `json:"path"`
				Lines      struct{ Text string } `json:"lines"`
				LineNumber int                   `json:"line_number"`
				Submatches []struct {
					Start int `json:"start"`
					End   int `json:"end"`
				} `json:"submatches"`
			} `json:"data"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil || ev.Type != "match" {
			continue
		}
		m := Match{Path: strings.TrimPrefix(ev.Data.Path.Text, "./"), Line: ev.Data.LineNumber, Text: strings.TrimRight(ev.Data.Lines.Text, "\r\n")}
		for _, s := range ev.Data.Submatches {
			m.Spans = append(m.Spans, [2]int{s.Start, s.End})
		}
		res = append(res, m)
	}
	return res
}
