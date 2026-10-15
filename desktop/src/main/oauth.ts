// Native OAuth sign-in (contracts/native.md, "OAuth sign-in from a native
// client"): the provider page opens in the default browser with a PKCE
// challenge, the API sends the browser back to opendesk://auth/callback, and the
// page exchanges the one-time code with the verifier it gets from here.

import { createHash, randomBytes } from "node:crypto";
import { shell } from "electron";
import { OAUTH_TIMEOUT_MS } from "./env";

export type OAuthResult = { code: string; verifier: string } | { error: string };

type Pending = { verifier: string; resolve: (r: OAuthResult) => void; timer: NodeJS.Timeout };
let pending: Pending | null = null;

const START_PATH = /^\/v1\/auth\/oauth\/[a-z0-9_-]{1,40}\/start$/;
const CODE = /^[A-Za-z0-9._~-]{1,1024}$/;
const ERROR = /^[a-z0-9_]{1,64}$/;

function b64url(buf: Buffer) {
  return buf.toString("base64").replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

export function pkce() {
  // 64 base64url characters, inside the 43..128 the contract asks for.
  const verifier = b64url(randomBytes(48));
  const challenge = b64url(createHash("sha256").update(verifier).digest());
  return { verifier, challenge };
}

function settle(r: OAuthResult) {
  const p = pending;
  if (!p) return;
  pending = null;
  clearTimeout(p.timer);
  p.resolve(r);
}

/**
 * Starts a flow. Only `{apiOrigin}/v1/auth/oauth/{provider}/start` is accepted;
 * any `redirect_to` the page set is kept. A new flow cancels one in progress.
 */
export async function startOAuth(startUrl: unknown, apiOrigin: string): Promise<OAuthResult> {
  if (typeof startUrl !== "string" || startUrl.length > 4096) return { error: "invalid_request" };
  let u: URL;
  try {
    u = new URL(startUrl);
  } catch {
    return { error: "invalid_request" };
  }
  if (u.origin !== apiOrigin || !START_PATH.test(u.pathname) || u.hash) return { error: "invalid_request" };

  settle({ error: "cancelled" });
  const { verifier, challenge } = pkce();
  u.searchParams.set("client", "native");
  u.searchParams.set("code_challenge", challenge);

  const result = new Promise<OAuthResult>((resolve) => {
    pending = { verifier, resolve, timer: setTimeout(() => settle({ error: "timeout" }), OAUTH_TIMEOUT_MS) };
  });
  try {
    await shell.openExternal(u.toString(), { activate: true });
  } catch {
    settle({ error: "browser_unavailable" });
  }
  return result;
}

/** Handles opendesk://auth/callback?code=…|error=…. True when a flow was waiting. */
export function finishOAuth(url: URL): boolean {
  if (!pending) return false;
  const code = url.searchParams.get("code");
  const error = url.searchParams.get("error");
  if (code && CODE.test(code)) settle({ code, verifier: pending.verifier });
  else settle({ error: error && ERROR.test(error) ? error : "invalid_request" });
  return true;
}

/** Ends any flow in progress (switching server, quitting). */
export function cancelOAuth() {
  settle({ error: "cancelled" });
}
