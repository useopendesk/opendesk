// The demo workspace's WhatsApp number and two customers who wrote to it (contracts/whatsapp.md):
// one whose 24-hour window is open, one whose window has closed.

import type { Contact, UserRef, WhatsAppConnection } from "@/lib/api/types";
import type { WsData } from "./db";
import { addMessage, newTicket } from "./tickets";
import { uuid } from "./util";

const HOUR = 3600_000;
const iso = (ms: number) => new Date(ms).toISOString();

export function whatsappConnection(over: Partial<WhatsAppConnection> = {}): WhatsAppConnection {
  const id = uuid();
  return {
    id,
    provider: "twilio",
    name: "Margin WhatsApp",
    sender: "+14155550100",
    account_name: "Margin",
    status: "connected",
    last_error: "",
    health_status: "healthy",
    health_code: "receiving",
    health_detail: "Messages are sent to opendesk.",
    health_checked_at: iso(Date.now() - 2 * HOUR),
    brand_id: null,
    reuse_days: 7,
    last_event_at: iso(Date.now() - 20 * 60_000),
    webhook_url: `https://api.opendesk.sh/v1/whatsapp/hooks/twilio/${id}`,
    status_url: `https://api.opendesk.sh/v1/whatsapp/hooks/twilio/${id}/status`,
    created_at: iso(Date.now() - 12 * 24 * HOUR),
    ...over,
  };
}

function customer(ws: WsData, name: string, phone: string): Contact {
  const c: Contact = {
    id: uuid(),
    email: null,
    emails: [],
    name,
    phone,
    avatar_url: null,
    avatar_source: null,
    company: null,
    external_id: null,
    notes: "",
    custom: {},
    tags: [],
    fields: {},
    blocked: false,
    ticket_count: 1,
    created_at: iso(Date.now() - 3 * 24 * HOUR),
    updated_at: iso(Date.now() - HOUR),
  };
  ws.contacts.push(c);
  return c;
}

/** A message of a conversation: from the customer or not, its words and, for ours, how far it got. */
type Line = [inbound: boolean, text: string] | [inbound: boolean, text: string, status: "read" | "delivered" | "sent"];

export function seedWhatsApp(ws: WsData, agent: UserRef) {
  ws.whatsapp = [whatsappConnection()];
  const talk = (name: string, phone: string, subject: string, hoursAgo: number, lines: Line[]) => {
    const c = customer(ws, name, phone);
    const start = Date.now() - hoursAgo * HOUR;
    const t = newTicket(ws, {
      subject,
      contact: { id: c.id, email: null, name: c.name, avatar_url: null },
      channel: "whatsapp",
      mailbox_id: null,
      assignee: agent,
      created_at: iso(start),
    });
    lines.forEach(([inbound, text, ...rest], i) => {
      const status = rest[0];
      addMessage(
        ws,
        t,
        { kind: "reply", direction: inbound ? "inbound" : "outbound", html: `<p>${text}</p>`, user: inbound ? null : agent, at: iso(start + i * 4 * 60_000), delivery: inbound ? "none" : (status ?? "read") },
        { publish: false },
      );
    });
    return t;
  };
  talk("Mateo Ibarra-Silva", "+34 612 345 678", "Do you ship to Austria?", 1.2, [
    [true, "Hi! Do you ship to Austria?"],
    [false, "Hello Mateo, yes we do. Delivery takes 3 to 5 working days.", "read"],
    [true, "Great, and is the VAT included?"],
  ]);
  talk("Ana Souza-Lindqvist", "+55 11 91234 5678", "My invoice is wrong", 30, [
    [true, "My invoice is wrong, the VAT number is missing."],
    [false, "Thanks Ana, I will send a corrected invoice today.", "delivered"],
  ]);
}

/** The customer's last WhatsApp message on a ticket: the 24-hour window counts from it. */
export function lastInbound(ws: WsData, ticketId: string): number | null {
  const times = ws.messages.filter((m) => m.ticket_id === ticketId && m.direction === "inbound" && m.kind === "reply").map((m) => Date.parse(m.created_at));
  return times.length ? Math.max(...times) : null;
}
