import { describe, expect, it } from "vitest";
import { parseHelpDomain } from "./config";
import { defaultHelpUrl, helpAddressForm, helpCenterHref, legacyHelpTarget, looksLikeHostname, normalizeHostname, relativeRecordName, withSlug } from "./helpCenter";

const loc = (pathname: string, search = "", hash = "") => ({ pathname, search, hash });
const hc = (url: string, default_url = url) => ({ enabled: true, title: "", url, default_url, domain: null });

describe("legacyHelpTarget", () => {
  it("sends /help/<slug>/... to the same page on the default address", () => {
    expect(legacyHelpTarget(loc("/help/margin"), "opendeskhelp.com")).toBe("https://margin.opendeskhelp.com/");
    expect(legacyHelpTarget(loc("/help/margin/"), "opendeskhelp.com")).toBe("https://margin.opendeskhelp.com/");
    expect(legacyHelpTarget(loc("/help/margin/c/billing"), "opendeskhelp.com")).toBe("https://margin.opendeskhelp.com/c/billing");
    expect(legacyHelpTarget(loc("/help/Margin/a/sso", "?lang=de", "#setup"), "opendeskhelp.com")).toBe("https://margin.opendeskhelp.com/a/sso?lang=de#setup");
  });
  it("leaves the link alone without helpDomain, or off the help paths", () => {
    expect(legacyHelpTarget(loc("/help/margin/a/sso"), "")).toBeNull();
    expect(legacyHelpTarget(loc("/helpers"), "opendeskhelp.com")).toBeNull();
    expect(legacyHelpTarget(loc("/help/evil.example.com/a"), "opendeskhelp.com")).toBeNull();
  });
});

describe("addresses", () => {
  it("builds the default address from the slug", () => {
    expect(defaultHelpUrl("margin", "opendeskhelp.com", "https://app.opendesk.sh")).toBe("https://margin.opendeskhelp.com");
    expect(defaultHelpUrl("margin", "", "https://help.acme.com/")).toBe("https://help.acme.com/help/margin");
  });
  it("links pages under the help center's url", () => {
    expect(helpCenterHref({ help_center: hc("https://help.margin.app") }, "/a/sso")).toBe("https://help.margin.app/a/sso");
    expect(helpCenterHref({ help_center: hc("https://acme.example/help/margin/") })).toBe("https://acme.example/help/margin");
  });
  it("tells the address form from the config, else from the brand", () => {
    expect(helpAddressForm(undefined, "opendeskhelp.com")).toEqual({ kind: "subdomain", suffix: "opendeskhelp.com" });
    expect(helpAddressForm({ slug: "margin", help_center: hc("https://margin.help.acme.com") }, "")).toEqual({ kind: "subdomain", suffix: "help.acme.com" });
    expect(helpAddressForm({ slug: "margin", help_center: hc("https://acme.example/help/margin") }, "")).toEqual({ kind: "path", prefix: "/help/" });
  });
  it("moves the default address with the slug", () => {
    expect(withSlug({ slug: "margin", help_center: hc("https://help.margin.app", "https://margin.opendeskhelp.com") }, "ledger")).toBe("https://ledger.opendeskhelp.com");
    expect(withSlug({ slug: "margin", help_center: hc("https://acme.example/help/margin") }, "ledger")).toBe("https://acme.example/help/ledger");
  });
});

describe("custom domain input", () => {
  it("keeps the hostname of what was typed", () => {
    expect(normalizeHostname("  https://Help.Acme.com/kb?x=1 ")).toBe("help.acme.com");
    expect(normalizeHostname("help.acme.com.")).toBe("help.acme.com");
    expect(normalizeHostname("help.acme.com:8443")).toBe("help.acme.com");
  });
  it("checks the hostname's shape", () => {
    expect(looksLikeHostname("help.acme.com")).toBe(true);
    expect(looksLikeHostname("localhost")).toBe(false);
    expect(looksLikeHostname("-help.acme.com")).toBe(false);
    expect(looksLikeHostname("help acme.com")).toBe(false);
  });
  it("suggests the name without the domain for providers that add it", () => {
    expect(relativeRecordName("_opendesk-verify.help.acme.com")).toBe("_opendesk-verify.help");
    expect(relativeRecordName("acme.com")).toBe("acme.com");
  });
});

describe("parseHelpDomain", () => {
  it("accepts a bare hostname only", () => {
    expect(parseHelpDomain("opendeskhelp.com")).toBe("opendeskhelp.com");
    expect(parseHelpDomain(" OpendeskHelp.com. ")).toBe("opendeskhelp.com");
    expect(parseHelpDomain("https://opendeskhelp.com")).toBe("");
    expect(parseHelpDomain("")).toBe("");
  });
});
