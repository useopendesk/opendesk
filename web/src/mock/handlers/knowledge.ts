// Knowledge sources for the widget agent (contracts/agent.md "Knowledge
// sources"). Crawls and file extraction are simulated with timers that move a
// source through pending → syncing → ready, publishing ai_knowledge.updated at
// each step.

import type { KnowledgeDoc, KnowledgeSource, KnowledgeWebsite } from "@/lib/api/types";
import { db, userRef, type WsData } from "../db";
import { publish } from "../gateway";
import { HttpError, del, get, invalid, notFound, paginate, patch, post, respond, type Ctx } from "../router";
import { DAY, HOUR, NOW, isRecord, iso, nowIso, uuid } from "../util";
import { assertAiInstance } from "./ai";

const MAX_SOURCES = 50;
const MAX_DOCS = 5000;
const MAX_CHARS = 20_000_000;
const MAX_FILE = 20 << 20;
const NO_TEXT = "No text found in this file";
const UNREACHABLE = "We couldn't reach this site. Check the address and try again.";
const FILE_TYPES: Record<string, string> = {
  pdf: "application/pdf",
  docx: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
  txt: "text/plain",
  md: "text/markdown",
  markdown: "text/markdown",
  html: "text/html",
  htm: "text/html",
  csv: "text/csv",
};

// ─── Generated documents ───────────────────────────────────────────────────

/** A small deterministic PRNG per source, so documents read the same on every load. */
function prng(seed: string) {
  let h = 2166136261;
  for (const c of seed) h = Math.imul(h ^ c.charCodeAt(0), 16777619);
  return () => {
    h = Math.imul(h ^ (h >>> 15), 2246822507);
    h = Math.imul(h ^ (h >>> 13), 3266489909);
    return ((h ^= h >>> 16) >>> 0) / 4294967296;
  };
}

const DOC_TOPICS = [
  "Getting started with Margin", "Create your first notebook", "Invite your team", "Share a note with a link", "Password-protect shared links",
  "Export a notebook to PDF", "Export to Markdown", "Import from Evernote", "Import from Notion", "Sync across devices", "Work offline",
  "Fix sync paused on iPad", "Search inside images (OCR)", "Tags and filters", "Templates", "Keyboard shortcuts", "Version history",
  "Restore a deleted note", "Set up SSO with Okta", "Set up SSO with Google Workspace", "SCIM provisioning", "Audit logs", "Roles and permissions",
  "Change your plan", "Update your payment method", "Download invoices", "Cancel your subscription", "Refunds", "Student and teacher discounts",
  "Data retention", "Two-factor authentication", "Delete your account", "Integrations: Slack", "Integrations: Google Drive", "API keys", "Webhooks",
  "Attachments and file limits", "Embed a note on your site", "Dark mode", "Accessibility", "Mobile widgets", "Apple Pencil support",
];
const BLOG_TOPICS = [
  "What's new in Margin 6.2", "How architects use Margin on site", "Faster sync, explained", "Our approach to privacy", "Offline mode, rebuilt",
  "A better PDF export", "Five templates for field research", "Margin for classrooms", "Meet the new search", "Behind the scenes of OCR",
  "Planning a renovation with shared notebooks", "Release notes: September", "Release notes: August", "Notes from our team retreat",
];

function makeDocs(s: KnowledgeSource, count: number, at = nowIso()): KnowledgeDoc[] {
  const r = prng(`${s.id}:${s.name}`);
  if (s.kind === "snippet") return [{ id: uuid(), title: s.snippet!.question, url: null, chars: s.snippet!.question.length + s.snippet!.answer.length, updated_at: at }];
  if (s.kind === "file") {
    const base = s.file!.filename.replace(/\.[^.]+$/, "");
    return Array.from({ length: count }, (_, i) => ({ id: uuid(), title: count > 1 ? `${base} (part ${i + 1} of ${count})` : base, url: null, chars: 1200 + Math.floor(r() * 5800), updated_at: at }));
  }
  const site = s.website!.url.replace(/\/+$/, "");
  const blog = /blog/.test(site);
  const topics = blog ? BLOG_TOPICS : DOC_TOPICS;
  const prefix = s.website!.include[0] ?? "";
  return Array.from({ length: count }, (_, i) => {
    const topic = topics[i % topics.length];
    const round = Math.floor(i / topics.length);
    const title = round ? `${topic} (${["advanced", "reference", "FAQ", "examples", "admins"][(round - 1) % 5]})` : topic;
    const slug = title.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
    return { id: uuid(), title: blog ? title : `${title} | Margin Docs`, url: `${site}${prefix}/${slug}`, chars: 900 + Math.floor(r() * 7400), updated_at: at };
  });
}

/** How many documents a crawl or extraction ends with. */
function targetCount(s: KnowledgeSource): number {
  if (s.kind === "snippet") return 1;
  if (s.kind === "file") return s.file!.content_type === "application/pdf" ? 1 + Math.floor(prng(s.file!.filename)() * 4) : 1;
  const r = prng(s.website!.url)();
  return Math.min(s.website!.max_pages, 12 + Math.floor(r * 30));
}

function recount(ws: WsData, s: KnowledgeSource) {
  const docs = ws.knowledgeDocs[s.id] ?? [];
  s.doc_count = docs.length;
  s.char_count = docs.reduce((n, d) => n + d.chars, 0);
}

// ─── Simulated sync ────────────────────────────────────────────────────────

const timers = new Map<string, ReturnType<typeof setTimeout>[]>();

const out = (s: KnowledgeSource): KnowledgeSource => structuredClone(s);

function changed(ws: WsData, s: KnowledgeSource) {
  s.updated_at = nowIso();
  publish(ws, "ai_knowledge.updated", { source: out(s) });
}

function stop(id: string) {
  for (const t of timers.get(id) ?? []) clearTimeout(t);
  timers.delete(id);
}

function later(ws: WsData, s: KnowledgeSource, ms: number, fn: () => void) {
  const t = setTimeout(() => {
    if (!ws.knowledge.includes(s)) return;
    fn();
    changed(ws, s);
  }, ms);
  timers.set(s.id, [...(timers.get(s.id) ?? []), t]);
}

/**
 * pending → syncing (documents arrive in steps) → ready, or error. Websites
 * take about 6 s, files about 2.5 s. `from` keeps documents already crawled.
 */
function runSync(ws: WsData, s: KnowledgeSource, opts: { fast?: boolean; from?: number; startMs?: number } = {}) {
  stop(s.id);
  const fast = opts.fast ?? s.kind === "file";
  const steps = fast ? 2 : 5;
  const stepMs = fast ? 700 : 1050;
  let t = opts.startMs ?? 700;
  // Extracting the same file again finds the same (lack of) text.
  const failFile = s.kind === "file" && (/empty/i.test(s.file!.filename) || s.error === NO_TEXT);
  const failWebsite = s.kind === "website" && /invalid|unreachable/.test(s.website!.url);
  if (opts.from === undefined) {
    s.status = "pending";
    s.error = null;
  }
  const start = opts.from ?? 0;
  const target = Math.max(targetCount(s), start ? start + 18 : 0);
  // A resumed crawl keeps the documents it already has.
  const kept = start ? (ws.knowledgeDocs[s.id] ?? []).slice(0, start) : [];
  const all = [...kept, ...makeDocs(s, target).slice(kept.length)];
  later(ws, s, t, () => {
    s.status = "syncing";
    if (start === 0) ws.knowledgeDocs[s.id] = [];
    recount(ws, s);
  });
  if (failWebsite || failFile) {
    later(ws, s, t + stepMs, () => {
      s.status = "error";
      s.error = failFile ? NO_TEXT : UNREACHABLE;
      ws.knowledgeDocs[s.id] = [];
      recount(ws, s);
      s.last_synced_at = nowIso();
    });
    return;
  }
  for (let i = 1; i <= steps; i++) {
    t += stepMs;
    const upto = Math.round(start + ((target - start) * i) / steps);
    const last = i === steps;
    later(ws, s, t, () => {
      ws.knowledgeDocs[s.id] = all.slice(0, Math.max(upto, (ws.knowledgeDocs[s.id] ?? []).length));
      recount(ws, s);
      if (last) {
        s.status = "ready";
        s.last_synced_at = nowIso();
      }
    });
  }
}

// ─── Validation ────────────────────────────────────────────────────────────

function checkName(v: unknown): string {
  const name = typeof v === "string" ? v.replace(/\s+/g, " ").trim() : "";
  if (!name || name.length > 100) throw invalid("Name must be 1 to 100 characters");
  if (/[<>]/.test(name)) throw invalid("Name can't contain < or >");
  return name;
}

function paths(key: string, v: unknown): string[] {
  if (!Array.isArray(v) || v.some((x) => typeof x !== "string")) throw invalid(`${key} must be a list of paths`);
  const list = (v as string[]).map((x) => x.trim()).filter(Boolean);
  if (list.length > 20) throw invalid(`Add at most 20 ${key} paths`);
  for (const p of list) if (!p.startsWith("/") || /\s/.test(p)) throw invalid(`${p} isn't a path. Paths start with /, like /guides`);
  return [...new Set(list)];
}

function checkWebsite(v: unknown, prev?: KnowledgeWebsite): KnowledgeWebsite {
  if (!isRecord(v)) throw invalid("website must be an object with a url");
  const w: KnowledgeWebsite = prev ? { ...prev } : { url: "", max_pages: 100, include: [], exclude: [], recrawl: "weekly" };
  if ("url" in v || !prev) {
    let u: URL;
    try {
      u = new URL(typeof v.url === "string" ? v.url.trim() : "");
    } catch {
      throw invalid("Enter the site's address, like https://docs.example.com");
    }
    if (u.protocol !== "https:") throw invalid("Website addresses must use https");
    if (/^(localhost|127\.|10\.|192\.168\.|169\.254\.)/.test(u.hostname)) throw invalid("Website addresses can't point to private or local addresses");
    w.url = u.toString().replace(/\/$/, "");
  }
  if ("max_pages" in v) {
    const n = v.max_pages;
    if (typeof n !== "number" || !Number.isInteger(n) || n < 1 || n > 500) throw invalid("max_pages must be a whole number from 1 to 500");
    w.max_pages = n;
  }
  if ("include" in v) w.include = paths("include", v.include);
  if ("exclude" in v) w.exclude = paths("exclude", v.exclude);
  if ("recrawl" in v) {
    if (v.recrawl !== "off" && v.recrawl !== "weekly") throw invalid("recrawl must be off or weekly");
    w.recrawl = v.recrawl;
  }
  return w;
}

function checkSnippet(v: unknown): { question: string; answer: string } {
  if (!isRecord(v)) throw invalid("snippet must have a question and an answer");
  const question = typeof v.question === "string" ? v.question.replace(/\s+/g, " ").trim() : "";
  const answer = typeof v.answer === "string" ? v.answer.trim() : "";
  if (question.length < 3 || question.length > 300) throw invalid("The question must be 3 to 300 characters");
  if (!answer || answer.length > 4000) throw invalid("The answer must be 1 to 4,000 characters");
  return { question, answer };
}

function checkLimits(ws: WsData) {
  const limit = (detail: string) => new HttpError(422, "knowledge_limit", detail);
  if (ws.knowledge.length >= MAX_SOURCES) throw limit(`A workspace can have at most ${MAX_SOURCES} knowledge sources`);
  const all = Object.values(ws.knowledgeDocs).flat();
  if (all.length >= MAX_DOCS) throw limit(`A workspace can have at most ${MAX_DOCS.toLocaleString("en")} knowledge documents`);
  if (all.reduce((n, d) => n + d.chars, 0) >= MAX_CHARS) throw limit("A workspace can have at most 20 million characters of knowledge");
}

function role(ws: WsData) {
  return ws.members.find((m) => m.user_id === db.meId)?.role ?? "viewer";
}
function requireAdmin(ws: WsData) {
  const r = role(ws);
  if (r !== "owner" && r !== "admin") throw new HttpError(403, "forbidden", "Only admins can change knowledge sources");
}

function find(ctx: Ctx): KnowledgeSource {
  const s = ctx.ws.knowledge.find((x) => x.id === ctx.params.id);
  if (!s) throw notFound("Knowledge source");
  return s;
}

function audit(ws: WsData, action: string, s: KnowledgeSource) {
  ws.audit.unshift({
    id: uuid(),
    actor: { type: "user", user: userRef(db.meId), api_key_id: null },
    action,
    entity_type: "knowledge_source",
    entity_id: s.id,
    data: { name: s.name, kind: s.kind },
    ip: "84.152.33.17",
    created_at: nowIso(),
  });
}

function blank(kind: KnowledgeSource["kind"], name: string): KnowledgeSource {
  const now = nowIso();
  return { id: uuid(), kind, name, status: "pending", error: null, enabled: true, doc_count: 0, char_count: 0, last_synced_at: null, created_at: now, updated_at: now };
}

function defaultName(url: string) {
  const u = new URL(url);
  const path = u.pathname.replace(/\/$/, "");
  return `${u.hostname.replace(/^www\./, "")}${path}`.slice(0, 100);
}

// ─── Seed ──────────────────────────────────────────────────────────────────

function seedSources(ws: WsData) {
  const at = (ms: number) => iso(NOW - ms);
  const src = (s: Partial<KnowledgeSource> & Pick<KnowledgeSource, "kind" | "name">, docs: number, syncedAgo: number | null): KnowledgeSource => {
    const row: KnowledgeSource = {
      ...blank(s.kind, s.name),
      status: "ready",
      ...s,
      last_synced_at: syncedAgo === null ? null : at(syncedAgo),
      created_at: at(Math.max(syncedAgo ?? 0, 0) + 20 * DAY),
      updated_at: at(syncedAgo ?? 0),
    };
    ws.knowledgeDocs[row.id] = docs ? makeDocs(row, docs, row.updated_at) : [];
    recount(ws, row);
    return row;
  };
  const docsSite = src({ kind: "website", name: "Margin docs", website: { url: "https://docs.margin.app", max_pages: 200, include: ["/guides"], exclude: [], recrawl: "weekly" } }, 121, 2 * DAY + 3 * HOUR);
  const blog = src({ kind: "website", name: "Margin blog", status: "syncing", website: { url: "https://margin.app/blog", max_pages: 100, include: [], exclude: ["/blog/tag"], recrawl: "weekly" } }, 23, null);
  const refund = src({ kind: "file", name: "Refund policy 2026.pdf", file: { filename: "Refund policy 2026.pdf", content_type: "application/pdf", size_bytes: 184_320 } }, 2, 9 * DAY);
  const prices = src({ kind: "file", name: "price-list.csv", status: "error", error: NO_TEXT, file: { filename: "price-list.csv", content_type: "text/csv", size_bytes: 2_048 } }, 0, 6 * DAY);
  const student = src(
    {
      kind: "snippet",
      name: "Do you offer student discounts?",
      snippet: { question: "Do you offer student discounts?", answer: "Yes. Students and teachers get **50% off** the Team plan with a school email address. Apply at [margin.app/education](https://margin.app/education); approval takes about a day." },
    },
    1,
    12 * DAY,
  );
  const refundTime = src(
    {
      kind: "snippet",
      name: "How long does a refund take?",
      snippet: { question: "How long does a refund take?", answer: "Refunds go back to the original payment method. They usually show within 5 to 10 business days after we approve them." },
    },
    1,
    4 * DAY,
  );
  const legacy = src({ kind: "website", name: "Old help center", enabled: false, website: { url: "https://help-legacy.margin.app", max_pages: 100, include: [], exclude: [], recrawl: "off" } }, 41, 64 * DAY);
  ws.knowledge = [docsSite, blog, refund, prices, student, refundTime, legacy];
  // The blog crawl is still running: it finishes a few seconds after load.
  runSync(ws, blog, { from: 23, startMs: 2700 });
}

// ─── Routes ────────────────────────────────────────────────────────────────

/** A Q&A snippet created like POST /v1/ai/knowledge (Improve answer, contracts/agent.md). */
export function addSnippet(ws: WsData, question: string, answer: string): KnowledgeSource {
  checkLimits(ws);
  const snippet = checkSnippet({ question, answer });
  const s: KnowledgeSource = { ...blank("snippet", snippet.question.slice(0, 100)), snippet, status: "ready", last_synced_at: nowIso() };
  ws.knowledge.push(s);
  ws.knowledgeDocs[s.id] = makeDocs(s, 1);
  recount(ws, s);
  publish(ws, "ai_knowledge.created", { source: out(s) });
  audit(ws, "ai.knowledge_created", s);
  return s;
}

export function registerKnowledge() {
  const margin = db.workspaces[0];
  if (margin && !margin.knowledge.length) seedSources(margin);

  get("/v1/ai/knowledge", (ctx) => {
    assertAiInstance();
    const kind = ctx.query.get("kind");
    return { data: ctx.ws.knowledge.filter((s) => !kind || s.kind === kind).map(out), next_cursor: null };
  });

  post("/v1/ai/knowledge", (ctx) => {
    assertAiInstance();
    requireAdmin(ctx.ws);
    const b = ctx.body;
    if (b.kind !== "website" && b.kind !== "snippet") throw invalid("kind must be website or snippet. Upload files to /v1/ai/knowledge/files.");
    checkLimits(ctx.ws);
    let s: KnowledgeSource;
    if (b.kind === "website") {
      const website = checkWebsite(b.website);
      s = { ...blank("website", b.name !== undefined && b.name !== "" ? checkName(b.name) : defaultName(website.url)), website };
      ctx.ws.knowledge.push(s);
      ctx.ws.knowledgeDocs[s.id] = [];
      runSync(ctx.ws, s);
    } else {
      const snippet = checkSnippet(b.snippet);
      s = { ...blank("snippet", b.name !== undefined && b.name !== "" ? checkName(b.name) : snippet.question.slice(0, 100)), snippet, status: "ready", last_synced_at: nowIso() };
      ctx.ws.knowledge.push(s);
      ctx.ws.knowledgeDocs[s.id] = makeDocs(s, 1);
      recount(ctx.ws, s);
    }
    publish(ctx.ws, "ai_knowledge.created", { source: out(s) });
    audit(ctx.ws, "ai.knowledge_created", s);
    return respond(201, out(s));
  });

  post("/v1/ai/knowledge/files", (ctx) => {
    assertAiInstance();
    requireAdmin(ctx.ws);
    const f = ctx.form?.get("file");
    if (!(f instanceof File)) throw invalid("Choose a file to upload");
    const ext = (f.name.split(".").pop() ?? "").toLowerCase();
    const type = FILE_TYPES[ext] ?? Object.values(FILE_TYPES).find((t) => t === f.type.split(";")[0]);
    if (!type) throw invalid("Upload a PDF, DOCX, TXT, Markdown, HTML or CSV file");
    if (f.size > MAX_FILE) throw invalid("Files can be at most 20 MB");
    checkLimits(ctx.ws);
    const rawName = ctx.form?.get("name");
    const name = typeof rawName === "string" && rawName.trim() ? checkName(rawName) : checkName(f.name);
    const s: KnowledgeSource = { ...blank("file", name), file: { filename: f.name, content_type: type, size_bytes: f.size } };
    ctx.ws.knowledge.push(s);
    ctx.ws.knowledgeDocs[s.id] = [];
    runSync(ctx.ws, s);
    publish(ctx.ws, "ai_knowledge.created", { source: out(s) });
    audit(ctx.ws, "ai.knowledge_created", s);
    return respond(201, out(s));
  });

  get("/v1/ai/knowledge/:id", (ctx) => {
    assertAiInstance();
    return out(find(ctx));
  });

  patch("/v1/ai/knowledge/:id", (ctx) => {
    assertAiInstance();
    requireAdmin(ctx.ws);
    const s = find(ctx);
    const b = ctx.body;
    const next = structuredClone(s);
    if ("name" in b) next.name = checkName(b.name);
    if ("enabled" in b) {
      if (typeof b.enabled !== "boolean") throw invalid("enabled must be true or false");
      next.enabled = b.enabled;
    }
    if ("website" in b) {
      if (s.kind !== "website") throw invalid("Only website sources have website settings");
      next.website = checkWebsite(b.website, s.website);
    }
    if ("snippet" in b) {
      if (s.kind !== "snippet") throw invalid("Only snippets have a question and an answer");
      next.snippet = checkSnippet(b.snippet);
    }
    Object.assign(s, next);
    if ("snippet" in b) {
      s.last_synced_at = nowIso();
      ctx.ws.knowledgeDocs[s.id] = makeDocs(s, 1);
      recount(ctx.ws, s);
    }
    changed(ctx.ws, s);
    return out(s);
  });

  del("/v1/ai/knowledge/:id", (ctx) => {
    assertAiInstance();
    requireAdmin(ctx.ws);
    const s = find(ctx);
    stop(s.id);
    ctx.ws.knowledge = ctx.ws.knowledge.filter((x) => x !== s);
    delete ctx.ws.knowledgeDocs[s.id];
    publish(ctx.ws, "ai_knowledge.deleted", { source_id: s.id });
    audit(ctx.ws, "ai.knowledge_deleted", s);
    return respond(204);
  });

  post("/v1/ai/knowledge/:id/sync", (ctx) => {
    assertAiInstance();
    requireAdmin(ctx.ws);
    const s = find(ctx);
    if (s.status === "pending" || s.status === "syncing") throw new HttpError(409, "conflict", "This source is already syncing");
    if (s.kind === "snippet") {
      s.last_synced_at = nowIso();
      changed(ctx.ws, s);
      return out(s);
    }
    runSync(ctx.ws, s);
    changed(ctx.ws, s);
    return out(s);
  });

  get("/v1/ai/knowledge/:id/docs", (ctx) => {
    assertAiInstance();
    const s = find(ctx);
    return paginate(ctx, ctx.ws.knowledgeDocs[s.id] ?? [], 50);
  });

  del("/v1/ai/knowledge/:id/docs/:doc_id", (ctx) => {
    assertAiInstance();
    requireAdmin(ctx.ws);
    const s = find(ctx);
    const docs = ctx.ws.knowledgeDocs[s.id] ?? [];
    if (!docs.some((d) => d.id === ctx.params.doc_id)) throw notFound("Document");
    ctx.ws.knowledgeDocs[s.id] = docs.filter((d) => d.id !== ctx.params.doc_id);
    recount(ctx.ws, s);
    changed(ctx.ws, s);
    return respond(204);
  });
}

/** Documents of enabled sources that synced at least once (a recrawl keeps answering), for retrieval. */
export function knowledgeDocs(ws: WsData): { source: KnowledgeSource; doc: KnowledgeDoc }[] {
  return ws.knowledge.filter((s) => s.enabled && s.last_synced_at !== null && s.status !== "error").flatMap((s) => (ws.knowledgeDocs[s.id] ?? []).map((doc) => ({ source: s, doc })));
}
