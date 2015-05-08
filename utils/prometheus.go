package utils

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	// PrometheusNamespace is the namespace to use for Prometheus metric names
	PrometheusNamespace = "docker_registry"
)

// PrometheusObserveDuration observes the duration between t and the call to the
// function on the given metric.
func PrometheusObserveDuration(t time.Time, metric *prometheus.SummaryVec, labels ...string) {
	metric.WithLabelValues(labels...).Observe(time.Since(t).Seconds())
}
