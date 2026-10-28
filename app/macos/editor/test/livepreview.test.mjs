// The live preview without a screen: what buildDecorations hides, replaces and styles for a
// state, and the helpers the app relies on. Run with npm test.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { EditorState } from "@codemirror/state";
import { markdown, markdownLanguage } from "@codemirror/lang-markdown";
import {
  anchorOf,
  assetURL,
  buildDecorations,
  frontmatterField,
  frontmatterSummary,
  headingAt,
  linkDests,
  noteField,
  resolve,
  setBroken,
  setFocus,
} from "../src/livepreview.js";

function state(doc, { path = "homelab/2026-10-21-nas-setup.md", cursor = 0, focused = false, broken = [] } = {}) {
  let s = EditorState.create({
    doc,
    selection: { anchor: cursor },
    extensions: [noteField.init(() => ({ path, broken: new Set(), focused: false })), markdown({ base: markdownLanguage }), frontmatterField],
  });
  s = s.update({ effects: [setFocus.of(focused), setBroken.of(broken)] }).state;
  return s;
}

function decos(set) {
  const out = [];
  set.between(0, 1e9, (from, to, value) => {
    out.push({
      from,
      to,
      cls: value.spec.class ?? "",
      widget: value.spec.widget?.constructor.name ?? "",
      attrs: value.spec.attributes ?? {},
      w: value.spec.widget,
      block: !!value.spec.block,
    });
  });
  return out;
}

const hidden = (ds, from, to) => ds.some((d) => d.from === from && d.to === to && !d.cls && !d.widget);

test("a heading's marks are hidden off the cursor line and hang on it", () => {
  const doc = "# NAS setup\n\nFour disks.";
  assert.ok(hidden(decos(buildDecorations(state(doc))), 0, 2), "the # is shown without focus");
  const on = decos(buildDecorations(state(doc, { cursor: 5, focused: true })));
  assert.ok(on.some((d) => d.cls === "cm-hang" && d.from === 0 && d.to === 2), "the # does not hang on the cursor line");
  assert.ok(on.some((d) => d.cls === "cm-h1"), "the line is not a heading");
});

test("a link shows its text, and its target only on the cursor line", () => {
  const doc = "Nightly [backups](2026-10-22-backups.md) go out.";
  const off = decos(buildDecorations(state(doc)));
  assert.ok(hidden(off, 8, 9), "[ is shown");
  const link = off.find((d) => d.cls === "cm-link");
  assert.deepEqual([link.from, link.to, link.attrs["data-href"]], [9, 16, "2026-10-22-backups.md"]);
  assert.ok(hidden(off, 16, 40), "](target) is shown");
  const on = decos(buildDecorations(state(doc, { cursor: 2, focused: true })));
  assert.ok(on.some((d) => d.cls === "cm-url"), "the target is hidden on the cursor line");
  assert.ok(!hidden(on, 8, 9), "[ is hidden on the cursor line");
});

test("a link to nothing is struck through", () => {
  const doc = "From the [old wiki](../wiki/router.md).";
  const ds = decos(buildDecorations(state(doc, { broken: ["../wiki/router.md"] })));
  const link = ds.find((d) => d.cls.includes("cm-link"));
  assert.equal(link.cls, "cm-link cm-link-broken");
  assert.match(link.attrs.title, /wiki\/router\.md is not in the repository/);
});

test("tasks become checkboxes, done ones muted, dates in mono", () => {
  const doc = "- [ ] Replace the fan\n- [x] Update the firmware done:2026-10-22";
  const ds = decos(buildDecorations(state(doc)));
  const checks = ds.filter((d) => d.widget === "CheckWidget");
  assert.deepEqual(
    checks.map((d) => d.w.checked),
    [false, true],
  );
  assert.ok(ds.some((d) => d.cls === "cm-done"), "the done task is not muted");
  assert.ok(ds.some((d) => d.cls === "cm-meta" && doc.slice(d.from, d.to) === "done:2026-10-22"));
});

test("a journal line's time is set apart", () => {
  const doc = "## Log\n\n- 08:58 Stand-up\n- 10:14 Fan is loud";
  const ds = decos(buildDecorations(state(doc, { path: "daily/2026/2026-10-27.md" })));
  assert.deepEqual(
    ds.filter((d) => d.cls === "cm-time").map((d) => doc.slice(d.from, d.to)),
    ["08:58", "10:14"],
  );
  assert.equal(ds.filter((d) => d.widget === "TextWidget" && d.w.cls === "cm-bullet").length, 2);
});

test("an image alone on its line is drawn from the repository; a remote one shows its alt", () => {
  const doc = "![The NAS](assets/2026-10-21-nas.png)\n\n![tracker](https://example.com/p.png)\n\nText ![inline](assets/x.png) here.";
  const ds = decos(buildDecorations(state(doc)));
  const imgs = ds.filter((d) => d.widget === "ImageWidget");
  assert.equal(imgs.length, 1);
  assert.equal(imgs[0].w.src, "nt-asset://repo/homelab/assets/2026-10-21-nas.png");
  assert.equal(imgs[0].w.alt, "The NAS");
  const alts = ds.filter((d) => d.widget === "TextWidget" && d.w.cls === "cm-imgalt").map((d) => d.w.text);
  assert.deepEqual(alts, ["tracker", "inline"]);
});

test("the frontmatter folds to one line, and its YAML is not read as markdown", () => {
  const doc = "---\ntitle: NAS setup\ndate: 2026-10-21\ntags: [homelab, storage]\n---\n\n# NAS setup\n";
  const s = state(doc);
  const fm = decos(s.field(frontmatterField));
  assert.equal(fm.length, 1);
  assert.ok(fm[0].block && fm[0].from === 0);
  assert.equal(fm[0].w.summary, "Wednesday 21 October 2026 · homelab, storage");
  const ds = decos(buildDecorations(s));
  assert.ok(!ds.some((d) => d.from < fm[0].to), "decorations inside the frontmatter");
  const open = decos(state(doc, { cursor: 6, focused: true }).field(frontmatterField));
  assert.ok(open.every((d) => d.cls === "cm-fm"), "the frontmatter stays folded with the cursor in it");
});

test("frontmatter summaries", () => {
  assert.equal(frontmatterSummary("---\ntitle: Monday\ndate: 2026-10-26\ntype: daily\n---"), "");
  assert.equal(frontmatterSummary("---\ntags:\n  - work\n  - 'q4'\n---"), "work, q4");
  assert.equal(frontmatterSummary("---\nagent: claude-code\n---"), "properties");
});

test("links resolve against the note's folder", () => {
  assert.equal(resolve("daily/2026/2026-10-27.md", "../../homelab/a%20b.md#setup"), "homelab/a b.md");
  assert.equal(resolve("homelab/n.md", "<my notes.md>"), "homelab/my notes.md");
  assert.equal(resolve("n.md", "../outside.md"), "");
  assert.equal(resolve("n.md", "https://example.com"), "");
  assert.equal(resolve("n.md", "#setup"), "");
  assert.equal(assetURL("homelab/assets/a b.png"), "nt-asset://repo/homelab/assets/a%20b.png");
  const s = state("[a](b.md) [c](https://x.y) ![d](assets/e.png) [f](#g)");
  assert.deepEqual(linkDests(s).sort(), ["assets/e.png", "b.md"]);
});

test("anchors follow GitHub", () => {
  assert.equal(anchorOf("Mounting the pool"), "mounting-the-pool");
  assert.equal(anchorOf("Q4: goals & risks"), "q4-goals--risks");
  const s = state("# NAS\n\n## Mounting the pool\n\ntext");
  assert.equal(headingAt(s, "#mounting-the-pool"), 7);
  assert.equal(headingAt(s, "nothing"), -1);
});

test("editor.css carries the design system's colours", () => {
  const tokens = JSON.parse(readFileSync(new URL("../../../../docs/design-system/tokens.json", import.meta.url)));
  const css = readFileSync(new URL("../src/editor.css", import.meta.url), "utf8");
  const [light, dark] = css.split("@media (prefers-color-scheme: dark)");
  const value = (block, name) => new RegExp(`--${name}:\\s*(#[0-9A-Fa-f]{6});`).exec(block)?.[1];
  const names = ["ground", "surface", "surface-sunk", "line", "line-strong", "ink", "ink-muted", "ink-faint", "accent", "accent-soft", "ok", "warn", "alert"];
  for (const name of names) {
    const t = tokens.color.tokens.find((t) => t.name === name);
    assert.equal(value(light, name), t.value.light, `${name} light`);
    assert.equal(value(dark, name), t.value.dark, `${name} dark`);
  }
});
