import { describe, expect, it } from "vitest";
import { nextTarget, safeNext } from "./next";

describe("safeNext", () => {
  it("keeps paths inside the panel", () => {
    expect(safeNext("/workspaces/w1")).toBe("/workspaces/w1");
    expect(safeNext("/users?q=a")).toBe("/users?q=a");
  });

  it("refuses other origins and the sign-in page", () => {
    for (const bad of ["//evil.example", "/\\evil.example", "https://evil.example", "workspaces", "/login", "/login?next=/x", "/x\u0000", 42, undefined]) {
      expect(safeNext(bad)).toBeUndefined();
    }
  });

  it("splits into a path and search", () => {
    expect(nextTarget("/users?q=a&filter=admins")).toEqual({ to: "/users", search: { q: "a", filter: "admins" } });
    expect(nextTarget(undefined)).toEqual({ to: "/", search: {} });
  });
});
