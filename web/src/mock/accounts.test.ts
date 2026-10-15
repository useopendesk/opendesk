import { beforeAll, describe, expect, it } from "vitest";
import { installMock } from "./index";
import { dispatch } from "./router";
import { db } from "./db";
import { SECOND_ACCOUNT_EMAIL } from "./seed";

async function call(method: string, path: string, body?: unknown, token?: string) {
  const headers: Record<string, string> = {};
  if (token) headers.Authorization = `Bearer ${token}`;
  const r = await dispatch(`http://mock${path}`, { method, body: body ? JSON.stringify(body) : undefined, headers });
  return { status: r.status, body: r.status === 204 ? null : await r.json() };
}

type Accounts = { accounts: { user: { id: string; email: string }; memberships: { workspace: { name: string } }[] }[] };

describe("mock accounts (several signed in at once)", () => {
  beforeAll(() => {
    localStorage.clear();
    installMock();
  });

  it("starts with Maya's two accounts and lists them with their workspaces", async () => {
    const r = await call("POST", "/v1/auth/accounts");
    const list = (r.body as Accounts).accounts;
    expect(list.map((a) => a.user.email)).toEqual([db.users[0].email, SECOND_ACCOUNT_EMAIL]);
    expect(list[1].memberships.map((m) => m.workspace.name)).toEqual(["Harbor Studio"]);
  });

  it("refreshes the named account and runs requests as the token's account", async () => {
    const second = db.users.find((u) => u.email === SECOND_ACCOUNT_EMAIL)!;
    const a = await call("POST", "/v1/auth/refresh", { user_id: db.users[0].id });
    const b = await call("POST", "/v1/auth/refresh", { user_id: second.id });
    expect(a.body.user.id).toBe(db.users[0].id);
    expect(b.body.user.id).toBe(second.id);
    expect((await call("GET", "/v1/me", undefined, a.body.access_token)).body.user.email).toBe(db.users[0].email);
    expect((await call("GET", "/v1/me", undefined, b.body.access_token)).body.user.email).toBe(SECOND_ACCOUNT_EMAIL);
    expect((await call("POST", "/v1/auth/refresh", { user_id: "00000000-0000-4000-8000-000000000000" })).status).toBe(401);
  });

  it("logs out one account, then all", async () => {
    const second = db.users.find((u) => u.email === SECOND_ACCOUNT_EMAIL)!;
    const b = await call("POST", "/v1/auth/refresh", { user_id: second.id });
    expect((await call("POST", "/v1/auth/logout", { user_id: second.id }, b.body.access_token)).status).toBe(204);
    expect((await call("POST", "/v1/auth/accounts")).body.accounts).toHaveLength(1);
    expect((await call("POST", "/v1/auth/refresh", { user_id: second.id })).status).toBe(401);

    // Signing in again adds it back beside the first.
    const login = await call("POST", "/v1/auth/login", { email: SECOND_ACCOUNT_EMAIL, password: "anything at all" });
    expect(login.body.user.id).toBe(second.id);
    expect((await call("POST", "/v1/auth/accounts")).body.accounts).toHaveLength(2);

    expect((await call("POST", "/v1/auth/logout", { all: true })).status).toBe(204);
    expect((await call("POST", "/v1/auth/accounts")).body.accounts).toHaveLength(0);
    expect((await call("POST", "/v1/auth/refresh")).status).toBe(401);
  });

  it("holds at most five accounts", async () => {
    for (let i = 0; i < 5; i++) {
      const r = await call("POST", "/v1/auth/register", { email: `cap${i}@example.com`, name: `Cap ${i}`, password: "correct horse battery" });
      expect(r.status).toBe(200);
    }
    const r = await call("POST", "/v1/auth/register", { email: "cap5@example.com", name: "Cap 5", password: "correct horse battery" });
    expect(r.status).toBe(409);
    expect(r.body.code).toBe("too_many_accounts");
  });
});
