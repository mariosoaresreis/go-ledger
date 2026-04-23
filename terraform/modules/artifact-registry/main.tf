# Google Artifact Registry – stores the ledger-command-service Docker image
resource "google_artifact_registry_repository" "ledger" {
  location      = var.region
  repository_id = "ledger"
  description   = "Ledger application Docker images"
  format        = "DOCKER"
  project       = var.project_id

  labels = {
    managed-by = "terraform"
  }
}

