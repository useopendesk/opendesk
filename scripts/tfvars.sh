#!/usr/bin/env bash
# Production settings (deploy/terraform/environments/production/terraform.tfvars)
# are never in git. They live next to the state, in the private, versioned state
# bucket, at opendesk/production.tfvars; deploy.yml and ci.yml plan with that copy.
#
#   scripts/tfvars.sh pull [-f]   download it (refuses to overwrite a local file
#                                 that differs, unless -f)
#   scripts/tfvars.sh diff        local file against the bucket's copy (exit 1
#                                 when they differ)
#   scripts/tfvars.sh push        upload the local file, after showing the diff
#                                 and asking
#
# After a push, run Deploy with target "backend" to apply it and roll the services
# onto the task definitions it registers ("infrastructure" alone leaves the
# services on their current revisions). The bucket is
# TF_STATE_BUCKET, or opendesk-terraform-state-<account id> of the current AWS
# credentials (AWS_PROFILE or an SSO session).
set -euo pipefail

usage() {
  echo "usage: $0 pull [-f] | diff | push" >&2
  exit 2
}

[ $# -ge 1 ] || usage
cmd=$1
shift
force=false
case "$cmd" in
  pull)
    if [ "${1:-}" = -f ]; then force=true; shift; fi
    ;;
  diff | push) ;;
  *) usage ;;
esac
[ $# -eq 0 ] || usage

export AWS_PAGER=""
umask 077

root=$(cd "$(dirname "$0")/.." && pwd)
file="$root/deploy/terraform/environments/production/terraform.tfvars"
key="opendesk/production.tfvars"

bucket=${TF_STATE_BUCKET:-}
if [ -z "$bucket" ]; then
  account=$(aws sts get-caller-identity --query Account --output text)
  bucket="opendesk-terraform-state-$account"
fi
uri="s3://$bucket/$key"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
remote="$tmp/remote.tfvars"

# Downloads the bucket's copy to $remote; false when there is none yet.
fetch() {
  local err
  if ! err=$(aws s3api head-object --bucket "$bucket" --key "$key" 2>&1 >/dev/null); then
    if grep -qE '404|Not Found' <<<"$err"; then return 1; fi
    echo "$err" >&2
    exit 1
  fi
  aws s3 cp "$uri" "$remote" --only-show-errors
}

show_diff() { # from to
  diff -u --label "$uri" --label "${file#"$root"/}" "$1" "$2"
}

case "$cmd" in
  pull)
    fetch || { echo "no settings at $uri yet: write terraform.tfvars and run '$0 push' once" >&2; exit 1; }
    if [ -f "$file" ] && ! cmp -s "$remote" "$file"; then
      if [ "$force" != true ]; then
        echo "terraform.tfvars differs from $uri:" >&2
        show_diff "$remote" "$file" >&2 || true
        echo "push it, or pull -f to replace it" >&2
        exit 1
      fi
    fi
    cp "$remote" "$file"
    chmod 600 "$file"
    echo "pulled $uri"
    ;;

  diff)
    [ -f "$file" ] || { echo "no local terraform.tfvars: run '$0 pull'" >&2; exit 1; }
    fetch || { echo "no settings at $uri yet"; exit 1; }
    status=0
    show_diff "$remote" "$file" || status=$?
    [ "$status" -ne 0 ] || echo "no differences"
    exit "$status"
    ;;

  push)
    [ -f "$file" ] || { echo "no local terraform.tfvars to push" >&2; exit 1; }
    if command -v terraform >/dev/null 2>&1; then
      terraform fmt - <"$file" >/dev/null || { echo "terraform.tfvars is not valid HCL" >&2; exit 1; }
    fi
    if fetch; then
      if cmp -s "$remote" "$file"; then
        echo "no differences: nothing to push"
        exit 0
      fi
      show_diff "$remote" "$file" || true
    else
      echo "no settings at $uri yet: this creates them"
    fi
    read -r -p "push to $uri? [y/N] " answer
    case "$answer" in
      y | Y | yes) ;;
      *) echo "not pushed"; exit 1 ;;
    esac
    aws s3 cp "$file" "$uri" --sse AES256 --content-type text/plain --only-show-errors
    echo "pushed $uri; run Deploy with target backend to apply it and roll the services"
    ;;
esac
