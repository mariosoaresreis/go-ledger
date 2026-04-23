output "app_service_ip" {
  description = "NodePort service details for ledger-command-service"
  value       = {
    cluster_ip = kubernetes_service.ledger_app.spec[0].cluster_ip
    node_port  = kubernetes_service.ledger_app.spec[0].port[0].node_port
    type       = kubernetes_service.ledger_app.spec[0].type
  }
}

output "kafka_service_host" {
  description = "Kafka broker address inside the cluster"
  value       = "${var.kafka_release_name}.ledger.svc.cluster.local:9092"
}

output "namespace" {
  description = "Kubernetes namespace used for the ledger workloads"
  value       = kubernetes_namespace.ledger.metadata[0].name
}
