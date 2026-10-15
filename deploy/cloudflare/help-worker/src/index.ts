// opendesk help center worker.
//
// Every request to the help center zone (opendeskhelp.com) runs here: the
// <slug>.opendeskhelp.com addresses and every customer domain that reaches the
// zone as a Cloudflare for SaaS custom hostname (contracts/teams.md "Help
// center addresses"). The API renders the sites; this worker names the host
// for it and caches what it answers:
//
//   <method> {ORIGIN}<path>?<query>
//   OD-Help-Host: <the hostname the visitor asked for>
//   OD-Help-Client-IP: <CF-Connecting-IP>
//   OD-Help-Secret: <HELP_PROXY_SECRET>
//   X-Forwarded-Proto: https
//
// The API believes OD-Help-Host and OD-Help-Client-IP only with the secret.
// Only the headers in FORWARDED_HEADERS reach the origin: a help center is
// public and anonymous, so cookies and credentials never leave the edge.
//
// Hosts of the zone itself:
//   - the apex and www redirect to the product site (SITE_URL);
//   - custom.<HELP_DOMAIN> (what customer domains CNAME to) and
//     origin.<HELP_DOMAIN> (the custom hostname fallback origin) are DNS
//     targets, not sites, and answer 404 when asked for by name.
//
// Caching: GET and HEAD are answered from the Workers cache of this data
// center first, keyed by the URL the visitor asked for, so each host has its
// own entries. A response is stored only when the origin marks it
// Cache-Control: public, answers 200, 301 or 404, and sets no cookie; the
// stored copy lives as long as that Cache-Control says. Other methods (the
// POST /_help/views beacon) always go to the origin. The origin fetch itself
// skips the cache of the origin's zone, where one URL stands for every help
// center host.

export interface Env {
  /** The API, e.g. https://api.opendesk.sh. */
  ORIGIN: string;
  /** The product site the zone apex redirects to, e.g. https://opendesk.sh. */
  SITE_URL: string;
  /** The help center zone, e.g. opendeskhelp.com (the API's HELP_DOMAIN). */
  HELP_DOMAIN: string;
  /** Shared with the API (HELP_PROXY_SECRET), at least 32 bytes. */
  HELP_PROXY_SECRET?: string;
}

/** The subset of the Workers Cache this worker uses. */
export interface EdgeCache {
  match(request: Request): Promise<Response | undefined>;
  put(request: Request, response: Response): Promise<void>;
}

/** The subset of ExecutionContext this worker uses. */
export interface WaitUntil {
  waitUntil(promise: Promise<unknown>): void;
}

export interface HandleOptions {
  fetch?: typeof fetch;
  cache?: EdgeCache | null;
  log?: (line: Record<string, unknown>) => void;
}

/** Request headers the origin sees from the visitor; nothing else is forwarded. */
export const FORWARDED_HEADERS = [
  "accept",
  "accept-language",
  "content-type",
  "if-none-match",
  "user-agent",
  "referer",
] as const;

/** Origin statuses worth keeping at the edge. */
export const CACHEABLE_STATUS = new Set([200, 301, 404]);

const TEXT = { "Content-Type": "text/plain; charset=utf-8", "Cache-Control": "no-store" };

function text(body: string, status: number): Response {
  return new Response(body, { status, headers: TEXT });
}

function normHost(host: string): string {
  return host.trim().toLowerCase().replace(/\.$/, "");
}

/** What the zone's own hosts are: a redirect to the product site, a bare DNS target, or a help center. */
export function classify(hostname: string, helpDomain: string): "site" | "target" | "help" {
  const host = normHost(hostname);
  const zone = normHost(helpDomain);
  if (zone === "") return "help";
  if (host === zone || host === `www.${zone}`) return "site";
  if (host === `custom.${zone}` || host === `origin.${zone}`) return "target";
  return "help";
}

/** Whether an origin response may be stored at the edge. */
export function cacheable(res: Response): boolean {
  if (!CACHEABLE_STATUS.has(res.status)) return false;
  if (res.headers.has("Set-Cookie")) return false;
  const cc = (res.headers.get("Cache-Control") ?? "").toLowerCase();
  const directives = cc.split(",").map((d) => d.trim().split("=")[0]);
  if (!directives.includes("public")) return false;
  return !directives.some((d) => d === "private" || d === "no-store" || d === "no-cache");
}

/** The request to the origin: same method, path, query and body; only safe headers. */
export function originRequest(request: Request, env: Env, body: ArrayBuffer | null = null): Request {
  const url = new URL(request.url);
  const target = env.ORIGIN.replace(/\/+$/, "") + url.pathname + url.search;
  const headers = new Headers();
  for (const name of FORWARDED_HEADERS) {
    const v = request.headers.get(name);
    if (v !== null) headers.set(name, v);
  }
  headers.set("OD-Help-Host", normHost(url.hostname));
  const ip = request.headers.get("CF-Connecting-IP");
  if (ip) headers.set("OD-Help-Client-IP", ip);
  headers.set("OD-Help-Secret", env.HELP_PROXY_SECRET ?? "");
  headers.set("X-Forwarded-Proto", "https");
  return new Request(target, {
    method: request.method,
    headers,
    body,
    redirect: "manual",
    cache: "no-store",
  });
}

/** The cache key: the URL the visitor asked for, host included. */
function cacheKey(request: Request): Request {
  return new Request(request.url, { method: "GET" });
}

/** A lookup of the key that lets the cache answer 304 to a matching If-None-Match. */
function cacheLookup(request: Request): Request {
  const inm = request.headers.get("If-None-Match");
  return new Request(request.url, { method: "GET", headers: inm ? { "If-None-Match": inm } : {} });
}

export async function handle(request: Request, env: Env, ctx: WaitUntil, opts: HandleOptions = {}): Promise<Response> {
  const doFetch = opts.fetch ?? fetch;
  const cache = opts.cache === undefined ? caches.default : opts.cache;
  const log = opts.log ?? ((line) => console.log(JSON.stringify(line)));
  const url = new URL(request.url);

  switch (classify(url.hostname, env.HELP_DOMAIN)) {
    case "site":
      return Response.redirect(env.SITE_URL, 301);
    case "target":
      return text("Not found.", 404);
  }

  if (!env.HELP_PROXY_SECRET || !env.ORIGIN) {
    log({ msg: "help worker is not configured" });
    return text("The help center is unavailable. Please try again soon.", 502);
  }

  const readOnly = request.method === "GET" || request.method === "HEAD";
  const key = readOnly && cache ? cacheKey(request) : null;
  if (key && cache) {
    const hit = await cache.match(cacheLookup(request)).catch(() => undefined);
    if (hit) {
      return request.method === "HEAD" ? new Response(null, hit) : hit;
    }
  }

  let res: Response;
  try {
    // Bodies here are small (the views beacon), so one is read whole and sent
    // with a Content-Length.
    const body = readOnly ? null : await request.arrayBuffer();
    res = await doFetch(originRequest(request, env, body));
  } catch (err) {
    log({ msg: "help origin fetch failed", error: err instanceof Error ? err.name : "network error" });
    return text("The help center is unavailable. Please try again soon.", 502);
  }

  // A HEAD answer has no body, so only a GET fills the cache.
  if (key && cache && request.method === "GET" && cacheable(res)) {
    const copy = res.clone();
    ctx.waitUntil(cache.put(key, copy).catch(() => {}));
  }
  return res;
}

export default {
  async fetch(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
    return handle(request, env, ctx);
  },
} satisfies ExportedHandler<Env>;
