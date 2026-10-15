variable "name" {
  type = string
}

variable "environment" {
  type = string
}

variable "kms_key_arn" {
  type = string
}

variable "database_url" {
  description = "Pooled connection string (PgBouncer)."
  type        = string
  sensitive   = true
}

variable "database_url_direct" {
  description = "Direct connection string (River, the event relay)."
  type        = string
  sensitive   = true
}

variable "database_url_migrate" {
  description = "Direct connection string for the role that runs migrations (DDL)."
  type        = string
  sensitive   = true
}

variable "recovery_window_in_days" {
  type    = number
  default = 30
}
