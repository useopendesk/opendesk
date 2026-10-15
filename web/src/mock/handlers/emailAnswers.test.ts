import { describe, expect, it } from "vitest";
import type { Ticket } from "@/lib/api/types";
import { installMock } from "../index";
import { dispatch } from "../router";
import { db, type WsData } from "../db";
import { agentReport } from "./agent";
import { suggestReply } from "./emailAnswers";

async function call(method: string, path: string, body?: unknown) {
  const ws = db.workspaces[0]?.workspace.id;
  const r = await dispatch(`http://mock${path}`, { method, body: body ? JSON.stringify(body) : undefined, headers: ws ? { "OD-Workspace": ws } : {} });
  return { status: r.status, body: r.status === 204 ? null : await r.json() };
}

/** Open, unassigned email tickets in a mailbox where Ollie answers, whose latest reply is the customer's. */
function answerable(ws: WsData): Ticket[] {
  return ws.tickets.filter((t) => {
    if (t.channel !== "email" || t.assignee || t.spam || !["open", "pending", "on_hold"].includes(t.status)) return false;
    if (!ws.mailboxes.some((m) => m.id === t.mailbox_id && m.ai_answers)) return false;
    const last = ws.messages.filter((m) => m.ticket_id === t.id && m.kind === "reply").sort((a, b) => a.created_at.localeCompare(b.created_at)).at(-1);
    return last?.direction === "inbound";
  });
}

describe("mock Ollie email answers", () => {
  installMock();
  const ws = () => db.workspaces[0];

  it("keeps the mailbox setting off by default and validates it", async () => {
    const mbs = (await call("GET", "/v1/mailboxes")).body.data as { id: string; name: string; ai_answers: boolean; ai_answers_mode: string }[];
    expect(mbs.find((m) => m.name === "Support")).toMatchObject({ ai_answers: true, ai_answers_mode: "draft" });
    const billing = mbs.find((m) => m.name === "Billing")!;
    expect(billing).toMatchObject({ ai_answers: false, ai_answers_mode: "draft", ai_answers_outside_hours: false });

    expect((await call("PATCH", `/v1/mailboxes/${billing.id}`, { ai_answers_mode: "maybe" })).status).toBe(422);
    ws().ai.enabled = false;
    const off = await call("PATCH", `/v1/mailboxes/${billing.id}`, { ai_answers: true });
    expect(off.status).toBe(422);
    expect(off.body.code).toBe("ai_off");
    ws().ai.enabled = true;
    const on = await call("PATCH", `/v1/mailboxes/${billing.id}`, { ai_answers: true, ai_answers_mode: "send", ai_answers_outside_hours: true });
    expect(on.body).toMatchObject({ ai_answers: true, ai_answers_mode: "send", ai_answers_outside_hours: true });
    await call("PATCH", `/v1/mailboxes/${billing.id}`, { ai_answers: false, ai_answers_mode: "draft", ai_answers_outside_hours: false });
  });

  it("quotes whole sentences of the article, not its headings", async () => {
    const t = ws().tickets.find((x) => x.contact?.name === "Elena Rossi" && /export/i.test(x.subject))!;
    expect(t).toBeTruthy();
    const got = (await call("GET", `/v1/tickets/${t.id}/ai/suggested-reply`)).body.suggested_reply ?? suggestReply(ws(), t);
    const html: string = got.body_html;
    expect(html).toContain("Exports run in the background.");
    expect(html).not.toContain("If the export fails");
    expect(html).not.toContain("…");
  });

  it("sends a suggested reply as Ollie once", async () => {
    const t = answerable(ws())[0];
    expect(t).toBeTruthy();
    const s = suggestReply(ws(), t)!;
    expect(s.status).toBe("pending");
    expect(s.body_html).toContain("Best regards");
    const got = (await call("GET", `/v1/tickets/${t.id}/ai/suggested-reply`)).body.suggested_reply;
    expect(got.id).toBe(s.id);

    const before = ws().seq;
    const sent = await call("POST", `/v1/tickets/${t.id}/ai/suggested-reply/${s.id}/send`);
    expect(sent.status).toBe(201);
    expect(sent.body).toMatchObject({ ai: true, direction: "outbound", kind: "reply" });
    const types = (await call("GET", `/v1/events?since=${before}`)).body.data.map((e: { type: string }) => e.type);
    expect(types).toEqual(expect.arrayContaining(["message.created", "ai_suggested_reply.updated", "activity.created"]));

    const again = await call("POST", `/v1/tickets/${t.id}/ai/suggested-reply/${s.id}/send`);
    expect(again.status).toBe(409);
    expect(again.body.code).toBe("suggestion_not_pending");
    expect((await call("GET", `/v1/tickets/${t.id}/ai/suggested-reply`)).body.suggested_reply).toBeNull();
    // Ollie's own reply is now the latest: it does not suggest another.
    expect(suggestReply(ws(), t)).toBeNull();
  });

  it("discards, and refuses viewers", async () => {
    const t = answerable(ws())[0];
    const s = suggestReply(ws(), t)!;
    const me = ws().members.find((m) => m.user_id === db.meId)!;
    const role = me.role;
    me.role = "viewer";
    expect((await call("DELETE", `/v1/tickets/${t.id}/ai/suggested-reply/${s.id}`)).status).toBe(403);
    expect((await call("POST", `/v1/tickets/${t.id}/ai/suggested-reply/${s.id}/send`)).status).toBe(403);
    me.role = role;
    expect((await call("DELETE", `/v1/tickets/${t.id}/ai/suggested-reply/${s.id}`)).status).toBe(204);
    expect((await call("DELETE", `/v1/tickets/${t.id}/ai/suggested-reply/${s.id}`)).status).toBe(204);
    expect((await call("GET", `/v1/tickets/${t.id}/ai/suggested-reply`)).body.suggested_reply).toBeNull();
  });

  it("never suggests on mailboxes that are off or assigned tickets", () => {
    const other = ws().tickets.find((t) => t.channel === "email" && !ws().mailboxes.some((m) => m.id === t.mailbox_id && m.ai_answers));
    if (other) expect(suggestReply(ws(), other)).toBeNull();
    const assigned = ws().tickets.find((t) => t.channel === "email" && t.assignee);
    if (assigned) expect(suggestReply(ws(), assigned)).toBeNull();
  });

  it("splits the agent report by channel", () => {
    const r = agentReport(ws(), "2026-09-01", "2026-09-30", "2026-10-12");
    expect(r.channels?.email.conversations).toBeGreaterThan(0);
    expect((r.channels?.chat.conversations ?? 0) + (r.channels?.email.conversations ?? 0)).toBe(r.conversations);
  });
});
