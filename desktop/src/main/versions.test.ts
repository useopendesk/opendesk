import { describe, expect, it } from "vitest";
import { newerVersion } from "./versions";

describe("newerVersion", () => {
  it.each([
    ["1.4.0", "1.3.9", true],
    ["1.10.0", "1.9.0", true],
    ["2.0.0", "1.99.99", true],
    ["1.3.0", "1.3.0-beta.4", true],
    ["1.3.0-beta.10", "1.3.0-beta.9", true],
    ["1.3.0-beta.2", "1.3.0-alpha.9", true],
    ["1.3.0-beta.2.1", "1.3.0-beta.2", true],
    ["v1.3.1", "1.3.0", true],
    ["1.3.0", "1.3.0", false],
    ["1.3.0-beta.4", "1.3.0", false],
    ["1.2.9", "1.3.0-beta.1", false],
    ["1.3.0-beta.2", "1.3.0-beta.2.1", false],
  ])("%s after %s: %s", (a, b, want) => {
    expect(newerVersion(a, b)).toBe(want);
  });
});
