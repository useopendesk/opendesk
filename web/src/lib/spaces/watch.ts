// Keeps the numbers of other workspaces in the open Space live (contracts/spaces.md "Rows"):
// for each such workspace, at most MAX_WATCHED of them, a light socket with a `ws` token for
// that workspace joins `workspace:<wid>`, and any event refreshes that workspace's numbers a
// moment later. The rest, and any whose socket is down, refetch every minute and on focus.

import type { QueryClient } from "@tanstack/react-query";
import { api } from "../api/endpoints";
import { config } from "../config";
import { qk } from "../queries";
import { createSocket, type RtSocket } from "../realtime/transport";

export const MAX_WATCHED = 8;
/** How long after an event the workspace's numbers are read again (events come in bursts). */
export const REFRESH_DELAY_MS = 1200;
/** Without a live socket, other workspaces' numbers refresh this often. */
export const POLL_MS = 60_000;
const TOKEN_REFRESH_MS = 4 * 60_000;

/** Workspaces whose socket is joined right now. */
const live = new Set<string>();

/** How often a workspace's numbers refetch on their own: never while its socket is live. */
export function pollInterval(wid: string): number | false {
  return live.has(wid) ? false : POLL_MS;
}

type Entry = { socket: RtSocket | null; token: string; timer: ReturnType<typeof setTimeout> | null; refresh: ReturnType<typeof setInterval> | null; stopped: boolean };

/** The workspaces to keep a socket for: the first MAX_WATCHED, the open one never. */
export function watchList(wids: string[], open: string | null): string[] {
  const out: string[] = [];
  for (const w of wids) if (w !== open && !out.includes(w)) out.push(w);
  return out.slice(0, MAX_WATCHED);
}

export class SpaceWatcher {
  private entries = new Map<string, Entry>();

  constructor(private qc: QueryClient) {}

  /** Opens sockets for these workspaces and closes the others. */
  sync(wids: string[]) {
    const want = new Set(wids.slice(0, MAX_WATCHED));
    for (const [wid, e] of this.entries) if (!want.has(wid)) this.close(wid, e);
    for (const wid of want) if (!this.entries.has(wid)) void this.open(wid);
  }

  stop() {
    for (const [wid, e] of this.entries) this.close(wid, e);
  }

  private refreshSoon(wid: string, e: Entry) {
    if (e.timer) return;
    e.timer = setTimeout(() => {
      e.timer = null;
      if (!e.stopped) void this.qc.invalidateQueries({ queryKey: qk.ws(wid) });
    }, REFRESH_DELAY_MS);
  }

  private async token(wid: string, e: Entry): Promise<{ token: string; url: string } | null> {
    try {
      const r = await api.inWorkspace(wid).realtimeToken();
      e.token = r.token;
      return r;
    } catch {
      return null;
    }
  }

  private async open(wid: string) {
    const e: Entry = { socket: null, token: "", timer: null, refresh: null, stopped: false };
    this.entries.set(wid, e);
    const r = await this.token(wid, e);
    // Quietly left to polling when the workspace gives no token (suspended, left, offline).
    if (!r || e.stopped) return;
    let url = config.gatewayUrl;
    if (r.url && !window.__OPENDESK__?.gatewayUrl && !import.meta.env.VITE_GATEWAY_URL) url = r.url;
    let socket: RtSocket;
    try {
      socket = createSocket(url, () => ({ token: e.token }));
    } catch {
      return;
    }
    e.socket = socket;
    const down = () => {
      if (e.stopped) return;
      live.delete(wid);
      // Tokens only need to be valid at connect: a fresh one for the reconnect.
      void this.token(wid, e);
    };
    socket.onClose(down);
    socket.onError(down);
    socket.connect();
    e.refresh = setInterval(() => void this.token(wid, e), TOKEN_REFRESH_MS);
    const ch = socket.channel(`workspace:${wid}`);
    ch.on("event", () => this.refreshSoon(wid, e));
    let joined = false;
    ch.join().receive("ok", () => {
      if (e.stopped) return;
      live.add(wid);
      // Anything published while it joined, or while it was down: read once.
      if (joined) this.refreshSoon(wid, e);
      joined = true;
    });
  }

  private close(wid: string, e: Entry) {
    e.stopped = true;
    if (e.timer) clearTimeout(e.timer);
    if (e.refresh) clearInterval(e.refresh);
    try {
      e.socket?.disconnect();
    } catch {
      /* already closed */
    }
    live.delete(wid);
    this.entries.delete(wid);
  }
}
