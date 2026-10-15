// The widget agent's MCP connections (contracts/agent.md "MCP connections").
// Connecting and syncing take a moment and finish before the response (the
// contract never shows "connecting"); secrets are never kept, only whether one
// was given. OAuth survives the full-page redirect via
// sessionStorage, like checkout in handlers/billing.ts.

import type { McpAuthKind, McpServer, McpTool } from "@/lib/api/types";
import { db, userRef, type WsData } from "../db";
import { publish } from "../gateway";
import { HttpError, del, get, invalid, notFound, patch, post, respond, type Ctx } from "../router";
import { DAY, HOUR, MIN, NOW, isRecord, iso, nowIso, slugify, uuid } from "../util";
import { assertAiInstance, requireAiAdmin } from "./ai";

const MAX_SERVERS = 10;
const MAX_ENABLED_TOOLS = 64;
const AUTH_KINDS: McpAuthKind[] = ["none", "bearer", "header", "oauth"];
const FORBIDDEN_HEADERS = new Set(["host", "cookie", "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade"]);
const CONNECT_ERROR = "Couldn't connect to the server. Check the URL and try again.";

// ─── Tool catalogues ───────────────────────────────────────────────────────

type ToolDef = Omit<McpTool, "enabled" | "policy" | "verified_only"> & Partial<Pick<McpTool, "enabled" | "policy" | "verified_only">>;

const obj = (properties: Record<string, unknown>, required: string[] = []) => ({ type: "object", properties, required, additionalProperties: false });

const SHOP_TOOLS: ToolDef[] = [
  {
    name: "lookup_order",
    title: "Look up an order",
    description: "Finds an order by its number and returns its status, items, total and tracking details. Only orders of the signed-in customer are returned.",
    input_schema: obj({ order_id: { type: "string", description: "The order number, with or without the leading #" }, include_items: { type: "boolean", description: "Include the line items", default: true } }, ["order_id"]),
    read_only: true,
    destructive: false,
  },
  {
    name: "list_invoices",
    title: "List invoices",
    description: "Lists the customer's invoices, newest first, with amounts, status and a PDF link.",
    input_schema: obj({ status: { type: "string", description: "Only invoices with this status", enum: ["paid", "open", "void"] }, limit: { type: "integer", description: "How many to return, 1 to 20", minimum: 1, maximum: 20, default: 5 } }),
    read_only: true,
    destructive: false,
    enabled: true,
  },
  {
    name: "update_shipping_address",
    title: "Update the shipping address",
    description: "Changes the shipping address of an order that hasn't shipped yet.",
    input_schema: obj(
      {
        order_id: { type: "string", description: "The order number" },
        line1: { type: "string", description: "Street and number" },
        line2: { type: "string", description: "Apartment, suite or floor" },
        city: { type: "string", description: "City" },
        postal_code: { type: "string", description: "Postal code" },
        country: { type: "string", description: "Two-letter country code", enum: ["DE", "AT", "CH", "FR", "NL", "GB", "US"] },
      },
      ["order_id", "line1", "city", "postal_code", "country"],
    ),
    read_only: false,
    destructive: false,
    enabled: true,
    policy: "confirm",
    verified_only: true,
  },
  {
    name: "cancel_order",
    title: "Cancel an order",
    description: "Cancels an order that hasn't shipped and refunds the original payment method.",
    input_schema: obj({ order_id: { type: "string", description: "The order number" }, reason: { type: "string", description: "Why the customer cancels", enum: ["ordered_by_mistake", "found_cheaper", "too_slow", "other"] } }, ["order_id"]),
    read_only: false,
    destructive: true,
    enabled: false,
    policy: "confirm",
  },
];

const LINEAR_TOOLS: ToolDef[] = [
  {
    name: "search_issues",
    title: "Search issues",
    description: "Searches Linear issues by text, team or state and returns their identifiers, titles and states.",
    input_schema: obj({ query: { type: "string", description: "Words to search for" }, state: { type: "string", description: "Only issues in this state", enum: ["backlog", "todo", "in_progress", "done", "canceled"] } }, ["query"]),
    read_only: true,
    destructive: false,
  },
  {
    name: "create_issue",
    title: "Create an issue",
    description: "Creates a Linear issue in the given team with a title and a markdown description.",
    input_schema: obj({ team: { type: "string", description: "The team key, like FN" }, title: { type: "string", description: "A short title" }, description: { type: "string", description: "Markdown description" }, priority: { type: "integer", description: "0 none, 1 urgent to 4 low", minimum: 0, maximum: 4 } }, ["team", "title"]),
    read_only: false,
    destructive: false,
  },
];

function genericTools(): ToolDef[] {
  return [
    {
      name: "search",
      title: "Search",
      description: "Searches the service's records by text and returns the best matches.",
      input_schema: obj({ query: { type: "string", description: "Words to search for" }, limit: { type: "integer", description: "At most this many results", minimum: 1, maximum: 25 } }, ["query"]),
      read_only: true,
      destructive: false,
    },
    {
      name: "get_record",
      title: "Get a record",
      description: "Returns one record by its id.",
      input_schema: obj({ id: { type: "string", description: "The record id" } }, ["id"]),
      read_only: true,
      destructive: false,
    },
    {
      name: "create_record",
      title: "Create a record",
      description: "Creates a record with a title and optional notes.",
      input_schema: obj({ title: { type: "string", description: "A short title" }, notes: { type: "string", description: "Optional notes" } }, ["title"]),
      read_only: false,
      destructive: false,
    },
  ];
}

function catalogueFor(s: McpServer): { tools: ToolDef[]; info: McpServer["server_info"] } {
  const hay = `${s.name} ${s.url}`.toLowerCase();
  if (hay.includes("linear")) return { tools: LINEAR_TOOLS, info: { name: "linear-mcp", version: "2.3.0" } };
  if (hay.includes("shop")) return { tools: SHOP_TOOLS, info: { name: "margin-shop", version: "1.4.2" } };
  return { tools: genericTools(), info: { name: s.slug, version: "1.0.0" } };
}

/** Listed tools with their defaults, keeping the workspace's settings by name. */
function withSettings(defs: ToolDef[], previous: McpTool[]): McpTool[] {
  return defs.map((d) => {
    const old = previous.find((t) => t.name === d.name);
    return {
      ...d,
      enabled: old?.enabled ?? d.enabled ?? d.read_only,
      policy: old?.policy ?? d.policy ?? (d.read_only ? "auto" : "confirm"),
      verified_only: old?.verified_only ?? d.verified_only ?? false,
    };
  });
}

// ─── OAuth persistence across the redirect ────────────────────────────────

const KEY = "od-mock-mcp-oauth";
type Persisted = Record<string, string[]>; // workspace slug → server slugs

function loadOauth(): Persisted {
  try {
    return JSON.parse(sessionStorage.getItem(KEY) ?? "{}") as Persisted;
  } catch {
    return {};
  }
}
function rememberOauth(ws: WsData, slug: string) {
  try {
    const p = loadOauth();
    p[ws.workspace.slug] = [...new Set([...(p[ws.workspace.slug] ?? []), slug])];
    sessionStorage.setItem(KEY, JSON.stringify(p));
  } catch {
    /* private mode: lasts until reload */
  }
}

// ─── State ─────────────────────────────────────────────────────────────────

const out = (s: McpServer): McpServer => structuredClone(s);

function audit(ws: WsData, action: string, s: McpServer) {
  ws.audit.unshift({
    id: uuid(),
    actor: { type: "user", user: userRef(db.meId), api_key_id: null },
    action,
    entity_type: "mcp_server",
    entity_id: s.id,
    data: { name: s.name, url: s.url },
    ip: "84.152.33.17",
    created_at: nowIso(),
  });
}

function changed(ws: WsData, s: McpServer) {
  s.updated_at = nowIso();
  publish(ws, "ai_mcp_server.updated", { mcp_server: out(s) });
}

function oauthConnect(s: McpServer) {
  s.auth.oauth = { connected: true, scopes: ["read", "write"], expires_at: iso(Date.now() + HOUR) };
}

/** Finishes a (re)connect: the tools are listed, or the server wants sign-in. */
function finishConnect(ws: WsData, s: McpServer) {
  s.last_synced_at = nowIso();
  if (/fail|error|invalid/i.test(s.url)) {
    s.status = "error";
    s.error = CONNECT_ERROR;
    return;
  }
  s.error = null;
  if (s.auth.kind === "oauth" && !s.auth.oauth?.connected) {
    s.status = "needs_auth";
    s.server_info = null;
    s.tools = [];
    return;
  }
  const { tools, info } = catalogueFor(s);
  s.status = "connected";
  s.server_info = info;
  s.tools = withSettings(tools, s.tools);
  // Keep the workspace within the enabled-tools limit when a server brings many.
  let room = MAX_ENABLED_TOOLS - enabledTools(ws, s.id);
  for (const t of s.tools) {
    if (!t.enabled) continue;
    if (room > 0) room--;
    else t.enabled = false;
  }
}

/** Connects (initialize, tools/list) before answering, like the API. */
async function connect(ws: WsData, s: McpServer, ms = 1200) {
  await new Promise((r) => setTimeout(r, ms));
  finishConnect(ws, s);
}

const limitReached = (detail: string) => new HttpError(422, "limit_reached", detail);

function enabledTools(ws: WsData, exceptId?: string) {
  return ws.mcpServers.filter((s) => s.id !== exceptId).reduce((n, s) => n + s.tools.filter((t) => t.enabled).length, 0);
}

function find(ctx: Ctx): McpServer {
  const s = ctx.ws.mcpServers.find((x) => x.id === ctx.params.id);
  if (!s) throw notFound("MCP server");
  return s;
}

function guard(ctx: Ctx) {
  assertAiInstance();
  requireAiAdmin(ctx.ws, "Only admins can manage tools");
}

// ─── Validation ────────────────────────────────────────────────────────────

function checkName(v: unknown): string {
  const name = typeof v === "string" ? v.replace(/\s+/g, " ").trim() : "";
  if (!name || name.length > 60) throw invalid("Name must be 1 to 60 characters");
  if (/[<>]/.test(name)) throw invalid("Name can't contain < or >");
  return name;
}

function checkUrl(v: unknown): string {
  const url = typeof v === "string" ? v.trim() : "";
  let u: URL;
  try {
    u = new URL(url);
  } catch {
    throw new HttpError(422, "invalid_url", "Enter the server's URL, like https://mcp.example.com/mcp");
  }
  if (u.protocol !== "https:") throw new HttpError(422, "invalid_url", "MCP server URLs must use https");
  if (/^(localhost|127\.|10\.|192\.168\.|169\.254\.|\[::1\])/.test(u.hostname)) throw new HttpError(422, "invalid_url", "MCP server URLs can't point to private or local addresses");
  return u.toString();
}

/** The auth to store: secrets are checked, then only "a secret was given" is kept. */
function checkAuth(v: unknown, prev?: McpServer["auth"]): McpServer["auth"] {
  if (!isRecord(v)) throw invalid("auth must be an object with a kind");
  const kind = v.kind as McpAuthKind;
  if (!AUTH_KINDS.includes(kind)) throw invalid("auth.kind must be none, bearer, header or oauth");
  const sameKind = prev?.kind === kind;
  if (kind === "none") return { kind, has_secret: false };
  if (kind === "oauth") return { kind, has_secret: false, oauth: sameKind && prev?.oauth ? prev.oauth : { connected: false, scopes: [], expires_at: null } };
  if (kind === "bearer") {
    const token = typeof v.token === "string" ? v.token.trim() : "";
    if (!token && !(sameKind && prev?.has_secret)) throw invalid("Enter the bearer token");
    return { kind, has_secret: true };
  }
  const name = typeof v.header_name === "string" ? v.header_name.trim() : (sameKind ? prev?.header_name : "") ?? "";
  if (!/^[A-Za-z0-9!#$%&'*+.^_`|~-]{1,64}$/.test(name)) throw invalid("Enter a valid header name, like X-Api-Key");
  if (FORBIDDEN_HEADERS.has(name.toLowerCase()) || name.toLowerCase().startsWith("content-")) throw invalid(`${name} can't be used as the auth header`);
  const value = typeof v.header_value === "string" ? v.header_value.trim() : "";
  if (!value && !(sameKind && prev?.has_secret)) throw invalid("Enter the header value");
  return { kind, header_name: name, has_secret: true };
}

function uniqueSlug(ws: WsData, name: string): string {
  const base = (slugify(name).replace(/-+/g, "_") || "server").slice(0, 24).replace(/_+$/, "");
  let slug = base;
  for (let i = 2; ws.mcpServers.some((s) => s.slug === slug); i++) slug = `${base.slice(0, 24 - String(i).length - 1)}_${i}`;
  return slug;
}

// ─── Tool test results ─────────────────────────────────────────────────────

function testResult(server: McpServer, tool: McpTool, args: Record<string, unknown>): string {
  const id = String(args.order_id ?? "4821").replace(/^#/, "");
  const results: Record<string, unknown> = {
    lookup_order: {
      order_id: id,
      status: "shipped",
      placed_at: "2026-10-07T14:12:00Z",
      items: [{ sku: "FN-NB-A5-3", name: "Paper notebook bundle (3 x A5)", quantity: 1, price: "$48.00" }],
      total: "$48.00",
      carrier: "DHL",
      tracking_number: "JD014600006281935",
      estimated_delivery: "2026-10-14",
    },
    list_invoices: {
      invoices: [
        { id: "in_1Q8x2k", number: "FN-2026-0912", amount: "$96.00", status: "paid", date: "2026-10-01", pdf: "https://margin-shop.com/invoices/FN-2026-0912.pdf" },
        { id: "in_1PzT7c", number: "FN-2026-0841", amount: "$96.00", status: "paid", date: "2026-09-01", pdf: "https://margin-shop.com/invoices/FN-2026-0841.pdf" },
      ],
    },
    update_shipping_address: { ok: true, order_id: id, address: { line1: args.line1, city: args.city, postal_code: args.postal_code, country: args.country } },
    cancel_order: { ok: true, order_id: id, status: "cancelled", refund: { amount: "$48.00", to: "card ending 4242" } },
    search_issues: { issues: [{ id: "FN-482", title: "PDF export stalls at 90% for large notebooks", state: "in_progress" }, { id: "FN-455", title: "Sync paused on iPad after 6.2 update", state: "todo" }] },
    create_issue: { id: "FN-497", url: "https://linear.app/margin/issue/FN-497", title: args.title },
    search: { results: [{ id: "rec_7Hc2", title: `First match for "${String(args.query ?? "")}"` }], total: 1 },
    get_record: { id: args.id, title: "Sample record", updated_at: "2026-10-10T09:30:00Z" },
    create_record: { id: "rec_9Kd4", title: args.title, created: true },
  };
  return JSON.stringify(results[tool.name] ?? { ok: true, server: server.slug }, null, 2);
}

// ─── Seed ──────────────────────────────────────────────────────────────────

function seedServers(ws: WsData) {
  const oauthDone = loadOauth()[ws.workspace.slug] ?? [];
  const shop: McpServer = {
    id: uuid(),
    name: "Shop",
    slug: "shop",
    url: "https://mcp.margin-shop.com/mcp",
    enabled: true,
    auth: { kind: "bearer", has_secret: true },
    status: "connected",
    error: null,
    server_info: { name: "margin-shop", version: "1.4.2" },
    tools: withSettings(SHOP_TOOLS, []),
    last_synced_at: iso(NOW - 25 * MIN),
    created_at: iso(NOW - 34 * DAY),
    updated_at: iso(NOW - 25 * MIN),
  };
  const linear: McpServer = {
    id: uuid(),
    name: "Linear",
    slug: "linear",
    url: "https://mcp.linear.app/mcp",
    enabled: true,
    auth: { kind: "oauth", has_secret: false, oauth: { connected: false, scopes: [], expires_at: null } },
    status: "needs_auth",
    error: null,
    server_info: null,
    tools: [],
    last_synced_at: null,
    created_at: iso(NOW - 2 * DAY),
    updated_at: iso(NOW - 2 * DAY),
  };
  ws.mcpServers = [shop, linear];
  for (const s of ws.mcpServers) {
    if (s.auth.kind === "oauth" && oauthDone.includes(s.slug)) {
      oauthConnect(s);
      finishConnect(ws, s);
    }
  }
}

// ─── Routes ────────────────────────────────────────────────────────────────

export function registerMcp() {
  const margin = db.workspaces[0];
  if (margin && !margin.mcpServers.length) seedServers(margin);

  get("/v1/ai/mcp-servers", (ctx) => {
    guard(ctx);
    return { data: ctx.ws.mcpServers.map(out), next_cursor: null };
  });

  post("/v1/ai/mcp-servers", async (ctx) => {
    guard(ctx);
    const name = checkName(ctx.body.name);
    const url = checkUrl(ctx.body.url);
    const auth = checkAuth(ctx.body.auth ?? { kind: "none" });
    if (ctx.ws.mcpServers.length >= MAX_SERVERS) throw limitReached(`A workspace can connect at most ${MAX_SERVERS} MCP servers`);
    const now = nowIso();
    const s: McpServer = {
      id: uuid(),
      name,
      slug: uniqueSlug(ctx.ws, name),
      url,
      enabled: true,
      auth,
      status: "connecting",
      error: null,
      server_info: null,
      tools: [],
      last_synced_at: null,
      created_at: now,
      updated_at: now,
    };
    await connect(ctx.ws, s);
    if (ctx.ws.mcpServers.length >= MAX_SERVERS) throw limitReached(`A workspace can connect at most ${MAX_SERVERS} MCP servers`);
    s.slug = uniqueSlug(ctx.ws, name);
    ctx.ws.mcpServers.push(s);
    publish(ctx.ws, "ai_mcp_server.created", { mcp_server: out(s) });
    audit(ctx.ws, "ai.mcp_server_created", s);
    return respond(201, out(s));
  });

  patch("/v1/ai/mcp-servers/:id", async (ctx) => {
    guard(ctx);
    const s = find(ctx);
    const b = ctx.body;
    // Validate everything on a copy, so a refused patch changes nothing.
    const next = structuredClone(s);
    if ("name" in b) next.name = checkName(b.name);
    if ("url" in b) next.url = checkUrl(b.url);
    if ("enabled" in b) {
      if (typeof b.enabled !== "boolean") throw invalid("enabled must be true or false");
      next.enabled = b.enabled;
    }
    if ("auth" in b) next.auth = checkAuth(b.auth, s.auth);
    if ("tools" in b) {
      if (!isRecord(b.tools)) throw invalid("tools must map tool names to settings");
      for (const [name, p] of Object.entries(b.tools)) {
        const t = next.tools.find((x) => x.name === name);
        if (!t) throw new HttpError(422, "unknown_tool", `${name} isn't one of this server's tools`);
        if (!isRecord(p)) throw invalid(`Settings for ${name} must be an object`);
        if ("enabled" in p) {
          if (typeof p.enabled !== "boolean") throw invalid("enabled must be true or false");
          t.enabled = p.enabled;
        }
        if ("policy" in p) {
          if (p.policy !== "auto" && p.policy !== "confirm") throw invalid("policy must be auto or confirm");
          t.policy = p.policy;
        }
        if ("verified_only" in p) {
          if (typeof p.verified_only !== "boolean") throw invalid("verified_only must be true or false");
          t.verified_only = p.verified_only;
        }
      }
      if (enabledTools(ctx.ws, s.id) + next.tools.filter((t) => t.enabled).length > MAX_ENABLED_TOOLS) {
        throw limitReached(`At most ${MAX_ENABLED_TOOLS} tools can be on across all servers. Turn some off first.`);
      }
    }
    const reconnect = next.url !== s.url || next.auth.kind !== s.auth.kind || ("auth" in b && next.auth.has_secret);
    Object.assign(s, next);
    if (reconnect) await connect(ctx.ws, s, 900);
    changed(ctx.ws, s);
    audit(ctx.ws, "ai.mcp_server_updated", s);
    return out(s);
  });

  del("/v1/ai/mcp-servers/:id", (ctx) => {
    guard(ctx);
    const s = find(ctx);
    ctx.ws.mcpServers = ctx.ws.mcpServers.filter((x) => x !== s);
    publish(ctx.ws, "ai_mcp_server.deleted", { mcp_server_id: s.id });
    audit(ctx.ws, "ai.mcp_server_deleted", s);
    return respond(204);
  });

  post("/v1/ai/mcp-servers/:id/sync", async (ctx) => {
    guard(ctx);
    const s = find(ctx);
    await connect(ctx.ws, s, 900);
    changed(ctx.ws, s);
    return out(s);
  });

  post("/v1/ai/mcp-servers/:id/oauth/start", (ctx) => {
    guard(ctx);
    const s = find(ctx);
    if (s.auth.kind !== "oauth") throw new HttpError(409, "not_oauth", "This server doesn't use OAuth");
    if (/deny/i.test(s.name)) return { url: "/settings/ai/tools?error=access_denied" };
    // The provider's consent screen is skipped: the server is connected when the browser comes back.
    rememberOauth(ctx.ws, s.slug);
    oauthConnect(s);
    finishConnect(ctx.ws, s);
    changed(ctx.ws, s);
    audit(ctx.ws, "ai.mcp_server_connected", s);
    return { url: `/settings/ai/tools?connected=${s.id}` };
  });

  get("/v1/ai/mcp-servers/oauth/callback", (ctx) => {
    const s = ctx.ws.mcpServers.find((x) => x.id === ctx.query.get("state"));
    return respond(302, undefined, { location: s ? `/settings/ai/tools?connected=${s.id}` : "/settings/ai/tools?error=invalid_state" });
  });

  post("/v1/ai/mcp-servers/:id/tools/:name/test", async (ctx) => {
    guard(ctx);
    const s = find(ctx);
    const tool = s.tools.find((t) => t.name === ctx.params.name);
    if (!tool) throw notFound("Tool");
    if (s.status !== "connected") throw new HttpError(409, "conflict", "Connect this server before testing its tools");
    const args = isRecord(ctx.body.arguments) ? ctx.body.arguments : {};
    const started = Date.now();
    await new Promise((r) => setTimeout(r, 250 + Math.random() * 450));
    const required = Array.isArray(tool.input_schema.required) ? (tool.input_schema.required as string[]) : [];
    const missing = required.filter((k) => args[k] === undefined || args[k] === null || args[k] === "");
    const ms = Date.now() - started;
    if (missing.length) return { content: `Missing required argument${missing.length > 1 ? "s" : ""}: ${missing.join(", ")}`, is_error: true, ms };
    return { content: testResult(s, tool, args), is_error: false, ms };
  });
}

/** An enabled, connected tool of the workspace by server slug and tool name (the playground uses it). */
export function mcpTool(ws: WsData, slug: string, name: string): { server: McpServer; tool: McpTool } | null {
  const server = ws.mcpServers.find((s) => s.slug === slug && s.enabled && s.status === "connected");
  const tool = server?.tools.find((t) => t.name === name);
  return server && tool ? { server, tool } : null;
}
