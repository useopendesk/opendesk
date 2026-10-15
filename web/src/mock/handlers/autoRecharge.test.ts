import { afterEach, describe, expect, it, vi } from "vitest";
import type { Billing } from "@/lib/api/types";
import { installMock } from "../index";
import { dispatch } from "../router";
import { db } from "../db";
import { spend } from "../aiBalance";
import { OUT_BELOW_USD, RECHARGE_DELAY, billingOut, saveCard, setRechargeFailure } from "./billing";

async function call(method: string, path: string, body?: unknown) {
  const ws = db.workspaces[0].workspace.id;
  const r = await dispatch(`http://mock${path}`, { method, body: body ? JSON.stringify(body) : undefined, headers: { "OD-Workspace": ws } });
  return { status: r.status, body: await r.json() };
}

const ws = () => db.workspaces[0];
const billing = async () => (await call("GET", "/v1/billing")).body as Billing;

/** Leaves exactly `left` US dollars: the allowance used up and `left` of the top-ups remaining. */
function leave(left: number) {
  const b = billingOut(ws());
  const c = ws().billing.ai;
  // Top-ups left are all top-ups less the overflow past the allowance; work in micro-dollars.
  const micro = (n: number) => Math.round(n * 1e6);
  const total = micro(b.ai.purchased_usd) + Math.max(0, micro(c.used_usd) - micro(c.monthly_usd));
  c.used_usd = (micro(c.monthly_usd) + total - micro(left)) / 1e6;
  expect(billingOut(ws()).ai.remaining_usd).toBe(left);
}

describe("mock auto-recharge", () => {
  installMock();
  afterEach(() => {
    vi.useRealTimers();
    setRechargeFailure(null);
  });

  it("starts off with the defaults and validates changes", async () => {
    const b = await billing();
    expect(b.auto_recharge).toMatchObject({
      enabled: false,
      threshold_usd: 5,
      amount_usd: 25,
      monthly_limit_usd: 250,
      recharges_this_month: 0,
      last_recharge: null,
      retry: null,
      failure: null,
    });
    for (const bad of [{ amount_usd: 4 }, { amount_usd: 5001 }, { threshold_usd: 0 }, { threshold_usd: 1001 }, { monthly_limit_usd: 50_001 }, { enabled: "yes" }]) {
      expect((await call("PATCH", "/v1/billing/auto-recharge", bad)).status, JSON.stringify(bad)).toBe(422);
    }
    const small = await call("PATCH", "/v1/billing/auto-recharge", { threshold_usd: 25 });
    expect([small.status, small.body.code]).toEqual([422, "recharge_too_small"]);
    const limit = await call("PATCH", "/v1/billing/auto-recharge", { amount_usd: 100, monthly_limit_usd: 50 });
    expect([limit.status, limit.body.code]).toEqual([422, "monthly_limit_too_small"]);
    // Rounded to cents.
    const ok = await call("PATCH", "/v1/billing/auto-recharge", { threshold_usd: 2.504 });
    expect(ok.body.auto_recharge.threshold_usd).toBe(2.5);
  });

  it("needs a saved payment method to turn on", async () => {
    saveCard(ws(), false);
    const r = await call("PATCH", "/v1/billing/auto-recharge", { enabled: true });
    expect([r.status, r.body.code]).toEqual([409, "payment_method_required"]);
    const pm = await call("POST", "/v1/billing/payment-method");
    expect(pm.body.url).toBe("/settings/billing?payment_method=success");
    const on = await call("PATCH", "/v1/billing/auto-recharge", { enabled: true, threshold_usd: 5, amount_usd: 20, monthly_limit_usd: 40 });
    expect(on.status).toBe(200);
    expect(on.body.auto_recharge).toMatchObject({ enabled: true, threshold_usd: 5, amount_usd: 20, monthly_limit_usd: 40, low_at_usd: 5 });
  });

  it("recharges below the threshold, up to the monthly limit, then tells admins when the balance runs out", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    leave(5.005);
    spend(ws(), "draft_reply");
    spend(ws(), "summarize"); // one recharge in flight, not two
    await vi.advanceTimersByTimeAsync(RECHARGE_DELAY);
    let b = await billing();
    // $5.005 less two calls (a few cents), plus a $20 recharge.
    expect(b.ai.remaining_usd).toBeGreaterThan(24.9);
    expect(b.ai.remaining_usd).toBeLessThan(25.005);
    expect(b.auto_recharge).toMatchObject({ recharges_this_month: 1, charged_this_month_usd: 20, last_recharge: { usd: 20, amount: 2000, currency: "usd" } });

    leave(5.005);
    spend(ws(), "draft_reply");
    await vi.advanceTimersByTimeAsync(RECHARGE_DELAY);
    expect((await billing()).auto_recharge?.recharges_this_month).toBe(2);

    // At the limit: no third recharge; crossing below $0.01 notifies once.
    const outBefore = ws().notifications.filter((n) => n.type === "ai_balance_out").length;
    leave(OUT_BELOW_USD + 0.005);
    spend(ws(), "draft_reply");
    spend(ws(), "summarize");
    await vi.advanceTimersByTimeAsync(RECHARGE_DELAY);
    b = await billing();
    expect(b.auto_recharge?.recharges_this_month).toBe(2);
    expect(b.ai.remaining_usd).toBeLessThan(OUT_BELOW_USD);
    expect(ws().notifications.filter((n) => n.type === "ai_balance_out").length).toBe(outBefore + 1);
    const out = ws().notifications.find((n) => n.type === "ai_balance_out")?.data.remaining_usd as number;
    expect(out).toBeLessThan(OUT_BELOW_USD);
    expect(Math.round(out * 1e6) / 1e6).toBe(out);
  });

  it("refuses a call the balance does not cover", async () => {
    leave(0.005);
    const r = await call("POST", "/v1/ai/rewrite", { html: "<p>Hi there</p>", mode: "friendlier" });
    expect([r.status, r.body.code]).toEqual([402, "ai_balance_empty"]);
  });

  it("turns itself off when the payment fails", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    await call("PATCH", "/v1/billing/auto-recharge", { monthly_limit_usd: 200 });
    setRechargeFailure("card_declined");
    leave(1);
    spend(ws(), "summarize");
    await vi.advanceTimersByTimeAsync(RECHARGE_DELAY);
    const b = await billing();
    expect(b.auto_recharge?.enabled).toBe(false);
    expect(b.auto_recharge?.failure?.code).toBe("card_declined");
    expect(ws().notifications[0]).toMatchObject({ type: "auto_recharge_failed", data: { reason: "card_declined" } });
    // Turning it back on clears the failure and, still below the threshold, recharges.
    setRechargeFailure(null);
    const on = await call("PATCH", "/v1/billing/auto-recharge", { enabled: true });
    expect(on.body.auto_recharge.failure).toBeNull();
    await vi.advanceTimersByTimeAsync(RECHARGE_DELAY);
    expect((await billing()).auto_recharge?.recharges_this_month).toBe(3);
  });

  it("with auto-recharge off, notices low once and out once", async () => {
    await call("PATCH", "/v1/billing/auto-recharge", { enabled: false, threshold_usd: 2 });
    const lowAt = (await billing()).auto_recharge!.low_at_usd;
    const count = (t: string) => ws().notifications.filter((n) => n.type === t).length;
    const low0 = count("ai_balance_low");
    const out0 = count("ai_balance_out");
    leave(lowAt + 0.005);
    spend(ws(), "draft_reply");
    spend(ws(), "summarize");
    expect(count("ai_balance_low")).toBe(low0 + 1);
    leave(OUT_BELOW_USD + 0.005);
    spend(ws(), "draft_reply");
    expect(count("ai_balance_out")).toBe(out0 + 1);
    // Once a period: crossing again sends nothing more.
    leave(OUT_BELOW_USD + 0.005);
    spend(ws(), "draft_reply");
    expect(count("ai_balance_out")).toBe(out0 + 1);
    expect(count("ai_balance_low")).toBe(low0 + 1);
  });

  it("buys any amount with the saved card, with an invoice", async () => {
    saveCard(ws());
    const before = (await billing()).ai.purchased_usd;
    for (const bad of [4.99, 5000.01, "10"]) expect((await call("POST", "/v1/billing/top-up", { amount_usd: bad })).status).toBe(422);
    const r = await call("POST", "/v1/billing/top-up", { amount_usd: 42.5 });
    expect(r.status).toBeLessThan(300);
    expect(r.body).toMatchObject({ status: "paid", usd: 42.5 });
    expect(r.body.billing.ai.purchased_usd).toBeCloseTo(before + 42.5, 6);
    const inv = (await call("GET", "/v1/billing/invoices")).body.data[0];
    expect(inv).toMatchObject({ kind: "top_up", status: "paid", total: 4250 });
    setRechargeFailure("authentication_required");
    expect((await call("POST", "/v1/billing/top-up", { amount_usd: 10 })).body.code).toBe("authentication_required");
    saveCard(ws(), false);
    expect((await call("POST", "/v1/billing/top-up", { amount_usd: 10 })).body.code).toBe("payment_method_required");
    expect((await billing()).payment_method).toBeNull();
  });
});
