---
title: Widget
description: The endpoints the chat widget uses, and the admin settings behind it.
---

The widget endpoints are called by the visitor's browser. Apart from config and session, they use the visitor token: `Authorization: Visitor od_vis_...`. The workspace comes from the session.

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/v1/widget/config?key=<widget_key>` | Public config for the widget |
| `POST` | `/v1/widget/session` | Start or resume a visitor session |
| `GET` | `/v1/widget/conversations` | The visitor's chat conversations |
| `POST` | `/v1/widget/conversations` | Start a conversation |
| `GET` | `/v1/widget/conversations/{id}/messages` | Public messages only, never notes |
| `POST` | `/v1/widget/conversations/{id}/messages` | Send a message |
| `POST` | `/v1/widget/attachments` | Upload a file (multipart) |
| `POST` | `/v1/widget/conversations/{id}/handoff` | Ask for a person after an AI answer |
| `GET`, `PATCH` | `/v1/widget/settings` | Admin: widget settings |

## Config

```sh
curl "https://api.opendesk.sh/v1/widget/config?key=<widget_key>"
```

```json
{ "workspace_name": "Example", "brand_color": "#0A0A0A", "greeting": "Hi, how can we help?", "help_center_url": "https://...", "require_email": false }
```

## Session

```sh
curl https://api.opendesk.sh/v1/widget/session \
  -H "Content-Type: application/json" \
  -d '{ "key": "<widget_key>", "user_token": "<jwt signed with the widget secret>", "email": "ana@example.com", "name": "Ana" }'
```

```json
{ "visitor_token": "od_vis_...", "contact": { "id": "...", "email": "ana@example.com", "name": "Ana", "avatar_url": "" }, "ws_token": "<jwt>", "gateway_url": "wss://gateway.opendesk.sh/socket/widget" }
```

| Field | Notes |
| --- | --- |
| `key` | the workspace's public widget key, required |
| `visitor_token` | resume an existing session |
| `email`, `name` | what the visitor told you |
| `user_token` | a [verified identity](/guides/live-chat/identity-verification/) that expires: an HS256 JWT signed with the widget secret, with `user_id` and an `exp` at most the widget's `user_token_max_hours` ahead (24 by default). An expired token answers `401 user_token_expired`, except when it resumes the session it verified, with that session's `visitor_token` |
| `user_id`, `user_hash` | a [verified identity](/guides/live-chat/identity-verification/#use-a-user-hash): `user_hash` is hex HMAC-SHA256 of `user_id` with the widget secret. Refused with `401 user_token_required` while the widget has `require_user_token` on. With `user_token`, `user_id` is optional and must be the token's, and `user_hash` is not read |
| `company` | `{ id }`, with a verified identity only: your own id for the user's account. The contact is linked to the company whose `external_id` it is. A company is never created or changed from here, and an unknown id is ignored |
| `fields` | with a verified identity only: contact custom fields by key. Only keys that start with `widget_` are set; others are ignored |

`ws_token` is a 5-minute `widget_ws` token for the [realtime gateway](/api/realtime/#the-chat-widget)'s widget endpoint. It lets the visitor join only their own contact topic.

## Conversations

```sh
curl https://api.opendesk.sh/v1/widget/conversations \
  -H "Authorization: Visitor od_vis_..." \
  -H "Content-Type: application/json" \
  -d '{ "body": "Hi, my order has not arrived." }'
```

## Settings (admin)

`GET` and `PATCH /v1/widget/settings` with `{ greeting, require_email, enabled, allowed_origins, require_user_token, user_token_max_hours }`, from the agent side. `require_user_token` (default `false`) verifies signed-in visitors with `user_token` alone and refuses `user_hash`; `user_token_max_hours` (1 to 720, default 24) is how far ahead a token's `exp` may be. Rotate the widget secret with `POST /v1/workspace/widget-secret/rotate`.
