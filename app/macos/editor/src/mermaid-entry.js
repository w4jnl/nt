// mermaid.js: mermaid as one file beside editor.js, loaded by diagram.js the first time a note
// has a diagram. It is built apart so that a note without one pays nothing for it.
import mermaid from "mermaid";

window.mermaid = mermaid;
