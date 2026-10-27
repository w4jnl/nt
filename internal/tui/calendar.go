package tui

import (
	"os"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/w4jnl/nt/internal/editor"
	"github.com/w4jnl/nt/internal/theme"
	"github.com/w4jnl/nt/internal/todo"
)

// The calendar (docs/design.md §4, Calendar): a month in the list pane, with the days that have
// a journal and the days with todos due marked, and the selected day's todos and agent sessions
// under it. The preview shows that day's journal. It writes nothing but a journal e starts.

// calendar is the open calendar.
type calendar struct {
	day      time.Time              // the selected day, at midnight
	journals map[string]bool        // the days of the shown month that have a journal
	sessions int                    // agent reports on the selected day
	due      map[string][]todo.Item // YYYY-MM-DD → the todos due that day
	done     map[string][]todo.Item // YYYY-MM-DD → the todos done that day
	seq      int                    // the reading of the todos in flight
	clicked  bool                   // the last click selected a day; a second one opens it
}

type (
	calTodosMsg struct {
		seq   int
		items []todo.Item
	}
	calPreviewMsg struct{ seq int }
)

const calWidth = 28 // 7 days of 4 columns: list-min

// openCalendar shows the month around today.
func (b *browser) openCalendar() tea.Cmd {
	b.cal = &calendar{day: b.v.Day(b.v.Now())}
	b.focus = onList
	b.calRead()
	return tea.Batch(b.calShow(), b.calTodos())
}

func (b *browser) closeCalendar() tea.Cmd {
	b.cal = nil
	return b.loadNow()
}

// calRead notes which days of the shown month have a journal, and the selected day's agent
// sessions: a few stat calls, so it runs on every move and every poll.
func (b *browser) calRead() {
	c := b.cal
	c.journals = map[string]bool{}
	first := time.Date(c.day.Year(), c.day.Month(), 1, 0, 0, 0, 0, c.day.Location())
	for d := first; d.Month() == first.Month(); d = d.AddDate(0, 0, 1) {
		if isFile(b.v.Abs(b.v.JournalRel(d))) {
			c.journals[d.Format("2006-01-02")] = true
		}
	}
	c.sessions = 0
	des, _ := os.ReadDir(b.v.Abs(b.v.AgentsRel(c.day)))
	for _, d := range des {
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".md") && d.Name() != "README.md" {
			c.sessions++
		}
	}
}

// calTodos reads the todos in the background, for the marks and the day's list.
func (b *browser) calTodos() tea.Cmd {
	b.cal.seq++
	seq, v := b.cal.seq, b.v
	return func() tea.Msg { return calTodosMsg{seq: seq, items: todo.Collect(v)} }
}

func (b *browser) setCalTodos(msg calTodosMsg) {
	c := b.cal
	if c == nil || msg.seq != c.seq {
		return
	}
	todo.Sort(msg.items)
	c.due, c.done = map[string][]todo.Item{}, map[string][]todo.Item{}
	for _, it := range msg.items {
		if it.Due != "" {
			c.due[it.Due] = append(c.due[it.Due], it)
		}
		if it.Done && it.DoneOn != "" && it.DoneOn != it.Due {
			c.done[it.DoneOn] = append(c.done[it.DoneOn], it)
		}
	}
}

// calGo selects day and shows its journal a moment later, so holding a key does not render
// every journal on the way.
func (b *browser) calGo(day time.Time) tea.Cmd {
	b.cal.day = day
	b.cal.clicked = false
	b.calRead()
	b.loadSeq++
	seq := b.loadSeq
	return tea.Tick(60*time.Millisecond, func(time.Time) tea.Msg { return calPreviewMsg{seq} })
}

// calShow puts the selected day's journal in the preview, or nothing when it has none.
func (b *browser) calShow() tea.Cmd {
	rel := b.v.JournalRel(b.cal.day)
	if isFile(b.v.Abs(rel)) {
		return b.show(rel)
	}
	b.showing = ""
	b.p.open("", "")
	b.p.vp.SetContent("")
	return b.p.sendImages()
}

// calKey handles the calendar's keys and reports whether k was one of them. Keys that act on
// the list's selection, which the calendar hides, do nothing.
func (b *browser) calKey(k tea.KeyPressMsg) (tea.Cmd, bool) {
	c := b.cal
	switch s := k.String(); s {
	case "h", "left":
		return b.calGo(c.day.AddDate(0, 0, -1)), true
	case "l", "right":
		return b.calGo(c.day.AddDate(0, 0, 1)), true
	case "k", "up":
		return b.calGo(c.day.AddDate(0, 0, -7)), true
	case "j", "down":
		return b.calGo(c.day.AddDate(0, 0, 7)), true
	case "[", "]":
		return b.calGo(addMonths(c.day, map[string]int{"[": -1, "]": 1}[s])), true
	case "t":
		return b.calGo(b.v.Day(b.v.Now())), true
	case "enter":
		rel := b.v.JournalRel(c.day)
		if !isFile(b.v.Abs(rel)) {
			b.say("no journal on "+c.day.Format("Mon 2 Jan")+": e starts one", false)
			return nil, true
		}
		b.cal = nil
		return b.reveal(rel), true
	case "e":
		at := c.day.Add(12 * time.Hour) // midday: day_starts does not move it to the day before
		rel, _, err := b.v.EnsureJournal(at)
		if err != nil {
			b.say(err.Error(), true)
			return nil, true
		}
		b.calRead()
		if b.showing == rel {
			return b.edit(rel), true
		}
		return b.editAt(rel, editor.LastLine(b.v.Abs(rel))), true
	case "x":
		return b.openDayTodos(c.day.Format("2006-01-02")), true
	case "tab":
		if b.showing != "" && !b.narrow() {
			b.focus = onPreview
		}
		return nil, true
	case "r":
		b.calRead()
		return tea.Batch(b.calShow(), b.calTodos()), true
	case "esc", "c", "q":
		if b.help {
			b.help = false
			return nil, true
		}
		return b.closeCalendar(), true
	case "s", "?", "a", "n", "/", "ctrl+p", "<", ">", "ctrl+c":
		return nil, false // the browser's own: sync, help, add, new, search, resize, quit
	}
	return nil, true
}

// addMonths moves day by n months, to the month's last day when it is shorter.
func addMonths(day time.Time, n int) time.Time {
	first := time.Date(day.Year(), day.Month()+time.Month(n), 1, 0, 0, 0, 0, day.Location())
	last := first.AddDate(0, 1, -1).Day()
	return first.AddDate(0, 0, min(day.Day(), last)-1)
}

// calClick selects the day under a click on the grid, row and column in the list pane; a second
// click on it opens its journal.
func (b *browser) calClick(row, col int) tea.Cmd {
	c := b.cal
	first := time.Date(c.day.Year(), c.day.Month(), 1, 0, 0, 0, 0, c.day.Location())
	week, wd := row-2, col/4 // under the title and the weekday names
	if week < 0 || week > 5 || col >= calWidth {
		return nil
	}
	d := first.AddDate(0, 0, week*7+wd-weekday(first))
	if d.Month() != first.Month() {
		return nil
	}
	if d.Equal(c.day) && c.clicked {
		return b.calEnter()
	}
	cmd := b.calGo(d)
	c.clicked = true
	return cmd
}

func (b *browser) calEnter() tea.Cmd {
	cmd, _ := b.calKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	return cmd
}

// weekday is the day's column: Monday 0 to Sunday 6.
func weekday(d time.Time) int { return (int(d.Weekday()) + 6) % 7 }

// calendarPane draws the month and, under it, the selected day: its journal, its todos (due or
// done that day) and its agent sessions.
func (b *browser) calendarPane(w, h int) string {
	c := b.cal
	today := b.v.Day(b.v.Now())
	first := time.Date(c.day.Year(), c.day.Month(), 1, 0, 0, 0, 0, c.day.Location())
	rows := []string{" " + b.st.Muted.Render(first.Format("January 2006")), b.st.Faint.Render("  mo  tu  we  th  fr  sa  su")}
	warn, done := lipgloss.NewStyle().Foreground(b.pal.Warn), lipgloss.NewStyle().Foreground(b.pal.Color(theme.Done))
	var line strings.Builder
	line.WriteString(strings.Repeat("    ", weekday(first)))
	for d := first; d.Month() == first.Month(); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		num, mark, dot, lead := b.st.Faint, b.st.Muted, " ", " "
		if c.journals[key] {
			num = b.st.Text
		}
		if d.Equal(today) {
			num = b.st.Key
		}
		for _, it := range c.due[key] {
			if !it.Done {
				dot = "•"
				if d.Before(today) {
					mark = warn
				}
			}
		}
		if d.Equal(c.day) {
			num, mark = num.Background(b.pal.Surface), mark.Background(b.pal.Surface)
			lead = b.st.Bar.Render("▎")
		}
		line.WriteString(lead + num.Render(pad2(d.Day())) + mark.Render(dot))
		if weekday(d) == 6 {
			rows = append(rows, line.String())
			line.Reset()
		}
	}
	if line.Len() > 0 {
		rows = append(rows, line.String())
	}

	day := c.day.Format("Monday 2 January")
	if c.day.Year() != today.Year() {
		day += " " + c.day.Format("2006")
	}
	for word, d := range map[string]time.Time{"today": today, "yesterday": today.AddDate(0, 0, -1), "tomorrow": today.AddDate(0, 0, 1)} {
		if c.day.Equal(d) {
			day += b.st.Muted.Render(" · " + word)
		}
	}
	rows = append(rows, "", " "+b.st.Text.Render(day))
	if c.journals[c.day.Format("2006-01-02")] {
		rows = append(rows, " "+b.st.Muted.Render("journal · enter opens it"))
	} else {
		rows = append(rows, " "+b.st.Faint.Render("no journal · e starts one"))
	}
	key := c.day.Format("2006-01-02")
	for _, it := range append(append([]todo.Item{}, c.due[key]...), c.done[key]...) {
		mark := b.st.Muted.Render(" ○ ")
		if it.Done {
			mark = done.Render(" " + theme.Glyph(theme.Done, 0) + " ")
		} else if c.day.Before(today) {
			mark = warn.Render(" ○ ")
		}
		rows = append(rows, ansi.Truncate(mark+b.st.Text.Render(it.Text), w, "…"))
	}
	if c.sessions > 0 {
		rows = append(rows, " "+b.st.Muted.Render(plural(c.sessions, "agent session")))
	}
	if len(rows) > h {
		rows = rows[:h]
	}
	return strings.Join(rows, "\n")
}

func pad2(n int) string {
	if n < 10 {
		return " " + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// openDayTodos opens the todo list with only the todos due or done on day, YYYY-MM-DD; a todo
// added there is due that day.
func (b *browser) openDayTodos(day string) tea.Cmd {
	b.pick = &picker{title: "todo · reading the notes", todos: true, folder: b.plainFolder(), day: day}
	return b.loadTodos("")
}
