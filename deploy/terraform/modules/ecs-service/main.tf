# A Fargate task definition, and optionally its service.
#
# Each service gets its own execution role (pull the image, read exactly the
# secrets it lists, write its own log group) and its own task role (whatever
# the caller passes, nothing else). The root filesystem is read-only; the
# paths in `writable_paths` are task-local scratch volumes.
#
# The service ignores task_definition and desired_count: deploy.yml registers
# a new revision with the new image and autoscaling owns the count.

data "aws_region" "current" {}

locals {
  secret_arns = distinct(values(var.secrets))

  volumes = { for i, p in var.writable_paths : "scratch-${i}" => p }

  container = merge(
    {
      name                   = var.container_name
      image                  = var.image
      essential              = true
      readonlyRootFilesystem = var.readonly_root_filesystem
      stopTimeout            = var.stop_timeout
      environment            = [for k in sort(keys(var.environment)) : { name = k, value = var.environment[k] }]
      secrets                = [for k in sort(keys(var.secrets)) : { name = k, valueFrom = var.secrets[k] }]
      portMappings = [for p in var.port_mappings : {
        containerPort = p.container_port
        protocol      = "tcp"
        name          = p.name
      }]
      mountPoints = [for v, p in local.volumes : { sourceVolume = v, containerPath = p, readOnly = false }]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.this.name
          awslogs-region        = data.aws_region.current.region
          awslogs-stream-prefix = var.container_name
        }
      }
      linuxParameters = {
        initProcessEnabled = var.enable_execute_command
      }
    },
    var.command == null ? {} : { command = var.command },
    var.entry_point == null ? {} : { entryPoint = var.entry_point },
    var.health_check == null ? {} : {
      healthCheck = {
        command     = var.health_check.command
        interval    = var.health_check.interval
        timeout     = var.health_check.timeout
        retries     = var.health_check.retries
        startPeriod = var.health_check.start_period
      }
    },
  )
}

resource "aws_cloudwatch_log_group" "this" {
  name              = "/ecs/${var.name}"
  retention_in_days = var.log_retention_days
  kms_key_id        = var.kms_key_arn
}

# ── Execution role: used by the ECS agent, never by the application ──────

data "aws_iam_policy_document" "assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "execution" {
  name               = "${var.name}-execution"
  assume_role_policy = data.aws_iam_policy_document.assume.json
}

data "aws_iam_policy_document" "execution" {
  statement {
    sid       = "EcrAuth"
    actions   = ["ecr:GetAuthorizationToken"]
    resources = ["*"]
  }

  dynamic "statement" {
    for_each = length(var.ecr_repository_arns) > 0 ? [1] : []
    content {
      sid       = "EcrPull"
      actions   = ["ecr:BatchCheckLayerAvailability", "ecr:GetDownloadUrlForLayer", "ecr:BatchGetImage"]
      resources = var.ecr_repository_arns
    }
  }

  statement {
    sid       = "Logs"
    actions   = ["logs:CreateLogStream", "logs:PutLogEvents"]
    resources = ["${aws_cloudwatch_log_group.this.arn}:*"]
  }

  dynamic "statement" {
    for_each = length(local.secret_arns) > 0 ? [1] : []
    content {
      sid       = "Secrets"
      actions   = ["secretsmanager:GetSecretValue"]
      resources = local.secret_arns
    }
  }

  statement {
    sid       = "Kms"
    actions   = ["kms:Decrypt"]
    resources = [var.kms_key_arn]
  }
}

resource "aws_iam_role_policy" "execution" {
  name   = "execution"
  role   = aws_iam_role.execution.id
  policy = data.aws_iam_policy_document.execution.json
}

# ── Task role: what the application itself may call ──────────────────────

resource "aws_iam_role" "task" {
  name               = "${var.name}-task"
  assume_role_policy = data.aws_iam_policy_document.assume.json
}

# Keyed by name so the plan knows how many there are before the policy
# documents (which reference resources created in the same apply) are known.
resource "aws_iam_role_policy" "task" {
  for_each = var.task_role_policies

  name   = each.key
  role   = aws_iam_role.task.id
  policy = each.value
}

data "aws_iam_policy_document" "exec" {
  statement {
    actions = [
      "ssmmessages:CreateControlChannel",
      "ssmmessages:CreateDataChannel",
      "ssmmessages:OpenControlChannel",
      "ssmmessages:OpenDataChannel",
    ]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "exec" {
  count = var.enable_execute_command ? 1 : 0

  name   = "ecs-exec"
  role   = aws_iam_role.task.id
  policy = data.aws_iam_policy_document.exec.json
}

# ── Task definition ────────────────────────────────────────────────────────

resource "aws_ecs_task_definition" "this" {
  family                   = var.name
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.cpu
  memory                   = var.memory
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task.arn
  container_definitions    = jsonencode([local.container])

  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = var.cpu_architecture
  }

  dynamic "ephemeral_storage" {
    for_each = var.ephemeral_storage_gib == null ? [] : [1]
    content {
      size_in_gib = var.ephemeral_storage_gib
    }
  }

  dynamic "volume" {
    for_each = local.volumes
    content {
      name = volume.key
    }
  }
}

# ── Service ────────────────────────────────────────────────────────────────

resource "aws_service_discovery_service" "this" {
  # discovery_name is a plain string; the namespace id is only known after apply.
  count = var.create_service && var.discovery_name != null ? 1 : 0

  name = var.discovery_name

  dns_config {
    namespace_id   = var.discovery_namespace_id
    routing_policy = "MULTIVALUE"
    dns_records {
      type = "A"
      ttl  = 10
    }
  }
}

resource "aws_ecs_service" "this" {
  count = var.create_service ? 1 : 0

  name                               = var.name
  cluster                            = var.cluster_arn
  task_definition                    = aws_ecs_task_definition.this.arn
  desired_count                      = var.desired_count
  launch_type                        = var.spot ? null : "FARGATE"
  platform_version                   = "LATEST"
  enable_execute_command             = var.enable_execute_command
  propagate_tags                     = "SERVICE"
  deployment_minimum_healthy_percent = var.deployment_minimum_healthy_percent
  deployment_maximum_percent         = var.deployment_maximum_percent
  health_check_grace_period_seconds  = var.target_group_arn == null ? null : var.health_check_grace_period_seconds
  wait_for_steady_state              = false

  dynamic "capacity_provider_strategy" {
    for_each = var.spot ? [1] : []
    content {
      capacity_provider = "FARGATE_SPOT"
      weight            = 1
    }
  }

  network_configuration {
    subnets          = var.subnet_ids
    security_groups  = var.security_group_ids
    assign_public_ip = var.assign_public_ip
  }

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  dynamic "load_balancer" {
    for_each = var.target_group_arn == null ? [] : [1]
    content {
      target_group_arn = var.target_group_arn
      container_name   = var.container_name
      container_port   = var.port_mappings[0].container_port
    }
  }

  dynamic "service_registries" {
    for_each = aws_service_discovery_service.this
    content {
      registry_arn = service_registries.value.arn
    }
  }

  lifecycle {
    ignore_changes = [task_definition, desired_count]
  }
}

# ── Autoscaling on CPU ─────────────────────────────────────────────────────

resource "aws_appautoscaling_target" "this" {
  count = var.create_service && var.autoscaling != null ? 1 : 0

  service_namespace  = "ecs"
  scalable_dimension = "ecs:service:DesiredCount"
  resource_id        = "service/${var.cluster_name}/${aws_ecs_service.this[0].name}"
  min_capacity       = var.autoscaling.min
  max_capacity       = var.autoscaling.max
}

resource "aws_appautoscaling_policy" "cpu" {
  count = var.create_service && var.autoscaling != null ? 1 : 0

  name               = "${var.name}-cpu"
  policy_type        = "TargetTrackingScaling"
  service_namespace  = aws_appautoscaling_target.this[0].service_namespace
  scalable_dimension = aws_appautoscaling_target.this[0].scalable_dimension
  resource_id        = aws_appautoscaling_target.this[0].resource_id

  target_tracking_scaling_policy_configuration {
    target_value       = var.autoscaling.cpu_target
    scale_in_cooldown  = 300
    scale_out_cooldown = 60

    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageCPUUtilization"
    }
  }
}
