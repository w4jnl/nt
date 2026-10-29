// Writing in the editor: the changes made since the note was last saved, laid over a version
// another writer put on disk (nt a, nt todo, a sync merge, another editor), ticking a task, and
// putting an image on a line of its own. Pure functions of text and ChangeSets, so the tests run
// them without a screen.
import { ChangeSet, Text } from "@codemirror/state";

// lines splits s into its lines, each with its "\n"; the last has none when s does not end in one.
function lines(s) {
  const out = [];
  for (let i = 0; i < s.length; ) {
    const j = s.indexOf("\n", i);
    const end = j < 0 ? s.length : j + 1;
    out.push(s.slice(i, end));
    i = end;
  }
  return out;
}

// MAX_CELLS bounds the table the middle of a diff is worked out in; past it the middle is one
// replacement.
const MAX_CELLS = 1 << 20;

// lineDiff is the change from a to b, line by line: the lines both have stay where they are, the
// others are replaced. Its ranges start and end at line starts.
export function lineDiff(a, b) {
  const A = lines(a);
  const B = lines(b);
  let pre = 0;
  while (pre < A.length && pre < B.length && A[pre] === B[pre]) pre++;
  let suf = 0;
  while (suf < A.length - pre && suf < B.length - pre && A[A.length - 1 - suf] === B[B.length - 1 - suf]) suf++;
  const am = A.slice(pre, A.length - suf);
  const bm = B.slice(pre, B.length - suf);
  const hunks = [];
  if (am.length && bm.length && am.length * bm.length <= MAX_CELLS) {
    // the longest common subsequence of the middle lines, then the runs between its lines
    const n = am.length;
    const m = bm.length;
    const w = m + 1;
    const lcs = new Uint32Array((n + 1) * w);
    for (let i = n - 1; i >= 0; i--) {
      for (let j = m - 1; j >= 0; j--) {
        lcs[i * w + j] = am[i] === bm[j] ? lcs[(i + 1) * w + j + 1] + 1 : Math.max(lcs[(i + 1) * w + j], lcs[i * w + j + 1]);
      }
    }
    let i = 0;
    let j = 0;
    let start = null;
    const close = () => {
      if (start && (start[0] < i || start[1] < j)) hunks.push([start[0], i, start[1], j]);
      start = null;
    };
    while (i < n || j < m) {
      if (i < n && j < m && am[i] === bm[j]) {
        close();
        i++;
        j++;
        continue;
      }
      start ??= [i, j];
      if (j < m && (i === n || lcs[i * w + j + 1] >= lcs[(i + 1) * w + j])) j++;
      else i++;
    }
    close();
  } else if (am.length || bm.length) {
    hunks.push([0, am.length, 0, bm.length]);
  }
  const at = [0];
  for (const l of A) at.push(at[at.length - 1] + l.length);
  const changes = hunks.map(([i0, i1, j0, j1]) => ({ from: at[pre + i0], to: at[pre + i1], insert: bm.slice(j0, j1).join("") }));
  return ChangeSet.of(changes, a.length);
}

// clashes says whether theirs replaces a line of base that ours changed. Lines theirs only adds
// between two lines clash with nothing.
export function clashes(theirs, ours, base) {
  const doc = Text.of(base.split("\n"));
  const touched = new Set();
  ours.iterChangedRanges((fromA, toA) => {
    for (let n = doc.lineAt(fromA).number; n <= doc.lineAt(toA).number; n++) touched.add(n);
  });
  let clash = false;
  theirs.iterChangedRanges((fromA, toA) => {
    if (fromA === toA) return;
    for (let n = doc.lineAt(fromA).number; n <= doc.lineAt(toA - 1).number; n++) clash ||= touched.has(n);
  });
  return clash;
}

// rebase lays theirs, the note as another writer left it on disk, over the editor's changes ours
// to base. It returns the change to make to the editor's text and what is then left to save
// (theirs → the editor's text), or null when both changed the same line. Lines both added at one
// place come theirs first.
export function rebase(base, theirs, ours) {
  const change = lineDiff(base, theirs);
  if (clashes(change, ours, base)) return null;
  return { apply: change.map(ours, true), pending: ours.map(change) };
}

// normalize gives text the line breaks the editor keeps: "\n".
export function normalize(text) {
  return text.replace(/\r\n?/g, "\n");
}

const TASK = /^(\s*(?:[-*+]|\d+[.)])\s+\[)([ xX])(\].*)$/;
const DONE = /\s+done:\d{4}-\d{2}-\d{2}/g;

// toggleTask is a todo line checked, with done:<day>, or opened again without it, as nt and the
// nvim plugin do it (docs/format.md §12); null when the line is not a todo.
export function toggleTask(line, day) {
  const m = TASK.exec(line);
  if (!m) return null;
  const rest = m[3].replace(DONE, "").replace(/\s+$/, "");
  return m[2] === " " ? `${m[1]}x${rest} done:${day}` : `${m[1]} ${rest}`;
}

const ITEM = /^(\s*)([-*+] |\d+[.)] )?(\[[ xX]\] )?(.*)$/;

// taskLine is the keyboard bar's task: a line gets a box, "- [ ] " after its indent or "[ ] "
// after the list marker it has; a task loses its box and stays a list item.
export function taskLine(text) {
  const [, indent, marker = "", box = "", rest] = ITEM.exec(text);
  if (box) return indent + marker + rest;
  return indent + (marker || "- ") + "[ ] " + rest;
}

// headingLine is the keyboard bar's heading: a line steps through ## and ### back to text; the
// title's # steps to ##.
export function headingLine(text) {
  const m = /^(#{1,6}) (.*)$/.exec(text);
  if (!m) return "## " + text.trimStart();
  if (m[1].length === 2) return "### " + m[2];
  if (m[1].length >= 3) return m[2];
  return "## " + m[2];
}

// ownLine is the change that puts text on a line of its own at the line holding pos: in its
// place when that line is blank, else after it. The cursor goes to the end of text.
export function ownLine(doc, pos, text) {
  const l = doc.lineAt(Math.min(Math.max(pos, 0), doc.length));
  if (l.text.trim() === "") return { changes: { from: l.from, to: l.to, insert: text }, selection: { anchor: l.from + text.length } };
  return { changes: { from: l.to, insert: "\n" + text }, selection: { anchor: l.to + 1 + text.length } };
}

// paragraphAt is the run of non-blank lines around pos, as line numbers [first, last]; a blank
// line is a run of its own.
export function paragraphAt(doc, pos) {
  const l = doc.lineAt(pos);
  if (l.text.trim() === "") return [l.number, l.number];
  let a = l.number;
  let b = l.number;
  while (a > 1 && doc.line(a - 1).text.trim() !== "") a--;
  while (b < doc.lines && doc.line(b + 1).text.trim() !== "") b++;
  return [a, b];
}
