import { describe, expect, it } from "vitest";
import { aggregateUsage } from "./billing";
import { MODELS, meter, typicalUsd } from "../aiBalance";

const sonnet = "anthropic/claude-sonnet-5.5";
const haiku = "anthropic/claude-haiku-5.5";
const rows = [
  { feature: "draft_reply" as const, user_id: "u1", micro_usd: 31_000, model: sonnet, input_tokens: 4478, output_tokens: 482, at: "2026-10-01T09:00:00Z" },
  { feature: "summarize" as const, user_id: "u2", micro_usd: 2_000, model: haiku, input_tokens: 867, output_tokens: 93, at: "2026-10-01T10:00:00Z" },
  { feature: "answer" as const, user_id: null, micro_usd: 27_000, model: sonnet, input_tokens: 3900, output_tokens: 420, at: "2026-10-03T23:59:00Z" },
  { feature: "answer_check" as const, user_id: null, micro_usd: 50, model: "jev", input_tokens: 333, output_tokens: 0, at: "2026-10-03T23:59:30Z" },
  { feature: "summarize" as const, user_id: "u1", micro_usd: 10_000, model: haiku, input_tokens: 10, output_tokens: 10, at: "2026-09-30T23:59:00Z" },
];
const ref = (id: string | null) => (id ? { id, name: id.toUpperCase(), email: `${id}@x.io`, avatar_url: "" } : null);

describe("aggregateUsage", () => {
  it("groups the range by day, feature, member and model, in US dollars that add up", () => {
    const u = aggregateUsage(rows, "2026-10-01", "2026-10-03", ref);
    expect([u.total_usd, u.total_calls]).toEqual([0.06005, 4]);
    expect([u.from, u.to]).toEqual(["2026-10-01T00:00:00.000Z", "2026-10-04T00:00:00.000Z"]);
    // Days without usage are left out, as the server does.
    expect(u.by_day.map((d) => [d.date, d.usd])).toEqual([
      ["2026-10-01", 0.033],
      ["2026-10-03", 0.02705],
    ]);
    expect(u.by_day[0].by_feature).toEqual({ draft_reply: 0.031, summarize: 0.002 });
    expect(u.by_feature[0]).toEqual({ feature: "draft_reply", usd: 0.031, calls: 1 });
    expect(u.by_member.map((m) => [m.user?.id ?? null, m.usd])).toEqual([
      ["u1", 0.031],
      [null, 0.02705],
      ["u2", 0.002],
    ]);
    expect(u.by_model).toEqual([
      { model: sonnet, source: "instance", usd: 0.058, calls: 2, input_tokens: 8378, output_tokens: 902 },
      { model: haiku, source: "instance", usd: 0.002, calls: 1, input_tokens: 867, output_tokens: 93 },
      { model: "jev", source: "instance", usd: 0.00005, calls: 1, input_tokens: 333, output_tokens: 0 },
    ]);
    const sum = (xs: { usd: number }[]) => Math.round(xs.reduce((s, x) => s + x.usd * 1e6, 0)) / 1e6;
    expect(sum(u.by_feature)).toBe(u.total_usd);
    expect(sum(u.by_day)).toBe(u.total_usd);
    expect(sum(u.by_member)).toBe(u.total_usd);
    expect(u.own_key_calls).toBe(0);
  });
});

describe("aggregateUsage on an own key", () => {
  it("keeps own-key calls apart at $0 and counts them", () => {
    const own = [...rows, { feature: "draft_reply" as const, user_id: "u1", micro_usd: 0, model: sonnet, input_tokens: 4000, output_tokens: 400, at: "2026-10-02T09:00:00Z", source: "own_key" as const }];
    const u = aggregateUsage(own, "2026-10-01", "2026-10-03", ref);
    expect([u.total_usd, u.total_calls, u.own_key_calls]).toEqual([0.06005, 5, 1]);
    expect(u.by_model?.find((m) => m.source === "own_key")).toMatchObject({ model: sonnet, usd: 0, calls: 1 });
  });
});

describe("metering", () => {
  it("charges the call's tokens at the model's price, rounded up to the micro-dollar", () => {
    const mid = () => 0.5; // length factor 1.125
    expect(meter("draft_reply", sonnet, mid)).toMatchObject({ input_tokens: 4500, output_tokens: 563, micro_usd: 21_945 });
    expect(meter("draft_reply", haiku, mid).micro_usd).toBe(1_098);
    expect(meter("triage", "jev", mid)).toMatchObject({ micro_usd: 844, output_tokens: 0 });
    // Unknown prices charge the feature's typical cost.
    expect(meter("draft_reply", "some/unknown-model", mid).micro_usd).toBe(30_000);
    const r = meter("answer", sonnet);
    expect(r.micro_usd).toBeGreaterThanOrEqual(7_000);
    expect(r.micro_usd).toBeLessThanOrEqual(40_000);
    expect(Number.isInteger(r.micro_usd)).toBe(true);
    expect(r.input_tokens).toBeGreaterThan(r.output_tokens);
  });

  it("prices models as the workspace pays them, with a typical draft reply", () => {
    const price = (id: string) => MODELS.find((m) => m.id === id);
    expect(price(sonnet)).toMatchObject({ usd_per_mtok: { input: 3, output: 15 }, typical_reply_usd: 0.0195 });
    expect(price(haiku)).toMatchObject({ usd_per_mtok: { input: 0.15, output: 0.75 }, typical_reply_usd: 0.000975 });
    expect(price("anthropic/claude-opus-5.5")?.typical_reply_usd).toBe(0.039);
    expect(price("openai/gpt-5.6-luna")?.typical_reply_usd).toBe(0.0021);
    expect(price("google/gemini-3.8-flash")).toMatchObject({ usd_per_mtok: { input: 1.125, output: 5.625 }, typical_reply_usd: 0.007313 });
    expect(price("deepseek/deepseek-v4.1-flash")?.typical_reply_usd).toBe(0.00135);
    expect(typicalUsd("summarize", sonnet)).toBe(0.0135);
    expect(typicalUsd("kb_draft", "nope")).toBe(0.05);
  });
});
