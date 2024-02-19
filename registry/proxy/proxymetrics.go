package proxy

import (
	"expvar"
	"sync/atomic"

	prometheus "github.com/distribution/distribution/v3/metrics"
	"github.com/docker/go-metrics"
)

var (
	// requests is the number of total incoming proxy request received for blob/manifest
	requests = prometheus.ProxyNamespace.NewLabeledCounter("requests", "The number of total incoming proxy request received", "type")
	// hits is the number of total proxy request hits for blob/manifest
	hits = prometheus.ProxyNamespace.NewLabeledCounter("hits", "The number of total proxy request hits", "type")
	// hits is the number of total proxy request misses for blob/manifest
	misses = prometheus.ProxyNamespace.NewLabeledCounter("misses", "The number of total proxy request misses", "type")
	// pulledBytes is the size of total bytes pulled from the upstream for blob/manifest
	pulledBytes = prometheus.ProxyNamespace.NewLabeledCounter("pulled_bytes", "The size of total bytes pulled from the upstream", "type")
	// pushedBytes is the size of total bytes pushed to the client for blob/manifest
	pushedBytes = prometheus.ProxyNamespace.NewLabeledCounter("pushed_bytes", "The size of total bytes pushed to the client", "type")
)

// Metrics is used to hold metric counters
// related to the proxy
type Metrics struct {
	Requests    uint64
	Hits        uint64
	Misses      uint64
	BytesPulled uint64
	BytesPushed uint64
}

type proxyMetricsCollector struct {
	blobMetrics     Metrics
	manifestMetrics Metrics
}

// proxyMetrics tracks metrics about the proxy cache.  This is
// kept globally and made available via expvar.
var proxyMetrics = &proxyMetricsCollector{}

func init() {
	registry := expvar.Get("registry")
	if registry == nil {
		registry = expvar.NewMap("registry")
	}

	pm := registry.(*expvar.Map).Get("proxy")
	if pm == nil {
		pm = &expvar.Map{}
		pm.(*expvar.Map).Init()
		registry.(*expvar.Map).Set("proxy", pm)
	}

	pm.(*expvar.Map).Set("blobs", expvar.Func(func() interface{} {
		return proxyMetrics.blobMetrics
	}))

	pm.(*expvar.Map).Set("manifests", expvar.Func(func() interface{} {
		return proxyMetrics.manifestMetrics
	}))

	metrics.Register(prometheus.ProxyNamespace)
	initPrometheusMetrics("blob")
	initPrometheusMetrics("manifest")
}

func initPrometheusMetrics(value string) {
	requests.WithValues(value).Inc(0)
	hits.WithValues(value).Inc(0)
	misses.WithValues(value).Inc(0)
	pulledBytes.WithValues(value).Inc(0)
	pushedBytes.WithValues(value).Inc(0)
}

// BlobPull tracks metrics about blobs pulled into the cache
func (pmc *proxyMetricsCollector) BlobPull(bytesPulled uint64) {
	atomic.AddUint64(&pmc.blobMetrics.Misses, 1)
	atomic.AddUint64(&pmc.blobMetrics.BytesPulled, bytesPulled)

	misses.WithValues("blob").Inc(1)
	pulledBytes.WithValues("blob").Inc(float64(bytesPulled))
}

// BlobPush tracks metrics about blobs pushed to clients
func (pmc *proxyMetricsCollector) BlobPush(bytesPushed uint64, isHit bool) {
	atomic.AddUint64(&pmc.blobMetrics.Requests, 1)
	atomic.AddUint64(&pmc.blobMetrics.BytesPushed, bytesPushed)

	requests.WithValues("blob").Inc(1)
	pushedBytes.WithValues("blob").Inc(float64(bytesPushed))

	if isHit {
		atomic.AddUint64(&pmc.blobMetrics.Hits, 1)

		hits.WithValues("blob").Inc(1)
	}
}

// ManifestPull tracks metrics related to Manifests pulled into the cache
func (pmc *proxyMetricsCollector) ManifestPull(bytesPulled uint64) {
	atomic.AddUint64(&pmc.manifestMetrics.Misses, 1)
	atomic.AddUint64(&pmc.manifestMetrics.BytesPulled, bytesPulled)

	misses.WithValues("manifest").Inc(1)
	pulledBytes.WithValues("manifest").Inc(float64(bytesPulled))
}

// ManifestPush tracks metrics about manifests pushed to clients
func (pmc *proxyMetricsCollector) ManifestPush(bytesPushed uint64, isHit bool) {
	atomic.AddUint64(&pmc.manifestMetrics.Requests, 1)
	atomic.AddUint64(&pmc.manifestMetrics.BytesPushed, bytesPushed)

	requests.WithValues("manifest").Inc(1)
	pushedBytes.WithValues("manifest").Inc(float64(bytesPushed))

	if isHit {
		atomic.AddUint64(&pmc.manifestMetrics.Hits, 1)

		hits.WithValues("manifest").Inc(1)
	}
}
