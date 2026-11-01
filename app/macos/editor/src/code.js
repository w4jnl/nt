// Code in a fenced block is parsed in its language, when the fence names one of these, and
// coloured as the terminal colours it (internal/render, Chroma): keywords in accent, strings in
// ok, numbers in warn, comments muted, types and builtins in accent, functions and classes bold,
// operators and punctuation muted. Few colours on purpose: code in a note is read, not edited.
// A fence naming another language, or none, stays plain mono.
import { LanguageDescription, LanguageSupport, StreamLanguage } from "@codemirror/language";
import { highlightTree, tagHighlighter, tags as t } from "@lezer/highlight";
import { javascript } from "@codemirror/lang-javascript";
import { python } from "@codemirror/lang-python";
import { go } from "@codemirror/lang-go";
import { json } from "@codemirror/lang-json";
import { yaml } from "@codemirror/lang-yaml";
import { css } from "@codemirror/lang-css";
import { html } from "@codemirror/lang-html";
import { sql } from "@codemirror/lang-sql";
import { rust } from "@codemirror/lang-rust";
import { cpp } from "@codemirror/lang-cpp";
import { shell } from "@codemirror/legacy-modes/mode/shell";
import { swift } from "@codemirror/legacy-modes/mode/swift";

const lang = (name, alias, support) => LanguageDescription.of({ name, alias, support });
const legacy = (mode) => new LanguageSupport(StreamLanguage.define(mode));

// codeLanguages are the languages a fence can name (`js`, `python`, `sh`, …), matched by name
// or alias, case ignored.
export const codeLanguages = [
  lang("JavaScript", ["js", "jsx", "mjs", "node"], javascript({ jsx: true })),
  lang("TypeScript", ["ts", "tsx"], javascript({ jsx: true, typescript: true })),
  lang("Python", ["py"], python()),
  lang("Go", ["golang"], go()),
  lang("JSON", ["jsonc"], json()),
  lang("YAML", ["yml"], yaml()),
  lang("CSS", [], css()),
  lang("HTML", ["htm"], html()),
  lang("SQL", ["sqlite", "postgres", "postgresql", "psql", "mysql"], sql()),
  lang("Rust", ["rs"], rust()),
  lang("C", ["h"], cpp()),
  lang("C++", ["cpp", "cc", "cxx", "hpp"], cpp()),
  lang("Shell", ["sh", "bash", "zsh", "fish", "console", "shell"], legacy(shell)),
  lang("Swift", [], legacy(swift)),
];

// The classes editor.css colours, from the tokens, each the Chroma role the terminal gives it.
const highlighter = tagHighlighter([
  { tag: [t.keyword, t.self, t.modifier], class: "cm-kw" },
  { tag: [t.typeName, t.tagName, t.namespace, t.standard(t.variableName), t.standard(t.name)], class: "cm-type" },
  { tag: [t.string, t.character, t.regexp, t.docString], class: "cm-str" },
  { tag: [t.number, t.bool, t.null, t.atom, t.escape, t.color, t.unit, t.constant(t.name), t.annotation], class: "cm-num" },
  { tag: [t.comment, t.meta], class: "cm-cmt" },
  { tag: [t.function(t.variableName), t.function(t.propertyName), t.className], class: "cm-fn" },
  { tag: [t.operator, t.punctuation], class: "cm-op" },
  { tag: t.invalid, class: "cm-bad" },
]);

// codeMarks calls put(from, to, class) for each coloured run of the code between from and to,
// from the language's parse nested in the markdown tree.
export function codeMarks(tree, from, to, put) {
  highlightTree(tree, highlighter, put, from, to);
}
