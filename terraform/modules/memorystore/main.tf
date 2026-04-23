locals {
  name_prefix = "${var.project_id}-${var.environment}"
}

# Cloud Memorystore Redis – accessible via private IP within the VPC (DIRECT_PEERING)
resource "google_redis_instance" "redis" {
  name               = "${local.name_prefix}-redis"
  tier               = var.tier
  memory_size_gb     = var.memory_size_gb
  region             = var.region
  project            = var.project_id
  location_id        = "${var.region}-a"

  authorized_network = var.vpc_self_link
  connect_mode       = "DIRECT_PEERING"

  redis_version = "REDIS_7_0"

  display_name = "Ledger Redis (${var.environment})"

  redis_configs = {
    maxmemory-policy = "allkeys-lru"
  }
}

