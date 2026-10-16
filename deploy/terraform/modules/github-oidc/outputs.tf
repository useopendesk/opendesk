output "role_arn" {
  description = "The deploy role (GitHub variable AWS_DEPLOY_ROLE_ARN in production)."
  value       = aws_iam_role.deploy.arn
}

output "terraform_plan_role_arn" {
  description = "The read-only Terraform role (AWS_TERRAFORM_ROLE_ARN in infrastructure-plan)."
  value       = aws_iam_role.terraform_plan.arn
}

output "terraform_apply_role_arn" {
  description = "The Terraform apply role (AWS_TERRAFORM_ROLE_ARN in infrastructure and infrastructure-approval)."
  value       = aws_iam_role.terraform_apply.arn
}
