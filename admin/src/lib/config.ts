// Runtime configuration: `window.__OPENDESK_ADMIN__` from <base>config.js (served
// by the self-hosted binary at /admin/config.js, or written at deploy time on
// Cloudflare Pages), falling back to the VITE_* build env for development. One
// build therefore serves any deployment.

export type AdminConfig = {
  /** The API origin, without a trailing slash. */
  apiUrl: string;
  /** Cloudflare Turnstile site key; "" turns the widget off. */
  turnstileSiteKey: string;
  /** Where the instance's app lives (security settings links). */
  appUrl: string;
  /** A short label for this instance ("Production", "Staging"); "" shows none. */
  envLabel: string;
  /** The base path the panel is served under: "/" or "/admin/". */
  base: string;
};

type RawConfig = Partial<Record<"apiUrl" | "turnstileSiteKey" | "appUrl" | "envLabel", unknown>>;

declare global {
  interface Window {
    __OPENDESK_ADMIN__?: RawConfig;
  }
}

type Env = { VITE_API_URL?: string; VITE_TURNSTILE_SITE_KEY?: string; VITE_APP_URL?: string; VITE_ENV_LABEL?: string; BASE_URL?: string };

const DEV_API = "http://localhost:8080";
const DEV_APP = "http://localhost:5173";

/** A usable string value: set, not empty and not an unreplaced "${...}" template. */
function value(v: unknown): string {
  return typeof v === "string" && v.trim() !== "" && !v.startsWith("${") ? v.trim() : "";
}

function trimSlash(s: string) {
  return s.replace(/\/+$/, "");
}

/** "/" or "/admin/": always one leading and one trailing slash. */
export function normalizeBase(raw: string | undefined): string {
  const parts = (raw ?? "/").split("/").filter(Boolean);
  return parts.length ? `/${parts.join("/")}/` : "/";
}

/** The router's basepath: the base without its trailing slash ("/" stays "/"). */
export function routerBasepath(base: string): string {
  return base === "/" ? "/" : base.replace(/\/$/, "");
}

/** A URL under the panel's base: assetUrl("config.js") -> "/admin/config.js". */
export function assetUrl(path: string, base = config.base): string {
  return base + path.replace(/^\/+/, "");
}

/** Only http(s) URLs are taken; anything else is treated as unset. */
function httpUrl(v: string): string {
  if (!v) return "";
  try {
    const u = new URL(v);
    return u.protocol === "https:" || u.protocol === "http:" ? trimSlash(v) : "";
  } catch {
    return "";
  }
}

export function readConfig(raw: RawConfig | undefined, env: Env, origin?: string): AdminConfig {
  const base = normalizeBase(env.BASE_URL);
  // Embedded in the binary (base "/admin/") the API and the app share the panel's origin.
  const sameOrigin = base !== "/" && origin ? origin : "";
  const apiUrl = httpUrl(value(raw?.apiUrl)) || httpUrl(value(env.VITE_API_URL)) || sameOrigin || DEV_API;
  const appUrl = httpUrl(value(raw?.appUrl)) || httpUrl(value(env.VITE_APP_URL)) || sameOrigin || DEV_APP;
  return {
    apiUrl,
    turnstileSiteKey: value(raw?.turnstileSiteKey) || value(env.VITE_TURNSTILE_SITE_KEY),
    appUrl,
    envLabel: (value(raw?.envLabel) || value(env.VITE_ENV_LABEL)).slice(0, 24),
    base,
  };
}

export const config: AdminConfig = readConfig(
  typeof window !== "undefined" ? window.__OPENDESK_ADMIN__ : undefined,
  import.meta.env,
  typeof location !== "undefined" ? location.origin : undefined,
);
