output "arns" {
  description = "Secret key => ARN (never the values)."
  value       = { for k, s in aws_secretsmanager_secret.this : k => s.arn }
}

output "generated_keys" {
  value = local.generated_keys
}

output "manual_keys" {
  value = local.manual
}
