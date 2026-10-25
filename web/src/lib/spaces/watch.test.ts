import { describe, expect, it } from "vitest";
import { MAX_WATCHED, pollInterval, POLL_MS, watchList } from "./watch";

describe("keeping other workspaces live", () => {
  it("watches each other workspace once, never the open one, at most eight", () => {
    expect(watchList(["a", "b", "a", "open", "c"], "open")).toEqual(["a", "b", "c"]);
    const many = Array.from({ length: 12 }, (_, i) => `w${i}`);
    expect(watchList(many, null)).toHaveLength(MAX_WATCHED);
    expect(watchList(many, null)).toEqual(many.slice(0, 8));
  });

  it("polls a workspace without a live socket every minute", () => {
    expect(pollInterval("nobody-watches-this")).toBe(POLL_MS);
  });
});
