import { describe, expect, it } from "vitest";
import { assetUrl, normalizeBase, readConfig, routerBasepath } from "./config";

describe("base path", () => {
  it("normalizes to one leading and trailing slash", () => {
    expect(normalizeBase(undefined)).toBe("/");
    expect(normalizeBase("/")).toBe("/");
    expect(normalizeBase("/admin/")).toBe("/admin/");
    expect(normalizeBase("admin")).toBe("/admin/");
    expect(normalizeBase("//admin//")).toBe("/admin/");
  });

  it("gives the router a basepath without the trailing slash", () => {
    expect(routerBasepath("/")).toBe("/");
    expect(routerBasepath("/admin/")).toBe("/admin");
  });

  it("puts assets under the base", () => {
    expect(assetUrl("config.js", "/")).toBe("/config.js");
    expect(assetUrl("/config.js", "/admin/")).toBe("/admin/config.js");
  });
});

describe("readConfig", () => {
  it("prefers config.js over the build env", () => {
    const c = readConfig(
      { apiUrl: "https://api.example.com/", turnstileSiteKey: "0x4", appUrl: "https://app.example.com", envLabel: "Production" },
      { VITE_API_URL: "http://localhost:9999", BASE_URL: "/" },
    );
    expect(c).toEqual({ apiUrl: "https://api.example.com", turnstileSiteKey: "0x4", appUrl: "https://app.example.com", envLabel: "Production", base: "/" });
  });

  it("falls back to the VITE_ env, then to local development", () => {
    expect(readConfig({}, { VITE_API_URL: "http://localhost:8081", VITE_ENV_LABEL: "Dev", BASE_URL: "/" })).toMatchObject({
      apiUrl: "http://localhost:8081",
      envLabel: "Dev",
      appUrl: "http://localhost:5173",
      turnstileSiteKey: "",
    });
    expect(readConfig(undefined, {})).toMatchObject({ apiUrl: "http://localhost:8080", base: "/" });
  });

  it("ignores empty, template and non-http values", () => {
    const c = readConfig({ apiUrl: "${ADMIN_API_URL}", appUrl: "javascript:alert(1)", turnstileSiteKey: "  ", envLabel: 3 }, { BASE_URL: "/" });
    expect(c).toMatchObject({ apiUrl: "http://localhost:8080", appUrl: "http://localhost:5173", turnstileSiteKey: "", envLabel: "" });
  });

  it("uses the panel's own origin when embedded under /admin/", () => {
    const c = readConfig({}, { BASE_URL: "/admin/" }, "https://desk.example.com");
    expect(c).toMatchObject({ apiUrl: "https://desk.example.com", appUrl: "https://desk.example.com", base: "/admin/" });
  });

  it("keeps the env label short", () => {
    expect(readConfig({ envLabel: "A very long label that goes on and on" }, {}).envLabel).toHaveLength(24);
  });
});
