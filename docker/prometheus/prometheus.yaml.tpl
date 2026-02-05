global:
  scrape_interval: 15s
  scrape_timeout: 10s

  evaluation_interval: 15s

  external_labels:
    cluster: "polyglot-lab"
    env: "${APP_ENV:-integration}"

alerting:
  alertmanagers:
    - static_configs:
        - targets: ["alertmanager:9093"]

rule_files:
  - /etc/prometheus/alerts.yaml

scrape_configs:
  - job_name: "golang-gin-app"
    metrics_path: /metrics
    static_configs:
      - targets: ["golang-gin-app:8080"]
        labels:
          stack: "polyglot-lab"
          kind: "app"
          env: "${APP_ENV:-integration}"

  - job_name: "java-springboot-app"
    metrics_path: /metrics
    static_configs:
      - targets: ["java-springboot-app:8080"]
        labels:
          stack: "polyglot-lab"
          kind: "app"
          env: "${APP_ENV:-integration}"

  - job_name: "python-django-app"
    metrics_path: /metrics
    static_configs:
      - targets: ["python-django-app:8080"]
        labels:
          stack: "polyglot-lab"
          kind: "app"
          env: "${APP_ENV:-integration}"

  - job_name: "alloy"
    metrics_path: /metrics
    static_configs:
      - targets: ["alloy:12345"]
        labels:
          stack: "polyglot-lab"
          kind: "infra"
          env: "${APP_ENV:-integration}"

  - job_name: "loki"
    metrics_path: /metrics
    static_configs:
      - targets: ["loki:3100"]
        labels:
          stack: "polyglot-lab"
          kind: "infra"
          env: "${APP_ENV:-integration}"

  - job_name: "tempo"
    metrics_path: /metrics
    static_configs:
      - targets: ["tempo:3200"]
        labels:
          stack: "polyglot-lab"
          kind: "infra"
          env: "${APP_ENV:-integration}"

  - job_name: "prometheus"
    metrics_path: /metrics
    static_configs:
      - targets: ["prometheus:9090"]
        labels:
          stack: "polyglot-lab"
          kind: "infra"
          env: "${APP_ENV:-integration}"

  - job_name: "grafana"
    metrics_path: /metrics
    static_configs:
      - targets: ["grafana:3000"]
        labels:
          stack: "polyglot-lab"
          kind: "infra"
          env: "${APP_ENV:-integration}"
