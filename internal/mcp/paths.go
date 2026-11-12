package mcp

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/w4jnl/nt/internal/config"
	"github.com/w4jnl/nt/internal/vault"
)

// Sizes a tool takes: a note's text, and one image.
const (
	maxNote  = 1 << 20
	maxImage = 5 << 20
)

// scope is the repository and what of it the server may touch, read anew for each call so a
// change of the settings holds from the next one.
type scope struct {
	v    *vault.Vault
	hide []string // mcp.hide: folders kept from the server, as repository paths
}

// Hidden are the folders of mcp.hide as repository paths.
func Hidden(m config.Machine) []string {
	var out []string
	for _, h := range m.MCP.Hide {
		if h = strings.Trim(path.Clean("/"+filepath.ToSlash(strings.TrimSpace(h))), "/"); h != "" {
			out = append(out, h)
		}
	}
	return out
}

// hidden says whether rel is one of the hidden folders or in one.
func (sc scope) hidden(rel string) bool {
	for _, h := range sc.hide {
		if rel == h || strings.HasPrefix(rel, h+"/") {
			return true
		}
	}
	return false
}

// holds is the hidden folder inside the folder rel, or "".
func (sc scope) holds(rel string) string {
	for _, h := range sc.hide {
		if rel == "" || strings.HasPrefix(h, rel+"/") {
			return h
		}
	}
	return ""
}

// rel is the repository path p names, "" the root: a path from the root of the notes that stays
// in the repository, also through symlinks, names nothing that starts with a dot, and is not
// hidden.
func (sc scope) rel(p string) (string, error) {
	p = filepath.ToSlash(strings.TrimSpace(p))
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~") || filepath.IsAbs(p) {
		return "", fmt.Errorf("%s: give a path from the root of the notes, like work/plan.md", p)
	}
	c := path.Clean(p)
	if c == "." {
		return "", nil
	}
	if c == ".." || strings.HasPrefix(c, "../") {
		return "", fmt.Errorf("%s is outside the notes", p)
	}
	for _, part := range strings.Split(c, "/") {
		if strings.HasPrefix(part, ".") {
			return "", fmt.Errorf("%s: names that start with . are not notes", p)
		}
	}
	if sc.hidden(c) {
		return "", fmt.Errorf("%s is kept from the Claude app on this machine (the setting mcp.hide)", c)
	}
	if err := sc.inside(c); err != nil {
		return "", err
	}
	return c, nil
}

// inside checks that what is there of rel resolves to a place in the repository.
func (sc scope) inside(rel string) error {
	root, err := filepath.EvalSymlinks(sc.v.Root)
	if err != nil {
		return err
	}
	for at := rel; ; at = path.Dir(at) {
		if at == "." {
			return nil
		}
		real, err := filepath.EvalSymlinks(sc.v.Abs(at))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if r, err := filepath.Rel(root, real); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%s leads outside the notes", rel)
		}
		return nil
	}
}

// note is the repository path of the note p names, which is there.
func (sc scope) note(p string) (string, error) {
	rel, err := sc.rel(p)
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(rel, ".md") {
		return "", fmt.Errorf("%s is not a note: notes end in .md", p)
	}
	if st, err := os.Stat(sc.v.Abs(rel)); err != nil || !st.Mode().IsRegular() {
		return "", fmt.Errorf("%s: no such note; list or search finds the notes there are", rel)
	}
	return rel, nil
}

// compiled says why the server does not write rel, or nil: nt compiles the agents' reports and
// the weekly digests.
func (sc scope) compiled(rel string) error {
	for _, f := range []string{sc.v.Cfg.Folders.Agents, sc.v.Cfg.Folders.Weekly} {
		if f != "" && (rel == f || strings.HasPrefix(rel, f+"/")) {
			return fmt.Errorf("%s: nt compiles what is in %s/; read it, and write notes elsewhere", rel, f)
		}
	}
	return nil
}
