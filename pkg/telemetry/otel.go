package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// InitTraceProvider initializes OpenTelemetry trace provider
// Uses stdout exporter (traces output to logs, no external Collector needed)
// Future upgrade: Switch to OTLP HTTP exporter by changing only the exporter creation
func InitTraceProvider(
	ctx context.Context, serviceName, serviceVersion string, samplingRate float64,
) (*trace.TracerProvider, error) {
	// Create stdout exporter
	exporter, err := stdouttrace.New(
		stdouttrace.WithPrettyPrint(), // Formatted output
	)
	if err != nil {
		logger.WithError(ctx, err).Error("Failed to create OpenTelemetry stdout exporter")
		return nil, err
	}

	// Create resource (service information)
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(serviceVersion),
		),
	)
	if err != nil {
		logger.With(ctx,
			"service_name", serviceName,
			"service_version", serviceVersion,
		).WithError(err).Error("Failed to create OpenTelemetry resource")
		return nil, err
	}

	// Determine sampler based on sampling rate
	var sampler trace.Sampler
	switch {
	case samplingRate >= 1.0:
		sampler = trace.AlwaysSample() // Sample all
	case samplingRate <= 0.0:
		sampler = trace.NeverSample() // Sample none
	default:
		sampler = trace.TraceIDRatioBased(samplingRate)
	}

	// Create trace provider
	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(res),
		trace.WithSampler(sampler),
	)

	// Set global trace provider
	otel.SetTracerProvider(tp)

	return tp, nil
}

// Shutdown gracefully shuts down the trace provider
func Shutdown(ctx context.Context, tp *trace.TracerProvider) error {
	if tp == nil {
		return nil
	}
	return tp.Shutdown(ctx)
}
