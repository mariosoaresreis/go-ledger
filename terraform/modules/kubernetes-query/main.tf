# ── Namespace ────────────────────────────────────────────────────────────────
resource "kubernetes_namespace" "ledger_query" {
  metadata {
    name = "ledger-query"
    labels = {
      managed-by = "terraform"
      component  = "query"
    }
  }
}

# ── In-cluster Redis ──────────────────────────────────────────────────────────
resource "kubernetes_stateful_set" "redis" {
  count            = var.redis_host == "" ? 1 : 0
  wait_for_rollout = false

  metadata {
    name      = "redis"
    namespace = kubernetes_namespace.ledger_query.metadata[0].name
    labels    = { app = "redis" }
  }

  spec {
    service_name = "redis"
    replicas     = 1

    selector { match_labels = { app = "redis" } }

    template {
      metadata { labels = { app = "redis" } }
      spec {
        container {
          name  = "redis"
          image = "redis:7-alpine"
          port { container_port = 6379 }
          resources {
            requests = { cpu = "50m", memory = "64Mi" }
            limits   = { cpu = "100m", memory = "128Mi" }
          }
        }
      }
    }
  }
}

resource "kubernetes_service" "redis" {
  count = var.redis_host == "" ? 1 : 0
  metadata {
    name      = "redis"
    namespace = kubernetes_namespace.ledger_query.metadata[0].name
  }
  spec {
    selector   = { app = "redis" }
    cluster_ip = "None"
    port {
      port        = 6379
      target_port = 6379
    }
  }
}

# ── ConfigMap ─────────────────────────────────────────────────────────────────
resource "kubernetes_config_map" "query_config" {
  metadata {
    name      = "ledger-query-config"
    namespace = kubernetes_namespace.ledger_query.metadata[0].name
  }

  data = {
    QUERY_DB_HOST                = var.query_db_host
    QUERY_DB_PORT                = "5432"
    LEDGER_KAFKA_BOOTSTRAP_SERVERS = var.kafka_bootstrap_servers
    LEDGER_REDIS_HOST            = var.redis_host != "" ? var.redis_host : "redis.ledger-query.svc.cluster.local"
    LEDGER_REDIS_PORT            = "6379"
  }
}

# ── Secret ────────────────────────────────────────────────────────────────────
resource "kubernetes_secret" "query_secret" {
  metadata {
    name      = "ledger-query-secret"
    namespace = kubernetes_namespace.ledger_query.metadata[0].name
  }
  data = {
    QUERY_DB_USERNAME = var.query_db_username
    QUERY_DB_PASSWORD = var.query_db_password
  }
  type = "Opaque"
}

# ── Deployment ────────────────────────────────────────────────────────────────
resource "kubernetes_deployment" "query_app" {
  wait_for_rollout = false

  metadata {
    name      = "ledger-query-service"
    namespace = kubernetes_namespace.ledger_query.metadata[0].name
    labels = {
      app        = "ledger-query-service"
      managed-by = "terraform"
    }
  }

  spec {
    replicas = var.app_replicas

    selector { match_labels = { app = "ledger-query-service" } }

    template {
      metadata { labels = { app = "ledger-query-service" } }

      spec {
        container {
          name              = "ledger-query-service"
          image             = var.app_image
          image_pull_policy = "Always"

          port {
            name           = "http"
            container_port = 8081
          }

          env_from {
            config_map_ref {
              name = kubernetes_config_map.query_config.metadata[0].name
            }
          }
          env_from {
            secret_ref {
              name = kubernetes_secret.query_secret.metadata[0].name
            }
          }

          env {
            name  = "JAVA_TOOL_OPTIONS"
            value = "-Xmx320m -Xms128m -XX:+UseContainerSupport"
          }

          readiness_probe {
            http_get {
              path = "/actuator/health"
              port = 8081
            }
            initial_delay_seconds = 60
            period_seconds        = 10
            failure_threshold     = 10
          }

          liveness_probe {
            http_get {
              path = "/actuator/health"
              port = 8081
            }
            initial_delay_seconds = 90
            period_seconds        = 15
            failure_threshold     = 5
          }

          resources {
            requests = { cpu = "50m", memory = "200Mi" }
            limits   = { cpu = "500m", memory = "512Mi" }
          }
        }
      }
    }
  }

  depends_on = [
    kubernetes_config_map.query_config,
    kubernetes_secret.query_secret,
    kubernetes_stateful_set.redis,
  ]
}

# ── Service ───────────────────────────────────────────────────────────────────
resource "kubernetes_service" "query_app" {
  metadata {
    name      = "ledger-query-service"
    namespace = kubernetes_namespace.ledger_query.metadata[0].name
    labels    = { app = "ledger-query-service", managed-by = "terraform" }
  }
  spec {
    selector = { app = "ledger-query-service" }
    port {
      name        = "http"
      port        = 80
      target_port = 8081
    }
    type = "NodePort"
  }
}

# ── Ingress ───────────────────────────────────────────────────────────────────
resource "kubernetes_ingress_v1" "query_app" {
  metadata {
    name      = "ledger-query-service"
    namespace = kubernetes_namespace.ledger_query.metadata[0].name
    labels    = { app = "ledger-query-service", managed-by = "terraform" }
    annotations = {
      "kubernetes.io/ingress.class" = "gce"
    }
  }

  spec {
    rule {
      http {
        path {
          path      = "/"
          path_type = "Prefix"
          backend {
            service {
              name = kubernetes_service.query_app.metadata[0].name
              port { number = 80 }
            }
          }
        }
      }
    }
  }

  depends_on = [kubernetes_service.query_app]
}

