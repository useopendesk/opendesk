variable "name" {
  type = string
}

variable "organization" {
  description = "PlanetScale organization slug."
  type        = string
}

variable "database" {
  description = "PlanetScale database name, created in the dashboard in the ECS region."
  type        = string
}

variable "branch" {
  description = "Production branch."
  type        = string
  default     = "main"
}

variable "database_name" {
  description = "Postgres database inside the cluster, when the role does not report one."
  type        = string
  default     = "postgres"
}

variable "vpc_id" {
  type = string
}

variable "subnet_ids" {
  type = list(string)
}

variable "endpoint_subnet_count" {
  description = "Subnets the endpoint gets a network interface in (PlanetScale asks for at least two; each costs about $7/month)."
  type        = number
  default     = 2

  validation {
    condition     = var.endpoint_subnet_count >= 2
    error_message = "The PrivateLink endpoint needs at least two subnets."
  }
}

variable "client_security_group_ids" {
  description = "Security groups allowed to reach the endpoint, by name."
  type        = map(string)
}
