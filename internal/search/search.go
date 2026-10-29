// Package search runs ripgrep over a notes repository: smart case, markdown files only, temp
// files left out. nt s, the browser's live search and the apps share it. Where rg is missing,
// as on a phone, a search in Go finds the same lines. #tag words keep a search to the notes
// with those tags.
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
	"sort"
	"strings"
	"unicode"

	"github.com/w4jnl/nt/internal/mdparse"
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
// #tag words in query keep to the notes that have every one of those tags (docs/format.md §4;
// #work takes in work/q3). With no other words that is one line per note, the line that tags
// it, newest first; with words, the lines that have them in those notes.
func Search(ctx context.Context, root, query string) ([]Match, error) {
	tags, words := Split(query)
	if len(tags) == 0 {
		words = query
	} else {
		tagged, err := Tagged(ctx, root, tags)
		if err != nil || words == "" || len(tagged) == 0 {
			return tagged, err
		}
		paths := make([]string, len(tagged))
		for i, m := range tagged {
			paths[i] = m.Path
		}
		if _, err := exec.LookPath("rg"); err != nil {
			return goSearch(ctx, root, words, paths)
		}
		return rg(ctx, root, words, paths)
	}
	if _, err := exec.LookPath("rg"); err != nil {
		return Go(ctx, root, words)
	}
	return Rg(ctx, root, words)
}

// tagWord is a #tag in a query: a letter after the #, then letters, digits, -, _ and /.
var tagWord = regexp.MustCompile(`^#\p{L}[\p{L}\p{N}_/-]*$`)

// Split takes the #tag words out of query, lowercase and without the #, and gives back the
// other words with one space between them.
func Split(query string) (tags []string, words string) {
	var rest []string
	for _, w := range strings.Fields(query) {
		if tagWord.MatchString(w) {
			tags = append(tags, strings.ToLower(strings.TrimRight(w[1:], "/")))
		} else {
			rest = append(rest, w)
		}
	}
	return tags, strings.Join(rest, " ")
}

// Tagged finds the notes under root that have every tag in tags, or one under it: one Match per
// note, the first line that gives it one of them, with the tag marked; newest first by the date
// in the note's path (a journal's, a dated note's, a report's day), then the undated A to Z.
func Tagged(ctx context.Context, root string, tags []string) ([]Match, error) {
	var res []Match
	err := walk(ctx, root, func(rel string, b []byte) {
		have := mdparse.Tags(b)
		var first *mdparse.Tag
		for _, q := range tags {
			found := false
			for i := range have {
				if mdparse.HasTag(have[i].Name, q) {
					found = true
					if first == nil || have[i].Line < first.Line {
						first = &have[i]
					}
					break
				}
			}
			if !found {
				return
			}
		}
		lines := strings.Split(string(b), "\n")
		text := strings.TrimRight(lines[first.Line-1], "\r")
		m := Match{Path: rel, Line: first.Line, Text: text}
		if i := strings.Index(strings.ToLower(text), first.Name); i >= 0 {
			m.Spans = [][2]int{{i, i + len(first.Name)}}
		}
		res = append(res, m)
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(res, func(i, j int) bool {
		a, b := dated.FindAllString(res[i].Path, -1), dated.FindAllString(res[j].Path, -1)
		switch {
		case (len(a) > 0) != (len(b) > 0):
			return len(a) > 0
		case len(a) > 0 && a[len(a)-1] != b[len(b)-1]:
			return a[len(a)-1] > b[len(b)-1]
		}
		return res[i].Path < res[j].Path
	})
	return res, nil
}

var dated = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

// Go searches the repository at root for query as rg does: the query a regular expression,
// case-insensitive unless it has a capital letter, in markdown files outside hidden folders,
// temp files left out. No match is no error.
func Go(ctx context.Context, root, query string) ([]Match, error) {
	return goSearch(ctx, root, query, nil)
}

// goSearch is Go's search, in the notes at paths only when paths is not nil.
func goSearch(ctx context.Context, root, query string, paths []string) ([]Match, error) {
	expr := query
	if !strings.ContainsFunc(query, unicode.IsUpper) {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("%q is not a regular expression nt can search for: %w", query, err)
	}
	var res []Match
	find := func(rel string, b []byte) {
		for i, line := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
			line = strings.TrimRight(line, "\r")
			locs := re.FindAllStringIndex(line, -1)
			if locs == nil {
				continue
			}
			m := Match{Path: rel, Line: i + 1, Text: line}
			for _, l := range locs {
				if l[0] < l[1] {
					m.Spans = append(m.Spans, [2]int{l[0], l[1]})
				}
			}
			res = append(res, m)
		}
	}
	if paths == nil {
		err = walk(ctx, root, find)
	}
	for _, rel := range paths {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
			find(rel, b)
		}
	}
	if err != nil {
		return nil, err
	}
	return res, nil
}

// walk gives fn every markdown file under root, by repository path, as rg finds them: hidden
// folders and files (temp files among them) left out.
func walk(ctx context.Context, root string, fn func(rel string, b []byte)) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
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
		fn(filepath.ToSlash(rel), b)
		return nil
	})
}

// Rg searches the repository at root for query with ripgrep. No match is no error.
func Rg(ctx context.Context, root, query string) ([]Match, error) {
	return rg(ctx, root, query, nil)
}

// rg is ripgrep's search, in the notes at paths only when paths is not nil.
func rg(ctx context.Context, root, query string, paths []string) ([]Match, error) {
	if _, err := exec.LookPath("rg"); err != nil {
		return nil, ErrNoRg
	}
	if paths == nil {
		paths = []string{"."}
	}
	args := append([]string{"--json", "--smart-case", "--max-columns", "400",
		"--glob", "*.md", "--glob", "!.nt-tmp-*", "--", query}, paths...)
	cmd := exec.CommandContext(ctx, "rg", args...)
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
