import { afterEach, describe, expect, it, vi } from "vitest";
import type { AgentReviewItem, AgentTest, AgentTestRun } from "@/lib/api/types";
import { installMock } from "../index";
import { dispatch } from "../router";
import { db } from "../db";
import { STEP_MS } from "./agentQuality";

async function call(method: string, path: string, body?: unknown) {
  const ws = db.workspaces[0]?.workspace.id;
  const r = await dispatch(`http://mock${path}`, { method, body: body ? JSON.stringify(body) : undefined, headers: ws ? { "OD-Workspace": ws } : {} });
  return { status: r.status, body: r.status === 204 ? null : await r.json() };
}

const events = (type: string, since = 0) => db.workspaces[0].events.slice(since).filter((e) => e.type === type);
const tests = async () => (await call("GET", "/v1/ai/agent/tests")).body.data as AgentTest[];

/** Starts a run and lets the simulated worker finish it. */
async function runToEnd(body: Record<string, unknown> = {}): Promise<AgentTestRun> {
  const start = await call("POST", "/v1/ai/agent/test-runs", body);
  expect(start.status).toBe(202);
  await vi.advanceTimersByTimeAsync(300 + STEP_MS * (start.body.total + 2));
  return (await call("GET", `/v1/ai/agent/test-runs/${start.body.id}`)).body;
}

describe("mock agent quality loop", () => {
  installMock();
  afterEach(() => {
    vi.useRealTimers();
  });

  it("seeds tests with results and a newly failing one", async () => {
    const all = await tests();
    expect(all.length).toBeGreaterThanOrEqual(5);
    const refund = all.find((t) => t.name === "Refund timing")!;
    expect(refund.last_result).toMatchObject({ status: "passed", previous_status: "passed", draft: false });
    expect(refund.last_result!.checks.map((c) => c.kind)).toEqual(["mentions", "cites_knowledge", "no_handoff"]);
    const person = all.find((t) => t.name === "Asks for a person")!;
    expect(person.last_result).toMatchObject({ status: "passed", handoff: true });
    const paypal = all.find((t) => t.name === "PayPal")!;
    expect(paypal.last_result).toMatchObject({ status: "failed", previous_status: "passed" });
    const runs = (await call("GET", "/v1/ai/agent/test-runs")).body.data as AgentTestRun[];
    expect(runs).toHaveLength(2);
    expect(runs[0].created_at > runs[1].created_at).toBe(true);
    expect(runs[0].results).toBeUndefined();
  });

  it("validates tests", async () => {
    const bad = [
      { messages: [], expectations: [{ kind: "handoff" }] },
      { messages: [{ role: "agent", text: "Hi" }], expectations: [{ kind: "handoff" }] },
      { messages: Array.from({ length: 7 }, () => ({ role: "visitor", text: "Hi" })), expectations: [{ kind: "handoff" }] },
      { messages: [{ role: "visitor", text: "Hi" }], expectations: [] },
      { messages: [{ role: "visitor", text: "Hi" }], expectations: [{ kind: "mentions", value: "" }] },
      { messages: [{ role: "visitor", text: "Hi" }], expectations: [{ kind: "uses_tool", value: "lookup" }] },
      { messages: [{ role: "visitor", text: "Hi" }], expectations: [{ kind: "handoff" }, { kind: "no_handoff" }] },
      { messages: [{ role: "visitor", text: "Hi" }], expectations: [{ kind: "smiles" }] },
    ];
    for (const b of bad) expect((await call("POST", "/v1/ai/agent/tests", b)).status, JSON.stringify(b).slice(0, 80)).toBe(422);
  });

  it("creates, edits, runs and deletes a test with live events", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const mark = db.workspaces[0].events.length;
    const created = await call("POST", "/v1/ai/agent/tests", { messages: [{ role: "visitor", text: "  Can I talk to a human please?  " }], expectations: [{ kind: "no_handoff" }] });
    expect(created.status).toBe(201);
    const t = created.body as AgentTest;
    expect(t).toMatchObject({ name: "Can I talk to a human please?", last_result: null, visitor: null });
    expect(events("ai_agent_test.created", mark)).toHaveLength(1);

    const edited = await call("PATCH", `/v1/ai/agent/tests/${t.id}`, { name: "Human please", expectations: [{ kind: "handoff" }, { kind: "not_mentions", value: "refund" }], visitor: { email: "ada@example.com" } });
    expect(edited.body).toMatchObject({ name: "Human please", visitor: { email: "ada@example.com", verified: false } });

    const run = await runToEnd({ test_ids: [t.id] });
    expect(run).toMatchObject({ status: "done", total: 1, passed: 1, failed: 0, draft: false });
    expect(run.results![0]).toMatchObject({ status: "passed", handoff: true, previous_status: null });
    expect(events("ai_agent_test_run.created", mark)).toHaveLength(1);
    expect(events("ai_agent_test_result.updated", mark)).toHaveLength(1);
    expect(events("ai_agent_test_run.updated", mark).at(-1)?.data).toMatchObject({ run: { status: "done", passed: 1 } });
    expect((await tests()).find((x) => x.id === t.id)?.last_result?.run_id).toBe(run.id);

    expect((await call("DELETE", `/v1/ai/agent/tests/${t.id}`)).status).toBe(204);
    expect(events("ai_agent_test.deleted", mark)[0].data).toEqual({ test_id: t.id });
    expect((await call("GET", `/v1/ai/agent/tests/${t.id}`)).status).toBe(404);
  });

  it("runs every test against a draft, one run at a time, and stops on request", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    expect((await call("POST", "/v1/ai/agent/test-runs", { settings: { widget_model: "openai/gpt-4" } })).status).toBe(422);
    expect((await call("POST", "/v1/ai/agent/test-runs", { test_ids: ["nope"] })).status).toBe(404);

    const started = await call("POST", "/v1/ai/agent/test-runs", { settings: { bot: { personality: { reply_length: "short" } } } });
    expect(started.body).toMatchObject({ status: "queued", draft: true });
    expect((await call("POST", "/v1/ai/agent/test-runs", {})).status).toBe(409);
    await vi.advanceTimersByTimeAsync(300 + STEP_MS + 50);
    const mid = (await call("GET", `/v1/ai/agent/test-runs/${started.body.id}`)).body as AgentTestRun;
    expect(mid.status).toBe("running");
    expect(mid.results!.filter((r) => r.status === "pending").length).toBeLessThan(mid.total);

    const stopped = await call("POST", `/v1/ai/agent/test-runs/${started.body.id}/cancel`);
    expect(stopped.body.status).toBe("cancelled");
    await vi.advanceTimersByTimeAsync(STEP_MS * 3);
    const after = (await call("GET", `/v1/ai/agent/test-runs/${started.body.id}`)).body as AgentTestRun;
    expect(after.results!.some((r) => r.status === "pending")).toBe(true);
    expect(after.results!.filter((r) => r.status !== "pending").every((r) => r.draft)).toBe(true);

    const full = await runToEnd();
    expect(full.status).toBe("done");
    expect(full.passed + full.failed + full.errored).toBe(full.total);
    expect(full.usd).toBeGreaterThan(0);
  });

  it("lists answers to review, dismisses them and improves them into a snippet and a test", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const queue = (await call("GET", "/v1/ai/agent/review")).body.data as AgentReviewItem[];
    const down = queue.find((x) => x.reason === "rated_down")!;
    expect(down).toMatchObject({ question: expect.stringContaining("before Friday"), answer: expect.stringContaining("can't promise") });
    expect(queue.some((x) => x.reason === "handoff")).toBe(true);

    const other = queue.find((x) => x.id !== down.id)!;
    expect((await call("POST", `/v1/ai/agent/review/${other.id}/dismiss`)).status).toBe(204);
    const human = db.workspaces[0].messages.find((m) => !m.ai)!;
    expect((await call("POST", `/v1/ai/agent/review/${human.id}/dismiss`)).status).toBe(404);

    expect((await call("POST", "/v1/ai/agent/improve", { question: "Hi", answer: "x" })).status).toBe(422);
    expect((await call("POST", "/v1/ai/agent/improve", { question: "Will it come?", answer: "Yes", message_id: human.id })).status).toBe(404);

    const mark = db.workspaces[0].events.length;
    const sources = db.workspaces[0].knowledge.length;
    const improved = await call("POST", "/v1/ai/agent/improve", {
      question: "Can I pay with PayPal?",
      answer: "Yes. **PayPal is accepted at checkout** for every plan, next to cards.",
      message_id: down.id,
      save_test: true,
    });
    expect(improved.status).toBe(201);
    expect(improved.body.source).toMatchObject({ kind: "snippet", name: "Can I pay with PayPal?" });
    expect(improved.body.test).toMatchObject({ messages: [{ role: "visitor", text: "Can I pay with PayPal?" }], expectations: [{ kind: "mentions", value: "Yes. PayPal is accepted at checkout for every plan, next to cards." }] });
    expect(db.workspaces[0].knowledge.length).toBe(sources + 1);
    expect(events("ai_knowledge.created", mark)).toHaveLength(1);
    expect(events("ai_agent_review.updated", mark)[0].data).toEqual({ message_id: down.id, outcome: "improved" });
    expect(db.workspaces[0].audit.some((a) => a.action === "ai.agent_answer_improved")).toBe(true);

    const left = (await call("GET", "/v1/ai/agent/review")).body.data as AgentReviewItem[];
    expect(left.some((x) => x.id === down.id || x.id === other.id)).toBe(false);

    // Ollie answers from the snippet at once: the failing PayPal test now passes.
    const paypal = (await tests()).find((t) => t.name === "PayPal")!;
    const run = await runToEnd({ test_ids: [paypal.id, improved.body.test.id] });
    expect(run.results!.map((r) => r.status)).toEqual(["passed", "passed"]);
    expect(run.results![0]).toMatchObject({ previous_status: "failed" });
  });
});
