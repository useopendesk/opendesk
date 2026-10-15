// Bundles main, the preloads and the server picker into dist/ with esbuild,
// and copies the picker's static files (HTML, CSS, brand tokens, Geist).

import { build } from "esbuild";
import { cpSync, mkdirSync, rmSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const out = join(root, "dist");
const require = createRequire(import.meta.url);

rmSync(out, { recursive: true, force: true });
mkdirSync(join(out, "picker", "fonts"), { recursive: true });

const common = { bundle: true, sourcemap: "linked", logLevel: "warning", legalComments: "none", minifySyntax: true };

await Promise.all([
  build({
    ...common,
    entryPoints: [join(root, "src/main/index.ts")],
    outfile: join(out, "main.cjs"),
    platform: "node",
    format: "cjs",
    target: "node22",
    external: ["electron", "electron-updater"],
  }),
  // Sandboxed preloads: CommonJS, and only `electron` may be required.
  ...[
    ["src/preload/instance.ts", "preload.cjs"],
    ["src/preload/picker.ts", "picker-preload.cjs"],
  ].map(([src, file]) =>
    build({ ...common, entryPoints: [join(root, src)], outfile: join(out, file), platform: "node", format: "cjs", target: "chrome130", external: ["electron"] }),
  ),
  build({ ...common, entryPoints: [join(root, "src/picker/picker.ts")], outfile: join(out, "picker/picker.js"), platform: "browser", format: "iife", target: "chrome130" }),
]);

for (const f of ["index.html", "offline.html", "picker.css"]) cpSync(join(root, "src/picker", f), join(out, "picker", f));
cpSync(join(root, "..", "brand", "tokens.css"), join(out, "picker", "tokens.css"));
cpSync(join(root, "..", "brand", "logo", "mark.svg"), join(out, "picker", "mark.svg"));
const geist = dirname(require.resolve("@fontsource-variable/geist/package.json"));
cpSync(join(geist, "files", "geist-latin-wght-normal.woff2"), join(out, "picker", "fonts", "geist.woff2"));

console.log("built desktop/dist");
