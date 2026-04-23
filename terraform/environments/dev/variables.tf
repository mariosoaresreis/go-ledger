# ── Required ──────────────────────────────────────────────────────────────────
variable "project_id" {
  type        = string
  description = "GCP project ID where all resources are provisioned"
}
variable "db_password" {
  type        = string
  description = "Password for the Cloud SQL ledger user"
  sensitive   = true
}
# ── Optional (defaults match the design doc) ──────────────────────────────────
variable "environment" {
  type        = string
  description = "Environment label (dev, staging, prod)"
  default     = "dev"
}
variable "region" {
  type        = string
  description = "GCP region"
  default     = "us-central1"
}
variable "subnet_cidr" {
  type    = string
  default = "10.10.0.0/24"
}
variable "pods_cidr" {
  type    = string
  default = "10.20.0.0/16"
}
variable "services_cidr" {
  type    = string
  default = "10.30.0.0/16"
}
variable "node_count" {
  type        = number
  description = "Initial nodes per zone in the GKE node pool"
  default     = 2
}
variable "machine_type" {
  type    = string
  default = "e2-standard-2"
}
variable "min_nodes" {
  type    = number
  default = 1
}
variable "max_nodes" {
  type    = number
  default = 3
}
variable "db_tier" {
  type        = string
  description = "Cloud SQL machine tier (db-f1-micro for dev, db-custom-2-7680 for prod)"
  default     = "db-f1-micro"
}
variable "db_username" {
  type    = string
  default = "ledger"
}
variable "redis_memory_size_gb" {
  type    = number
  default = 1
}
variable "app_image_tag" {
  type        = string
  description = "Docker image tag to deploy (e.g. git SHA or semver)"
  default     = "latest"
}
variable "app_replicas" {
  type        = number
  description = "Number of ledger-command-service pod replicas"
  default     = 2
}
variable "kafka_replicas" {
  type        = number
  description = "Number of Kafka controller pods"
  default     = 1
}
