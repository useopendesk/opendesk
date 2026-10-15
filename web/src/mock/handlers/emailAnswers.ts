// Mock Ollie answering email (contracts/agent.md "Email"): per-mailbox settings
// live on the mailbox (resources.ts), this file keeps the suggested replies of
// draft mode, the send and discard routes, and the email split of the agent
// report.

import type { AgentChannelReport, AgentReport, SuggestedReply, Ticket } from "@/lib/api/types";
import { db, me, userRef, type WsData } from "../db";
import { publish } from "../gateway";
import { reserve, spend } from "../aiBalance";
import { del, forbidden, get, HttpError, notFound, post, respond, type Ctx } from "../router";
import { addMessage, findTicket, OPENISH } from "../tickets";
import { escapeHtml, nowIso, sentencesUpTo, uuid } from "../util";
import { aiOff } from "./ai";
import { botName } from "./aiTicket";
import { retrieve } from "./agent";

const store = new WeakMap<WsData, SuggestedReply[]>();
const suggestions = (ws: WsData) => {
  let list = store.get(ws);
  if (!list) store.set(ws, (list = []));
  return list;
};

function role(ws: WsData) {
  return ws.members.find((m) => m.user_id === db.meId)?.role ?? "viewer";
}

function ticketOf(ctx: Ctx): Ticket {
  const t = findTicket(ctx.ws, ctx.params.id);
  if (!t || ctx.ws.deletedTickets.has(t.id)) throw notFound("Ticket");
  return t;
}

/** The latest reply of the ticket, the one a suggestion must answer to stay current. */
function latestReply(ws: WsData, t: Ticket) {
  return ws.messages.filter((m) => m.ticket_id === t.id && m.kind === "reply").sort((a, b) => a.created_at.localeCompare(b.created_at)).at(-1);
}

function current(ws: WsData, t: Ticket): SuggestedReply | null {
  const latest = latestReply(ws, t);
  return suggestions(ws).find((s) => s.ticket_id === t.id && s.status === "pending" && s.message_id === latest?.id) ?? null;
}

const firstName = (name: string | undefined) => (name ?? "").trim().split(/\s+/)[0] ?? "";

/**
 * Ollie writes a suggested reply to the ticket's latest customer email, as the
 * draft-mode job would. Null when the ticket is not one Ollie may answer.
 */
export function suggestReply(ws: WsData, t: Ticket, opts: { publish?: boolean; at?: string } = {}): SuggestedReply | null {
  const latest = latestReply(ws, t);
  const mailbox = ws.mailboxes.find((m) => m.id === t.mailbox_id);
  if (aiOff() || !ws.ai.enabled || !mailbox?.ai_answers || t.channel !== "email" || t.spam || t.assignee || !OPENISH.includes(t.status) || latest?.direction !== "inbound") return null;
  const doPublish = opts.publish ?? true;
  if (doPublish) {
    try {
      reserve(ws, "answer_email");
    } catch {
      return null; // AI balance too low: Ollie stays silent and the team takes over
    }
  }
  const bot = botName(ws, t);
  const found = retrieve(ws, `${t.subject} ${latest.body_text}`, 2);
  const lead = found[0];
  const body = [
    `<p>Hi ${escapeHtml(firstName(t.contact?.name) || "there")},</p>`,
    `<p>Thanks for getting in touch about “${escapeHtml(t.subject)}”.</p>`,
    lead
      ? `<p>${escapeHtml(sentencesUpTo(lead.text.replace(/\s+/g, " ").trim(), 280))}</p><p>There are more details in <a href="${escapeHtml(lead.source.url ?? "#")}">${escapeHtml(lead.source.title)}</a>.</p>`
      : `<p>I've passed this on and someone from the team will follow up with the details.</p>`,
    `<p>Best regards,<br>${escapeHtml(bot)}<br>${escapeHtml(ws.workspace.name)}</p>`,
  ].join("");
  for (const s of suggestions(ws)) if (s.ticket_id === t.id && s.status === "pending") s.status = "superseded";
  const s: SuggestedReply = {
    id: uuid(),
    ticket_id: t.id,
    message_id: latest.id,
    body_html: body,
    sources: found.map((f) => f.source),
    tools: ["search_knowledge"],
    status: "pending",
    reply_id: null,
    created_at: opts.at ?? nowIso(),
  };
  suggestions(ws).push(s);
  const activity = { id: uuid(), ticket_id: t.id, actor: { type: "ai", user: null }, type: "ai_suggested_reply", data: { suggested_reply_id: s.id, sources: s.sources, tools: s.tools }, created_at: s.created_at };
  ws.activities.push(activity);
  if (doPublish) {
    publish(ws, "activity.created", { ticket_id: t.id, activity });
    publish(ws, "ai_suggested_reply.created", { ticket_id: t.id, suggested_reply: s });
    spend(ws, "answer_email", null);
  }
  return s;
}

/** Seeds a suggestion on the open email tickets of mailboxes with email answers on. */
export function seedSuggestedReplies(ws: WsData) {
  const tickets = ws.tickets.filter((t) => ws.mailboxes.some((m) => m.id === t.mailbox_id && m.ai_answers) && !t.assignee && t.channel === "email");
  for (const t of tickets.slice(0, 3)) suggestReply(ws, t, { publish: false, at: latestReply(ws, t)?.created_at });
}

/** The agent report's split by channel: email from the mailboxes that answer, chat the rest. */
export function channelSplit(ws: WsData, r: AgentReport): AgentReport["channels"] {
  const zero: AgentChannelReport = { conversations: 0, replies: 0, resolved: 0, handoffs: 0, suggested: 0, suggested_sent: 0 };
  const on = ws.mailboxes.filter((m) => m.ai_answers);
  if (!on.length) return { chat: { ...zero, conversations: r.conversations, replies: r.replies, resolved: r.resolved, handoffs: r.handoffs }, email: zero };
  const share = on.some((m) => m.ai_answers_mode === "send") ? 0.3 : 0.12;
  const email: AgentChannelReport = {
    conversations: Math.round(r.conversations * share),
    replies: Math.round(r.replies * share * 0.6),
    resolved: Math.round(r.resolved * share),
    handoffs: Math.round(r.handoffs * share),
    suggested: Math.round(r.conversations * 0.18),
    suggested_sent: Math.round(r.conversations * 0.13),
  };
  return {
    chat: {
      ...zero,
      conversations: r.conversations - email.conversations,
      replies: r.replies - email.replies,
      resolved: r.resolved - email.resolved,
      handoffs: r.handoffs - email.handoffs,
    },
    email,
  };
}

export function registerEmailAnswers() {
  for (const ws of db.workspaces) seedSuggestedReplies(ws);

  get("/v1/tickets/:id/ai/suggested-reply", (ctx) => ({ suggested_reply: current(ctx.ws, ticketOf(ctx)) }));

  post("/v1/tickets/:id/ai/suggested-reply/:sid/send", (ctx) => {
    if (role(ctx.ws) === "viewer") throw forbidden("Viewers can't reply");
    const t = ticketOf(ctx);
    const s = current(ctx.ws, t);
    if (!s || s.id !== ctx.params.sid) {
      if (!suggestions(ctx.ws).some((x) => x.id === ctx.params.sid && x.ticket_id === t.id)) throw notFound("Suggested reply");
      throw new HttpError(409, "suggestion_not_pending", "This suggested reply was already sent, discarded or replaced.");
    }
    const msg = addMessage(ctx.ws, t, { kind: "reply", direction: "outbound", html: s.body_html, bot: botName(ctx.ws, t), delivery: "pending" });
    Object.assign(s, { status: "sent", reply_id: msg.id });
    const activity = {
      id: uuid(),
      ticket_id: t.id,
      actor: { type: "user", user: userRef(me().id) },
      type: "ai_suggested_reply_sent",
      data: { suggested_reply_id: s.id, message_id: msg.id },
      created_at: nowIso(),
    };
    ctx.ws.activities.push(activity);
    publish(ctx.ws, "activity.created", { ticket_id: t.id, activity });
    publish(ctx.ws, "ai_suggested_reply.updated", { ticket_id: t.id, suggested_reply: s });
    // The mock "delivers" a moment later.
    setTimeout(() => {
      if (msg.delivery_status !== "pending") return;
      msg.delivery_status = "sent";
      publish(ctx.ws, "message.updated", { ticket_id: t.id, message: msg });
    }, 1200);
    return msg;
  });

  del("/v1/tickets/:id/ai/suggested-reply/:sid", (ctx) => {
    if (role(ctx.ws) === "viewer") throw forbidden("Viewers can't change tickets");
    const t = ticketOf(ctx);
    const s = suggestions(ctx.ws).find((x) => x.id === ctx.params.sid && x.ticket_id === t.id);
    if (!s) throw notFound("Suggested reply");
    if (s.status === "pending") {
      s.status = "discarded";
      publish(ctx.ws, "ai_suggested_reply.updated", { ticket_id: t.id, suggested_reply: s });
    }
    return respond(204);
  });
}
