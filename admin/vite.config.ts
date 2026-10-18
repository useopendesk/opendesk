import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";

// One source, two homes: https://admin.opendesk.sh/ (Cloudflare Pages, base "/")
// and <APP_URL>/admin/ on a self-hosted binary (`pnpm build:embed`, base "/admin/").
// Every asset, the router and config.js derive from import.meta.env.BASE_URL.
const base = process.env.ADMIN_BASE ?? "/";
if (!base.startsWith("/") || !base.endsWith("/")) throw new Error(`ADMIN_BASE must start and end with "/", got ${base}`);

export default defineConfig({
  base,
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(import.meta.dirname, "./src") } },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    sourcemap: false,
    target: "es2022",
  },
  server: { port: 5174, strictPort: true },
  preview: { port: 5174 },
  optimizeDeps: { entries: ["index.html", "src/**/*.{ts,tsx}", "!src/**/*.test.{ts,tsx}"] },
});
