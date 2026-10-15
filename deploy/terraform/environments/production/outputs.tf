output "github_deploy_role_arn" {
  description = "GitHub variable AWS_DEPLOY_ROLE_ARN."
  value       = module.github_oidc.role_arn
}

output "ecr_opendesk_url" {
  value = aws_ecr_repository.this["opendesk"].repository_url
}

output "ecr_realtime_url" {
  value = aws_ecr_repository.this["realtime"].repository_url
}

output "ecs_cluster_name" {
  value = aws_ecs_cluster.this.name
}

output "ecs_services" {
  value = {
    api     = module.api.service_name
    worker  = module.worker.service_name
    gateway = module.gateway.service_name
  }
}

output "migrate_task_family" {
  value = module.migrate.family
}

output "migrate_network_configuration" {
  description = "Pass to `aws ecs run-task --network-configuration` (GitHub variable ECS_MIGRATE_NETWORK)."
  value = jsonencode({
    awsvpcConfiguration = {
      subnets        = module.network.public_subnet_ids
      securityGroups = [aws_security_group.worker.id]
      assignPublicIp = "ENABLED"
    }
  })
}

output "migrate_log_group" {
  value = module.migrate.log_group_name
}

output "alb_dns_name" {
  value = module.alb.dns_name
}

output "nats_url" {
  value = module.nats.url
}

output "s3_bucket" {
  value = module.s3.bucket
}

output "postgres_private_host" {
  description = "PlanetScale's private host; it resolves to the VPC endpoint only inside the VPC."
  value       = module.planetscale.private_host
}

output "postgres_endpoint_id" {
  value = module.planetscale.endpoint_id
}

output "pages_projects" {
  value = module.cloudflare.pages_projects
}

output "ses_identity" {
  value = module.ses.identity
}

output "ses_configuration_set" {
  value = module.ses.configuration_set_name
}

output "secret_arns" {
  description = "Secret key => ARN. Values are never output."
  value       = module.secrets.arns
}

output "help_cname_target" {
  description = "What help center custom domains CNAME to (HELP_CNAME_TARGET); null without help_domain."
  value       = local.help ? module.cloudflare_help[0].cname_target : null
}
