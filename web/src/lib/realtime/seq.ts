// Contiguous-seq bookkeeping for one workspace's event stream. Events arrive in
// order with seq = last + 1; anything else means we missed some, so we fetch
// them from GET /v1/events?since=<last> and apply those first. A `resync`
// answer means the gap is older than retention: drop the caches instead.

import type { Envelope, EventsPage } from "../api/types";

export type SeqSyncDeps = {
  fetchSince: (since: number) => Promise<EventsPage>;
  apply: (env: Envelope) => void;
  resync: () => void;
};

const SEEN_CAP = 1000;

export class SeqSync {
  last: number | null = null;
  private catching: Promise<void> | null = null;
  private buffer: Envelope[] = [];
  private seen = new Set<string>();
  private seenOrder: string[] = [];

  constructor(private deps: SeqSyncDeps) {}

  /** Has this envelope (by id) been applied? Used to dedupe the user channel. */
  hasSeen(id: string) {
    return this.seen.has(id);
  }

  markSeen(id: string) {
    if (this.seen.has(id)) return;
    this.seen.add(id);
    this.seenOrder.push(id);
    if (this.seenOrder.length > SEEN_CAP) this.seen.delete(this.seenOrder.shift()!);
  }

  /** Start from a known position (the latest seq when we first connect). */
  baseline(seq: number) {
    this.last = seq;
  }

  /** Feed one envelope from the workspace channel. */
  receive(env: Envelope): void {
    if (this.catching) {
      this.buffer.push(env);
      return;
    }
    if (this.last === null) {
      this.applyOne(env);
      return;
    }
    if (env.seq <= this.last) return; // duplicate or already fetched
    if (env.seq === this.last + 1) {
      this.applyOne(env);
      return;
    }
    this.buffer.push(env);
    void this.catchUp();
  }

  /** Fetch everything after `last` (on reconnect, or on a gap). */
  catchUp(): Promise<void> {
    if (this.catching) return this.catching;
    if (this.last === null) return Promise.resolve();
    this.catching = (async () => {
      try {
        for (let guard = 0; guard < 20; guard++) {
          const page = await this.deps.fetchSince(this.last!);
          if (page.resync) {
            this.deps.resync();
            this.last = page.latest_seq;
            break;
          }
          const fresh = page.data.filter((e) => e.seq > this.last!).sort((a, b) => a.seq - b.seq);
          for (const e of fresh) this.applyOne(e);
          if (!fresh.length || this.last! >= page.latest_seq) {
            this.last = Math.max(this.last!, page.latest_seq);
            break;
          }
        }
      } catch {
        // Can't reach the events API: refetch rather than show stale data.
        this.deps.resync();
      } finally {
        this.catching = null;
      }
      const pending = this.buffer.sort((a, b) => a.seq - b.seq);
      this.buffer = [];
      for (const e of pending) this.receive(e);
    })();
    return this.catching;
  }

  private applyOne(env: Envelope) {
    this.last = this.last === null ? env.seq : Math.max(this.last, env.seq);
    // A private team's stand-in only moves seq on; the real envelope (same id) may still
    // come on the team channel, before or after it, and must not look already applied.
    if (env.type === "event.hidden") return;
    if (this.seen.has(env.id)) return;
    this.markSeen(env.id);
    this.deps.apply(env);
  }
}
