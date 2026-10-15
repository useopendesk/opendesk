import { describe, expect, it } from "vitest";
import type { NotificationPreferences } from "@/lib/api/types";
import { installMock } from "../index";
import { dispatch } from "../router";
import { db } from "../db";

async function call(method: string, path: string, body?: unknown) {
  const ws = db.workspaces[0]?.workspace.id;
  const r = await dispatch(`http://mock${path}`, {
    method,
    body: body ? JSON.stringify(body) : undefined,
    headers: ws ? { "OD-Workspace": ws } : {},
  });
  return { status: r.status, body: r.status === 204 ? null : await r.json() };
}

const prefs = async () => (await call("GET", "/v1/me/notification-preferences")).body as NotificationPreferences;
const update = (b: unknown) => call("PATCH", "/v1/me/notification-preferences", b);

describe("notification preferences (mock)", () => {
  installMock();

  it("starts from the contract's defaults, with the AI and billing group for an owner", async () => {
    const p = await prefs();
    expect(p).toMatchObject({ email_cadence: "instant", digest_hour: 9, paused_until: null, delay_minutes: 5 });
    const row = (t: string) => p.types.find((x) => x.type === t)!;
    expect(row("assigned")).toMatchObject({ group: "tickets", email: true, push: true, desktop: true });
    expect(row("team_joined")).toMatchObject({ group: "team", email: false, push: false, desktop: true });
    expect(row("ai_balance_low")).toMatchObject({ group: "ai_billing", email: true, push: false, desktop: true, urgent: false });
    expect(row("mentioned").urgent).toBe(true);
    expect(p.pacing).toEqual({ delay_minutes: 5, cooldown_minutes: [15, 30, 60], quiet_reset_minutes: 120, urgent_gap_minutes: 10, urgent_per_day: 6, per_hour: 4, per_day: 24, active_minutes: 5 });
  });

  it("keeps changes and answers the whole preferences", async () => {
    const p = (await update({ email_cadence: "daily", digest_hour: 7, timezone: "Europe/Budapest", types: [{ type: "mentioned", push: false }] })).body as NotificationPreferences;
    expect(p).toMatchObject({ email_cadence: "daily", digest_hour: 7, timezone: "Europe/Budapest" });
    expect(p.types.find((x) => x.type === "mentioned")).toMatchObject({ email: true, push: false, desktop: true });
    expect((await prefs()).types.find((x) => x.type === "mentioned")!.push).toBe(false);
  });

  it("pauses and resumes, and refuses what it does not know", async () => {
    const until = new Date(Date.now() + 3_600_000).toISOString();
    expect((await update({ paused_until: until })).body.paused_until).toBe(until);
    expect((await update({ paused_until: "" })).body.paused_until).toBeNull();
    expect((await update({ timezone: "Mars/Olympus" })).status).toBe(422);
    expect((await update({ types: [{ type: "nope", email: false }] })).status).toBe(422);
    expect((await update({ paused_until: new Date(Date.now() + 40 * 86_400_000).toISOString() })).status).toBe(422);
  });

  it("sends a test email and unsubscribes from a link", async () => {
    expect((await call("POST", "/v1/me/notification-preferences/test")).status).toBe(204);
    expect((await call("POST", "/v1/notifications/unsubscribe", { token: "expired" })).status).toBe(400);
    const r = await call("POST", "/v1/notifications/unsubscribe", { token: "demo" });
    expect(r.status).toBe(200);
    expect(r.body.workspace.name).toBe(db.workspaces[0].workspace.name);
    expect((await prefs()).types.every((x) => !x.email)).toBe(true);
  });

  it("turns email back on with the choices from before the unsubscribe", async () => {
    await update({ types: [{ type: "team_joined", email: true }, { type: "assigned", email: false }, { type: "mentioned", email: true }] });
    await call("POST", "/v1/notifications/unsubscribe", { token: "demo" });
    // A second link click keeps the first snapshot.
    await call("POST", "/v1/notifications/unsubscribe", { token: "demo" });
    expect((await prefs()).unsubscribed_at).not.toBeNull();
    const p = (await call("POST", "/v1/me/notification-preferences/resubscribe")).body as NotificationPreferences;
    expect(p.unsubscribed_at).toBeNull();
    const email = (t: string) => p.types.find((x) => x.type === t)!.email;
    expect([email("team_joined"), email("assigned"), email("mentioned")]).toEqual([true, false, true]);
  });

  it("ends the unsubscribe when an email is turned on by hand", async () => {
    await call("POST", "/v1/notifications/unsubscribe", { token: "demo" });
    expect((await update({ types: [{ type: "mentioned", email: true }] })).body.unsubscribed_at).toBeNull();
  });
});
