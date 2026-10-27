package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/w4jnl/nt/internal/config"
	"github.com/w4jnl/nt/internal/weekly"
)

// nt week [date|last] [--summarize] [--json]: compile the week's digest now and print where it
// is. The machine with weekly = true does this on every sync run; this is for any week, by hand.
// --summarize first has this machine's [summary] tool write its summary of the week, again if
// there is one.
func runWeek(args []string) int {
	args, asJSON := takeBool(args, "--json")
	args, summarize := takeBool(args, "--summarize")
	pos, err := positional("week", args)
	if err != nil {
		return fail(err)
	}
	if len(pos) > 1 {
		return fail(errors.New("week: name one day of the week: YYYY-MM-DD, last or this"))
	}
	v, m, err := openVault()
	if err != nil {
		return fail(err)
	}
	day := v.Day(v.Now())
	if len(pos) == 1 {
		switch pos[0] {
		case "this":
		case "last":
			day = day.AddDate(0, 0, -7)
		default:
			if day, err = time.ParseInLocation("2006-01-02", pos[0], time.Local); err != nil {
				return fail(fmt.Errorf("week: %q is not a date: use YYYY-MM-DD, last or this", pos[0]))
			}
		}
	}
	summary := ""
	if summarize {
		if m.Summary.Tool == "" {
			return fail(errors.New(`week: no tool writes this machine's summaries: set [summary] tool = "claude" or "copilot" in ` + tilde(config.MachinePath())))
		}
		if summary, err = weekly.Summarize(context.Background(), v, weekly.Tool{Name: m.Summary.Tool, Model: m.Summary.Model}, day); err != nil {
			return fail(fmt.Errorf("week: %w", err))
		}
	}
	changed, err := weekly.Write(v, day)
	if err != nil {
		return fail(err)
	}
	rel := v.WeekRel(day)
	filled := isFile(v.Abs(rel)) // a week with nothing in it gets no digest
	if asJSON {
		out := map[string]any{"path": "", "rel": "", "changed": false, "summary": summary}
		if filled {
			out["path"], out["rel"], out["changed"] = v.Abs(rel), rel, changed
		}
		return printJSON(out)
	}
	st := cliStyles(m)
	switch {
	case summarize && summary == "":
		lipgloss.Println(st.Muted.Render("nothing was written on " + v.Host + " that week: no summary"))
	case summarize:
		lipgloss.Println(st.Muted.Render("wrote ") + st.Text.Render(summary))
	}
	switch {
	case !filled:
		lipgloss.Println(st.Muted.Render("nothing in the week of " + day.Format("Monday 2 January 2006")))
	case changed:
		lipgloss.Println(st.Muted.Render("wrote ") + st.Text.Render(rel))
	default:
		lipgloss.Println(st.Muted.Render("unchanged ") + st.Text.Render(rel))
	}
	return 0
}
