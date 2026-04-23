output "instance_name" {
  description = "Cloud SQL instance name"
  value       = google_sql_database_instance.postgres.name
}

output "private_ip" {
  description = "Private IP address of the Cloud SQL instance"
  value       = google_sql_database_instance.postgres.private_ip_address
}

output "connection_name" {
  description = "Cloud SQL connection name (project:region:instance)"
  value       = google_sql_database_instance.postgres.connection_name
}

output "public_ip" {
  description = "Public IP address of the Cloud SQL instance (only set when enable_public_ip=true)"
  value       = google_sql_database_instance.postgres.public_ip_address
}

output "db_host" {
  description = "Best IP to use: public if enabled, else private"
  value       = var.enable_public_ip ? google_sql_database_instance.postgres.public_ip_address : google_sql_database_instance.postgres.private_ip_address
}
