import { describe, expect, it } from "vitest";
import type { Notification } from "@/lib/api/types";
import { unreadForTicket, withRead } from "./notificationRead";

const n = (id: string, ticket: string | null, read_at: string | null = null): Notification => ({
  id,
  type: "assigned",
  ticket: ticket ? { id: ticket, number: 1, subject: "x" } : null,
  data: {},
  read_at,
  created_at: "2026-10-13T09:00:00Z",
});

describe("unreadForTicket", () => {
  it("lists only the ticket's unread notifications", () => {
    const list = [n("1", "t1"), n("2", "t1", "2026-10-13T09:10:00Z"), n("3", "t2"), n("4", null), n("5", "t1")];
    expect(unreadForTicket(list, "t1")).toEqual(["1", "5"]);
    expect(unreadForTicket(list, "")).toEqual([]);
    expect(unreadForTicket(undefined, "t1")).toEqual([]);
  });
});

describe("withRead", () => {
  it("marks the named ones and keeps earlier read times", () => {
    const list = [n("1", "t1"), n("2", "t1", "2026-10-13T09:10:00Z"), n("3", "t2")];
    const out = withRead(list, { ids: ["1", "2"] }, "2026-10-13T09:42:00Z");
    expect(out.map((x) => x.read_at)).toEqual(["2026-10-13T09:42:00Z", "2026-10-13T09:10:00Z", null]);
    expect(out[2]).toBe(list[2]);
  });
  it("marks everything with all", () => {
    expect(withRead([n("1", null), n("2", "t1")], { all: true }, "now").every((x) => x.read_at)).toBe(true);
  });
});
