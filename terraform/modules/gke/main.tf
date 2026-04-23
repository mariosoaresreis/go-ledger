locals {
  name_prefix = "${var.project_id}-${var.environment}"
}

# Dedicated service account for GKE nodes (least-privilege)
resource "google_service_account" "gke_nodes" {
  account_id   = "${var.environment}-gke-sa"
  display_name = "GKE Node Service Account (${var.environment})"
  project      = var.project_id
}

resource "google_project_iam_member" "gke_log_writer" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.gke_nodes.email}"
}

resource "google_project_iam_member" "gke_metric_writer" {
  project = var.project_id
  role    = "roles/monitoring.metricWriter"
  member  = "serviceAccount:${google_service_account.gke_nodes.email}"
}

resource "google_project_iam_member" "gke_artifact_reader" {
  project = var.project_id
  role    = "roles/artifactregistry.reader"
  member  = "serviceAccount:${google_service_account.gke_nodes.email}"
}

resource "google_container_cluster" "primary" {
  name     = "${local.name_prefix}-gke"
  location = "${var.region}-${var.zone_suffix}"
  project  = var.project_id

  # Remove default node pool; we manage our own below
  remove_default_node_pool = true
  initial_node_count       = 1

  network    = var.vpc_id
  subnetwork = var.subnet_name

  networking_mode = "VPC_NATIVE"

  ip_allocation_policy {
    cluster_secondary_range_name  = var.pods_range_name
    services_secondary_range_name = var.services_range_name
  }

  # Workload Identity lets pods authenticate to GCP APIs without key files
  workload_identity_config {
    workload_pool = "${var.project_id}.svc.id.goog"
  }

  private_cluster_config {
    enable_private_nodes    = true
    enable_private_endpoint = false
    master_ipv4_cidr_block  = "172.16.0.0/28"
  }

  master_authorized_networks_config {
    cidr_blocks {
      cidr_block   = "0.0.0.0/0"
      display_name = "All – restrict to your office CIDR in production"
    }
  }

  # Never enable deletion_protection in dev; flip to true for prod
  deletion_protection = false
}

resource "google_container_node_pool" "primary" {
  name       = "${local.name_prefix}-node-pool"
  location   = "${var.region}-${var.zone_suffix}"
  cluster    = google_container_cluster.primary.name
  project    = var.project_id
  node_count = var.node_count

  lifecycle {
    ignore_changes = [node_count]
  }

  autoscaling {
    min_node_count = var.min_nodes
    max_node_count = var.max_nodes
  }

  management {
    auto_repair  = true
    auto_upgrade = true
  }

  node_config {
    machine_type = var.machine_type
    disk_size_gb = var.disk_size_gb
    disk_type    = "pd-standard"

    service_account = google_service_account.gke_nodes.email
    oauth_scopes = [
      "https://www.googleapis.com/auth/cloud-platform",
    ]

    workload_metadata_config {
      mode = "GKE_METADATA"
    }

    labels = {
      environment = var.environment
      managed-by  = "terraform"
    }
  }
}

