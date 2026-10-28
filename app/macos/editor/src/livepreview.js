// nt's live preview: a note reads as rendered text, and the markdown of a line shows only while
// the cursor is on it. Headings' # marks then hang in the left margin, so the text never moves.
// Everything here is decoration: the document is the note's markdown, byte for byte.
import { Decoration, EditorView, ViewPlugin, WidgetType } from "@codemirror/view";
import { StateEffect, StateField } from "@codemirror/state";
import { ensureSyntaxTree, syntaxTree } from "@codemirror/language";
import { post } from "./bridge.js";

// setBroken names the link destinations, as written, that point at nothing (the app checks).
export const setBroken = StateEffect.define();
// setFocus says whether the editor has the keyboard: without it no line shows its markdown.
export const setFocus = StateEffect.define();
// setBacklinks gives the notes that link to this one: [{path, title, folder}].
export const setBacklinks = StateEffect.define();

// noteField is what the preview knows besides the text: the note's repository path, its broken
// links and the focus.
export const noteField = StateField.define({
  create: () => ({ path: "", broken: new Set(), focused: false }),
  update(v, tr) {
    for (const e of tr.effects) {
      if (e.is(setBroken)) v = { ...v, broken: new Set(e.value) };
      else if (e.is(setFocus)) v = { ...v, focused: e.value };
    }
    return v;
  },
});

// activeLines are the line numbers the selection touches while the editor has focus.
export function activeLines(state) {
  const lines = new Set();
  if (!state.field(noteField).focused) return lines;
  for (const r of state.selection.ranges) {
    const a = state.doc.lineAt(r.from).number;
    const b = state.doc.lineAt(r.to).number;
    for (let n = a; n <= b; n++) lines.add(n);
  }
  return lines;
}

function touches(state, active, from, to) {
  const a = state.doc.lineAt(from).number;
  const b = state.doc.lineAt(to).number;
  for (let n = a; n <= b; n++) if (active.has(n)) return true;
  return false;
}

// external says whether a link destination leaves the repository: a URL or an absolute path.
export function external(dest) {
  return /^[a-z][a-z0-9+.-]*:/i.test(dest) || dest.startsWith("/");
}

// resolve is the repository path a relative link in the note at notePath points to, without its
// #anchor, or "" for an external link or one that only names an anchor.
export function resolve(notePath, dest) {
  dest = cleanDest(dest);
  if (external(dest)) return "";
  let p = dest.split("#")[0];
  if (!p) return "";
  try {
    p = decodeURIComponent(p);
  } catch {
    // a stray % stays as written
  }
  const parts = notePath.split("/").slice(0, -1);
  for (const seg of p.split("/")) {
    if (seg === "" || seg === ".") continue;
    if (seg === "..") {
      if (!parts.length) return "";
      parts.pop();
    } else parts.push(seg);
  }
  return parts.join("/");
}

// cleanDest takes the angle brackets off <path with spaces.md>.
export function cleanDest(dest) {
  return dest.startsWith("<") && dest.endsWith(">") ? dest.slice(1, -1) : dest;
}

// assetURL is the URL the app serves a repository file at, for images.
export function assetURL(repoPath) {
  return "nt-asset://repo/" + repoPath.split("/").map(encodeURIComponent).join("/");
}

// frontmatterRange is the frontmatter's span, from the opening --- to the end of the closing one.
export function frontmatterRange(doc) {
  if (doc.lines < 2 || doc.line(1).text !== "---") return null;
  for (let n = 2; n <= doc.lines; n++) {
    const t = doc.line(n).text;
    if (t === "---" || t === "...") return { from: 0, to: doc.line(n).to, lines: n };
  }
  return null;
}

const MONTHS = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"];
const DAYS = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

// frontmatterSummary is the one line a folded frontmatter shows: the long date and the tags. A
// journal shows nothing: its heading is the date.
export function frontmatterSummary(text) {
  const keys = {};
  let list = null;
  for (const line of text.split("\n").slice(1, -1)) {
    const item = /^\s+-\s+(.*)$/.exec(line);
    if (item && list) {
      keys[list].push(unquote(item[1]));
      continue;
    }
    const kv = /^([A-Za-z0-9_-]+):\s*(.*)$/.exec(line);
    if (!kv) continue;
    list = null;
    let v = kv[2].trim();
    if (v === "") {
      keys[kv[1]] = [];
      list = kv[1];
    } else if (v.startsWith("[") && v.endsWith("]")) {
      keys[kv[1]] = v.slice(1, -1).split(",").map((s) => unquote(s.trim())).filter(Boolean);
    } else keys[kv[1]] = unquote(v);
  }
  if (keys.type === "daily") return "";
  const parts = [];
  const d = /^(\d{4})-(\d{2})-(\d{2})$/.exec(keys.date ?? "");
  if (d) {
    const day = new Date(Date.UTC(+d[1], +d[2] - 1, +d[3]));
    parts.push(`${DAYS[day.getUTCDay()]} ${+d[3]} ${MONTHS[+d[2] - 1]} ${d[1]}`);
  }
  const tags = Array.isArray(keys.tags) ? keys.tags : keys.tags ? [keys.tags] : [];
  if (tags.length) parts.push(tags.join(", "));
  if (!parts.length) return "properties";
  return parts.join(" · ");
}

function unquote(s) {
  return s.replace(/^(["'])(.*)\1$/, "$2");
}

class ImageWidget extends WidgetType {
  constructor(src, alt) {
    super();
    this.src = src;
    this.alt = alt;
  }
  eq(o) {
    return o.src === this.src && o.alt === this.alt;
  }
  toDOM() {
    const wrap = document.createElement("span");
    wrap.className = "cm-image";
    const img = document.createElement("img");
    img.src = this.src;
    img.alt = this.alt;
    img.addEventListener("error", () => wrap.classList.add("cm-image-missing"));
    wrap.append(img);
    if (this.alt) {
      const cap = document.createElement("span");
      cap.className = "cm-caption";
      cap.textContent = this.alt;
      wrap.append(cap);
    }
    return wrap;
  }
  get estimatedHeight() {
    return 240;
  }
}

class TextWidget extends WidgetType {
  constructor(text, cls) {
    super();
    this.text = text;
    this.cls = cls;
  }
  eq(o) {
    return o.text === this.text && o.cls === this.cls;
  }
  toDOM() {
    const s = document.createElement("span");
    s.className = this.cls;
    s.textContent = this.text;
    return s;
  }
}

class CheckWidget extends WidgetType {
  constructor(checked) {
    super();
    this.checked = checked;
  }
  eq(o) {
    return o.checked === this.checked;
  }
  toDOM() {
    const s = document.createElement("span");
    s.className = "cm-check" + (this.checked ? " cm-check-done" : "");
    s.textContent = this.checked ? "✓" : "○";
    s.setAttribute("role", "checkbox");
    s.setAttribute("aria-checked", String(this.checked));
    return s;
  }
}

class FrontmatterWidget extends WidgetType {
  constructor(summary) {
    super();
    this.summary = summary;
  }
  eq(o) {
    return o.summary === this.summary;
  }
  toDOM() {
    const d = document.createElement("div");
    d.className = this.summary ? "cm-fm-summary" : "cm-fm-none";
    d.textContent = this.summary;
    d.title = "Click to show the frontmatter";
    return d;
  }
  ignoreEvent() {
    return false;
  }
}

class BacklinksWidget extends WidgetType {
  constructor(items) {
    super();
    this.items = items;
  }
  eq(o) {
    return JSON.stringify(o.items) === JSON.stringify(this.items);
  }
  toDOM() {
    const d = document.createElement("div");
    d.className = "cm-backlinks";
    const h = document.createElement("div");
    h.className = "cm-backlinks-head";
    h.textContent = "Linked from";
    d.append(h);
    for (const it of this.items) {
      const a = document.createElement("a");
      a.className = "cm-backlink";
      a.dataset.open = it.path;
      a.href = "#";
      const t = document.createElement("span");
      t.textContent = it.title || it.path;
      const f = document.createElement("span");
      f.className = "cm-backlink-folder";
      f.textContent = it.folder;
      a.append(t, f);
      d.append(a);
    }
    return d;
  }
  ignoreEvent() {
    return true;
  }
}

const hide = Decoration.replace({});
const mark = (cls, attributes) => Decoration.mark(attributes ? { class: cls, attributes } : { class: cls });
const line = (cls) => Decoration.line({ class: cls });

const NAV = /^\s*(\[[^\]]*\]\([^)]*\)\s*(·\s*)?)+$/;
const TIME = /^\d{1,2}:\d{2}(?=\s|$)/;
const META = /\b(?:due|done):\d{4}-\d{2}-\d{2}\b/g;

// buildDecorations is the preview of the whole note for its state: what is hidden, replaced or
// styled. It reads only the state, so the tests run it without a screen.
export function buildDecorations(state) {
  const { path, broken } = state.field(noteField);
  const active = activeLines(state);
  const doc = state.doc;
  const fm = frontmatterRange(doc);
  const out = [];
  const add = (deco, from, to = from) => out.push(deco.range(from, to));
  const isActive = (from, to = from) => touches(state, active, from, to);
  const lines = (from, to, cls) => {
    const a = doc.lineAt(from).number;
    const b = doc.lineAt(to).number;
    for (let n = a; n <= b; n++) {
      let c = cls;
      if (n === a) c += " " + cls + "-first";
      if (n === b) c += " " + cls + "-last";
      add(line(c), doc.line(n).from);
    }
  };
  const tree = ensureSyntaxTree(state, doc.length, 500) ?? syntaxTree(state);

  tree.iterate({
    enter(node) {
      const { name, from, to } = node;
      if (fm && name !== "Document" && from <= fm.to) return to <= fm.to ? false : undefined;
      const h = /^ATXHeading(\d)$/.exec(name) ?? /^SetextHeading(\d)$/.exec(name);
      if (h) {
        add(line("cm-h" + h[1]), doc.lineAt(from).from);
        return;
      }
      switch (name) {
        case "HeaderMark": {
          const parent = node.node.parent;
          if (parent?.name.startsWith("Setext")) {
            if (!isActive(from)) add(hide, from, to);
            return;
          }
          const space = doc.sliceString(to, to + 1) === " " ? 1 : 0;
          if (from === parent?.from) {
            if (isActive(from)) add(mark("cm-hang"), from, to + space);
            else add(hide, from, to + space);
          } else if (!isActive(from)) add(hide, from, to);
          return;
        }
        case "Emphasis":
          add(mark("cm-em"), from, to);
          return;
        case "StrongEmphasis":
          add(mark("cm-strong"), from, to);
          return;
        case "Strikethrough":
          add(mark("cm-strike"), from, to);
          return;
        case "InlineCode":
          add(mark("cm-icode"), from, to);
          return;
        case "EmphasisMark":
        case "StrikethroughMark":
          if (!isActive(from)) add(hide, from, to);
          return;
        case "CodeMark":
          if (node.node.parent?.name === "InlineCode" && !isActive(from)) add(hide, from, to);
          return;
        case "Link":
          return link(node.node);
        case "Image":
          image(node.node);
          return false;
        case "URL": {
          const parent = node.node.parent?.name;
          if (parent === "Link" || parent === "Image" || parent === "LinkReference") return;
          const href = doc.sliceString(from, to);
          add(mark("cm-link", { "data-href": href, title: href }), from, to);
          return;
        }
        case "Autolink": {
          if (isActive(from)) return;
          for (const m of node.node.getChildren("LinkMark")) add(hide, m.from, m.to);
          return;
        }
        case "ListMark":
          listMark(node.node);
          return;
        case "Task": {
          const text = doc.sliceString(from, to);
          for (const m of text.matchAll(META)) add(mark("cm-meta"), from + m.index, from + m.index + m[0].length);
          return;
        }
        case "Blockquote":
          lines(from, to, "cm-quote");
          return;
        case "QuoteMark": {
          if (isActive(from)) add(mark("cm-syntax"), from, to);
          else add(hide, from, doc.sliceString(to, to + 1) === " " ? to + 1 : to);
          return;
        }
        case "HorizontalRule":
          if (!isActive(from)) add(Decoration.replace({ widget: new TextWidget("", "cm-hr") }), from, to);
          return;
        case "FencedCode": {
          lines(from, to, "cm-code");
          const open = isActive(from, to);
          for (const m of node.node.getChildren("CodeMark")) {
            if (open) add(mark("cm-syntax"), m.from, m.to);
            else add(hide, m.from, m.to);
          }
          const info = node.node.getChild("CodeInfo");
          if (info) add(mark("cm-codeinfo"), info.from, info.to);
          if (!open) {
            // the fences, hidden, become the block's top and bottom edge
            const first = doc.lineAt(from);
            const last = doc.lineAt(to);
            add(line("cm-fence"), first.from);
            if (last.number > first.number && /^\s*(`{3,}|~{3,})\s*$/.test(last.text)) add(line("cm-fence"), last.from);
          }
          return false;
        }
        case "CodeBlock":
          lines(from, to, "cm-code");
          return false;
        case "Table":
          lines(from, to, "cm-table");
          return false;
        case "Paragraph": {
          const l = doc.lineAt(from);
          if (l.to >= to && NAV.test(l.text)) add(line("cm-nav"), l.from);
          return;
        }
      }
    },
  });

  function link(n) {
    const marks = n.getChildren("LinkMark");
    const url = n.getChild("URL");
    if (marks.length < 2 || !url) return;
    const textFrom = marks[0].to;
    const textTo = marks[1].from;
    const dest = cleanDest(doc.sliceString(url.from, url.to));
    const bad = broken.has(dest);
    const cls = bad ? "cm-link cm-link-broken" : "cm-link";
    const attrs = { "data-href": dest, title: bad ? resolve(path, dest) + " is not in the repository" : dest };
    if (isActive(n.from, n.to)) {
      add(mark(cls, attrs), textFrom, textTo);
      add(mark("cm-url"), url.from, url.to);
      return false;
    }
    add(hide, n.from, textFrom);
    if (textTo > textFrom) add(mark(cls, attrs), textFrom, textTo);
    add(hide, textTo, n.to);
  }

  function image(n) {
    const marks = n.getChildren("LinkMark");
    const url = n.getChild("URL");
    if (marks.length < 2 || !url) return;
    const alt = doc.sliceString(marks[0].to, marks[1].from);
    const dest = cleanDest(doc.sliceString(url.from, url.to));
    const l = doc.lineAt(n.from);
    let inBlock = false;
    for (let p = n.parent; p; p = p.parent) if (p.name === "ListItem" || p.name === "Table" || p.name === "Blockquote") inBlock = true;
    const alone = !inBlock && doc.sliceString(l.from, n.from).trim() === "" && doc.sliceString(n.to, l.to).trim() === "";
    const repoPath = resolve(path, dest);
    const on = isActive(n.from, n.to);
    if (alone && repoPath) {
      const w = new ImageWidget(assetURL(repoPath), alt);
      if (on) add(Decoration.widget({ widget: w, side: 1 }), n.to);
      else add(Decoration.replace({ widget: w }), n.from, n.to);
      return;
    }
    if (!on) add(Decoration.replace({ widget: new TextWidget(alt || dest, "cm-imgalt") }), n.from, n.to);
  }

  function listMark(n) {
    const item = n.parent;
    const text = doc.sliceString(n.from, n.to);
    const on = isActive(n.from);
    const task = n.nextSibling?.name === "Task" ? n.nextSibling : null;
    const marker = task?.getChild("TaskMarker");
    if (marker) {
      const checked = /\[[xX]\]/.test(doc.sliceString(marker.from, marker.to));
      if (on) add(mark("cm-syntax"), n.from, marker.to);
      else add(Decoration.replace({ widget: new CheckWidget(checked) }), n.from, marker.to);
      if (checked) add(mark("cm-done"), marker.to, task.to);
      return;
    }
    if (/^[-*+]$/.test(text)) {
      if (on) add(mark("cm-syntax"), n.from, n.to);
      else add(Decoration.replace({ widget: new TextWidget("•", "cm-bullet") }), n.from, n.to);
    } else add(mark("cm-syntax"), n.from, n.to);
    const after = n.to + 1;
    if (item && after < item.to) {
      const t = TIME.exec(doc.sliceString(after, Math.min(item.to, after + 6)));
      if (t) add(mark("cm-time"), after, after + t[0].length);
    }
  }

  return Decoration.set(out, true);
}

// linkDests are the destinations of the note's relative links and images, for the app to check.
export function linkDests(state) {
  const dests = new Set();
  const tree = ensureSyntaxTree(state, state.doc.length, 500) ?? syntaxTree(state);
  tree.iterate({
    enter(node) {
      if (node.name !== "URL") return;
      const parent = node.node.parent?.name;
      if (parent !== "Link" && parent !== "Image") return;
      const dest = cleanDest(state.doc.sliceString(node.from, node.to));
      if (resolve(state.field(noteField).path, dest)) dests.add(dest);
    },
  });
  return [...dests];
}

// anchorOf is GitHub's anchor for a heading: lowercase, punctuation dropped, spaces as hyphens.
export function anchorOf(text) {
  return text
    .trim()
    .toLowerCase()
    .replace(/[^\p{L}\p{N}\s_-]/gu, "")
    .replace(/\s/g, "-");
}

// headingAt is the start of the heading whose anchor is anchor, or -1.
export function headingAt(state, anchor) {
  const want = anchor.replace(/^#/, "").toLowerCase();
  let found = -1;
  const tree = ensureSyntaxTree(state, state.doc.length, 500) ?? syntaxTree(state);
  tree.iterate({
    enter(node) {
      if (found >= 0 || !/^(ATX|Setext)Heading/.test(node.name)) return;
      const text = state.doc.sliceString(node.from, node.to).split("\n")[0].replace(/^#+\s*/, "").replace(/\s#+\s*$/, "");
      if (anchorOf(text) === want) found = node.from;
      return false;
    },
  });
  return found;
}

const previewPlugin = ViewPlugin.fromClass(
  class {
    constructor(view) {
      this.decorations = buildDecorations(view.state);
    }
    update(u) {
      const effects = u.transactions.some((tr) => tr.effects.some((e) => e.is(setBroken) || e.is(setFocus)));
      if (u.docChanged || u.selectionSet || u.viewportChanged || effects || syntaxTree(u.startState) !== syntaxTree(u.state)) {
        this.decorations = buildDecorations(u.state);
      }
    }
  },
  { decorations: (p) => p.decorations },
);

// The frontmatter folds to one line; it is a block, so its decoration comes from a state field.
function frontmatterDecorations(state) {
  const r = frontmatterRange(state.doc);
  if (!r) return Decoration.none;
  if (touches(state, activeLines(state), r.from, r.to)) {
    const out = [];
    for (let n = 1; n <= r.lines; n++) out.push(line("cm-fm").range(state.doc.line(n).from));
    return Decoration.set(out);
  }
  const summary = frontmatterSummary(state.doc.sliceString(r.from, r.to));
  return Decoration.set([Decoration.replace({ widget: new FrontmatterWidget(summary), block: true }).range(r.from, r.to)]);
}

export const frontmatterField = StateField.define({
  create: frontmatterDecorations,
  update(deco, tr) {
    if (tr.docChanged || tr.selection || tr.effects.some((e) => e.is(setFocus))) return frontmatterDecorations(tr.state);
    return deco;
  },
  provide: (f) => EditorView.decorations.from(f),
});

// "Linked from" comes after the note, as in the terminal browser.
export const backlinksField = StateField.define({
  create: () => Decoration.none,
  update(deco, tr) {
    for (const e of tr.effects) {
      if (e.is(setBacklinks)) {
        if (!e.value.length) return Decoration.none;
        const w = Decoration.widget({ widget: new BacklinksWidget(e.value), block: true, side: 1 });
        return Decoration.set([w.range(tr.state.doc.length)]);
      }
    }
    return tr.docChanged ? deco.map(tr.changes) : deco;
  },
  provide: (f) => EditorView.decorations.from(f),
});

// A click on a rendered link follows it; on the line being edited it places the cursor, and
// Command-click follows.
const clicks = EditorView.domEventHandlers({
  mousedown(e, view) {
    const back = e.target.closest?.("[data-open]");
    if (back) {
      e.preventDefault();
      post({ type: "open", path: back.dataset.open });
      return true;
    }
    const fmw = e.target.closest?.(".cm-fm-summary");
    if (fmw) {
      e.preventDefault();
      view.focus();
      view.dispatch({ selection: { anchor: 4 }, effects: setFocus.of(true) });
      return true;
    }
    const el = e.target.closest?.("[data-href]");
    if (!el) return false;
    const pos = view.posAtDOM(el);
    const editing = activeLines(view.state).has(view.state.doc.lineAt(pos).number);
    if (editing && !e.metaKey) return false;
    e.preventDefault();
    post({ type: "open", href: el.dataset.href });
    return true;
  },
});

export const livePreview = [
  previewPlugin,
  frontmatterField,
  backlinksField,
  clicks,
  EditorView.focusChangeEffect.of((_state, focusing) => setFocus.of(focusing)),
];
