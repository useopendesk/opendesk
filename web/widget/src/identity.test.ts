import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Controller } from "./controller";
import { expiresSoon, RENEW_BEFORE_MS, tokenExpiry } from "./identity";
import { createMock, demoConfig } from "./mock";

const b64url = (s: string) => btoa(s).replace(/=+$/, "").replace(/\+/g, "-").replace(/\//g, "_");

/** A user_token as the host's server would sign it; the widget reads it, the API checks the signature. */
const token = (claims: Record<string, unknown>) => `${b64url(JSON.stringify({ alg: "HS256", typ: "JWT" }))}.${b64url(JSON.stringify(claims))}.c2ln`;

const seconds = () => Math.floor(Date.now() / 1000);

describe("reading a user_token", () => {
  it("reads exp in ms, and nothing from what is not a JWT with one", () => {
    expect(tokenExpiry(token({ user_id: "42", exp: 1_800_000_000 }))).toBe(1_800_000_000_000);
    expect(tokenExpiry(token({ user_id: "42" }))).toBeNull();
    expect(tokenExpiry(token({ user_id: "42", exp: "soon" }))).toBeNull();
    expect(tokenExpiry("not a token")).toBeNull();
    expect(tokenExpiry("a.%%%.b")).toBeNull();
  });

  it("is due for renewal within a minute of exp, and past it", () => {
    const now = 1_800_000_000_000;
    const at = (ms: number) => token({ user_id: "42", exp: (now + ms) / 1000 });
    expect(expiresSoon(at(RENEW_BEFORE_MS + 1000), now)).toBe(false);
    expect(expiresSoon(at(RENEW_BEFORE_MS), now)).toBe(true);
    expect(expiresSoon(at(-1000), now)).toBe(true);
    // Without exp there is nothing to renew ahead of; the API decides.
    expect(expiresSoon(token({ user_id: "42" }), now)).toBe(false);
  });
});

describe("a signed-in user with a user_token", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-11-11T10:00:00Z"));
  });
  afterEach(() => vi.useRealTimers());

  type Sent = Record<string, unknown>;

  /**
   * The mock API, keeping each session request's body and answering `user_token_expired` for a
   * token whose exp has passed or that the test refuses (a server clock ahead of the page's).
   */
  function tokenApi(refused: Set<string> = new Set()) {
    const mock = createMock({ seed: "demo", latency: 5 });
    const sent: Sent[] = [];
    const fetcher = async (url: string, init: RequestInit) => {
      if (url.endsWith("/v1/widget/session") && init.method === "POST") {
        const body = JSON.parse(String(init.body)) as Sent;
        sent.push(body);
        const t = typeof body.user_token === "string" ? body.user_token : null;
        const exp = t ? tokenExpiry(t) : null;
        if (t && (refused.has(t) || (exp !== null && exp <= Date.now()))) {
          return new Response(JSON.stringify({ status: 401, code: "user_token_expired", detail: "The user_token has expired." }), {
            status: 401,
            headers: { "content-type": "application/json" },
          });
        }
      }
      return mock.fetch(url, init);
    };
    const c = new Controller({ base: "https://mock.invalid", key: "wk_token", fetcher, socketFactory: mock.socket, persist: false });
    return { c, sent, mock };
  }

  async function boot(c: Controller) {
    const b = c.boot(demoConfig("wk_token"));
    await vi.advanceTimersByTimeAsync(200);
    await b;
  }

  it("renews a token about to expire before the session request", async () => {
    const { c, sent, mock } = tokenApi();
    const fresh = token({ user_id: "42", exp: seconds() + 86_400 });
    const renew = vi.fn(async () => fresh);
    void c.identify({ user_token: token({ user_id: "42", exp: seconds() + 30 }), refresh_user_token: renew, email: "ada@example.com" });
    await boot(c);

    expect(renew).toHaveBeenCalledTimes(1);
    expect(sent).toHaveLength(1);
    expect(sent[0].user_token).toBe(fresh);
    expect(sent[0].email).toBe("ada@example.com");
    expect("refresh_user_token" in sent[0]).toBe(false);
    expect(c.store.get().error).toBeNull();
    c.stop();
    mock.dispose();
  });

  it("gets the first token from refresh_user_token when the page gives none", async () => {
    const { c, sent, mock } = tokenApi();
    const fresh = token({ user_id: "42", exp: seconds() + 3600 });
    void c.identify({ refresh_user_token: () => fresh });
    await boot(c);

    expect(sent.map((b) => b.user_token)).toEqual([fresh]);
    c.stop();
    mock.dispose();
  });

  it("renews once and tries again when the API says the token expired, keeping the visitor token", async () => {
    const first = token({ user_id: "42", exp: seconds() + 3600 });
    const refused = new Set<string>();
    const { c, sent, mock } = tokenApi(refused);
    const second = token({ user_id: "42", exp: seconds() + 7200 });
    const renew = vi.fn(() => Promise.resolve(second));
    void c.identify({ user_token: first, refresh_user_token: renew });
    await boot(c);
    expect(renew).not.toHaveBeenCalled();
    expect(c.api.visitorToken).toBe("od_vis_mock");

    // The API's clock says the first token is done; the page's does not.
    refused.add(first);
    const update = c.identify({ fields: { widget_plan: "pro" } });
    await vi.advanceTimersByTimeAsync(200);
    await update;

    expect(renew).toHaveBeenCalledTimes(1);
    expect(sent.slice(1).map((b) => [b.user_token, b.visitor_token])).toEqual([
      [first, "od_vis_mock"],
      [second, "od_vis_mock"],
    ]);
    expect(sent[2].fields).toEqual({ widget_plan: "pro" });
    expect(c.store.get().error).toBeNull();
    c.stop();
    mock.dispose();
  });

  it("without refresh_user_token, says so and does not drop the visitor token", async () => {
    const first = token({ user_id: "42", exp: seconds() + 3600 });
    const refused = new Set<string>();
    const { c, sent, mock } = tokenApi(refused);
    void c.identify({ user_token: first });
    await boot(c);

    refused.add(first);
    const update = c.identify({ fields: { widget_plan: "pro" } });
    await vi.advanceTimersByTimeAsync(200);
    await update;

    expect(sent).toHaveLength(2);
    expect(c.api.visitorToken).toBe("od_vis_mock");
    expect(c.store.get().error).toBe("The user_token has expired.");
    c.stop();
    mock.dispose();
  });

  it("asks a failed renewal only once per request, and reports the refusal", async () => {
    const refused = new Set<string>();
    const first = token({ user_id: "42", exp: seconds() + 3600 });
    const { c, sent, mock } = tokenApi(refused);
    const renew = vi.fn(() => Promise.reject(new Error("signed out")));
    void c.identify({ user_token: first, refresh_user_token: renew });
    await boot(c);

    refused.add(first);
    const update = c.identify({ name: "Ada" });
    await vi.advanceTimersByTimeAsync(200);
    await update;

    expect(renew).toHaveBeenCalledTimes(1);
    expect(sent).toHaveLength(2);
    expect(c.store.get().error).toBe("The user_token has expired.");
    c.stop();
    mock.dispose();
  });

  it("drops the token for a user_hash, and the previous user's renewal for another user", async () => {
    const { c, sent, mock } = tokenApi();
    const renew = vi.fn(() => token({ user_id: "42", exp: seconds() + 3600 }));
    void c.identify({ user_id: "42", user_token: token({ user_id: "42", exp: seconds() + 3600 }), refresh_user_token: renew });
    await boot(c);

    const other = c.identify({ user_id: "7", user_hash: "ab12" });
    await vi.advanceTimersByTimeAsync(200);
    await other;

    expect(sent[1]).toMatchObject({ user_id: "7", user_hash: "ab12" });
    expect("user_token" in sent[1]).toBe(false);
    expect(renew).not.toHaveBeenCalled();
    c.stop();
    mock.dispose();
  });
});
