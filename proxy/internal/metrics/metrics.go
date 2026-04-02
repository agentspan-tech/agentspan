package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// HTTP proxy metrics
	ProxyRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agentspan_proxy_requests_total",
		Help: "Total number of proxy requests by endpoint, status code, and provider type.",
	}, []string{"endpoint", "status", "provider_type"})

	ProxyRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "agentspan_proxy_request_duration_seconds",
		Help:    "End-to-end proxy request duration in seconds (includes upstream latency).",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120},
	}, []string{"endpoint", "provider_type"})

	ProxyOverheadDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "agentspan_proxy_overhead_seconds",
		Help:    "Proxy overhead duration in seconds (auth + routing, excludes upstream).",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25},
	}, []string{"endpoint"})

	// Auth cache metrics
	AuthCacheHits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "agentspan_proxy_auth_cache_hits_total",
		Help: "Total auth cache hits (fresh entries served without calling Processing).",
	})

	AuthCacheMisses = promauto.NewCounter(prometheus.CounterOpts{
		Name: "agentspan_proxy_auth_cache_misses_total",
		Help: "Total auth cache misses (required call to Processing).",
	})

	AuthCacheStaleServed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "agentspan_proxy_auth_cache_stale_served_total",
		Help: "Total times a stale cache entry was served (fail-open during Processing outage).",
	})

	AuthCacheCircuitOpen = promauto.NewCounter(prometheus.CounterOpts{
		Name: "agentspan_proxy_auth_cache_circuit_open_total",
		Help: "Total times the auth cache circuit breaker was open.",
	})

	AuthCacheSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "agentspan_proxy_auth_cache_entries",
		Help: "Current number of entries in the auth cache.",
	})

	// Span dispatcher metrics
	SpansDispatched = promauto.NewCounter(prometheus.CounterOpts{
		Name: "agentspan_proxy_spans_dispatched_total",
		Help: "Total spans successfully enqueued for dispatch.",
	})

	SpansDropped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "agentspan_proxy_spans_dropped_total",
		Help: "Total spans dropped due to full buffer.",
	})

	SpansSent = promauto.NewCounter(prometheus.CounterOpts{
		Name: "agentspan_proxy_spans_sent_total",
		Help: "Total spans successfully sent to Processing.",
	})

	SpanSendErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "agentspan_proxy_span_send_errors_total",
		Help: "Total span send failures.",
	})

	SpanBufferUsage = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "agentspan_proxy_span_buffer_usage",
		Help: "Current number of spans in the dispatch buffer.",
	})

	SpanSendDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "agentspan_proxy_span_send_duration_seconds",
		Help:    "Duration of span send to Processing.",
		Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	})

	// Upstream provider metrics
	UpstreamRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "agentspan_proxy_upstream_duration_seconds",
		Help:    "Upstream provider request duration in seconds.",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120},
	}, []string{"provider_type", "model"})

	UpstreamErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agentspan_proxy_upstream_errors_total",
		Help: "Total upstream provider errors by type (timeout, connection, http_error).",
	}, []string{"provider_type", "error_type"})

	// Token metrics
	TokensProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agentspan_proxy_tokens_total",
		Help: "Total tokens processed by direction and provider.",
	}, []string{"direction", "provider_type"})

	// Rate limiter metrics
	RateLimitRejections = promauto.NewCounter(prometheus.CounterOpts{
		Name: "agentspan_proxy_rate_limit_rejections_total",
		Help: "Total requests rejected by per-key rate limiter.",
	})
)

// Handler returns the Prometheus HTTP handler for the /metrics endpoint.
func Handler() http.Handler {
	return promhttp.Handler()
}

// ObserveRequest records HTTP metrics for a completed proxy request.
func ObserveRequest(endpoint, providerType string, statusCode int, duration time.Duration) {
	status := strconv.Itoa(statusCode)
	ProxyRequestsTotal.WithLabelValues(endpoint, status, providerType).Inc()
	ProxyRequestDuration.WithLabelValues(endpoint, providerType).Observe(duration.Seconds())
}
