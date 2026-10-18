import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import vm from "node:vm";
import { buildConfig, envLabel, writeConfig } from "./write-config.mjs";

test("defaults to production URLs and leaves optional keys out", () => {
  assert.deepEqual(buildConfig({}), { apiUrl: "https://api.opendesk.sh", appUrl: "https://app.opendesk.sh" });
});

test("reads every key from the environment", () => {
  const cfg = buildConfig({
    ADMIN_API_URL: "https://api.example.com/",
    ADMIN_APP_URL: "https://desk.example.com",
    TURNSTILE_SITE_KEY: " 0x4AAA ",
    ADMIN_ENV_LABEL: " Production ",
  });
  assert.deepEqual(cfg, {
    apiUrl: "https://api.example.com",
    appUrl: "https://desk.example.com",
    turnstileSiteKey: "0x4AAA",
    envLabel: "Production",
  });
});

test("refuses URLs that are not http(s) origins", () => {
  assert.throws(() => buildConfig({ ADMIN_API_URL: "not a url" }), /ADMIN_API_URL/);
  assert.throws(() => buildConfig({ ADMIN_API_URL: "ftp://api.example.com" }), /ADMIN_API_URL/);
  assert.throws(() => buildConfig({ ADMIN_APP_URL: "javascript:alert(1)" }), /ADMIN_APP_URL/);
  assert.throws(() => buildConfig({ ADMIN_API_URL: "https://api.example.com/v1" }), /ADMIN_API_URL/);
});

test("ADMIN_ENV_LABEL is a short plain label", () => {
  assert.equal(envLabel("Staging (eu)"), "Staging (eu)");
  assert.equal("envLabel" in buildConfig({ ADMIN_ENV_LABEL: "  " }), false);
  for (const bad of ["<script>", "a".repeat(25), "prod\nuction", undefined]) {
    assert.throws(() => envLabel(bad), /ADMIN_ENV_LABEL/, String(bad));
  }
});

test("writes a config.js that sets window.__OPENDESK_ADMIN__", () => {
  const dir = mkdtempSync(join(tmpdir(), "od-admin-config-"));
  const [file] = writeConfig(dir, { TURNSTILE_SITE_KEY: "0x4</script>", ADMIN_ENV_LABEL: "Production" });
  assert.equal(file, join(dir, "config.js"));
  const ctx = { window: {} };
  vm.runInNewContext(readFileSync(file, "utf8"), ctx);
  assert.equal(ctx.window.__OPENDESK_ADMIN__.apiUrl, "https://api.opendesk.sh");
  assert.equal(ctx.window.__OPENDESK_ADMIN__.turnstileSiteKey, "0x4</script>");
  assert.equal(ctx.window.__OPENDESK_ADMIN__.envLabel, "Production");
});

test("needs an output directory", () => {
  assert.throws(() => writeConfig(undefined, {}), /usage/);
});
