// Addresses, origins and paths: turning what people type into an instance
// origin, checking it serves opendesk (`/config.json`, contracts/native.md), and
// the allowlists the window and IPC rely on.

import { net } from "electron";
import { isIP } from "node:net";
import { CLOUD_HOSTS, CLOUD_ORIGIN, isDev } from "./env";
import type { Instance } from "./store";

export type AddressError = "empty" | "invalid" | "insecure";
export type ResolveError = AddressError | "unreachable" | "timeout" | "not_opendesk" | "redirected" | "certificate";

/** localhost, loopback and private-network hosts: http is allowed for these in development builds. */
export function isLocalHost(host: string): boolean {
  const h = host.replace(/^\[|\]$/g, "").toLowerCase();
  if (h === "localhost" || h.endsWith(".localhost") || h.endsWith(".local")) return true;
  if (isIP(h) === 4) {
    const [a, b] = h.split(".").map(Number);
    return a === 127 || a === 10 || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168);
  }
  if (isIP(h) === 6) return h === "::1" || h.startsWith("fc") || h.startsWith("fd");
  return false;
}

/** http is acceptable for this URL: development builds, local hosts only. */
export function httpAllowed(u: URL): boolean {
  return isDev && u.protocol === "http:" && isLocalHost(u.hostname);
}

/** Normalizes a typed address to an origin. `opendesk.sh` means Cloud. */
export function normalizeAddress(input: unknown): { origin: string } | { error: AddressError } {
  if (typeof input !== "string") return { error: "invalid" };
  let s = input.trim();
  if (!s) return { error: "empty" };
  if (s.length > 2048 || /\s/.test(s)) return { error: "invalid" };
  if (!/^[a-z][a-z0-9+.-]*:\/\//i.test(s)) s = `https://${s}`;
  let u: URL;
  try {
    u = new URL(s);
  } catch {
    return { error: "invalid" };
  }
  if (u.username || u.password || !u.hostname) return { error: "invalid" };
  if (u.protocol !== "https:" && u.protocol !== "http:") return { error: "invalid" };
  if (!u.hostname.includes(".") && !isLocalHost(u.hostname)) return { error: "invalid" };
  if (CLOUD_HOSTS.has(u.hostname.toLowerCase())) return { origin: CLOUD_ORIGIN };
  if (u.protocol === "http:" && !httpAllowed(u)) return { error: "insecure" };
  return { origin: u.origin };
}

function acceptableUrl(v: unknown): URL | null {
  if (typeof v !== "string" || !v) return null;
  try {
    const u = new URL(v);
    if (u.protocol === "https:" || httpAllowed(u)) return u;
  } catch {
    /* fall through */
  }
  return null;
}

/**
 * Reads `<origin>/config.json` and returns the instance. The API origin comes
 * from `apiUrl`; an empty one means the API is served from the app origin
 * (self-hosted behind one domain).
 */
export async function resolveInstance(origin: string, opts: { lenient?: boolean } = {}): Promise<{ instance: Instance } | { error: ResolveError }> {
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(), 12_000);
  try {
    const res = await net.fetch(`${origin}/config.json`, {
      signal: ctrl.signal,
      headers: { accept: "application/json" },
      cache: "no-store",
      credentials: "omit",
    });
    if (new URL(res.url || `${origin}/config.json`).origin !== origin) return { error: "redirected" };
    if (!res.ok) return opts.lenient ? { instance: { appOrigin: origin, apiOrigin: origin } } : { error: "not_opendesk" };
    let body: unknown;
    try {
      body = JSON.parse(await res.text());
    } catch {
      body = null;
    }
    if (!body || typeof body !== "object" || Array.isArray(body)) {
      return opts.lenient ? { instance: { appOrigin: origin, apiOrigin: origin } } : { error: "not_opendesk" };
    }
    const cfg = body as Record<string, unknown>;
    if ("apiUrl" in cfg && typeof cfg.apiUrl !== "string") return { error: "not_opendesk" };
    const api = acceptableUrl(cfg.apiUrl);
    if (cfg.apiUrl && !api) return { error: "insecure" };
    return { instance: { appOrigin: origin, apiOrigin: api ? api.origin : origin } };
  } catch (e) {
    if (ctrl.signal.aborted) return { error: "timeout" };
    const msg = String((e as Error)?.message ?? "");
    if (/CERT|SSL|TLS/i.test(msg)) return { error: "certificate" };
    return { error: "unreachable" };
  } finally {
    clearTimeout(timer);
  }
}

/**
 * An app path the page may be sent to: same-origin, absolute, no scheme or
 * protocol-relative tricks. Returns the normalized path or null.
 */
export function appPath(v: unknown, appOrigin: string): string | null {
  if (typeof v !== "string" || v.length > 2048) return null;
  if (!v.startsWith("/") || v.startsWith("//") || v.includes("\\")) return null;
  // eslint-disable-next-line no-control-regex
  if (/[\u0000-\u001f\u007f]/.test(v)) return null;
  try {
    const u = new URL(v, appOrigin);
    if (u.origin !== appOrigin) return null;
    return u.pathname + u.search + u.hash;
  } catch {
    return null;
  }
}

/** A URL the default browser may open: https (and http to local hosts in development). */
export function externalUrl(v: unknown): string | null {
  if (typeof v !== "string" || v.length > 8192) return null;
  const u = acceptableUrl(v);
  if (!u || u.username || u.password) return null;
  return u.toString();
}
