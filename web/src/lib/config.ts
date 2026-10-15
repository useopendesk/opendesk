// Runtime configuration: `window.__OPENDESK__` from /config.js (served by the
// self-hosted binary or generated at deploy), falling back to VITE_* build env.
// One build therefore serves any deployment.

export type RuntimeConfig = {
  apiUrl: string;
  gatewayUrl: string;
  posthogKey: string;
  posthogHost: string;
  turnstileSiteKey: string;
  selfHosted: boolean;
  /**
   * The sign-ins the app may offer for a new mailbox, or null when the deployment
   * does not restrict them. The API's connect options must allow a provider too.
   */
  mailboxOAuth: MailboxOAuthProvider[] | null;
};

export type MailboxOAuthProvider = "google" | "microsoft";

declare global {
  interface Window {
    __OPENDESK__?: Partial<Record<keyof RuntimeConfig, string | boolean | string[]>>;
  }
}

function pick(key: keyof RuntimeConfig, env: string | undefined): string {
  const v = typeof window !== "undefined" ? window.__OPENDESK__?.[key] : undefined;
  if (typeof v === "string" && v !== "" && !v.startsWith("${")) return v;
  return env ?? "";
}

/**
 * mailboxOAuth: a list from config.js, else VITE_MAILBOX_OAUTH ("google,microsoft",
 * "none" for an empty list), else null.
 */
export function parseMailboxOAuth(raw: unknown, env: string | undefined): MailboxOAuthProvider[] | null {
  const valid = (v: unknown): v is MailboxOAuthProvider => v === "google" || v === "microsoft";
  if (Array.isArray(raw)) return raw.filter(valid);
  const src = typeof raw === "string" && raw !== "" && !raw.startsWith("${") ? raw : env;
  if (src === undefined || src.trim() === "") return null;
  if (src.trim() === "none") return [];
  return src.split(",").map((s) => s.trim()).filter(valid);
}

function trimSlash(s: string) {
  return s.replace(/\/+$/, "");
}

function defaultGateway(): string {
  if (typeof location === "undefined") return "";
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${location.host}/socket`;
}

export function readConfig(): RuntimeConfig {
  const env = import.meta.env;
  const selfHostedRaw = typeof window !== "undefined" ? window.__OPENDESK__?.selfHosted : undefined;
  return {
    apiUrl: trimSlash(pick("apiUrl", env.VITE_API_URL)),
    gatewayUrl: trimSlash(pick("gatewayUrl", env.VITE_GATEWAY_URL)) || defaultGateway(),
    posthogKey: pick("posthogKey", env.VITE_POSTHOG_KEY),
    posthogHost: pick("posthogHost", env.VITE_POSTHOG_HOST) || "https://us.i.posthog.com",
    turnstileSiteKey: pick("turnstileSiteKey", env.VITE_TURNSTILE_SITE_KEY),
    selfHosted: selfHostedRaw === true || selfHostedRaw === "true" || env.VITE_SELF_HOSTED === "1",
    mailboxOAuth: parseMailboxOAuth(typeof window !== "undefined" ? window.__OPENDESK__?.mailboxOAuth : undefined, env.VITE_MAILBOX_OAUTH),
  };
}

export const config: RuntimeConfig = readConfig();

const HOSTED_APP = "https://app.opendesk.sh";

/**
 * Where this instance serves its app, help centers and chat widget: the hosted app on
 * opendesk.sh, otherwise the origin the app runs on (a self-hosted domain, or a local
 * address while developing).
 */
export function appOrigin(): string {
  if (typeof window === "undefined") return HOSTED_APP;
  if (!config.selfHosted && window.location.hostname.endsWith("opendesk.sh")) return HOSTED_APP;
  return window.location.origin;
}

// Mock mode is checked inline as `import.meta.env.VITE_MOCK === "1"` at each use,
// so the bundler can drop the mock branches from production builds.
