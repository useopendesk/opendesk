import { describe, expect, it, vi } from "vitest";
import { cacheable, classify, handle, type EdgeCache, type Env } from "../src/index";

const env: Env = {
  ORIGIN: "https://api.opendesk.sh",
  SITE_URL: "https://opendesk.sh",
  HELP_DOMAIN: "opendeskhelp.com",
  HELP_PROXY_SECRET: "s3cret-s3cret-s3cret-s3cret-s3cret",
};

type Step = Response | Error;
function fakeFetch(steps: Step[]) {
  const calls: Request[] = [];
  const fn = vi.fn(async (input: string | URL | Request) => {
    calls.push(input as Request);
    const step = steps.shift();
    if (!step) throw new Error("no more steps");
    if (step instanceof Error) throw step;
    return step;
  });
  return { fetch: fn as unknown as typeof fetch, calls };
}

function memoryCache() {
  const store = new Map<string, Response>();
  const cache: EdgeCache = {
    match: vi.fn(async (req: Request) => store.get(req.url)?.clone()),
    put: vi.fn(async (req: Request, res: Response) => {
      expect(req.method).toBe("GET");
      store.set(req.url, res);
    }),
  };
  return { cache, store };
}

function context() {
  const pending: Promise<unknown>[] = [];
  return { ctx: { waitUntil: (p: Promise<unknown>) => void pending.push(p) }, settle: () => Promise.all(pending) };
}

const page = (status = 200, headers: Record<string, string> = {}) =>
  new Response(status === 301 || status === 304 ? null : "<!doctype html>", {
    status,
    headers: { "Content-Type": "text/html; charset=utf-8", "Cache-Control": "public, max-age=60", ...headers },
  });

const log = () => {};

describe("handle", () => {
  it("forwards a help center request with only safe headers and the proxy headers", async () => {
    const { fetch, calls } = fakeFetch([page()]);
    const { ctx } = context();
    const req = new Request("https://acme.opendeskhelp.com/a/reset-password?lang=de", {
      headers: {
        Accept: "text/html",
        "Accept-Language": "de",
        "User-Agent": "test",
        Referer: "https://www.google.com/",
        "If-None-Match": '"abc"',
        Cookie: "session=1",
        Authorization: "Bearer x",
        "CF-Connecting-IP": "203.0.113.7",
        "X-Forwarded-For": "198.51.100.1",
        "OD-Help-Host": "evil.example.com",
        "OD-Help-Secret": "guess",
      },
    });
    const res = await handle(req, env, ctx, { fetch, cache: null, log });
    expect(res.status).toBe(200);
    expect(calls).toHaveLength(1);
    const o = calls[0]!;
    expect(o.url).toBe("https://api.opendesk.sh/a/reset-password?lang=de");
    expect(o.method).toBe("GET");
    expect(o.redirect).toBe("manual");
    expect(o.headers.get("OD-Help-Host")).toBe("acme.opendeskhelp.com");
    expect(o.headers.get("OD-Help-Client-IP")).toBe("203.0.113.7");
    expect(o.headers.get("OD-Help-Secret")).toBe(env.HELP_PROXY_SECRET);
    expect(o.headers.get("X-Forwarded-Proto")).toBe("https");
    expect(o.headers.get("Accept")).toBe("text/html");
    expect(o.headers.get("Accept-Language")).toBe("de");
    expect(o.headers.get("User-Agent")).toBe("test");
    expect(o.headers.get("Referer")).toBe("https://www.google.com/");
    expect(o.headers.get("If-None-Match")).toBe('"abc"');
    expect(o.headers.get("Cookie")).toBeNull();
    expect(o.headers.get("Authorization")).toBeNull();
    expect(o.headers.get("X-Forwarded-For")).toBeNull();
    expect(o.headers.get("CF-Connecting-IP")).toBeNull();
  });

  it("forwards custom hostnames as asked for", async () => {
    const { fetch, calls } = fakeFetch([page()]);
    const { ctx } = context();
    await handle(new Request("https://Help.Example.com/"), env, ctx, { fetch, cache: null, log });
    expect(calls[0]!.url).toBe("https://api.opendesk.sh/");
    expect(calls[0]!.headers.get("OD-Help-Host")).toBe("help.example.com");
  });

  it("redirects the apex and www to the product site", async () => {
    const { fetch, calls } = fakeFetch([]);
    const { ctx } = context();
    for (const u of ["https://opendeskhelp.com/", "https://www.opendeskhelp.com/pricing?x=1"]) {
      const res = await handle(new Request(u), env, ctx, { fetch, cache: null, log });
      expect(res.status).toBe(301);
      expect(res.headers.get("Location")).toBe("https://opendesk.sh/");
    }
    expect(calls).toHaveLength(0);
  });

  it("answers 404 on the DNS targets", async () => {
    const { fetch, calls } = fakeFetch([]);
    const { ctx } = context();
    for (const u of ["https://custom.opendeskhelp.com/", "https://origin.opendeskhelp.com/x"]) {
      const res = await handle(new Request(u), env, ctx, { fetch, cache: null, log });
      expect(res.status).toBe(404);
      expect(res.headers.get("Content-Type")).toMatch(/^text\/plain/);
    }
    expect(calls).toHaveLength(0);
  });

  it("caches a public 200 and answers the next request from the cache", async () => {
    const { fetch, calls } = fakeFetch([page()]);
    const { cache } = memoryCache();
    const { ctx, settle } = context();
    const url = "https://acme.opendeskhelp.com/c/billing";
    const first = await handle(new Request(url), env, ctx, { fetch, cache, log });
    expect(await first.text()).toBe("<!doctype html>");
    await settle();
    expect(cache.put).toHaveBeenCalledTimes(1);
    const second = await handle(new Request(url), env, ctx, { fetch, cache, log });
    expect(second.status).toBe(200);
    expect(await second.text()).toBe("<!doctype html>");
    expect(calls).toHaveLength(1);
  });

  it("keys the cache by host", async () => {
    const { fetch, calls } = fakeFetch([page(), page()]);
    const { cache } = memoryCache();
    const { ctx, settle } = context();
    await handle(new Request("https://acme.opendeskhelp.com/"), env, ctx, { fetch, cache, log });
    await settle();
    await handle(new Request("https://beta.opendeskhelp.com/"), env, ctx, { fetch, cache, log });
    expect(calls).toHaveLength(2);
  });

  it("caches 301 and 404 too", async () => {
    const { fetch } = fakeFetch([page(301, { Location: "https://help.acme.com/" }), page(404)]);
    const { cache, store } = memoryCache();
    const { ctx, settle } = context();
    await handle(new Request("https://acme.opendeskhelp.com/"), env, ctx, { fetch, cache, log });
    await handle(new Request("https://acme.opendeskhelp.com/a/gone"), env, ctx, { fetch, cache, log });
    await settle();
    expect([...store.keys()]).toEqual(["https://acme.opendeskhelp.com/", "https://acme.opendeskhelp.com/a/gone"]);
  });

  it("does not cache private answers, cookies or other statuses", async () => {
    const { fetch } = fakeFetch([
      page(200, { "Cache-Control": "no-store" }),
      page(200, { "Set-Cookie": "a=b" }),
      page(500),
      page(429),
      new Response("x", { status: 200 }),
    ]);
    const { cache } = memoryCache();
    const { ctx, settle } = context();
    for (let i = 0; i < 5; i++) {
      await handle(new Request(`https://acme.opendeskhelp.com/p${i}`), env, ctx, { fetch, cache, log });
    }
    await settle();
    expect(cache.put).not.toHaveBeenCalled();
  });

  it("never caches a POST and forwards its body", async () => {
    const { fetch, calls } = fakeFetch([new Response(null, { status: 204, headers: { "Cache-Control": "no-store" } })]);
    const { cache } = memoryCache();
    const { ctx, settle } = context();
    const id = "0b8f3c1e-1d2a-4c55-9a77-2f1f0d6a9e10";
    const res = await handle(
      new Request("https://acme.opendeskhelp.com/_help/views", {
        method: "POST",
        body: id,
        headers: { "Content-Type": "text/plain" },
      }),
      env,
      ctx,
      { fetch, cache, log },
    );
    await settle();
    expect(res.status).toBe(204);
    expect(calls[0]!.method).toBe("POST");
    expect(await calls[0]!.text()).toBe(id);
    expect(calls[0]!.headers.get("Content-Type")).toBe("text/plain");
    expect(cache.match).not.toHaveBeenCalled();
    expect(cache.put).not.toHaveBeenCalled();
  });

  it("answers HEAD from the cache without a body and never stores a HEAD answer", async () => {
    const { fetch, calls } = fakeFetch([new Response(null, { status: 200, headers: { "Cache-Control": "public" } })]);
    const { cache, store } = memoryCache();
    store.set("https://acme.opendeskhelp.com/", page());
    const { ctx, settle } = context();
    const hit = await handle(new Request("https://acme.opendeskhelp.com/", { method: "HEAD" }), env, ctx, {
      fetch,
      cache,
      log,
    });
    expect(hit.status).toBe(200);
    expect(hit.body).toBeNull();
    expect(calls).toHaveLength(0);
    await handle(new Request("https://acme.opendeskhelp.com/other", { method: "HEAD" }), env, ctx, { fetch, cache, log });
    await settle();
    expect(calls[0]!.method).toBe("HEAD");
    expect(cache.put).not.toHaveBeenCalled();
  });

  it("answers 502 when the origin cannot be reached", async () => {
    const { fetch } = fakeFetch([new TypeError("fetch failed")]);
    const { ctx } = context();
    const res = await handle(new Request("https://acme.opendeskhelp.com/"), env, ctx, { fetch, cache: null, log });
    expect(res.status).toBe(502);
    expect(res.headers.get("Cache-Control")).toBe("no-store");
  });

  it("refuses to forward without the proxy secret", async () => {
    const { fetch, calls } = fakeFetch([]);
    const { ctx } = context();
    const res = await handle(
      new Request("https://acme.opendeskhelp.com/"),
      { ...env, HELP_PROXY_SECRET: undefined },
      ctx,
      { fetch, cache: null, log },
    );
    expect(res.status).toBe(502);
    expect(calls).toHaveLength(0);
  });

  it("passes the origin's redirects through untouched", async () => {
    const { fetch } = fakeFetch([page(301, { Location: "https://help.acme.com/a/x?lang=de" })]);
    const { ctx } = context();
    const res = await handle(new Request("https://acme.opendeskhelp.com/a/x?lang=de"), env, ctx, {
      fetch,
      cache: null,
      log,
    });
    expect(res.status).toBe(301);
    expect(res.headers.get("Location")).toBe("https://help.acme.com/a/x?lang=de");
  });
});

describe("classify", () => {
  it("knows the zone's own hosts", () => {
    expect(classify("opendeskhelp.com", "opendeskhelp.com")).toBe("site");
    expect(classify("WWW.opendeskhelp.com.", "opendeskhelp.com")).toBe("site");
    expect(classify("custom.opendeskhelp.com", "opendeskhelp.com")).toBe("target");
    expect(classify("origin.opendeskhelp.com", "opendeskhelp.com")).toBe("target");
    expect(classify("acme.opendeskhelp.com", "opendeskhelp.com")).toBe("help");
    expect(classify("help.acme.com", "opendeskhelp.com")).toBe("help");
    expect(classify("opendeskhelp.com.evil.com", "opendeskhelp.com")).toBe("help");
  });
});

describe("cacheable", () => {
  it("needs public, a kept status and no cookie", () => {
    expect(cacheable(page())).toBe(true);
    expect(cacheable(page(200, { "Cache-Control": "public, max-age=31536000, immutable" }))).toBe(true);
    expect(cacheable(page(200, { "Cache-Control": "max-age=60" }))).toBe(false);
    expect(cacheable(page(200, { "Cache-Control": "public, no-cache" }))).toBe(false);
    expect(cacheable(page(200, { "Set-Cookie": "a=b" }))).toBe(false);
    expect(cacheable(page(304))).toBe(false);
    expect(cacheable(page(503))).toBe(false);
  });
});
