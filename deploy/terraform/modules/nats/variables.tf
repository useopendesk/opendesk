variable "name" {
  description = "Name prefix, e.g. opendesk (services become opendesk-nats-1..3)."
  type        = string
}

variable "mode" {
  description = "cluster = run NATS on ECS; external = use external_url (e.g. Synadia Cloud)."
  type        = string
  default     = "cluster"

  validation {
    condition     = contains(["cluster", "external"], var.mode)
    error_message = "mode must be cluster or external."
  }
}

variable "external_url" {
  description = "NATS URL(s) when mode = external."
  type        = string
  default     = ""
}

variable "node_count" {
  description = "1 for a standalone server; a cluster needs an odd number (3 or 5) for quorum."
  type        = number
  default     = 3
}

variable "image" {
  type    = string
  default = "nats:2.15-alpine"
}

variable "cluster_name_nats" {
  description = "NATS cluster name."
  type        = string
  default     = "opendesk"
}

variable "cluster_arn" {
  type = string
}

variable "cluster_name" {
  type = string
}

variable "vpc_id" {
  type = string
}

variable "subnet_ids" {
  type = list(string)
}

variable "client_security_group_ids" {
  description = "Security groups allowed to connect on 4222, keyed by a static name (api, worker, gateway)."
  type        = map(string)
}

variable "namespace_id" {
  type = string
}

variable "namespace_name" {
  type = string
}

variable "token_secret_arn" {
  type = string
}

variable "kms_key_arn" {
  type = string
}

variable "cpu" {
  type    = number
  default = 256
}

variable "memory" {
  type    = number
  default = 512
}

variable "cpu_architecture" {
  type    = string
  default = "ARM64"
}

variable "ephemeral_storage_gib" {
  type    = number
  default = 30
}

variable "enable_execute_command" {
  type    = bool
  default = false
}

variable "log_retention_days" {
  type    = number
  default = 30
}
