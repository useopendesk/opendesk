import { describe, expect, it } from "vitest";
import type { Mailbox, MailboxCause } from "@/lib/api/types";
import { installMock } from "../index";
import { dispatch } from "../router";
import { db } from "../db";
import catalogue from "../mailCauses.json";
import { mailCause, refusal } from "./mailhost";

async function call(method: string, path: string, body?: unknown) {
  const ws = db.workspaces[0]?.workspace.id;
  const r = await dispatch(`http://mock${path}`, { method, body: body ? JSON.stringify(body) : undefined, headers: ws ? { "OD-Workspace": ws } : {} });
  return { status: r.status, body: r.status === 204 ? null : await r.json() };
}

// mailCauses.json is the API's catalogue (internal/email/mailcause), checked by its Go test.
describe("mock mail causes", () => {
  it("is the API's catalogue", () => {
    const codes = catalogue.map((c) => c.code);
    expect(new Set(codes).size).toBe(codes.length);
    for (const c of catalogue as MailboxCause[]) {
      expect(c.code).toMatch(/^[a-z0-9_]+$/);
      expect(c.title.length).toBeGreaterThan(0);
      expect(c.fix.length).toBeGreaterThan(0);
      expect(typeof c.retryable).toBe("boolean");
      expect(c.title + c.fix).not.toContain("—");
    }
    expect(codes).toContain("google_app_password_required");
    expect(() => mailCause("google_app_password")).toThrow();
  });

  it("answers refusals with catalogue entries", () => {
    const cases: [string, string, boolean, string][] = [
      ["gmail", "my-account-password", false, "google_app_password_required"],
      ["yahoo", "short", false, "yahoo_app_password_required"],
      ["aol", "short", true, "aol_app_password_required"],
      ["microsoft365", "wrong", true, "microsoft_smtp_auth_disabled"],
      ["outlook", "wrong", false, "microsoft_basic_auth_disabled"],
      ["other", "wrong", false, "auth_refused"],
    ];
    for (const [host, pw, smtpOnly, code] of cases) {
      const r = refusal(host, pw, smtpOnly);
      expect(r?.cause).toEqual(catalogue.find((c) => c.code === code));
      expect(r?.inbound.ok).toBe(smtpOnly);
    }
    expect(refusal("gmail", "abcdefghijklmnop", false)).toBeNull();
    expect(mailCause("microsoft_smtp_auth_disabled").fix).toContain("phasing out password sign-in for SMTP AUTH");
  });
});

describe("mock mailbox sending", () => {
  installMock();

  it("moves a forwarding mailbox to its own SMTP server and back", async () => {
    const created = await call("POST", "/v1/mailboxes", { name: "Sales", email: "sales@sending.example", inbound_kind: "forwarding", outbound_kind: "system" });
    expect(created.status).toBe(201);
    const url = `/v1/mailboxes/${(created.body as Mailbox).id}`;
    const smtp = (pw?: string) => ({
      outbound_kind: "smtp",
      verify: true,
      config: { smtp_host: "smtp.sending.example", smtp_port: 587, smtp_tls: "starttls", smtp_username: "sales@sending.example" },
      ...(pw ? { smtp_password: pw } : {}),
    });

    expect((await call("PATCH", url, smtp())).status).toBe(422);
    expect((await call("PATCH", url, { ...smtp("pw-123456"), config: { smtp_host: "10.0.0.5" } })).status).toBe(422);
    const refused = await call("PATCH", url, smtp("wrong"));
    expect(refused.status).toBe(422);
    expect(refused.body).toMatchObject({ code: "mailbox_connection_failed", cause: { code: "auth_refused" }, inbound: { ok: true }, outbound: { ok: false } });
    expect(((await call("GET", url)).body as Mailbox).outbound_kind).toBe("system");

    const ok = await call("PATCH", url, smtp("pw-123456"));
    expect(ok.status).toBe(200);
    expect((ok.body as Mailbox).outbound_kind).toBe("smtp");
    expect((ok.body as Mailbox).config.smtp_host).toBe("smtp.sending.example");
    // The stored password is kept when none is sent.
    const kept = await call("PATCH", url, { verify: true, config: { smtp_port: 465, smtp_tls: "tls" } });
    expect(kept.body).toMatchObject({ outbound_kind: "smtp" });
    const test = await call("POST", `${url}/test`);
    expect(test.body).toMatchObject({ outbound: { ok: true } });

    const back = await call("PATCH", url, { outbound_kind: "system" });
    expect((back.body as Mailbox).config.smtp_host).toBeUndefined();
    // No password is left to sign in with.
    expect((await call("PATCH", url, { ...smtp(), verify: false })).status).toBe(422);
  });
});
