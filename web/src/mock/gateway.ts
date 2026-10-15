// A fake Phoenix gateway: sockets, channels, presence and event fan-out, all
// in memory. Mutations in the mock API publish envelopes here exactly as the
// Go relay and Elixir gateway would.

import type { Envelope } from "@/lib/api/types";
import type { PushReply, RtChannel, RtSocket, SocketFactory } from "@/lib/realtime/transport";
import { db, type WsData } from "./db";
import { privateTeamOf } from "./teams";
import { uuid7 } from "./util";

type Handler = { ref: number; event: string; cb: (payload: unknown) => void };

export type Meta = {
  user_id: string;
  name: string;
  avatar: string;
  state: "viewing" | "replying" | "idle";
  online_at: string;
  phx_ref: string;
};

const channels = new Set<FakeChannel>();
const EVENT_LOG_CAP = 500;

/** Teammates "present" on tickets, keyed by ticket id. */
export const teammatePresence = new Map<string, Meta[]>();

let refCounter = 1;
const nextRef = () => refCounter++;

/** A push/join reply that always succeeds shortly after. */
function reply(): PushReply {
  const r: PushReply = {
    receive(status, cb) {
      if (status === "ok") setTimeout(() => cb({ server_time: new Date().toISOString() }), 30);
      return r;
    },
  };
  return r;
}

class FakeChannel implements RtChannel {
  handlers: Handler[] = [];
  joined = false;
  selfMeta: Meta | null = null;

  constructor(
    readonly socket: FakeSocket,
    readonly topic: string,
    readonly params: Record<string, unknown>,
  ) {}

  get isWatch() {
    return this.params.watch === true;
  }

  ticketId(): string | null {
    const m = /^ticket:[^:]+:(.+)$/.exec(this.topic);
    return m ? m[1] : null;
  }

  join(): PushReply {
    this.joined = true;
    channels.add(this);
    const tid = this.ticketId();
    if (tid) {
      setTimeout(() => {
        if (!this.isWatch) {
          const u = db.users.find((x) => x.id === db.meId);
          if (u) {
            this.selfMeta = {
              user_id: u.id,
              name: u.name,
              avatar: u.avatar_url ?? "",
              state: "viewing",
              online_at: new Date().toISOString(),
              phx_ref: `ref${nextRef()}`,
            };
          }
        }
        this.emit("presence_state", presenceStateFor(tid));
        if (this.selfMeta) broadcastDiff(tid, { [this.selfMeta.user_id]: { metas: [this.selfMeta] } }, {}, this);
      }, 40);
    }
    return reply();
  }

  leave(): PushReply {
    this.joined = false;
    channels.delete(this);
    const tid = this.ticketId();
    if (tid && this.selfMeta) {
      const m = this.selfMeta;
      this.selfMeta = null;
      broadcastDiff(tid, {}, { [m.user_id]: { metas: [m] } }, this);
    }
    return reply();
  }

  on(event: string, cb: (payload: unknown) => void): number {
    const ref = nextRef();
    this.handlers.push({ ref, event, cb });
    return ref;
  }

  off(event: string, ref?: number): void {
    this.handlers = this.handlers.filter((h) => !(h.event === event && (ref === undefined || h.ref === ref)));
  }

  push(event: string, payload: object): PushReply {
    const tid = this.ticketId();
    const p = payload as Record<string, unknown>;
    if (tid && event === "presence" && this.selfMeta) {
      const state = p.state === "replying" || p.state === "idle" ? p.state : "viewing";
      const old = this.selfMeta;
      this.selfMeta = { ...old, state, phx_ref: `ref${nextRef()}` };
      broadcastDiff(tid, { [old.user_id]: { metas: [this.selfMeta] } }, { [old.user_id]: { metas: [old] } }, null);
    }
    // "typing" from the only real agent has nobody else to reach in mock mode.
    return reply();
  }

  onError(): void {}
  onClose(): void {}

  emit(event: string, payload: unknown) {
    if (!this.joined || !this.socket.connected) return;
    for (const h of this.handlers) if (h.event === event) h.cb(payload);
  }
}

class FakeSocket implements RtSocket {
  connected = false;
  private openCbs: (() => void)[] = [];
  private closeCbs: ((e?: unknown) => void)[] = [];

  constructor(
    readonly url: string,
    readonly params: () => Record<string, string>,
  ) {}

  connect(): void {
    setTimeout(() => {
      this.connected = true;
      this.openCbs.forEach((c) => c());
    }, 120);
  }

  disconnect(cb?: () => void): void {
    this.connected = false;
    for (const ch of [...channels]) if (ch.socket === this) channels.delete(ch);
    this.closeCbs.forEach((c) => c());
    cb?.();
  }

  onOpen(cb: () => void) {
    this.openCbs.push(cb);
  }
  onClose(cb: (e?: unknown) => void) {
    this.closeCbs.push(cb);
  }
  onError(): void {}

  channel(topic: string, params?: object | (() => object)): RtChannel {
    const p = typeof params === "function" ? params() : (params ?? {});
    return new FakeChannel(this, topic, p as Record<string, unknown>);
  }

  isConnected() {
    return this.connected;
  }

  /** Debug: simulate a dropped connection and reconnect. */
  bounce() {
    this.connected = false;
    this.closeCbs.forEach((c) => c());
    setTimeout(() => {
      this.connected = true;
      this.openCbs.forEach((c) => c());
      for (const ch of channels) if (ch.socket === this) ch.join();
    }, 1500);
  }
}

export const sockets: FakeSocket[] = [];

export const socketFactory: SocketFactory = (url, params) => {
  const s = new FakeSocket(url, params);
  sockets.push(s);
  return s;
};

type PState = Record<string, { metas: Meta[] }>;

function presenceStateFor(tid: string): PState {
  const out: PState = {};
  for (const m of teammatePresence.get(tid) ?? []) out[m.user_id] = { metas: [m] };
  for (const ch of channels) {
    if (ch.ticketId() === tid && ch.selfMeta) out[ch.selfMeta.user_id] = { metas: [ch.selfMeta] };
  }
  return out;
}

function broadcastDiff(tid: string, joins: PState, leaves: PState, except: FakeChannel | null) {
  for (const ch of channels) {
    if (ch === except || ch.ticketId() !== tid) continue;
    ch.emit("presence_diff", { joins, leaves });
  }
}

/** Change a teammate's presence on a ticket and tell any joined channel. */
export function setTeammatePresence(tid: string, meta: Omit<Meta, "phx_ref" | "online_at"> | null, userId?: string) {
  const list = teammatePresence.get(tid) ?? [];
  const uid = meta?.user_id ?? userId;
  const old = list.find((m) => m.user_id === uid);
  const rest = list.filter((m) => m.user_id !== uid);
  const leaves: PState = old ? { [old.user_id]: { metas: [old] } } : {};
  let joins: PState = {};
  if (meta) {
    const full: Meta = { ...meta, online_at: new Date().toISOString(), phx_ref: `ref${nextRef()}` };
    rest.push(full);
    joins = { [full.user_id]: { metas: [full] } };
  }
  teammatePresence.set(tid, rest);
  broadcastDiff(tid, joins, leaves, null);
}

/** Emit a non-event message on a ticket topic (typing, customer_typing). */
export function emitOnTicket(tid: string, event: string, payload: unknown) {
  for (const ch of channels) if (ch.ticketId() === tid) ch.emit(event, payload);
}

export function activeTicketIds(): string[] {
  const ids: string[] = [];
  for (const ch of channels) {
    const t = ch.ticketId();
    if (t && !ch.isWatch) ids.push(t);
  }
  return ids;
}

/** The private team of each logged envelope that has one (contracts/teams.md "Realtime"). */
export const envelopeTeam = new WeakMap<Envelope, string>();

/** The ticket an event is about, from its payload. */
function ticketIdOf(data: Record<string, unknown>): string | null {
  const pick = (v: unknown, k: string) => (v && typeof v === "object" && typeof (v as Record<string, unknown>)[k] === "string" ? ((v as Record<string, unknown>)[k] as string) : null);
  return (typeof data.ticket_id === "string" ? data.ticket_id : null) ?? pick(data.ticket, "id") ?? pick(data.message, "ticket_id") ?? pick(data.activity, "ticket_id");
}

/** What `workspace:<wid>` gets for an event of a private team: same id and seq, no data. */
export function standIn(env: Envelope): Envelope {
  return { id: env.id, seq: env.seq, workspace_id: env.workspace_id, type: "event.hidden", visibility: "agents", topics: [`workspace:${env.workspace_id}`], data: {}, occurred_at: env.occurred_at };
}

/**
 * Write an envelope to the workspace's event log and fan it out. Events about a
 * ticket in a private team go in full to `team:<wid>:<team>` and as a stand-in
 * to `workspace:<wid>`, so seq stays contiguous for everyone; `user:` topics
 * are unaffected. `ticket.hidden` always goes to the workspace as is.
 */
export function publish(
  ws: WsData,
  type: string,
  data: Record<string, unknown>,
  opts: { topics?: string[]; visibility?: "agents" | "public"; teamId?: string | null } = {},
): Envelope {
  ws.seq += 1;
  const wid = ws.workspace.id;
  const team = type === "ticket.hidden" ? null : opts.teamId !== undefined ? opts.teamId : privateTeamOf(ws, ticketIdOf(data));
  const topics = [team ? `team:${wid}:${team}` : `workspace:${wid}`, ...(opts.topics ?? [])];
  const env: Envelope = {
    id: uuid7(),
    seq: ws.seq,
    workspace_id: wid,
    type,
    visibility: opts.visibility ?? "agents",
    topics,
    data: structuredClone(data),
    occurred_at: new Date().toISOString(),
  };
  if (team) envelopeTeam.set(env, team);
  ws.events.push(env);
  if (ws.events.length > EVENT_LOG_CAP) ws.events.splice(0, ws.events.length - EVENT_LOG_CAP);
  if (!paused.drop) {
    setTimeout(() => {
      for (const ch of channels) {
        if (ch.topic === `workspace:${wid}`) ch.emit("event", team ? standIn(env) : env);
        else if (team && ch.topic === `team:${wid}:${team}`) ch.emit("event", env);
        else if (ch.topic.startsWith("user:") && topics.includes(ch.topic)) ch.emit("event", env);
      }
    }, 25);
  }
  return env;
}

/** Debug: drop deliveries (events still logged) to exercise gap recovery. */
export const paused = { drop: false };
