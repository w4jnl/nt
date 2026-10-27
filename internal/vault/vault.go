// Package vault is a notes repository on disk (docs/format.md): where things go, what they are
// called, and how nt writes them. Every write is a temp file renamed into place under the
// repository lock; nothing here runs git.
package vault

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/w4jnl/nt/internal/config"
)

// Vault is one notes repository.
type Vault struct {
	Root string
	Cfg  config.Repo
	Host string
	Now  func() time.Time
}

// Open reads the repository at root. It must be a git working tree (the lock lives in .git).
func Open(root, host string) (*Vault, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if st, err := os.Stat(filepath.Join(abs, ".git")); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("%s is not a notes repository (no .git folder): run nt init %s", abs, root)
	}
	cfg, err := config.LoadRepo(abs)
	if err != nil {
		return nil, err
	}
	return &Vault{Root: abs, Cfg: cfg, Host: Slug(host), Now: time.Now}, nil
}

// Find picks the repository nt works on: $NT_REPO, else the nt repository around the working
// directory (a folder with .nt/config.toml and .git), else the machine config's repo.
func Find(m config.Machine) string {
	if r := os.Getenv("NT_REPO"); r != "" {
		return config.Expand(r)
	}
	if wd, err := os.Getwd(); err == nil {
		for d := wd; ; d = filepath.Dir(d) {
			if isFile(filepath.Join(d, ".nt", "config.toml")) && isDir(filepath.Join(d, ".git")) {
				return d
			}
			if filepath.Dir(d) == d {
				break
			}
		}
	}
	return m.Repo
}

// Abs turns a repository path (slash-separated, relative to the root) into a file path.
func (v *Vault) Abs(rel string) string { return filepath.Join(v.Root, filepath.FromSlash(rel)) }

// RelPath turns a file path inside the repository into a repository path.
func (v *Vault) RelPath(abs string) string {
	r, err := filepath.Rel(v.Root, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(r)
}

// Day is the date an entry made at t belongs to: before day_starts it is still the previous
// day. The result is midnight of that date in t's location.
func (v *Vault) Day(t time.Time) time.Time {
	start, _ := v.Cfg.DayStart()
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	if t.Sub(d) < start {
		d = d.AddDate(0, 0, -1)
	}
	return d
}

// JournalRel is the repository path of a day's journal: daily/YYYY/YYYY-MM-DD.md.
func (v *Vault) JournalRel(day time.Time) string {
	return fmt.Sprintf("%s/%s/%s.md", v.Cfg.Folders.Daily, day.Format("2006"), day.Format("2006-01-02"))
}

// AgentsRel is the repository path of a day's agent folder: agents/YYYY/YYYY-MM-DD.
func (v *Vault) AgentsRel(day time.Time) string {
	return fmt.Sprintf("%s/%s/%s", v.Cfg.Folders.Agents, day.Format("2006"), day.Format("2006-01-02"))
}

// WeekStart is the Monday of day's ISO week, at midnight.
func WeekStart(day time.Time) time.Time {
	d := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
}

// WeekRel is the repository path of the digest of day's week: weekly/GGGG/GGGG-Www.md, named by
// the ISO week-year, so the days around New Year go to the right week.
func (v *Vault) WeekRel(day time.Time) string {
	y, w := WeekStart(day).ISOWeek()
	return fmt.Sprintf("%s/%04d/%04d-W%02d.md", v.Cfg.Folders.Weekly, y, y, w)
}

// Dated reports a file whose path is a date nt fills in, which may not exist yet: a day's
// journal, a day's agent digest, a week's digest. The journals link the days around them and
// their digest, the weekly digests the weeks around them, so such a link to nothing is expected,
// not broken.
func (v *Vault) Dated(rel string) bool {
	c := v.Cfg.Folders
	for _, f := range []struct{ dir, re string }{
		{c.Daily, `\d{4}/\d{4}-\d{2}-\d{2}\.md`},
		{c.Agents, `\d{4}/\d{4}-\d{2}-\d{2}/README\.md`},
		{c.Weekly, `\d{4}/\d{4}-W\d{2}\.md`},
	} {
		if rest, ok := strings.CutPrefix(rel, f.dir+"/"); ok && regexp.MustCompile(`^`+f.re+`$`).MatchString(rest) {
			return true
		}
	}
	return false
}

// Slug makes a file name part from a title (docs/format.md §3): lowercase, accents folded,
// every other run of characters one "-", at most 60 characters cut at a "-".
func Slug(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range norm.NFKD.String(strings.ToLower(title)) {
		switch {
		case unicode.Is(unicode.Mn, r): // a combining accent left by NFKD
			continue
		case r == 'ß':
			b.WriteString("ss")
			dash = false
		case r == 'æ' || r == 'ø' || r == 'œ' || r == 'ł' || r == 'đ' || r == 'ð' || r == 'þ':
			b.WriteString(map[rune]string{'æ': "ae", 'ø': "o", 'œ': "oe", 'ł': "l", 'đ': "d", 'ð': "d", 'þ': "th"}[r])
			dash = false
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 60 {
		s = s[:61]
		if i := strings.LastIndexByte(s, '-'); i > 0 {
			s = s[:i]
		} else {
			s = s[:60]
		}
	}
	return strings.Trim(s, "-")
}

// Unique returns rel, or rel with -2, -3, … before the extension, whichever names no file yet.
// Names that differ only in case count as taken: macOS treats them as one file.
func (v *Vault) Unique(rel string) string {
	ext := filepath.Ext(rel)
	stem := strings.TrimSuffix(rel, ext)
	for i := 1; ; i++ {
		cand := rel
		if i > 1 {
			cand = fmt.Sprintf("%s-%d%s", stem, i, ext)
		}
		if !v.taken(cand) {
			return cand
		}
	}
}

func (v *Vault) taken(rel string) bool {
	abs := v.Abs(rel)
	if _, err := os.Lstat(abs); err == nil {
		return true
	}
	entries, err := os.ReadDir(filepath.Dir(abs))
	if err != nil {
		return false
	}
	base := filepath.Base(abs)
	for _, e := range entries {
		if strings.EqualFold(e.Name(), base) {
			return true
		}
	}
	return false
}

// Lock takes the repository lock, .git/nt.lock (flock LOCK_EX), and returns its release.
func (v *Vault) Lock() (func(), error) {
	return lockFile(filepath.Join(v.Root, ".git", "nt.lock"), false)
}

// LockEdit marks an editor as open on the repository (.git/nt-edit, shared): sync commits but
// does not merge while one is held, so the editor never sees its file change underneath it.
func (v *Vault) LockEdit() (func(), error) {
	return lockFile(filepath.Join(v.Root, ".git", "nt-edit"), true)
}

// Editing reports whether an editor started by nt holds the repository.
func (v *Vault) Editing() bool {
	f, err := os.OpenFile(filepath.Join(v.Root, ".git", "nt-edit"), os.O_RDONLY, 0)
	if err != nil {
		return false
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return true
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return false
}

func lockFile(path string, shared bool) (func(), error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	how := syscall.LOCK_EX
	if shared {
		how = syscall.LOCK_SH
	}
	if err := syscall.Flock(int(f.Fd()), how); err != nil {
		f.Close()
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}

// TryLock takes an exclusive lock on path without waiting; ok is false when another process
// holds it.
func TryLock(path string) (release func(), ok bool, err error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, true, nil
}

// WriteFile writes data to path through a temp file .nt-tmp-* in the same folder, renamed over
// the target, so a reader never sees half a file. The folder is created when missing.
func WriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".nt-tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.Write(data)
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp)
		return err
	}
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	if err := os.Chmod(tmp, mode); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// Update rewrites the repository file rel under the lock: fn gets the current bytes (nil when
// the file does not exist) and returns the new ones; returning the same bytes writes nothing.
func (v *Vault) Update(rel string, fn func(old []byte) ([]byte, error)) error {
	unlock, err := v.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	path := v.Abs(rel)
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := fn(old)
	if err != nil {
		return err
	}
	if old != nil && bytes.Equal(old, data) {
		return nil
	}
	return WriteFile(path, data)
}

func isFile(p string) bool { st, err := os.Stat(p); return err == nil && st.Mode().IsRegular() }
func isDir(p string) bool  { st, err := os.Stat(p); return err == nil && st.IsDir() }

// SetSetting writes a setting of the repository's .nt/config.toml (config.Settings, File
// "repo") typed as raw, under the repository lock, keeping the rest of the file; sync shares
// it. Nothing is written when the result would not load.
func (v *Vault) SetSetting(key, raw string) error {
	s, ok := config.FindSetting(key)
	if !ok || s.File != "repo" {
		return fmt.Errorf("%q is not a setting of the repository", key)
	}
	value, err := s.TOML(raw)
	if err != nil {
		return err
	}
	err = v.Update(".nt/config.toml", func(old []byte) ([]byte, error) {
		if old == nil {
			old = []byte(config.RepoTemplate)
		}
		out := config.Edit(old, key, value)
		if err := config.CheckRepo(out); err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		return out, nil
	})
	if err != nil {
		return err
	}
	cfg, err := config.LoadRepo(v.Root)
	if err == nil {
		v.Cfg = cfg
	}
	return err
}
