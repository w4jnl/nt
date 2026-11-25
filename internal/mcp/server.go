// Package mcp is nt as a Model Context Protocol server on stdin and stdout (docs/design.md §4,
// The Claude app): the Claude app starts `nt mcp` and calls its tools to read, write and
// reorganize the notes. Every tool is one of nt's own operations (internal/ops, internal/move),
// so it writes as the command line does: under the repository lock, through temp files, with
// nt's names. The server runs no git: after a write it asks for a sync, as an editor does.
package mcp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/w4jnl/nt/internal/ops"
	"github.com/w4jnl/nt/internal/style"
	"github.com/w4jnl/nt/skills"
)

// versions are the protocol versions this server speaks, the newest last. Its tools read the
// same in all of them.
var versions = []string{"2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25"}

// Server answers one client.
type Server struct {
	Version  string        // nt's version
	ReadOnly bool          // only the tools that read
	Kick     func()        // start a sync in the background; nil: none
	Delay    time.Duration // how long after the last write Kick runs; 0: 3 s
	Log      io.Writer     // what went wrong, for the person: stderr

	mu    sync.Mutex
	timer *time.Timer
	out   io.Writer
}

type request struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Serve reads requests from in, one JSON object a line, and writes the answers to out, until in
// ends. Only protocol messages go to out. A sync still waiting to start is started before it
// returns.
func (s *Server) Serve(in io.Reader, out io.Writer) error {
	s.out = out
	defer s.flush()
	r := bufio.NewReaderSize(in, 1<<16)
	for {
		line, err := r.ReadBytes('\n')
		if len(trim(line)) > 0 {
			s.handle(trim(line))
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func trim(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r' || b[len(b)-1] == ' ') {
		b = b[:len(b)-1]
	}
	return b
}

func (s *Server) handle(line []byte) {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		s.reply(json.RawMessage("null"), nil, &rpcError{-32700, "not a JSON-RPC message: " + err.Error()})
		return
	}
	note := len(req.ID) == 0 // a notification: no answer
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		v := versions[len(versions)-1]
		if slices.Contains(versions, p.ProtocolVersion) {
			v = p.ProtocolVersion
		}
		s.reply(req.ID, map[string]any{
			"protocolVersion": v,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "nt", "title": "nt notes", "version": s.Version},
			"instructions":    s.instructions(),
		}, nil)
	case "ping":
		s.reply(req.ID, map[string]any{}, nil)
	case "tools/list":
		list := []map[string]any{}
		for _, t := range s.tools() {
			list = append(list, t.describe())
		}
		s.reply(req.ID, map[string]any{"tools": list}, nil)
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			s.reply(req.ID, nil, &rpcError{-32602, "tools/call: " + err.Error()})
			return
		}
		res, err := s.call(p.Name, p.Arguments)
		if err != nil {
			s.reply(req.ID, nil, err)
			return
		}
		s.reply(req.ID, res, nil)
	default:
		if !note {
			s.reply(req.ID, nil, &rpcError{-32601, "nt mcp has no method " + req.Method})
		}
	}
}

func (s *Server) reply(id json.RawMessage, result any, e *rpcError) {
	if len(id) == 0 {
		return
	}
	msg := map[string]any{"jsonrpc": "2.0", "id": id}
	if e != nil {
		msg["error"] = e
	} else {
		msg["result"] = result
	}
	b, err := json.Marshal(msg)
	if err != nil {
		s.logf("answer: %v", err)
		return
	}
	if _, err := s.out.Write(append(b, '\n')); err != nil {
		s.logf("answer: %v", err)
	}
}

func (s *Server) logf(format string, a ...any) {
	if s.Log != nil {
		fmt.Fprintf(s.Log, "nt mcp: "+format+"\n", a...)
	}
}

// wrote starts a sync a moment after the last write, so a run of writes is one commit.
func (s *Server) wrote() {
	if s.Kick == nil {
		return
	}
	d := s.Delay
	if d == 0 {
		d = 3 * time.Second
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(d, func() {
		s.mu.Lock()
		s.timer = nil
		s.mu.Unlock()
		s.Kick()
	})
}

// settled drops the sync that waits to start, and says whether one did.
func (s *Server) settled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer == nil {
		return false
	}
	stopped := s.timer.Stop()
	s.timer = nil
	return stopped
}

func (s *Server) flush() {
	if s.settled() {
		s.Kick()
	}
}

const instructions = `nt keeps the person's notes: plain markdown files in a git repository, synced to their other devices.

Paths are from the root of the notes, with forward slashes: work/2026-11-12-plan.md. Use list and search to find a note before you read or change it.

- A short fact for today goes to journal_add. Anything longer is a note: note_new takes a title and a folder, and nt names the file YYYY-MM-DD-slug.md.
- Notes are GitHub markdown: [Title](relative/path.md) links, ![alt](assets/x.png) images, tables, task lists. No wikilinks, embeds or callouts. Do not write the title as a heading in the body of a new note: nt writes it.
- Images go with note_new or note_append, in "images". Name each one in the text by its file name, ![alt](chart.svg), on a line of its own; nt saves it beside the note and fixes the link.
- To change a note, read it with note_read, then send the whole new text with note_write and the sha you read. To add to it, use note_append.
- Reorganize with folder_new, move and rename, never by writing a note again elsewhere: they keep every link working. For more than one move, run each with dry_run first and tell the person what will change.
- remove deletes; ask the person before you use it.
- Agent reports and weekly digests are compiled by nt: read them, never write them.
- Writes sync by themselves a few seconds later. Call sync only when the person asks for it now.

How to write, a recommendation and not a rule. When the person asks for another style, or the note already has one, follow that.

- Simplified Technical English: short sentences, one topic in each. Active voice, present tense. Instructions in the imperative.
- Plain words, and the same word for the same thing every time. Write the articles. "must" for a requirement, "can" for a possibility.
- Steps in a numbered list, in the order they are done. Facts that line up in a table. A warning before the step it belongs to.
- A note or warning box is a GitHub alert: a quote whose first line is [!NOTE], [!TIP], [!IMPORTANT], [!WARNING] or [!CAUTION].
- Code, commands, paths, quoted text and the person's names for things stay as they are.

Drawings, a recommendation too. A technical note is easier to follow with a picture of the thing it explains: draw one without being asked when it helps, and only then.

- Draw when the text explains how parts connect, how data or control moves, how something changes in stages or over time, or how a deployment is laid out. A long technical note has one near its top, "in one picture".
- Do not draw for a short answer, a list of facts, plain steps or a journal line: a table or a numbered list is the better form there. One drawing for one idea, under the heading it explains, with a sentence before it that says what it shows.
- Make a designed SVG and give it in "images". Not a flowchart: no mermaid and no ASCII art unless the person asks for one.
- Follow the pattern below: copy its <style> and <defs> as they are (nt's colours, light and dark) and build the drawing from its parts. A canvas 1000 wide and as high as it needs, on the "bg" card; the title at the top left ("t"), one line at the top right that says how to read it ("s e"). A part is a "card" with its name ("h") and one or two short lines ("s"); "key" is the part the note is about, one in a drawing; "out" is a part outside the person's control. Arrows ("a") end short of the cards, with a label by each ("l") and an endpoint, port or command under it in mono ("c"). The warning colour ("w", "bad") is for a fault or a risk only.
- A <title> and a <desc>, text of at least 11 px, nothing that overlaps, room around every card. No other colours, no gradients, no shadows. The alt text says what the picture shows.

The pattern for a drawing:

`

// instructions is what the client reads when it connects: how the notes work and how to write
// and draw in them (nt's recommendations, with the pattern for a drawing), then the person's
// own style: the root's style.md in full, and where the folders' are (docs/format.md §15).
// Style notes in a folder of mcp.hide are left out.
func (s *Server) instructions() string {
	var b strings.Builder
	b.WriteString(instructions)
	b.WriteString(strings.TrimSpace(skills.Drawing))
	v, m, err := ops.Open()
	if err != nil {
		return b.String()
	}
	sc := scope{v: v, hide: Hidden(m)}
	var folders []string
	for _, rel := range style.All(v.Root) {
		switch {
		case sc.hidden(rel):
		case style.Folder(rel) == "":
			if ns := style.For(v.Root, ""); len(ns) == 1 {
				b.WriteString("\n\nThe person's own style, from style.md in the root of the notes. It comes before the recommendations above; where it says nothing, they hold.\n\n")
				b.WriteString(ns[0].Text)
			}
		default:
			folders = append(folders, rel)
		}
	}
	if len(folders) > 0 {
		b.WriteString("\n\nThese folders have a style.md of their own. Before you write a note in one, or in a folder under it, read it with note_read: it comes before the root's style and the recommendations.\n")
		for _, rel := range folders {
			b.WriteString("\n- " + rel)
		}
	}
	if !s.ReadOnly {
		b.WriteString("\n\nWhen the person tells you how they want their notes written or drawn from now on, say that a note named style.md, in the root of the notes or in a folder, keeps it for every agent. When it is there, add to it with note_append.")
	}
	return b.String()
}
