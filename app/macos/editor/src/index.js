// The editor page the Mac app shows a note in. The app calls window.nt with the note's text and
// path, and saves what the person writes: it takes a snapshot with beginSave, has nt write it, and
// says how that went (saved, failed, or conflict with the note as another writer left it). The
// page tells the app what it needs through post: an edit was made, links to check, a link to
// follow, a task to tick, an image to save, the word count. Nothing here touches the network or
// the disk.
import { Annotation, ChangeSet, EditorState, Transaction } from "@codemirror/state";
import { EditorView, drawSelection, keymap } from "@codemirror/view";
import { defaultKeymap, history, historyKeymap, indentWithTab, redo, undo, undoDepth, redoDepth } from "@codemirror/commands";
import { markdown, markdownLanguage } from "@codemirror/lang-markdown";
import { post } from "./bridge.js";
import { normalize, ownLine, rebase, toggleTask } from "./edits.js";
import { frontmatterRange, headingAt, linkDests, livePreview, noteField, setBacklinks, setBroken, setFocusMode } from "./livepreview.js";

// fromDisk marks a change another writer made, laid over the text: it is not the person's to save.
const fromDisk = Annotation.define();

// The note as nt last read or wrote it, the changes made since (base → the text), and a save
// under way: {seq, text, changes} with changes base → text.
let base = "";
let pending = ChangeSet.empty(0);
let inflight = null;
let seq = 0;
let focusMode = false;
let idle = 0;

function extensions(path, readOnly) {
  return [
    noteField.init(() => ({ path, broken: new Set(), focused: false, focusMode })),
    history(),
    keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
    markdown({ base: markdownLanguage }),
    EditorView.lineWrapping,
    EditorState.readOnly.of(readOnly),
    drawSelection(),
    EditorView.contentAttributes.of({ spellcheck: "false", autocorrect: "off", autocapitalize: "off" }),
    livePreview,
    EditorView.updateListener.of(changed),
    EditorView.domEventHandlers({ paste: pasted, dragover: dragging, drop: dropped }),
  ];
}

const view = new EditorView({
  parent: document.getElementById("editor"),
  state: EditorState.create({ doc: "", extensions: extensions("", true) }),
});

function changed(u) {
  if (!u.docChanged) return;
  let mine = false;
  for (const tr of u.transactions) {
    if (!tr.docChanged || tr.annotation(fromDisk)) continue;
    pending = pending.compose(tr.changes);
    mine = true;
  }
  if (mine) post({ type: "edit" });
  clearTimeout(idle);
  idle = setTimeout(report, 400);
}

// report tells the app the word count and the links to check, once the typing pauses.
function report() {
  post({ type: "check", dests: linkDests(view.state) });
  post({ type: "stats", words: words(view.state) });
}

function words(state) {
  const fm = frontmatterRange(state.doc);
  const text = state.doc.sliceString(fm ? fm.to : 0);
  return text.split(/\s+/).filter((w) => /[\p{L}\p{N}]/u.test(w)).length;
}

// goLine puts the cursor at the start of line n, from a search, so typing starts at what was
// found, and scrolls it to the middle.
function goLine(n) {
  if (n < 1 || n > view.state.doc.lines) return;
  const at = view.state.doc.line(n).from;
  view.dispatch({ selection: { anchor: at }, effects: EditorView.scrollIntoView(at, { y: "center" }) });
}

function scrollToAnchor(anchor) {
  const at = headingAt(view.state, anchor);
  if (at >= 0) view.dispatch({ effects: EditorView.scrollIntoView(at, { y: "start", yMargin: 24 }) });
}

// Images pasted or dropped go to the app, which saves them with nt paste and answers with the
// line that shows them (window.nt.insertImage).
function images(list) {
  return [...(list ?? [])].filter((f) => f.type.startsWith("image/"));
}

function sendImages(files, pos) {
  for (const f of files) {
    const r = new FileReader();
    r.onload = () => post({ type: "image", name: f.name || "image", mime: f.type, data: String(r.result).replace(/^data:[^,]*,/, ""), pos });
    r.readAsDataURL(f);
  }
}

function pasted(e, v) {
  const files = images(e.clipboardData?.files);
  if (!files.length || v.state.readOnly) return false;
  e.preventDefault();
  sendImages(files, v.state.selection.main.head);
  return true;
}

// dragging takes a drag that carries files, so that it is dropped here and not opened.
function dragging(e, v) {
  if (v.state.readOnly || !e.dataTransfer?.types?.includes("Files")) return false;
  e.preventDefault();
  e.dataTransfer.dropEffect = "copy";
  return true;
}

function dropped(e, v) {
  const files = images(e.dataTransfer?.files);
  if (!files.length || v.state.readOnly) return false;
  e.preventDefault();
  sendImages(files, v.posAtCoords({ x: e.clientX, y: e.clientY }, false));
  return true;
}

window.nt = {
  // load shows a note: {doc, path, readOnly, backlinks: [{path, title, folder}], anchor, line,
  // end (put the cursor at the end and take the keyboard)}.
  load({ doc, path, readOnly = false, backlinks = [], anchor = "", line = 0, end = false }) {
    view.setState(EditorState.create({ doc: normalize(doc), extensions: extensions(path, readOnly) }));
    base = view.state.doc.toString();
    pending = ChangeSet.empty(base.length);
    inflight = null;
    view.dispatch({ effects: setBacklinks.of(backlinks) });
    view.scrollDOM.scrollTop = 0;
    if (end) {
      view.dispatch({ selection: { anchor: view.state.doc.length }, scrollIntoView: true });
      view.focus();
    } else if (anchor) requestAnimationFrame(() => scrollToAnchor(anchor));
    else if (line > 0) requestAnimationFrame(() => goLine(line));
    report();
  },
  line: goLine,
  backlinks(list) {
    view.dispatch({ effects: setBacklinks.of(list) });
  },
  // broken names the link destinations that point at nothing.
  broken(dests) {
    view.dispatch({ effects: setBroken.of(dests) });
  },
  anchor: scrollToAnchor,

  // beginSave is the text to save, {seq, text}, or null when there is nothing to save. Changes
  // made while it is saved are kept apart until the app says how the save went.
  beginSave() {
    if (inflight) return null;
    const text = view.state.doc.toString();
    if (text === base) {
      pending = ChangeSet.empty(text.length);
      return null;
    }
    inflight = { seq: ++seq, text, changes: pending };
    pending = ChangeSet.empty(text.length);
    return { seq: inflight.seq, text };
  },
  // saved: nt wrote the snapshot. It returns whether there is more to save.
  saved(n) {
    if (!inflight || inflight.seq !== n) return !pending.empty;
    base = inflight.text;
    inflight = null;
    return !pending.empty;
  },
  // failed: nothing was written; the snapshot's changes are unsaved again.
  failed(n) {
    if (!inflight || inflight.seq !== n) return;
    pending = inflight.changes.compose(pending);
    inflight = null;
  },
  // conflict: another writer changed the note since it was read, and nt wrote nothing. Their
  // version is laid under the person's changes ("merged", saved next), or, when both changed the
  // same line, the person's version is saved over it and theirs kept as a conflict copy ("keep").
  conflict(n, theirs) {
    if (inflight && inflight.seq === n) window.nt.failed(n);
    return lay(normalize(theirs)) ? "merged" : "keep";
  },
  // external: the file changed on disk while nothing was being saved. It returns whether the
  // editor took the change; when the person changed the same lines, the next save sorts it out.
  external(theirs) {
    if (inflight) return false;
    return lay(normalize(theirs));
  },

  // toggle checks or opens again the todo on line n with the day nt counts (done:).
  toggle(n, day) {
    const doc = view.state.doc;
    if (view.state.readOnly || n < 1 || n > doc.lines) return;
    const l = doc.line(n);
    const text = toggleTask(l.text, day);
    if (text !== null) view.dispatch({ changes: { from: l.from, to: l.to, insert: text }, userEvent: "input.toggle" });
  },
  // insertImage puts the markdown line nt paste printed on a line of its own at pos.
  insertImage(pos, line) {
    if (view.state.readOnly) return;
    view.dispatch({ ...ownLine(view.state.doc, pos, line), scrollIntoView: true, userEvent: "input.paste" });
    view.focus();
  },
  focusMode(on) {
    focusMode = on;
    view.dispatch({ effects: setFocusMode.of(on) });
  },
  undo: () => undo(view),
  redo: () => redo(view),
  canUndo: () => undoDepth(view.state) > 0,
  canRedo: () => redoDepth(view.state) > 0,
  focus: () => view.focus(),
};

// lay puts theirs under the changes not yet saved; false when both changed the same line.
function lay(theirs) {
  if (theirs === base) return true;
  const r = rebase(base, theirs, pending);
  if (!r) return false;
  view.dispatch({ changes: r.apply, annotations: [fromDisk.of(true), Transaction.addToHistory.of(false)] });
  base = theirs;
  pending = r.pending;
  return true;
}

post({ type: "ready" });
