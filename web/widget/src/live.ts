// The agent's replies as they stream in: `"live"` pushes on the contact topic
// (contracts/realtime.md "Live events"). Live events are only a preview: the
// posted message arrives as `message.created` and is the source of truth.
// Events are never replayed and may be missed (a reconnect, a late join), so
// every step here tolerates gaps and arrival in any order.

import { readBotRef, type BotSeen } from "./config";
import type { LiveEvent, Message } from "./types";

export type LiveTool = { name: string; status: "running" | "done" | "error" };

export type LiveReply = {
  id: string;
  ticket_id: string;
  /** The bot writing it (from `started`); null shows the widget's. */
  bot: BotSeen | null;
  /** Raw markdown so far (markers included; the view hides them). */
  text: string;
  tools: LiveTool[];
  /**
   * typing: started, nothing written yet; streaming: text is arriving;
   * done: ended with a message id, waiting for its `message.created`;
   * leaving: ended with nothing posted, fading out.
   */
  phase: "typing" | "streaming" | "done" | "leaving";
  /** Joined mid-reply (no `started` seen): text is incomplete, so only the typing state shows. */
  partial: boolean;
  message_id: string | null;
  handoff: boolean;
  action_id: string | null;
  /** Last event time (ms), for dropping replies whose end was missed. */
  at: number;
};

export type LiveState = {
  replies: Record<string, LiveReply>;
  /** Reply ids already finished, so a late or repeated event can't bring one back. Newest last. */
  closed: string[];
};

const CLOSED_MAX = 50;

const close = (st: LiveState, ids: string[]): LiveState => {
  if (!ids.length) return st;
  const replies = { ...st.replies };
  for (const id of ids) delete replies[id];
  return { replies, closed: [...st.closed.filter((x) => !ids.includes(x)), ...ids].slice(-CLOSED_MAX) };
};

/**
 * Apply one live event. `posted(id)` tells whether a message is already in the
 * thread (its `message.created` came first), in which case the preview just goes.
 */
export function reduceLive(st: LiveState, ev: LiveEvent, now: number, posted: (messageId: string) => boolean): LiveState {
  const d = ev.data as { reply_id?: unknown; ticket_id?: unknown };
  if (typeof d?.reply_id !== "string" || typeof d.ticket_id !== "string") return st;
  const id = d.reply_id;
  if (st.closed.includes(id)) return st;
  const cur = st.replies[id];
  const fresh = (): LiveReply => ({
    id,
    ticket_id: d.ticket_id as string,
    bot: null,
    text: "",
    tools: [],
    phase: "typing",
    partial: true,
    message_id: null,
    handoff: false,
    action_id: null,
    at: now,
  });
  const put = (r: LiveReply): LiveState => ({ ...st, replies: { ...st.replies, [id]: r } });
  // Once ended, a reply only waits for its message: later events change nothing.
  if (cur && (cur.phase === "done" || cur.phase === "leaving") && ev.type !== "ai.reply.ended") return st;

  switch (ev.type) {
    case "ai.reply.started": {
      const bot = readBotRef(ev.data.bot);
      if (cur) return put({ ...cur, bot: bot ?? cur.bot, partial: cur.text === "" ? false : cur.partial, at: now });
      return put({ ...fresh(), bot, partial: false });
    }
    case "ai.reply.delta": {
      if (typeof ev.data.text !== "string" || !ev.data.text) return st;
      const r = cur ?? fresh();
      return put({ ...r, text: r.text + ev.data.text, phase: "streaming", at: now });
    }
    case "ai.reply.tool": {
      const { tool, status } = ev.data;
      if (typeof tool !== "string" || !tool) return st;
      const s: LiveTool["status"] = status === "done" || status === "error" ? status : "running";
      const r = cur ?? fresh();
      const tools = [...r.tools];
      // The latest call of that tool that is still running takes the update; otherwise it's a new call.
      let i = -1;
      for (let j = tools.length - 1; j >= 0; j--)
        if (tools[j].name === tool && tools[j].status === "running") {
          i = j;
          break;
        }
      if (i >= 0) tools[i] = { name: tool, status: s };
      else tools.push({ name: tool, status: s });
      return put({ ...r, tools, at: now });
    }
    case "ai.reply.ended": {
      const mid = typeof ev.data.message_id === "string" && ev.data.message_id ? ev.data.message_id : null;
      if (mid && posted(mid)) return close(st, [id]);
      const r = cur ?? fresh();
      const tools = r.tools.map((t) => (t.status === "running" ? { ...t, status: "done" as const } : t));
      return put({
        ...r,
        tools,
        phase: mid ? "done" : "leaving",
        message_id: mid,
        handoff: ev.data.handoff === true,
        action_id: typeof ev.data.action_id === "string" ? ev.data.action_id : null,
        at: now,
      });
    }
    default:
      return st;
  }
}

/**
 * A message arrived (`message.created`, or a fetch). Previews it replaces go:
 * the one whose `ended` named it, and, when the message came before its
 * `ended`, any agent reply still writing on that ticket. Returns the new state
 * and whether a preview was replaced (so the message shows without an entrance).
 */
export function liveMessage(st: LiveState, m: Pick<Message, "id" | "ticket_id" | "ai" | "direction">): { state: LiveState; replaced: boolean } {
  const ids: string[] = [];
  let replaced = false;
  for (const r of Object.values(st.replies)) {
    if (r.message_id === m.id) {
      ids.push(r.id);
      replaced = true;
    } else if (m.ai === true && m.direction === "outbound" && r.ticket_id === m.ticket_id && (r.phase === "typing" || r.phase === "streaming")) {
      ids.push(r.id);
      replaced = true;
    }
  }
  return { state: close(st, ids), replaced };
}

/** Remove one reply (after its fade out), or every reply idle for longer than `maxIdle` ms. */
export function dropLive(st: LiveState, which: { id: string } | { idleBefore: number; doneBefore: number }): LiveState {
  if ("id" in which) return st.replies[which.id] ? close(st, [which.id]) : st;
  const ids = Object.values(st.replies)
    .filter((r) => (r.phase === "done" || r.phase === "leaving" ? r.at < which.doneBefore : r.at < which.idleBefore))
    .map((r) => r.id);
  return close(st, ids);
}

/** Replies to show in a ticket, oldest first. */
export function repliesFor(st: LiveState, ticketId: string): LiveReply[] {
  // Insertion order: reply ids are uuids, never integer-like keys.
  return Object.values(st.replies).filter((r) => r.ticket_id === ticketId);
}
