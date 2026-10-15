import { describe, expect, it } from "vitest";
import { parseHelpDomain, parseMailboxImap, parseMailboxOAuth } from "./config";

describe("parseMailboxImap", () => {
  it("takes the runtime value first", () => {
    expect(parseMailboxImap(false, "true")).toBe(false);
    expect(parseMailboxImap(true, "false")).toBe(true);
    expect(parseMailboxImap("false", undefined)).toBe(false);
    expect(parseMailboxImap(" TRUE ", "false")).toBe(true);
  });
  it("falls back to the build env, then on", () => {
    expect(parseMailboxImap(undefined, "false")).toBe(false);
    expect(parseMailboxImap("", "false")).toBe(false);
    expect(parseMailboxImap("${MAILBOX_IMAP}", "false")).toBe(false);
    expect(parseMailboxImap(undefined, "true")).toBe(true);
    expect(parseMailboxImap(undefined, undefined)).toBe(true);
    expect(parseMailboxImap(undefined, "")).toBe(true);
    expect(parseMailboxImap(["google"], "maybe")).toBe(true);
  });
});

describe("parseMailboxOAuth", () => {
  it("reads a list, a comma list or none", () => {
    expect(parseMailboxOAuth(["google", "nope"], undefined)).toEqual(["google"]);
    expect(parseMailboxOAuth(undefined, "google, microsoft")).toEqual(["google", "microsoft"]);
    expect(parseMailboxOAuth(undefined, "none")).toEqual([]);
    expect(parseMailboxOAuth(undefined, undefined)).toBeNull();
  });
});

describe("parseHelpDomain", () => {
  it("keeps a bare hostname only", () => {
    expect(parseHelpDomain("Help.Example.com.")).toBe("help.example.com");
    expect(parseHelpDomain("https://help.example.com")).toBe("");
  });
});
