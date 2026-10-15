// The desktop app (contracts/native.md, "Desktop shell"). The Electron shell
// exposes `window.opendeskDesktop`; everything here is a no-op in a browser.

export type DesktopPlatform = "darwin" | "win32" | "linux";

export type DesktopUpdateChannel = "stable" | "beta";

export type DesktopUpdateState =
  | { status: "idle" }
  | { status: "checking" }
  | { status: "available"; version: string }
  | { status: "downloading"; version: string; percent: number }
  | { status: "ready"; version: string; notes_url: string }
  | { status: "error" };

export interface DesktopUpdate {
  state(): DesktopUpdateState;
  onChange(cb: (s: DesktopUpdateState) => void): () => void;
  restart(): void;
  check(): void;
  channel(): DesktopUpdateChannel;
  setChannel(c: DesktopUpdateChannel): void;
}

export interface OpendeskDesktop {
  version: string;
  platform: DesktopPlatform;
  notify(n: { id: string; title: string; body?: string; path?: string }): void;
  setBadge(count: number): void;
  oauth(startUrl: string): Promise<{ code: string; verifier: string } | { error: string }>;
  openExternal(url: string): void;
  switchServer(): void;
  onNavigate(cb: (path: string) => void): () => void;
  /** Auto-update; absent in desktop builds older than the update banner. */
  update?: DesktopUpdate;
}

declare global {
  interface Window {
    opendeskDesktop?: OpendeskDesktop;
  }
}

function detect(): OpendeskDesktop | null {
  if (typeof window === "undefined") return null;
  const d = window.opendeskDesktop;
  if (!d || typeof d !== "object" || typeof d.notify !== "function") return null;
  return d;
}

/** The desktop bridge, or null in a browser. */
export const desktop: OpendeskDesktop | null = detect();
export const isDesktop = desktop !== null;

/**
 * Marks <html data-desktop="darwin|win32|linux"> so the layout can make room for
 * the window chrome. public/theme.js does the same before first paint.
 */
export function initDesktop() {
  if (!desktop) return;
  const p = desktop.platform;
  document.documentElement.dataset.desktop = p === "darwin" || p === "win32" ? p : "linux";
}

/** Opens an app path of this instance in the default browser (connect flows that need the browser's session). */
export function openInBrowser(path: string) {
  if (!desktop) return;
  desktop.openExternal(new URL(path, location.origin).toString());
}

// OAuth sign-in hands the verifier to /auth/callback in memory, never in the URL or storage.
let handoff: { code: string; verifier: string } | null = null;

export function setOAuthHandoff(h: { code: string; verifier: string }) {
  handoff = h;
}

/** The PKCE verifier for this code, once. */
export function takeVerifier(code: string): string | undefined {
  if (!handoff || handoff.code !== code) return undefined;
  const v = handoff.verifier;
  handoff = null;
  return v;
}
