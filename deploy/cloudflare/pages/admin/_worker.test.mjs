import assert from "node:assert/strict";
import { test } from "node:test";
import worker from "./_worker.js";

const assets = (res) => ({ ASSETS: { fetch: async () => res } });
const get = (path) => new Request("https://admin.example.com" + path);

test("a missing bundle answers 404 that nothing caches", async () => {
  const fallback = new Response("<!doctype html>", { headers: { "Content-Type": "text/html; charset=utf-8" } });
  const res = await worker.fetch(get("/assets/Page-gone1234.js"), assets(fallback));
  assert.equal(res.status, 404);
  assert.equal(res.headers.get("Cache-Control"), "no-store");
  assert.ok(!(await res.text()).includes("<"));
});

test("a bundle the deployment has passes through", async () => {
  const js = new Response("export {}", { headers: { "Content-Type": "application/javascript", "Cache-Control": "public, max-age=31536000, immutable" } });
  const res = await worker.fetch(get("/assets/index-AbCdEf12.js"), assets(js));
  assert.equal(res, js);
});
