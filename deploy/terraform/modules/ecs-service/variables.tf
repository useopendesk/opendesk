variable "name" {
  description = "Task definition family and service name, e.g. opendesk-api."
  type        = string
}

variable "container_name" {
  type = string
}

variable "image" {
  type = string
}

variable "cluster_arn" {
  type    = string
  default = null
}

variable "cluster_name" {
  type    = string
  default = null
}

variable "create_service" {
  description = "false registers only the task definition (one-off tasks such as migrations)."
  type        = bool
  default     = true
}

variable "cpu" {
  type    = number
  default = 512
}

variable "memory" {
  type    = number
  default = 1024
}

variable "cpu_architecture" {
  description = "ARM64 (Graviton, about 20% cheaper) or X86_64. Images must be built for it."
  type        = string
  default     = "ARM64"

  validation {
    condition     = contains(["ARM64", "X86_64"], var.cpu_architecture)
    error_message = "cpu_architecture must be ARM64 or X86_64."
  }
}

variable "command" {
  type    = list(string)
  default = null
}

variable "entry_point" {
  type    = list(string)
  default = null
}

variable "environment" {
  description = "Plain environment variables."
  type        = map(string)
  default     = {}
}

variable "secrets" {
  description = "Environment variable name => Secrets Manager secret ARN."
  type        = map(string)
  default     = {}
}

variable "port_mappings" {
  type = list(object({
    container_port = number
    name           = string
  }))
  default = []
}

variable "health_check" {
  type = object({
    command      = list(string)
    interval     = optional(number, 30)
    timeout      = optional(number, 5)
    retries      = optional(number, 3)
    start_period = optional(number, 30)
  })
  default = null
}

variable "readonly_root_filesystem" {
  type    = bool
  default = true
}

variable "writable_paths" {
  description = "Container paths backed by task-local scratch volumes (the root filesystem is read-only)."
  type        = list(string)
  default     = []
}

variable "ephemeral_storage_gib" {
  description = "Fargate ephemeral storage, 21-200 GiB. null = the 20 GiB default."
  type        = number
  default     = null
}

variable "stop_timeout" {
  type    = number
  default = 30
}

variable "task_role_policies" {
  description = "Least-privilege inline policies for the application, name => policy JSON. Empty = the task role has no permissions."
  type        = map(string)
  default     = {}
}

variable "ecr_repository_arns" {
  description = "ECR repositories the execution role may pull from (empty for public images)."
  type        = list(string)
  default     = []
}

variable "kms_key_arn" {
  description = "Key for the log group and the secrets."
  type        = string
}

variable "log_retention_days" {
  type    = number
  default = 30
}

variable "subnet_ids" {
  type    = list(string)
  default = []
}

variable "security_group_ids" {
  type    = list(string)
  default = []
}

variable "assign_public_ip" {
  description = "Public subnets without a NAT Gateway: the public IP is the egress path. Security groups close ingress."
  type        = bool
  default     = true
}

variable "desired_count" {
  type    = number
  default = 1
}

variable "deployment_minimum_healthy_percent" {
  type    = number
  default = 100
}

variable "deployment_maximum_percent" {
  type    = number
  default = 200
}

variable "target_group_arn" {
  description = "Attach the first port mapping to this target group."
  type        = string
  default     = null
}

variable "health_check_grace_period_seconds" {
  type    = number
  default = 60
}

variable "discovery_namespace_id" {
  description = "Cloud Map namespace to register the tasks in (A records, MULTIVALUE). Used when discovery_name is set."
  type        = string
  default     = null
}

variable "discovery_name" {
  description = "Cloud Map service name, e.g. nats-1 => nats-1.<namespace>."
  type        = string
  default     = null
}

variable "enable_execute_command" {
  type    = bool
  default = false
}

variable "autoscaling" {
  type = object({
    min        = number
    max        = number
    cpu_target = optional(number, 60)
  })
  default = null
}

variable "spot" {
  description = "Run on Fargate Spot (the cluster must list FARGATE_SPOT as a capacity provider). For work that tolerates a two-minute interruption."
  type        = bool
  default     = false
}
