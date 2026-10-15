// Mock billing (contracts/billing.md). Margin is on Pro, Lumen Labs on Free.
// Override every workspace with ?mock_plan=free|pro|past_due|cancel|self_hosted.
//
// Stripe Checkout is simulated: the checkout endpoints return a URL back to
// /settings/billing, and the "webhook" lands a moment later (billing.updated),
// even across the full page load that follows, via sessionStorage.

import type { AIFeature, AutoRecharge, AutoRechargeFailure, Billing, BillingInvoice, BillingUsage, NotificationType, PaymentMethod, UserRef } from "@/lib/api/types";
import { db, userRef, type WsData } from "../db";
import { publish } from "../gateway";
import { HttpError, forbidden, get, patch, post, type Ctx } from "../router";
import { FAST_MODEL, DEFAULT_MODEL, JEV, fromMicro, meter, toMicro } from "../aiBalance";
// Not from ../aiBalance: it imports this module, so its constants aren't ready while this one loads.
import { MAX_TOP_UP_USD, MIN_TOP_UP_USD, OUT_OF_BALANCE_BELOW, TOP_UP_USD } from "@/components/billing/model";
import { DAY, NOW, int, nowIso, pick, rand, uuid } from "../util";

type MockPlan = "free" | "pro" | "past_due" | "cancel" | "self_hosted";

const PRICES: Billing["prices"] = {
  pro_month: { id: "price_mock_pro_month", unit_amount: 1800, currency: "usd", interval: "month" },
  pro_year: { id: "price_mock_pro_year", unit_amount: 19000, currency: "usd", interval: "year" },
  ai_top_up: { id: "price_mock_ai_top_up", unit_amount: 1000, currency: "usd", interval: null, usd: TOP_UP_USD },
};
const FREE_USD = 0.5;
const PRO_USD_PER_SEAT = 10;

/** AI usage ever added, per workspace, in micro-dollars; `ai.purchased_usd` is what's left. */
const purchasedTotal = new WeakMap<WsData, number>();
const seeded = new WeakSet<WsData>();

// ─── Persistence across the reload that follows a checkout ─────────────────

type Persisted = Record<
  string,
  {
    plan?: MockPlan;
    interval?: "month" | "year";
    /** AI usage bought, in US dollars. */
    usd?: number;
    pending?: { plan?: MockPlan; interval?: "month" | "year"; usd?: number };
    /** A saved payment method (top-ups, auto-recharge), and the auto-recharge settings. */
    card?: boolean;
    autoRecharge?: Pick<AutoRecharge, "enabled" | "threshold_usd" | "amount_usd" | "monthly_limit_usd">;
  }
>;
const KEY = "od-mock-billing-amounts";

function load(): Persisted {
  try {
    return JSON.parse(sessionStorage.getItem(KEY) ?? "{}") as Persisted;
  } catch {
    return {};
  }
}
function save(p: Persisted) {
  try {
    sessionStorage.setItem(KEY, JSON.stringify(p));
  } catch {
    /* private mode: the change lasts until reload */
  }
}

// ─── State ─────────────────────────────────────────────────────────────────

function role(ws: WsData) {
  return ws.members.find((m) => m.user_id === db.meId)?.role ?? "viewer";
}
function requireAdmin(ws: WsData) {
  const r = role(ws);
  if (r !== "owner" && r !== "admin") throw forbidden("Only owners and admins can manage billing");
}
function billedSeats(ws: WsData) {
  return ws.members.filter((m) => m.role !== "viewer").length;
}

function applyPlan(ws: WsData, plan: MockPlan, interval: "month" | "year" = "month") {
  const b = ws.billing;
  const c = b.ai;
  if (plan === "self_hosted") {
    Object.assign(b, {
      plan: "self_hosted",
      billing_enabled: false,
      status: "none",
      interval: "month",
      seats: 0,
      current_period_end: null,
      cancel_at_period_end: false,
      limits: { max_mailboxes: 0, max_members: 0 },
      prices: { pro_month: null, pro_year: null, ai_top_up: null },
      auto_recharge: null,
    } satisfies Partial<Billing>);
    Object.assign(c, { unlimited: true, monthly_usd: 0 });
    return;
  }
  b.billing_enabled = true;
  b.prices = structuredClone(PRICES);
  c.unlimited = false;
  if (plan === "free") {
    Object.assign(b, { plan: "free", status: "none", interval: "month", seats: 0, current_period_end: null, cancel_at_period_end: false, limits: { max_mailboxes: 1, max_members: 1 } } satisfies Partial<Billing>);
    c.monthly_usd = FREE_USD;
    return;
  }
  const seats = billedSeats(ws);
  Object.assign(b, {
    plan: "pro",
    status: plan === "past_due" ? "past_due" : "active",
    interval,
    seats,
    current_period_end: c.period_end,
    cancel_at_period_end: plan === "cancel",
    limits: { max_mailboxes: 0, max_members: 0 },
  } satisfies Partial<Billing>);
  c.monthly_usd = PRO_USD_PER_SEAT * seats;
}

function addUsage(ws: WsData, usd: number) {
  purchasedTotal.set(ws, (purchasedTotal.get(ws) ?? 0) + toMicro(usd));
}

/** The mock card on file. */
const CARD: PaymentMethod = { brand: "visa", last4: "4242", exp_month: 8, exp_year: 2030 };

/** Invoices, newest last, per workspace. */
const invoices = new WeakMap<WsData, BillingInvoice[]>();

function addInvoice(ws: WsData, kind: BillingInvoice["kind"], usd: number, at = nowIso()) {
  const rows = invoices.get(ws) ?? [];
  const id = `in_mock_${uuid().slice(0, 8)}`;
  rows.push({ id, number: `OD-${String(1040 + rows.length).padStart(4, "0")}`, created: at, total: Math.round(usd * 100), currency: "usd", status: "paid", kind,
    description: kind === "subscription" ? "Opendesk Pro" : `AI usage ${kind === "auto_recharge" ? "auto-recharge" : "top-up"}, $${usd}`,
    url: `https://invoice.stripe.com/mock/${id}`, pdf_url: `https://invoice.stripe.com/mock/${id}.pdf` });
  invoices.set(ws, rows);
}

/** The Billing as the API returns it: usage counted live, the AI balance derived in micro-dollars. */
export function billingOut(ws: WsData): Billing {
  ensure(ws);
  const b = ws.billing;
  const c = b.ai;
  const monthly = toMicro(c.monthly_usd);
  const used = toMicro(c.used_usd);
  const purchased = Math.max(0, (purchasedTotal.get(ws) ?? 0) - Math.max(0, used - monthly));
  c.used_usd = fromMicro(used);
  c.purchased_usd = fromMicro(purchased);
  c.remaining_usd = c.unlimited ? 0 : fromMicro(Math.max(0, monthly - used) + purchased);
  const members = ws.members.length + (b.plan === "free" ? ws.invitations.length : 0);
  b.auto_recharge = autoRechargeOut(ws);
  b.payment_method = b.billing_enabled && cards.has(ws) ? { ...CARD } : null;
  return structuredClone({
    ...b,
    billed_seats: billedSeats(ws),
    usage: { mailboxes: ws.mailboxes.length, members },
  });
}

/** 402 plan_limit when creating one more `kind` would pass the plan's cap. */
export function checkLimit(ws: WsData, kind: "mailbox" | "member") {
  const b = billingOut(ws);
  if (kind === "mailbox" && b.limits.max_mailboxes > 0 && b.usage.mailboxes >= b.limits.max_mailboxes) {
    throw new HttpError(402, "plan_limit", `The Free plan includes ${b.limits.max_mailboxes} mailbox. Upgrade to Pro for unlimited mailboxes.`, { limit: "mailboxes" });
  }
  if (kind === "member" && b.limits.max_members > 0 && b.usage.members >= b.limits.max_members) {
    throw new HttpError(402, "plan_limit", `The Free plan includes ${b.limits.max_members} member. Upgrade to Pro to invite your team.`, { limit: "members" });
  }
}

function changed(ws: WsData) {
  publish(ws, "billing.updated", billingOut(ws) as unknown as Record<string, unknown>);
}

// ─── Seed ──────────────────────────────────────────────────────────────────

const WEIGHTS: [AIFeature, number][] = [
  ["summarize", 30],
  ["draft_reply", 22],
  ["rewrite", 16],
  ["copilot", 10],
  ["translate", 9],
  ["triage", 8],
  ["answer_check", 8],
  ["answer", 6],
  ["assistant", 5],
  ["decide", 2],
  ["playground", 1],
  ["kb_draft", 1],
];

/** The model each feature ran on in the seeded history. */
function seedModel(f: AIFeature): string {
  if (f === "triage" || f === "decide" || f === "answer_check") return JEV;
  if (f === "rewrite" || f === "translate") return FAST_MODEL;
  return DEFAULT_MODEL;
}

/** A seeded usage row: metered like a real call, deterministic. */
function seedRow(f: AIFeature, user_id: string | null, at: number): WsData["aiUsage"][number] {
  return { ...meter(f, seedModel(f), rand), user_id, at: new Date(at).toISOString() };
}

/** The widget agent and automations act for nobody in particular. */
const unattended = (f: AIFeature) => f === "answer" || f === "answer_check" || f === "decide";
function feature(): AIFeature {
  let r = rand() * WEIGHTS.reduce((s, [, w]) => s + w, 0);
  for (const [f, w] of WEIGHTS) if ((r -= w) < 0) return f;
  return "summarize";
}

/** About `perDay` calls a day for 60 days, fewer on weekends, from the team. */
function seedUsage(ws: WsData, perDay: number) {
  const users = ws.members.filter((m) => m.role !== "viewer").map((m) => m.user_id);
  for (let d = 59; d >= 0; d--) {
    const dayStart = NOW - d * DAY;
    const dow = new Date(dayStart).getUTCDay();
    const n = Math.max(0, Math.round(perDay * (dow === 0 || dow === 6 ? 0.3 : 0.7 + rand() * 0.6)));
    for (let i = 0; i < n; i++) {
      const f = feature();
      const at = dayStart - int(0, 20) * 3_600_000;
      if (at > NOW) continue;
      ws.aiUsage.push(seedRow(f, unattended(f) ? null : pick(users), at));
    }
  }
}

function periodUsed(ws: WsData) {
  const end = Date.parse(ws.billing.ai.period_end);
  const start = end - 30 * DAY;
  const micro = ws.aiUsage
    .filter((u) => {
      const t = Date.parse(u.at);
      return t >= start && t < end;
    })
    .reduce((s, u) => s + u.micro_usd, 0);
  return fromMicro(micro);
}

function seedWorkspace(ws: WsData, plan: MockPlan) {
  const c = ws.billing.ai;
  if (plan === "free") {
    c.period_end = new Date(NOW + 12 * DAY).toISOString();
    applyPlan(ws, "free");
    // Free history is light; the current period sits at about $0.38 of $0.50.
    seedUsage(ws, 0.6);
    const start = Date.parse(c.period_end) - 30 * DAY;
    ws.aiUsage = ws.aiUsage.filter((u) => Date.parse(u.at) < start);
    const users = ws.members.map((m) => m.user_id);
    let left = toMicro(0.38);
    while (left > toMicro(0.03)) {
      const f = rand() < 0.3 ? "draft_reply" : pick<AIFeature>(["summarize", "rewrite", "translate", "copilot"]);
      const at = start + rand() * (NOW - start);
      const row = seedRow(f, pick(users), at);
      ws.aiUsage.push(row);
      left -= row.micro_usd;
    }
  } else {
    c.period_end = new Date(NOW + 18 * DAY).toISOString();
    applyPlan(ws, plan);
    seedUsage(ws, 3.2 * Math.max(1, billedSeats(ws)));
    addUsage(ws, 10);
    // Three months of Pro and the top-up, as Stripe would list them.
    for (const m of [2, 1, 0]) addInvoice(ws, "subscription", (PRICES.pro_month!.unit_amount / 100) * Math.max(1, billedSeats(ws)), new Date(NOW - (m * 30 + 12) * DAY).toISOString());
    addInvoice(ws, "top_up", 10, new Date(NOW - 9 * DAY).toISOString());
  }
  ws.aiUsage.sort((a, b) => a.at.localeCompare(b.at));
  c.used_usd = periodUsed(ws);
}

/** Workspaces created after install (onboarding) start on Free. */
function ensure(ws: WsData) {
  if (seeded.has(ws)) return;
  seeded.add(ws);
  ws.billing.ai.period_end = new Date(NOW + 30 * DAY).toISOString();
  applyPlan(ws, "free");
  ws.billing.ai.used_usd = 0;
}

// ─── Usage aggregation ─────────────────────────────────────────────────────

export function aggregateUsage(rows: WsData["aiUsage"], from: string, to: string, ref: (id: string | null) => UserRef | null): BillingUsage {
  const start = Date.parse(`${from}T00:00:00Z`);
  const end = Date.parse(`${to}T00:00:00Z`) + DAY;
  const inRange = rows.filter((r) => {
    const t = Date.parse(r.at);
    return t >= start && t < end;
  });
  // Sums are kept in micro-dollars, so every total equals the sum of its parts.
  type Sum = { micro: number; calls: number };
  const days = new Map<string, Sum & { by_feature: Partial<Record<AIFeature, number>> }>();
  const feats = new Map<AIFeature, Sum>();
  const members = new Map<string, Sum>();
  const models = new Map<string, Sum & { model: string; source: "instance" | "own_key"; input_tokens: number; output_tokens: number }>();
  for (const r of inRange) {
    const dayKey = r.at.slice(0, 10);
    const day = days.get(dayKey) ?? { micro: 0, calls: 0, by_feature: {} };
    day.micro += r.micro_usd;
    day.calls += 1;
    day.by_feature[r.feature] = (day.by_feature[r.feature] ?? 0) + r.micro_usd;
    days.set(dayKey, day);
    const f = feats.get(r.feature) ?? { micro: 0, calls: 0 };
    f.micro += r.micro_usd;
    f.calls += 1;
    feats.set(r.feature, f);
    const key = r.user_id ?? "";
    const m = members.get(key) ?? { micro: 0, calls: 0 };
    m.micro += r.micro_usd;
    m.calls += 1;
    members.set(key, m);
    const source = r.source ?? "instance";
    const mo = models.get(`${source}:${r.model}`) ?? { model: r.model, source, micro: 0, calls: 0, input_tokens: 0, output_tokens: 0 };
    mo.micro += r.micro_usd;
    mo.calls += 1;
    mo.input_tokens += r.input_tokens;
    mo.output_tokens += r.output_tokens;
    models.set(`${source}:${r.model}`, mo);
  }
  const usd = <T extends Sum>({ micro, ...v }: T) => ({ ...v, usd: fromMicro(micro) });
  return {
    from: new Date(start).toISOString(),
    to: new Date(end).toISOString(),
    total_usd: fromMicro(inRange.reduce((s, r) => s + r.micro_usd, 0)),
    total_calls: inRange.length,
    by_feature: [...feats].map(([feature, v]) => ({ feature, ...usd(v) })).sort((a, b) => b.usd - a.usd),
    by_day: [...days]
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([date, { by_feature, ...v }]) => ({
        date,
        ...usd(v),
        by_feature: Object.fromEntries(Object.entries(by_feature).map(([k, n]) => [k, fromMicro(n)])) as Partial<Record<AIFeature, number>>,
      })),
    by_member: [...members].map(([id, v]) => ({ user: id ? ref(id) : null, ...usd(v) })).sort((a, b) => b.usd - a.usd),
    by_model: [...models.values()].map((v) => usd(v)).sort((a, b) => b.usd - a.usd || b.calls - a.calls),
    own_key_calls: inRange.filter((r) => r.source === "own_key").length,
  };
}

// ─── Routes ────────────────────────────────────────────────────────────────

function requireStripe(ws: WsData) {
  if (!billingOut(ws).billing_enabled) throw new HttpError(404, "billing_disabled", "Billing isn't available on this instance.");
}

function slugOf(ctx: Ctx) {
  return ctx.ws.workspace.slug;
}

function settle(ws: WsData) {
  const p = load();
  const entry = p[ws.workspace.slug];
  if (!entry?.pending) return;
  const { plan, interval, usd } = entry.pending;
  if (plan) {
    applyPlan(ws, plan, interval);
    entry.plan = plan;
    entry.interval = interval;
  }
  if (usd) {
    addUsage(ws, usd);
    addInvoice(ws, "top_up", usd);
    entry.usd = (entry.usd ?? 0) + usd;
    notified.delete(ws);
  }
  delete entry.pending;
  save(p);
  changed(ws);
}

/** The "Stripe webhook" arrives a moment after the customer is back. */
function scheduleSettle(ws: WsData, ms = 1600) {
  setTimeout(() => settle(ws), ms);
}

export function registerBilling() {
  const params = new URLSearchParams(location.search);
  const forced = params.get("mock_plan") as MockPlan | null;
  // ?mock_balance=low|out shows the running-low banner; ?mock_recharge=decline|auth makes recharges fail.
  const forcedBalance = params.get("mock_balance");
  const failMode = params.get("mock_recharge");
  rechargeFailure = failMode === "decline" ? "card_declined" : failMode === "auth" ? "authentication_required" : null;
  const persisted = load();

  db.workspaces.forEach((ws, i) => {
    seeded.add(ws);
    seedWorkspace(ws, i === 0 ? "pro" : "free");
    const entry = persisted[ws.workspace.slug];
    if (entry?.plan) applyPlan(ws, entry.plan, entry.interval);
    if (entry?.usd) addUsage(ws, entry.usd);
    if (forced && ["free", "pro", "past_due", "cancel", "self_hosted"].includes(forced)) {
      applyPlan(ws, forced);
      // A forced Free reads like a real Free workspace: a light month, no top-ups.
      if (forced === "free") {
        ws.billing.ai.used_usd = 0.38;
        purchasedTotal.set(ws, 0);
      }
    }
    if (entry?.pending) scheduleSettle(ws);
    // Margin has a card on file from its Pro checkout.
    if (i === 0 || entry?.card) cards.add(ws);
    if (entry?.autoRecharge && ws.billing.billing_enabled) Object.assign(autoRechargeOut(ws) ?? {}, entry.autoRecharge);
    if (forcedBalance === "low" || forcedBalance === "out") forceBalance(ws, forcedBalance);
  });
  registerAutoRecharge();

  get("/v1/billing", (ctx) => billingOut(ctx.ws));

  post("/v1/billing/checkout", (ctx) => {
    requireStripe(ctx.ws);
    requireAdmin(ctx.ws);
    if (ctx.ws.billing.plan === "pro" && ctx.ws.billing.status !== "canceled") throw new HttpError(409, "conflict", "This workspace is already on Pro. Manage it in the billing portal.");
    const interval = ctx.body.interval === "year" ? "year" : "month";
    const p = load();
    p[slugOf(ctx)] = { ...p[slugOf(ctx)], pending: { plan: "pro", interval } };
    save(p);
    scheduleSettle(ctx.ws);
    return { url: "/settings/billing?checkout=success" };
  });

  post("/v1/billing/portal", (ctx) => {
    requireStripe(ctx.ws);
    requireAdmin(ctx.ws);
    if (ctx.ws.billing.plan !== "pro") throw new HttpError(409, "conflict", "There's no subscription to manage yet.");
    return { url: "/settings/billing" };
  });

  post("/v1/billing/top-up/checkout", (ctx) => {
    requireStripe(ctx.ws);
    requireAdmin(ctx.ws);
    if (!ctx.ws.billing.prices.ai_top_up) throw new HttpError(422, "top_up_unavailable", "AI top-ups are not available.");
    const usd = topUpAmount(ctx.body.amount_usd ?? (typeof ctx.body.top_ups === "number" ? ctx.body.top_ups * TOP_UP_USD : undefined));
    const p = load();
    const entry = (p[slugOf(ctx)] ??= {});
    // Checkout saves the card for later.
    entry.card = true;
    cards.add(ctx.ws);
    entry.pending = { ...entry.pending, usd: (entry.pending?.usd ?? 0) + usd };
    save(p);
    scheduleSettle(ctx.ws);
    return { url: "/settings/billing?top_up=success" };
  });

  // Paid with the saved card, in place.
  post("/v1/billing/top-up", (ctx) => {
    requireStripe(ctx.ws);
    requireAdmin(ctx.ws);
    if (!ctx.ws.billing.prices.ai_top_up) throw new HttpError(422, "top_up_unavailable", "AI top-ups are not available.");
    const usd = topUpAmount(ctx.body.amount_usd);
    if (!cards.has(ctx.ws)) throw new HttpError(409, "payment_method_required", "Add a payment method first.");
    if (rechargeFailure === "authentication_required")
      throw new HttpError(402, "authentication_required", "Your bank wants to confirm this payment. Continue in Stripe Checkout to confirm it.");
    if (rechargeFailure) throw new HttpError(402, "card_declined", "The card was declined, so nothing was charged. Try another card.");
    addUsage(ctx.ws, usd);
    addInvoice(ctx.ws, "top_up", usd);
    const p = load();
    const entry = (p[slugOf(ctx)] ??= {});
    entry.usd = (entry.usd ?? 0) + usd;
    save(p);
    notified.delete(ctx.ws);
    changed(ctx.ws);
    return { status: "paid", usd, billing: billingOut(ctx.ws) };
  });

  get("/v1/billing/invoices", (ctx) => {
    requireStripe(ctx.ws);
    requireAdmin(ctx.ws);
    return { data: [...(invoices.get(ctx.ws) ?? [])].reverse().slice(0, 24) };
  });

  get("/v1/billing/usage", (ctx) => {
    requireStripe(ctx.ws);
    requireAdmin(ctx.ws);
    const today = new Date().toISOString().slice(0, 10);
    const from = ctx.query.get("from") ?? new Date(Date.now() - 29 * DAY).toISOString().slice(0, 10);
    const to = ctx.query.get("to") ?? today;
    if (!/^\d{4}-\d{2}-\d{2}$/.test(from) || !/^\d{4}-\d{2}-\d{2}$/.test(to) || from > to) {
      throw new HttpError(422, "validation_failed", "from and to must be YYYY-MM-DD, from before to");
    }
    return aggregateUsage(ctx.ws.aiUsage, from, to, userRef);
  });
}


/** A purchase or recharge amount: $5 to $5,000, rounded to cents. */
function topUpAmount(v: unknown): number {
  if (typeof v !== "number" || !Number.isFinite(v)) throw new HttpError(422, "validation_failed", "amount_usd is required");
  const usd = Math.round(v * 100) / 100;
  if (usd < MIN_TOP_UP_USD || usd > MAX_TOP_UP_USD) throw new HttpError(422, "validation_failed", "amount_usd must be from 5 to 5000");
  return usd;
}

// ─── Auto-recharge (contracts/billing.md "Auto-recharge") ──────────────────

/** Below this balance no call can start: Ollie is paused. */
export const OUT_BELOW_USD = OUT_OF_BALANCE_BELOW;
/** How long the simulated off-session payment takes. */
export const RECHARGE_DELAY = 900;

type RechargeRow = NonNullable<AutoRecharge["last_recharge"]>;
const recharges = new WeakMap<WsData, RechargeRow[]>();
const cards = new WeakSet<WsData>();
const inFlight = new WeakSet<WsData>();
/** Running-low notices already sent this period (period_end), per workspace. */
const notified = new WeakMap<WsData, { low?: string; out?: string }>();
let rechargeFailure: AutoRechargeFailure | null = null;

/** Tests: make the next recharges fail with `code`, or succeed with null. */
export function setRechargeFailure(code: AutoRechargeFailure | null) {
  rechargeFailure = code;
}
/** Tests and the setup Checkout: the workspace has a saved card. */
export function saveCard(ws: WsData, saved = true) {
  if (saved) cards.add(ws);
  else cards.delete(ws);
}

function defaultAutoRecharge(): AutoRecharge {
  return {
    enabled: false,
    threshold_usd: 5,
    amount_usd: 25,
    monthly_limit_usd: 250,
    charged_this_month_usd: 0,
    recharges_this_month: 0,
    low_at_usd: 0.1,
    last_recharge: null,
    retry: null,
    failure: null,
  };
}

/** Running low: the threshold, but never more than a fifth of the monthly allowance (at least $0.05); with auto-recharge on, the threshold. */
function lowAt(ar: AutoRecharge, monthlyUsd: number): number {
  if (ar.enabled) return ar.threshold_usd;
  return fromMicro(Math.max(toMicro(0.05), Math.min(toMicro(ar.threshold_usd), Math.floor(toMicro(monthlyUsd) / 5))));
}

/** The live auto-recharge state: counts and low_at_usd derived; null when billing is off. */
function autoRechargeOut(ws: WsData): AutoRecharge | null {
  const b = ws.billing;
  if (!b.billing_enabled) return null;
  const ar = (b.auto_recharge ??= defaultAutoRecharge());
  const month = new Date().toISOString().slice(0, 7);
  const rows = recharges.get(ws) ?? [];
  const thisMonth = rows.filter((r) => r.at.startsWith(month));
  ar.recharges_this_month = thisMonth.length;
  ar.charged_this_month_usd = fromMicro(thisMonth.reduce((s, r) => s + toMicro(r.usd), 0));
  ar.last_recharge = rows.at(-1) ?? null;
  ar.low_at_usd = lowAt(ar, b.ai.monthly_usd);
  if (ar.enabled) ar.failure = null;
  return ar;
}

function persistAutoRecharge(ws: WsData, ar: AutoRecharge) {
  const p = load();
  const entry = (p[ws.workspace.slug] ??= {});
  entry.autoRecharge = { enabled: ar.enabled, threshold_usd: ar.threshold_usd, amount_usd: ar.amount_usd, monthly_limit_usd: ar.monthly_limit_usd };
  save(p);
}

function isAdmin(ws: WsData) {
  const r = role(ws);
  return r === "owner" || r === "admin";
}

function notify(ws: WsData, type: NotificationType, data: Record<string, unknown>) {
  if (!isAdmin(ws)) return;
  const n = { id: uuid(), type, ticket: null, data, read_at: null, created_at: nowIso() };
  ws.notifications.unshift(n);
  publish(ws, "notification.created", { notification: n }, { topics: [`user:${db.meId}`] });
}

/** A running-low or out notice, once a period; a grant re-arms both. */
function noticeOnce(ws: WsData, kind: "low" | "out", remainingUsd: number) {
  const period = ws.billing.ai.period_end;
  const sent = notified.get(ws) ?? {};
  if (sent[kind] === period) return;
  notified.set(ws, { ...sent, [kind]: period });
  notify(ws, kind === "low" ? "ai_balance_low" : "ai_balance_out", { remaining_usd: remainingUsd });
  changed(ws);
}

/**
 * After a charged call (aiBalance.ts spend): queue a recharge below the threshold
 * when auto-recharge is on with room left under its monthly limit; otherwise tell
 * admins when the balance crosses below low_at_usd, and again when it runs out.
 */
export function afterSpend(ws: WsData, before: number, after: number) {
  billingOut(ws);
  const ar = ws.billing.auto_recharge;
  if (!ar || ws.billing.ai.unlimited) return;
  if (ar.enabled && after < ar.threshold_usd && roomLeft(ar) >= MIN_TOP_UP_USD) {
    scheduleRecharge(ws);
    return;
  }
  if (before >= OUT_BELOW_USD && after < OUT_BELOW_USD) noticeOnce(ws, "out", after);
  else if (before >= ar.low_at_usd && after < ar.low_at_usd && after >= OUT_BELOW_USD) noticeOnce(ws, "low", after);
}

function scheduleRecharge(ws: WsData) {
  if (inFlight.has(ws)) return;
  inFlight.add(ws);
  setTimeout(() => {
    inFlight.delete(ws);
    recharge(ws);
  }, RECHARGE_DELAY);
}

/** What the monthly limit has left, in US dollars. */
const roomLeft = (ar: AutoRecharge) => fromMicro(toMicro(ar.monthly_limit_usd) - toMicro(ar.charged_this_month_usd));

/** The simulated off-session invoice: AI usage granted, or auto-recharge off on failure. */
function recharge(ws: WsData) {
  const b = billingOut(ws);
  const ar = ws.billing.auto_recharge;
  const topUp = b.prices.ai_top_up;
  // Near the monthly limit a recharge adds only what the limit has left.
  const usd = ar ? Math.min(ar.amount_usd, roomLeft(ar)) : 0;
  if (!ar?.enabled || !topUp || b.ai.remaining_usd >= ar.threshold_usd || usd < MIN_TOP_UP_USD) return;
  const failure: AutoRechargeFailure | null = !cards.has(ws) ? "no_payment_method" : rechargeFailure;
  if (failure) {
    ar.enabled = false;
    ar.failure = { code: failure, at: nowIso() };
    persistAutoRecharge(ws, ar);
    notify(ws, "auto_recharge_failed", { reason: failure });
    changed(ws);
    return;
  }
  addUsage(ws, usd);
  addInvoice(ws, "auto_recharge", usd);
  const rows = recharges.get(ws) ?? [];
  rows.push({ at: nowIso(), usd, amount: Math.round((usd * topUp.unit_amount) / (topUp.usd ?? TOP_UP_USD)), currency: topUp.currency });
  recharges.set(ws, rows);
  notified.delete(ws);
  changed(ws);
}

/** ?mock_balance=low|out: a balance that shows the banner. */
function forceBalance(ws: WsData, kind: "low" | "out") {
  const ar = autoRechargeOut(ws);
  if (!ar) return;
  purchasedTotal.set(ws, 0);
  const left = kind === "out" ? toMicro(0.004) : Math.max(toMicro(OUT_BELOW_USD) * 2, Math.round(toMicro(ar.low_at_usd) * 0.6));
  ws.billing.ai.used_usd = fromMicro(Math.max(0, toMicro(ws.billing.ai.monthly_usd) - left));
}


function registerAutoRecharge() {
  patch("/v1/billing/auto-recharge", (ctx) => {
    requireStripe(ctx.ws);
    requireAdmin(ctx.ws);
    const body = (ctx.body ?? {}) as Record<string, unknown>;
    if (body.enabled !== undefined && typeof body.enabled !== "boolean") throw new HttpError(422, "validation_failed", "enabled must be true or false");
    if (body.threshold_usd !== undefined && (typeof body.threshold_usd !== "number" || !Number.isFinite(body.threshold_usd) || body.threshold_usd < 0.01 || body.threshold_usd > 1000))
      throw new HttpError(422, "validation_failed", "threshold_usd must be from $0.01 to $1,000");
    const money = (v: unknown, lo: number, hi: number) => typeof v === "number" && Number.isFinite(v) && v >= lo && v <= hi;
    if (body.amount_usd !== undefined && !money(body.amount_usd, 5, 5000)) throw new HttpError(422, "validation_failed", "amount_usd must be from 5 to 5000");
    if (body.monthly_limit_usd !== undefined && !money(body.monthly_limit_usd, 5, 50_000))
      throw new HttpError(422, "validation_failed", "monthly_limit_usd must be from 5 to 50000");
    billingOut(ctx.ws);
    const ar = ctx.ws.billing.auto_recharge!;
    // Rounded to cents.
    const cents = (v: unknown, cur: number) => (typeof v === "number" ? Math.round(v * 100) / 100 : cur);
    const threshold = cents(body.threshold_usd, ar.threshold_usd);
    const amount = cents(body.amount_usd, ar.amount_usd);
    const limit = cents(body.monthly_limit_usd, ar.monthly_limit_usd);
    if (toMicro(amount) <= toMicro(threshold))
      throw new HttpError(422, "recharge_too_small", "Each recharge must bring your AI balance back above the threshold. Recharge more or lower the threshold.");
    if (limit < amount) throw new HttpError(422, "monthly_limit_too_small", "The monthly limit must cover at least one recharge.");
    if (body.enabled === true && !ar.enabled) {
      if (!ctx.ws.billing.prices.ai_top_up) throw new HttpError(422, "top_up_unavailable", "AI top-ups are not available.");
      if (!cards.has(ctx.ws)) throw new HttpError(409, "payment_method_required", "Add a payment method first.");
      ar.failure = null;
    }
    if (typeof body.enabled === "boolean") ar.enabled = body.enabled;
    ar.threshold_usd = threshold;
    ar.amount_usd = amount;
    ar.monthly_limit_usd = limit;
    persistAutoRecharge(ctx.ws, ar);
    const out = billingOut(ctx.ws);
    changed(ctx.ws);
    // Already below the threshold: recharge now.
    if (ar.enabled && out.ai.remaining_usd < ar.threshold_usd && roomLeft(ar) >= MIN_TOP_UP_USD) scheduleRecharge(ctx.ws);
    return out;
  });

  // Stripe Checkout in setup mode: the card is attached before the customer is back.
  post("/v1/billing/payment-method", (ctx) => {
    requireStripe(ctx.ws);
    requireAdmin(ctx.ws);
    cards.add(ctx.ws);
    const p = load();
    (p[slugOf(ctx)] ??= {}).card = true;
    save(p);
    return { url: "/settings/billing?payment_method=success" };
  });
}
