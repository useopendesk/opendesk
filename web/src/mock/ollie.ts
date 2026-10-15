// The widget agent replying live on a chat ticket (contracts/agent.md
// "Output and posting", contracts/realtime.md "Live events"): a visitor
// message, then "live" pushes on the ticket topic while Ollie writes, then the
// posted message as a normal message.created.

import type { NotificationType, Ticket } from "@/lib/api/types";
import { db, type WsData } from "./db";
import { fromMicro, reserve, spend, toMicro } from "./aiBalance";
import { activeTicketIds, emitOnTicket, publish } from "./gateway";
import { aiOff } from "./handlers/ai";
import { retrieve } from "./handlers/agent";
import { customerReply } from "./handlers/tickets";
import { chunk } from "./router";
import { addMessage, findTicket, OPENISH } from "./tickets";
import { escapeHtml, markdownToHtml, nowIso, textToHtml, uuid } from "./util";
import { botForTicket, botRef } from "./bots";

type Script = { question: string; tool: string | null; answer: string };

const SCRIPTS: Script[] = [
  {
    question: "Where can I download my invoices?",
    tool: "Shop",
    answer: "You can download every invoice under **Settings > Billing > Invoices** [1]. Your last two were paid on October 1 and September 1, $96.00 each.\n\nWould you like me to send them to your email?",
  },
  {
    question: "Does the Business plan include SSO?",
    tool: "Knowledge",
    answer: "Yes. SSO with Okta, Google Workspace or any SAML 2.0 provider is included in the Business plan [1]. An admin can turn it on under **Settings > Security**, and setup takes about ten minutes.",
  },
  {
    question: "Can I export a whole notebook to Markdown?",
    tool: null,
    answer: "Yes. Open the notebook, choose **Export**, then **Markdown** [1]. You get a .zip with one file per note, and the images in a separate folder.",
  },
  {
    question: "Is there a way to use Margin offline on my phone?",
    tool: "Knowledge",
    answer: "Yes. Notes you opened recently are available offline on iOS and Android [1]. To keep a whole notebook offline, open it and turn on **Available offline** in its menu.",
  },
];

/** US dollars of AI balance the agent spent this calendar month (UTC): features answer and answer_check. */
export function agentSpendThisMonth(ws: WsData, now = new Date()): number {
  const month = now.toISOString().slice(0, 7);
  const used = ws.aiUsage.filter((u) => (u.feature === "answer" || u.feature === "answer_check") && u.at.startsWith(month)).reduce((n, u) => n + u.micro_usd, 0);
  return fromMicro(used);
}

const capNotified = new Set<string>();

/** At the monthly cap the agent stays silent; owners and admins hear about it once a month. */
function capReached(ws: WsData): boolean {
  const cap = ws.ai.widget_monthly_cap_usd ?? 0;
  if (!cap || toMicro(agentSpendThisMonth(ws)) < toMicro(cap)) return false;
  const key = `${ws.workspace.id}:${new Date().toISOString().slice(0, 7)}`;
  const role = ws.members.find((m) => m.user_id === db.meId)?.role;
  if (!capNotified.has(key) && (role === "owner" || role === "admin")) {
    capNotified.add(key);
    const n = { id: uuid(), type: "ai_cap_reached" as string as NotificationType, ticket: null, data: { cap_usd: cap }, read_at: null, created_at: nowIso() };
    ws.notifications.unshift(n);
    publish(ws, "notification.created", { notification: n }, { topics: [`user:${db.meId}`] });
  }
  return true;
}

let next = 0;
const busy = new Set<string>();
const wait = (ms: number) => new Promise((r) => setTimeout(r, ms));

function liveOn(t: Ticket, type: string, data: Record<string, unknown>) {
  emitOnTicket(t.id, "live", { type, data: { ticket_id: t.id, ...data } });
}

/** An open chat ticket: the given one (id or number), else one an agent is viewing, else any. */
export function chatTicket(ws: WsData, idOrNumber?: string): Ticket | undefined {
  const ok = (t: Ticket | undefined) => !!t && t.channel === "chat" && !t.spam && OPENISH.includes(t.status) && !!t.contact && !ws.deletedTickets.has(t.id);
  if (idOrNumber) {
    const t = findTicket(ws, idOrNumber);
    return ok(t) ? t : undefined;
  }
  const active = activeTicketIds()
    .map((id) => ws.tickets.find((t) => t.id === id))
    .find(ok);
  return active ?? ws.tickets.filter(ok).sort((a, b) => Date.parse(b.last_message_at) - Date.parse(a.last_message_at))[0];
}

/**
 * The visitor asks, Ollie streams a reply and posts it. Resolves with the
 * posted message id, or null when nothing was posted (AI balance too low, busy).
 */
export async function ollieReply(ws: WsData, t: Ticket, opts: { deltaMs?: number } = {}): Promise<string | null> {
  if (busy.has(t.id) || capReached(ws)) return null;
  try {
    reserve(ws, "answer");
  } catch {
    return null; // AI balance too low: the agent stays silent and the team takes over
  }
  busy.add(t.id);
  try {
    const script = SCRIPTS[next++ % SCRIPTS.length];
    const b = botForTicket(ws, t);
    const bot = b.name;
    emitOnTicket(t.id, "customer_typing", { ticket_id: t.id, contact_id: t.contact?.id });
    await wait(900);
    customerReply(ws, t, textToHtml(script.question));
    // The job runs 1.5 s after the visitor's message.
    await wait(1500);
    const reply_id = uuid();
    liveOn(t, "ai.reply.started", { reply_id, bot: botRef(ws, b) });
    if (script.tool) {
      await wait(300);
      liveOn(t, "ai.reply.tool", { reply_id, tool: script.tool, status: "running" });
      await wait(900);
      liveOn(t, "ai.reply.tool", { reply_id, tool: script.tool, status: "done" });
    }
    const words = script.answer.split(/\s+/).length;
    for (const text of chunk(script.answer, Math.max(1, Math.ceil(words / 30)))) {
      liveOn(t, "ai.reply.delta", { reply_id, text });
      await wait(opts.deltaMs ?? 60);
    }
    if (ws.deletedTickets.has(t.id) || !ws.tickets.includes(t)) {
      liveOn(t, "ai.reply.ended", { reply_id, message_id: null, handoff: false, action_id: null });
      return null;
    }
    const source = retrieve(ws, script.question, 1)[0]?.source;
    const sources = source?.url ? `<p><strong>Sources</strong></p><ul><li><a href="${escapeHtml(source.url)}">${escapeHtml(source.title)}</a></li></ul>` : "";
    const msg = addMessage(ws, t, { kind: "reply", direction: "outbound", html: markdownToHtml(script.answer) + sources, bot });
    const activity = {
      id: uuid(),
      ticket_id: t.id,
      actor: { type: "ai", user: null },
      type: "ai_answered",
      data: { message_id: msg.id, sources: source ? [source] : [], tools: script.tool ? [script.tool === "Shop" ? "shop__list_invoices" : "search_knowledge"] : [] },
      created_at: nowIso(),
    };
    ws.activities.push(activity);
    publish(ws, "activity.created", { ticket_id: t.id, activity });
    spend(ws, "answer_check", null);
    spend(ws, "answer", null);
    liveOn(t, "ai.reply.ended", { reply_id, message_id: msg.id, handoff: false, action_id: null });
    return msg.id;
  } finally {
    busy.delete(t.id);
  }
}

/** The live loop's turn: only on a chat ticket someone is viewing, when the agent is on. */
export function ollieOnActive() {
  const ws = db.workspaces[0];
  if (!ws || aiOff() || !ws.ai.enabled || !ws.ai.widget_answers) return;
  const t = activeTicketIds()
    .map((id) => ws.tickets.find((x) => x.id === id))
    .find((x) => x && x.channel === "chat" && !x.spam && OPENISH.includes(x.status) && x.contact);
  if (t) void ollieReply(ws, t);
}
