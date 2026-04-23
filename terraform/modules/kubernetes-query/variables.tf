variable "query_db_host" {
  type        = string
  description = "Private IP of the query-side Cloud SQL instance"
}

variable "query_db_username" {
  type    = string
  default = "ledger_query"
}

variable "query_db_password" {
  type      = string
  sensitive = true
}

variable "kafka_bootstrap_servers" {
  type        = string
  description = "Kafka bootstrap servers reachable from the query cluster"
}

variable "app_image" {
  type        = string
  description = "Full Docker image reference for ledger-query-service"
}

variable "app_replicas" {
  type    = number
  default = 1
}

variable "redis_host" {
  type    = string
  default = ""
}

