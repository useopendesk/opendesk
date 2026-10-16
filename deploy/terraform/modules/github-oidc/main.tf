# The roles GitHub Actions assumes through GitHub's OIDC provider: no long-lived
# AWS keys anywhere. Each trusts only named GitHub environments of one
# repository (and the deploy role also the main branch):
#
#   opendesk-github-deploy           environment production, or main. Pushes the
#                                    two images, registers task definitions, runs
#                                    the migration task, updates the services.
#   opendesk-github-terraform-plan   environment infrastructure-plan. Reads: AWS
#                                    read-only, the state and settings objects,
#                                    and the project's secret values (a refresh
#                                    reads the versions Terraform generated). It
#                                    writes nothing, so every plan it makes runs
#                                    with -lock=false. It reads no object outside
#                                    the state bucket and no log line.
#   opendesk-github-terraform-apply  environments infrastructure and
#                                    infrastructure-approval, which deploy only
#                                    from main. Administrator, for one hour: it
#                                    applies the reviewed configuration.
#
# Who may run in an environment is configured in GitHub (deploy/terraform/README.md,
# "Applying changes"): all three deploy only from main, and infrastructure-approval
# waits for a required reviewer. The Terraform roles trust only these environment
# subjects, so no other branch and no pull request can assume them.

data "aws_caller_identity" "current" {}
data "aws_region" "current" {}

locals {
  account  = data.aws_caller_identity.current.account_id
  region   = data.aws_region.current.region
  issuer   = "token.actions.githubusercontent.com"
  provider = var.create_oidc_provider ? aws_iam_openid_connect_provider.github[0].arn : var.existing_oidc_provider_arn
  ecs_base = "arn:aws:ecs:${local.region}:${local.account}"
  services = [for s in var.service_names : "${local.ecs_base}:service/${var.cluster_name}/${s}"]
  migrate  = "${local.ecs_base}:task-definition/${var.migrate_family}:*"
  cluster  = "${local.ecs_base}:cluster/${var.cluster_name}"
  repos    = compact([var.github_repo, var.github_repo_immutable])
  state    = "arn:aws:s3:::${var.terraform_state_bucket}"
}

resource "aws_iam_openid_connect_provider" "github" {
  count = var.create_oidc_provider ? 1 : 0

  url            = "https://${local.issuer}"
  client_id_list = ["sts.amazonaws.com"]
}

data "aws_iam_policy_document" "trust" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals {
      type        = "Federated"
      identifiers = [local.provider]
    }
    condition {
      test     = "StringEquals"
      variable = "${local.issuer}:aud"
      values   = ["sts.amazonaws.com"]
    }
    condition {
      test     = "StringEquals"
      variable = "${local.issuer}:sub"
      # Both subject forms: by name, and the immutable one with the owner's and the
      # repository's numeric ids, which GitHub sends once a repository opts in.
      values = flatten([for r in local.repos : [
        "repo:${r}:ref:refs/heads/main",
        "repo:${r}:environment:production",
      ]])
    }
  }
}

resource "aws_iam_role" "deploy" {
  name                 = var.role_name
  assume_role_policy   = data.aws_iam_policy_document.trust.json
  max_session_duration = 3600
}

data "aws_iam_policy_document" "deploy" {
  statement {
    sid       = "EcrLogin"
    actions   = ["ecr:GetAuthorizationToken"]
    resources = ["*"]
  }

  statement {
    sid = "EcrPush"
    actions = [
      "ecr:BatchCheckLayerAvailability",
      "ecr:BatchGetImage",
      "ecr:CompleteLayerUpload",
      "ecr:DescribeImages",
      "ecr:GetDownloadUrlForLayer",
      "ecr:InitiateLayerUpload",
      "ecr:PutImage",
      "ecr:UploadLayerPart",
    ]
    resources = var.ecr_repository_arns
  }

  # These ECS actions do not support resource-level permissions.
  statement {
    sid       = "TaskDefinitions"
    actions   = ["ecs:DescribeTaskDefinition", "ecs:RegisterTaskDefinition", "ecs:ListTaskDefinitions"]
    resources = ["*"]
  }

  # A new revision carries the tags of the one it is rendered from, and ECS checks
  # TagResource for them; only while registering a task definition.
  statement {
    sid       = "TagTaskDefinitions"
    actions   = ["ecs:TagResource"]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "ecs:CreateAction"
      values   = ["RegisterTaskDefinition"]
    }
  }

  statement {
    sid       = "Services"
    actions   = ["ecs:DescribeServices", "ecs:UpdateService"]
    resources = local.services
  }

  statement {
    sid       = "Migrate"
    actions   = ["ecs:RunTask"]
    resources = [local.migrate]
    condition {
      test     = "ArnEquals"
      variable = "ecs:cluster"
      values   = [local.cluster]
    }
  }

  statement {
    sid       = "MigrateStatus"
    actions   = ["ecs:DescribeTasks"]
    resources = ["${local.ecs_base}:task/${var.cluster_name}/*"]
  }

  statement {
    sid       = "PassTaskRoles"
    actions   = ["iam:PassRole"]
    resources = var.pass_role_arns
    condition {
      test     = "StringEquals"
      variable = "iam:PassedToService"
      values   = ["ecs-tasks.amazonaws.com"]
    }
  }

  statement {
    sid       = "MigrateLogs"
    actions   = ["logs:GetLogEvents", "logs:FilterLogEvents"]
    resources = ["${var.migrate_log_group_arn}:*"]
  }
}

resource "aws_iam_role_policy" "deploy" {
  name   = "deploy"
  role   = aws_iam_role.deploy.id
  policy = data.aws_iam_policy_document.deploy.json
}

# ── Terraform ─────────────────────────────────────────────────────────────

data "aws_iam_policy_document" "terraform_trust" {
  for_each = {
    plan  = ["infrastructure-plan"]
    apply = ["infrastructure", "infrastructure-approval"]
  }

  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals {
      type        = "Federated"
      identifiers = [local.provider]
    }
    condition {
      test     = "StringEquals"
      variable = "${local.issuer}:aud"
      values   = ["sts.amazonaws.com"]
    }
    condition {
      test     = "StringEquals"
      variable = "${local.issuer}:sub"
      values = flatten([for r in local.repos : [
        for e in each.value : "repo:${r}:environment:${e}"
      ]])
    }
  }
}

resource "aws_iam_role" "terraform_plan" {
  name                 = var.terraform_plan_role_name
  assume_role_policy   = data.aws_iam_policy_document.terraform_trust["plan"].json
  max_session_duration = 3600
}

resource "aws_iam_role_policy_attachment" "terraform_plan" {
  role       = aws_iam_role.terraform_plan.name
  policy_arn = "arn:aws:iam::aws:policy/ReadOnlyAccess"
}

data "aws_iam_policy_document" "terraform_plan" {
  # A refresh reads the secret versions Terraform generated.
  statement {
    sid       = "SecretValues"
    actions   = ["secretsmanager:GetSecretValue"]
    resources = ["arn:aws:secretsmanager:${local.region}:${local.account}:secret:${var.secret_prefix}/*"]
  }

  # Only through Secrets Manager: the same key encrypts attachments and logs.
  statement {
    sid       = "SecretValuesKey"
    actions   = ["kms:Decrypt"]
    resources = [var.kms_key_arn]
    condition {
      test     = "StringEquals"
      variable = "kms:ViaService"
      values   = ["secretsmanager.${local.region}.amazonaws.com"]
    }
  }

  statement {
    sid       = "StateList"
    actions   = ["s3:ListBucket"]
    resources = [local.state]
  }

  statement {
    sid       = "StateRead"
    actions   = ["s3:GetObject"]
    resources = [for k in var.terraform_state_keys : "${local.state}/${k}"]
  }

  # Plans need configuration, not data: no object outside the state bucket and
  # no log line, whatever ReadOnlyAccess grants.
  statement {
    sid           = "NoData"
    effect        = "Deny"
    actions       = ["s3:GetObject", "s3:GetObjectVersion"]
    not_resources = ["${local.state}/*"]
  }

  statement {
    sid    = "NoLogs"
    effect = "Deny"
    actions = [
      "logs:FilterLogEvents",
      "logs:GetLogEvents",
      "logs:GetLogRecord",
      "logs:GetQueryResults",
      "logs:StartLiveTail",
      "logs:StartQuery",
    ]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "terraform_plan" {
  name   = "terraform-plan"
  role   = aws_iam_role.terraform_plan.id
  policy = data.aws_iam_policy_document.terraform_plan.json
}

resource "aws_iam_role" "terraform_apply" {
  name                 = var.terraform_apply_role_name
  assume_role_policy   = data.aws_iam_policy_document.terraform_trust["apply"].json
  max_session_duration = 3600
}

# Terraform manages IAM, the network, KMS and everything else in this account.
resource "aws_iam_role_policy_attachment" "terraform_apply" {
  role       = aws_iam_role.terraform_apply.name
  policy_arn = "arn:aws:iam::aws:policy/AdministratorAccess"
}
