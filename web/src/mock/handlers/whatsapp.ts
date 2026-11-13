// WhatsApp connections and a WhatsApp ticket's window (contracts/whatsapp.md). The demo Twilio
// account takes the auth token "demo" with any Account SID; anything else is refused.

import type { Ticket, WhatsAppProvider, WhatsAppTemplate } from "@/lib/api/types";
import type { WsData } from "../db";
import { lastInbound, whatsappConnection } from "../whatsapp";
import { del, get, HttpError, invalid, notFound, patch, post, respond } from "../router";
import { publish } from "../gateway";
import { nowIso } from "../util";

const WINDOW = 24 * 3600_000;

const providers: WhatsAppProvider[] = [
  {
    key: "twilio",
    name: "Twilio",
    fields: [
      { name: "account_sid", label: "Account SID", secret: false },
      { name: "auth_token", label: "Auth token", secret: true },
    ],
    docs: "https://www.twilio.com/docs/whatsapp/self-sign-up",
  },
];

const templates: WhatsAppTemplate[] = [
  { id: "HX1", name: "follow_up", language: "en", body: "Hi {{1}}, did our answer about {{2}} help?", variables: ["1", "2"] },
  { id: "HX2", name: "order_update", language: "en", body: "Hi {{1}}, your order has shipped.", variables: ["1"] },
];

const waiting = {
  health_status: "action_required",
  health_code: "webhook_missing",
  health_detail: "No message has come in yet. Add the webhook address in the provider, then send a message to the number.",
  last_event_at: null,
} as const;

const fill = (body: string, vars: Record<string, string>) => body.replace(/\{\{\s*([0-9A-Za-z_]+)\s*\}\}/g, (_, k: string) => vars[k] ?? "");

/** The number as E.164, "" when it has no country code. */
const normalize = (s: string) => {
  const n = s.replace(/^whatsapp:/, "").replace(/[\s().-]/g, "").replace(/^00/, "+");
  return /^\+[0-9]{6,15}$/.test(n) ? n : "";
};

/** What a WhatsApp reply goes out as: its words, or a template once the window is closed. */
export function whatsappReply(ws: WsData, t: Ticket, body: Record<string, unknown>, html: string): string {
  if (!ws.whatsapp.length) throw new HttpError(422, "whatsapp_disconnected", "The WhatsApp number this conversation came in on is no longer connected.");
  const last = lastInbound(ws, t.id);
  if (last === null) throw new HttpError(422, "whatsapp_no_conversation", "This ticket has no WhatsApp message from the customer to answer.");
  const tpl = body.whatsapp_template as { id?: string; variables?: Record<string, string> } | undefined;
  if (!tpl) {
    if (Date.now() - last >= WINDOW) throw new HttpError(422, "whatsapp_window_closed", "The customer last wrote more than 24 hours ago, so WhatsApp only allows an approved template message.");
    return html;
  }
  const found = templates.find((x) => x.id === tpl.id);
  if (!found) throw invalid("This template is not an approved template of the connected number.");
  if (!found.variables.every((v) => tpl.variables?.[v]?.trim())) throw invalid("Fill in every variable of the template (up to 1024 characters each).");
  return `<p>${fill(found.body, tpl.variables ?? {})}</p>`;
}

/** Sent, then delivered to the phone, then read: what the provider reports. */
export function simulateWhatsAppDelivery(ws: WsData, t: Ticket, msgId: string) {
  const step = (status: "sent" | "delivered" | "read", after: number) =>
    setTimeout(() => {
      const m = ws.messages.find((x) => x.id === msgId);
      if (!m || m.delivery_status === "failed") return;
      m.delivery_status = status;
      m.delivery_error = "";
      publish(ws, "message.updated", { ticket_id: t.id, message: m });
    }, after);
  step("sent", 1200);
  step("delivered", 3200);
  step("read", 6500);
}

export function registerWhatsApp() {
  get("/v1/whatsapp/providers", () => ({ data: providers }));
  get("/v1/whatsapp/connections", (ctx) => ({ data: ctx.ws.whatsapp }));
  post("/v1/whatsapp/connections", (ctx) => {
    const b = ctx.body as { provider?: string; name?: string; sender?: string; brand_id?: string; credentials?: Record<string, string> };
    const p = providers.find((x) => x.key === b.provider);
    if (!p) throw invalid("Unknown WhatsApp provider.");
    const sender = normalize(b.sender ?? "");
    if (!sender) throw invalid("Enter the WhatsApp number with its country code, like +4915123456789.");
    if (!p.fields.every((f) => b.credentials?.[f.name]?.trim())) throw invalid("Provide every required credential.");
    if (b.credentials?.auth_token !== "demo") throw new HttpError(422, "whatsapp_credentials_refused", "The provider refused these credentials.");
    if (ctx.ws.whatsapp.some((c) => c.sender === sender)) throw new HttpError(409, "conflict", "This workspace already has a connection for that number.");
    const row = whatsappConnection({ name: b.name?.trim() || "Twilio", sender, brand_id: b.brand_id ?? null, created_at: nowIso(), health_checked_at: nowIso(), ...waiting });
    ctx.ws.whatsapp.push(row);
    return respond(201, row);
  });
  patch("/v1/whatsapp/connections/:id", (ctx) => {
    const row = ctx.ws.whatsapp.find((c) => c.id === ctx.params.id);
    if (!row) throw notFound("WhatsApp connection");
    const b = ctx.body as { name?: string; brand_id?: string; clear_brand?: boolean; reuse_days?: number; credentials?: Record<string, string> };
    if (b.name) row.name = b.name;
    if (b.reuse_days !== undefined) row.reuse_days = b.reuse_days;
    if (b.clear_brand) row.brand_id = null;
    else if (b.brand_id) row.brand_id = b.brand_id;
    if (b.credentials && b.credentials.auth_token !== "demo") throw new HttpError(422, "whatsapp_credentials_refused", "The provider refused these credentials.");
    return row;
  });
  post("/v1/whatsapp/connections/:id/test", (ctx) => {
    const row = ctx.ws.whatsapp.find((c) => c.id === ctx.params.id);
    if (!row) throw notFound("WhatsApp connection");
    if (!row.last_event_at) Object.assign(row, { ...waiting, status: "connected", health_checked_at: nowIso() });
    else Object.assign(row, { health_status: "healthy", health_code: "receiving", health_detail: "Messages are sent to opendesk.", status: "connected", health_checked_at: nowIso() });
    return row;
  });
  del("/v1/whatsapp/connections/:id", (ctx) => {
    const before = ctx.ws.whatsapp.length;
    ctx.ws.whatsapp = ctx.ws.whatsapp.filter((c) => c.id !== ctx.params.id);
    if (ctx.ws.whatsapp.length === before) throw notFound("WhatsApp connection");
    return respond(204, null);
  });
  get("/v1/tickets/:id/whatsapp", (ctx) => {
    const t = ctx.ws.tickets.find((x) => x.id === ctx.params.id);
    if (!t) throw notFound("Ticket");
    const last = lastInbound(ctx.ws, t.id);
    const c = ctx.ws.whatsapp[0];
    if (t.channel !== "whatsapp" || last === null || !c) return { connection: null, window: { open: false, closes_at: null } };
    const closes = last + WINDOW;
    return { connection: { id: c.id, name: c.name, sender: c.sender }, window: { open: Date.now() < closes, closes_at: new Date(closes).toISOString() } };
  });
  get("/v1/tickets/:id/whatsapp/templates", (ctx) => {
    if (!ctx.ws.tickets.some((x) => x.id === ctx.params.id)) throw notFound("Ticket");
    return { data: templates };
  });
}
