package telemetry

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

const (
	samplerAlwaysOn             = "always_on"
	samplerAlwaysOff            = "always_off"
	samplerTraceIDRatio         = "traceidratio"
	envOtelTracesSampler        = "OTEL_TRACES_SAMPLER"
	envOtelTracesSamplerArg     = "OTEL_TRACES_SAMPLER_ARG"
	envOtelExporterOtlpEndpoint = "OTEL_EXPORTER_OTLP_ENDPOINT"
	envOtelExporterOtlpProtocol = "OTEL_EXPORTER_OTLP_PROTOCOL"
	parentBasedTraceIDRatio     = "parentbased_traceidratio"
	parentBasedAlwaysOn         = "parentbased_always_on"
	parentBasedAlwaysOff        = "parentbased_always_off"
	defaultSamplingRate         = 1.0
)

// InitTraceProvider initializes OpenTelemetry trace provider
// Configuration is driven entirely by standard OpenTelemetry environment variables:
//   - OTEL_EXPORTER_OTLP_ENDPOINT: OTLP collector endpoint (if not set, uses stdout)
//   - OTEL_EXPORTER_OTLP_PROTOCOL: "grpc" (default) or "http/protobuf"
//   - OTEL_TRACES_SAMPLER: sampler type (default: "parentbased_traceidratio")
//   - OTEL_TRACES_SAMPLER_ARG: sampling rate 0.0-1.0 (default: 1.0)
//   - OTEL_RESOURCE_ATTRIBUTES: additional resource attributes (k=v,k2=v2)
//   - K8S_NAMESPACE: kubernetes namespace (added as k8s.namespace.name)
func InitTraceProvider(ctx context.Context, serviceName, serviceVersion string) (*trace.TracerProvider, error) {

	var exporter trace.SpanExporter
	var err error

	if otlpEndpoint := os.Getenv(envOtelExporterOtlpEndpoint); otlpEndpoint != "" {
		protocol := os.Getenv(envOtelExporterOtlpProtocol)
		switch strings.ToLower(protocol) {
		case "http", "http/protobuf":
			exporter, err = otlptracehttp.New(ctx)
		case "grpc", "": // Default to gRPC per standard
			exporter, err = otlptracegrpc.New(ctx)
		// Uses gRPC exporter (port 4317) following OpenTelemetry standards
		// This is compatible with standard OTEL Collector configurations
		default:
			logger.With(ctx, logger.FieldProtocol, protocol).Warn("Unrecognized OTEL_EXPORTER_OTLP_PROTOCOL, using default grpc")
			exporter, err = otlptracegrpc.New(ctx)
		}
		if err != nil {
			logger.With(ctx, logger.FieldProtocol, protocol).WithError(err).Error("Failed to create OTLP exporter")
			return nil, fmt.Errorf("failed to create OTLP exporter (protocol=%s): %w", protocol, err)
		}
	} else {
		// Create stdout exporter
		exporter, err = stdouttrace.New(
			stdouttrace.WithPrettyPrint(), // Formatted output
		)
		if err != nil {
			logger.WithError(ctx, err).Error("Failed to create OpenTelemetry stdout exporter")
			return nil, fmt.Errorf("failed to create OpenTelemetry stdout exporter: %w", err)
		}
	}

	// Create resource (service information)
	res, err := resource.New(ctx,
		resource.WithFromEnv(), // parse OTEL_RESOURCE_ATTRIBUTES
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(serviceVersion),
		),
	)
	if err != nil {
		if shutdownErr := exporter.Shutdown(ctx); shutdownErr != nil {
			logger.WithError(ctx, shutdownErr).Warn("Failed to shutdown exporter")
		}
		logger.With(ctx,
			logger.FieldServiceName, serviceName,
			logger.FieldServiceVersion, serviceVersion,
		).WithError(err).Error("Failed to create OpenTelemetry resource")
		return nil, fmt.Errorf("failed to create OTel resource: %w", err)
	}

	var sampler trace.Sampler
	samplerType := strings.ToLower(os.Getenv(envOtelTracesSampler))

	switch samplerType {
	case samplerAlwaysOn:
		sampler = trace.AlwaysSample()
	case samplerAlwaysOff:
		sampler = trace.NeverSample()
	case samplerTraceIDRatio:
		sampler = trace.TraceIDRatioBased(parseSamplingRate(ctx))
	case parentBasedTraceIDRatio, "":
		// Default per tracing standard
		sampler = trace.ParentBased(trace.TraceIDRatioBased(parseSamplingRate(ctx)))
	case parentBasedAlwaysOn:
		sampler = trace.ParentBased(trace.AlwaysSample())
	case parentBasedAlwaysOff:
		sampler = trace.ParentBased(trace.NeverSample())
	default:
		logger.With(ctx, logger.FieldSampler, samplerType).Warn("Unrecognized sampler, using default")
		sampler = trace.ParentBased(trace.TraceIDRatioBased(parseSamplingRate(ctx)))
	}

	// Create trace provider
	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(res),
		trace.WithSampler(sampler),
	)

	// Set global trace provider
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp, nil
}

// Shutdown gracefully shuts down the trace provider
func Shutdown(ctx context.Context, tp *trace.TracerProvider) error {
	if tp == nil {
		return nil
	}
	return tp.Shutdown(ctx)
}

// parseSamplingRate parses sampling rate from OTEL_TRACES_SAMPLER_ARG environment variable
func parseSamplingRate(ctx context.Context) float64 {
	rate := defaultSamplingRate
	if arg := os.Getenv(envOtelTracesSamplerArg); arg != "" {
		if parsedRate, err := strconv.ParseFloat(arg, 64); err == nil && parsedRate >= 0.0 && parsedRate <= 1.0 {
			rate = parsedRate
		} else {
			logger.With(ctx, envOtelTracesSamplerArg, arg, "default", rate).
				Warn("Invalid OTEL_TRACES_SAMPLER_ARG value, using default")
		}
	}
	return rate
}
