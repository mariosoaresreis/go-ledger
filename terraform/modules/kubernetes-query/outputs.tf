output "query_service_ip" {
  description = "External IP of the ledger-query-service (from Ingress)"
  value       = try(kubernetes_ingress_v1.query_app.status[0].load_balancer[0].ingress[0].ip, "pending")
}

output "namespace" {
  value = kubernetes_namespace.ledger_query.metadata[0].name
}

