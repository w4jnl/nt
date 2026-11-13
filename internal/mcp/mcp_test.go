package mcp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// repo is a notes repository of the test's own, which the server finds by NT_REPO.
func repo(t *testing.T, config string) string {
	t.Helper()
	root := t.TempDir()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(dir, "config.toml"), config)
	t.Setenv("NT_REPO", root)
	t.Setenv("NT_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("NT_STATE", filepath.Join(dir, "state"))
	return root
}

func put(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// one finds the one file that matches pattern under root.
func one(t *testing.T, root, pattern string) string {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(root, pattern))
	if len(m) != 1 {
		t.Fatalf("%s: %d files, want 1: %v", pattern, len(m), m)
	}
	return m[0]
}

// ask sends the lines to a server and returns its answers, each of which must be a JSON-RPC
// message: nothing else may be on stdout.
func ask(t *testing.T, s *Server, lines ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := s.Serve(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	var msgs []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if l == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil || m["jsonrpc"] != "2.0" {
			t.Fatalf("not a protocol message on stdout: %q", l)
		}
		msgs = append(msgs, m)
	}
	return msgs
}

// call runs one tool and returns its answer's text and whether it is an error.
func call(t *testing.T, s *Server, tool string, args any) (string, bool) {
	t.Helper()
	a, _ := json.Marshal(args)
	req, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": json.RawMessage(a)}})
	msgs := ask(t, s, string(req))
	if len(msgs) != 1 || msgs[0]["result"] == nil {
		t.Fatalf("%s: no result: %v", tool, msgs)
	}
	res := msgs[0]["result"].(map[string]any)
	var texts []string
	for _, c := range res["content"].([]any) {
		texts = append(texts, c.(map[string]any)["text"].(string))
	}
	failed, _ := res["isError"].(bool)
	return strings.Join(texts, "\n"), failed
}

// ok runs a tool that must work and returns its first answer as JSON.
func ok(t *testing.T, s *Server, tool string, args any) map[string]any {
	t.Helper()
	text, failed := call(t, s, tool, args)
	if failed {
		t.Fatalf("%s: %s", tool, text)
	}
	var m map[string]any
	if err := json.NewDecoder(strings.NewReader(text)).Decode(&m); err != nil {
		t.Fatalf("%s: %v in %q", tool, err, text)
	}
	return m
}

// refused runs a tool that must refuse, with want in what it says.
func refused(t *testing.T, s *Server, tool string, args any, want string) {
	t.Helper()
	text, failed := call(t, s, tool, args)
	if !failed || !strings.Contains(text, want) {
		t.Errorf("%s %v = %q (error %v), want an error with %q", tool, args, text, failed, want)
	}
}

type M = map[string]any

func TestProtocol(t *testing.T) {
	repo(t, "")
	msgs := ask(t, &Server{Version: "1.2.3"},
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":4,"method":"resources/list"}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"nope"}}`,
		`not json`,
	)
	if len(msgs) != 6 {
		t.Fatalf("%d answers, want 6 (the notification gets none): %v", len(msgs), msgs)
	}
	init := msgs[0]["result"].(map[string]any)
	if init["protocolVersion"] != "2025-06-18" || init["serverInfo"].(map[string]any)["version"] != "1.2.3" {
		t.Errorf("initialize = %v", init)
	}
	tools := msgs[2]["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 15 {
		t.Errorf("%d tools, want 15", len(tools))
	}
	for _, x := range tools {
		tl := x.(map[string]any)
		schema, _ := tl["inputSchema"].(map[string]any)
		if tl["description"] == "" || schema["type"] != "object" {
			t.Errorf("tool %v lacks a description or a schema", tl["name"])
		}
		for name, p := range schema["properties"].(map[string]any) {
			if p.(map[string]any)["description"] == "" {
				t.Errorf("%v.%s has no description", tl["name"], name)
			}
		}
	}
	for i, want := range map[int]float64{3: -32601, 4: -32602, 5: -32700} {
		if e, _ := msgs[i]["error"].(map[string]any); e == nil || e["code"] != want {
			t.Errorf("answer %d = %v, want error %v", i, msgs[i], want)
		}
	}
	// a version the server does not know gets its newest
	old := ask(t, &Server{}, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`)
	if v := old[0]["result"].(map[string]any)["protocolVersion"]; v != versions[len(versions)-1] {
		t.Errorf("version = %v", v)
	}
}

func TestReadOnlyHasOnlyTheToolsThatRead(t *testing.T) {
	root := repo(t, "")
	s := &Server{ReadOnly: true}
	var names []string
	for _, tl := range s.tools() {
		names = append(names, tl.name)
	}
	if got := strings.Join(names, " "); got != "list search note_read todo_list" {
		t.Errorf("tools = %s", got)
	}
	msgs := ask(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"journal_add","arguments":{"text":"x"}}}`)
	if msgs[0]["error"] == nil {
		t.Errorf("journal_add ran on a read-only server: %v", msgs[0])
	}
	if e, _ := os.ReadDir(root); len(e) != 1 {
		t.Errorf("something was written: %v", e)
	}
}

const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect width="10" height="10"/></svg>`

func TestNewNoteWithImages(t *testing.T) {
	root := repo(t, "")
	var kicks atomic.Int32
	s := &Server{Delay: time.Millisecond, Kick: func() { kicks.Add(1) }}
	res := ok(t, s, "note_new", M{
		"title":  "Cumulative delta vs price",
		"folder": "trading",
		"tags":   []string{"Trading", "#order flow"},
		"body":   "# Cumulative delta vs price\n\nIntro.\n\n![The four](four.svg)\n\nMore.\n",
		"images": []M{
			{"file": "four.svg", "data": svg},
			{"file": "Offer Absorption.svg", "data": base64.StdEncoding.EncodeToString([]byte(svg)), "base64": true},
		},
	})
	rel := res["path"].(string)
	if !strings.HasPrefix(rel, "trading/") || !strings.HasSuffix(rel, "-cumulative-delta-vs-price.md") {
		t.Fatalf("path = %s", rel)
	}
	note := get(t, filepath.Join(root, rel))
	day := strings.TrimPrefix(rel, "trading/")[:10]
	for _, want := range []string{
		"title: Cumulative delta vs price\n", "tags: [trading, order-flow]\n", "\n# Cumulative delta vs price\n\nIntro.\n",
		"![The four](assets/" + day + "-four.svg)", "![Offer Absorption](assets/" + day + "-offer-absorption.svg)\n",
	} {
		if !strings.Contains(note, want) {
			t.Errorf("the note lacks %q:\n%s", want, note)
		}
	}
	if strings.Count(note, "# Cumulative delta vs price") != 1 {
		t.Errorf("the title is there twice:\n%s", note)
	}
	for _, f := range []string{"four", "offer-absorption"} {
		if got := get(t, filepath.Join(root, "trading/assets", day+"-"+f+".svg")); got != svg {
			t.Errorf("%s.svg = %q", f, got)
		}
	}
	if j := get(t, one(t, root, "daily/*/*.md")); !strings.Contains(j, "Cumulative delta vs price") {
		t.Errorf("today's journal does not link the note:\n%s", j)
	}
	if kicks.Load() != 1 {
		t.Errorf("%d syncs asked for, want 1", kicks.Load())
	}

	// what nt does not keep is refused before anything is written
	before, _ := filepath.Glob(filepath.Join(root, "trading/*"))
	refused(t, s, "note_new", M{"title": "Run", "folder": "trading", "images": []M{{"file": "run.sh", "data": "#!/bin/sh"}}}, "not an image nt keeps")
	refused(t, s, "note_new", M{"title": "Run", "folder": "trading", "images": []M{{"file": "a.png", "data": "notbase64"}}}, "base64")
	refused(t, s, "note_new", M{"title": "Big", "folder": "trading", "images": []M{{"file": "big.svg", "data": strings.Repeat("x", maxImage+1)}}}, "5 MB")
	refused(t, s, "note_new", M{"title": "Report", "folder": "agents/2026"}, "nt's own folder")
	refused(t, s, "note_new", M{"title": "x", "overwrite": true}, "not what this tool takes")
	if after, _ := filepath.Glob(filepath.Join(root, "trading/*")); len(after) != len(before) {
		t.Errorf("a refused note left files: %v", after)
	}
}

func TestReadWriteAppend(t *testing.T) {
	root := repo(t, "")
	s := &Server{}
	put(t, filepath.Join(root, "work/2026-11-01-plan.md"), "---\ntitle: Plan\n# not a heading\n---\n\n# Plan\n\n## Decisions\n\n- one\n\n## Open\n\nNothing.\n")
	text, failed := call(t, s, "note_read", M{"path": "work/2026-11-01-plan.md"})
	if failed || !strings.Contains(text, "## Decisions\n\n- one\n") {
		t.Fatalf("note_read = %q", text)
	}
	read := ok(t, s, "note_read", M{"path": "work/2026-11-01-plan.md"})
	if read["title"] != "Plan" || len(read["sha"].(string)) != 64 {
		t.Errorf("note_read = %v", read)
	}

	ok(t, s, "note_append", M{"path": "work/2026-11-01-plan.md", "heading": "decisions", "text": "- two"})
	ok(t, s, "note_append", M{"path": "work/2026-11-01-plan.md", "text": "Later.", "images": []M{{"file": "c.svg", "data": svg}}})
	note := get(t, filepath.Join(root, "work/2026-11-01-plan.md"))
	if !strings.Contains(note, "## Decisions\n\n- one\n- two\n\n## Open\n\nNothing.\n\nLater.\n\n![c](assets/") {
		t.Errorf("after the appends:\n%s", note)
	}
	refused(t, s, "note_append", M{"path": "work/2026-11-01-plan.md", "heading": "Risks", "text": "x"}, "it has: Plan, Decisions, Open")

	// a write needs the version that was read
	refused(t, s, "note_write", M{"path": "work/2026-11-01-plan.md", "text": "gone", "sha": read["sha"]}, "changed since you read it")
	refused(t, s, "note_write", M{"path": "work/2026-11-01-plan.md", "text": "gone", "sha": ""}, "read it first")
	if got := get(t, filepath.Join(root, "work/2026-11-01-plan.md")); got != note {
		t.Errorf("a refused write changed the note:\n%s", got)
	}
	now := ok(t, s, "note_read", M{"path": "work/2026-11-01-plan.md"})
	ok(t, s, "note_write", M{"path": "work/2026-11-01-plan.md", "text": "# Plan\n\nNew.", "sha": now["sha"]})
	if got := get(t, filepath.Join(root, "work/2026-11-01-plan.md")); got != "# Plan\n\nNew.\n" {
		t.Errorf("after the write: %q", got)
	}
	refused(t, s, "note_write", M{"path": "work/none.md", "text": "x", "sha": "x"}, "no such note")

	// what nt compiles is read, not written
	put(t, filepath.Join(root, "agents/2026/2026-11-01/README.md"), "# Digest\n")
	if _, failed := call(t, s, "note_read", M{"path": "agents/2026/2026-11-01/README.md"}); failed {
		t.Error("a digest is not read")
	}
	refused(t, s, "note_append", M{"path": "agents/2026/2026-11-01/README.md", "text": "x"}, "nt compiles")
}

func TestAppendTo(t *testing.T) {
	for _, tc := range []struct{ doc, add, heading, want string }{
		{"A\n", "B", "", "A\n\nB\n"},
		{"- a\n\n", "- b\n", "", "- a\n- b\n"},
		{"", "B", "", "B\n"},
		{"# T\n\n## A\n\ntext\n\n### Sub\n\ns\n\n## B\n\nb\n", "new", "A", "# T\n\n## A\n\ntext\n\n### Sub\n\ns\n\nnew\n\n## B\n\nb\n"},
		{"## A\n\n```\n## B\n```\n", "x", "## A", "## A\n\n```\n## B\n```\n\nx\n"},
		{"## Log\n", "- 10:00 x", "Log", "## Log\n\n- 10:00 x\n"},
	} {
		got, err := appendTo(tc.doc, tc.add, tc.heading)
		if err != nil || got != tc.want {
			t.Errorf("appendTo(%q, %q, %q) = %q, %v; want %q", tc.doc, tc.add, tc.heading, got, err, tc.want)
		}
	}
}

func TestPathsStayInTheNotes(t *testing.T) {
	root := repo(t, "")
	s := &Server{}
	outside := t.TempDir()
	put(t, filepath.Join(outside, "secret.md"), "secret\n")
	put(t, filepath.Join(root, "work/a.md"), "# A\n")
	if err := os.Symlink(outside, filepath.Join(root, "out")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"../x.md", "work/../../x.md", "/etc/hosts.md", filepath.Join(outside, "secret.md"), "~/x.md", ".git/config.md", "work/.hidden/x.md", "out/secret.md"} {
		for tool, args := range map[string]M{
			"note_read":   {"path": p},
			"note_write":  {"path": p, "text": "x", "sha": "x"},
			"note_append": {"path": p, "text": "x"},
			"remove":      {"path": p},
			"move":        {"from": "work/a.md", "to": p},
			"folder_new":  {"path": strings.TrimSuffix(p, ".md")},
		} {
			if text, failed := call(t, s, tool, args); !failed {
				t.Errorf("%s took %q: %s", tool, p, text)
			}
		}
	}
	if got := get(t, filepath.Join(outside, "secret.md")); got != "secret\n" {
		t.Errorf("the file outside changed: %q", got)
	}
	if e, _ := os.ReadDir(outside); len(e) != 1 {
		t.Errorf("something was written outside: %v", e)
	}
	if get(t, filepath.Join(root, "work/a.md")) != "# A\n" {
		t.Error("work/a.md moved or changed")
	}
}

func TestHiddenFoldersAreKeptFromTheServer(t *testing.T) {
	root := repo(t, "[mcp]\nhide = [\"private/health\", \"/diary/\"]\n")
	s := &Server{}
	put(t, filepath.Join(root, "private/health/2026-11-01-scan.md"), "# Scan\n\nzebra result\n\n- [ ] Call the clinic\n")
	put(t, filepath.Join(root, "private/2026-11-02-car.md"), "# Car\n\nzebra crossing, see [scan](health/2026-11-01-scan.md)\n")
	put(t, filepath.Join(root, "private/health/todo.md"), "---\ntodos: true\n---\n\n## Inbox\n\n- [ ] Book the dentist\n")
	put(t, filepath.Join(root, "diary/2026-11-03-day.md"), "# Day\n")

	all := ok(t, s, "list", M{"recursive": true})
	if got, _ := json.Marshal(all); strings.Contains(string(got), "health") || strings.Contains(string(got), "diary") || !strings.Contains(string(got), "private/2026-11-02-car.md") {
		t.Errorf("list = %s", got)
	}
	found := ok(t, s, "search", M{"query": "zebra"})
	if m := found["matches"].([]any); len(m) != 1 || m[0].(map[string]any)["path"] != "private/2026-11-02-car.md" {
		t.Errorf("search = %v", found)
	}
	if todos := ok(t, s, "todo_list", M{"show": "all"})["todos"].([]any); len(todos) != 0 {
		t.Errorf("todo_list = %v", todos)
	}
	const kept = "kept from the Claude app"
	refused(t, s, "note_read", M{"path": "private/health/2026-11-01-scan.md"}, kept)
	refused(t, s, "list", M{"folder": "private/health"}, kept)
	refused(t, s, "note_append", M{"path": "private/health/2026-11-01-scan.md", "text": "x"}, kept)
	refused(t, s, "note_new", M{"title": "X", "folder": "private/health"}, kept)
	refused(t, s, "todo_add", M{"text": "x", "folder": "private/health"}, kept)
	refused(t, s, "todo_change", M{"todo": "dentist", "action": "done"}, "no todo matches")
	refused(t, s, "todo_change", M{"todo": "private/health/todo.md:7", "action": "done"}, "no todo at")
	refused(t, s, "move", M{"from": "private/2026-11-02-car.md", "to": "private/health/"}, kept)
	refused(t, s, "move", M{"from": "private/2026-11-02-car.md", "to": "diary/"}, kept)
	refused(t, s, "move", M{"from": "private", "to": "personal"}, "holds private/health")
	refused(t, s, "rename", M{"path": "private", "name": "personal"}, "holds private/health")
	refused(t, s, "remove", M{"path": "private/health/2026-11-01-scan.md"}, kept)
	if !strings.Contains(get(t, filepath.Join(root, "private/health/todo.md")), "- [ ] Book the dentist") {
		t.Error("a hidden todo changed")
	}
	if _, err := os.Stat(filepath.Join(root, "private/2026-11-02-car.md")); err != nil {
		t.Error("the note moved")
	}

	// the setting holds from the next call
	put(t, os.Getenv("NT_CONFIG"), "")
	if _, failed := call(t, s, "note_read", M{"path": "private/health/2026-11-01-scan.md"}); failed {
		t.Error("still hidden after the setting was cleared")
	}
}

func TestReorganize(t *testing.T) {
	root := repo(t, "")
	s := &Server{}
	put(t, filepath.Join(root, "inbox/2026-11-01-scan.md"), "# Scan\n\n![x](assets/2026-11-01-x.svg)\n")
	put(t, filepath.Join(root, "inbox/assets/2026-11-01-x.svg"), svg)
	put(t, filepath.Join(root, "inbox/2026-11-02-other.md"), "# Other\n\nSee [the scan](2026-11-01-scan.md).\n")

	if got := ok(t, s, "folder_new", M{"path": "private/Health 2026"}); got["folder"] != "private/health-2026/" {
		t.Errorf("folder_new = %v", got)
	}
	plan := ok(t, s, "move", M{"from": "inbox/2026-11-01-scan.md", "to": "private/health-2026/", "dry_run": true})
	if plan["dry_run"] != true || plan["to"] != "private/health-2026/2026-11-01-scan.md" || plan["links_fixed"].(float64) < 1 {
		t.Errorf("the plan = %v", plan)
	}
	if _, err := os.Stat(filepath.Join(root, "inbox/2026-11-01-scan.md")); err != nil {
		t.Fatal("a dry run moved the note")
	}
	ok(t, s, "move", M{"from": "inbox/2026-11-01-scan.md", "to": "private/health-2026/"})
	if got := get(t, filepath.Join(root, "inbox/2026-11-02-other.md")); !strings.Contains(got, "(../private/health-2026/2026-11-01-scan.md)") {
		t.Errorf("the link to the moved note: %s", got)
	}
	if _, err := os.Stat(filepath.Join(root, "private/health-2026/assets/2026-11-01-x.svg")); err != nil {
		t.Error("the image did not go along")
	}

	renamed := ok(t, s, "rename", M{"path": "private/health-2026/2026-11-01-scan.md", "name": "MRI scan"})
	if renamed["to"] != "private/health-2026/2026-11-01-mri-scan.md" {
		t.Errorf("rename = %v", renamed)
	}
	ok(t, s, "rename", M{"path": "private/health-2026", "name": "health"})
	if got := get(t, filepath.Join(root, "inbox/2026-11-02-other.md")); !strings.Contains(got, "(../private/health/2026-11-01-mri-scan.md)") {
		t.Errorf("the link after the renames: %s", got)
	}
	refused(t, s, "move", M{"from": "inbox/2026-11-02-other.md", "to": "agents/"}, "agents")
	refused(t, s, "move", M{"from": "inbox/none.md", "to": "private/"}, "no such note or folder")

	gone := ok(t, s, "remove", M{"path": "private/health/2026-11-01-mri-scan.md", "dry_run": true})
	if gone["dry_run"] != true || gone["links"].(float64) != 1 {
		t.Errorf("remove's plan = %v", gone)
	}
	ok(t, s, "remove", M{"path": "private/health/2026-11-01-mri-scan.md"})
	if _, err := os.Stat(filepath.Join(root, "private/health/2026-11-01-mri-scan.md")); err == nil {
		t.Error("the note is still there")
	}
	refused(t, s, "remove", M{"path": "inbox"}, "")
}

func TestJournalAndTodos(t *testing.T) {
	root := repo(t, "")
	s := &Server{}
	line := ok(t, s, "journal_add", M{"text": "Met the plumber"})
	if !strings.Contains(get(t, filepath.Join(root, line["path"].(string))), "Met the plumber") {
		t.Errorf("journal_add = %v", line)
	}
	added := ok(t, s, "todo_add", M{"text": "Call the plumber", "due": "2030-01-02"})
	if added["todo"] == "" || added["due"] != "2030-01-02" {
		t.Errorf("todo_add = %v", added)
	}
	ok(t, s, "todo_add", M{"text": "Send the invoice", "folder": "work"})
	if got := ok(t, s, "todo_change", M{"todo": "plumber", "action": "start"}); got["in_progress"] != true {
		t.Errorf("start = %v", got)
	}
	list := ok(t, s, "todo_list", M{})["todos"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["text"] != "Call the plumber" {
		t.Errorf("todo_list = %v", list)
	}
	if got := ok(t, s, "todo_change", M{"todo": added["todo"], "action": "due", "when": "none"}); got["due"] != nil {
		t.Errorf("due none = %v", got)
	}
	ok(t, s, "todo_change", M{"todo": "invoice", "action": "done"})
	ok(t, s, "todo_change", M{"todo": "plumber", "action": "remove"})
	if todos := ok(t, s, "todo_list", M{"show": "all"})["todos"].([]any); len(todos) != 1 || todos[0].(map[string]any)["done"] != true {
		t.Errorf("at the end: %v", todos)
	}
	refused(t, s, "todo_change", M{"todo": "invoice", "action": "eat"}, "action is")
	refused(t, s, "todo_change", M{"todo": "nothing", "action": "done"}, "todo_list with show all")
}

func TestInstall(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "claude_desktop_config.json")
	if err := Install(filepath.Join(dir, "none", "c.json"), "/opt/nt", false); err == nil {
		t.Error("installed without the app's folder")
	}
	put(t, p, `{"theme":"dark","mcpServers":{"other":{"command":"/bin/x","args":["a"],"extra":1}}}`)
	if err := Install(p, "/opt/nt/bin/nt", true); err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Theme   string                     `json:"theme"`
		Servers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(get(t, p)), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Theme != "dark" || !strings.Contains(string(cfg.Servers["other"]), `"extra"`) {
		t.Errorf("the rest of the file changed: %s", get(t, p))
	}
	e, has, err := Installed(p)
	if err != nil || !has || e.Command != "/opt/nt/bin/nt" || strings.Join(e.Args, " ") != "mcp --read-only" || !e.ReadOnly() || !strings.HasPrefix(e.Env["PATH"], "/opt/nt/bin:") {
		t.Errorf("Installed = %+v, %v, %v", e, has, err)
	}
	if err := Remove(p); err != nil {
		t.Fatal(err)
	}
	if _, has, _ := Installed(p); has || !strings.Contains(get(t, p), `"other"`) {
		t.Errorf("after Remove: %s", get(t, p))
	}
	put(t, p, `{broken`)
	if err := Install(p, "/opt/nt", false); err == nil || get(t, p) != `{broken` {
		t.Errorf("a file that is not JSON was written over: %v", err)
	}
}
