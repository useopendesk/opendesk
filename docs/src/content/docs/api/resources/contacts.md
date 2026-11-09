---
title: Contacts and companies
description: Manage the people who write in and the companies they belong to.
---

Scopes: `contacts:read` to read, `contacts:write` to change.

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/v1/contacts` | List contacts. Filters: `q`, `company_id`, `tag`, `blocked`, `field.<key>`. `sort`: `-created` (default), `created`, `name`, `-name`, `-tickets`, `tickets` |
| `POST` | `/v1/contacts` | Create a contact |
| `GET` | `/v1/contacts/{id}` | Get a contact |
| `PATCH` | `/v1/contacts/{id}` | Update a contact |
| `DELETE` | `/v1/contacts/{id}` | Delete a contact |
| `GET` | `/v1/contacts/{id}/tickets` | The contact's tickets |
| `POST` | `/v1/contacts/bulk` | Label, set the company of, or delete many contacts: `ids` (up to 100) or `query` (the list's filters, at most 1000 contacts), with `patch` or `delete: true` |
| `GET`, `POST` | `/v1/companies` | List or create companies. Filters: `q`, `tag`, `field.<key>`. `sort`: `name` (default), `-name`, `created`, `-created`, `contacts`, `-contacts` |
| `GET`, `PATCH`, `DELETE` | `/v1/companies/{id}` | Get, update or delete a company |
| `POST` | `/v1/companies/bulk` | Label or delete many companies, by `ids` or `query` (`q`, `tag`) |

## Objects

```ts
type Contact = {
  id: string;
  email: string | null;
  name: string;
  phone: string;
  avatar_url: string;
  company: { id: string; name: string; domain: string } | null;
  external_id: string | null;
  notes: string;
  custom: Record<string, unknown>;
  tags: { id: string; name: string; color: LabelColor; group_id: string | null }[];
  fields: Record<string, FieldValue>;
  blocked: boolean;
  ticket_count: number;
  created_at: string;
  updated_at: string;
};

type Company = {
  id: string;
  name: string;
  domain: string;
  notes: string;
  custom: Record<string, unknown>;
  tags: { id: string; name: string; color: LabelColor; group_id: string | null }[];
  fields: Record<string, FieldValue>;
  contact_count: number;
  created_at: string;
  updated_at: string;
};
```

A contact's email is unique within the workspace, and so is its `external_id`, your own id for the person (set by [verified widget sessions](/guides/live-chat/identity-verification/)). A company's domain is unique within the workspace.

Writes to contacts and companies take `tags` (replaces), `add_tags`, `remove_tags` and `fields` (merged, `null` clears), with the same rules as tickets. See [Labels and custom fields](/api/resources/labels-fields/).

## Examples

```sh
# Search
curl "https://api.opendesk.sh/v1/contacts?q=ana" -H "Authorization: Bearer od_live_..."

# Create
curl https://api.opendesk.sh/v1/contacts \
  -H "Authorization: Bearer od_live_..." \
  -H "Content-Type: application/json" \
  -d '{ "email": "ana@example.com", "name": "Ana Silva", "custom": { "plan": "pro" } }'

# Block
curl -X PATCH https://api.opendesk.sh/v1/contacts/0192f0c4-... \
  -H "Authorization: Bearer od_live_..." \
  -H "Content-Type: application/json" \
  -d '{ "blocked": true }'
```

## Events

`contact.created`, `contact.updated`, `contact.deleted`, and the same for `company.*`.
