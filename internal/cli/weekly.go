package cli

import (
	"errors"
	"fmt"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/w4jnl/nt/internal/weekly"
)

// nt week [date|last] [--json]: compile the week's digest now and print where it is. The machine
// with weekly = true does this on every sync run; this is for any week, by hand.
func runWeek(args []string) int {
	args, asJSON := takeBool(args, "--json")
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
	changed, err := weekly.Write(v, day)
	if err != nil {
		return fail(err)
	}
	rel := v.WeekRel(day)
	filled := isFile(v.Abs(rel)) // a week with nothing in it gets no digest
	if asJSON {
		if !filled {
			return printJSON(map[string]any{"path": "", "rel": "", "changed": false})
		}
		return printJSON(map[string]any{"path": v.Abs(rel), "rel": rel, "changed": changed})
	}
	st := cliStyles(m)
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
