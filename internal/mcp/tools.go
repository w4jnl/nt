package mcp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/w4jnl/nt/internal/move"
	"github.com/w4jnl/nt/internal/ops"
	"github.com/w4jnl/nt/internal/render"
	"github.com/w4jnl/nt/internal/todo"
	"github.com/w4jnl/nt/internal/vault"
)

// tool is one thing the client can ask for.
type tool struct {
	name, title, about string
	props              map[string]any // the input's properties, by name
	required           []string
	write              bool // changes the notes: left out of a read-only server, and a sync follows
	destroys           bool // can delete or move what is there
	run                func(sc scope, args json.RawMessage) (any, error)
}

// text is an answer whose second part is a note's text as it is, not inside JSON.
type text struct {
	meta any
	body string
}

func (t tool) describe() map[string]any {
	schema := map[string]any{"type": "object", "properties": t.props, "additionalProperties": false}
	if len(t.props) == 0 {
		schema["properties"] = map[string]any{}
	}
	if len(t.required) > 0 {
		schema["required"] = t.required
	}
	return map[string]any{
		"name": t.name, "title": t.title, "description": t.about, "inputSchema": schema,
		"annotations": map[string]any{"title": t.title, "readOnlyHint": !t.write, "destructiveHint": t.destroys, "openWorldHint": false},
	}
}

// call runs the tool name with args. What a tool refuses is an answer with isError, which the
// model reads; only a name that is no tool is a protocol error.
func (s *Server) call(name string, args json.RawMessage) (any, *rpcError) {
	for _, t := range s.tools() {
		if t.name != name {
			continue
		}
		res, err := s.run(t, args)
		if err != nil {
			return map[string]any{"content": []any{block(err.Error())}, "isError": true}, nil
		}
		switch {
		case t.name == "sync":
			s.settled() // it just ran
		case t.write:
			s.wrote()
		}
		if tx, ok := res.(text); ok {
			return map[string]any{"content": []any{block(pretty(tx.meta)), block(tx.body)}}, nil
		}
		return map[string]any{"content": []any{block(pretty(res))}}, nil
	}
	return nil, &rpcError{-32602, "nt mcp has no tool " + name}
}

func (s *Server) run(t tool, args json.RawMessage) (any, error) {
	v, m, err := ops.Open()
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(args)) == 0 {
		args = json.RawMessage("{}")
	}
	return t.run(scope{v: v, hide: Hidden(m)}, args)
}

func block(s string) map[string]any { return map[string]any{"type": "text", "text": s} }

func pretty(v any) string {
	if raw, ok := v.(json.RawMessage); ok {
		return string(bytes.TrimSpace(raw))
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

// in reads a tool's arguments into v; a key the tool does not take is an error the model can fix.
func in(args json.RawMessage, v any) error {
	d := json.NewDecoder(bytes.NewReader(args))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return fmt.Errorf("the arguments are not what this tool takes: %w", err)
	}
	return nil
}

func str(about string) map[string]any { return map[string]any{"type": "string", "description": about} }
func flag(about string) map[string]any {
	return map[string]any{"type": "boolean", "description": about}
}
func count(about string) map[string]any {
	return map[string]any{"type": "integer", "minimum": 1, "description": about}
}

var imagesProp = map[string]any{
	"type":        "array",
	"description": "Image files to save beside the note (svg, png, jpg, gif, webp; at most 5 MB each). In the text, show each one on a line of its own as ![what it shows](file), with the same file name as here; nt saves it under assets/ and fixes the link. An image the text does not name is added at the end.",
	"items": map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"file", "data"},
		"properties": map[string]any{
			"file":   str("The file's name with its extension, e.g. price-vs-delta.svg. It becomes assets/YYYY-MM-DD-price-vs-delta.svg."),
			"data":   str("The file's content: an SVG's text as it is, any other image as base64."),
			"base64": flag("True when data is base64. Needed for every image that is not an SVG."),
		},
	},
}

const pathAbout = "The note's path from the root of the notes, e.g. work/2026-11-12-plan.md, as list and search give it."

// tools are the tools this server has: all of them, or with ReadOnly the ones that read.
func (s *Server) tools() []tool {
	all := []tool{
		{
			name: "list", title: "List notes and folders",
			about: "The folders and the notes in a folder of the person's notes, the newest note first, each note with its title. Start at the root (no folder) to see the top folders. With recursive, everything under the folder: use it to see a whole structure before reorganizing it.",
			props: map[string]any{
				"folder":    str("The folder, from the root of the notes, e.g. work or private/health. Empty: the root."),
				"recursive": flag("Also what is in the folders under it."),
				"limit":     count("The most notes to return. Default 100."),
			},
			run: list,
		},
		{
			name: "search", title: "Search the notes",
			about: "Find the lines that hold the words of query in every note. Case counts only when the query has capitals. A word that starts with # keeps to the notes with that tag: '#trading delta'. Each match has the note's path, the line number and the line.",
			props: map[string]any{
				"query": str("The words to find, or a regular expression."),
				"limit": count("The most matches to return. Default 20."),
			},
			required: []string{"query"},
			run:      search,
		},
		{
			name: "note_read", title: "Read a note",
			about:    "Read one note: its markdown as it is in the file, its title, and its sha, the version to give note_write when you change it.",
			props:    map[string]any{"path": str(pathAbout)},
			required: []string{"path"},
			run:      noteRead,
		},
		{
			name: "todo_list", title: "List todos",
			about: "The person's todos, the ones in progress first, then overdue, then by due date. Each has todo, the name to give todo_change (path:line), its text, due date, and whether it is in progress or done.",
			props: map[string]any{
				"folder": str("Only the todos under this folder. Empty: all of them."),
				"show":   map[string]any{"type": "string", "enum": []string{"open", "done", "all"}, "description": "Which ones. Default open."},
			},
			run: todoList,
		},
		{
			name: "journal_add", title: "Add to today's journal", write: true,
			about:    "Add a timestamped line to today's journal. For a short fact, decision or thing that happened, which the person finds in their day. Longer text is a note: use note_new.",
			props:    map[string]any{"text": str("The line, without the time: nt writes it.")},
			required: []string{"text"},
			run:      journalAdd,
		},
		{
			name: "note_new", title: "New note", write: true,
			about: "Make a new note. nt names the file YYYY-MM-DD-<title as a slug>.md in the folder, writes the frontmatter and the title heading, and links the note from today's journal. Give the text in body, without the title heading. A folder that is not there yet is made.",
			props: map[string]any{
				"title":  str("The note's title, as a person would write it: Cumulative delta vs price."),
				"folder": str("The folder for it, from the root of the notes, e.g. trading or work/reviews. Empty: the person's default folder (inbox). list shows the folders there are; prefer one of them."),
				"body":   str("The note's markdown, after the title."),
				"tags":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tags for the frontmatter, lowercase words: trading, order-flow."},
				"images": imagesProp,
			},
			required: []string{"title"},
			run:      noteNew,
		},
		{
			name: "note_append", title: "Add to a note", write: true,
			about: "Add markdown to a note that is there: at its end, or at the end of the section under a heading. The rest of the note stays as it is.",
			props: map[string]any{
				"path":    str(pathAbout),
				"text":    str("The markdown to add."),
				"heading": str("The heading of the section to add it to, without the #, e.g. Decisions. Empty: the end of the note."),
				"images":  imagesProp,
			},
			required: []string{"path", "text"},
			run:      noteAppend,
		},
		{
			name: "note_write", title: "Rewrite a note", write: true, destroys: true,
			about: "Replace a note's whole text, to edit it. Read it with note_read first, change what must change and keep the rest, the frontmatter too, then give the full new text with the sha you read. When the note changed in between, nothing is written: read it again. To only add text, use note_append.",
			props: map[string]any{
				"path": str(pathAbout),
				"text": str("The note's full new markdown, frontmatter included."),
				"sha":  str("The sha note_read gave for the text you changed."),
			},
			required: []string{"path", "text", "sha"},
			run:      noteWrite,
		},
		{
			name: "todo_add", title: "Add a todo", write: true,
			about: "Add a todo for the person, to their todo list or to a folder's.",
			props: map[string]any{
				"text":   str("What is to do."),
				"due":    str("When it is due: YYYY-MM-DD, today, tomorrow, mon to sun, +3d, +2w. Empty: no date."),
				"folder": str("Put it in this folder's todo list, e.g. work. Empty: the main list."),
			},
			required: []string{"text"},
			run:      todoAdd,
		},
		{
			name: "todo_change", title: "Change a todo", write: true,
			about: "Change one todo: done checks it, undo opens it again, start marks it in progress, stop takes that back, due sets or changes its date (when: none removes the date), remove deletes it.",
			props: map[string]any{
				"todo":   str("The todo: path:line as todo_list gives it, or words from its text when only one todo has them."),
				"action": map[string]any{"type": "string", "enum": []string{"done", "undo", "start", "stop", "due", "remove"}, "description": "What to do with it."},
				"when":   str("For due: YYYY-MM-DD, today, tomorrow, mon to sun, +3d, +2w, or none to remove the date."),
			},
			required: []string{"todo", "action"},
			run:      todoChange,
		},
		{
			name: "folder_new", title: "New folder", write: true,
			about:    "Make a folder for notes, with the folders above it when they are missing: private/health/2026. Names become lowercase slugs.",
			props:    map[string]any{"path": str("The folder, from the root of the notes.")},
			required: []string{"path"},
			run:      folderNew,
		},
		{
			name: "move", title: "Move a note or folder", write: true, destroys: true,
			about: "Move a note, or a folder with all it holds, to another place in the notes. nt fixes every link to it and in it, and a note's images go along. The folders it needs are made. Journals, agent reports and digests do not move. With dry_run nothing changes and the answer says what would: use it first when you move more than one thing.",
			props: map[string]any{
				"from":    str("The note or folder to move, from the root of the notes."),
				"to":      str("Where to: a folder to move it into, ending in /, e.g. private/health/; or the full new path."),
				"dry_run": flag("Only say what would change."),
			},
			required: []string{"from", "to"},
			run:      moveTool,
		},
		{
			name: "rename", title: "Rename a note or folder", write: true, destroys: true,
			about: "Give a note a new title, and the file name that goes with it (its date stays), or a folder a new name. nt fixes every link. With dry_run nothing changes and the answer says what would.",
			props: map[string]any{
				"path":    str("The note or folder, from the root of the notes."),
				"name":    str("A note's new title, as a person writes it, or a folder's new name."),
				"dry_run": flag("Only say what would change."),
			},
			required: []string{"path", "name"},
			run:      renameTool,
		},
		{
			name: "remove", title: "Delete a note or empty folder", write: true, destroys: true,
			about: "Delete a note, with the images only it uses and its line in its day's journal, or a folder that is empty. Ask the person first. git keeps the history, so a synced note can be brought back. With dry_run nothing changes and the answer says what would go and how many notes link to it.",
			props: map[string]any{
				"path":    str("The note or empty folder, from the root of the notes."),
				"dry_run": flag("Only say what would go."),
			},
			required: []string{"path"},
			run:      removeTool,
		},
		{
			name: "sync", title: "Sync now", write: true,
			about: "Commit the notes, merge with the person's other devices and push, now. Writes sync by themselves a few seconds later, so call this only when the person asks to sync.",
			run: func(scope, json.RawMessage) (any, error) {
				out, _, err := ops.Runner{}.Run([]string{"sync"}, nil)
				if err != nil {
					return nil, err
				}
				return json.RawMessage(out), nil
			},
		},
	}
	if !s.ReadOnly {
		return all
	}
	var reads []tool
	for _, t := range all {
		if !t.write {
			reads = append(reads, t)
		}
	}
	return reads
}

// MARK: Reading

type listed struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}

func list(sc scope, args json.RawMessage) (any, error) {
	var a struct {
		Folder    string `json:"folder"`
		Recursive bool   `json:"recursive"`
		Limit     int    `json:"limit"`
	}
	if err := in(args, &a); err != nil {
		return nil, err
	}
	folder, err := sc.rel(a.Folder)
	if err != nil {
		return nil, err
	}
	if st, err := os.Stat(sc.v.Abs(folder)); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("%s: no such folder; list without a folder shows the top ones", folder)
	}
	if a.Limit <= 0 {
		a.Limit = 100
	}
	folders, notes := []string{}, []listed{}
	var walk func(dir string) error
	walk = func(dir string) error {
		entries, err := os.ReadDir(sc.v.Abs(dir))
		if err != nil {
			return err
		}
		var sub []string
		for _, e := range entries {
			name := e.Name()
			rel := path.Join(dir, name)
			if strings.HasPrefix(name, ".") || sc.hidden(rel) {
				continue
			}
			switch {
			case e.IsDir() && name == "assets": // the notes' images, not a place for notes
			case e.IsDir():
				sub = append(sub, rel)
			case strings.HasSuffix(name, ".md"):
				notes = append(notes, listed{Path: rel})
			}
		}
		sort.Slice(sub, func(i, j int) bool { return vault.FolderBefore(path.Base(sub[i]), path.Base(sub[j])) })
		for _, d := range sub {
			folders = append(folders, d+"/")
			if a.Recursive {
				if err := walk(d); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(folder); err != nil {
		return nil, err
	}
	// newest first: notes are named by their date
	sort.SliceStable(notes, func(i, j int) bool { return path.Base(notes[i].Path) > path.Base(notes[j].Path) })
	more := 0
	if len(notes) > a.Limit {
		more, notes = len(notes)-a.Limit, notes[:a.Limit]
	}
	for i := range notes {
		notes[i].Title = titleOf(sc.v.Abs(notes[i].Path))
	}
	out := map[string]any{"folder": folder, "folders": folders, "notes": notes}
	if more > 0 {
		out["more"] = fmt.Sprintf("%d older notes are not shown: raise limit, or list a folder under this one", more)
	}
	return out, nil
}

// titleOf is a note's title, read from its head.
func titleOf(abs string) string {
	f, err := os.Open(abs)
	if err != nil {
		return ""
	}
	defer f.Close()
	head := make([]byte, 4096)
	n, _ := f.Read(head)
	return render.Title(head[:n], abs)
}

func search(sc scope, args json.RawMessage) (any, error) {
	var a struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := in(args, &a); err != nil {
		return nil, err
	}
	if strings.TrimSpace(a.Query) == "" {
		return nil, errors.New("give the words to find in query")
	}
	if a.Limit <= 0 {
		a.Limit = 20
	}
	found, err := ops.Search(sc.v, a.Query)
	if err != nil {
		return nil, err
	}
	type match struct {
		Path string `json:"path"`
		Line int    `json:"line"`
		Text string `json:"text"`
	}
	matches, more := []match{}, 0
	for _, m := range found {
		if sc.hidden(m.Path) {
			continue
		}
		if len(matches) == a.Limit {
			more++
			continue
		}
		matches = append(matches, match{m.Path, m.Line, m.Text})
	}
	out := map[string]any{"matches": matches}
	if more > 0 {
		out["more"] = fmt.Sprintf("%d more lines match: raise limit or add a word", more)
	}
	return out, nil
}

func noteRead(sc scope, args json.RawMessage) (any, error) {
	var a struct {
		Path string `json:"path"`
	}
	if err := in(args, &a); err != nil {
		return nil, err
	}
	rel, err := sc.note(a.Path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(sc.v.Abs(rel))
	if err != nil {
		return nil, err
	}
	if len(data) > maxNote {
		return nil, fmt.Errorf("%s is %d KB, more than this server reads (1 MB)", rel, len(data)>>10)
	}
	return text{
		meta: map[string]any{"path": rel, "title": render.Title(data, rel), "sha": ops.SHA256(data)},
		body: string(data),
	}, nil
}

type todoOut struct {
	Todo    string `json:"todo"` // path:line, the name todo_change takes
	Text    string `json:"text"`
	Due     string `json:"due,omitempty"`
	When    string `json:"when,omitempty"`
	Late    bool   `json:"late,omitempty"`
	Doing   bool   `json:"in_progress,omitempty"`
	Started string `json:"started,omitempty"`
	Done    bool   `json:"done,omitempty"`
	DoneOn  string `json:"done_on,omitempty"`
	Section string `json:"section,omitempty"`
}

func todoOf(sc scope, it todo.Item) todoOut {
	t := ops.TodoOf(sc.v, it)
	return todoOut{
		Todo: fmt.Sprintf("%s:%d", t.Rel, t.Line), Text: t.Text, Due: t.Due, When: t.When, Late: t.Late,
		Doing: t.Doing, Started: t.Started, Done: t.Done, DoneOn: t.DoneOn, Section: t.Section,
	}
}

func todoList(sc scope, args json.RawMessage) (any, error) {
	var a struct {
		Folder string `json:"folder"`
		Show   string `json:"show"`
	}
	if err := in(args, &a); err != nil {
		return nil, err
	}
	folder, err := sc.rel(a.Folder)
	if err != nil {
		return nil, err
	}
	if a.Show != "" && a.Show != "open" && a.Show != "done" && a.Show != "all" {
		return nil, fmt.Errorf("show is open, done or all, not %q", a.Show)
	}
	items, err := ops.Todos(sc.v, folder, a.Show == "done", a.Show == "all")
	if err != nil {
		return nil, err
	}
	out := []todoOut{}
	for _, it := range items {
		if !sc.hidden(it.Path) {
			out = append(out, todoOf(sc, it))
		}
	}
	return map[string]any{"todos": out}, nil
}

// MARK: Writing

func journalAdd(sc scope, args json.RawMessage) (any, error) {
	var a struct {
		Text string `json:"text"`
	}
	if err := in(args, &a); err != nil {
		return nil, err
	}
	if strings.TrimSpace(a.Text) == "" {
		return nil, errors.New("give the line to add in text")
	}
	if len(a.Text) > maxNote {
		return nil, errors.New("that is more than a journal line takes: make a note with note_new")
	}
	if sc.hidden(sc.v.Cfg.Folders.Daily) {
		return nil, fmt.Errorf("%s is kept from the Claude app on this machine (the setting mcp.hide)", sc.v.Cfg.Folders.Daily)
	}
	l, err := ops.Append(sc.v, a.Text)
	if err != nil {
		return nil, err
	}
	return map[string]any{"path": l.Rel, "line": l.Line}, nil
}

// image is one image a tool takes with a note's text.
type image struct {
	File   string `json:"file"`
	Data   string `json:"data"`
	Base64 bool   `json:"base64"`

	raw []byte
}

// check reads the images' data and refuses what nt does not keep, before anything is written.
func check(images []image) error {
	seen := map[string]bool{}
	for i := range images {
		im := &images[i]
		im.File = path.Base(strings.TrimSpace(strings.ReplaceAll(im.File, `\`, "/")))
		ext := strings.ToLower(path.Ext(im.File))
		if !ops.ImageTypes[ext] {
			return fmt.Errorf("%s is not an image nt keeps: svg, png, jpg, gif or webp", im.File)
		}
		if seen[im.File] {
			return fmt.Errorf("two images are named %s: give each its own name", im.File)
		}
		seen[im.File] = true
		switch {
		case im.Base64:
			raw, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(im.Data), ""))
			if err != nil {
				return fmt.Errorf("%s: data is not base64: %w", im.File, err)
			}
			im.raw = raw
		case ext == ".svg":
			im.raw = []byte(im.Data)
		default:
			return fmt.Errorf("%s: give its data as base64, with base64 true", im.File)
		}
		if len(im.raw) == 0 {
			return fmt.Errorf("%s has no data", im.File)
		}
		if len(im.raw) > maxImage {
			return fmt.Errorf("%s is %d MB, more than the 5 MB an image can be", im.File, len(im.raw)>>20)
		}
	}
	return nil
}

// attach saves the images beside the note and returns md with each one's link pointing at its
// file, the lines of the images md does not name added at its end, and the files' paths.
func attach(sc scope, note, md string, images []image) (string, []string, error) {
	var saved []string
	for _, im := range images {
		ext := path.Ext(im.File)
		name := strings.TrimSuffix(im.File, ext)
		rel, err := sc.v.SaveAsset(note, name, ext, im.raw, sc.v.Now())
		if err != nil {
			return md, saved, err
		}
		saved = append(saved, rel)
		link := vault.RelLink(note, rel)
		named := false
		for _, was := range []string{im.File, "./" + im.File, "assets/" + im.File, "./assets/" + im.File} {
			if strings.Contains(md, "]("+was+")") {
				md, named = strings.ReplaceAll(md, "]("+was+")", "]("+link+")"), true
			}
		}
		if !named {
			if md != "" && !strings.HasSuffix(md, "\n") {
				md += "\n"
			}
			if md != "" {
				md += "\n"
			}
			md += "![" + strings.NewReplacer("[", "", "]", "", "-", " ", "_", " ").Replace(name) + "](" + link + ")\n"
		}
	}
	return md, saved, nil
}

func noteNew(sc scope, args json.RawMessage) (any, error) {
	var a struct {
		Title  string   `json:"title"`
		Folder string   `json:"folder"`
		Body   string   `json:"body"`
		Tags   []string `json:"tags"`
		Images []image  `json:"images"`
	}
	if err := in(args, &a); err != nil {
		return nil, err
	}
	a.Title = strings.TrimSpace(a.Title)
	if a.Title == "" || strings.ContainsAny(a.Title, "\n\r") {
		return nil, errors.New("give the note a title of one line")
	}
	if len(a.Body) > maxNote {
		return nil, errors.New("the body is more than the 1 MB a note can be")
	}
	folder, err := sc.rel(a.Folder)
	if err != nil {
		return nil, err
	}
	if folder == "" {
		if folder, err = sc.rel(sc.v.Cfg.DefaultFolder); err != nil {
			return nil, err
		}
	}
	for _, f := range []string{sc.v.Cfg.Folders.Daily, sc.v.Cfg.Folders.Agents, sc.v.Cfg.Folders.Weekly} {
		if folder == f || strings.HasPrefix(folder, f+"/") {
			return nil, fmt.Errorf("%s/ is nt's own folder: put the note in another one, and use journal_add for today's journal", f)
		}
	}
	if err := check(a.Images); err != nil {
		return nil, err
	}
	if st, err := os.Stat(sc.v.Abs(folder)); err != nil || !st.IsDir() {
		if folder, err = move.Mkdir(sc.v, folder); err != nil {
			return nil, err
		}
	}
	n, err := ops.New(sc.v, a.Title, folder)
	if n.Rel == "" {
		return nil, err
	}
	out := map[string]any{"path": n.Rel, "title": a.Title}
	if err != nil { // the note is there; only the journal's link is not
		out["journal"] = "not linked from today's journal: " + err.Error()
	}
	body, saved, err := attach(sc, n.Rel, withoutTitle(a.Body, a.Title), a.Images)
	if err != nil {
		return nil, fmt.Errorf("%s is made, but an image is not saved: %w", n.Rel, err)
	}
	if len(saved) > 0 {
		out["images"] = saved
	}
	err = sc.v.Update(n.Rel, func(old []byte) ([]byte, error) {
		doc := withTags(string(old), a.Tags)
		if strings.TrimSpace(body) == "" {
			return []byte(doc), nil
		}
		return []byte(strings.TrimRight(doc, "\n") + "\n\n" + strings.TrimRight(body, "\n") + "\n"), nil
	})
	if err != nil {
		return nil, fmt.Errorf("%s is made, but its text is not written: %w", n.Rel, err)
	}
	return out, nil
}

// withoutTitle is body without a first heading that only says the title again: nt wrote it.
func withoutTitle(body, title string) string {
	rest := strings.TrimLeft(body, "\n\r \t")
	first, after, _ := strings.Cut(rest, "\n")
	if h, ok := strings.CutPrefix(strings.TrimSpace(first), "# "); ok && strings.EqualFold(strings.TrimSpace(h), title) {
		return strings.TrimLeft(after, "\n\r")
	}
	return body
}

// withTags is the note doc with tags in its frontmatter, when it has one and no tags yet.
func withTags(doc string, tags []string) string {
	var slugs []string
	for _, t := range tags {
		if s := vault.Slug(strings.TrimPrefix(strings.TrimSpace(t), "#")); s != "" {
			slugs = append(slugs, s)
		}
	}
	if len(slugs) == 0 || !strings.HasPrefix(doc, "---\n") {
		return doc
	}
	end := strings.Index(doc[4:], "\n---")
	if end < 0 {
		return doc
	}
	front := doc[:4+end+1]
	if strings.HasPrefix(front[4:], "tags:") || strings.Contains(front, "\ntags:") {
		return doc
	}
	return front + "tags: [" + strings.Join(slugs, ", ") + "]\n" + doc[4+end+1:]
}

func noteAppend(sc scope, args json.RawMessage) (any, error) {
	var a struct {
		Path    string  `json:"path"`
		Text    string  `json:"text"`
		Heading string  `json:"heading"`
		Images  []image `json:"images"`
	}
	if err := in(args, &a); err != nil {
		return nil, err
	}
	rel, err := sc.note(a.Path)
	if err != nil {
		return nil, err
	}
	if err := sc.compiled(rel); err != nil {
		return nil, err
	}
	if strings.TrimSpace(a.Text) == "" && len(a.Images) == 0 {
		return nil, errors.New("give the markdown to add in text")
	}
	if err := check(a.Images); err != nil {
		return nil, err
	}
	old, err := os.ReadFile(sc.v.Abs(rel))
	if err != nil {
		return nil, err
	}
	if len(old)+len(a.Text) > maxNote {
		return nil, errors.New("the note would be more than the 1 MB a note can be")
	}
	if _, err := appendTo(string(old), a.Text, a.Heading); err != nil {
		return nil, err // before an image is saved
	}
	add, saved, err := attach(sc, rel, a.Text, a.Images)
	if err != nil {
		return nil, err
	}
	var sha string
	err = sc.v.Update(rel, func(cur []byte) ([]byte, error) {
		if cur == nil {
			return nil, fmt.Errorf("%s is gone", rel)
		}
		doc, err := appendTo(string(cur), add, a.Heading)
		sha = ops.SHA256([]byte(doc))
		return []byte(doc), err
	})
	if err != nil {
		return nil, err
	}
	out := map[string]any{"path": rel, "sha": sha}
	if len(saved) > 0 {
		out["images"] = saved
	}
	return out, nil
}

// appendTo is doc with add at its end, or at the end of the section under heading: up to the
// next heading of its level or above. Headings in the frontmatter and in code are not headings.
func appendTo(doc, add, heading string) (string, error) {
	add = strings.Trim(add, "\n")
	var lines []string
	if strings.TrimSpace(doc) != "" {
		lines = strings.Split(strings.TrimRight(doc, "\n"), "\n")
	}
	at := len(lines)
	if want := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(heading), "#")); want != "" {
		level, found, fence := 0, -1, false
		var names []string
		start := 0
		if len(lines) > 0 && lines[0] == "---" {
			for i := 1; i < len(lines); i++ {
				if lines[i] == "---" {
					start = i + 1
					break
				}
			}
		}
		for i := start; i < len(lines); i++ {
			l := strings.TrimSpace(lines[i])
			if strings.HasPrefix(l, "```") || strings.HasPrefix(l, "~~~") {
				fence = !fence
			}
			n := len(l) - len(strings.TrimLeft(l, "#"))
			if fence || n == 0 || n > 6 || len(l) == n || l[n] != ' ' {
				continue
			}
			name := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(l[n:]), "#"))
			if found >= 0 && n <= level {
				at = i
				break
			}
			if found < 0 {
				names = append(names, name)
				if strings.EqualFold(name, want) {
					found, level = i, n
				}
			}
		}
		if found < 0 {
			if len(names) == 0 {
				return "", fmt.Errorf("the note has no heading %q, and no other: leave heading empty to add at its end", want)
			}
			return "", fmt.Errorf("the note has no heading %q; it has: %s", want, strings.Join(names, ", "))
		}
	}
	end := at // the next section's heading, or the end
	for at > 0 && strings.TrimSpace(lines[at-1]) == "" {
		at--
	}
	block := strings.Split(add, "\n")
	if at > 0 && !(isItem(lines[at-1]) && isItem(block[0])) {
		block = append([]string{""}, block...)
	}
	if end < len(lines) {
		block = append(block, "")
	}
	out := append(append(append([]string{}, lines[:at]...), block...), lines[end:]...)
	return strings.Join(out, "\n") + "\n", nil
}

// isItem says whether the line is a list item, which the next one follows without a blank line.
func isItem(l string) bool {
	t := strings.TrimLeft(l, " \t")
	return strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") || strings.HasPrefix(t, "+ ")
}

func noteWrite(sc scope, args json.RawMessage) (any, error) {
	var a struct {
		Path string `json:"path"`
		Text string `json:"text"`
		Sha  string `json:"sha"`
	}
	if err := in(args, &a); err != nil {
		return nil, err
	}
	rel, err := sc.note(a.Path)
	if err != nil {
		return nil, err
	}
	if err := sc.compiled(rel); err != nil {
		return nil, err
	}
	if strings.TrimSpace(a.Sha) == "" {
		return nil, errors.New("give the sha note_read gave for this note: read it first")
	}
	if strings.TrimSpace(a.Text) == "" {
		return nil, errors.New("text is empty: give the note's full new markdown, or use remove to delete it")
	}
	if len(a.Text) > maxNote {
		return nil, errors.New("the text is more than the 1 MB a note can be")
	}
	if !strings.HasSuffix(a.Text, "\n") {
		a.Text += "\n"
	}
	saved, err := ops.Save(sc.v, rel, []byte(a.Text), strings.TrimSpace(a.Sha), false)
	if err != nil {
		return nil, err
	}
	if !saved.Saved {
		return nil, fmt.Errorf("%s changed since you read it, so nothing is written: read it again with note_read and make the change in that text", rel)
	}
	return map[string]any{"path": rel, "sha": saved.Sha}, nil
}

func todoAdd(sc scope, args json.RawMessage) (any, error) {
	var a struct {
		Text   string `json:"text"`
		Due    string `json:"due"`
		Folder string `json:"folder"`
	}
	if err := in(args, &a); err != nil {
		return nil, err
	}
	if strings.TrimSpace(a.Text) == "" || strings.ContainsAny(strings.TrimSpace(a.Text), "\n\r") {
		return nil, errors.New("give the todo in text, on one line")
	}
	folder, err := sc.rel(a.Folder)
	if err != nil {
		return nil, err
	}
	it, err := ops.AddTodo(sc.v, folder, strings.TrimSpace(a.Text), strings.TrimSpace(a.Due))
	if err != nil {
		return nil, err
	}
	return todoOf(sc, it), nil
}

func todoChange(sc scope, args json.RawMessage) (any, error) {
	var a struct {
		Todo   string `json:"todo"`
		Action string `json:"action"`
		When   string `json:"when"`
	}
	if err := in(args, &a); err != nil {
		return nil, err
	}
	kind := a.Action
	if kind == "remove" {
		kind = "rm"
	}
	want := ops.TodoWant(kind)
	if want == nil {
		return nil, fmt.Errorf("action is done, undo, start, stop, due or remove, not %q", a.Action)
	}
	if kind == "due" && strings.TrimSpace(a.When) == "" {
		return nil, errors.New("for due, say when: a date, or none to remove it")
	}
	var seen []todo.Item
	for _, it := range todo.Collect(sc.v) {
		if !sc.hidden(it.Path) {
			seen = append(seen, it)
		}
	}
	it, err := ops.MatchTodo(seen, a.Todo, want)
	if err != nil {
		return nil, errors.New(strings.NewReplacer("nt todo --all lists them", "todo_list with show all lists them", "nt todo prints it", "todo_list gives it").Replace(err.Error()))
	}
	it, err = ops.ChangeTodo(sc.v, kind, fmt.Sprintf("%s:%d", it.Path, it.Line), strings.TrimSpace(a.When))
	if err != nil {
		return nil, err
	}
	if kind == "rm" {
		return map[string]any{"removed": it.Text}, nil
	}
	return todoOf(sc, it), nil
}

// MARK: Folders

func folderNew(sc scope, args json.RawMessage) (any, error) {
	var a struct {
		Path string `json:"path"`
	}
	if err := in(args, &a); err != nil {
		return nil, err
	}
	rel, err := sc.rel(a.Path)
	if err != nil {
		return nil, err
	}
	made, err := move.Mkdir(sc.v, rel)
	if err != nil {
		return nil, err
	}
	return map[string]any{"folder": made + "/"}, nil
}

// source is the note or folder p names, which is there and holds nothing hidden.
func (sc scope) source(p string) (string, error) {
	rel, err := sc.rel(p)
	if err != nil {
		return "", err
	}
	if rel == "" {
		return "", errors.New("name a note or a folder, not the root of the notes")
	}
	st, err := os.Stat(sc.v.Abs(rel))
	if err != nil {
		return "", fmt.Errorf("%s: no such note or folder; list shows what is there", rel)
	}
	if st.IsDir() {
		if h := sc.holds(rel); h != "" {
			return "", fmt.Errorf("%s holds %s, which is kept from the Claude app on this machine (the setting mcp.hide)", rel, h)
		}
	}
	return rel, nil
}

// landing says why the server does not make the move p plans, or nil: it would put something
// into a hidden folder, or among what nt compiles.
func (sc scope) landing(p *move.Plan) error {
	if sc.hidden(p.To) {
		return fmt.Errorf("%s is kept from the Claude app on this machine (the setting mcp.hide)", p.To)
	}
	return sc.compiled(p.To)
}

// planOf is a move's plan as the model reads it: what moves, and whose links change.
func planOf(p *move.Plan) map[string]any {
	const shown = 50
	files := []string{}
	for i, f := range p.Files {
		if i == shown {
			files = append(files, fmt.Sprintf("… and %d more", len(p.Files)-shown))
			break
		}
		files = append(files, f.From+" → "+f.To)
	}
	links := 0
	notes := []string{}
	for i, e := range p.Links {
		links += e.Links
		if i < shown {
			notes = append(notes, e.Path)
		}
	}
	out := map[string]any{"from": p.From, "to": p.To, "folder": p.Folder, "dry_run": p.DryRun, "moved": files, "links_fixed": links}
	if len(notes) > 0 {
		out["notes_with_links"] = notes
	}
	if p.Title != "" {
		out["title"] = p.Title
	}
	return out
}

func moveTool(sc scope, args json.RawMessage) (any, error) {
	var a struct {
		From string `json:"from"`
		To   string `json:"to"`
		Dry  bool   `json:"dry_run"`
	}
	if err := in(args, &a); err != nil {
		return nil, err
	}
	from, err := sc.source(a.From)
	if err != nil {
		return nil, err
	}
	to, err := sc.rel(a.To)
	if err != nil {
		return nil, err
	}
	if t := strings.TrimSpace(a.To); to == "" || strings.HasSuffix(t, "/") {
		to += "/" // into that folder, "/" the root
	}
	p, err := move.Move(sc.v, from, to, true)
	if err == nil {
		err = sc.landing(p)
	}
	if err == nil && !a.Dry {
		p, err = move.Move(sc.v, from, to, false)
	}
	if err != nil {
		return nil, err
	}
	return planOf(p), nil
}

func renameTool(sc scope, args json.RawMessage) (any, error) {
	var a struct {
		Path string `json:"path"`
		Name string `json:"name"`
		Dry  bool   `json:"dry_run"`
	}
	if err := in(args, &a); err != nil {
		return nil, err
	}
	rel, err := sc.source(a.Path)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(a.Name) == "" {
		return nil, errors.New("give the new title or name in name")
	}
	p, err := move.Rename(sc.v, rel, strings.TrimSpace(a.Name), true)
	if err == nil {
		err = sc.landing(p)
	}
	if err == nil && !a.Dry {
		p, err = move.Rename(sc.v, rel, strings.TrimSpace(a.Name), false)
	}
	if err != nil {
		return nil, err
	}
	return planOf(p), nil
}

func removeTool(sc scope, args json.RawMessage) (any, error) {
	var a struct {
		Path string `json:"path"`
		Dry  bool   `json:"dry_run"`
	}
	if err := in(args, &a); err != nil {
		return nil, err
	}
	rel, err := sc.source(a.Path)
	if err != nil {
		return nil, err
	}
	if err := sc.compiled(rel); err != nil {
		return nil, err
	}
	r, err := ops.Remove(sc.v, rel, a.Dry)
	if err != nil {
		return nil, err
	}
	return r, nil
}
