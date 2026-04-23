# ── Namespace ────────────────────────────────────────────────────────────────
resource "kubernetes_namespace" "ledger" {
  metadata {
    name = "ledger"
    labels = {
      managed-by = "terraform"
    }
  }
}

# ── ConfigMap – non-sensitive runtime config ─────────────────────────────────
resource "kubernetes_config_map" "ledger_config" {
  metadata {
    name      = "ledger-command-config"
    namespace = kubernetes_namespace.ledger.metadata[0].name
  }

  data = {
    # Cloud SQL private IP (set by Terraform from Cloud SQL module output)
    LEDGER_DB_HOST = var.db_host
    LEDGER_DB_PORT = "5432"

    # Kafka deployed on GKE via Helm; service name = <release-name>.<namespace>
    LEDGER_KAFKA_BOOTSTRAP_SERVERS = "${var.kafka_release_name}.${kubernetes_namespace.ledger.metadata[0].name}.svc.cluster.local:9092"

    # Redis: use Cloud Memorystore if provided, otherwise use in-cluster Redis
    LEDGER_REDIS_HOST = var.redis_host != "" ? var.redis_host : "redis.${kubernetes_namespace.ledger.metadata[0].name}.svc.cluster.local"
    LEDGER_REDIS_PORT = "6379"
  }
}

# ── Secret – sensitive credentials ───────────────────────────────────────────
resource "kubernetes_secret" "ledger_secret" {
  metadata {
    name      = "ledger-command-secret"
    namespace = kubernetes_namespace.ledger.metadata[0].name
  }

  data = {
    LEDGER_DB_USERNAME = var.db_username
    LEDGER_DB_PASSWORD = var.db_password
  }

  type = "Opaque"
}

# ── In-Cluster Redis (StatefulSet) – used when redis_host is not provided ──────
resource "kubernetes_stateful_set" "redis" {
  count            = var.redis_host == "" ? 1 : 0
  wait_for_rollout = false

  metadata {
    name      = "redis"
    namespace = kubernetes_namespace.ledger.metadata[0].name
    labels = {
      app        = "redis"
      managed-by = "terraform"
    }
  }

  spec {
    service_name = "redis"
    replicas     = 1

    selector {
      match_labels = {
        app = "redis"
      }
    }

    template {
      metadata {
        labels = {
          app = "redis"
        }
      }

      spec {
        container {
          name              = "redis"
          image             = "redis:7-alpine"
          image_pull_policy = "IfNotPresent"

          port {
            name           = "redis"
            container_port = 6379
            protocol       = "TCP"
          }

          resources {
            requests = {
              cpu    = "50m"
              memory = "64Mi"
            }
            limits = {
              cpu    = "100m"
              memory = "128Mi"
            }
          }

          volume_mount {
            name       = "redis-data"
            mount_path = "/data"
          }
        }

        security_context {
          fs_group = 999
        }
      }
    }

    volume_claim_template {
      metadata {
        name = "redis-data"
      }

      spec {
        access_modes       = ["ReadWriteOnce"]
        storage_class_name = "standard"

        resources {
          requests = {
            storage = "1Gi"
          }
        }
      }
    }
  }

  depends_on = [kubernetes_namespace.ledger]
}

# ── Redis Service – expose Redis within the cluster ──────────────────────────
resource "kubernetes_service" "redis" {
  count = var.redis_host == "" ? 1 : 0

  metadata {
    name      = "redis"
    namespace = kubernetes_namespace.ledger.metadata[0].name
    labels = {
      app        = "redis"
      managed-by = "terraform"
    }
  }

  spec {
    cluster_ip = "None"  # Headless service for StatefulSet

    selector = {
      app = "redis"
    }

    port {
      name       = "redis"
      port       = 6379
      target_port = 6379
      protocol    = "TCP"
    }
  }

  depends_on = [kubernetes_namespace.ledger]
}


resource "helm_release" "kafka" {
  name               = var.kafka_release_name
  repository         = "https://charts.bitnami.com/bitnami"
  chart              = "kafka"
  version            = "26.8.5" # Kafka 3.7.x – pin and bump deliberately
  namespace          = kubernetes_namespace.ledger.metadata[0].name
  timeout            = 600
  wait               = false

  # KRaft combined mode: single pod acts as broker + controller
  set {
    name  = "controller.replicaCount"
    value = tostring(var.kafka_replicas)
  }

  # No separate brokers; controller nodes handle both roles
  set {
    name  = "broker.replicaCount"
    value = "0"
  }

  set {
    name  = "controller.persistence.enabled"
    value = "true"
  }

  set {
    name  = "controller.persistence.size"
    value = "8Gi"
  }

  # PLAINTEXT inside the cluster – add TLS for production
  set {
    name  = "listeners.client.protocol"
    value = "PLAINTEXT"
  }

  set {
    name  = "listeners.controller.protocol"
    value = "PLAINTEXT"
  }

  set {
    name  = "listeners.interbroker.protocol"
    value = "PLAINTEXT"
  }

  set {
    name  = "externalAccess.enabled"
    value = "false"
  }

  set {
    name  = "serviceAccount.create"
    value = "true"
  }

  # Docker Hub tags used by this chart version may be unavailable; pin image to Bitnami public ECR.
  set {
    name  = "image.registry"
    value = "public.ecr.aws"
  }

  set {
    name  = "image.repository"
    value = "bitnami/kafka"
  }

  set {
    name  = "image.tag"
    value = "3.7.0"
  }

  # Reduce resource requirements for dev environment
  set {
    name  = "controller.resources.requests.cpu"
    value = "100m"
  }

  set {
    name  = "controller.resources.requests.memory"
    value = "512Mi"
  }

  set {
    name  = "controller.resources.limits.cpu"
    value = "500m"
  }

  set {
    name  = "controller.resources.limits.memory"
    value = "768Mi"
  }

  # Cap JVM heap well below the container limit to leave room for off-heap (page cache, etc.)
  set {
    name  = "controller.heapOpts"
    value = "-Xmx512m -Xms512m"
  }

  # Dev-only: avoid aggressive probe restarts in single-node constrained clusters.
  set {
    name  = "controller.livenessProbe.enabled"
    value = "false"
  }

  set {
    name  = "controller.readinessProbe.enabled"
    value = "false"
  }

  # Fallback probe tuning in case chart-level probe toggles are ignored by this version.
  set {
    name  = "livenessProbe.initialDelaySeconds"
    value = "120"
  }

  set {
    name  = "livenessProbe.failureThreshold"
    value = "12"
  }

  set {
    name  = "readinessProbe.initialDelaySeconds"
    value = "90"
  }

  set {
    name  = "readinessProbe.failureThreshold"
    value = "12"
  }

  set {
    name  = "controller.livenessProbe.initialDelaySeconds"
    value = "120"
  }

  set {
    name  = "controller.livenessProbe.failureThreshold"
    value = "12"
  }

  set {
    name  = "controller.readinessProbe.initialDelaySeconds"
    value = "90"
  }

  set {
    name  = "controller.readinessProbe.failureThreshold"
    value = "12"
  }

  # Single-broker settings: Kafka default replication factor is 3, which
  # fails on a single-node cluster. Set to 1 for dev.
  set {
    name  = "controller.extraConfig"
    value = "offsets.topic.replication.factor=1\ntransaction.state.log.replication.factor=1\ntransaction.state.log.min.isr=1\ndefault.replication.factor=1\nmin.insync.replicas=1"
  }

  depends_on = [kubernetes_namespace.ledger]
}

# External LoadBalancer for Kafka – allows the query service (us-east1) to reach
# Kafka in the command cluster (us-central1) over the public internet.
# For production, replace with VPC peering and remove this resource.
resource "kubernetes_service" "kafka_external" {
  metadata {
    name      = "kafka-external"
    namespace = kubernetes_namespace.ledger.metadata[0].name
    annotations = {
      "cloud.google.com/load-balancer-type" = "External"
    }
  }

  spec {
    type = "LoadBalancer"

    selector = {
      "app.kubernetes.io/name"      = "kafka"
      "app.kubernetes.io/instance"  = var.kafka_release_name
      "app.kubernetes.io/component" = "controller-eligible"
    }

    port {
      name        = "kafka-external"
      port        = 9095
      target_port = 9095
      protocol    = "TCP"
    }
  }

  depends_on = [helm_release.kafka]
}
# ── Application Deployment ────────────────────────────────────────────────────
resource "kubernetes_deployment" "ledger_app" {
  wait_for_rollout = false

  metadata {
    name      = "ledger-command-service"
    namespace = kubernetes_namespace.ledger.metadata[0].name
    labels = {
      app        = "ledger-command-service"
      managed-by = "terraform"
    }
  }

  spec {
    replicas = var.app_replicas

    selector {
      match_labels = {
        app = "ledger-command-service"
      }
    }

    template {
      metadata {
        labels = {
          app = "ledger-command-service"
        }
      }

      spec {
        container {
          name              = "ledger-command-service"
          image             = var.app_image
          image_pull_policy = "Always"

          port {
            name           = "http"
            container_port = 8080
            protocol       = "TCP"
          }

          env_from {
            config_map_ref {
              name = kubernetes_config_map.ledger_config.metadata[0].name
            }
          }

          env_from {
            secret_ref {
              name = kubernetes_secret.ledger_secret.metadata[0].name
            }
          }

          readiness_probe {
            http_get {
              path = "/actuator/health"
              port = 8080
            }
            initial_delay_seconds = 120
            period_seconds        = 10
            failure_threshold     = 5
          }

          liveness_probe {
            http_get {
              path = "/actuator/health"
              port = 8080
            }
            initial_delay_seconds = 180
            period_seconds        = 15
            failure_threshold     = 3
          }

          resources {
            requests = {
              cpu    = "50m"
              memory = "128Mi"
            }
            limits = {
              cpu    = "200m"
              memory = "256Mi"
            }
          }
        }
      }
    }
  }

  depends_on = [
    helm_release.kafka,
    kubernetes_config_map.ledger_config,
    kubernetes_secret.ledger_secret,
    kubernetes_stateful_set.redis,
  ]
}

# ── Application Service – NodePort exposes the API externally ──────────────────
resource "kubernetes_service" "ledger_app" {
  metadata {
    name      = "ledger-command-service"
    namespace = kubernetes_namespace.ledger.metadata[0].name
    labels = {
      app        = "ledger-command-service"
      managed-by = "terraform"
    }
  }

  spec {
    selector = {
      app = "ledger-command-service"
    }

    port {
      name        = "http"
      port        = 80
      target_port = 8080
      protocol    = "TCP"
    }

    type = "NodePort"
  }
}

# ── Ingress – External Access via LoadBalancer ───────────────────────────────
resource "kubernetes_ingress_v1" "ledger_app" {
  metadata {
    name      = "ledger-command-service"
    namespace = kubernetes_namespace.ledger.metadata[0].name
    labels = {
      app        = "ledger-command-service"
      managed-by = "terraform"
    }
    annotations = {
      "kubernetes.io/ingress.class" = "gce"
      # GCP creates a regional TCP load balancer by default
      # "cloud.google.com/load-balancer-type" = "Internal"  # Uncomment for internal LB
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
              name = kubernetes_service.ledger_app.metadata[0].name
              port {
                number = 80
              }
            }
          }
        }
      }
    }
  }

  depends_on = [kubernetes_service.ledger_app]
}
