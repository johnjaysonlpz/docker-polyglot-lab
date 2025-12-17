package server

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

type Metrics struct {
	Registry                   *prometheus.Registry
	HTTPRequestsTotal          *prometheus.CounterVec
	HTTPRequestDurationSeconds *prometheus.HistogramVec
	BuildInfo                  *prometheus.GaugeVec
}

func NewMetrics(cfg Config) *Metrics {
	reg := prometheus.NewRegistry()

	httpRequestsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests processed.",
		},
		[]string{"service", "method", "path", "status"},
	)

	httpRequestDurationSeconds := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latencies in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service", "method", "path", "status"},
	)

	buildInfo := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "build_info",
			Help: "Build information for the service.",
		},
		[]string{"service", "version", "build_time"},
	)

	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		httpRequestsTotal,
		httpRequestDurationSeconds,
		buildInfo,
	)

	buildInfo.WithLabelValues(cfg.ServiceName, cfg.Version, cfg.BuildTime).Set(1)

	return &Metrics{
		Registry:                   reg,
		HTTPRequestsTotal:          httpRequestsTotal,
		HTTPRequestDurationSeconds: httpRequestDurationSeconds,
		BuildInfo:                  buildInfo,
	}
}
