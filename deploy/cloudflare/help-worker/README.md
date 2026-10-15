# Help center worker

Help centers live at `<slug>.opendeskhelp.com`, and at a customer's own domain such as
`help.acme.com`, which the customer points at `custom.opendeskhelp.com` with a CNAME and
which the API adds to the zone as a Cloudflare for SaaS custom hostname
(contracts/teams.md "Help center addresses"). Every request that enters the
`opendeskhelp.com` zone runs this worker, which forwards it to the API:

```
<method> https://api.opendesk.sh<path>?<query>
OD-Help-Host: <the hostname the visitor asked for>
OD-Help-Client-IP: <CF-Connecting-IP>
OD-Help-Secret: <HELP_PROXY_SECRET>
X-Forwarded-Proto: https
```

The API renders the site of the help center that host belongs to
(`internal/helpdesk/helpsite`) and believes `OD-Help-Host` and `OD-Help-Client-IP` only
with the secret.

DNS, zone settings and the custom hostname fallback origin are managed by Terraform
(`deploy/terraform/modules/cloudflare-help`). This directory is the worker code and its
routes.

## Behaviour

- `opendeskhelp.com` and `www.opendeskhelp.com` answer `301` to the product site
  (`SITE_URL`).
- `custom.opendeskhelp.com` (the CNAME target) and `origin.opendeskhelp.com` (the
  fallback origin) are DNS targets and answer `404` when asked for by name.
- Every other host is forwarded with the same method, path, query and body, and
  `redirect: "manual"`, so the API's redirects (to a help center's custom domain) reach
  the visitor as they are.
- Only `Accept`, `Accept-Language`, `Content-Type`, `If-None-Match`, `User-Agent` and
  `Referer` are forwarded. Cookies, `Authorization` and every other header stay at the
  edge: help centers are public and anonymous.
- `GET` and `HEAD` are answered from the Workers cache of the data center first, keyed
  by the URL the visitor asked for (host included). The API's answer is stored only when
  it is `200`, `301` or `404` with `Cache-Control: public` and no `Set-Cookie`, for as
  long as that `Cache-Control` says (pages: 60 seconds; versioned assets: a year).
  `POST /_help/views` (the view counter) and other methods always go to the API.
- The fetch to the API skips the cache of the API's own zone, where one URL stands for
  every help center host.
- An API that cannot be reached, or a worker without `HELP_PROXY_SECRET`, answers `502`
  with a short plain message.
- Logs carry the failure only. Never the secret, a header or a body.

## Routes

`wrangler.toml` routes `opendeskhelp.com/*`, `*.opendeskhelp.com/*` and `*/*` on the
zone. `*/*` is the Cloudflare for SaaS catch-all: it matches every request that enters
the zone, custom hostnames included, whether or not the customer's own record is
proxied by Cloudflare.

## Deploy

```sh
pnpm install --frozen-lockfile
# Same value as the API's HELP_PROXY_SECRET (Secrets Manager: opendesk/production/HELP_PROXY_SECRET)
aws secretsmanager get-secret-value --secret-id opendesk/production/HELP_PROXY_SECRET \
  --query SecretString --output text | pnpm exec wrangler secret put HELP_PROXY_SECRET
pnpm run deploy
```

`deploy.yml` runs `wrangler deploy` on every push to main; the secret is set once and
only changes when `HELP_PROXY_SECRET` is rotated (rotate both together, then roll the
API). The deploy token needs Workers Scripts edit on the account and Workers Routes
edit (and Zone read) on `opendeskhelp.com`.

## Test

```sh
pnpm test        # unit tests, no network
pnpm typecheck
```
