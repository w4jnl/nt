// A diagram: a fenced block whose language is mermaid is drawn in place while the cursor is
// outside it, in nt's colours, and is its source when the cursor is on it. Mermaid is large, so
// it is a second file beside editor.js, mermaid.js, loaded the first time a note needs it; it
// comes from the app like everything on this page, and the page's CSP allows nothing else.
import { WidgetType } from "@codemirror/view";

export const isDiagram = (info) => /^mermaid\b/i.test(info.trim());

// mermaid.js sits beside editor.js, wherever the page loaded that from.
const beside = typeof document !== "undefined" && document.currentScript?.src ? new URL("mermaid.js", document.currentScript.src).href : "mermaid.js";

let loading = null;
export function loadMermaid() {
  if (!loading) {
    loading = new Promise((ok, fail) => {
      const s = document.createElement("script");
      s.src = beside;
      s.onload = () => (window.mermaid ? ok(window.mermaid) : fail(new Error("mermaid.js did not load")));
      s.onerror = () => fail(new Error("mermaid.js did not load"));
      document.head.append(s);
    });
  }
  return loading;
}

const scheme = () => matchMedia("(prefers-color-scheme: dark)");

// theme maps the page's tokens (editor.css) onto mermaid's base theme.
function theme() {
  const cs = getComputedStyle(document.documentElement);
  const v = (n) => cs.getPropertyValue(n).trim();
  return {
    startOnLoad: false,
    securityLevel: "strict",
    theme: "base",
    darkMode: scheme().matches,
    fontFamily: v("--sans"),
    themeVariables: {
      fontFamily: v("--sans"),
      fontSize: "14px",
      background: v("--ground"),
      textColor: v("--ink"),
      titleColor: v("--ink"),
      lineColor: v("--line-strong"),
      primaryColor: v("--accent-soft"),
      primaryTextColor: v("--ink"),
      primaryBorderColor: v("--accent"),
      secondaryColor: v("--surface-sunk"),
      secondaryTextColor: v("--ink"),
      secondaryBorderColor: v("--line"),
      tertiaryColor: v("--surface"),
      tertiaryTextColor: v("--ink"),
      tertiaryBorderColor: v("--line"),
      mainBkg: v("--accent-soft"),
      nodeBorder: v("--accent"),
      nodeTextColor: v("--ink"),
      clusterBkg: v("--surface"),
      clusterBorder: v("--line"),
      edgeLabelBackground: v("--ground"),
      noteBkgColor: v("--surface-sunk"),
      noteTextColor: v("--ink"),
      noteBorderColor: v("--line"),
      actorBkg: v("--accent-soft"),
      actorBorder: v("--accent"),
      actorTextColor: v("--ink"),
      actorLineColor: v("--line-strong"),
      signalColor: v("--ink"),
      signalTextColor: v("--ink"),
      labelBoxBkgColor: v("--surface-sunk"),
      labelBoxBorderColor: v("--line"),
      labelTextColor: v("--ink"),
      loopTextColor: v("--ink"),
      activationBkgColor: v("--surface-sunk"),
      activationBorderColor: v("--line-strong"),
      sectionBkgColor: v("--surface-sunk"),
      altSectionBkgColor: v("--surface"),
      sectionBkgColor2: v("--accent-soft"),
      taskBkgColor: v("--accent-soft"),
      taskBorderColor: v("--accent"),
      taskTextColor: v("--ink"),
      taskTextDarkColor: v("--ink"),
      taskTextLightColor: v("--ink"),
      taskTextOutsideColor: v("--ink"),
      activeTaskBkgColor: v("--accent"),
      activeTaskBorderColor: v("--accent"),
      doneTaskBkgColor: v("--surface-sunk"),
      doneTaskBorderColor: v("--line-strong"),
      critBkgColor: v("--alert"),
      critBorderColor: v("--alert"),
      gridColor: v("--line"),
      todayLineColor: v("--warn"),
      pie1: v("--accent"),
      pie2: v("--ok"),
      pie3: v("--warn"),
      pie4: v("--alert"),
      pie5: v("--line-strong"),
      pie6: v("--accent-soft"),
      pie7: v("--ink-muted"),
      pie8: v("--surface-sunk"),
      pieTitleTextColor: v("--ink"),
      pieSectionTextColor: v("--ink"),
      pieLegendTextColor: v("--ink"),
      pieStrokeColor: v("--ground"),
      pieOuterStrokeColor: v("--ground"),
      git0: v("--accent"),
      git1: v("--ok"),
      git2: v("--warn"),
      git3: v("--alert"),
      gitBranchLabel0: v("--ground"),
      gitBranchLabel1: v("--ground"),
      gitBranchLabel2: v("--ground"),
      gitBranchLabel3: v("--ground"),
      commitLabelColor: v("--ink"),
      commitLabelBackground: v("--surface-sunk"),
      tagLabelColor: v("--ink"),
      tagLabelBackground: v("--accent-soft"),
      tagLabelBorder: v("--accent"),
      attributeBackgroundColorOdd: v("--surface"),
      attributeBackgroundColorEven: v("--surface-sunk"),
    },
  };
}

let count = 0;
// renderDiagram is the diagram as SVG, drawn in the current scheme's colours.
export async function renderDiagram(text) {
  const mermaid = await loadMermaid();
  mermaid.initialize(theme());
  const { svg } = await mermaid.render(`nt-diagram-${++count}`, text);
  return svg;
}

export class DiagramWidget extends WidgetType {
  constructor(text) {
    super();
    this.text = text;
  }
  eq(o) {
    return o.text === this.text;
  }
  toDOM() {
    const el = document.createElement("div");
    el.className = "cm-diagram";
    const draw = () =>
      renderDiagram(this.text).then(
        (svg) => {
          el.classList.remove("cm-diagram-error");
          el.innerHTML = svg;
        },
        (e) => {
          el.classList.add("cm-diagram-error");
          el.textContent = String(e?.message ?? e).trim();
        },
      );
    draw();
    const mq = scheme();
    mq.addEventListener("change", draw);
    el.ntUnwatch = () => mq.removeEventListener("change", draw);
    return el;
  }
  destroy(el) {
    el.ntUnwatch?.();
  }
  ignoreEvent() {
    return false;
  }
  get estimatedHeight() {
    return 240;
  }
}
