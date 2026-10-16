#!/usr/bin/env bash
# Stores the value of an operator-supplied secret in Secrets Manager as
# opendesk/production/NAME. The value is read without echo (or from stdin, for a
# file such as the APNs .p8 key) and reaches the AWS CLI only on its stdin: never
# in argv, the shell history, a file or a log. Secret values never pass through
# GitHub.
#
#   scripts/put-secret.sh STRIPE_SECRET_KEY            # prompts
#   scripts/put-secret.sh APNS_KEY < AuthKey_<id>.p8    # a file, as is
#
# NAME is one of the operator-supplied secrets of deploy/terraform/modules/secrets
# (the "manual" list); Terraform generates the others. AWS credentials are the
# usual AWS_PROFILE or SSO session.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
secrets_tf="$root/deploy/terraform/modules/secrets/main.tf"

names=$(awk '/^[[:space:]]*manual[[:space:]]*=[[:space:]]*toset\(\[/ {on=1; next} on && /\]\)/ {exit} on' "$secrets_tf" |
  sed 's/#.*//' | grep -oE '"[A-Z0-9_]+"' | tr -d '"' | sort)
[ -n "$names" ] || { echo "found no secret names in $secrets_tf" >&2; exit 1; }

if [ $# -ne 1 ] || ! grep -qxF -- "$1" <<<"$names"; then
  echo "usage: $0 NAME, one of:" >&2
  awk '{print "  " $0}' <<<"$names" >&2
  exit 2
fi
name=$1
id="opendesk/production/$name"

export AWS_PAGER=""

value=""
trap 'unset value' EXIT
if [ -t 0 ]; then
  read -rs -p "$name (input hidden): " value
  echo >&2
  [ -n "$value" ] || { echo "empty value, nothing stored" >&2; exit 1; }
  printf '%s' "$value" | aws secretsmanager put-secret-value --secret-id "$id" --secret-string file:///dev/stdin >/dev/null
  unset value
else
  aws secretsmanager put-secret-value --secret-id "$id" --secret-string file:///dev/stdin >/dev/null
fi

echo "stored $name"
cat >&2 <<EOF
If $name is new, list it in optional_secrets (scripts/tfvars.sh pull, edit,
scripts/tfvars.sh push) and run Deploy with target infrastructure; the tasks
read secrets when they start, so an existing one takes effect on the next deploy.
EOF
