import preact from "@preact/preset-vite";
import { defineConfig } from "vite";

// The live widget on the marketing site: the same UI as v1.js, with the
// in-memory backend compiled in, as one ES module the site bundles as a lazy chunk.
//   pnpm build:site-widget   ->  ../site/src/vendor/widget-demo/demo.js
export default defineConfig({
  plugins: [preact()],
  publicDir: false,
  define: { __OD_WIDGET_MOCK__: "true" },
  build: {
    outDir: "../site/src/vendor/widget-demo",
    // demo.d.ts beside it is written by hand.
    emptyOutDir: false,
    target: "es2019",
    cssCodeSplit: false,
    sourcemap: false,
    minify: true,
    lib: { entry: "widget/src/site.ts", formats: ["es"], fileName: () => "demo.js" },
  },
});
