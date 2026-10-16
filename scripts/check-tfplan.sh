#!/usr/bin/env bash
# Tests scripts/tfplan.sh on hand-written plans: the action of each change, the
# risk class, and that no value from the plan reaches its output.
set -euo pipefail

cd "$(dirname "$0")/.."
tfplan=scripts/tfplan.sh
fail=0

check() { # name want got
  if [ "$2" = "$3" ]; then
    echo "ok   $1"
  else
    echo "FAIL $1: want '$2', got '$3'"
    fail=1
  fi
}

# Every kind of change. Values that must never be printed contain "s3cr3t".
plan='{
  "format_version": "1.2",
  "resource_changes": [
    {"address": "module.api.aws_ecs_task_definition.this", "mode": "managed", "type": "aws_ecs_task_definition",
     "change": {"actions": ["delete", "create"], "before": {"container_definitions": "s3cr3t-env"}, "after": {"container_definitions": "s3cr3t-env-2"}}},
    {"address": "aws_kms_alias.this", "mode": "managed", "type": "aws_kms_alias",
     "change": {"actions": ["create"], "after": {"name": "alias/s3cr3t"}}},
    {"address": "module.secrets.aws_secretsmanager_secret.this[\"STRIPE_SECRET_KEY\"]", "mode": "managed", "type": "aws_secretsmanager_secret",
     "change": {"actions": ["update"], "before": {"description": "s3cr3t"}, "after": {"description": "s3cr3t-2"}}},
    {"address": "module.nats.aws_ecs_service.this[2]", "mode": "managed", "type": "aws_ecs_service",
     "change": {"actions": ["delete"], "before": {"name": "s3cr3t"}}},
    {"address": "module.alb.aws_lb.this", "mode": "managed", "type": "aws_lb",
     "change": {"actions": ["create", "delete"], "before": {"name": "s3cr3t"}, "after": {"name": "s3cr3t-2"}}},
    {"address": "aws_s3_bucket.old", "mode": "managed", "type": "aws_s3_bucket",
     "change": {"actions": ["forget"], "before": {"bucket": "s3cr3t"}}},
    {"address": "aws_iam_role.imported", "mode": "managed", "type": "aws_iam_role",
     "change": {"actions": ["no-op"], "importing": {"id": "s3cr3t"}}},
    {"address": "module.network.aws_vpc.main", "previous_address": "module.network.aws_vpc.this", "mode": "managed", "type": "aws_vpc",
     "change": {"actions": ["no-op"]}},
    {"address": "aws_ecr_repository.this[\"opendesk\"]", "mode": "managed", "type": "aws_ecr_repository",
     "change": {"actions": ["no-op"], "before": {"name": "s3cr3t"}, "after": {"name": "s3cr3t"}}},
    {"address": "data.aws_caller_identity.current", "mode": "data", "type": "aws_caller_identity",
     "change": {"actions": ["read"], "after": {"account_id": "s3cr3t"}}}
  ],
  "output_changes": {
    "secret_arns": {"actions": ["update"], "before": "s3cr3t", "after": "s3cr3t-2"},
    "nats_url": {"actions": ["no-op"], "before": "s3cr3t", "after": "s3cr3t"}
  },
  "variables": {"planetscale": {"value": "s3cr3t"}},
  "prior_state": {"values": {"root_module": {"resources": [{"values": {"secret_string": "s3cr3t"}}]}}}
}'

reduced=$("$tfplan" reduce <<<"$plan")
summary=$("$tfplan" markdown <<<"$reduced")

check "actions" \
  "delete module.nats.aws_ecs_service.this[2]
replace module.alb.aws_lb.this
replace module.api.aws_ecs_task_definition.this
forget aws_s3_bucket.old
create aws_kms_alias.this
update module.secrets.aws_secretsmanager_secret.this[\"STRIPE_SECRET_KEY\"]
import aws_iam_role.imported
move module.network.aws_vpc.main" \
  "$(grep '^- ' <<<"$summary" | sed 's/^- //; s/`//g')"
check "counts" "1 to create, 1 to update, 2 to replace, 1 to delete, 1 to forget, 1 to import, 1 to move, 1 output to change." \
  "$(sed -n 3p <<<"$summary")"
check "outputs" 1 "$(jq .outputs <<<"$reduced")"
check "risky plan" risky "$("$tfplan" class <<<"$reduced")"
check "risky plan needs approval" 1 "$(grep -c 'Needs approval' <<<"$summary")"
check "no values in the reduced plan" 0 "$(grep -c s3cr3t <<<"$reduced" || true)"
check "no values in the summary" 0 "$(grep -c s3cr3t <<<"$summary" || true)"

# Only task definitions replaced, plus creates and updates: applied on its own.
safe=$(jq '.resource_changes |= map(select(.type != "aws_lb" and .type != "aws_s3_bucket" and .type != "aws_ecs_service"))' <<<"$plan")
safe_reduced=$("$tfplan" reduce <<<"$safe")
check "safe plan" safe "$("$tfplan" class <<<"$safe_reduced")"
check "safe plan needs no approval" 0 "$("$tfplan" markdown <<<"$safe_reduced" | grep -c 'Needs approval' || true)"

# A deleted task definition is routine too.
td_delete=$(jq '.resource_changes = [.resource_changes[0] | .change.actions = ["delete"]] | .output_changes = {}' <<<"$plan")
check "task definition delete" safe "$("$tfplan" reduce <<<"$td_delete" | "$tfplan" class)"

# Outputs alone still need an apply.
outputs_only=$(jq '.resource_changes = []' <<<"$plan")
check "outputs only" safe "$("$tfplan" reduce <<<"$outputs_only" | "$tfplan" class)"

empty=$(jq '.resource_changes |= map(.change.actions = ["no-op"] | del(.change.importing, .previous_address)) | .output_changes = {}' <<<"$plan")
empty_reduced=$("$tfplan" reduce <<<"$empty")
check "no changes" none "$("$tfplan" class <<<"$empty_reduced")"
check "no changes summary" "No changes." "$("$tfplan" markdown <<<"$empty_reduced" | sed -n 3p)"

# The digest covers the set of changes, not their order or their values.
reordered=$(jq '.resource_changes |= reverse | .resource_changes[1].change.after.name = "other"' <<<"$plan")
check "digest ignores order and values" "$("$tfplan" digest <<<"$reduced")" "$("$tfplan" reduce <<<"$reordered" | "$tfplan" digest)"
check "digest sees a new change" different \
  "$([ "$("$tfplan" digest <<<"$reduced")" != "$("$tfplan" digest <<<"$safe_reduced")" ] && echo different || echo same)"

exit "$fail"
