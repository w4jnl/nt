// Package search runs ripgrep over a notes repository: smart case, markdown files only, temp
// files left out. nt s, the browser's live search and the apps share it. Where rg is missing,
// as on a phone, a search in Go finds the same lines.
package search

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
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

// Search searches the repository at root for query: with rg when it is there, else in Go.
func Search(ctx context.Context, root, query string) ([]Match, error) {
	if _, err := exec.LookPath("rg"); err != nil {
		return Go(ctx, root, query)
	}
	return Rg(ctx, root, query)
}

// Go searches the repository at root for query as rg does: the query a regular expression,
// case-insensitive unless it has a capital letter, in markdown files outside hidden folders,
// temp files left out. No match is no error.
func Go(ctx context.Context, root, query string) ([]Match, error) {
	expr := query
	if !strings.ContainsFunc(query, unicode.IsUpper) {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("%q is not a regular expression nt can search for: %w", query, err)
	}
	var res []Match
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".md") || strings.HasPrefix(name, ".") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		for i, line := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
			line = strings.TrimRight(line, "\r")
			locs := re.FindAllStringIndex(line, -1)
			if locs == nil {
				continue
			}
			m := Match{Path: filepath.ToSlash(rel), Line: i + 1, Text: line}
			for _, l := range locs {
				if l[0] < l[1] {
					m.Spans = append(m.Spans, [2]int{l[0], l[1]})
				}
			}
			res = append(res, m)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// Rg searches the repository at root for query with ripgrep. No match is no error.
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
