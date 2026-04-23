variable "db_host" {
  type        = string
  description = "Private IP of the Cloud SQL PostgreSQL instance"
}

variable "db_username" {
  type        = string
  description = "PostgreSQL username"
  default     = "ledger"
}

variable "db_password" {
  type        = string
  description = "PostgreSQL password"
  sensitive   = true
}

variable "redis_host" {
  type        = string
  description = "Private IP of the Cloud Memorystore Redis instance (optional; if not provided, in-cluster Redis will be used)"
  default     = ""
}

variable "app_image" {
  type        = string
  description = "Full Docker image reference for ledger-command-service (e.g. us-central1-docker.pkg.dev/PROJECT/ledger/ledger-command-service:TAG)"
}

variable "app_replicas" {
  type        = number
  description = "Number of application pod replicas"
  default     = 2
}

variable "kafka_release_name" {
  type        = string
  description = "Helm release name for Kafka (also becomes the K8s Service name)"
  default     = "kafka"
}

variable "kafka_replicas" {
  type        = number
  description = "Number of Kafka controller/broker pods"
  default     = 1
}

