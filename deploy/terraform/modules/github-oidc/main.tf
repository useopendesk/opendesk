# The role deploy.yml assumes through GitHub's OIDC provider: no long-lived
# AWS keys anywhere. It trusts only the main branch and the production
# environment of one repository, and may only push the two images, register
# task definitions, run the migration task and update the opendesk services.

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
      values = flatten([for r in compact([var.github_repo, var.github_repo_immutable]) : [
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
