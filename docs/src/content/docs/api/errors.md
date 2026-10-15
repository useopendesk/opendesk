---
title: Errors
description: opendesk returns RFC 9457 problem details with a stable machine-readable code.
---

Errors use [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) problem details with content type `application/problem+json`:

```json
{
  "status": 422,
  "title": "Unprocessable Entity",
  "detail": "subject is required",
  "code": "validation_failed",
  "request_id": "01J9...",
  "errors": [
    { "message": "expected length >= 1", "location": "body.subject", "value": "" }
  ]
}
```

| Field | Meaning |
| --- | --- |
| `status` | The HTTP status code. |
| `title` | The standard text for that status. |
| `detail` | A human-readable explanation. Show it to people, but do not parse it. |
| `code` | A stable machine string. Branch on this. |
| `request_id` | Quote it if you contact support. Every response also has an `X-Request-Id` header. |
| `errors` | For validation errors, one entry per problem, with where it was found. |

## Codes

The common codes:

| Code | Status | Meaning |
| --- | --- | --- |
| `bad_request` | 400 | The request is malformed. |
| `validation_failed` | 400, 422 | One or more fields are invalid. See `errors`. |
| `workspace_required` | 400 | A workspace-scoped route was called without `OD-Workspace`. |
| `captcha_failed` | 400 | The bot challenge on sign-in, registration or reset failed. |
| `unauthenticated` | 401 | Credentials are missing or invalid. |
| `invalid_credentials` | 401 | The email or password is wrong. The same answer is given for an unknown email. |
| `forbidden` | 403 | Your role or API key scopes do not allow this. |
| `mfa_required` | 403 | This action needs two-factor authentication on your account. |
| `reauth_required` | 403 | This action needs a recent sign-in. Sign in again and retry. |
| `plan_limit` | 402 | The workspace's plan limit is reached; `detail` names the limit. Cloud only. |
| `ai_balance_empty` | 402 | The workspace's AI balance does not cover the call's estimate; the model was not called. Add AI usage or upgrade. Cloud only. |
| `not_found` | 404 | The resource does not exist in this workspace. |
| `ai_disabled` | 404 | No AI provider is configured on this instance. |
| `ai_key_rejected` | 502 | The workspace's own AI provider key was refused by its provider. An admin can replace it in Settings > AI. |
| `ai_key_no_funds` | 502 | The provider account behind the workspace's own AI key is out of funds. |
| `provider_test_failed` | 422 | A provider key failed its test call and was not saved; `detail` says why. |
| `billing_disabled` | 404 | Billing is not enabled on this instance. |
| `top_up_unavailable` | 422 | AI top-ups are not available on this instance. |
| `payment_method_required` | 409 | The workspace has no saved payment method. Save one, then retry. |
| `card_declined` | 402 | The card was declined, so nothing was charged. |
| `authentication_required` | 402 | The bank wants the buyer to confirm the payment. Pay in Checkout instead. |
| `recharge_too_small` | 422 | An auto-recharge would not bring the balance back above its threshold. |
| `monthly_limit_too_small` | 422 | The auto-recharge monthly limit is below one recharge. |
| `billing_unavailable` | 502 | The payment provider could not be reached. Retry shortly. |
| `conflict` | 409 | The request conflicts with the current state, such as a duplicate. |
| `idempotency_in_progress` | 409 | A request with the same [idempotency key](/api/idempotency/) is still running. Retry shortly. |
| `idempotency_key_reused` | 422 | The idempotency key was used for a different request. Use a new key. |
| `too_large` | 413 | The upload or message is too large. |
| `rate_limited` | 429 | Too many requests. See [Rate limits](/api/rate-limits/). |
| `internal` | 500 | Something went wrong on our side. |

Some routes return more specific codes, such as `weak_password`, `email_taken` or `sole_owner`. They are listed per operation in the [OpenAPI reference](/api/openapi/). Treat a code you do not recognize by its HTTP status.

A request without valid credentials returns `401`. A resource in another workspace returns `404`, the same as one that does not exist.

Server errors always carry the same fixed message and never include internal details. The `request_id` lets the operator find the logged cause.

## Handling errors

- Retry `429` after the time in `RateLimit-Reset`, and `5xx` with exponential backoff.
- Do not retry other `4xx` responses unchanged.
- Use an [idempotency key](/api/idempotency/) on creates so a retry never creates a duplicate.
