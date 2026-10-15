import { describe, expect, it } from "vitest";
import { isDesktop, setOAuthHandoff, takeVerifier } from "./desktop";

describe("desktop bridge", () => {
  it("is off in a browser", () => {
    expect(isDesktop).toBe(false);
  });

  it("hands the PKCE verifier to its code once", () => {
    setOAuthHandoff({ code: "abc", verifier: "v".repeat(64) });
    expect(takeVerifier("other")).toBeUndefined();
    expect(takeVerifier("abc")).toBe("v".repeat(64));
    expect(takeVerifier("abc")).toBeUndefined();
  });
});
