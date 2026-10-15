# NATS: one server or a JetStream cluster on Fargate, or an external NATS.
#
# One node (node_count = 1) runs standalone, with no cluster routes: if it is
# replaced, publishers reconnect and browsers resync from the outbox (below).
# Three or five nodes form a cluster that rides out losing one.
#
# Each node is its own ECS service with one task and its own Cloud Map name
# (nats-1.<namespace>, ...), so routes and client URLs are stable DNS names
# and a replaced task re-registers under the same name.
#
# JetStream data lives on the task's ephemeral storage. That is acceptable
# here: streams are created with three replicas, so a replaced node resyncs
# from its two peers, and the source of truth for every event is the
# transactional outbox in Postgres (NATS is the delivery path, not the record;
# browsers that miss events resync through GET /v1/events). Keep at least two
# nodes running at all times: the services deploy one task at a time
# (minimum healthy 0, maximum 100) and each node is a separate service, so a
# rolling deploy never takes down more than one.
#
# Cluster routes (6222) are reachable only from the NATS security group, and
# clients (4222) must present NATS_TOKEN.
#
# The alternative is a managed NATS such as Synadia Cloud: set
# mode = "external" and external_url, and no NATS tasks run. The services then
# get NATS_URL = external_url and the NATS_TOKEN secret is what they present.

locals {
  cluster = var.mode == "cluster"
  nodes   = local.cluster ? toset([for i in range(1, var.node_count + 1) : tostring(i)]) : toset([])
  routes  = join(",", [for i in range(1, var.node_count + 1) : "nats://nats-${i}.${var.namespace_name}:6222"])
  # Cluster flags only with peers to route to.
  cluster_flags = var.node_count > 1 ? [
    "--cluster_name ${var.cluster_name_nats}",
    "--cluster nats://0.0.0.0:6222",
    "--routes ${local.routes}",
  ] : []
  url = local.cluster ? join(",", [for i in range(1, var.node_count + 1) : "nats://nats-${i}.${var.namespace_name}:4222"]) : var.external_url
}

resource "aws_security_group" "nats" {
  name        = "${var.name}-nats"
  description = "NATS: clients on 4222, cluster routes and monitoring within the cluster"
  vpc_id      = var.vpc_id
}

resource "aws_vpc_security_group_ingress_rule" "clients" {
  for_each = local.cluster ? var.client_security_group_ids : {}

  security_group_id            = aws_security_group.nats.id
  referenced_security_group_id = each.value
  ip_protocol                  = "tcp"
  from_port                    = 4222
  to_port                      = 4222
}

resource "aws_vpc_security_group_ingress_rule" "routes" {
  for_each = local.cluster ? { routes = 6222, monitor = 8222, clients = 4222 } : {}

  security_group_id            = aws_security_group.nats.id
  referenced_security_group_id = aws_security_group.nats.id
  ip_protocol                  = "tcp"
  from_port                    = each.value
  to_port                      = each.value
}

resource "aws_vpc_security_group_egress_rule" "all" {
  security_group_id = aws_security_group.nats.id
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
}

module "node" {
  source   = "../ecs-service"
  for_each = local.nodes

  name           = "${var.name}-nats-${each.key}"
  container_name = "nats"
  image          = var.image
  cluster_arn    = var.cluster_arn
  cluster_name   = var.cluster_name
  cpu            = var.cpu
  memory         = var.memory
  # The official image is multi-arch.
  cpu_architecture   = var.cpu_architecture
  kms_key_arn        = var.kms_key_arn
  subnet_ids         = var.subnet_ids
  security_group_ids = [aws_security_group.nats.id]

  entry_point = ["sh", "-c"]
  # max_payload has no CLI flag, so it goes in a one-line config file; the
  # flags after -c override everything else. 4MB matches the embedded server.
  command = [join(" ", concat([
    "printf 'max_payload: 4MB\\n' > /data/nats.conf &&",
    "exec nats-server -c /data/nats.conf",
    "--name nats-${each.key}",
    "--jetstream --store_dir /data/jetstream",
    ], local.cluster_flags, [
    "--auth \"$NATS_TOKEN\"",
    "--http_port 8222",
  ]))]
  secrets = { NATS_TOKEN = var.token_secret_arn }

  port_mappings = [
    { container_port = 4222, name = "client" },
    { container_port = 6222, name = "cluster" },
    { container_port = 8222, name = "monitor" },
  ]
  health_check = {
    command      = ["CMD-SHELL", "wget -q -O /dev/null http://127.0.0.1:8222/healthz || exit 1"]
    start_period = 30
  }

  writable_paths        = ["/data"]
  ephemeral_storage_gib = var.ephemeral_storage_gib
  stop_timeout          = 60 # time to flush JetStream and leave the cluster cleanly

  desired_count                      = 1
  deployment_minimum_healthy_percent = 0
  deployment_maximum_percent         = 100

  discovery_namespace_id = var.namespace_id
  discovery_name         = "nats-${each.key}"
  enable_execute_command = var.enable_execute_command
  log_retention_days     = var.log_retention_days
}
