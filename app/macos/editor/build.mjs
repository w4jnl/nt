// Builds the editor into ../Nt/Resources/editor: editor.js (CodeMirror and nt's live preview, one
// file, no network), mermaid.js (diagrams, loaded only when a note has one), katex.css and
// fonts/ (math), editor.css, index.html, and LICENSES.txt with the licence of every package
// bundled. The output is committed, so building the app needs no Node; make mac-editor runs this
// after a change under src/.
import { build } from "esbuild";
import { copyFileSync, existsSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const here = new URL(".", import.meta.url).pathname;
const out = join(here, "../Nt/Resources/editor");
mkdirSync(out, { recursive: true });

const bundle = (entry, file) =>
  build({
    entryPoints: [join(here, "src", entry)],
    bundle: true,
    format: "iife",
    minify: true,
    legalComments: "none",
    logLevel: "error",
    target: ["safari18"],
    outfile: join(out, file),
    metafile: true,
  });
const results = [await bundle("index.js", "editor.js"), await bundle("mermaid-entry.js", "mermaid.js")];
for (const f of ["index.html", "editor.css"]) copyFileSync(join(here, "src", f), join(out, f));

// KaTeX's stylesheet and its fonts (woff2 only; WebKit never asks for the others).
const katex = join(here, "node_modules/katex/dist");
copyFileSync(join(katex, "katex.min.css"), join(out, "katex.css"));
rmSync(join(out, "fonts"), { recursive: true, force: true });
mkdirSync(join(out, "fonts"));
for (const f of readdirSync(join(katex, "fonts"))) if (f.endsWith(".woff2")) copyFileSync(join(katex, "fonts", f), join(out, "fonts", f));

// The packages in the bundles, each with its licence, as their licences ask.
const pkgs = new Set(["katex"]);
for (const r of results) {
  for (const input of Object.keys(r.metafile.inputs)) {
    const m = /node_modules\/((?:@[^/]+\/)?[^/]+)\//.exec(input);
    if (m) pkgs.add(m[1]);
  }
}
let text = "The editor bundles these packages (editor.js, mermaid.js, katex.css and fonts/).\n";
for (const name of [...pkgs].sort()) {
  const dir = join(here, "node_modules", name);
  const meta = JSON.parse(readFileSync(join(dir, "package.json"), "utf8"));
  const file = ["LICENSE", "LICENSE.md", "LICENSE.txt", "LICENCE", "license", "License.txt"].map((f) => join(dir, f)).find(existsSync);
  text += `\n${"=".repeat(72)}\n${name} ${meta.version} (${meta.license})\n${"=".repeat(72)}\n\n`;
  text += file ? readFileSync(file, "utf8").trim() + "\n" : `${meta.license}; no licence file in the package.\n`;
}
writeFileSync(join(out, "LICENSES.txt"), text);
console.log(`built ${out}: ${pkgs.size} packages`);
