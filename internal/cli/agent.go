package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/w4jnl/nt/internal/agents"
	"github.com/w4jnl/nt/internal/config"
	"github.com/w4jnl/nt/internal/theme"
	"github.com/w4jnl/nt/skills"
)

// nt agent import|write.
func runAgent(args []string) int {
	if len(args) == 0 {
		return fail(errors.New("agent: use import (flok) or write"))
	}
	switch args[0] {
	case "import":
		return runAgentImport(args[1:])
	case "write":
		return runAgentWrite(args[1:])
	case "start", "turn", "end":
		return runOldHook()
	}
	return fail(fmt.Errorf("agent: unknown %q: use import or write", args[0]))
}

// nt agent start|turn|end: what nt's own hooks ran before flok wrote the reports. It reads the
// hook's payload and does nothing, so a machine that still has them sees no error; nt doctor
// warns about them and nt uninstall --hooks takes them out.
func runOldHook() int {
	if !stdinIsTerminal() {
		_, _ = io.Copy(io.Discard, io.LimitReader(os.Stdin, 4<<20))
	}
	return 0
}

// nt agent import [--probe]: flok's record of one agent session, JSON on stdin, written as the
// session's report (agents.Import). It prints nothing and exits 0; on a failure it says why on
// one line and exits 1, or 2 for a record newer than this nt. A session in a temporary folder is
// skipped. The end of a session starts a sync. --probe prints the newest record version nt reads.
func runAgentImport(args []string) int {
	args, probe := takeBool(args, "--probe")
	if _, err := positional("agent import", args); err != nil {
		return fail(err)
	}
	if probe {
		fmt.Println(agents.RecordVersion)
		return 0
	}
	if stdinIsTerminal() {
		return report(errors.New("agent import: give flok's session record on stdin"))
	}
	b, err := io.ReadAll(io.LimitReader(os.Stdin, 4<<20))
	if err != nil {
		return report(fmt.Errorf("agent import: %w", err))
	}
	rec, err := agents.ParseRecord(b)
	if errors.Is(err, agents.ErrNewerRecord) {
		return fail(fmt.Errorf("agent import: %w", err))
	} else if err != nil {
		return report(fmt.Errorf("agent import: %w", err))
	}
	if agents.Temporary(rec.Cwd) && os.Getenv("NT_REPORT_TEMP") == "" {
		return 0 // a throwaway session
	}
	v, m, err := openVault()
	if err != nil {
		return report(fmt.Errorf("agent import: %w", err))
	}
	now := v.Now()
	_, ended, err := agents.Import(v, config.StateDir(), rec, now)
	if err != nil {
		return report(fmt.Errorf("agent import: %w", err))
	}
	_ = agents.MarkImported(config.StateDir(), now)
	if ended {
		kickSync(m)
	}
	return 0
}

// nt agent write [--session id] [--agent name] [--status s] [--summary text]: the agent's own
// account of its session, read on stdin as markdown under ## Done, ## Decisions and ## Open.
// Without --session it fills the latest report whose session started in this folder or one
// above it; with none, it makes one.
func runAgentWrite(args []string) int {
	args, asJSON := takeBool(args, "--json")
	var sel agents.Selector
	var status, summary string
	for _, f := range []struct {
		name string
		to   *string
	}{{"--session", &sel.Session}, {"--agent", &sel.Agent}, {"--status", &status}, {"--summary", &summary}} {
		var err error
		if args, *f.to, _, err = takeValue(args, f.name); err != nil {
			return fail(fmt.Errorf("agent write: %w", err))
		}
	}
	if _, err := positional("agent write", args); err != nil {
		return fail(err)
	}
	var text []byte
	if !stdinIsTerminal() {
		var err error
		if text, err = io.ReadAll(io.LimitReader(os.Stdin, 1<<20)); err != nil {
			return fail(err)
		}
	}
	if len(strings.TrimSpace(string(text))) == 0 && status == "" && summary == "" {
		return fail(errors.New("agent write: give the report on stdin (## Done, ## Decisions, ## Open), --status or --summary"))
	}
	v, m, err := openVault()
	if err != nil {
		return fail(err)
	}
	sel.Cwd, _ = os.Getwd()
	rel, err := agents.Write(v, config.StateDir(), sel, string(text), status, summary, v.Now())
	if err != nil {
		return fail(fmt.Errorf("agent write: %w", err))
	}
	if asJSON {
		return printJSON(map[string]any{"path": v.Abs(rel), "rel": rel})
	}
	st := cliStyles(m)
	lipgloss.Println(st.Muted.Render("wrote ") + st.Text.Render(rel))
	return 0
}

// nt digest [date] [--json]: write the day's agents/YYYY/YYYY-MM-DD/README.md from its reports
// and print them. date is YYYY-MM-DD, today or yesterday; the default is today.
func runDigest(args []string) int {
	args, asJSON := takeBool(args, "--json")
	pos, err := positional("digest", args)
	if err != nil {
		return fail(err)
	}
	v, m, err := openVault()
	if err != nil {
		return fail(err)
	}
	day := v.Day(v.Now())
	if len(pos) > 0 {
		switch pos[0] {
		case "today":
		case "yesterday":
			day = day.AddDate(0, 0, -1)
		default:
			if day, err = time.ParseInLocation("2006-01-02", pos[0], time.Local); err != nil {
				return fail(fmt.Errorf("digest: %q is not a date: use YYYY-MM-DD, today or yesterday", pos[0]))
			}
		}
	}
	changed, err := agents.Digest(v, day)
	if err != nil {
		return fail(err)
	}
	lines := agents.Day(v, day)
	rel := v.AgentsRel(day) + "/README.md"
	if asJSON {
		if lines == nil {
			lines = []agents.Line{}
		}
		return printJSON(map[string]any{"path": v.Abs(rel), "rel": rel, "changed": changed, "reports": lines})
	}
	st, pal := cliStyles(m), cliPalette(m)
	if len(lines) == 0 {
		lipgloss.Println(st.Muted.Render("no agent reports on " + day.Format("Monday 2 January 2006")))
		return 0
	}
	project := "\x00"
	for _, l := range lines {
		if l.Project != project {
			project = l.Project
			lipgloss.Println(st.Text.Render(project))
		}
		s := theme.State(l.Status)
		mark := lipgloss.NewStyle().Foreground(pal.Color(s)).Render(theme.Glyph(s, 0) + " " + l.Status)
		lipgloss.Println("  " + mark + st.Muted.Render(" · "+l.Agent+" · "+l.Host+" · ") + st.Text.Render(l.Summary) + st.Faint.Render("  "+l.Rel))
	}
	verb := "unchanged"
	if changed {
		verb = "wrote"
	}
	lipgloss.Println(st.Faint.Render(verb + " " + rel))
	return 0
}

// removeHooks takes nt's own hooks out of Claude Code's settings and Copilot CLI's hooks
// directory: flok writes the reports now.
func removeHooks() error {
	cp, cpp := agents.ClaudeSettingsPath(), agents.CopilotHooksPath()
	claude, err := agents.RemoveClaudeHooks(cp)
	if err != nil {
		return err
	}
	copilot, err := agents.RemoveCopilotHooks(cpp)
	if err != nil {
		return err
	}
	var from []string
	if claude {
		from = append(from, tilde(cp))
	}
	if copilot {
		from = append(from, tilde(cpp))
	}
	if len(from) == 0 {
		fmt.Println("no hooks of nt's to remove")
	} else {
		fmt.Printf("removed nt's hooks from %s; flok writes the session reports\n", strings.Join(from, " and "))
	}
	return nil
}

// installSkill writes the agent skill and links it for Claude Code and Copilot CLI, or removes it.
func installSkill(remove bool) error {
	dir, links := skills.Dir(), skills.Links()
	if remove {
		if err := skills.Remove(dir, links); err != nil {
			return err
		}
		fmt.Println("removed the nt skill")
		return nil
	}
	if err := skills.Install(dir, links); err != nil {
		return err
	}
	var at []string
	for _, l := range links {
		at = append(at, tilde(l))
	}
	fmt.Printf("installed the nt skill in %s, linked from %s\n", tilde(dir), strings.Join(at, " and "))
	return nil
}
