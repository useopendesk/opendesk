import { describe, expect, it } from "vitest";
import { installMock } from "../index";
import { dispatch } from "../router";
import { db } from "../db";
import { draftArticle, score, semanticSearch, terms } from "./ai";

async function call(method: string, path: string, body?: unknown) {
  const ws = db.workspaces[0]?.workspace.id;
  const r = await dispatch(`http://mock${path}`, { method, body: body ? JSON.stringify(body) : undefined, headers: ws ? { "OD-Workspace": ws } : {} });
  return { status: r.status, body: r.status === 204 ? null : await r.json() };
}

describe("mock AI scoring", () => {
  it("drops stop words and plurals", () => {
    expect(terms("How do I get my refunds?")).toEqual(["refund"]);
  });

  it("ranks exact title hits above concept matches above nothing", () => {
    const exact = score("refund", "Refund for duplicate annual charge", "");
    const concept = score("money back", "Refund for duplicate annual charge", "");
    const none = score("dark mode", "Refund for duplicate annual charge", "");
    expect(exact).toBeGreaterThan(concept);
    expect(concept).toBeGreaterThan(none);
    expect(none).toBe(0);
  });
});

describe("mock AI routes", () => {
  installMock();

  it("reports config with costs", async () => {
    const r = await call("GET", "/v1/ai/config");
    expect(r.status).toBe(200);
    expect(r.body.enabled).toBe(true);
    // 8,000 input and 1,500 output tokens on Claude Sonnet 5.5 at $3 / $15 per 1M.
    expect(r.body.costs.kb_draft).toBe(0.0465);
  });

  it("finds billing tickets by meaning", () => {
    const res = semanticSearch(db.workspaces[0], "customer was charged twice");
    expect(res.tickets.some((t) => /charge|refund|billing/i.test(t.subject))).toBe(true);
  });

  it("validates and saves settings", async () => {
    expect((await call("PATCH", "/v1/ai/settings", { tone: "x".repeat(201) })).status).toBe(422);
    const ok = await call("PATCH", "/v1/ai/settings", { tone: "professional", auto_triage: true });
    expect(ok.status).toBe(200);
    expect(ok.body.auto_triage).toBe(true);
    expect(db.workspaces[0].workspace.settings.ai?.tone).toBe("professional");
  });

  it("drafts a structured article from tickets", () => {
    const ws = db.workspaces[0];
    const t = ws.tickets.find((x) => x.number === 1072)!;
    const d = draftArticle(ws, [t.id], "");
    expect(d.title.length).toBeGreaterThan(5);
    expect(d.body_html).toContain("<h2>");
    expect(d.body_html).toContain("<ol>");
    expect(() => draftArticle(ws, [], "")).toThrow();
  });
});
