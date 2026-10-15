import { afterEach, describe, expect, it, vi } from "vitest";
import { installMock } from "../index";
import { dispatch } from "../router";
import { db } from "../db";
import { socketFactory } from "../gateway";
import { chatTicket, ollieReply } from "../ollie";
import { agentReport } from "./agent";

async function call(method: string, path: string, body?: unknown, headers: Record<string, string> = {}) {
  const ws = db.workspaces[0]?.workspace.id;
  const r = await dispatch(`http://mock${path}`, {
    method,
    body: body instanceof FormData ? body : body ? JSON.stringify(body) : undefined,
    headers: { ...(ws ? { "OD-Workspace": ws } : {}), ...headers },
  });
  const ct = r.headers.get("content-type") ?? "";
  return { status: r.status, body: r.status === 204 ? null : ct.includes("event-stream") ? await r.text() : await r.json() };
}

function frames(text: string) {
  return text
    .split("\n\n")
    .filter(Boolean)
    .map((f) => {
      const [ev, data] = f.split("\n");
      return { event: ev.replace("event: ", ""), data: JSON.parse(data.replace("data: ", "")) };
    });
}

describe("mock widget agent", () => {
  installMock();
  afterEach(() => {
    vi.useRealTimers();
  });

  it("lists models and reports effective, metered config", async () => {
    const m = await call("GET", "/v1/ai/models");
    expect(m.body.default).toBe("anthropic/claude-sonnet-5.5");
    expect(m.body.fast).toBe("anthropic/claude-haiku-5.5");
    expect(m.body.data).toHaveLength(7);
    expect(m.body.data.find((x: { id: string }) => x.id === "openai/gpt-5.6-sol")).toMatchObject({ vendor: "OpenAI", usd_per_mtok: { input: 3, output: 15 }, typical_reply_usd: 0.0195 });
    const c = (await call("GET", "/v1/ai/config")).body;
    expect(c).toMatchObject({ provider: "openrouter", metered: true, models: { default: "anthropic/claude-sonnet-5.5", fast: "anthropic/claude-haiku-5.5" } });
    // What a typical call costs on the chosen models, in US dollars: Sonnet for the agent, Haiku for rewrites, Jev for checks.
    expect(c.costs).toMatchObject({ playground: 0.021, draft_reply: 0.0195, rewrite: 0.00042, answer_check: 0.00075 });
    await call("PATCH", "/v1/ai/settings", { model: "anthropic/claude-opus-5.5" });
    expect((await call("GET", "/v1/ai/config")).body.costs.draft_reply).toBe(0.039);
    await call("PATCH", "/v1/ai/settings", { model: "" });
  });

  it("validates the agent settings", async () => {
    const bad = [
      { model: "openai/gpt-4" },
      { widget_max_replies: 0 },
      { widget_max_replies: 2.5 },
      { widget_blocked_topics: Array.from({ length: 21 }, (_, i) => `topic ${i}`) },
      { widget_blocked_topics: ["x"] },
      { widget_blocked_topics: ["<b>legal</b>"] },
      { widget_monthly_cap_usd: -1 },
      { widget_monthly_cap_usd: 10_001 },
      { widget_collect_email: "yes" },
    ];
    for (const b of bad) expect((await call("PATCH", "/v1/ai/settings", b)).status, JSON.stringify(b).slice(0, 60)).toBe(422);
    const ok = await call("PATCH", "/v1/ai/settings", { widget_model: "anthropic/claude-haiku-5.5", model: "openai/gpt-5.6-sol", widget_max_replies: 12, widget_blocked_topics: ["  legal   advice "] });
    expect(ok.body).toMatchObject({ widget_model: "anthropic/claude-haiku-5.5", widget_max_replies: 12, widget_blocked_topics: ["legal advice"], widget_answers: true });
    expect((await call("GET", "/v1/ai/config")).body.models.default).toBe("openai/gpt-5.6-sol");
    await call("PATCH", "/v1/ai/settings", { model: "", widget_model: "" });
  });

  it("validates widget appearance and builds the public config", async () => {
    for (const b of [{ color: "red" }, { theme: "neon" }, { offset_x: 201 }, { offset_y: 1.5 }, { title: "x".repeat(41) }, { suggestions: ["a"] }, { suggestions: ["one", "two", "three", "four", "five"] }]) {
      expect((await call("PATCH", "/v1/widget/settings", b)).status, JSON.stringify(b)).toBe(422);
    }
    const w = await call("PATCH", "/v1/widget/settings", { position: "left", offset_x: 32 });
    expect(w.body).toMatchObject({ position: "left", offset_x: 32, title: "Margin support", color: "#4f46e5" });
    const key = db.workspaces[0].workspace.widget_key;
    const cfg = (await call("GET", `/v1/widget/config?key=${key}`)).body;
    expect(cfg).toMatchObject({ workspace_name: "Margin", brand_color: "#4f46e5", ai_answers: true, position: "left" });
    // The widget answers as the default bot (contracts/bots.md).
    expect(cfg.bot).toMatchObject({ name: "Ollie", role: "Margin support", look: { kind: "built", shape: "blob" } });
    expect(cfg.bot.id).toBeTruthy();
    expect(cfg.bot.avatar_url).toMatch(/^data:image\/svg\+xml/);
    expect(cfg.topics.length).toBeGreaterThan(0);
    expect(Object.keys(cfg.topics[0]).sort()).toEqual(["id", "label"]);
    expect(typeof cfg.open).toBe("boolean");
    expect(cfg.offline_message).not.toBe("");
    expect(cfg.brand).toMatchObject({ name: "Margin" });
    expect(cfg.team.length).toBeLessThanOrEqual(3);
    expect(cfg.team[0].name).not.toContain(" ");
    expect((await call("GET", "/v1/widget/config?key=nope")).status).toBe(404);
    await call("PATCH", "/v1/widget/settings", { position: "right", offset_x: 20 });
  });

  it("reports usage in US dollars with a by-model breakdown", async () => {
    const u = (await call("GET", "/v1/billing/usage")).body;
    expect(u.by_model.length).toBeGreaterThan(1);
    expect(u.by_model.map((m: { model: string }) => m.model)).toContain("jev");
    const usd = u.by_model.map((m: { usd: number }) => m.usd);
    expect(usd).toEqual([...usd].sort((a, b) => b - a));
    expect(usd.some((c: number) => !Number.isInteger(c))).toBe(true);
    for (const c of [u.total_usd, ...usd]) expect(Math.round(c * 1e6) / 1e6).toBe(c);
    expect(Math.round(usd.reduce((s: number, c: number) => s + c * 1e6, 0)) / 1e6).toBe(u.total_usd);
  });

  it("builds a consistent agent report", () => {
    const r = agentReport(db.workspaces[0], "2026-09-01", "2026-09-30", "2026-10-11");
    expect(r.by_day).toHaveLength(30);
    expect(r.conversations).toBe(r.by_day.reduce((n, d) => n + d.conversations, 0));
    expect(r.gaps.length).toBeGreaterThanOrEqual(8);
    expect(r.tools.some((t) => t.tool === "shop__lookup_order")).toBe(true);
    const sat = r.by_day.find((d) => new Date(`${d.date}T00:00:00Z`).getUTCDay() === 6)!;
    const mon = r.by_day.find((d) => new Date(`${d.date}T00:00:00Z`).getUTCDay() === 1)!;
    expect(sat.conversations).toBeLessThan(mon.conversations);
  });

  it("answers in the playground: JSON fallback, actions, handoffs and blocked topics", async () => {
    expect((await call("POST", "/v1/ai/agent/playground", { messages: [{ role: "agent", text: "Hi" }] })).status).toBe(422);
    const ask = (text: string, extra: Record<string, unknown> = {}) => call("POST", "/v1/ai/agent/playground", { messages: [{ role: "visitor", text }], ...extra });
    const cancel = (await ask("Please cancel my order #4821")).body;
    expect(cancel.action).toEqual({ tool: "shop__cancel_order", title: "Cancel order", summary: "Cancel order #4821 and refund $48.00 to the card ending 4242" });
    expect(cancel.usd).toBeGreaterThan(0);
    const human = (await ask("Can I talk to a human?", { visitor: { email: "dana@oakridge.org" } })).body;
    expect(human).toMatchObject({ handoff: true, usd: 0 });
    const blocked = (await ask("I need legal advice about my contract", { settings: { widget_blocked_topics: ["legal advice"] } })).body;
    expect(blocked.text_html).toContain("can't help with legal advice");
    const unverified = (await ask("Where is my order #5100?", { visitor: { verified: false } })).body;
    expect(unverified.text_html).toContain("signed-in customers");
  });

  it("streams tool steps, sources and deltas, then done without citation markers", async () => {
    const order = await call("POST", "/v1/ai/agent/playground", { messages: [{ role: "visitor", text: "Where is my order #4821?" }], visitor: { name: "Dana Whitfield", verified: true } }, { Accept: "text/event-stream" });
    const ev = frames(order.body);
    expect(ev.filter((e) => e.event === "tool").map((e) => [e.data.name, e.data.status])).toEqual([
      ["search_knowledge", "running"],
      ["search_knowledge", "done"],
      ["shop__lookup_order", "running"],
      ["shop__lookup_order", "done"],
    ]);
    const done = ev[ev.length - 1];
    expect(done.event).toBe("done");
    expect(done.data.text_html).toContain("Hi Dana!");
    const kb = frames((await call("POST", "/v1/ai/agent/playground", { messages: [{ role: "visitor", text: "How do I export my notebook to PDF?" }] }, { Accept: "text/event-stream" })).body);
    const text = kb.filter((e) => e.event === "delta").map((e) => e.data.text).join("");
    expect(text).toMatch(/\[1\]/);
    const end = kb[kb.length - 1].data;
    expect(end.sources.length).toBeGreaterThan(0);
    expect(end.text_html).not.toMatch(/\[\d\]/);
  }, 15_000);

  it("connects, patches and deletes MCP servers", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const list = (await call("GET", "/v1/ai/mcp-servers")).body.data;
    expect(list.map((s: { slug: string }) => s.slug)).toEqual(["shop", "linear"]);
    expect(list[0].tools.find((t: { name: string }) => t.name === "cancel_order")).toMatchObject({ destructive: true, enabled: false, policy: "confirm" });
    expect((await call("POST", "/v1/ai/mcp-servers", { name: "CRM", url: "http://crm.example.com/mcp", auth: { kind: "none" } })).body.code).toBe("invalid_url");
    expect((await call("POST", "/v1/ai/mcp-servers", { name: "CRM", url: "https://crm.example.com/mcp", auth: { kind: "bearer" } })).status).toBe(422);
    const created = await call("POST", "/v1/ai/mcp-servers", { name: "CRM tools", url: "https://crm.example.com/mcp", auth: { kind: "bearer", token: "sk_live_secret" } });
    expect(created.status).toBe(201);
    expect(created.body).toMatchObject({ status: "connected", slug: "crm_tools", auth: { kind: "bearer", has_secret: true } });
    expect(JSON.stringify(created.body)).not.toContain("sk_live_secret");
    const id = created.body.id;
    expect(created.body.tools.length).toBeGreaterThan(0);
    const oauthNew = (await call("POST", "/v1/ai/mcp-servers", { name: "Docs", url: "https://mcp.docs.example.com/mcp", auth: { kind: "oauth" } })).body;
    expect(oauthNew).toMatchObject({ status: "needs_auth", tools: [] });
    expect((await call("POST", `/v1/ai/mcp-servers/${id}/oauth/start`)).body.code).toBe("not_oauth");
    const p = await call("PATCH", `/v1/ai/mcp-servers/${id}`, { tools: { create_record: { enabled: true, policy: "confirm", verified_only: true } } });
    expect(p.body.tools.find((t: { name: string }) => t.name === "create_record")).toMatchObject({ enabled: true, verified_only: true });
    expect((await call("PATCH", `/v1/ai/mcp-servers/${id}`, { tools: { nope: { enabled: true } } })).body.code).toBe("unknown_tool");
    const test = await call("POST", `/v1/ai/mcp-servers/${list[0].id}/tools/lookup_order/test`, { arguments: {} });
    expect(test.body).toMatchObject({ is_error: true });
    const okTest = await call("POST", `/v1/ai/mcp-servers/${list[0].id}/tools/lookup_order/test`, { arguments: { order_id: "4821" } });
    expect(JSON.parse(okTest.body.content).order_id).toBe("4821");
    const oauth = await call("POST", `/v1/ai/mcp-servers/${list[1].id}/oauth/start`);
    expect(oauth.body.url).toBe(`/settings/ai/tools?connected=${list[1].id}`);
    expect(JSON.parse(sessionStorage.getItem("od-mock-mcp-oauth") ?? "{}").margin).toContain("linear");
    const before = db.workspaces[0].seq;
    expect((await call("DELETE", `/v1/ai/mcp-servers/${id}`)).status).toBe(204);
    const ev = (await call("GET", `/v1/events?since=${before}`)).body.data;
    expect(ev.some((e: { type: string; data: { mcp_server_id?: string } }) => e.type === "ai_mcp_server.deleted" && e.data.mcp_server_id === id)).toBe(true);
   }, 15_000);

  it("runs a website source through pending, syncing and ready", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const seeded = (await call("GET", "/v1/ai/knowledge")).body.data;
    expect(seeded.map((s: { status: string }) => s.status)).toEqual(expect.arrayContaining(["ready", "error"]));
    expect(seeded.find((s: { name: string }) => s.name === "Margin blog").doc_count).toBeGreaterThanOrEqual(23);
    expect(seeded.some((s: { enabled: boolean }) => !s.enabled)).toBe(true);
    const docs = (await call("GET", `/v1/ai/knowledge/${seeded[0].id}/docs?limit=20`)).body;
    expect(docs.data).toHaveLength(20);
    expect(docs.next_cursor).toBe("20");
    const before = db.workspaces[0].seq;
    const created = (await call("POST", "/v1/ai/knowledge", { kind: "website", website: { url: "https://docs.example.com", include: ["/guides"] } })).body;
    expect(created).toMatchObject({ status: "pending", name: "docs.example.com", doc_count: 0 });
    await vi.advanceTimersByTimeAsync(2000);
    const mid = (await call("GET", `/v1/ai/knowledge/${created.id}`)).body;
    expect(mid.status).toBe("syncing");
    expect((await call("POST", `/v1/ai/knowledge/${created.id}/sync`)).status).toBe(409);
    await vi.advanceTimersByTimeAsync(6000);
    const done = (await call("GET", `/v1/ai/knowledge/${created.id}`)).body;
    expect(done.status).toBe("ready");
    expect(done.doc_count).toBeGreaterThan(mid.doc_count);
    const updates = (await call("GET", `/v1/events?since=${before}`)).body.data.filter((e: { type: string; data: { source?: { id: string } } }) => e.type === "ai_knowledge.updated" && e.data.source?.id === created.id);
    expect(updates.length).toBeGreaterThanOrEqual(4);

    const empty = new FormData();
    empty.append("file", new File(["x"], "empty-notes.txt", { type: "text/plain" }));
    const file = (await call("POST", "/v1/ai/knowledge/files", empty)).body;
    expect(file.kind).toBe("file");
    await vi.advanceTimersByTimeAsync(3000);
    expect((await call("GET", `/v1/ai/knowledge/${file.id}`)).body).toMatchObject({ status: "error", error: "No text found in this file" });
    const exe = new FormData();
    exe.append("file", new File(["x"], "setup.exe", { type: "application/octet-stream" }));
    expect((await call("POST", "/v1/ai/knowledge/files", exe)).status).toBe(422);

    const snip = (await call("POST", "/v1/ai/knowledge", { kind: "snippet", snippet: { question: "Do you ship to Canada?", answer: "Yes, in 5 to 8 days." } })).body;
    expect(snip).toMatchObject({ status: "ready", doc_count: 1 });
    expect((await call("POST", "/v1/ai/knowledge", { kind: "snippet", snippet: { question: "Hi", answer: "x" } })).status).toBe(422);
  });

  it("streams a live Ollie reply on a chat ticket and posts it", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const ws = db.workspaces[0];
    const t = chatTicket(ws)!;
    expect(t).toBeTruthy();
    const sock = socketFactory("ws://mock", () => ({ token: "t" }));
    sock.connect();
    await vi.advanceTimersByTimeAsync(200);
    const ch = sock.channel(`ticket:${ws.workspace.id}:${t.id}`, {});
    const live: { type: string; data: Record<string, unknown> }[] = [];
    ch.on("live", (p) => live.push(p as (typeof live)[number]));
    ch.join();
    const pending = ollieReply(ws, t, { deltaMs: 5 });
    await vi.advanceTimersByTimeAsync(6000);
    const id = await pending;
    expect(id).toBeTruthy();
    expect(live[0]).toMatchObject({ type: "ai.reply.started", data: { ticket_id: t.id, bot: { name: "Ollie" } } });
    expect(live.filter((e) => e.type === "ai.reply.delta").length).toBeGreaterThanOrEqual(10);
    expect(live[live.length - 1]).toMatchObject({ type: "ai.reply.ended", data: { message_id: id, handoff: false, action_id: null } });
    const msg = ws.messages.find((m) => m.id === id)!;
    expect(msg).toMatchObject({ ai: true, from_name: "Ollie", author: { type: "system" }, ai_action: null, ai_rating: null });
    ch.leave();
  });

  it("stays silent at the monthly cap and tells admins once", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const ws = db.workspaces[0];
    const cap = ws.ai.widget_monthly_cap_usd;
    ws.ai.widget_monthly_cap_usd = 0.01;
    const t = chatTicket(ws)!;
    expect(await ollieReply(ws, t)).toBeNull();
    expect(await ollieReply(ws, t)).toBeNull();
    const notes = ws.notifications.filter((n) => n.type === ("ai_cap_reached" as string));
    expect(notes).toHaveLength(1);
    expect(notes[0]).toMatchObject({ ticket: null, data: { cap_usd: 0.01 } });
    ws.ai.widget_monthly_cap_usd = cap;
  });

  it("seeds agent replies with an action and ratings", () => {
    const ws = db.workspaces[0];
    const ai = ws.messages.filter((m) => m.ai);
    expect(ai.some((m) => m.ai_rating === "up")).toBe(true);
    expect(ai.some((m) => m.ai_rating === "down")).toBe(true);
    expect(ai.find((m) => m.ai_action)?.ai_action).toMatchObject({ title: "Cancel order", status: "confirmed", tool: "Shop" });
    expect(ws.messages.filter((m) => !m.ai).every((m) => m.ai_action === null && m.ai_rating === null)).toBe(true);
  });
});
