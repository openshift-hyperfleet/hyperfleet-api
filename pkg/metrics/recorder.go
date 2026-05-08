// Package metrics provides Prometheus metrics recording for the HyperFleet adapter.
// It follows the HyperFleet Metrics Standard with the hyperfleet_adapter_ prefix.
package metrics

import (
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Deletion status constants per HyperFleet adapter-metrics.md standard
const (
	DeletionStatusSuccess = "success"
	DeletionStatusError   = "error"
)

// Resource type constants
const (
	ResourceTypeUnknown = "Unknown"
)

// ExtractAdapterName derives a short adapter name from the component name.
// It strips common prefixes ("hyperfleet-adapter-", "adapter-") and suffixes ("-adapter")
// to produce a short identifier suitable for the adapter_name metric label.
// If no pattern matches, it returns the component name unchanged.
//
// Examples:
//   - "hyperfleet-adapter-gcp" → "gcp"
//   - "adapter-validation" → "validation"
//   - "test-adapter" → "test"
//   - "standalone" → "standalone"
func ExtractAdapterName(component string) string {
	switch {
	case strings.HasPrefix(component, "hyperfleet-adapter-"):
		return strings.TrimPrefix(component, "hyperfleet-adapter-")
	case strings.HasPrefix(component, "adapter-"):
		return strings.TrimPrefix(component, "adapter-")
	case strings.HasSuffix(component, "-adapter"):
		return strings.TrimSuffix(component, "-adapter")
	default:
		return component
	}
}

// normalizeResourceType normalizes resource type labels to prevent cardinality issues.
// Empty or whitespace-only values are replaced with ResourceTypeUnknown.
func normalizeResourceType(resourceType string) string {
	resourceType = strings.TrimSpace(resourceType)
	if resourceType == "" {
		return ResourceTypeUnknown
	}
	return resourceType
}

// normalizeDeletionStatus validates and normalizes deletion status labels.
// Only DeletionStatusSuccess and DeletionStatusError are valid.
// Invalid values are replaced with DeletionStatusError to prevent bad time series.
func normalizeDeletionStatus(status string) string {
	status = strings.TrimSpace(status)
	switch status {
	case DeletionStatusSuccess, DeletionStatusError:
		return status
	default:
		// Invalid status values default to "error" as a safe fallback
		return DeletionStatusError
	}
}

// Recorder registers and records adapter-level Prometheus metrics.
// All methods are nil-safe: calling methods on a nil *Recorder is a no-op,
// which allows dry-run mode to skip metrics without nil checks at every call site.
type Recorder struct {
	eventsProcessed    *prometheus.CounterVec
	processingDuration prometheus.Observer
	errorsTotal        *prometheus.CounterVec
	deletionTotal      *prometheus.CounterVec
	deletionDuration   *prometheus.HistogramVec
	deletionInProgress *prometheus.GaugeVec
}

// NewRecorder creates a new Recorder and registers metrics with the given registerer.
// If reg is nil, prometheus.DefaultRegisterer is used.
// The adapterName should be a short identifier (e.g., "gcp", "validation") derived from component.
func NewRecorder(component, version, adapterName string, reg prometheus.Registerer) *Recorder {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	eventsProcessed := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hyperfleet_adapter_events_processed_total",
			Help: "Total number of CloudEvents processed by the adapter",
			ConstLabels: prometheus.Labels{
				"component":    component,
				"version":      version,
				"adapter_name": adapterName,
			},
		},
		[]string{"status"},
	)

	processingDuration := prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "hyperfleet_adapter_event_processing_duration_seconds",
			Help:    "Duration of event processing in seconds",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120},
			ConstLabels: prometheus.Labels{
				"component":    component,
				"version":      version,
				"adapter_name": adapterName,
			},
		},
	)

	errorsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hyperfleet_adapter_errors_total",
			Help: "Total number of errors encountered by the adapter",
			ConstLabels: prometheus.Labels{
				"component":    component,
				"version":      version,
				"adapter_name": adapterName,
			},
		},
		[]string{"error_type"},
	)

	deletionTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hyperfleet_adapter_resources_deleted_total",
			Help: "Total number of adapter resource deletion operations",
			ConstLabels: prometheus.Labels{
				"component":    component,
				"version":      version,
				"adapter_name": adapterName,
			},
		},
		[]string{"resource_type", "status"},
	)

	deletionDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hyperfleet_adapter_resource_deletion_duration_seconds",
			Help:    "Duration of resource deletion operations in seconds",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120},
			ConstLabels: prometheus.Labels{
				"component":    component,
				"version":      version,
				"adapter_name": adapterName,
			},
		},
		[]string{"resource_type"},
	)

	deletionInProgress := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hyperfleet_adapter_resource_deletions_in_progress",
			Help: "Number of resource deletions currently in progress",
			ConstLabels: prometheus.Labels{
				"component":    component,
				"version":      version,
				"adapter_name": adapterName,
			},
		},
		[]string{"resource_type"},
	)

	reg.MustRegister(eventsProcessed)
	reg.MustRegister(processingDuration)
	reg.MustRegister(errorsTotal)
	reg.MustRegister(deletionTotal)
	reg.MustRegister(deletionDuration)
	reg.MustRegister(deletionInProgress)

	return &Recorder{
		eventsProcessed:    eventsProcessed,
		processingDuration: processingDuration,
		errorsTotal:        errorsTotal,
		deletionTotal:      deletionTotal,
		deletionDuration:   deletionDuration,
		deletionInProgress: deletionInProgress,
	}
}

// RecordEventProcessed increments the events_processed_total counter for the given status.
// Valid status values: "success", "failed", "skipped".
func (r *Recorder) RecordEventProcessed(status string) {
	if r == nil {
		return
	}
	r.eventsProcessed.WithLabelValues(status).Inc()
}

// ObserveProcessingDuration records the event processing duration in seconds.
func (r *Recorder) ObserveProcessingDuration(d time.Duration) {
	if r == nil {
		return
	}
	r.processingDuration.Observe(d.Seconds())
}

// RecordError increments the errors_total counter for the given error type.
// Error types correspond to execution phases: "param_extraction", "preconditions",
// "resources", "post_actions".
func (r *Recorder) RecordError(errorType string) {
	if r == nil {
		return
	}
	r.errorsTotal.WithLabelValues(errorType).Inc()
}

// RecordDeletion increments the resources_deleted_total counter for the given resource type.
// resourceType should be the Kubernetes kind (e.g., "Namespace", "ServiceAccount").
// Valid status values: DeletionStatusSuccess ("success"), DeletionStatusError ("error").
// Invalid inputs are normalized to prevent bad time series.
func (r *Recorder) RecordDeletion(resourceType, status string) {
	if r == nil {
		return
	}
	resourceType = normalizeResourceType(resourceType)
	status = normalizeDeletionStatus(status)
	r.deletionTotal.WithLabelValues(resourceType, status).Inc()
}

// ObserveDeletionDuration records the deletion duration for a resource type in seconds.
// resourceType should be the Kubernetes kind (e.g., "Namespace", "ServiceAccount").
// Empty or invalid resourceType is normalized to prevent bad time series.
func (r *Recorder) ObserveDeletionDuration(resourceType string, d time.Duration) {
	if r == nil {
		return
	}
	resourceType = normalizeResourceType(resourceType)
	r.deletionDuration.WithLabelValues(resourceType).Observe(d.Seconds())
}

// IncDeletionInProgress increments the in-progress deletion gauge for a resource type.
// resourceType should be the Kubernetes kind (e.g., "Namespace", "ServiceAccount").
// Empty or invalid resourceType is normalized to prevent bad time series.
func (r *Recorder) IncDeletionInProgress(resourceType string) {
	if r == nil {
		return
	}
	resourceType = normalizeResourceType(resourceType)
	r.deletionInProgress.WithLabelValues(resourceType).Inc()
}

// DecDeletionInProgress decrements the in-progress deletion gauge for a resource type.
// resourceType should be the Kubernetes kind (e.g., "Namespace", "ServiceAccount").
// Empty or invalid resourceType is normalized to prevent bad time series.
func (r *Recorder) DecDeletionInProgress(resourceType string) {
	if r == nil {
		return
	}
	resourceType = normalizeResourceType(resourceType)
	r.deletionInProgress.WithLabelValues(resourceType).Dec()
}
