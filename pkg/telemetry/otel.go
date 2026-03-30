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
//   - OTEL_RESOURCE_ATTRIBUTES: additional resource attributes (k=v,k2=v2 format)
func InitTraceProvider(ctx context.Context, serviceName, serviceVersion string) (*trace.TracerProvider, error) {
	// Create exporter (OTLP or stdout)
	exporter, err := createExporter(ctx)
	if err != nil {
		return nil, err
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

	// Select sampler
	sampler := selectSampler(ctx)

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

// createExporter creates the appropriate span exporter based on environment variables
func createExporter(ctx context.Context) (trace.SpanExporter, error) {
	otlpEndpoint := os.Getenv(envOtelExporterOtlpEndpoint)
	if otlpEndpoint == "" {
		// Create stdout exporter when no OTLP endpoint is configured
		exporter, err := stdouttrace.New(
			stdouttrace.WithPrettyPrint(), // Formatted output
		)
		if err != nil {
			logger.WithError(ctx, err).Error("Failed to create OpenTelemetry stdout exporter")
			return nil, fmt.Errorf("failed to create OpenTelemetry stdout exporter: %w", err)
		}
		return exporter, nil
	}

	// Create OTLP exporter
	protocol := os.Getenv(envOtelExporterOtlpProtocol)
	switch strings.ToLower(protocol) {
	case "http/protobuf":
		// Note: http/json not yet supported - use http/protobuf
		exporter, err := otlptracehttp.New(ctx)
		if err != nil {
			logger.With(ctx, logger.FieldProtocol, protocol).WithError(err).Error("Failed to create OTLP exporter")
			return nil, fmt.Errorf("failed to create OTLP exporter (protocol=%s): %w", protocol, err)
		}
		return exporter, nil
	case "grpc", "": // Default to gRPC per standard
		exporter, err := otlptracegrpc.New(ctx)
		if err != nil {
			logger.With(ctx, logger.FieldProtocol, protocol).WithError(err).Error("Failed to create OTLP exporter")
			return nil, fmt.Errorf("failed to create OTLP exporter (protocol=%s): %w", protocol, err)
		}
		return exporter, nil
	default:
		// Spec-compliant values: grpc, http/protobuf
		logger.With(ctx, logger.FieldProtocol, protocol).Warn("Unrecognized OTEL_EXPORTER_OTLP_PROTOCOL, using default grpc")
		exporter, err := otlptracegrpc.New(ctx)
		if err != nil {
			logger.With(ctx, logger.FieldProtocol, protocol).WithError(err).Error("Failed to create OTLP exporter")
			return nil, fmt.Errorf("failed to create OTLP exporter (protocol=%s): %w", protocol, err)
		}
		return exporter, nil
	}
}

// selectSampler returns the appropriate sampler based on environment variables
func selectSampler(ctx context.Context) trace.Sampler {
	samplerType := strings.ToLower(os.Getenv(envOtelTracesSampler))

	switch samplerType {
	case samplerAlwaysOn:
		return trace.AlwaysSample()
	case samplerAlwaysOff:
		return trace.NeverSample()
	case samplerTraceIDRatio:
		return trace.TraceIDRatioBased(parseSamplingRate(ctx))
	case parentBasedTraceIDRatio, "":
		// Default per tracing standard
		return trace.ParentBased(trace.TraceIDRatioBased(parseSamplingRate(ctx)))
	case parentBasedAlwaysOn:
		return trace.ParentBased(trace.AlwaysSample())
	case parentBasedAlwaysOff:
		return trace.ParentBased(trace.NeverSample())
	default:
		logger.With(ctx, logger.FieldSampler, samplerType).Warn("Unrecognized sampler, using default")
		return trace.ParentBased(trace.TraceIDRatioBased(parseSamplingRate(ctx)))
	}
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
			logger.With(ctx, logger.FieldSamplingRate, rate, "raw_value", arg).
				Warn("Invalid OTEL_TRACES_SAMPLER_ARG value, using default")
		}
	}
	return rate
}
