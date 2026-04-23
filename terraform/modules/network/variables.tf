variable "project_id" {
  type        = string
  description = "GCP project ID"
}

variable "environment" {
  type        = string
  description = "Environment name (e.g. dev, staging, prod)"
}

variable "region" {
  type        = string
  description = "GCP region"
  default     = "us-central1"
}

variable "subnet_cidr" {
  type        = string
  description = "Primary CIDR range for the GKE subnet"
  default     = "10.10.0.0/24"
}

variable "pods_cidr" {
  type        = string
  description = "Secondary CIDR range for GKE pods"
  default     = "10.20.0.0/16"
}

variable "services_cidr" {
  type        = string
  description = "Secondary CIDR range for GKE services"
  default     = "10.30.0.0/16"
}

