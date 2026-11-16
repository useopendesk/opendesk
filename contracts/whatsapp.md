# WhatsApp (`internal/whatsapp`)

A WhatsApp connection links a workspace to a WhatsApp number held at a business solution
provider (BSP). It ships Twilio. A customer's message lands on their ticket, on the `whatsapp`
channel, and the team answers from the ticket, with the delivery of each reply shown on it
(sent, delivered, read, failed).

This is not a support channel (`contracts/support-channels.md`: those mirror tickets into chat tools
agents work in) and not a phone system (`contracts/phone.md`: calls). It is a way customers
reach the team, like a mailbox, and it follows the phone engine's patterns (sealed
credentials, a verified webhook stored and queued in one transaction, health, admin-only connect
routes) in a package of its own. The cloud and self-host use the same code; there is no plan gate.

## Model and API

```ts
type WhatsAppProvider = { key: string; name: string; fields: { name: string; label: string; secret: boolean }[];
  docs: string };               // where an admin sets the webhook URL in the provider
type WhatsAppConnection = { id: string; provider: string; name: string;
  sender: string;               // the number customers write to, E.164
  account_name: string; status: "connected" | "error"; last_error: string;
  health_status: "unknown" | "healthy" | "action_required" | "error";
  health_code: "" | "receiving" | "webhook_missing" | "credentials_refused" | "provider_unreachable";
  health_detail: string; health_checked_at: string | null;
  brand_id: string | null;      // the brand the number's conversations belong to; null: the default brand
  reuse_days: number;           // 0..90, default 7 (Placement)
  last_event_at: string | null;
  webhook_url?: string;         // admins only
  status_url?: string;          // admins only
  created_at: string };
type WhatsAppTicketState = { connection: { id: string; name: string; sender: string } | null;
  window: { open: boolean; closes_at: string | null } };
type WhatsAppTemplate = { id: string; name: string; language: string; body: string; variables: string[] };
```

| method | path | role | body → response |
| --- | --- | --- | --- |
| GET | `/v1/whatsapp/providers` | viewer, `settings:read` | → `{data: WhatsAppProvider[]}` |
| GET | `/v1/whatsapp/connections` | viewer, `settings:read` | → `{data: WhatsAppConnection[]}`; never credentials; the URLs for admins only |
| POST | `/v1/whatsapp/connections` | admin user | `{provider, sender, name?, brand_id?, credentials}` → 201; `422 whatsapp_credentials_refused`; `422` for a number without its country code; `409` when the workspace already has that number |
| PATCH | `/v1/whatsapp/connections/{id}` | admin user | `{name?, brand_id?, clear_brand?, reuse_days?, credentials?}` → connection; new credentials are checked with the provider first |
| POST | `/v1/whatsapp/connections/{id}/test` | admin user | checks the credentials and stores the health → connection. The webhook counts as working once a message or a delivery status came in at it |
| DELETE | `/v1/whatsapp/connections/{id}` | admin user | the connection goes; conversations already on tickets stay → 204. Removing the webhook in the provider is the admin's |
| GET | `/v1/tickets/{id}/whatsapp` | viewer, `tickets:read` | → `WhatsAppTicketState`; `connection` is null when the ticket has no WhatsApp message to answer or its number was disconnected |
| GET | `/v1/tickets/{id}/whatsapp/templates` | agent, `tickets:write` | → `{data: WhatsAppTemplate[]}`: the approved templates of the ticket's number (kept five minutes) |
| POST | `/v1/whatsapp/hooks/{provider}/{connectionId}` | provider | a customer's message (Security) |
| POST | `/v1/whatsapp/hooks/{provider}/{connectionId}/status` | provider | the delivery status of a message sent (Security) |

"Admin user" routes need a person signed in as an admin: connecting a number hands it a way into
tickets, so API keys get `403`. A number is one connection per workspace.

Events, to admins only (`events.Event.Admins`): `whatsapp_connection.created` / `.updated`
`{whatsapp_connection}` (also on every new health state), `whatsapp_connection.deleted`
`{whatsapp_connection_id}`. Audit: `whatsapp.connected`, `whatsapp.updated`,
`whatsapp.credentials_rotated`, `whatsapp.disconnected`.

## Provider adapter boundary

The engine owns connections, sealed credentials, contact matching, placement, the window and every
ticket write. An adapter (`internal/whatsapp/provider`) owns its provider's protocol only, never
imports a helpdesk package and never reads a table:

```go
type Adapter interface {
    Describe() Descriptor
    Validate(ctx, *http.Client, credentials) (Account, error)
    Verify(Request, credentials) error               // the request came from the provider
    ParseMessage(url.Values) (Event, error)           // ErrIgnored for what is of no use
    ParseStatus(url.Values) (Event, error)
    Send(ctx, *http.Client, credentials, Outbound) (id string, error)
    Templates(ctx, *http.Client, credentials) ([]WhatsAppTemplate, error)
    Media(ctx, *http.Client, credentials, Media, limit int64) (io.ReadCloser, error)
}
```

`Send` answers `OutsideWindow` when the provider refuses a free-form message for the window,
`Rejected{Reason}` for a refusal for good (a number not on WhatsApp, a template not approved),
`RateLimited` when the provider asks to wait and any other error for a retry. Adding a provider
means implementing this interface and passing it to `whatsapp.New` in `For`.

## Messages

`whatsapp_inbound_events` holds verified events, normalized (the raw payload is never stored),
unique by `(connection, event id)` so the provider's retries are recognised, until a week after
they were applied (30 days at most). The `whatsapp.inbound` job applies one in a transaction that
locks its row and marks it processed. While the workspace has Tickets off the event stays stored
and the job snoozes (`products.Hold`).

`whatsapp_messages` links a ticket's message to the conversation: the connection, the customer's
number and the provider's id of it. A delivery status is matched by that id; the connection a reply
goes out through is the one of the customer's latest message on the ticket.

### Contacts and placement

The customer's number is the contact's phone (`domain.ContactByPhone`: matched on its digits, a new
number makes a contact named after the WhatsApp profile name, else the number). A blocked contact's
message reaches no ticket, as their email doesn't.

A message goes on the contact's open ticket **on the `whatsapp` channel** (open, pending or on hold,
most recently updated), else on their latest ticket on it solved within the connection's
`reuse_days` (0: never), else on a new ticket, routed like any new ticket (`routing.Resolve` with
channel `whatsapp`, so routing rules and automations can take the channel as a condition), named after
the first line of the message (80 characters), else "WhatsApp from …", and running the `ticket_created`
automations. A ticket on another channel is never reused: a reply there would not go through WhatsApp.
Customer messages are stored as the customer's own words, run `customer_replied` and reopen a ticket
as an email does. Images, documents, audio and video (16 MB at most each) are fetched from the
provider and kept as attachments; what can't be taken (refused type, too large, gone) is left out and
the message still lands. A message with neither words nor a file (a location, a contact card, a
reaction) is acknowledged and dropped.

### Window

WhatsApp allows free-form replies for 24 hours after the customer's last message (`Window`).
After that only an approved template may be sent. `PlanReply` decides, in the reply's transaction:

- window open and no template: the words go out as written;
- window closed and no template: `422 whatsapp_window_closed`;
- a template (`whatsapp_template: {id, variables}` on the reply, open window or not): it must be an
  approved template of the connected number and every variable must be filled (1,024 characters at
  most each), else `422`. The reply's body is the template's words with the variables put in, so the
  ticket shows what the customer received.

A reply on a ticket without a connected number is `422 whatsapp_no_conversation` or
`whatsapp_disconnected`. Replies are text only for now: attachments are refused with `422`. Replies
written by automations or the AI on a WhatsApp ticket are recorded, not sent, as on an `api` ticket.

### Delivery

A reply is stored `pending` and the `whatsapp.send` job sends it (`email` queue, 8 attempts) with a
status callback URL. It becomes `sent` with the provider's id, then `delivered` and `read` as the
provider reports them. A status only moves a message forward, in any order, and `failed` never
replaces `delivered` or `read`. A status that beats the id being stored waits (the job snoozes, up
to 15 minutes). A refusal for good, an expired window at the provider, a removed connection and a
last failed attempt mark the message `failed` with a reason (`delivery_error`) the agent reads under
it. A reply deleted before it went out is skipped. `messages.delivery_status` is
`none | pending | sent | delivered | read | failed | bounced`.

## Twilio

Docs: [WhatsApp API](https://www.twilio.com/docs/whatsapp/api),
[webhooks](https://www.twilio.com/docs/usage/webhooks/webhooks-security),
[Content API](https://www.twilio.com/docs/content).

- Credentials: an Account SID and an Auth Token (Basic authentication), checked with
  `GET /2010-04-01/Accounts/{sid}.json`. The WhatsApp number is the connection's `sender`.
- Webhook: Twilio has no API for a WhatsApp sender's webhook, so an admin sets the connection's
  `webhook_url` in the console (Messaging, Senders, WhatsApp senders, Endpoint configuration) with
  the method POST. Health is `action_required` / `webhook_missing` until a message comes in.
- Security: every request is verified before it is read. `X-Twilio-Signature` is the base64
  HMAC-SHA1 of the URL Twilio was given followed by the form's keys and values in key order, under
  the Auth Token; the URL is the connection's, never read from the request. Nothing in the form
  decides which workspace or ticket it reaches: the connection is found by the id in the path.
- Messages: a form with `MessageSid`, `From` (`whatsapp:+…`; anything else is an SMS and ignored),
  `Body`, `ProfileName`, `NumMedia` and `MediaUrlN` / `MediaContentTypeN`. A media link is followed
  only when it is Twilio's or its storage (https, `*.twilio.com`, `*.twiliocdn.com`,
  `*.amazonaws.com`), and the credentials go only to Twilio's own host.
- Sending: `POST /2010-04-01/Accounts/{sid}/Messages.json` with `From`, `To` (both `whatsapp:+…`),
  `Body` or `ContentSid` and `ContentVariables`, and `StatusCallback`. Error `63016` is the window.
- Delivery: `MessageStatus` `sent`, `delivered`, `read`, `failed` and `undelivered` (with
  `ErrorCode`) arrive at the status URL; `queued`, `sending` and `accepted` are ignored.
- Templates: `GET content.twilio.com/v1/ContentAndApprovals`; only those WhatsApp approved are
  offered, with the text of their `twilio/text`, `quick-reply`, `call-to-action`, `media` or `card`
  type and the `{{1}}`, `{{2}}` variables of it.
- Rate limit: a 429 snoozes the job for `Retry-After`.

## Security

- Credentials are sealed with `d.Box` (`whatsapp_connections.credentials_enc`, in `internal/sealed`)
  and never returned. A request that names no connection or fails verification is refused (`404`,
  `401`) and counts against a per-address limit before any work (30 a minute); verified requests are
  limited per connection (600 a minute).
- A workspace's rows are read and written with the workspace of the principal or of the connection;
  a ticket's window, templates and replies go through the same visibility as the ticket
  (`domain.ResolveVisibleTicket`).
- A customer's text is HTML-escaped before it is stored.

## Not yet

Starting a conversation (a template to a customer who has not written in the last 24 hours) from the
new ticket dialog or a contact; opt-in records; sending files, images and voice notes; other
providers (Bird, Meta's Cloud API); a WhatsApp number that is also the contact's `phone` when
the contact has several. Localized ticket subjects for a message without words.
