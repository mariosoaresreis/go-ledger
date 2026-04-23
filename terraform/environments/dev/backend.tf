# Remote state stored in GCS.
# Create the bucket before running terraform init:
#   gsutil mb -p YOUR_PROJECT_ID -l us-central1 gs://YOUR_PROJECT_ID-ledger-tfstate
#   gsutil versioning set on gs://YOUR_PROJECT_ID-ledger-tfstate
terraform {
  backend "gcs" {
    bucket = "ledger-493222-ledger-tfstate"
    prefix = "ledger/dev/terraform.tfstate"
  }
}
