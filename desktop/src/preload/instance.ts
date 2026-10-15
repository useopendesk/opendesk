// Preload for the instance window: exposes `window.opendeskDesktop` exactly as
// contracts/native.md types it. Every argument is checked again in main.

import { contextBridge, ipcRenderer } from "electron";

type Platform = "darwin" | "win32" | "linux";
type UpdateChannel = "stable" | "beta";
type UpdateState =
  | { status: "idle" }
  | { status: "checking" }
  | { status: "available"; version: string }
  | { status: "downloading"; version: string; percent: number }
  | { status: "ready"; version: string; notes_url: string }
  | { status: "error" };

const info = (ipcRenderer.sendSync("od:info") ?? {}) as { version?: string; platform?: Platform };

const listeners = new Set<(path: string) => void>();
let queued: string | null = null;

ipcRenderer.on("od:navigate", (_e, path: unknown) => {
  if (typeof path !== "string") return;
  if (!listeners.size) {
    queued = path;
    return;
  }
  for (const cb of listeners) cb(path);
});

// Update state: read once now, then pushed by main on every change.
const updateInfo = (ipcRenderer.sendSync("od:update-info") ?? {}) as { state?: UpdateState; channel?: UpdateChannel };
let updateState: UpdateState = updateInfo.state ?? { status: "idle" };
let updateChannel: UpdateChannel = updateInfo.channel === "beta" ? "beta" : "stable";
const updateListeners = new Set<(s: UpdateState) => void>();

ipcRenderer.on("od:update", (_e, msg: unknown) => {
  if (!msg || typeof msg !== "object") return;
  const m = msg as { state?: UpdateState; channel?: unknown };
  if (m.state && typeof m.state === "object" && typeof m.state.status === "string") updateState = m.state;
  if (m.channel === "stable" || m.channel === "beta") updateChannel = m.channel;
  for (const cb of updateListeners) cb(updateState);
});

const update = {
  state(): UpdateState {
    return updateState;
  },
  onChange(cb: (s: UpdateState) => void): () => void {
    if (typeof cb !== "function") return () => {};
    updateListeners.add(cb);
    return () => {
      updateListeners.delete(cb);
    };
  },
  restart() {
    ipcRenderer.send("od:update-restart");
  },
  check() {
    ipcRenderer.send("od:update-check");
  },
  channel(): UpdateChannel {
    return updateChannel;
  },
  setChannel(channel: UpdateChannel) {
    if (channel !== "stable" && channel !== "beta") return;
    ipcRenderer.send("od:update-channel", channel);
  },
};

const api = {
  version: String(info.version ?? ""),
  platform: (info.platform ?? "linux") as Platform,
  notify(n: { id: string; title: string; body?: string; path?: string }) {
    if (!n || typeof n !== "object") return;
    ipcRenderer.send("od:notify", { id: n.id, title: n.title, body: n.body, path: n.path });
  },
  setBadge(count: number) {
    ipcRenderer.send("od:badge", count);
  },
  oauth(startUrl: string): Promise<{ code: string; verifier: string } | { error: string }> {
    return ipcRenderer.invoke("od:oauth", startUrl);
  },
  openExternal(url: string) {
    ipcRenderer.send("od:open-external", url);
  },
  switchServer() {
    ipcRenderer.send("od:switch-server");
  },
  onNavigate(cb: (path: string) => void): () => void {
    if (typeof cb !== "function") return () => {};
    listeners.add(cb);
    if (queued) {
      const p = queued;
      queued = null;
      queueMicrotask(() => cb(p));
    }
    return () => {
      listeners.delete(cb);
    };
  },
  update: Object.freeze(update),
};

contextBridge.exposeInMainWorld("opendeskDesktop", Object.freeze(api));

// <html data-fullscreen> while the window is full screen, so the page can use the
// space the macOS traffic lights leave.
ipcRenderer.on("od:fullscreen", (_e, on: unknown) => {
  const el = document.documentElement;
  if (!el) return;
  if (on === true) el.dataset.fullscreen = "";
  else delete el.dataset.fullscreen;
});

// The native chrome follows the app's theme: the page records the preference
// on <html data-theme-pref>, and main sets the window appearance to match.
function sendTheme() {
  const pref = document.documentElement?.dataset.themePref;
  if (pref) ipcRenderer.send("od:theme", pref);
}
window.addEventListener("DOMContentLoaded", () => {
  sendTheme();
  new MutationObserver(sendTheme).observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme-pref"] });
});
