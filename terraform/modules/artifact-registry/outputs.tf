output "repository_id" {
  description = "Artifact Registry repository ID"
  value       = google_artifact_registry_repository.ledger.repository_id
}

output "repository_url" {
  description = "Full Docker repository URL (use as image prefix)"
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/ledger"
}

