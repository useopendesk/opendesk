import { describe, expect, it } from "vitest";
import { installMock } from "../index";
import { dispatch } from "../router";
import { db } from "../db";
import type { Bot } from "@/lib/api/types";
import { randomLook } from "@/lib/botlook";

async function call(method: string, path: string, body?: unknown) {
  const ws = db.workspaces[0]?.workspace.id;
  const r = await dispatch(`http://mock${path}`, {
    method,
    body: body instanceof FormData ? body : body !== undefined ? JSON.stringify(body) : undefined,
    headers: ws ? { "OD-Workspace": ws } : {},
  });
  return { status: r.status, body: r.status === 204 ? null : await r.json() };
}
const bots = async (): Promise<Bot[]> => (await call("GET", "/v1/bots")).body.data;
const named = async (name: string) => (await bots()).find((b) => b.name === name)!;

describe("mock bots", () => {
  installMock();

  it("seeds Ollie as the default, Pip on Docs and the Concierge on Pressroom", async () => {
    const all = await bots();
    expect(all.map((b) => b.name)).toEqual(["Ollie", "Pip", "Margin Concierge"]);
    const [ollie, pip, concierge] = all;
    expect(ollie).toMatchObject({ is_default: true, look: { kind: "built", shape: "blob" } });
    expect(pip).toMatchObject({ is_default: false, personality: { tone: "playful", emoji: "lots" }, usage: { widgets: [{ name: "Docs" }], mailboxes: [] } });
    expect(concierge.personality.tone).toBe("professional");
    expect(concierge.usage.widgets.map((w) => w.name)).toEqual(["Pressroom app"]);
    expect(concierge.usage.mailboxes.map((m) => m.email)).toEqual(["hello@pressroom.app"]);
    for (const b of all) expect(b.avatar_url).toMatch(/^data:image\/svg\+xml/);
  });

  it("answers widget config as the widget's bot", async () => {
    const ws = db.workspaces[0];
    const docs = ws.widgets.find((w) => w.name === "Docs")!;
    const cfg = (await call("GET", `/v1/widget/config?key=${docs.key}`)).body;
    expect(cfg.bot).toMatchObject({ name: "Pip", role: "Docs sidekick", look: { kind: "built", shape: "squircle", accessory: "sprout" } });
    expect(cfg.bot.greeting).not.toBe("");
  });

  it("creates, validates and edits a bot", async () => {
    for (const body of [
      { name: "" },
      { name: "<b>Nova</b>" },
      { name: "x".repeat(41) },
      { name: "Nova", look: { kind: "built", shape: "star" } },
      { name: "Nova", look: { ...randomLook(4), kind: "image" } },
      { name: "Nova", language: "english!" },
      { name: "Nova", personality: { tone: "grumpy" } },
      { name: "Nova", personality: { tone: "custom" } },
      { name: "Nova", personality: { greeting: "Hi <script>" } },
      { name: "Nova", personality: { blocked_topics: ["x"] } },
    ])
      expect((await call("POST", "/v1/bots", body)).status, JSON.stringify(body)).toBe(422);

    const made = await call("POST", "/v1/bots", { name: "  Nova  ", look: randomLook(7), personality: { greeting: "Hey!\nHow can I help?" } });
    expect(made.status).toBe(201);
    expect(made.body).toMatchObject({ name: "Nova", is_default: false, look: randomLook(7), personality: { greeting: "Hey!\nHow can I help?", tone: "friendly" } });
    const id = made.body.id as string;

    const edited = await call("PATCH", `/v1/bots/${id}`, { role: "Billing help", personality: { tone: "warm", blocked_topics: [" refunds ", "Refunds"] } });
    expect(edited.body).toMatchObject({ role: "Billing help", personality: { tone: "warm", blocked_topics: ["refunds"], greeting: "Hey!\nHow can I help?" } });
    expect(db.workspaces[0].audit[0]).toMatchObject({ action: "bot.updated", entity_id: id, data: { changes: ["role", "personality.tone", "personality.blocked_topics"] } });
    expect((await call("DELETE", `/v1/bots/${id}`)).status).toBe(204);
    expect((await call("GET", `/v1/bots/${id}`)).status).toBe(404);
  });

  it("duplicates, reorders and moves the default", async () => {
    const pip = await named("Pip");
    const copy = (await call("POST", `/v1/bots/${pip.id}/duplicate`)).body as Bot;
    expect(copy).toMatchObject({ name: "Pip copy", is_default: false, look: pip.look, personality: pip.personality, usage: { widgets: [], mailboxes: [] } });
    expect((await bots()).map((b) => b.name)).toEqual(["Ollie", "Pip", "Pip copy", "Margin Concierge"]);

    const ids = (await bots()).map((b) => b.id);
    expect((await call("POST", "/v1/bots/reorder", { ids: ids.slice(1) })).status).toBe(422);
    const reordered = (await call("POST", "/v1/bots/reorder", { ids: [...ids].reverse() })).body.data as Bot[];
    expect(reordered.map((b) => b.name)).toEqual(["Margin Concierge", "Pip copy", "Pip", "Ollie"]);

    await call("POST", `/v1/bots/${copy.id}/default`);
    const after = await bots();
    expect(after.filter((b) => b.is_default).map((b) => b.name)).toEqual(["Pip copy"]);
    // Widgets without their own bot now answer as the new default.
    const key = db.workspaces[0].workspace.widget_key;
    expect((await call("GET", `/v1/widget/config?key=${key}`)).body.bot.name).toBe("Pip copy");

    const ollie = after.find((b) => b.name === "Ollie")!;
    await call("POST", `/v1/bots/${ollie.id}/default`);
    await call("POST", "/v1/bots/reorder", { ids });
    expect((await call("DELETE", `/v1/bots/${copy.id}`)).status).toBe(204);
  });

  it("refuses to delete the default or a bot in use, and says where", async () => {
    const ollie = await named("Ollie");
    const del = await call("DELETE", `/v1/bots/${ollie.id}`);
    expect(del.status).toBe(409);
    expect(del.body.code).toBe("bot_is_default");
    const concierge = await named("Margin Concierge");
    const busy = await call("DELETE", `/v1/bots/${concierge.id}`);
    expect(busy.status).toBe(409);
    expect(busy.body.code).toBe("bot_in_use");
    expect(busy.body.detail).toContain("Pressroom app");
    expect(busy.body.detail).toContain("hello@pressroom.app");
  });

  it("checks a widget's or address's bot belongs to the workspace", async () => {
    const ws = db.workspaces[0];
    const site = ws.widgets.find((w) => w.name === "Marketing site")!;
    expect((await call("PATCH", `/v1/widgets/${site.id}`, { bot_id: "6f1c1f5e-0000-4000-8000-000000000000" })).status).toBe(422);
    const pip = await named("Pip");
    expect((await call("PATCH", `/v1/widgets/${site.id}`, { bot_id: pip.id })).body.bot_id).toBe(pip.id);
    expect((await call("GET", `/v1/widget/config?key=${site.key}`)).body.bot.name).toBe("Pip");
    expect((await named("Pip")).usage.widgets.map((w) => w.name).sort()).toEqual(["Docs", "Marketing site"]);
    expect((await call("PATCH", `/v1/widgets/${site.id}`, { bot_id: null })).body.bot_id).toBeNull();

    const support = ws.mailboxes.find((m) => m.email !== "hello@pressroom.app")!;
    expect((await call("PATCH", `/v1/mailboxes/${support.id}`, { bot_id: "nope" })).status).toBe(422);
    expect((await call("PATCH", `/v1/mailboxes/${support.id}`, { bot_id: pip.id })).body.bot_id).toBe(pip.id);
    await call("PATCH", `/v1/mailboxes/${support.id}`, { bot_id: null });
  });

  it("caps a workspace at 50 bots", async () => {
    const made: string[] = [];
    const room = 50 - (await bots()).length;
    for (let i = 0; i < room; i++) made.push((await call("POST", "/v1/bots", { name: `Bot ${i}` })).body.id);
    const over = await call("POST", "/v1/bots", { name: "One too many" });
    expect(over.status).toBe(422);
    expect(over.body.code).toBe("limit_reached");
    for (const id of made) await call("DELETE", `/v1/bots/${id}`);
    expect((await bots()).length).toBe(3);
  }, 30_000);

  it("answers the playground as a draft of the bot", async () => {
    const pip = await named("Pip");
    const ask = (settings: unknown) => call("POST", "/v1/ai/agent/playground", { messages: [{ role: "visitor", text: "How do I set up SSO?" }], settings });
    const plain = (await ask({})).body.text_html as string;
    const playful = (await ask({ bot_id: pip.id })).body.text_html as string;
    expect(playful).not.toBe(plain);
    expect(playful).toMatch(/👋|✨/);
    const quiet = (await ask({ bot_id: pip.id, bot: { personality: { emoji: "none" } } })).body.text_html as string;
    expect(quiet).not.toMatch(/👋|✨/);
    expect((await ask({ bot_id: "nope" })).status).toBe(422);
    expect((await ask({ bot: { personality: { tone: "grumpy" } } })).status).toBe(422);
  });
});
