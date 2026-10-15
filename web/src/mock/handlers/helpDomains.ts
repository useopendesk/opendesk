// A help center's custom domain in mock mode (contracts/teams.md "Help center
// addresses"). Connecting checks right away and finds nothing; each later check,
// by hand or on the background schedule, gets one step further:
//
//   pending (txt_missing) → pending (cname_missing) → pending (certificate_pending) → active
//
// A hostname containing "nodns" never finds its records, to show the waiting state.
// Every change publishes brand.updated with the full brand, as the API does.

import type { Brand, HelpDomain } from "@/lib/api/types";
import { appOrigin, config } from "@/lib/config";
import { looksLikeHostname } from "@/lib/helpCenter";
import { db, userRef, type WsData } from "../db";
import { publish } from "../gateway";
import { HttpError, del, forbidden, notFound, post, put, respond, type Ctx } from "../router";
import { brandOut, isAdmin } from "../teams";
import { nowIso, randomToken, uuid } from "../util";

/** Background checks, as the help_domain.check job runs them (much faster here). */
const AUTO_CHECK_MS = 15_000;
const CHECKS_PER_MINUTE = 6;

const timers = new Map<string, ReturnType<typeof setTimeout>>();
const recentChecks = new Map<string, number[]>();

function stop(brandId: string) {
  const t = timers.get(brandId);
  if (t) clearTimeout(t);
  timers.delete(brandId);
}

/** The CNAME target: custom.<HELP_DOMAIN> with Cloudflare, else the app's host. */
function cnameTarget(): string {
  if (config.helpDomain) return `custom.${config.helpDomain}`;
  try {
    return new URL(appOrigin()).hostname;
  } catch {
    return "app.opendesk.sh";
  }
}

function newDomain(hostname: string): HelpDomain {
  return {
    hostname,
    status: "pending",
    problem: "txt_missing",
    records: [
      { type: "TXT", name: `_opendesk-verify.${hostname}`, value: `opendesk-verify=${randomToken(24).replace(/[-_]/g, "x")}`, ok: false },
      { type: "CNAME", name: hostname, value: cnameTarget(), ok: false },
    ],
    checked_at: null,
    verified_at: null,
    created_at: nowIso(),
  };
}

/** One check: the next step of the flow. */
function advance(d: HelpDomain) {
  const now = nowIso();
  d.checked_at = now;
  if (d.hostname.includes("nodns")) {
    d.records = d.records.map((r) => ({ ...r, ok: false }));
    d.problem = "txt_missing";
    return;
  }
  const [txt, cname] = d.records;
  if (d.status === "active") return;
  if (d.status === "failed") d.status = "pending";
  if (!txt.ok) {
    d.records = [{ ...txt, ok: true }, { ...cname, ok: false }];
    d.problem = "cname_missing";
  } else if (!cname.ok) {
    d.records = [txt, { ...cname, ok: true }];
    d.verified_at = now;
    d.problem = "certificate_pending";
  } else {
    d.status = "active";
    d.problem = "";
  }
}

function changed(ws: WsData, b: Brand) {
  b.updated_at = nowIso();
  publish(ws, "brand.updated", { brand: brandOut(ws, b) });
}

/** The next background check, while the domain is not active yet. */
function schedule(ws: WsData, b: Brand) {
  stop(b.id);
  // Tests drive every step by hand.
  if (import.meta.env.MODE === "test") return;
  const d = b.help_center.domain;
  if (!d || d.status === "active") return;
  timers.set(
    b.id,
    setTimeout(() => {
      timers.delete(b.id);
      const cur = b.help_center.domain;
      if (!ws.brands.includes(b) || cur !== d) return;
      advance(d);
      changed(ws, b);
      schedule(ws, b);
    }, AUTO_CHECK_MS),
  );
}

function findBrand(ctx: Ctx): Brand {
  const b = ctx.ws.brands.find((x) => x.id === ctx.params.id);
  if (!b) throw notFound("Brand");
  return b;
}

function requireAdmin(ws: WsData) {
  if (!isAdmin(ws, db.meId)) throw forbidden("Only admins can change this");
}

function audit(ws: WsData, action: string, b: Brand, data: Record<string, unknown>) {
  ws.audit.unshift({ id: uuid(), actor: { type: "user", user: userRef(db.meId), api_key_id: null }, action, entity_type: "brand", entity_id: b.id, data, ip: "84.152.33.17", created_at: nowIso() });
}

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms));

export function registerHelpDomainRoutes() {
  put("/v1/brands/:id/help-domain", async (ctx) => {
    requireAdmin(ctx.ws);
    const b = findBrand(ctx);
    const raw = typeof ctx.body.hostname === "string" ? ctx.body.hostname : "";
    const hostname = raw.trim().toLowerCase().replace(/\.$/, "");
    if (!looksLikeHostname(hostname) || /^\d+(\.\d+){3}$/.test(hostname)) {
      throw new HttpError(422, "hostname_invalid", "Enter a domain name like help.acme.com, without https:// or a path.");
    }
    if (hostname.split(".").length < 3) {
      throw new HttpError(422, "hostname_invalid", `Use a subdomain such as help.${hostname}, not ${hostname} itself.`);
    }
    const own = [config.helpDomain, "opendesk.sh", "opendeskhelp.com"].filter(Boolean);
    if (own.some((h) => hostname === h || hostname.endsWith(`.${h}`))) {
      throw new HttpError(422, "hostname_invalid", "This address belongs to opendesk. Use a domain of your own.");
    }
    if (b.help_center.domain?.hostname === hostname) return brandOut(ctx.ws, b);
    const taken = db.workspaces.some((w) => w.brands.some((x) => x !== b && x.help_center.domain?.hostname === hostname && x.help_center.domain.verified_at));
    if (taken) throw new HttpError(409, "domain_taken", "Another help center already uses this domain.");
    // The API checks the records right away, which takes a moment.
    if (import.meta.env.MODE !== "test") await wait(1200);
    const d = newDomain(hostname);
    d.checked_at = nowIso();
    audit(ctx.ws, "brand.help_domain_set", b, { hostname, previous: b.help_center.domain?.hostname ?? null });
    b.help_center.domain = d;
    recentChecks.delete(b.id);
    changed(ctx.ws, b);
    schedule(ctx.ws, b);
    return brandOut(ctx.ws, b);
  });

  post("/v1/brands/:id/help-domain/check", async (ctx) => {
    requireAdmin(ctx.ws);
    const b = findBrand(ctx);
    const d = b.help_center.domain;
    if (!d) throw notFound("Custom domain");
    const now = Date.now();
    const recent = (recentChecks.get(b.id) ?? []).filter((t) => now - t < 60_000);
    if (recent.length >= CHECKS_PER_MINUTE) throw new HttpError(429, "rate_limited", "Checked several times just now. Try again in a minute.");
    recentChecks.set(b.id, [...recent, now]);
    if (import.meta.env.MODE !== "test") await wait(700);
    advance(d);
    changed(ctx.ws, b);
    schedule(ctx.ws, b);
    return respond(200, brandOut(ctx.ws, b));
  });

  del("/v1/brands/:id/help-domain", (ctx) => {
    requireAdmin(ctx.ws);
    const b = findBrand(ctx);
    if (!b.help_center.domain) throw notFound("Custom domain");
    stop(b.id);
    recentChecks.delete(b.id);
    audit(ctx.ws, "brand.help_domain_removed", b, { hostname: b.help_center.domain.hostname });
    b.help_center.domain = null;
    changed(ctx.ws, b);
    return respond(204);
  });
}

/** A deleted brand releases its domain. */
export function forgetHelpDomain(brandId: string) {
  stop(brandId);
  recentChecks.delete(brandId);
}
