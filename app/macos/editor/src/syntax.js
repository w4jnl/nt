// noteMarkdown is the markdown the page parses: GFM as CodeMirror ships it, with the languages
// a fence can name (code.js) and pandoc's math (math.js). The tests build their states with it.
import { markdown, markdownLanguage } from "@codemirror/lang-markdown";
import { codeLanguages } from "./code.js";
import { mathExtension } from "./math.js";

export function noteMarkdown() {
  return markdown({ base: markdownLanguage, codeLanguages, extensions: [mathExtension] });
}
