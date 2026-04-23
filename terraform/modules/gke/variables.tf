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
  description = "GCP region for the cluster"
}

variable "zone_suffix" {
  type        = string
  description = "Zone suffix appended to region (a, b, c, d). Defaults to 'a'."
  default     = "a"
}

variable "vpc_id" {
  type        = string
  description = "VPC network ID"
}

variable "subnet_name" {
  type        = string
  description = "Subnet name for GKE nodes"
}

variable "pods_range_name" {
  type        = string
  description = "Secondary range name for pods"
}

variable "services_range_name" {
  type        = string
  description = "Secondary range name for services"
}

variable "node_count" {
  type        = number
  description = "Initial number of nodes per zone"
  default     = 2
}

variable "machine_type" {
  type        = string
  description = "GCE machine type for nodes"
  default     = "e2-standard-2"
}

variable "disk_size_gb" {
  type        = number
  description = "Boot disk size in GB"
  default     = 50
}

variable "min_nodes" {
  type        = number
  description = "Minimum nodes for autoscaling"
  default     = 1
}

variable "max_nodes" {
  type        = number
  description = "Maximum nodes for autoscaling"
  default     = 3
}

