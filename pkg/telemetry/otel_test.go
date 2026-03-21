package telemetry

import (
	"context"
	"os"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
)

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

	// Cleanup
	defer func() {
		if err := Shutdown(ctx, tp); err != nil {
			t.Errorf("Failed to shutdown trace provider: %v", err)
		}
	}()

	// Verify tracer is available
	tracer := otel.Tracer("test")
	if tracer == nil {
		t.Error("Expected tracer to be available")
	}
}

func TestInitTraceProvider_OTLPExporter(t *testing.T) {
	ctx := context.Background()

	// Set OTLP endpoint (can be fake - we're just testing initialization)
	err := os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://fake-otel-collector:4317")
	if err != nil {
		t.Fatalf("Failed to set OTEL_EXPORTER_OTLP_ENDPOINT: %v", err)
	}
	defer func() {
		unsetErr := os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
		if unsetErr != nil {
			t.Errorf("Failed to unset OTEL_EXPORTER_OTLP_ENDPOINT: %v", unsetErr)
		}
	}()

	// Test that trace provider initializes correctly with OTLP exporter
	tp, err := InitTraceProvider(ctx, "test-service", "v1.0.0")
	if err != nil {
		t.Fatalf("Failed to initialize trace provider with OTLP: %v", err)
	}
	if tp == nil {
		t.Fatal("Expected trace provider, got nil")
	}

	// Verify tracer is available
	tracer := otel.Tracer("test")
	if tracer == nil {
		t.Error("Expected tracer to be available")
	}

	// Test shutdown
	err = Shutdown(ctx, tp)
	if err != nil {
		t.Errorf("Failed to shutdown trace provider: %v", err)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			if tt.samplerType != "" {
				err := os.Setenv("OTEL_TRACES_SAMPLER", tt.samplerType)
				if err != nil {
					t.Fatalf("Failed to set OTEL_TRACES_SAMPLER: %v", err)
				}
				defer func() {
					err := os.Unsetenv("OTEL_TRACES_SAMPLER")
					if err != nil {
						t.Errorf("Failed to unset OTEL_TRACES_SAMPLER: %v", err)
					}
				}()
			}
			if tt.samplerArg != "" {
				err := os.Setenv("OTEL_TRACES_SAMPLER_ARG", tt.samplerArg)
				if err != nil {
					t.Fatalf("Failed to set OTEL_TRACES_SAMPLER_ARG: %v", err)
				}
				defer func() {
					err := os.Unsetenv("OTEL_TRACES_SAMPLER_ARG")
					if err != nil {
						t.Errorf("Failed to unset OTEL_TRACES_SAMPLER_ARG: %v", err)
					}
				}()
			}

			tp, err := InitTraceProvider(ctx, "test-service", "v1.0.0")
			if err != nil {
				t.Fatalf("Failed to initialize trace provider: %v", err)
			}
			defer func(ctx context.Context, tp *trace.TracerProvider) {
				err := Shutdown(ctx, tp)
				if err != nil {
					t.Errorf("Failed to shutdown trace provider")
				}
			}(ctx, tp)

			// Test sampling behavior by checking if spans are created
			tracer := otel.Tracer("test")
			_, span := tracer.Start(ctx, "test-span")

			if tt.expectedSample {
				if !span.SpanContext().IsValid() {
					t.Error("Expected valid span context for sampling=true")
				}
			} else {
				// Add missing validation for expectedSample=false
				if span.SpanContext().IsValid() && span.SpanContext().TraceFlags().IsSampled() {
					t.Error("Expected span to NOT be sampled for sampling=false")
				}
			}
			span.End()
		})
	}
}

