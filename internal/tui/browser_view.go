package tui

import (
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/w4jnl/nt/internal/theme"
)

// narrow reports a screen too small for both panes.
func (b *browser) narrow() bool { return b.width < listMin+previewMin+1 }

// listWidth is the list pane's width: the width asked for, at least list-min and leaving the
// note at least preview-min.
func (b *browser) listWidth() int {
	if b.narrow() {
		return b.width
	}
	return max(listMin, min(b.listW, b.width-previewMin-1))
}

// bodyHeight is the rows between the two rules: the screen less the brand line, the rules and
// the footer.
func (b *browser) bodyHeight() int { return max(1, b.height-4) }

func (b *browser) layout() {
	switch {
	case b.full:
		b.p.setSize(b.width, b.bodyHeight())
	case b.narrow():
		b.p.setSize(1, 1)
	default:
		b.p.setSize(b.width-b.listWidth()-1, b.bodyHeight())
	}
	b.clampList()
}

func (b *browser) View() tea.View {
	if b.width == 0 {
		return tea.NewView("")
	}
	rule := b.st.Rule.Render(strings.Repeat("─", b.width))
	v := tea.NewView(strings.Join([]string{b.brandLine(), rule, b.body(), rule, b.footer()}, "\n"))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion // clicks, the wheel, and motion while a button is held
	v.WindowTitle = "nt · " + firstNonEmpty(b.dir, "notes")
	return v
}

// brandLine is the top row: [nt], the folder, and the sync state at the right edge.
func (b *browser) brandLine() string {
	where := firstNonEmpty(b.dir, "notes")
	if b.cal != nil {
		where = "calendar"
	}
	if b.set != nil {
		where = "settings"
	}
	if b.full && b.showing != "" {
		where = b.showing
	}
	left := " " + b.st.Wordmark() + "  " + b.st.Text.Render(where)
	return joinLR(left, b.syncBadge()+" ", b.width)
}

// syncBadge is the sync state as glyph and word: ● synced, ↑ 2 unpushed, ◌ offline, ◆ conflict.
func (b *browser) syncBadge() string {
	paint := func(s theme.State, text string) string {
		return lipgloss.NewStyle().Foreground(b.pal.Color(s)).Render(theme.Glyph(s, b.frame) + " " + text)
	}
	if b.syncing {
		return paint(theme.Syncing, "syncing")
	}
	if !b.hasStatus {
		return b.st.Faint.Render("· not synced yet")
	}
	st := b.status
	var badge string
	switch st.State {
	case "synced":
		badge = paint(theme.Synced, "synced")
	case "local":
		badge = paint(theme.Synced, "local")
	case "unpushed":
		badge = paint(theme.Unpushed, strconv.Itoa(st.Ahead)+" unpushed")
	case "offline":
		badge = paint(theme.Offline, "offline")
	case "conflict":
		badge = paint(theme.Conflict, plural(len(st.Conflicts), "conflict"))
	default:
		badge = paint(theme.Conflict, "sync error")
	}
	if age := time.Since(st.At); age > 10*time.Minute && st.State != "error" {
		badge += b.st.Faint.Render(" · " + ago(age))
	}
	return badge
}

func ago(d time.Duration) string {
	switch {
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m ago"
	case d < 48*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h ago"
	}
	return strconv.Itoa(int(d.Hours()/24)) + "d ago"
}

// body is the two panes side by side, or one of them on a narrow screen.
func (b *browser) body() string {
	h := b.bodyHeight()
	if b.full {
		return b.previewPane(b.width, h)
	}
	lw := b.listWidth()
	left := b.listPane(lw, h)
	if b.narrow() {
		return left
	}
	pw := b.width - lw - 1
	right := b.previewPane(pw, h)
	sep := b.st.Rule.Render("│")
	if b.dragging {
		sep = b.st.Key.Render("│")
	}
	ll, rl := strings.Split(left, "\n"), strings.Split(right, "\n")
	out := make([]string, h)
	for i := range h {
		l, r := "", ""
		if i < len(ll) {
			l = ll[i]
		}
		if i < len(rl) {
			r = rl[i]
		}
		out[i] = pad(l, lw) + sep + r
	}
	return strings.Join(out, "\n")
}

func pad(s string, w int) string {
	if n := ansi.StringWidth(s); n < w {
		return s + strings.Repeat(" ", w-n)
	} else if n > w {
		return ansi.Truncate(s, w, "…")
	}
	return s
}

// previewPane is the note, the help, or nothing for a folder.
func (b *browser) previewPane(w, h int) string {
	if b.help {
		return b.helpText(w, h)
	}
	if b.set != nil {
		return b.settingsPane(w, h)
	}
	if b.showing == "" {
		if b.cal != nil {
			return "\n  " + b.st.Muted.Render("no journal on "+b.cal.day.Format("Monday 2 January 2006")) + "\n  " + b.st.Faint.Render("e starts one")
		}
		if e, ok := b.selected(); ok && e.view != "" {
			what, how := b.viewHint(e.view)
			return "\n  " + b.st.Muted.Render(what) + "\n  " + b.st.Faint.Render(how)
		}
		if e, ok := b.selected(); ok && e.dir {
			return "\n  " + b.st.Muted.Render(e.rel+"/") + "\n  " + b.st.Faint.Render("enter opens the folder")
		}
		if len(b.entries) == 0 {
			return "\n  " + b.st.Muted.Render("an empty folder") + "\n  " + b.st.Faint.Render("n makes a note here")
		}
		return ""
	}
	return b.p.View()
}

// viewHint is what the preview says for a view's row with no note to show: what is there, and
// what enter does.
func (b *browser) viewHint(view string) (what, how string) {
	switch view {
	case "today":
		return "no journal today yet", "enter starts it"
	case "todos":
		switch {
		case !b.todoKnown:
			return "the todo list", "enter opens it"
		case b.todoOpen == 0:
			return "no open todos", "enter opens the list, a adds one there"
		case b.todoLate > 0:
			return plural(b.todoOpen, "open todo") + " · " + strconv.Itoa(b.todoLate) + " late", "enter opens the list"
		}
		return plural(b.todoOpen, "open todo"), "enter opens the list"
	case "calendar":
		return b.v.Day(b.v.Now()).Format("January 2006"), "enter opens the calendar"
	case "weekly":
		return "no weekly digest yet", "nt config set weekly true on one machine writes them"
	case "agents":
		if isDir(b.v.Abs(b.v.AgentsRel(b.v.Day(b.v.Now())))) {
			return "today's agent sessions", "enter opens them"
		}
		return "no agent sessions today", "nt install --hooks reports Claude Code's and Copilot CLI's"
	}
	return "", ""
}

// listPane is the folder's rows, or the open picker in their place.
func (b *browser) listPane(w, h int) string {
	if b.pick != nil {
		return b.pickerPane(w, h)
	}
	if b.cal != nil {
		return b.calendarPane(w, h)
	}
	var rows []string
	for i := b.top; i < len(b.entries) && len(rows) < h; i++ {
		rows = append(rows, b.row(b.entries[i], i == b.sel, w))
	}
	if len(b.entries) == 0 {
		rows = append(rows, "  "+b.st.Faint.Render("no notes here"))
	}
	return strings.Join(rows, "\n")
}

// row draws one entry: a view by its name, a folder as name/, a journal as its date and day, a
// dated note as its date and title, any other note as its title; the home list's rows have their
// key at the right, the todos their count. The selected row has the accent bar on a surface
// background; when the preview has the focus the bar is faint.
func (b *browser) row(e entry, selected bool, w int) string {
	if e.rule {
		return "  " + b.st.Rule.Render(strings.Repeat("─", max(0, w-4)))
	}
	text, muted, faint, bar, key := b.st.Text, b.st.Muted, b.st.Faint, b.st.Bar, b.st.Key
	warn := lipgloss.NewStyle().Foreground(b.pal.Warn)
	lead := " "
	if selected {
		bg := b.pal.Surface
		text, muted, faint, key, warn = text.Background(bg), muted.Background(bg), faint.Background(bg), key.Background(bg), warn.Background(bg)
		if b.focus == onPreview {
			bar = b.st.Faint.Background(bg)
		}
		lead = bar.Render("▎")
	}
	var body string
	switch {
	case e.view != "":
		body = text.Render(" " + e.name)
	case e.dir:
		body = text.Render(" "+e.name) + muted.Render("/")
	case e.journal:
		body = text.Render(" "+e.date) + muted.Render("  "+dayWord(e.date))
	case e.date != "":
		body = faint.Render(" "+e.date+" ") + text.Render(firstNonEmpty(e.title, e.name))
	case e.host != "":
		body = faint.Render(" "+e.host+" ") + text.Render(firstNonEmpty(e.title, e.name))
	default:
		body = text.Render(" " + firstNonEmpty(e.title, e.name))
	}
	line := lead + body
	if e.key != "" {
		right := key.Render(e.key) + text.Render(" ")
		if e.view == "todos" && b.todoKnown && b.todoOpen > 0 {
			count := muted.Render(strconv.Itoa(b.todoOpen))
			if b.todoLate > 0 {
				count += muted.Render(" · ") + warn.Render(strconv.Itoa(b.todoLate)+" late")
			}
			right = count + text.Render("  ") + right
		}
		if gap := w - ansi.StringWidth(line) - ansi.StringWidth(right); gap > 0 {
			line += text.Render(strings.Repeat(" ", gap)) + right
		}
	}
	if n := ansi.StringWidth(line); n > w {
		line = ansi.Truncate(line, w, "…")
	} else if selected {
		line += text.Render(strings.Repeat(" ", w-n))
	}
	return line
}

// dayWord says how far a journal's day is: today, yesterday, a weekday within the week, else
// the short weekday.
func dayWord(date string) string {
	d, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return ""
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	switch days := int(today.Sub(d).Hours() / 24); {
	case days == 0:
		return "today"
	case days == 1:
		return "yesterday"
	case days == -1:
		return "tomorrow"
	case days > 1 && days < 7:
		return d.Format("Monday")
	}
	return d.Format("Mon")
}

func (b *browser) pickerPane(w, h int) string {
	p := b.pick
	rows := []string{" " + b.st.Muted.Render(p.title)}
	for i := p.top; i < len(p.items) && len(rows) < h; i++ {
		it := p.items[i]
		if it.todo != nil {
			rows = append(rows, b.todoRow(*it.todo, i == p.sel, w))
			continue
		}
		rows = append(rows, pickRow(b.st, b.pal, it, i == p.sel, w))
	}
	switch {
	case len(p.items) == 0 && b.input != nil && b.input.Value() != "" && b.inputKind == "search":
		rows = append(rows, "  "+b.st.Faint.Render("no lines match"))
	case len(p.items) == 0 && p.todos && p.ready && p.done:
		rows = append(rows, "  "+b.st.Faint.Render("nothing done yet"))
	case len(p.items) == 0 && p.todos && p.ready:
		rows = append(rows, "  "+b.st.Faint.Render("no open todos · a adds one"))
	}
	return strings.Join(rows, "\n")
}

// pickRow draws a picker's row: its label, and its hint faint after it.
func pickRow(st theme.Styles, pal theme.Palette, it pickItem, selected bool, w int) string {
	text, muted := st.Text, st.Faint
	lead := " "
	if selected {
		text, muted = text.Background(pal.Surface), muted.Background(pal.Surface)
		lead = st.Bar.Render("▎")
	}
	line := lead + text.Render(" "+it.label)
	if it.hint != "" && it.hint != strings.TrimPrefix(it.label, "→ ") {
		line += muted.Render("  " + it.hint)
	}
	if n := ansi.StringWidth(line); n > w {
		line = ansi.Truncate(line, w, "…")
	} else if selected {
		line += text.Render(strings.Repeat(" ", w-n))
	}
	return line
}

// helpText is every key, in the preview pane: two columns when the pane is wide enough.
func (b *browser) helpText(w, h int) string {
	groups := [][]string{
		{"move", "j k  ↑ ↓", "up and down", "enter →", "open a folder or note", "h ← ⌫", "up a folder", "tab", "to the note and back", "g G", "top, bottom"},
		{"go", "t", "today's journal", "[ ]", "the day before, after", "c", "the calendar", "w", "the newest weekly digest", "1–9", "pinned folders", "A", "today's agent reports", ",", "the settings"},
		{"find", "/ ctrl-p", "search every note, find a title"},
		{"todos", "x", "the todo list, then:", "x space", "check, uncheck", "d", "its due date", "a", "add one to todo.md", "X", "the done ones", "e D", "edit, delete"},
		{"in a note", "f", "follow a link: type its letters, or click it", "b", "back to the note before", "l o i", "its links, outline, images", "e", "edit in $EDITOR", "v", "read in glow", "y", "copy the selection, else its path", "D", "delete it, after asking"},
		{"write", "a", "add a line to today", "n", "a new note here", "N", "a new folder here", "m R", "move, rename it", "s", "sync now", "r", "reload"},
		{"mouse", "drag text", "select it in the note", "drag │", "resize the list (< > too)", "click", "select; again opens", "wheel", "scroll"},
		{"", "?", "this help", "q", "quit"},
	}
	block := func(gs [][]string) []string {
		var lines []string
		for _, g := range gs {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			if g[0] != "" {
				lines = append(lines, "  "+b.st.Muted.Render(g[0]))
			}
			for i := 1; i+1 < len(g); i += 2 {
				lines = append(lines, "  "+b.st.Key.Render(pad(g[i], 9))+" "+b.st.Verb.Render(g[i+1]))
			}
		}
		return lines
	}
	lines := append([]string{""}, block(groups)...)
	if len(lines) > h && w >= 66 {
		left, right := block(groups[:4]), block(groups[4:])
		lines = []string{""}
		for i := range max(len(left), len(right)) {
			l, r := "", ""
			if i < len(left) {
				l = left[i]
			}
			if i < len(right) {
				r = right[i]
			}
			lines = append(lines, pad(l, w/2)+r)
		}
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		lines[i] = pad(l, w)
	}
	return strings.Join(lines, "\n")
}

// footer is the prompt when one is open, else the key hints with the last message at the right,
// or nt's version when there is no message and the hints leave room for it.
func (b *browser) footer() string {
	if b.input != nil {
		return pad(lipgloss.NewStyle().Background(b.pal.Surface).Render(" ")+b.input.View(b.st, b.pal, b.width-1), b.width)
	}
	if b.ask != nil {
		q := lipgloss.NewStyle().Background(b.pal.Surface).Foreground(b.pal.Ink).Render(" " + b.ask.text + " ")
		return pad(q+"  "+hints(b.st, "y", "yes", "any key", "no"), b.width)
	}
	var left string
	switch {
	case b.p.hints != nil:
		left = hints(b.st, "letters", "follow the link they label", "esc", "cancel")
	case b.p.mark != nil && !b.p.mark.empty():
		left = hints(b.st, "y", "copy", "esc", "clear", "drag", "select more")
	case b.pick != nil && b.pick.todos:
		check, other := "done", "done ones"
		if b.pick.done {
			check, other = "open again", "open ones"
		}
		left = hints(b.st, "x", check, "d", "due", "a", "add", "X", other, "e", "edit", "D", "delete", "esc", "back")
	case b.set != nil:
		left = hints(b.st, "j k", "move", "enter", "change", "e", "edit the file", "esc", "back")
	case b.pick != nil:
		left = hints(b.st, "enter", "open", "j k", "move", "esc", "back")
	case b.cal != nil && b.focus == onList:
		left = hints(b.st, "h l", "day", "j k", "week", "[ ]", "month", "t", "today", "enter", "open", "e", "edit", "x", "todos", "esc", "back")
	case b.focus == onPreview:
		left = hints(b.st, "j k", "scroll", "e", "edit", "l", "links", "o", "outline", "D", "delete", "esc", "back")
	default:
		left = hints(b.st, "t", "today", "[ ]", "day", "a", "add", "n", "new", "/", "search", "^p", "title", "x", "todos", "D", "delete", "?", "help")
	}
	left = " " + left
	right := ""
	if b.msg != "" {
		if b.alert {
			right = lipgloss.NewStyle().Foreground(b.pal.Alert).Render("◆ "+b.msg) + " "
		} else {
			right = b.st.Muted.Render(b.msg) + " "
		}
	}
	if right == "" && b.o.Version != "" {
		if v := b.st.Faint.Render("nt "+strings.TrimPrefix(b.o.Version, "v")) + " "; lipgloss.Width(left)+2+lipgloss.Width(v) <= b.width {
			right = v
		}
	}
	if lipgloss.Width(left)+lipgloss.Width(right) > b.width && right != "" {
		return pad(right, b.width) // a message matters more than the hints it covers
	}
	return pad(joinLR(left, right, b.width), b.width)
}
