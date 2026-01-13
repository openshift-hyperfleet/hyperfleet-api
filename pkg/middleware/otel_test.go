package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// setupTestTracer creates an in-memory tracer for testing
func setupTestTracer() (*trace.TracerProvider, *tracetest.InMemoryExporter) {
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(
		trace.WithSyncer(exporter),
	)
	otel.SetTracerProvider(tp)
	return tp, exporter
}

// TestOTelMiddleware_SpanNameUsesRouteTemplate tests that span names use route templates
// to prevent cardinality explosion (H3 security fix)
func TestOTelMiddleware_SpanNameUsesRouteTemplate(t *testing.T) {
	// Initialize logger for testing
	logger.InitGlobalLogger(&logger.LogConfig{
		Level:     0, // Debug
		Format:    logger.FormatJSON,
		Output:    httptest.NewRecorder(),
		Component: "test",
		Version:   "test",
		Hostname:  "test",
	})

	tp, exporter := setupTestTracer()
	defer func() {
		_ = tp.Shutdown(context.Background())
	}()

	tests := []struct {
		name             string
		routePattern     string
		requestPath      string
		expectedSpanName string
	}{
		{
			name:             "cluster detail with UUID uses template",
			routePattern:     "/api/hyperfleet/v1/clusters/{id}",
			requestPath:      "/api/hyperfleet/v1/clusters/550e8400-e29b-41d4-a716-446655440000",
			expectedSpanName: "GET /api/hyperfleet/v1/clusters/{id}",
		},
		{
			name:             "cluster detail with different UUID uses same template",
			routePattern:     "/api/hyperfleet/v1/clusters/{id}",
			requestPath:      "/api/hyperfleet/v1/clusters/f47ac10b-58cc-4372-a567-0e02b2c3d479",
			expectedSpanName: "GET /api/hyperfleet/v1/clusters/{id}",
		},
		{
			name:             "cluster status with adapter name uses template",
			routePattern:     "/api/hyperfleet/v1/clusters/{id}/status/{adapter}",
			requestPath:      "/api/hyperfleet/v1/clusters/cluster-123/status/monitoring-adapter",
			expectedSpanName: "GET /api/hyperfleet/v1/clusters/{id}/status/{adapter}",
		},
		{
			name:             "list endpoint without parameters",
			routePattern:     "/api/hyperfleet/v1/clusters",
			requestPath:      "/api/hyperfleet/v1/clusters",
			expectedSpanName: "GET /api/hyperfleet/v1/clusters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear previous spans
			exporter.Reset()

			// Create test handler
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// Create mux router with route
			router := mux.NewRouter()
			router.Handle(tt.routePattern, OTelMiddleware(testHandler)).Methods("GET")

			// Create request
			req := httptest.NewRequest("GET", tt.requestPath, nil)
			w := httptest.NewRecorder()

			// Execute request
			router.ServeHTTP(w, req)

			// Force flush spans
			if err := tp.ForceFlush(context.Background()); err != nil {
				t.Errorf("failed to flush spans: %v", err)
			}

			// Verify span was created
			spans := exporter.GetSpans()
			if len(spans) == 0 {
				t.Fatal("expected at least one span to be created")
			}

			// Find the HTTP server span (not the internal otelhttp spans)
			var httpSpan *tracetest.SpanStub
			for i := range spans {
				if spans[i].Name == tt.expectedSpanName {
					httpSpan = &spans[i]
					break
				}
			}

			if httpSpan == nil {
				t.Fatalf("expected span with name %q, got spans: %v",
					tt.expectedSpanName, getSpanNames(spans))
			}

			// Verify span name uses route template (low cardinality)
			if httpSpan.Name != tt.expectedSpanName {
				t.Errorf("expected span name %q, got %q", tt.expectedSpanName, httpSpan.Name)
			}
		})
	}
}

// TestOTelMiddleware_TraceContextExtraction tests W3C trace context extraction
func TestOTelMiddleware_TraceContextExtraction(t *testing.T) {
	logger.InitGlobalLogger(&logger.LogConfig{
		Level:     0,
		Format:    logger.FormatJSON,
		Output:    httptest.NewRecorder(),
		Component: "test",
		Version:   "test",
		Hostname:  "test",
	})

	tp, exporter := setupTestTracer()
	defer func() {
		_ = tp.Shutdown(context.Background())
	}()

	tests := []struct {
		name             string
		traceparent      string
		expectTraceID    bool
		expectSpanID     bool
		expectValidTrace bool
	}{
		{
			name:             "valid W3C traceparent header",
			traceparent:      "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
			expectTraceID:    true,
			expectSpanID:     true,
			expectValidTrace: true,
		},
		{
			name:             "no traceparent header creates new trace",
			traceparent:      "",
			expectTraceID:    true,
			expectSpanID:     true,
			expectValidTrace: true,
		},
		{
			name:             "invalid traceparent header creates new trace",
			traceparent:      "invalid-trace-header",
			expectTraceID:    true,
			expectSpanID:     true,
			expectValidTrace: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter.Reset()

			var capturedTraceID, capturedSpanID string
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := r.Context()
				// Extract trace IDs from logger context
				if traceID, ok := logger.GetTraceID(ctx); ok {
					capturedTraceID = traceID
				}
				if spanID, ok := logger.GetSpanID(ctx); ok {
					capturedSpanID = spanID
				}
				w.WriteHeader(http.StatusOK)
			})

			router := mux.NewRouter()
			router.Handle("/test", OTelMiddleware(testHandler)).Methods("GET")

			req := httptest.NewRequest("GET", "/test", nil)
			if tt.traceparent != "" {
				req.Header.Set("traceparent", tt.traceparent)
			}
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)
			if err := tp.ForceFlush(context.Background()); err != nil {
				t.Errorf("failed to flush spans: %v", err)
			}

			// Verify trace context was extracted and added to logger context
			if tt.expectTraceID && capturedTraceID == "" {
				t.Error("expected trace_id in logger context, got none")
			}
			if tt.expectSpanID && capturedSpanID == "" {
				t.Error("expected span_id in logger context, got none")
			}

			// Verify span was created
			spans := exporter.GetSpans()
			if tt.expectValidTrace && len(spans) == 0 {
				t.Error("expected spans to be created")
			}
		})
	}
}

// TestOTelMiddleware_NoTraceContext tests middleware behavior without trace context
func TestOTelMiddleware_NoTraceContext(t *testing.T) {
	logger.InitGlobalLogger(&logger.LogConfig{
		Level:     0,
		Format:    logger.FormatJSON,
		Output:    httptest.NewRecorder(),
		Component: "test",
		Version:   "test",
		Hostname:  "test",
	})

	tp, exporter := setupTestTracer()
	defer func() {
		_ = tp.Shutdown(context.Background())
	}()

	exporter.Reset()

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	router := mux.NewRouter()
	router.Handle("/test", OTelMiddleware(testHandler)).Methods("GET")

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	// Should not panic
	router.ServeHTTP(w, req)
	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Errorf("failed to flush spans: %v", err)
	}

	// Verify response is successful
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Verify new trace was created
	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Error("expected new trace to be created when no trace context provided")
	}
}

// TestOTelMiddleware_CardinalityPrevention demonstrates cardinality fix
func TestOTelMiddleware_CardinalityPrevention(t *testing.T) {
	logger.InitGlobalLogger(&logger.LogConfig{
		Level:     0,
		Format:    logger.FormatJSON,
		Output:    httptest.NewRecorder(),
		Component: "test",
		Version:   "test",
		Hostname:  "test",
	})

	tp, exporter := setupTestTracer()
	defer func() {
		_ = tp.Shutdown(context.Background())
	}()

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	router := mux.NewRouter()
	router.Handle("/api/clusters/{id}", OTelMiddleware(testHandler)).Methods("GET")

	// Simulate 100 requests with different UUIDs
	uniqueSpanNames := make(map[string]bool)

	for i := 0; i < 100; i++ {
		exporter.Reset()

		// Each request has a different UUID
		path := "/api/clusters/" + generateTestUUID(i)
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		if err := tp.ForceFlush(context.Background()); err != nil {
			t.Errorf("failed to flush spans: %v", err)
		}

		spans := exporter.GetSpans()
		for _, span := range spans {
			uniqueSpanNames[span.Name] = true
		}
	}

	// Verify cardinality is LOW (should be 1-2 span names, not 100)
	// We expect only "GET /api/clusters/{id}" regardless of how many unique UUIDs
	if len(uniqueSpanNames) > 5 {
		t.Errorf("cardinality explosion detected: expected ≤5 unique span names, got %d: %v",
			len(uniqueSpanNames), keys(uniqueSpanNames))
	}

	// Verify the expected span name exists
	expectedSpanName := "GET /api/clusters/{id}"
	if !uniqueSpanNames[expectedSpanName] {
		t.Errorf("expected span name %q to exist, got: %v", expectedSpanName, keys(uniqueSpanNames))
	}
}

// Helper functions

func generateTestUUID(i int) string {
	// Generate unique UUID-like strings for testing
	return "550e8400-e29b-41d4-a716-" + fmt.Sprintf("%012d", i)
}

func getSpanNames(spans []tracetest.SpanStub) []string {
	names := make([]string, len(spans))
	for i, span := range spans {
		names[i] = span.Name
	}
	return names
}

func keys(m map[string]bool) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}
