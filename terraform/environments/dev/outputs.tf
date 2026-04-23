output "gke_cluster_name" {
  description = "Name of the GKE cluster"
  value       = module.gke.cluster_name
}
output "gke_cluster_location" {
  description = "Region of the GKE cluster"
  value       = module.gke.cluster_location
}
output "cloud_sql_instance_name" {
  description = "Cloud SQL instance name"
  value       = module.cloud_sql.instance_name
}
output "cloud_sql_private_ip" {
  description = "Private IP of the Cloud SQL instance (accessible only within the VPC)"
  value       = module.cloud_sql.private_ip
  sensitive   = true
}
output "artifact_registry_url" {
  description = "Artifact Registry URL – prefix your image tags with this"
  value       = module.artifact_registry.repository_url
}
output "app_external_ip" {
  description = "External IP of the ledger-command-service LoadBalancer"
  value       = module.kubernetes_app.app_service_ip
}
output "kubectl_config_command" {
  description = "Run this command to configure kubectl for this cluster"
  value       = "gcloud container clusters get-credentials ${module.gke.cluster_name} --region ${var.region} --project ${var.project_id}"
}
output "docker_push_command" {
  description = "Configure Docker to push to Artifact Registry"
  value       = "gcloud auth configure-docker ${var.region}-docker.pkg.dev"
}
