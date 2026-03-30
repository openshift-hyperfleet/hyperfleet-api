package telemetry

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

// cleanupTraceProvider shuts down the trace provider and resets global OpenTelemetry state.
// This prevents test pollution from global state modifications.
func cleanupTraceProvider(ctx context.Context, t *testing.T, tp *trace.TracerProvider) {
	t.Helper()
	if err := Shutdown(ctx, tp); err != nil {
		t.Errorf("Failed to shutdown trace provider: %v", err)
	}
	// Reset global state to prevent test interference
	otel.SetTracerProvider(nooptrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())
}

func TestInitTraceProvider_StdoutExporter(t *testing.T) {
	ctx := context.Background()

	// Test stdout exporter (default)
	tp, err := InitTraceProvider(ctx, "test-service", "v1.0.0")
	if err != nil {
		t.Fatalf("Failed to initialize trace provider: %v", err)
	}
	if tp == nil {
		t.Fatal("Expected trace provider, got nil")
	}
	defer cleanupTraceProvider(ctx, t, tp)

	// Verify tracer is available
	tracer := otel.Tracer("test")
	if tracer == nil {
		t.Error("Expected tracer to be available")
	}
}

func TestInitTraceProvider_OTLPExporter(t *testing.T) {
	ctx := context.Background()

	// Set OTLP endpoint (can be fake - we're just testing initialization)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://fake-otel-collector:4317")

	// Test that trace provider initializes correctly with OTLP exporter
	tp, err := InitTraceProvider(ctx, "test-service", "v1.0.0")
	if err != nil {
		t.Fatalf("Failed to initialize trace provider with OTLP: %v", err)
	}
	if tp == nil {
		t.Fatal("Expected trace provider, got nil")
	}
	defer cleanupTraceProvider(ctx, t, tp)

	// Verify tracer is available
	tracer := otel.Tracer("test")
	if tracer == nil {
		t.Error("Expected tracer to be available")
	}
}

func TestInitTraceProvider_OTLPHttpExporter(t *testing.T) {
	ctx := context.Background()

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://fake-otel-collector:4318")
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")

	tp, err := InitTraceProvider(ctx, "test-service", "v1.0.0")
	if err != nil {
		t.Fatalf("Failed to initialize trace provider with HTTP OTLP: %v", err)
	}
	if tp == nil {
		t.Fatal("Expected trace provider, got nil")
	}
	defer cleanupTraceProvider(ctx, t, tp)

	// Verify tracer is available
	tracer := otel.Tracer("test")
	if tracer == nil {
		t.Error("Expected tracer to be available")
	}
}

func TestInitTraceProvider_InvalidProtocol(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		protocol string
	}{
		{
			name:     "bare_http_is_ambiguous",
			protocol: "http",
		},
		{
			name:     "http_json_not_yet_supported",
			protocol: "http/json",
		},
		{
			name:     "invalid_protocol",
			protocol: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://fake-otel-collector:4317")
			t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", tt.protocol)

			// Should fall back to gRPC with a warning (not fail)
			tp, err := InitTraceProvider(ctx, "test-service", "v1.0.0")
			if err != nil {
				t.Fatalf("Failed to initialize trace provider: %v", err)
			}
			if tp == nil {
				t.Fatal("Expected trace provider, got nil")
			}
			defer cleanupTraceProvider(ctx, t, tp)

			// Verify tracer is available (using default gRPC exporter)
			tracer := otel.Tracer("test")
			if tracer == nil {
				t.Error("Expected tracer to be available")
			}
		})
	}
}

func TestInitTraceProvider_SamplerEnvironmentVariables(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		samplerType    string
		samplerArg     string
		expectedSample bool
	}{
		{
			name:           "always_on",
			samplerType:    "always_on",
			expectedSample: true,
		},
		{
			name:           "always_off",
			samplerType:    "always_off",
			expectedSample: false,
		},
		{
			name:           "traceidratio_high",
			samplerType:    "traceidratio",
			samplerArg:     "1.0",
			expectedSample: true,
		},
		{
			name:           "traceidratio_zero",
			samplerType:    "traceidratio",
			samplerArg:     "0.0",
			expectedSample: false,
		},
		{
			name:           "parentbased_traceidratio_default",
			samplerType:    "parentbased_traceidratio",
			samplerArg:     "1.0",
			expectedSample: true,
		},
		{
			name:           "parentbased_always_on",
			samplerType:    "parentbased_always_on",
			expectedSample: true,
		},
		{
			name:           "parentbased_always_off",
			samplerType:    "parentbased_always_off",
			expectedSample: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			if tt.samplerType != "" {
				t.Setenv("OTEL_TRACES_SAMPLER", tt.samplerType)
			}
			if tt.samplerArg != "" {
				t.Setenv("OTEL_TRACES_SAMPLER_ARG", tt.samplerArg)
			}

			tp, err := InitTraceProvider(ctx, "test-service", "v1.0.0")
			if err != nil {
				t.Fatalf("Failed to initialize trace provider: %v", err)
			}
			defer cleanupTraceProvider(ctx, t, tp)

			// Test sampling behavior by checking if spans are created
			tracer := otel.Tracer("test")
			_, span := tracer.Start(ctx, "test-span")

			if tt.expectedSample {
				if !span.SpanContext().IsValid() || !span.SpanContext().TraceFlags().IsSampled() {
					t.Error("Expected valid and sampled span context for sampling=true")
				}
			} else {
				// Verify span is NOT sampled for expectedSample=false
				if span.SpanContext().IsValid() && span.SpanContext().TraceFlags().IsSampled() {
					t.Error("Expected span to NOT be sampled for sampling=false")
				}
			}
			span.End()
		})
	}
}

func TestInitTraceProvider_InvalidSamplerArg(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		samplerArg     string
		expectedSample bool // Should fall back to default (1.0 = always sample)
	}{
		{
			name:           "negative_value",
			samplerArg:     "-1.0",
			expectedSample: true, // Falls back to default 1.0
		},
		{
			name:           "above_one",
			samplerArg:     "2.0",
			expectedSample: true, // Falls back to default 1.0
		},
		{
			name:           "non_numeric",
			samplerArg:     "invalid",
			expectedSample: true, // Falls back to default 1.0
		},
		{
			name:           "empty_string",
			samplerArg:     "",
			expectedSample: true, // Falls back to default 1.0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use parentbased_traceidratio to test the sampling rate parsing
			t.Setenv("OTEL_TRACES_SAMPLER", "parentbased_traceidratio")

			if tt.samplerArg != "" {
				t.Setenv("OTEL_TRACES_SAMPLER_ARG", tt.samplerArg)
			}

			tp, err := InitTraceProvider(ctx, "test-service", "v1.0.0")
			if err != nil {
				t.Fatalf("Failed to initialize trace provider: %v", err)
			}
			defer cleanupTraceProvider(ctx, t, tp)

			// Test that invalid values fall back to default (1.0 = always sample)
			tracer := otel.Tracer("test")
			_, span := tracer.Start(ctx, "test-span")

			if tt.expectedSample {
				if !span.SpanContext().IsValid() || !span.SpanContext().TraceFlags().IsSampled() {
					t.Errorf("Expected span to be sampled (fallback to default 1.0) for invalid arg %q", tt.samplerArg)
				}
			}
			span.End()
		})
	}
}

func TestCreateExporter_Stdout(t *testing.T) {
	ctx := context.Background()

	// Test stdout exporter when no OTLP endpoint is set
	exporter, err := createExporter(ctx)
	if err != nil {
		t.Fatalf("Failed to create stdout exporter: %v", err)
	}
	if exporter == nil {
		t.Fatal("Expected exporter, got nil")
	}
	defer func() {
		if err := exporter.Shutdown(ctx); err != nil {
			t.Errorf("Failed to shutdown exporter: %v", err)
		}
	}()
}

func TestCreateExporter_OTLP_gRPC(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		protocol string
	}{
		{
			name:     "explicit_grpc",
			protocol: "grpc",
		},
		{
			name:     "empty_protocol_defaults_to_grpc",
			protocol: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://fake-otel-collector:4317")
			if tt.protocol != "" {
				t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", tt.protocol)
			}

			exporter, err := createExporter(ctx)
			if err != nil {
				t.Fatalf("Failed to create gRPC exporter: %v", err)
			}
			if exporter == nil {
				t.Fatal("Expected exporter, got nil")
			}
			defer func() {
				if err := exporter.Shutdown(ctx); err != nil {
					t.Errorf("Failed to shutdown exporter: %v", err)
				}
			}()
		})
	}
}

func TestCreateExporter_OTLP_HTTP(t *testing.T) {
	ctx := context.Background()

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://fake-otel-collector:4318")
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")

	exporter, err := createExporter(ctx)
	if err != nil {
		t.Fatalf("Failed to create HTTP exporter: %v", err)
	}
	if exporter == nil {
		t.Fatal("Expected exporter, got nil")
	}
	defer func() {
		if err := exporter.Shutdown(ctx); err != nil {
			t.Errorf("Failed to shutdown exporter: %v", err)
		}
	}()
}

func TestCreateExporter_InvalidProtocol(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		protocol string
	}{
		{
			name:     "http_without_protobuf",
			protocol: "http",
		},
		{
			name:     "invalid_protocol",
			protocol: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://fake-otel-collector:4317")
			t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", tt.protocol)

			// Should fall back to gRPC with a warning
			exporter, err := createExporter(ctx)
			if err != nil {
				t.Fatalf("Failed to create exporter (should fall back to gRPC): %v", err)
			}
			if exporter == nil {
				t.Fatal("Expected exporter, got nil")
			}
			defer func() {
				if err := exporter.Shutdown(ctx); err != nil {
					t.Errorf("Failed to shutdown exporter: %v", err)
				}
			}()
		})
	}
}

func TestSelectSampler(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		samplerType string
		samplerArg  string
	}{
		{
			name:        "always_on",
			samplerType: "always_on",
		},
		{
			name:        "always_off",
			samplerType: "always_off",
		},
		{
			name:        "traceidratio",
			samplerType: "traceidratio",
			samplerArg:  "0.5",
		},
		{
			name:        "parentbased_traceidratio",
			samplerType: "parentbased_traceidratio",
			samplerArg:  "1.0",
		},
		{
			name:        "parentbased_always_on",
			samplerType: "parentbased_always_on",
		},
		{
			name:        "parentbased_always_off",
			samplerType: "parentbased_always_off",
		},
		{
			name:        "default_when_empty",
			samplerType: "",
		},
		{
			name:        "invalid_falls_back_to_default",
			samplerType: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.samplerType != "" {
				t.Setenv("OTEL_TRACES_SAMPLER", tt.samplerType)
			}
			if tt.samplerArg != "" {
				t.Setenv("OTEL_TRACES_SAMPLER_ARG", tt.samplerArg)
			}

			sampler := selectSampler(ctx)
			if sampler == nil {
				t.Fatal("Expected sampler, got nil")
			}
		})
	}
}

func TestInitTraceProvider_ParentBasedSampling(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		samplerType    string
		samplerArg     string
		withParent     bool
		expectedSample bool
	}{
		{
			name:           "root_span_with_ratio_high",
			samplerType:    "parentbased_traceidratio",
			samplerArg:     "1.0",
			withParent:     false,
			expectedSample: true, // Root span uses ratio (1.0 = sample)
		},
		{
			name:           "child_span_inherits_parent_sampling",
			samplerType:    "parentbased_traceidratio",
			samplerArg:     "1.0",
			withParent:     true,
			expectedSample: true, // Parent sampled, child follows
		},
		{
			name:           "root_span_with_ratio_zero",
			samplerType:    "parentbased_traceidratio",
			samplerArg:     "0.0",
			withParent:     false,
			expectedSample: false, // Root span uses ratio (0.0 = don't sample)
		},
		{
			name:           "root_span_always_on",
			samplerType:    "parentbased_always_on",
			samplerArg:     "",
			withParent:     false,
			expectedSample: true, // Root span always sampled
		},
		{
			name:           "root_span_always_off",
			samplerType:    "parentbased_always_off",
			samplerArg:     "",
			withParent:     false,
			expectedSample: false, // Root span never sampled
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OTEL_TRACES_SAMPLER", tt.samplerType)

			if tt.samplerArg != "" {
				t.Setenv("OTEL_TRACES_SAMPLER_ARG", tt.samplerArg)
			}

			tp, err := InitTraceProvider(ctx, "test-service", "v1.0.0")
			if err != nil {
				t.Fatalf("Failed to initialize trace provider: %v", err)
			}
			defer cleanupTraceProvider(ctx, t, tp)

			tracer := otel.Tracer("test")

			var testCtx context.Context
			if tt.withParent {
				// Create parent span and use its context
				parentCtx, parentSpan := tracer.Start(ctx, "parent-span")
				testCtx = parentCtx
				defer parentSpan.End()
			} else {
				// Root span (no parent)
				testCtx = ctx
			}

			// Create test span (root or child depending on withParent)
			_, span := tracer.Start(testCtx, "test-span")

			if tt.expectedSample {
				if !span.SpanContext().IsValid() || !span.SpanContext().TraceFlags().IsSampled() {
					t.Errorf("Expected span to be sampled for %s", tt.name)
				}
			} else {
				if span.SpanContext().IsValid() && span.SpanContext().TraceFlags().IsSampled() {
					t.Errorf("Expected span to NOT be sampled for %s", tt.name)
				}
			}
			span.End()
		})
	}
}
