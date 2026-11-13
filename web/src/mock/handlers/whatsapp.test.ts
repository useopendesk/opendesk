import { describe, expect, it } from "vitest";
import type { Message, Ticket, WhatsAppConnection, WhatsAppTicketState } from "@/lib/api/types";
import { db } from "../db";
import { installMock } from "../index";
import { dispatch } from "../router";

async function call(method: string, path: string, body?: unknown) {
  const ws = db.workspaces[0]?.workspace.id;
  const r = await dispatch(`http://mock${path}`, {
    method,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    headers: ws ? { "OD-Workspace": ws } : {},
  });
  return { status: r.status, body: r.status === 204 ? null : await r.json() };
}

const creds = (token: string) => ({ account_sid: "AC00000000000000000000000000000000", auth_token: token });

describe("mock WhatsApp", () => {
  installMock();

  const ticketOf = async (name: string): Promise<Ticket> => {
    const list = (await call("GET", "/v1/tickets?channel=whatsapp")).body.data as Ticket[];
    return list.find((t) => t.contact?.name === name)!;
  };

  it("seeds a connected number and two WhatsApp tickets, one with its window closed", async () => {
    const list = (await call("GET", "/v1/whatsapp/connections")).body.data as WhatsAppConnection[];
    expect(list).toHaveLength(1);
    expect(list[0]).toMatchObject({ provider: "twilio", sender: "+14155550100", health_status: "healthy" });
    const open = (await call("GET", `/v1/tickets/${(await ticketOf("Mateo Ibarra-Silva")).id}/whatsapp`)).body as WhatsAppTicketState;
    const closed = (await call("GET", `/v1/tickets/${(await ticketOf("Ana Souza-Lindqvist")).id}/whatsapp`)).body as WhatsAppTicketState;
    expect(open.window.open).toBe(true);
    expect(open.connection?.sender).toBe("+14155550100");
    expect(closed.window.open).toBe(false);
  });

  it("connects a number, refusing wrong credentials, a number without its country code and a duplicate", async () => {
    const at = (b: object) => call("POST", "/v1/whatsapp/connections", { provider: "twilio", ...b });
    expect((await at({ sender: "+49 151 2345 6789", credentials: creds("wrong") })).status).toBe(422);
    expect((await at({ sender: "0151 2345", credentials: creds("demo") })).status).toBe(422);
    const made = await at({ sender: "+49 151 2345 6789", credentials: creds("demo") });
    expect(made.status).toBe(201);
    expect(made.body).toMatchObject({ sender: "+4915123456789", health_code: "webhook_missing" });
    expect((await at({ sender: "+4915123456789", credentials: creds("demo") })).status).toBe(409);
    expect((await call("POST", `/v1/whatsapp/connections/${made.body.id}/test`)).body.health_code).toBe("webhook_missing");
    expect((await call("DELETE", `/v1/whatsapp/connections/${made.body.id}`)).status).toBe(204);
  });

  it("answers freely while the window is open", async () => {
    const t = await ticketOf("Mateo Ibarra-Silva");
    const sent = await call("POST", `/v1/tickets/${t.id}/messages`, { kind: "reply", body_html: "<p>Yes, VAT is included.</p>" });
    expect(sent.status).toBe(201);
    expect((sent.body as Message).delivery_status).toBe("pending");
  });

  it("asks for a template once the window is closed, and sends the template's words", async () => {
    const t = await ticketOf("Ana Souza-Lindqvist");
    const url = `/v1/tickets/${t.id}/messages`;
    const free = await call("POST", url, { kind: "reply", body_html: "<p>Hello</p>" });
    expect(free.status).toBe(422);
    expect(free.body.code).toBe("whatsapp_window_closed");
    const list = (await call("GET", `/v1/tickets/${t.id}/whatsapp/templates`)).body.data as { id: string }[];
    expect(list.length).toBeGreaterThan(0);
    expect((await call("POST", url, { kind: "reply", body_html: "<p>x</p>", whatsapp_template: { id: "HX1", variables: { "1": "Ana" } } })).status).toBe(422);
    const ok = await call("POST", url, { kind: "reply", body_html: "<p>x</p>", whatsapp_template: { id: "HX1", variables: { "1": "Ana", "2": "your invoice" } } });
    expect(ok.status).toBe(201);
    expect((ok.body as Message).body_html).toBe("<p>Hi Ana, did our answer about your invoice help?</p>");
  });
});
