# Secrets Manager entries for every secret the services read, named
# <name>/<env>/<KEY>, all encrypted with the one customer-managed key.
#
# Generated secrets are created once from the random provider and never
# rotate on their own: a new CREDENTIALS_KEY would make every stored mailbox
# credential unreadable, and a new AUTH_SECRET signs everyone out.
#
# Operator-supplied secrets (OAuth client secrets, Turnstile, PostHog, Stripe
# keys and price ids, the Anthropic and Voyage API keys, the APNs key) are
# created empty. Set them with
#   aws secretsmanager put-secret-value --secret-id <name>/<env>/<KEY> --secret-string '...'
# and then list the key in `optional_secrets` in the environment so the task
# definitions start referencing it. A task that references a secret with no
# value would fail to start, which is why they are opt-in.

locals {
  prefix = "${var.name}/${var.environment}"

  generated = {
    AUTH_SECRET     = random_password.auth_secret.result
    CREDENTIALS_KEY = random_id.credentials_key.hex
    INBOUND_SECRET  = random_password.inbound_secret.result
    NATS_TOKEN      = random_password.nats_token.result
    SECRET_KEY_BASE = random_password.secret_key_base.result
  }

  # Listed apart from the values so for_each never sees sensitive data.
  database_keys = ["DATABASE_URL", "DATABASE_URL_DIRECT", "DATABASE_URL_MIGRATE"]

  database = {
    DATABASE_URL         = var.database_url
    DATABASE_URL_DIRECT  = var.database_url_direct
    DATABASE_URL_MIGRATE = var.database_url_migrate
  }

  manual = toset([
    "GOOGLE_CLIENT_SECRET",
    "GOOGLE_MAILBOX_CLIENT_SECRET",
    "MICROSOFT_CLIENT_SECRET",
    "GITHUB_CLIENT_SECRET",
    "TURNSTILE_SECRET",
    "POSTHOG_KEY",
    "STRIPE_SECRET_KEY",
    "STRIPE_WEBHOOK_SECRET",
    "STRIPE_PRICE_PRO",
    "STRIPE_PRICE_PRO_YEARLY",
    "STRIPE_PRICE_CREDITS",
    "ANTHROPIC_API_KEY",
    "VOYAGE_API_KEY",
    "APNS_KEY",
  ])

  all_keys = setunion(
    toset(["AUTH_SECRET", "CREDENTIALS_KEY", "INBOUND_SECRET", "NATS_TOKEN", "SECRET_KEY_BASE"]),
    toset(local.database_keys),
    local.manual,
  )
}

resource "random_password" "auth_secret" {
  length  = 64
  special = false
}

# 32 bytes for AES-256-GCM, hex encoded (64 characters).
resource "random_id" "credentials_key" {
  byte_length = 32
}

resource "random_password" "inbound_secret" {
  length  = 48
  special = false
}

resource "random_password" "nats_token" {
  length  = 48
  special = false
}

resource "random_password" "secret_key_base" {
  length  = 96
  special = false
}

resource "aws_secretsmanager_secret" "this" {
  for_each = local.all_keys

  name                    = "${local.prefix}/${each.key}"
  kms_key_id              = var.kms_key_arn
  recovery_window_in_days = var.recovery_window_in_days
}

resource "aws_secretsmanager_secret_version" "generated" {
  for_each = toset(["AUTH_SECRET", "CREDENTIALS_KEY", "INBOUND_SECRET", "NATS_TOKEN", "SECRET_KEY_BASE"])

  secret_id     = aws_secretsmanager_secret.this[each.key].id
  secret_string = local.generated[each.key]
}

resource "aws_secretsmanager_secret_version" "database" {
  for_each = toset(local.database_keys)

  secret_id     = aws_secretsmanager_secret.this[each.key].id
  secret_string = local.database[each.key]
}
