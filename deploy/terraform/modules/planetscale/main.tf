# PlanetScale Postgres in the ECS region, reached only over AWS PrivateLink.
#
# The database itself (size, single node or HA) is created and resized in the
# PlanetScale dashboard: going from one node to three is a setting there, not
# a migration, and keeping it out of Terraform means no plan can ever replace
# or delete it. Terraform owns the two roles the services connect with and the
# AWS side of the private connection.
#
# Roles, least privilege:
#   app      reads and writes all data (pg_read_all_data, pg_write_all_data);
#            no DDL. The API, worker and River use it.
#   migrate  inherits postgres for DDL and CREATE EXTENSION; only the one-off
#            migration task gets it. Tables it creates are covered by the app
#            role's predefined roles, so no GRANTs are needed after a migration.
#
# Connections go to the role's private host, which resolves to the VPC
# endpoint inside the VPC: 6432 is PgBouncer (transaction mode) for request
# handlers, 5432 is Postgres itself for migrations, River and the event relay,
# which need session features (LISTEN/NOTIFY). Both use sslmode=verify-full.
#
# The endpoint alone does not close the public path. After the first apply,
# restrict the database to private addresses in the dashboard
# (deploy/terraform/README.md, "Bootstrap order").

resource "planetscale_postgres_branch_role" "app" {
  organization    = var.organization
  database        = var.database
  branch          = var.branch
  name            = "${var.name}-app"
  inherited_roles = ["pg_read_all_data", "pg_write_all_data"]
}

resource "planetscale_postgres_branch_role" "migrate" {
  organization    = var.organization
  database        = var.database
  branch          = var.branch
  name            = "${var.name}-migrate"
  inherited_roles = ["postgres"]
}

locals {
  # The provider documents these as connection hosts; tolerate a scheme or port.
  host = {
    for k, r in { app = planetscale_postgres_branch_role.app, migrate = planetscale_postgres_branch_role.migrate } :
    k => split(":", replace(r.private_access_host_url, "/^[a-z]+:\\/\\//", ""))[0]
  }

  dsn = {
    for k, r in { app = planetscale_postgres_branch_role.app, migrate = planetscale_postgres_branch_role.migrate } :
    k => { for port in [5432, 6432] : port => format("postgresql://%s:%s@%s:%d/%s?sslmode=verify-full",
    urlencode(r.username), urlencode(r.password), local.host[k], port, coalesce(r.database_name, var.database_name)) }
  }
}

# ── AWS side ──────────────────────────────────────────────────────────────

resource "aws_security_group" "endpoint" {
  name        = "${var.name}-postgres"
  description = "PlanetScale PrivateLink endpoint: Postgres 5432 and PgBouncer 6432 from the services"
  vpc_id      = var.vpc_id
}

resource "aws_vpc_security_group_ingress_rule" "clients" {
  for_each = { for pair in setproduct(keys(var.client_security_group_ids), [5432, 6432]) : "${pair[0]}-${pair[1]}" => pair }

  security_group_id            = aws_security_group.endpoint.id
  referenced_security_group_id = var.client_security_group_ids[each.value[0]]
  ip_protocol                  = "tcp"
  from_port                    = each.value[1]
  to_port                      = each.value[1]
}

resource "aws_vpc_endpoint" "this" {
  vpc_id              = var.vpc_id
  vpc_endpoint_type   = "Interface"
  service_name        = planetscale_postgres_branch_role.app.private_connection_service_name
  subnet_ids          = slice(var.subnet_ids, 0, min(var.endpoint_subnet_count, length(var.subnet_ids)))
  security_group_ids  = [aws_security_group.endpoint.id]
  private_dns_enabled = true

  tags = { Name = "${var.name}-planetscale" }
}
