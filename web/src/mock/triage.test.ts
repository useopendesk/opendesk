import { describe, expect, it } from "vitest";
import { installMock } from "./index";
import { dispatch } from "./router";
import { db } from "./db";
import { aiDecisions, aiSuggestions } from "@/lib/decisions";

async function call(method: string, path: string, body?: unknown) {
  const ws = db.workspaces[0]?.workspace.id;
  const r = await dispatch(`http://mock${path}`, { method, body: body ? JSON.stringify(body) : undefined, headers: ws ? { "OD-Workspace": ws } : {} });
  return { status: r.status, body: r.status === 204 ? null : await r.json() };
}

describe("seeded triage suggestions", () => {
  installMock();
  const sync = () => db.workspaces[0].tickets.find((t) => t.subject.startsWith("Synchronisierung"))!;

  it("leaves an open ticket with Ollie's pending suggestions to accept or dismiss", () => {
    const t = sync();
    expect(t.status).toBe("open");
    expect(Object.fromEntries(aiSuggestions(t).map(([f, s]) => [f, s.value]))).toEqual({ priority: "high", severity: "sev3", labels: ["Sync", "Mobile"] });
    // Nothing below the threshold was applied.
    expect(t.priority).toBe("normal");
    expect(t.severity).toBeNull();
    expect(t.tags).toEqual([]);
    const d = aiDecisions(t)!;
    for (const [, s] of aiSuggestions(t)) expect(s.confidence!).toBeLessThan(d.threshold);
    expect(d.fields.priority?.applied).toBe(false);
  });

  it("drops a suggestion when it is dismissed", async () => {
    const t = sync();
    expect((await call("DELETE", `/v1/tickets/${t.id}/ai/suggestions/severity`)).status).toBeLessThan(300);
    expect(aiSuggestions(sync()).map(([f]) => f)).toEqual(["priority", "labels"]);
  });
});
