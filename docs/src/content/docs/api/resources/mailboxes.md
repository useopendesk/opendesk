---
title: Mailboxes
description: Create, test and connect mailboxes, and post raw inbound email.
---

Mailboxes are settings: an `admin` user, or an API key with `settings:read` and `settings:write`.

| Method | Path | Description |
| --- | --- | --- |
| `GET`, `POST` | `/v1/mailboxes` | List or create mailboxes |
| `GET`, `PATCH`, `DELETE` | `/v1/mailboxes/{id}` | |
| `POST` | `/v1/mailboxes/{id}/test` | Test inbound and outbound |
| `POST` | `/v1/mailboxes/oauth/{provider}/start` | Start a Google or Microsoft connection |
| `GET` | `/v1/mailboxes/oauth/{provider}/callback` | OAuth redirect target |
| `POST` | `/v1/inbound/email` | Receive a raw email (forwarding) |

## The mailbox object

```ts
type Mailbox = {
  id: string;
  name: string;
  email: string;
  display_name: string;
  inbound_kind: "forwarding" | "imap" | "google" | "microsoft";
  outbound_kind: "smtp" | "google" | "microsoft" | "system";
  forwarding_address: string;
  config: {
    imap_host?: string;
    imap_port?: number;
    imap_username?: string;
    imap_tls?: "tls" | "starttls" | "none";
    smtp_host?: string;
    smtp_port?: number;
    smtp_username?: string;
    smtp_tls?: "tls" | "starttls" | "none";
    oauth_email?: string;
  };
  team_id: string | null;
  signature_html: string;
  auto_reply_enabled: boolean;
  auto_reply_subject: string;
  auto_reply_html: string;
  status: "active" | "error" | "disabled";
  last_error: string;
  last_synced_at: string | null;
  created_at: string;
  trusted_form_senders: string[];
};
```

Passwords and tokens are write-only: they are accepted on create and update, encrypted, and never returned.

## Contact forms

Website contact forms usually send `From` an address of the site with the visitor in `Reply-To`. List those addresses in `trusted_form_senders` (an exact address, or `@domain` for every address at that domain, at most 50) and a form message that passed DMARC is from the visitor: the visitor becomes the contact and replies go to them. Mail from a listed address that did not pass DMARC is handled like any other mail.

```sh
curl -X PATCH https://api.opendesk.sh/v1/mailboxes/0192f0c4-... \
  -H "Authorization: Bearer od_live_..." \
  -H "Content-Type: application/json" \
  -d '{ "trusted_form_senders": ["website@example.com"] }'
```

The list replaces the stored one. A domain of a public email provider (such as `@gmail.com`) is refused; list the form's exact address instead.

## Create an IMAP and SMTP mailbox

```sh
curl https://api.opendesk.sh/v1/mailboxes \
  -H "Authorization: Bearer od_live_..." \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Support",
    "email": "support@example.com",
    "display_name": "Example Support",
    "inbound_kind": "imap",
    "outbound_kind": "smtp",
    "config": {
      "imap_host": "imap.example.com", "imap_port": 993, "imap_username": "support@example.com", "imap_tls": "tls",
      "smtp_host": "smtp.example.com", "smtp_port": 587, "smtp_username": "support@example.com", "smtp_tls": "starttls"
    },
    "password": "imap-app-password",
    "smtp_password": "smtp-app-password"
  }'
```

For forwarding, set `"inbound_kind": "forwarding"` and read `forwarding_address` from the response.

## Test

```sh
curl -X POST https://api.opendesk.sh/v1/mailboxes/0192f0c4-.../test -H "Authorization: Bearer od_live_..."
```

```json
{ "inbound": { "ok": true, "error": "" }, "outbound": { "ok": false, "error": "535 authentication failed" } }
```

## Connect Google or Microsoft

`POST /v1/mailboxes/oauth/{provider}/start` with `{ name, email?, mailbox_id? }` returns `{ url }`. Send the user's browser to that URL. After consent, the provider redirects to the callback, which redirects to `APP_URL/settings/mailboxes?connected=<id>`. Pass `mailbox_id` to reconnect an existing mailbox. `provider` is `google` or `microsoft`.

## Inbound email

`POST /v1/inbound/email?to=<recipient>` accepts a raw RFC 822 message as the body and returns `202`. It requires the `OD-Inbound-Secret` header to match the server's `INBOUND_SECRET`. This is what the Cloudflare Email Worker calls; self-hosters can call it from their own mail pipeline. See [Email setup](/self-hosting/email/).

```sh
curl "https://desk.example.com/v1/inbound/email?to=acme-x7k2@in.example.com" \
  -H "OD-Inbound-Secret: $INBOUND_SECRET" \
  -H "Content-Type: message/rfc822" \
  --data-binary @message.eml
```

## Events

`mailbox.created`, `mailbox.updated`, `mailbox.deleted`.
