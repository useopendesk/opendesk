import { describe, expect, it } from "vitest";
import { stripWorkspaceParam, workspaceParam, workspaceTarget } from "./workspaceParam";

const A = "0b6c1f5e-6d1e-4a51-9a0e-1c1d2e3f4a5b";
const B = "7a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d";
const C = "11111111-2222-4333-8444-555555555555";

describe("workspaceParam", () => {
  it("reads the id, refuses junk and tells absent from bad", () => {
    expect(workspaceParam({ ws: A })).toBe(A);
    expect(workspaceParam({})).toBeUndefined();
    expect(workspaceParam({ ws: "" })).toBeNull();
    expect(workspaceParam({ ws: "../../x" })).toBeNull();
    expect(workspaceParam({ ws: { a: 1 } })).toBeNull();
    expect(workspaceParam({ ws: 42 })).toBe("42");
  });
});

describe("stripWorkspaceParam", () => {
  it("keeps the path, the other parameters and the hash", () => {
    expect(stripWorkspaceParam(`/inbox/mine/${B}?ws=${A}`)).toBe(`/inbox/mine/${B}`);
    expect(stripWorkspaceParam(`/inbox/open?sort=priority&ws=${A}&q=sso#top`)).toBe("/inbox/open?sort=priority&q=sso#top");
    expect(stripWorkspaceParam("/settings/notifications")).toBe("/settings/notifications");
  });
});

describe("workspaceTarget", () => {
  const user = { id: "u1" };
  const mine = [{ workspace: { id: A } }, { workspace: { id: B } }];
  const accounts = [
    { user, memberships: mine },
    { user: { id: "u2" }, memberships: [{ workspace: { id: C } }] },
  ];
  const s = { user, workspaceId: A, memberships: mine, accounts };
  it("does nothing for the open workspace", () => {
    expect(workspaceTarget(A, s)).toEqual({ kind: "current" });
  });
  it("selects another workspace of the active account", () => {
    expect(workspaceTarget(B, s)).toEqual({ kind: "select" });
  });
  it("switches to another signed-in account that is in it", () => {
    expect(workspaceTarget(C, s)).toEqual({ kind: "account", userId: "u2" });
  });
  it("ignores a workspace nobody here is in", () => {
    expect(workspaceTarget("nope", s)).toEqual({ kind: "none" });
  });
});
