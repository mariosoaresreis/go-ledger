output "vpc_id" {
  description = "VPC network ID"
  value       = google_compute_network.vpc.id
}

output "vpc_self_link" {
  description = "VPC network self-link (used by Cloud SQL / Memorystore)"
  value       = google_compute_network.vpc.self_link
}

output "gke_subnet_name" {
  description = "Name of the GKE subnet"
  value       = google_compute_subnetwork.gke.name
}

output "gke_subnet_self_link" {
  description = "Self-link of the GKE subnet"
  value       = google_compute_subnetwork.gke.self_link
}

output "pods_range_name" {
  description = "Secondary range name for pods"
  value       = "${var.project_id}-${var.environment}-pods"
}

output "services_range_name" {
  description = "Secondary range name for services"
  value       = "${var.project_id}-${var.environment}-services"
}

output "private_vpc_connection_id" {
  description = "Private VPC connection ID (use as depends_on for Cloud SQL)"
  value       = google_service_networking_connection.private_vpc_connection.id
}

