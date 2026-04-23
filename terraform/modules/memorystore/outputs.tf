output "redis_host" {
  description = "Private IP of the Memorystore Redis instance"
  value       = google_redis_instance.redis.host
}

output "redis_port" {
  description = "Port of the Memorystore Redis instance"
  value       = google_redis_instance.redis.port
}

output "instance_id" {
  description = "Redis instance ID"
  value       = google_redis_instance.redis.id
}

