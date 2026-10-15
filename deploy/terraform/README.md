# opendesk Terraform

Everything the hosted service runs on: AWS (ECS Fargate, ALB, S3, SES, Secrets
Manager, KMS, ECR, Cloud Map), Cloudflare (DNS, Pages, Email Routing, zone
security, the help center zone) and PlanetScale (Postgres roles, reached over AWS PrivateLink). The architecture and the full first-time runbook
are in [`deploy/README.md`](../README.md); this file is the Terraform-specific part.

## Layout

```
environments/production/   the root: wiring, variables, outputs, backend
modules/
  network/       VPC, public subnets, IGW, S3 gateway endpoint (no NAT Gateway)
  alb/           one ALB for api + gateway, ACM cert validated through Cloudflare
  ecs-service/   a Fargate task definition (+ service, roles, logs, autoscaling)
  nats/          one NATS server or a cluster of three on ECS, or an external NATS
  s3/            private attachments bucket
  ses/           domain identity, DKIM, MAIL FROM, DMARC, configuration set
  secrets/       Secrets Manager entries, generated and operator-supplied
  planetscale/   app and migrate roles, the PrivateLink endpoint, connection strings
  cloudflare/    DNS, Pages projects and domains, Email Routing, zone settings
  cloudflare-help/  the help center zone (opendeskhelp.com): records for the help
                 worker, the custom hostname fallback origin, no-mail records, zone settings
  github-oidc/   the role deploy.yml assumes (no long-lived AWS keys)
```

Names other tooling depends on: cluster `opendesk-production`; services and task
families `opendesk-api`, `opendesk-worker`, `opendesk-gateway`,
`opendesk-nats-{1,2,3}` (only `-1` at the launch size); one-off family `opendesk-migrate`; ECR repositories
`opendesk/opendesk` and `opendesk/realtime`; secrets `opendesk/production/<KEY>`;
Pages projects `opendesk-site`, `opendesk-docs`, `opendesk-app`; Email Worker
`opendesk-inbound-email`; help worker `opendesk-help`.

## Credentials

Terraform reads them from the environment only:

```sh
export AWS_PROFILE=opendesk-admin        # or an SSO session
export CLOUDFLARE_API_TOKEN=...          # Zone: DNS, Zone Settings, Email Routing; Account: Pages, Email Routing
                                         # (help zone: below)
export PLANETSCALE_SERVICE_TOKEN_ID=... PLANETSCALE_SERVICE_TOKEN=...   # access to the database's roles
```

The same token reaches the help center zone (`opendeskhelp.com`) with Zone Read,
DNS Edit, Zone Settings Edit, SSL and Certificates Edit and Workers Routes Edit,
and the account with Workers Scripts Edit (`deploy/README.md`, step 2). The API's
own token for custom hostnames is a different one (`CLOUDFLARE_SAAS_TOKEN`,
below).

## Bootstrap order

1. **Database and state bucket** (once, by hand). Create the PlanetScale
   database in its dashboard: Postgres, AWS us-east-1, single node or high
   availability (`deploy/README.md`, "Sizing and cost"). Terraform never creates
   or deletes it; it manages the roles and the private connection. Then the
   state bucket:

   ```sh
   ACCOUNT=$(aws sts get-caller-identity --query Account --output text)
   aws s3api create-bucket --bucket opendesk-terraform-state-$ACCOUNT --region us-east-1
   aws s3api put-bucket-versioning --bucket opendesk-terraform-state-$ACCOUNT \
     --versioning-configuration Status=Enabled
   aws s3api put-public-access-block --bucket opendesk-terraform-state-$ACCOUNT \
     --public-access-block-configuration BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true
   ```

2. **Deploy the workers once** (`pnpm exec wrangler deploy` in
   `deploy/cloudflare/email-worker` and in `deploy/cloudflare/help-worker`).
   The catch-all rule points at the email worker by name. If you skip this, the
   first apply fails on the catch-all; deploy the worker and apply again.

   **Enable Cloudflare for SaaS on the help zone** (`opendeskhelp.com`: SSL/TLS >
   Custom Hostnames > Enable). The fallback origin
   (`cloudflare_custom_hostname_fallback_origin`) is refused until it is on.
   Set `help_zone_id` in `terraform.tfvars`, or let Terraform look the zone up
   from `help_domain`.

3. **Apply:**

   ```sh
   cd deploy/terraform/environments/production
   cp terraform.tfvars.example terraform.tfvars   # fill in
   terraform init -backend-config="bucket=opendesk-terraform-state-$ACCOUNT" -backend-config="region=us-east-1"
   terraform apply
   ```

   The services are created with the `bootstrap` image tag, which does not exist
   yet, so their tasks fail until step 5. That is expected.

   Then allow only private addresses on the database (its IP restrictions in
   PlanetScale: `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`), so the
   PrivateLink endpoint is the only way in. Check that a connection to the
   public host from outside AWS is refused.

4. **Operator secrets.** For each of `GOOGLE_CLIENT_SECRET`,
   `MICROSOFT_CLIENT_SECRET`, `GITHUB_CLIENT_SECRET`, `TURNSTILE_SECRET`,
   `POSTHOG_KEY`, `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`, `STRIPE_PRICE_PRO`,
   `STRIPE_PRICE_PRO_YEARLY`, `STRIPE_PRICE_CREDITS`, `ANTHROPIC_API_KEY`,
   `VOYAGE_API_KEY` and `APNS_KEY` you use (`STRIPE_PRICE_CREDITS` holds the
   one-time $10 AI top-up price; the app reads it when `STRIPE_PRICE_AI_TOPUP`
   is unset):

   ```sh
   aws secretsmanager put-secret-value --secret-id opendesk/production/GOOGLE_CLIENT_SECRET --secret-string '...'
   aws secretsmanager put-secret-value --secret-id opendesk/production/APNS_KEY --secret-string file://AuthKey_<key id>.p8   # PEM as is
   ```

   then add the key to `optional_secrets` in `terraform.tfvars` and apply again.
   They are opt-in because a task that references a secret with no value does
   not start.

   Help centers: `HELP_PROXY_SECRET` is generated; give the help worker the same
   value (`deploy/README.md`, step 6):

   ```sh
   aws secretsmanager get-secret-value --secret-id opendesk/production/HELP_PROXY_SECRET \
     --query SecretString --output text | (cd deploy/cloudflare/help-worker && pnpm exec wrangler secret put HELP_PROXY_SECRET)
   ```

   `CLOUDFLARE_SAAS_TOKEN` is a token with **SSL and Certificates Edit** on the
   help zone only. Store it like the others and add it to `optional_secrets`:
   the apply then also sets `CLOUDFLARE_SAAS_ZONE_ID`, and custom domains get
   their certificates from Cloudflare.

5. **GitHub configuration** from the outputs (`terraform output`):
   `AWS_DEPLOY_ROLE_ARN` = `github_deploy_role_arn`, `ECS_MIGRATE_NETWORK` =
   `migrate_network_configuration`. Then
   run the deploy workflow: it pushes images, runs `opendesk-migrate` and updates
   the services.

## Day two

- **Deploys never go through Terraform.** `deploy.yml` registers a new revision
  of each family from the latest one, with only the image changed. The services
  ignore `task_definition` and `desired_count`, so `terraform apply` after a
  deploy changes neither. An apply that changes a task definition (new env or
  secret) registers a new revision, which the next deploy builds on.
- **Images are immutable and tagged by commit SHA.** The repositories keep the
  last 30.
- **Secrets never rotate on their own.** A new `CREDENTIALS_KEY` makes every
  stored mailbox credential unreadable; never taint `random_id.credentials_key`.
- **Sizes** are variables (`variables.tf`, "Sizes"): the defaults are the launch
  size, one of each. The database's size and high availability are settings in
  PlanetScale, changed online.
- **Database roles.** The services connect as `opendesk-app` (read and write all
  data, no DDL); only the migration task connects as `opendesk-migrate`.
  Replacing a role drops the old one and its password, and tasks read secrets
  only at start: run a deploy right after such an apply.
- **NATS as a service:** `nats_mode = "external"` and `external_nats_url` remove
  the NATS services; put the provider's token in
  `opendesk/production/NATS_TOKEN`.
- **R2 instead of S3:** see the header of `modules/s3/main.tf`.
- **Help centers** are on with `help_domain` (default `opendeskhelp.com`): the
  api and worker get `HELP_DOMAIN`, `HELP_CNAME_TARGET` (`custom.<help_domain>`)
  and `HELP_PROXY_SECRET`. `help_domain = ""` leaves help centers at
  `API_URL/help/<slug>` and manages no help zone. Rotating `HELP_PROXY_SECRET`
  means the worker secret and a deploy together: the API serves help center
  hosts only to requests that carry its current secret.

## Checks

```sh
terraform fmt -check -recursive deploy/terraform
for d in deploy/terraform/environments/* deploy/terraform/modules/*; do
  (cd "$d" && terraform init -backend=false -input=false >/dev/null && terraform validate)
done
```

`modules/planetscale/tests` checks the connection strings it builds, with mocked
providers: `cd deploy/terraform/modules/planetscale && terraform init && terraform test`.
