// Writing without a screen: the line diff, laying a version from disk under unsaved changes,
// ticking tasks, and where a pasted image goes. Run with npm test.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ChangeSet, EditorState, Text } from "@codemirror/state";
import { clashes, lineDiff, normalize, ownLine, paragraphAt, rebase, toggleTask } from "../src/edits.js";

// edit is the text after the person's changes, and those changes as a ChangeSet from base.
function edit(base, ...specs) {
  let s = EditorState.create({ doc: base });
  let changes = ChangeSet.empty(base.length);
  for (const spec of specs) {
    const tr = s.update({ changes: spec });
    changes = changes.compose(tr.changes);
    s = tr.state;
  }
  return { text: s.doc.toString(), changes };
}

test("lineDiff turns a into b and keeps the lines both have", () => {
  const cases = [
    ["a\nb\nc\n", "a\nb\nc\n"],
    ["a\nb\nc\n", "a\nb\nc\nd\n"],
    ["a\nb\nc\n", "z\na\nb\nc\n"],
    ["a\nb\nc\n", "a\nB\nc\n"],
    ["a\nb\nc\nd\ne\n", "a\nc\nd\nX\ne\nf\n"],
    ["", "new\n"],
    ["only\n", ""],
    ["no newline", "no newline\nmore"],
    ["x\ny\nx\ny\n", "y\nx\ny\nx\n"],
  ];
  for (const [a, b] of cases) {
    const c = lineDiff(a, b);
    assert.equal(c.apply(Text.of(a.split("\n"))).toString(), b, JSON.stringify([a, b]));
  }
  // an appended line is one insertion at the end, not a rewrite
  const ranges = [];
  lineDiff("a\nb\n", "a\nb\nc\n").iterChanges((fromA, toA, fromB, toB, ins) => ranges.push([fromA, toA, ins.toString()]));
  assert.deepEqual(ranges, [[4, 4, "c\n"]]);
});

test("rebase lays a line appended on disk under what is being typed", () => {
  const base = "# Monday\n\n## Log\n\n- 09:00 coffee\n";
  const theirs = base + "- 09:12 fan in bay 2\n";
  const mine = edit(base, { from: 8, insert: " 26 October" }); // typing in the heading
  const r = rebase(base, theirs, mine.changes);
  assert.ok(r);
  const merged = r.apply.apply(Text.of(mine.text.split("\n"))).toString();
  assert.equal(merged, "# Monday 26 October\n\n## Log\n\n- 09:00 coffee\n- 09:12 fan in bay 2\n");
  // what is left to save applies to theirs and gives the same text
  assert.equal(r.pending.apply(Text.of(theirs.split("\n"))).toString(), merged);
});

test("rebase puts lines added on disk before the line being typed at the same place", () => {
  const base = "- 09:00 coffee\n";
  const mine = edit(base, { from: base.length, insert: "- 09:30 standup" });
  const theirs = base + "- 09:12 fan\n";
  const r = rebase(base, theirs, mine.changes);
  assert.equal(r.apply.apply(Text.of(mine.text.split("\n"))).toString(), "- 09:00 coffee\n- 09:12 fan\n- 09:30 standup");
});

test("rebase gives up when both changed the same line", () => {
  const base = "- [ ] fan\n- [ ] disks\n";
  const mine = edit(base, { from: 8, insert: " in bay 2" });
  const theirs = "- [x] fan done:2026-10-28\n- [ ] disks\n";
  assert.equal(rebase(base, theirs, mine.changes), null);
  assert.equal(clashes(lineDiff(base, theirs), mine.changes, base), true);
  // a different line is fine
  const other = "- [ ] fan\n- [x] disks done:2026-10-28\n";
  assert.ok(rebase(base, other, mine.changes));
});

test("rebase with nothing unsaved is the version on disk", () => {
  const base = "a\nb\n";
  const theirs = "a\nb\nc\n";
  const r = rebase(base, theirs, ChangeSet.empty(base.length));
  assert.equal(r.apply.apply(Text.of(base.split("\n"))).toString(), theirs);
  assert.ok(r.pending.empty);
});

test("normalize keeps only \\n line breaks", () => {
  assert.equal(normalize("a\r\nb\rc\n"), "a\nb\nc\n");
});

test("toggleTask checks with done: and opens again without it", () => {
  const day = "2026-10-28";
  const cases = [
    ["- [ ] Replace the fan", "- [x] Replace the fan done:2026-10-28"],
    ["- [x] Replace the fan done:2026-10-27", "- [ ] Replace the fan"],
    ["  * [X] nested due:2026-11-01 done:2026-10-27  ", "  * [ ] nested due:2026-11-01"],
    ["1. [ ] numbered", "1. [x] numbered done:2026-10-28"],
    ["+ [ ] plus  ", "+ [x] plus done:2026-10-28"],
    ["- plain item", null],
    ["text [ ] not a task", null],
  ];
  for (const [line, want] of cases) assert.equal(toggleTask(line, day), want, line);
});

test("ownLine puts an image on a line of its own", () => {
  const doc = Text.of("Four disks.\n\nThe end.".split("\n"));
  const img = "![rack](assets/2026-10-28-rack.png)";
  // in a line of text: after it
  let spec = ownLine(doc, 3, img);
  assert.deepEqual(spec.changes, { from: 11, insert: "\n" + img });
  // on a blank line: in its place
  spec = ownLine(doc, 12, img);
  assert.deepEqual(spec.changes, { from: 12, to: 12, insert: img });
  assert.equal(spec.selection.anchor, 12 + img.length);
});

test("paragraphAt is the run of lines around the cursor", () => {
  const doc = Text.of("# Title\n\nOne\ntwo\n\nThree".split("\n"));
  assert.deepEqual(paragraphAt(doc, doc.line(4).from), [3, 4]);
  assert.deepEqual(paragraphAt(doc, doc.line(2).from), [2, 2]);
  assert.deepEqual(paragraphAt(doc, doc.length), [6, 6]);
});
