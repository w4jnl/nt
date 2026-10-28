// The editor page the Mac app shows a note in. The app calls window.nt.load with the note's text
// and path; the page tells the app what it needs through post: links to check, a link to
// follow, the word count. Nothing here touches the network or the disk.
import { EditorState } from "@codemirror/state";
import { EditorView, drawSelection } from "@codemirror/view";
import { markdown, markdownLanguage } from "@codemirror/lang-markdown";
import { post } from "./bridge.js";
import { frontmatterRange, headingAt, linkDests, livePreview, noteField, setBacklinks, setBroken } from "./livepreview.js";

function extensions(path, readOnly) {
  return [
    noteField.init(() => ({ path, broken: new Set(), focused: false })),
    markdown({ base: markdownLanguage, addKeymap: false }),
    EditorView.lineWrapping,
    EditorState.readOnly.of(readOnly),
    drawSelection(),
    EditorView.contentAttributes.of({ spellcheck: "false", autocorrect: "off", autocapitalize: "off" }),
    livePreview,
  ];
}

const view = new EditorView({
  parent: document.getElementById("editor"),
  state: EditorState.create({ doc: "", extensions: extensions("", true) }),
});

function words(state) {
  const fm = frontmatterRange(state.doc);
  const text = state.doc.sliceString(fm ? fm.to : 0);
  return text.split(/\s+/).filter((w) => /[\p{L}\p{N}]/u.test(w)).length;
}

function scrollToAnchor(anchor) {
  const at = headingAt(view.state, anchor);
  if (at >= 0) view.dispatch({ effects: EditorView.scrollIntoView(at, { y: "start", yMargin: 24 }) });
}

window.nt = {
  // load shows a note: {doc, path, readOnly, backlinks: [{path, title, folder}], anchor, line}.
  load({ doc, path, readOnly = true, backlinks = [], anchor = "", line = 0 }) {
    view.setState(EditorState.create({ doc, extensions: extensions(path, readOnly) }));
    view.dispatch({ effects: setBacklinks.of(backlinks) });
    view.scrollDOM.scrollTop = 0;
    if (anchor) requestAnimationFrame(() => scrollToAnchor(anchor));
    else if (line > 0 && line <= view.state.doc.lines) {
      const at = view.state.doc.line(line).from;
      requestAnimationFrame(() => view.dispatch({ effects: EditorView.scrollIntoView(at, { y: "center" }) }));
    }
    post({ type: "check", dests: linkDests(view.state) });
    post({ type: "stats", words: words(view.state) });
  },
  // broken names the link destinations that point at nothing.
  broken(dests) {
    view.dispatch({ effects: setBroken.of(dests) });
  },
  anchor: scrollToAnchor,
};

post({ type: "ready" });
