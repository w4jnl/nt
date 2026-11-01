// Math, written as pandoc has it: $x$ inline, and $$ … $$ on lines of its own as a block. An
// inline formula opens with a $ that has a character right after it and closes with one that has
// a character right before it and no digit after, so "$5 and $10" stays money. KaTeX draws the
// formula in place; the cursor on it shows the source. KaTeX runs here, in the page: nothing
// leaves the device.
import { WidgetType } from "@codemirror/view";
import katex from "katex";

const DOLLAR = 36;
const space = (c) => c === 32 || c === 9 || c === 10 || c === -1;
const digit = (c) => c >= 48 && c <= 57;
const opensBlock = (line) => line.next === DOLLAR && line.text.charCodeAt(line.pos + 1) === DOLLAR;
const CLOSE = /\$\$\s*$/;

// mathExtension adds BlockMath, InlineMath and MathMark to the markdown parser.
export const mathExtension = {
  defineNodes: [{ name: "BlockMath", block: true }, "InlineMath", "MathMark"],
  parseBlock: [
    {
      name: "BlockMath",
      parse(cx, line) {
        if (!opensBlock(line)) return false;
        const start = cx.lineStart + line.pos;
        const rest = line.text.slice(line.pos + 2);
        if (CLOSE.test(rest) && rest.replace(CLOSE, "").trim()) {
          // $$ x $$ on one line
          cx.addElement(cx.elt("BlockMath", start, cx.lineStart + line.text.trimEnd().length));
          cx.nextLine();
          return true;
        }
        while (cx.nextLine()) {
          if (CLOSE.test(line.text)) {
            cx.addElement(cx.elt("BlockMath", start, cx.lineStart + line.text.trimEnd().length));
            cx.nextLine();
            return true;
          }
        }
        cx.addElement(cx.elt("BlockMath", start, cx.prevLineEnd()));
        return true;
      },
      endLeaf: (_cx, line) => opensBlock(line),
    },
  ],
  parseInline: [
    {
      name: "InlineMath",
      parse(cx, next, pos) {
        if (next !== DOLLAR) return -1;
        const after = cx.char(pos + 1);
        if (after === DOLLAR || space(after)) return -1;
        for (let i = pos + 2; i < cx.end; i++) {
          const c = cx.char(i);
          if (c === 10) return -1;
          if (c !== DOLLAR || space(cx.char(i - 1)) || digit(cx.char(i + 1))) continue;
          return cx.addElement(cx.elt("InlineMath", pos, i + 1, [cx.elt("MathMark", pos, pos + 1), cx.elt("MathMark", i, i + 1)]));
        }
        return -1;
      },
    },
  ],
};

// mathSource is the TeX inside a BlockMath or InlineMath node's text.
export function mathSource(text, display) {
  return display ? text.replace(/^\$\$/, "").replace(CLOSE, "").trim() : text.slice(1, -1);
}

// renderMath is the formula as HTML. An error in the TeX shows the source in alert, in place.
export function renderMath(src, display) {
  return katex.renderToString(src, { displayMode: display, throwOnError: false, errorColor: "var(--alert)", output: "html", strict: "ignore" });
}

export class MathWidget extends WidgetType {
  constructor(src, display) {
    super();
    this.src = src;
    this.display = display;
  }
  eq(o) {
    return o.src === this.src && o.display === this.display;
  }
  toDOM() {
    const el = document.createElement(this.display ? "div" : "span");
    el.className = this.display ? "cm-math cm-math-block" : "cm-math";
    el.innerHTML = renderMath(this.src, this.display);
    return el;
  }
  ignoreEvent() {
    return false;
  }
  get estimatedHeight() {
    return this.display ? 60 : -1;
  }
}
