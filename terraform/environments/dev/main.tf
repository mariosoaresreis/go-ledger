# ── Enable required GCP APIs ──────────────────────────────────────────────────
locals {
  gcp_services = [
    "container.googleapis.com",
    "sqladmin.googleapis.com",
    "artifactregistry.googleapis.com",
    "servicenetworking.googleapis.com",
    "compute.googleapis.com",
    "iam.googleapis.com",
  ]
}
resource "google_project_service" "apis" {
  for_each = toset(local.gcp_services)
  project  = var.project_id
  service  = each.value
  disable_on_destroy = false
}
# ── VPC + Subnets + NAT + Private Service Access ──────────────────────────────
module "network" {
  source = "../../modules/network"
  project_id    = var.project_id
  environment   = var.environment
  region        = var.region
  subnet_cidr   = var.subnet_cidr
  pods_cidr     = var.pods_cidr
  services_cidr = var.services_cidr
  depends_on = [google_project_service.apis]
}
# ── GKE Cluster ───────────────────────────────────────────────────────────────
module "gke" {
  source = "../../modules/gke"
  project_id          = var.project_id
  environment         = var.environment
  region              = var.region
  vpc_id              = module.network.vpc_id
  subnet_name         = module.network.gke_subnet_name
  pods_range_name     = module.network.pods_range_name
  services_range_name = module.network.services_range_name
  node_count          = var.node_count
  machine_type        = var.machine_type
  min_nodes           = var.min_nodes
  max_nodes           = var.max_nodes
}
# ── Cloud SQL (PostgreSQL 16) ─────────────────────────────────────────────────
module "cloud_sql" {
  source = "../../modules/cloud-sql"
  project_id    = var.project_id
  environment   = var.environment
  region        = var.region
  vpc_self_link = module.network.vpc_self_link
  tier          = var.db_tier
  db_username   = var.db_username
  db_password   = var.db_password
  # Cloud SQL private IP requires the private service access connection to exist first
  depends_on = [module.network]
}
# ── Artifact Registry ─────────────────────────────────────────────────────────
module "artifact_registry" {
  source = "../../modules/artifact-registry"
  project_id = var.project_id
  region     = var.region
  depends_on = [google_project_service.apis]
}
# ── Kubernetes Workloads (Namespace, ConfigMap, Secret, Kafka, Redis, App) ─────
module "kubernetes_app" {
  source = "../../modules/kubernetes-app"
  db_host      = module.cloud_sql.private_ip
  db_username  = var.db_username
  db_password  = var.db_password
  app_image    = "${module.artifact_registry.repository_url}/ledger-command-service:${var.app_image_tag}"
  app_replicas = var.app_replicas
  kafka_replicas = var.kafka_replicas
  depends_on = [module.gke, module.cloud_sql]
}
