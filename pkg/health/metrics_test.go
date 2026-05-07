package health

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/metrics"
	"github.com/openshift-hyperfleet/hyperfleet-broker/broker"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrokerMetricsExposedOnMetricsEndpoint(t *testing.T) {
	// Use an isolated registry to avoid polluting the global one
	registry := prometheus.NewRegistry()

	// Register adapter baseline metrics (same as NewMetricsServer)
	buildInfo := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hyperfleet_adapter_build_info",
			Help: "Build information for the adapter",
		},
		[]string{"component", "version", "commit"},
	)
	upGauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "hyperfleet_adapter_up",
			Help: "Whether the adapter is up and running",
			ConstLabels: prometheus.Labels{
				"component": "test-adapter",
				"version":   "v0.1.0-test",
			},
		},
	)
	registry.MustRegister(buildInfo)
	registry.MustRegister(upGauge)
	buildInfo.WithLabelValues("test-adapter", "v0.1.0-test", "abc123").Set(1)
	upGauge.Set(1)

	// Register broker metrics with the same registry (same as main.go does via DefaultRegisterer)
	brokerMetrics := broker.NewMetricsRecorder("test-adapter", "v0.1.0-test", registry)

	// Simulate broker activity so Vec metrics emit at least one time series
	brokerMetrics.RecordConsumed("test-topic")
	brokerMetrics.RecordError("test-topic", "handler")
	brokerMetrics.RecordDuration("test-topic", 1)

	// Serve metrics from the shared registry
	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	metricsOutput := string(body)

	// Verify adapter baseline metrics are present
	assert.Contains(t, metricsOutput, "hyperfleet_adapter_build_info")
	assert.Contains(t, metricsOutput, "hyperfleet_adapter_up")

	// Verify broker metrics are registered and exposed
	assert.Contains(t, metricsOutput, "hyperfleet_broker_messages_consumed_total",
		"messages_consumed_total metric should be exposed on /metrics")
	assert.Contains(t, metricsOutput, "hyperfleet_broker_errors_total",
		"errors_total metric should be exposed on /metrics")
	assert.Contains(t, metricsOutput, "hyperfleet_broker_message_duration_seconds",
		"message_duration_seconds metric should be exposed on /metrics")
}

func TestAdapterMetricsExposedOnMetricsEndpoint(t *testing.T) {
	registry := prometheus.NewRegistry()

	// Register baseline adapter metrics (same as NewMetricsServer)
	buildInfo := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hyperfleet_adapter_build_info",
			Help: "Build information for the adapter",
		},
		[]string{"component", "version", "commit"},
	)
	upGauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "hyperfleet_adapter_up",
			Help: "Whether the adapter is up and running",
			ConstLabels: prometheus.Labels{
				"component": "test-adapter",
				"version":   "v0.1.0-test",
			},
		},
	)
	registry.MustRegister(buildInfo)
	registry.MustRegister(upGauge)
	buildInfo.WithLabelValues("test-adapter", "v0.1.0-test", "abc123").Set(1)
	upGauge.Set(1)

	// Register adapter event metrics using the same registry
	recorder := metrics.NewRecorder("test-adapter", "v0.1.0-test", "test", registry)

	// Simulate adapter activity
	recorder.RecordEventProcessed("success")
	recorder.RecordEventProcessed("failed")
	recorder.RecordEventProcessed("skipped")
	recorder.ObserveProcessingDuration(500 * time.Millisecond)
	recorder.RecordError("preconditions")

	// Serve metrics from the shared registry
	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	metricsOutput := string(body)

	// Verify baseline metrics
	assert.Contains(t, metricsOutput, "hyperfleet_adapter_build_info")
	assert.Contains(t, metricsOutput, "hyperfleet_adapter_up")

	// Verify new adapter metrics
	assert.Contains(t, metricsOutput, "hyperfleet_adapter_events_processed_total",
		"events_processed_total metric should be exposed on /metrics")
	assert.Contains(t, metricsOutput, "hyperfleet_adapter_event_processing_duration_seconds",
		"event_processing_duration_seconds metric should be exposed on /metrics")
	assert.Contains(t, metricsOutput, "hyperfleet_adapter_errors_total",
		"errors_total metric should be exposed on /metrics")

	// Verify status label values are present
	assert.Contains(t, metricsOutput, `status="success"`,
		"success status should be in output")
	assert.Contains(t, metricsOutput, `status="failed"`,
		"failed status should be in output")
	assert.Contains(t, metricsOutput, `status="skipped"`,
		"skipped status should be in output")

	// Verify error_type label
	assert.Contains(t, metricsOutput, `error_type="preconditions"`,
		"preconditions error_type should be in output")

	// Verify component and version labels are present on new metrics
	assert.Contains(t, metricsOutput, `component="test-adapter"`,
		"component label should be in output")
	assert.Contains(t, metricsOutput, `version="v0.1.0-test"`,
		"version label should be in output")
}
