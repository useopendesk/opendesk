#!/usr/bin/env bash
# What a Terraform plan does, without its values. The workflows read a plan only
# through this script, so logs and job summaries carry resource
# addresses and actions, never an attribute, a variable or an output value.
#
#   terraform show -json tfplan | scripts/tfplan.sh reduce > actions.json
#   scripts/tfplan.sh class    < actions.json   # none, safe or risky
#   scripts/tfplan.sh digest   < actions.json   # sha256 of the changes
#   scripts/tfplan.sh markdown < actions.json   # the summary
#
# reduce keeps, per managed resource that changes, its address, its type and one
# action: create, update, replace, delete, forget, import or move; and the number
# of outputs that change. Everything else in the plan is dropped there.
#
# A plan is risky when it deletes, replaces or forgets anything other than an
# aws_ecs_task_definition (a new revision on every environment change is
# routine). deploy.yml applies a safe plan on its own and a risky one only after
# a reviewer approves it in the infrastructure-approval environment.
#
# scripts/check-tfplan.sh tests this on a sample plan.
set -euo pipefail

usage() {
  echo "usage: $0 reduce|class|digest|markdown  (JSON on stdin)" >&2
  exit 2
}

[ $# -eq 1 ] || usage

case "$1" in
  reduce)
    jq -c '
      def action:
        (.change.actions // []) as $a
        | def did($x): $a | any(.[]; . == $x);
          if did("delete") and did("create") then "replace"
          elif did("delete") then "delete"
          elif did("forget") then "forget"
          elif did("create") then "create"
          elif did("update") then "update"
          elif .change.importing != null then "import"
          elif (.previous_address // .address) != .address then "move"
          else empty end;
      {
        changes: [
          .resource_changes[]?
          | select(.mode == "managed")
          | {address, type, action: action}
        ] | sort_by(.address),
        outputs: [(.output_changes // {}) | to_entries[] | select(.value.actions != ["no-op"])] | length
      }'
    ;;
  class)
    jq -r '
      if ([.changes[] | select((.action == "delete" or .action == "replace" or .action == "forget") and .type != "aws_ecs_task_definition")] | length) > 0 then "risky"
      elif (.changes | length) > 0 or .outputs > 0 then "safe"
      else "none" end'
    ;;
  digest)
    if command -v sha256sum >/dev/null 2>&1; then sum() { sha256sum; }; else sum() { shasum -a 256; }; fi
    jq -S -c '{changes: [.changes[] | {address, action}] | sort_by(.address), outputs}' | sum | cut -d' ' -f1
    ;;
  markdown)
    jq -r '
      def order: {"delete": 0, "replace": 1, "forget": 2, "create": 3, "update": 4, "import": 5, "move": 6}[.action];
      def risky: (.action == "delete" or .action == "replace" or .action == "forget") and .type != "aws_ecs_task_definition";
      def count($a): [.changes[] | select(.action == $a)] | length;
      . as $p
      | 300 as $max
      | ([.changes[] | select(risky)] | length) as $risky
      | [
          "### Terraform plan",
          "",
          (if (.changes | length) == 0 and .outputs == 0 then "No changes."
           else
             ([["create", "to create"], ["update", "to update"], ["replace", "to replace"], ["delete", "to delete"],
               ["forget", "to forget"], ["import", "to import"], ["move", "to move"]]
              | map(.[0] as $k | ($p | count($k)) as $n | select($n > 0) | "\($n) \(.[1])")
              + (if $p.outputs > 0 then ["\($p.outputs) output\(if $p.outputs == 1 then "" else "s" end) to change"] else [] end)
              | join(", ")) + "."
           end),
          (if (.changes | length) > 0 then
             "",
             (.changes | sort_by(order, .address) | .[:$max][] | "- \(.action) `\(.address)`"),
             (if (.changes | length) > $max then "- and \((.changes | length) - $max) more" else empty end)
           else empty end),
          (if $risky > 0 then
             "",
             "**Needs approval.** It deletes, replaces or forgets something other than a task definition, so the apply on main waits for a reviewer in the `infrastructure-approval` environment."
           else empty end)
        ]
      | .[]'
    ;;
  *) usage ;;
esac
