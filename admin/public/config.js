// Runtime configuration of the admin panel. Served by the self-hosted binary at
// <APP_URL>/admin/config.js, or written at deploy time on Cloudflare Pages
// (deploy/cloudflare/pages/admin/write-config.mjs). Empty values fall back to the
// VITE_* build-time env (src/lib/config.ts).
window.__OPENDESK_ADMIN__ = window.__OPENDESK_ADMIN__ || {};
