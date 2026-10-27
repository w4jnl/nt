// Builds the editor into ../Nt/Resources/editor: editor.js (CodeMirror and nt's live preview, one
// file, no network), editor.css, index.html, and LICENSES.txt with the licence of every package
// bundled. The output is committed, so building the app needs no Node; make mac-editor runs this
// after a change under src/.
import { build } from "esbuild";
import { copyFileSync, existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";

const here = new URL(".", import.meta.url).pathname;
const out = join(here, "../Nt/Resources/editor");
mkdirSync(out, { recursive: true });

const result = await build({
  entryPoints: [join(here, "src/index.js")],
  bundle: true,
  format: "iife",
  minify: true,
  legalComments: "none",
  target: ["safari18"],
  outfile: join(out, "editor.js"),
  metafile: true,
});
for (const f of ["index.html", "editor.css"]) copyFileSync(join(here, "src", f), join(out, f));

// The packages in the bundle, each with its licence, as their licences ask.
const pkgs = new Set();
for (const input of Object.keys(result.metafile.inputs)) {
  const m = /node_modules\/((?:@[^/]+\/)?[^/]+)\//.exec(input);
  if (m) pkgs.add(m[1]);
}
let text = "The editor bundles these packages (editor.js).\n";
for (const name of [...pkgs].sort()) {
  const dir = join(here, "node_modules", name);
  const meta = JSON.parse(readFileSync(join(dir, "package.json"), "utf8"));
  const file = ["LICENSE", "LICENSE.md", "LICENSE.txt"].map((f) => join(dir, f)).find(existsSync);
  text += `\n${"=".repeat(72)}\n${name} ${meta.version} (${meta.license})\n${"=".repeat(72)}\n\n`;
  text += file ? readFileSync(file, "utf8").trim() + "\n" : `${meta.license}; no licence file in the package.\n`;
}
writeFileSync(join(out, "LICENSES.txt"), text);
console.log(`built ${out}: ${pkgs.size} packages`);
