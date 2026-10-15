// Label and custom field rules shared by the seed and the handlers
// (contracts/api.md "Labels and custom fields").

import type {
  CustomField,
  FieldEntity,
  FieldOp,
  FieldValue,
  FieldValues,
  LabelColor,
  LabelScope,
  Tag,
  TagRef,
} from "@/lib/api/types";
import { LABEL_COLORS } from "@/lib/api/types";
import { db, type WsData } from "./db";
import { publish } from "./gateway";
import { HttpError, invalid } from "./router";
import { nowIso, uuid } from "./util";

export const ALL_SCOPES: LabelScope[] = ["tickets", "contacts", "companies"];
export const MAX_LABELS_PER_RECORD = 50;
export const MAX_FIELDS_PER_ENTITY = 100;
export const MAX_OPTIONS = 200;

/** A palette colour picked from the name (FNV-1a, as the API does). */
export function defaultColor(name: string): LabelColor {
  let h = 0x811c9dc5;
  for (const b of new TextEncoder().encode(name.toLowerCase())) {
    h ^= b;
    h = Math.imul(h, 0x01000193) >>> 0;
  }
  return LABEL_COLORS[h % LABEL_COLORS.length];
}

const TAG_CHARS = /^[\p{L}\p{N} _\-./:&+#]+$/u;

export function cleanTagName(raw: string): string {
  const name = raw.normalize("NFC").split(/\s+/).filter(Boolean).join(" ");
  if (!name) throw invalid("label name is required");
  if ([...name].length > 50) throw invalid("label names must be 50 characters or less");
  if (!TAG_CHARS.test(name)) throw invalid("label names may contain letters, numbers, spaces and - _ . / : & + #");
  return name;
}

export function tagRef(t: Tag): TagRef {
  return { id: t.id, name: t.name, color: t.color, group_id: t.group_id };
}

export function findTagByName(ws: WsData, name: string): Tag | undefined {
  const n = name.trim().toLowerCase();
  return ws.tags.find((x) => x.name.toLowerCase() === n);
}

export function makeTag(ws: WsData, name: string, init: Partial<Omit<Tag, "id" | "name" | "usage">> = {}): Tag {
  const at = nowIso();
  const position = init.position ?? ws.tags.filter((x) => x.group_id === (init.group_id ?? null)).length;
  const tag: Tag = {
    id: uuid(),
    name,
    color: init.color ?? defaultColor(name),
    description: init.description ?? "",
    group_id: init.group_id ?? null,
    position,
    archived: init.archived ?? false,
    applies_to: init.applies_to ?? [...ALL_SCOPES],
    usage: { tickets: 0, contacts: 0, companies: 0 },
    created_at: init.created_at ?? at,
    updated_at: init.updated_at ?? at,
  };
  ws.tags.push(tag);
  return tag;
}

/** The Tag with live usage counts. */
export function tagOut(ws: WsData, t: Tag): Tag {
  const has = (tags: TagRef[]) => tags.some((x) => x.id === t.id);
  t.usage = {
    tickets: ws.tickets.filter((x) => !ws.deletedTickets.has(x.id) && has(x.tags)).length,
    contacts: ws.contacts.filter((x) => has(x.tags)).length,
    companies: ws.companies.filter((x) => has(x.tags)).length,
  };
  return t;
}

export function groupPos(ws: WsData, groupId: string | null): number {
  if (!groupId) return Number.MAX_SAFE_INTEGER;
  return ws.tagGroups.find((g) => g.id === groupId)?.position ?? Number.MAX_SAFE_INTEGER;
}

/** Group position, then position, then name: the order labels are listed and embedded in. */
export function compareTags(ws: WsData) {
  return (a: TagRef, b: TagRef) => {
    const ta = ws.tags.find((x) => x.id === a.id);
    const tb = ws.tags.find((x) => x.id === b.id);
    return (
      groupPos(ws, a.group_id) - groupPos(ws, b.group_id) ||
      (ta?.position ?? 0) - (tb?.position ?? 0) ||
      a.name.localeCompare(b.name)
    );
  };
}

/** Refresh the embedded copies of labels after a rename, recolour or regroup. */
export function syncTagRefs(ws: WsData) {
  const fix = (tags: TagRef[]) =>
    tags
      .map((r) => ws.tags.find((x) => x.id === r.id))
      .filter((x): x is Tag => !!x)
      .map(tagRef)
      .sort(compareTags(ws));
  for (const t of ws.tickets) t.tags = fix(t.tags);
  for (const c of ws.contacts) c.tags = fix(c.tags);
  for (const c of ws.companies) c.tags = fix(c.tags);
}

export type TagOps = { replace?: string[]; add?: string[]; remove?: string[] };

/**
 * The next label set of a record. Unknown names are created when `create` is
 * set and refused (422 unknown_label) otherwise, unless `lenient`. In an
 * exclusive group the last label named wins.
 */
export function applyLabels(
  ws: WsData,
  scope: LabelScope,
  current: TagRef[],
  ops: TagOps,
  opts: { create: boolean; lenient?: boolean; publishEvents?: boolean },
): { next: TagRef[]; added: string[]; removed: string[] } {
  const resolve = (names: string[]) => {
    const out: Tag[] = [];
    for (const raw of names) {
      let name: string;
      try {
        name = cleanTagName(raw);
      } catch (e) {
        if (opts.lenient) continue;
        throw e;
      }
      let tag = findTagByName(ws, name);
      if (!tag) {
        if (!opts.create) {
          if (opts.lenient) continue;
          throw new HttpError(422, "unknown_label", `There is no label named "${name}"; ask an admin to create it.`);
        }
        tag = makeTag(ws, name);
        if (opts.publishEvents !== false) publish(ws, "tag.created", { tag: tagOut(ws, tag) });
      }
      if (!tag.applies_to.includes(scope)) {
        if (opts.lenient) continue;
        throw invalid(`the label "${tag.name}" does not apply to ${scope}`);
      }
      const i = out.findIndex((x) => x.id === tag!.id);
      if (i >= 0) out.splice(i, 1);
      out.push(tag);
    }
    return out;
  };
  const byId = (id: string) => ws.tags.find((x) => x.id === id);
  if (ops.replace && ops.replace.length > MAX_LABELS_PER_RECORD) throw invalid(`a record can have at most ${MAX_LABELS_PER_RECORD} labels`);
  let seq: Tag[] = ops.replace ? resolve(ops.replace) : current.map((r) => byId(r.id)).filter((x): x is Tag => !!x);
  for (const t of resolve(ops.add ?? [])) {
    seq = seq.filter((x) => x.id !== t.id);
    seq.push(t);
  }
  const drop = new Set((ops.remove ?? []).map((x) => x.trim().toLowerCase()));
  seq = seq.filter((x) => !drop.has(x.name.toLowerCase()));
  const taken = new Set<string>();
  const keep: Tag[] = [];
  for (let i = seq.length - 1; i >= 0; i--) {
    const t = seq[i];
    const g = t.group_id ? ws.tagGroups.find((x) => x.id === t.group_id) : undefined;
    if (g?.exclusive) {
      if (taken.has(g.id)) continue;
      taken.add(g.id);
    }
    keep.unshift(t);
  }
  if (keep.length > MAX_LABELS_PER_RECORD) throw invalid(`a record can have at most ${MAX_LABELS_PER_RECORD} labels`);
  const next = keep.map(tagRef).sort(compareTags(ws));
  const added = next.filter((x) => !current.some((y) => y.id === x.id)).map((x) => x.name).sort();
  const removed = current.filter((x) => !next.some((y) => y.id === x.id)).map((x) => x.name).sort();
  return { next, added, removed };
}

/** Whether the signed-in member may create labels by naming them. */
export function canCreateLabels(ws: WsData): boolean {
  const role = ws.members.find((m) => m.user_id === db.meId)?.role;
  if (role === "owner" || role === "admin") return true;
  return ws.workspace.settings.label_creation !== "admins";
}

// ─── Custom fields ──────────────────────────────────────────────────────────

export function fieldDefs(ws: WsData, entity: FieldEntity): CustomField[] {
  return ws.customFields.filter((f) => f.entity === entity).sort((a, b) => a.position - b.position);
}

export function fieldEmpty(v: unknown): boolean {
  return v === undefined || v === null || v === "" || (Array.isArray(v) && v.length === 0);
}

const DATE = /^\d{4}-\d{2}-\d{2}$/;
const EMAIL = /^[^\s@<>]+@[^\s@<>]+\.[^\s@<>]+$/;

/** A validated value, or null to clear. Errors locate `body.fields.<key>`. */
export function cleanFieldValue(f: CustomField, v: unknown): FieldValue | null {
  const bad = (msg: string) => invalid(`fields.${f.key}: ${msg}`);
  if (v === null) return null;
  switch (f.type) {
    case "text":
    case "long_text":
    case "url":
    case "email": {
      if (typeof v !== "string") throw bad("value must be text");
      const s = f.type === "long_text" ? v.trim() : v.trim().replace(/\s+/g, " ");
      if (!s) return null;
      if ([...s].length > (f.type === "long_text" ? 10000 : f.type === "url" ? 2000 : 1000)) throw bad("value is too long");
      if (f.type === "url" && !/^https?:\/\/\S+$/i.test(s)) throw bad("value must be an http or https URL");
      if (f.type === "email" && !EMAIL.test(s)) throw bad("value must be an email address");
      return f.type === "email" ? s.toLowerCase() : s;
    }
    case "number":
      if (typeof v !== "number" || !Number.isFinite(v)) throw bad("value must be a number");
      return v;
    case "checkbox":
      if (typeof v !== "boolean") throw bad("value must be true or false");
      return v;
    case "date":
      if (typeof v !== "string" || !DATE.test(v) || Number.isNaN(Date.parse(v))) throw bad("value must be a date like 2026-10-11");
      return v;
    case "select":
      if (typeof v !== "string") throw bad("value must be an option id");
      if (!v) return null;
      if (!f.options.some((o) => o.id === v)) throw bad(`unknown option "${v}"`);
      return v;
    case "multi_select": {
      if (!Array.isArray(v) || v.some((x) => typeof x !== "string")) throw bad("value must be a list of option ids");
      const ids = [...new Set(v as string[])];
      for (const id of ids) if (!f.options.some((o) => o.id === id)) throw bad(`unknown option "${id}"`);
      return ids.length ? f.options.filter((o) => ids.includes(o.id)).map((o) => o.id) : null;
    }
    case "user":
      if (typeof v !== "string") throw bad("value must be a user id");
      if (!v) return null;
      if (!db.users.some((u) => u.id === v)) throw bad("unknown member");
      return v;
  }
}

export type FieldChange = { key: string; name: string; from: FieldValue | null; to: FieldValue | null };

/** Merge a patch into stored values: unknown keys and archived fields are 422; null clears. */
export function mergeFields(ws: WsData, entity: FieldEntity, cur: FieldValues, patch: Record<string, unknown>): { next: FieldValues; changes: FieldChange[] } {
  const defs = fieldDefs(ws, entity);
  const next: FieldValues = { ...cur };
  const changes: FieldChange[] = [];
  for (const [key, raw] of Object.entries(patch)) {
    const f = defs.find((x) => x.key === key);
    if (!f) throw invalid(`fields.${key}: there is no ${entity} field with this key`);
    const v = cleanFieldValue(f, raw);
    const from = cur[key] ?? null;
    if (JSON.stringify(from) === JSON.stringify(v)) continue;
    if (f.archived) throw invalid(`fields.${key}: the field is archived`);
    if (v === null) delete next[key];
    else next[key] = v;
    changes.push({ key, name: f.name, from, to: v });
  }
  return { next, changes };
}

/** 422 field_required when a required-on-solve ticket field is empty. */
export function checkRequiredOnSolve(ws: WsData, values: FieldValues) {
  const missing = fieldDefs(ws, "ticket").filter((f) => f.required_on_solve && !f.archived && (f.type === "checkbox" ? values[f.key] !== true : fieldEmpty(values[f.key])));
  if (missing.length) {
    throw new HttpError(422, "field_required", `Fill in ${missing.map((f) => f.name).join(", ")} before solving this ticket.`);
  }
}

export type FieldFilterIn = { key: string; op: FieldOp; value: string };

export function opsFor(type: CustomField["type"]): FieldOp[] {
  switch (type) {
    case "text":
    case "long_text":
    case "url":
    case "email":
      return ["eq", "ne", "contains", "not_contains", "set", "not_set"];
    case "number":
    case "date":
      return ["eq", "ne", "gt", "gte", "lt", "lte", "set", "not_set"];
    case "checkbox":
      return ["eq", "ne", "set", "not_set"];
    case "select":
    case "user":
      return ["eq", "ne", "any", "none", "set", "not_set"];
    case "multi_select":
      return ["any", "all", "none", "eq", "ne", "set", "not_set"];
  }
}

export function defaultOp(type: CustomField["type"]): FieldOp {
  return type === "select" || type === "multi_select" || type === "user" ? "any" : "eq";
}

/** `field.<key>` and `field.<key>[op]` query params. */
export function fieldFiltersFromQuery(ws: WsData, entity: FieldEntity, query: URLSearchParams): FieldFilterIn[] {
  const out: FieldFilterIn[] = [];
  for (const [k, value] of query.entries()) {
    const m = /^field\.([a-z][a-z0-9_]{0,39})(?:\[([a-z_]+)\])?$/.exec(k);
    if (!m) continue;
    const f = fieldDefs(ws, entity).find((x) => x.key === m[1]);
    if (!f) throw invalid(`field.${m[1]}: there is no ${entity} field with this key`);
    const op = (m[2] ?? defaultOp(f.type)) as FieldOp;
    if (!opsFor(f.type).includes(op)) throw invalid(`field.${f.key} supports the ops ${opsFor(f.type).join(", ")}`);
    out.push({ key: f.key, op, value });
  }
  return out;
}

const list = (v: string) => v.split(",").map((x) => x.trim()).filter(Boolean);

export function matchField(f: CustomField, op: FieldOp, value: string, have: FieldValue | undefined, meId = db.meId): boolean {
  const empty = fieldEmpty(have);
  if (op === "set") return !empty;
  if (op === "not_set") return empty;
  switch (f.type) {
    case "text":
    case "long_text":
    case "url":
    case "email": {
      const h = String(have ?? "").toLowerCase();
      const w = value.toLowerCase();
      if (op === "eq") return h === w;
      if (op === "ne") return h !== w;
      if (op === "contains") return h.includes(w);
      return !h.includes(w);
    }
    case "number":
    case "date": {
      if (empty) return op === "ne";
      const h = f.type === "number" ? Number(have) : Date.parse(String(have));
      const w = f.type === "number" ? Number(value) : Date.parse(value);
      return { eq: h === w, ne: h !== w, gt: h > w, gte: h >= w, lt: h < w, lte: h <= w }[op as "eq"] ?? false;
    }
    case "checkbox": {
      const h = have === true;
      return op === "eq" ? h === (value === "true") : h !== (value === "true");
    }
    case "select":
    case "user": {
      const ids = list(value).map((x) => (x === "me" ? meId : x));
      const h = String(have ?? "");
      if (op === "eq" || op === "any") return ids.includes(h);
      return !ids.includes(h);
    }
    case "multi_select": {
      const ids = list(value);
      const h = Array.isArray(have) ? have : [];
      if (op === "any") return ids.some((x) => h.includes(x));
      if (op === "all") return ids.every((x) => h.includes(x));
      if (op === "none") return !ids.some((x) => h.includes(x));
      // eq means "has that option".
      const has = ids.every((x) => h.includes(x));
      return op === "eq" ? has : !has;
    }
  }
}

export function matchFields(ws: WsData, entity: FieldEntity, values: FieldValues, filters: FieldFilterIn[] | undefined): boolean {
  for (const ff of filters ?? []) {
    const f = fieldDefs(ws, entity).find((x) => x.key === ff.key);
    if (f && !matchField(f, ff.op, ff.value, values[f.key])) return false;
  }
  return true;
}

/** Plain-text values for search (`q` matches field values too). */
export function fieldsText(values: FieldValues): string {
  return Object.values(values)
    .flatMap((v) => (Array.isArray(v) ? v : [v]))
    .filter((v) => typeof v === "string" || typeof v === "number")
    .join(" ");
}

/** A sortable key for `sort=field.<key>`: numbers and dates compare, the rest as text; empty sorts lowest. */
export function fieldSortValue(f: CustomField, v: FieldValue | undefined): number | string | null {
  if (fieldEmpty(v)) return null;
  if (f.type === "number") return Number(v);
  if (f.type === "date") return Date.parse(String(v));
  if (f.type === "checkbox") return v ? 1 : 0;
  if (f.type === "select") return f.options.findIndex((o) => o.id === v);
  return String(Array.isArray(v) ? v.join(",") : v).toLowerCase();
}

/** The labels every new workspace starts with (internal/core/workspaces.go). */
export function seedDefaultLabels(ws: WsData) {
  const at = nowIso();
  const type = { id: uuid(), name: "Type", color: null, exclusive: true, position: 0, tag_count: 0, created_at: at, updated_at: at };
  ws.tagGroups.push(type);
  const defaults: [string, LabelColor, string, boolean][] = [
    ["Question", "blue", "How do I, can I, where is", true],
    ["Problem", "orange", "Something is not working as expected", true],
    ["Bug", "red", "A defect in the product", true],
    ["Feature request", "violet", "Asks for something new", true],
    ["Billing", "green", "Invoices, payments and plans", true],
    ["VIP", "amber", "A customer to look after closely", false],
    ["Feedback", "teal", "Praise, complaints and suggestions", false],
  ];
  defaults.forEach(([name, color, description, grouped], i) =>
    makeTag(ws, name, { color, description, group_id: grouped ? type.id : null, position: i }),
  );
}
