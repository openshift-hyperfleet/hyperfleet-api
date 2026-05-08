package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRecorder(t *testing.T) {
	registry := prometheus.NewRegistry()
	recorder := NewRecorder("test-adapter", "v0.1.0", "test", registry)
	require.NotNil(t, recorder)

	// Trigger all metrics so they appear in Gather()
	recorder.RecordEventProcessed("success")
	recorder.ObserveProcessingDuration(1 * time.Millisecond)
	recorder.RecordError("test")

	families, err := registry.Gather()
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, f := range families {
		names[f.GetName()] = true
	}

	assert.True(t, names["hyperfleet_adapter_events_processed_total"],
		"events_processed_total should be registered")
	assert.True(t, names["hyperfleet_adapter_event_processing_duration_seconds"],
		"event_processing_duration_seconds should be registered")
	assert.True(t, names["hyperfleet_adapter_errors_total"],
		"errors_total should be registered")
}

func TestRecordEventProcessed(t *testing.T) {
	registry := prometheus.NewRegistry()
	recorder := NewRecorder("test-adapter", "v0.1.0", "test", registry)

	recorder.RecordEventProcessed("success")
	recorder.RecordEventProcessed("success")
	recorder.RecordEventProcessed("failed")
	recorder.RecordEventProcessed("skipped")
	recorder.RecordEventProcessed("skipped")
	recorder.RecordEventProcessed("skipped")

	families, err := registry.Gather()
	require.NoError(t, err)

	var eventsFamily *dto.MetricFamily
	for _, f := range families {
		if f.GetName() == "hyperfleet_adapter_events_processed_total" {
			eventsFamily = f
			break
		}
	}
	require.NotNil(t, eventsFamily, "events_processed_total metric family should exist")

	counts := make(map[string]float64)
	for _, m := range eventsFamily.GetMetric() {
		for _, l := range m.GetLabel() {
			if l.GetName() == "status" {
				counts[l.GetValue()] = m.GetCounter().GetValue()
			}
		}
	}

	assert.Equal(t, float64(2), counts["success"], "success count")
	assert.Equal(t, float64(1), counts["failed"], "failed count")
	assert.Equal(t, float64(3), counts["skipped"], "skipped count")
}

func TestRecordEventProcessed_ConstLabels(t *testing.T) {
	registry := prometheus.NewRegistry()
	recorder := NewRecorder("my-adapter", "v1.2.3", "my", registry)

	recorder.RecordEventProcessed("success")

	families, err := registry.Gather()
	require.NoError(t, err)

	var eventsFamily *dto.MetricFamily
	for _, f := range families {
		if f.GetName() == "hyperfleet_adapter_events_processed_total" {
			eventsFamily = f
			break
		}
	}
	require.NotNil(t, eventsFamily)

	// Verify component and version ConstLabels are present
	m := eventsFamily.GetMetric()[0]
	labels := make(map[string]string)
	for _, l := range m.GetLabel() {
		labels[l.GetName()] = l.GetValue()
	}

	assert.Equal(t, "my-adapter", labels["component"], "component label")
	assert.Equal(t, "v1.2.3", labels["version"], "version label")
}

func TestObserveProcessingDuration(t *testing.T) {
	registry := prometheus.NewRegistry()
	recorder := NewRecorder("test-adapter", "v0.1.0", "test", registry)

	recorder.ObserveProcessingDuration(500 * time.Millisecond)
	recorder.ObserveProcessingDuration(2 * time.Second)

	families, err := registry.Gather()
	require.NoError(t, err)

	var durationFamily *dto.MetricFamily
	for _, f := range families {
		if f.GetName() == "hyperfleet_adapter_event_processing_duration_seconds" {
			durationFamily = f
			break
		}
	}
	require.NotNil(t, durationFamily, "event_processing_duration_seconds metric family should exist")

	m := durationFamily.GetMetric()[0]
	histogram := m.GetHistogram()
	require.NotNil(t, histogram)

	assert.Equal(t, uint64(2), histogram.GetSampleCount(), "sample count")
	assert.InDelta(t, 2.5, histogram.GetSampleSum(), 0.01, "sample sum")

	// Verify bucket boundaries match expected values
	expectedBuckets := []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120}
	buckets := histogram.GetBucket()
	require.Len(t, buckets, len(expectedBuckets), "number of buckets")
	for i, b := range buckets {
		assert.Equal(t, expectedBuckets[i], b.GetUpperBound(), "bucket %d upper bound", i)
	}
}

func TestRecordError(t *testing.T) {
	registry := prometheus.NewRegistry()
	recorder := NewRecorder("test-adapter", "v0.1.0", "test", registry)

	recorder.RecordError("param_extraction")
	recorder.RecordError("preconditions")
	recorder.RecordError("preconditions")
	recorder.RecordError("resources")

	families, err := registry.Gather()
	require.NoError(t, err)

	var errorsFamily *dto.MetricFamily
	for _, f := range families {
		if f.GetName() == "hyperfleet_adapter_errors_total" {
			errorsFamily = f
			break
		}
	}
	require.NotNil(t, errorsFamily, "errors_total metric family should exist")

	counts := make(map[string]float64)
	for _, m := range errorsFamily.GetMetric() {
		for _, l := range m.GetLabel() {
			if l.GetName() == "error_type" {
				counts[l.GetValue()] = m.GetCounter().GetValue()
			}
		}
	}

	assert.Equal(t, float64(1), counts["param_extraction"], "param_extraction error count")
	assert.Equal(t, float64(2), counts["preconditions"], "preconditions error count")
	assert.Equal(t, float64(1), counts["resources"], "resources error count")
}

func TestRecordDeletion(t *testing.T) {
	registry := prometheus.NewRegistry()
	recorder := NewRecorder("test-adapter", "v0.1.0", "test", registry)

	recorder.RecordDeletion("Namespace", DeletionStatusSuccess)
	recorder.RecordDeletion("Namespace", DeletionStatusSuccess)
	recorder.RecordDeletion("ServiceAccount", DeletionStatusError)
	recorder.RecordDeletion("ConfigMap", DeletionStatusSuccess)

	families, err := registry.Gather()
	require.NoError(t, err)

	var deletionFamily *dto.MetricFamily
	for _, f := range families {
		if f.GetName() == "hyperfleet_adapter_resources_deleted_total" {
			deletionFamily = f
			break
		}
	}
	require.NotNil(t, deletionFamily, "resources_deleted_total metric family should exist")

	counts := make(map[string]float64)
	for _, m := range deletionFamily.GetMetric() {
		labels := make(map[string]string)
		for _, l := range m.GetLabel() {
			labels[l.GetName()] = l.GetValue()
		}
		key := labels["resource_type"] + "/" + labels["status"]
		counts[key] = m.GetCounter().GetValue()
	}

	assert.Equal(t, float64(2), counts["Namespace/success"], "Namespace success count")
	assert.Equal(t, float64(1), counts["ServiceAccount/error"], "ServiceAccount error count")
	assert.Equal(t, float64(1), counts["ConfigMap/success"], "ConfigMap success count")
}

func TestObserveDeletionDuration(t *testing.T) {
	registry := prometheus.NewRegistry()
	recorder := NewRecorder("test-adapter", "v0.1.0", "test", registry)

	recorder.ObserveDeletionDuration("Namespace", 500*time.Millisecond)
	recorder.ObserveDeletionDuration("Namespace", 2*time.Second)
	recorder.ObserveDeletionDuration("ServiceAccount", 100*time.Millisecond)

	families, err := registry.Gather()
	require.NoError(t, err)

	var durationFamily *dto.MetricFamily
	for _, f := range families {
		if f.GetName() == "hyperfleet_adapter_resource_deletion_duration_seconds" {
			durationFamily = f
			break
		}
	}
	require.NotNil(t, durationFamily, "resource_deletion_duration_seconds metric family should exist")

	// Find Namespace histogram
	var namespaceHistogram *dto.Histogram
	for _, m := range durationFamily.GetMetric() {
		for _, l := range m.GetLabel() {
			if l.GetName() == "resource_type" && l.GetValue() == "Namespace" {
				namespaceHistogram = m.GetHistogram()
				break
			}
		}
	}
	require.NotNil(t, namespaceHistogram, "Namespace histogram should exist")

	assert.Equal(t, uint64(2), namespaceHistogram.GetSampleCount(), "Namespace sample count")
	assert.InDelta(t, 2.5, namespaceHistogram.GetSampleSum(), 0.01, "Namespace sample sum")
}

func TestDeletionInProgress(t *testing.T) {
	registry := prometheus.NewRegistry()
	recorder := NewRecorder("test-adapter", "v0.1.0", "test", registry)

	recorder.IncDeletionInProgress("Namespace")
	recorder.IncDeletionInProgress("Namespace")
	recorder.IncDeletionInProgress("ServiceAccount")
	recorder.DecDeletionInProgress("Namespace")

	families, err := registry.Gather()
	require.NoError(t, err)

	var gaugeFamily *dto.MetricFamily
	for _, f := range families {
		if f.GetName() == "hyperfleet_adapter_resource_deletions_in_progress" {
			gaugeFamily = f
			break
		}
	}
	require.NotNil(t, gaugeFamily, "resource_deletions_in_progress metric family should exist")

	gauges := make(map[string]float64)
	for _, m := range gaugeFamily.GetMetric() {
		for _, l := range m.GetLabel() {
			if l.GetName() == "resource_type" {
				gauges[l.GetValue()] = m.GetGauge().GetValue()
			}
		}
	}

	assert.Equal(t, float64(1), gauges["Namespace"], "Namespace gauge value")
	assert.Equal(t, float64(1), gauges["ServiceAccount"], "ServiceAccount gauge value")
}

func TestNewRecorder_DeletionMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	recorder := NewRecorder("test-adapter", "v0.1.0", "test", registry)
	require.NotNil(t, recorder)

	// Trigger deletion metrics so they appear in Gather()
	recorder.RecordDeletion("Namespace", DeletionStatusSuccess)
	recorder.ObserveDeletionDuration("Namespace", 1*time.Millisecond)
	recorder.IncDeletionInProgress("Namespace")

	families, err := registry.Gather()
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, f := range families {
		names[f.GetName()] = true
	}

	assert.True(t, names["hyperfleet_adapter_resources_deleted_total"],
		"resources_deleted_total should be registered")
	assert.True(t, names["hyperfleet_adapter_resource_deletion_duration_seconds"],
		"resource_deletion_duration_seconds should be registered")
	assert.True(t, names["hyperfleet_adapter_resource_deletions_in_progress"],
		"resource_deletions_in_progress should be registered")
}

func TestNilRecorderNoPanic(t *testing.T) {
	var recorder *Recorder

	// All methods should be no-ops and not panic
	assert.NotPanics(t, func() {
		recorder.RecordEventProcessed("success")
	}, "RecordEventProcessed on nil recorder")

	assert.NotPanics(t, func() {
		recorder.ObserveProcessingDuration(1 * time.Second)
	}, "ObserveProcessingDuration on nil recorder")

	assert.NotPanics(t, func() {
		recorder.RecordError("test_error")
	}, "RecordError on nil recorder")

	assert.NotPanics(t, func() {
		recorder.RecordDeletion("Namespace", DeletionStatusSuccess)
	}, "RecordDeletion on nil recorder")

	assert.NotPanics(t, func() {
		recorder.ObserveDeletionDuration("Namespace", 1*time.Second)
	}, "ObserveDeletionDuration on nil recorder")

	assert.NotPanics(t, func() {
		recorder.IncDeletionInProgress("Namespace")
	}, "IncDeletionInProgress on nil recorder")

	assert.NotPanics(t, func() {
		recorder.DecDeletionInProgress("Namespace")
	}, "DecDeletionInProgress on nil recorder")
}

func TestExtractAdapterName(t *testing.T) {
	tests := []struct {
		component string
		want      string
	}{
		{"hyperfleet-adapter-gcp", "gcp"},
		{"hyperfleet-adapter-aws", "aws"},
		{"adapter-validation", "validation"},
		{"test-adapter", "test"},
		{"my-adapter", "my"},
		{"standalone", "standalone"},
	}

	for _, tt := range tests {
		t.Run(tt.component, func(t *testing.T) {
			got := ExtractAdapterName(tt.component)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeResourceType(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"valid kind", "Namespace", "Namespace"},
		{"empty string", "", "Unknown"},
		{"whitespace only", "   ", "Unknown"},
		{"with leading/trailing spaces", "  Deployment  ", "Deployment"},
		{"lowercase (no change)", "pod", "pod"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeResourceType(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeDeletionStatus(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"valid success", "success", "success"},
		{"valid error", "error", "error"},
		{"invalid failed", "failed", "error"},
		{"invalid skipped", "skipped", "error"},
		{"empty string", "", "error"},
		{"whitespace only", "   ", "error"},
		{"typo in status", "sucess", "error"}, //nolint:misspell // intentional typo for testing
		{"uppercase SUCCESS", "SUCCESS", "error"},
		{"with spaces", " success ", "success"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeDeletionStatus(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRecordDeletion_Normalization(t *testing.T) {
	registry := prometheus.NewRegistry()
	recorder := NewRecorder("test-adapter", "v0.1.0", "test", registry)

	// Record with invalid status - should normalize to "error"
	recorder.RecordDeletion("Namespace", "invalid-status")
	// Record with empty resourceType - should normalize to "Unknown"
	recorder.RecordDeletion("", "success")
	// Record with valid values
	recorder.RecordDeletion("ServiceAccount", "success")

	families, err := registry.Gather()
	require.NoError(t, err)

	var deletionFamily *dto.MetricFamily
	for _, f := range families {
		if f.GetName() == "hyperfleet_adapter_resources_deleted_total" {
			deletionFamily = f
			break
		}
	}
	require.NotNil(t, deletionFamily)

	counts := make(map[string]float64)
	for _, m := range deletionFamily.GetMetric() {
		labels := make(map[string]string)
		for _, l := range m.GetLabel() {
			labels[l.GetName()] = l.GetValue()
		}
		key := labels["resource_type"] + "/" + labels["status"]
		counts[key] = m.GetCounter().GetValue()
	}

	// invalid-status normalized to error
	assert.Equal(t, float64(1), counts["Namespace/error"], "Invalid status should normalize to error")
	// empty resourceType normalized to Unknown
	assert.Equal(t, float64(1), counts["Unknown/success"], "Empty resourceType should normalize to Unknown")
	// valid values unchanged
	assert.Equal(t, float64(1), counts["ServiceAccount/success"], "Valid values should be preserved")
}
