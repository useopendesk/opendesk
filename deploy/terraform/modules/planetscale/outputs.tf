output "database_url" {
  description = "App role through PgBouncer (6432)."
  value       = local.dsn.app[6432]
  sensitive   = true
}

output "database_url_direct" {
  description = "App role, direct (5432): River and the event relay."
  value       = local.dsn.app[5432]
  sensitive   = true
}

output "database_url_migrate" {
  description = "Migrate role, direct (5432): the migration task only."
  value       = local.dsn.migrate[5432]
  sensitive   = true
}

output "endpoint_id" {
  value = aws_vpc_endpoint.this.id
}

output "private_host" {
  value = local.host.app
}
