#!/usr/bin/env node
// Writes the runtime config of the opendesk-admin Pages project:
//
//   <outdir>/config.js     window.__OPENDESK_ADMIN__ = {...}, read by the admin panel
//
// The self-hosted binary serves <APP_URL>/admin/config.js itself; on Cloudflare
// Pages there is no server, so the deploy writes the file into the build output
// before uploading it. Keys match admin/src/lib/config.ts.
//
//   node deploy/cloudflare/pages/admin/write-config.mjs admin/dist
//
// Env: ADMIN_API_URL (default https://api.opendesk.sh), ADMIN_APP_URL (default
// https://app.opendesk.sh), TURNSTILE_SITE_KEY (unset: no Turnstile widget),
// ADMIN_ENV_LABEL (a short label shown in the panel, e.g. "Production"; unset
// shows none).
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const DEFAULTS = {
  apiUrl: "https://api.opendesk.sh",
  appUrl: "https://app.opendesk.sh",
};

function url(name, value) {
  let u;
  try {
    u = new URL(value);
  } catch {
    throw new Error(`${name} is not a URL: ${value}`);
  }
  if (u.protocol !== "https:" && u.protocol !== "http:") throw new Error(`${name} must use https: or http:, got ${u.protocol}`);
  if (u.pathname !== "/" || u.search || u.hash) throw new Error(`${name} must be an origin without a path, got ${value}`);
  return u.origin;
}

// ADMIN_ENV_LABEL: up to 24 printable characters, letters, digits, spaces and - _ . ( ).
export function envLabel(value) {
  const v = (value ?? "").trim();
  if (!/^[\p{L}\p{N} ._()-]{1,24}$/u.test(v)) throw new Error(`ADMIN_ENV_LABEL must be 1 to 24 letters, digits, spaces or - _ . ( ), got ${value}`);
  return v;
}

export function buildConfig(env = process.env) {
  const cfg = {
    apiUrl: url("ADMIN_API_URL", env.ADMIN_API_URL || DEFAULTS.apiUrl),
    appUrl: url("ADMIN_APP_URL", env.ADMIN_APP_URL || DEFAULTS.appUrl),
  };
  // omitempty, as in the Go struct.
  if (env.TURNSTILE_SITE_KEY) cfg.turnstileSiteKey = env.TURNSTILE_SITE_KEY.trim();
  if (env.ADMIN_ENV_LABEL !== undefined && env.ADMIN_ENV_LABEL.trim() !== "") cfg.envLabel = envLabel(env.ADMIN_ENV_LABEL);
  return cfg;
}

export function render(cfg) {
  return `window.__OPENDESK_ADMIN__=${JSON.stringify(cfg)};`;
}

export function writeConfig(outdir, env = process.env) {
  if (!outdir) throw new Error("usage: write-config.mjs <outdir>");
  const cfg = buildConfig(env);
  mkdirSync(outdir, { recursive: true });
  const file = join(outdir, "config.js");
  writeFileSync(file, render(cfg) + "\n");
  return [file];
}

if (import.meta.url === pathToFileURL(process.argv[1] ?? "").href) {
  try {
    for (const file of writeConfig(process.argv[2])) console.log(`wrote ${file}`);
  } catch (err) {
    console.error(err.message);
    process.exit(1);
  }
}
