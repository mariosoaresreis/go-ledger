variable "project_id" {
  type        = string
  description = "GCP project ID"
}

variable "environment" {
  type        = string
  description = "Environment name"
}

variable "region" {
  type        = string
  description = "GCP region"
}

variable "vpc_self_link" {
  type        = string
  description = "VPC network self-link for authorized network"
}

variable "tier" {
  type        = string
  description = "BASIC (single) or STANDARD_HA (replicated)"
  default     = "BASIC"
}

variable "memory_size_gb" {
  type        = number
  description = "Redis memory size in GB"
  default     = 1
}

