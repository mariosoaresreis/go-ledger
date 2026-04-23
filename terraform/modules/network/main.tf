locals {
  name_prefix = "${var.project_id}-${var.environment}"
}

resource "google_compute_network" "vpc" {
  name                    = "${local.name_prefix}-vpc"
  auto_create_subnetworks = false
  project                 = var.project_id
}

resource "google_compute_subnetwork" "gke" {
  name                     = "${local.name_prefix}-gke-subnet"
  ip_cidr_range            = var.subnet_cidr
  region                   = var.region
  network                  = google_compute_network.vpc.id
  project                  = var.project_id
  private_ip_google_access = true

  secondary_ip_range {
    range_name    = "${local.name_prefix}-pods"
    ip_cidr_range = var.pods_cidr
  }

  secondary_ip_range {
    range_name    = "${local.name_prefix}-services"
    ip_cidr_range = var.services_cidr
  }
}

# Cloud Router + NAT – lets private GKE nodes reach the internet (image pulls, Maven, etc.)
resource "google_compute_router" "router" {
  name    = "${local.name_prefix}-router"
  region  = var.region
  network = google_compute_network.vpc.id
  project = var.project_id
}

resource "google_compute_router_nat" "nat" {
  name                               = "${local.name_prefix}-nat"
  router                             = google_compute_router.router.name
  region                             = var.region
  project                            = var.project_id
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "ALL_SUBNETWORKS_ALL_IP_RANGES"
}

# Private services access range – used by Cloud SQL and Memorystore (PRIVATE_SERVICE_ACCESS)
resource "google_compute_global_address" "private_service_range" {
  name          = "${local.name_prefix}-private-svc-range"
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  prefix_length = 16
  network       = google_compute_network.vpc.id
  project       = var.project_id
}

resource "google_service_networking_connection" "private_vpc_connection" {
  network                 = google_compute_network.vpc.id
  service                 = "servicenetworking.googleapis.com"
  reserved_peering_ranges = [google_compute_global_address.private_service_range.name]

  # ABANDON skips the API call to delete the peering connection during `terraform destroy`.
  # This prevents the "Producer services are still using this connection" error that occurs
  # when Cloud SQL hasn't fully released the connection before Terraform tries to remove it.
  # GCP cleans up the peering automatically once all dependent services are deleted.
  deletion_policy = "ABANDON"
}

# Firewall – allow internal traffic within the VPC
resource "google_compute_firewall" "allow_internal" {
  name    = "${local.name_prefix}-allow-internal"
  network = google_compute_network.vpc.name
  project = var.project_id

  allow {
    protocol = "tcp"
    ports    = ["0-65535"]
  }

  allow {
    protocol = "udp"
    ports    = ["0-65535"]
  }

  allow {
    protocol = "icmp"
  }

  source_ranges = [var.subnet_cidr, var.pods_cidr, var.services_cidr]
}

