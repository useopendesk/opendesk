// Sign-in with a provider in a popup window (contracts/api.md, OAuth start `popup`).
//
// The tab opens the provider in a popup and waits. The API sends the popup back to
// /auth/popup with a one-time code and the popup's id, and that page hands the result to
// the waiting tab: through window.opener when the link survived, and through a
// same-origin BroadcastChannel when it did not (the provider's pages cut it, or the
// browser blocked the popup and the person opened it from the address bar later). The
// tab acknowledges on the channel; without an answer the popup finishes the sign-in
// itself, so it always completes somewhere.

const CHANNEL = "opendesk-oauth-popup";
const ACK_WAIT_MS = 1500;

export type PopupResult = { type: "opendesk-oauth-popup"; popup: string; code?: string; error?: string };
type Ack = { type: "opendesk-oauth-popup-ack"; popup: string };

/** A fresh id for one popup sign-in, 22 url-safe characters. */
export function newPopupId(): string {
  const b = crypto.getRandomValues(new Uint8Array(16));
  return btoa(String.fromCharCode(...b)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

/** Opens the provider's sign-in centred over this window; null when the browser blocked it. */
export function openSignInPopup(url: string): Window | null {
  const w = 480;
  const h = 640;
  const left = Math.max(0, Math.round(window.screenX + (window.outerWidth - w) / 2));
  const top = Math.max(0, Math.round(window.screenY + (window.outerHeight - h) / 2.5));
  try {
    const win = window.open(url, "opendesk-sign-in", `popup=yes,width=${w},height=${h},left=${left},top=${top}`);
    win?.focus();
    return win;
  } catch {
    return null;
  }
}

function isResult(d: unknown, popup: string): d is PopupResult {
  const r = d as PopupResult | null;
  return !!r && r.type === "opendesk-oauth-popup" && r.popup === popup && (typeof r.code === "string" || typeof r.error === "string");
}

/**
 * Waits for the popup with this id to report back, by message or on the channel, and
 * calls `onResult` once. Returns a function that stops waiting.
 */
export function waitForPopup(popup: string, onResult: (r: PopupResult) => void): () => void {
  let done = false;
  const ch = typeof BroadcastChannel === "undefined" ? null : new BroadcastChannel(CHANNEL);
  const take = (d: unknown) => {
    if (done || !isResult(d, popup)) return;
    done = true;
    ch?.postMessage({ type: "opendesk-oauth-popup-ack", popup } satisfies Ack);
    stop();
    onResult(d);
  };
  const onMessage = (e: MessageEvent) => {
    if (e.origin === window.location.origin) take(e.data);
  };
  if (ch) ch.onmessage = (e: MessageEvent) => take(e.data);
  window.addEventListener("message", onMessage);
  function stop() {
    window.removeEventListener("message", onMessage);
    // Closed a moment later, so the acknowledgement leaves first.
    if (ch) window.setTimeout(() => ch.close(), 50);
  }
  return () => {
    done = true;
    stop();
  };
}

/**
 * From the popup: hands the result to the tab that opened the sign-in. Resolves true when
 * that tab acknowledged it, false when no tab did within a moment.
 */
export function deliverPopupResult(result: PopupResult): Promise<boolean> {
  return new Promise((resolve) => {
    const ch = typeof BroadcastChannel === "undefined" ? null : new BroadcastChannel(CHANNEL);
    let settled = false;
    const finish = (ok: boolean) => {
      if (settled) return;
      settled = true;
      window.clearTimeout(timer);
      ch?.close();
      resolve(ok);
    };
    const timer = window.setTimeout(() => finish(false), ACK_WAIT_MS);
    if (ch) {
      ch.onmessage = (e: MessageEvent) => {
        const a = e.data as Ack | null;
        if (a?.type === "opendesk-oauth-popup-ack" && a.popup === result.popup) finish(true);
      };
      ch.postMessage(result);
    }
    try {
      window.opener?.postMessage(result, window.location.origin);
    } catch {
      /* the opener is gone or another origin */
    }
  });
}
