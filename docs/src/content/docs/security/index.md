---
title: Security controls
description: The security invariants opendesk is built to, aligned with OWASP ASVS Level 1 and CASA Tier 1.
---

A self-hosted opendesk can sync Gmail mailboxes with a restricted Google scope, so opendesk is built to the App Defense Alliance **CASA Tier 1** controls (OWASP ASVS 4.0.3 Level 1). The controls below are invariants: every change to opendesk must keep them. Because the hosted service and the self-hosted install are the same code, they hold in both.

## Authentication

- Passwords are hashed with **Argon2id** (64 MiB memory, 3 iterations, 2 lanes, 16-byte salt).
- Every new or changed password, at registration, reset, change and invitation, must be 12 to 128 characters and is refused if it appears in a list of breached passwords.
- Login, registration and password reset are protected by a bot challenge (Cloudflare Turnstile) and rate limited per IP and per account.
- Login answers identically, in the same time, for an unknown account and a wrong password.
- Two-factor authentication uses TOTP. A code cannot be replayed. Recovery codes are stored hashed and work once.
- Federated sign-in (Google, Microsoft, GitHub, OIDC) binds identities by issuer and subject. An existing password account is linked to a new identity only after its password is presented.
- Owner-level operations (deleting a workspace, transferring ownership, rotating the widget secret, disabling two-factor) require two-factor authentication and a recent sign-in.

## Sessions and tokens

- Every token has a single **purpose** (access, realtime, widget, two-factor, OAuth state), and each consumer accepts exactly one. Tokens are signed with HS256, always expire, and the signing secret must be at least 32 bytes or the service does not start.
- Access tokens live 15 minutes. Refresh tokens rotate on every use and are stored hashed.
- Logout, password change, member removal and session revocation revoke the sessions **and** close their live connections.
- Cookies are HttpOnly, Secure, SameSite=Lax and path-scoped.
- API keys, visitor tokens and one-time email links are random, at least 32 bytes, and stored only as hashes.

## Access control

- Every workspace-owned record is read and written scoped to the workspace of the authenticated principal, never one taken from the request body.
- Any referenced record (contact, team, tag, mailbox, attachment) is checked to belong to the same workspace before it is used.
- Roles are enforced on every route: viewers read only, settings need an admin, owner operations need the owner. API keys are limited to their scopes.

## Input and output

- Email HTML and agent-written HTML are sanitized before they are stored, and stored HTML is only ever rendered sanitized.
- Names from untrusted sources (email headers, identity provider claims) are cleaned before display.
- Webhook and other outbound URLs must be HTTPS and are refused when they point at private, loopback or link-local addresses (SSRF protection).
- Every HTTP response carries security headers. The realtime gateway checks the browser `Origin`.

## Data protection

- Mailbox passwords, OAuth tokens, webhook secrets and TOTP secrets are encrypted at rest with **AES-256-GCM**.
- All traffic is encrypted in transit with TLS.
- Logs never contain secrets, tokens, passwords, credentials or full connection strings. Request logs never include query strings or bodies.
- A Google or Microsoft mailbox connection asks only for what its mode needs: sending, unless the instance syncs that mailbox over IMAP.
- Gmail data obtained through Google APIs is used only to provide the helpdesk features the user enabled, in line with the Google API Services User Data Policy, including its Limited Use requirements.

## Accountability

Security events are written to the audit log: logins, failed logins, two-factor changes, password changes, member and role changes, API key and webhook changes, and mailbox connections and disconnections.

## Deletion

Users can delete their account, and owners can delete their workspace. Data is removed, not hidden.

## Supply chain

Continuous integration runs `govulncheck` for Go, `pnpm audit` for JavaScript, `mix deps.audit` for Elixir, and a Trivy scan of every container image. Release files are published with SHA-256 checksums, and the installer verifies them.

## Reporting a vulnerability

See [Responsible disclosure](/security/disclosure/).
