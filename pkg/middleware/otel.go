package middleware

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// OTelMiddleware wraps HTTP handlers with OpenTelemetry instrumentation
// Automatically:
//   - Creates spans for HTTP requests
//   - Extracts trace context from traceparent header (W3C Trace Context)
//   - Injects trace context into outbound requests
//   - Adds trace_id and span_id to logger context
func OTelMiddleware(handler http.Handler) http.Handler {
	// Use otelhttp to automatically create spans
	otelHandler := otelhttp.NewHandler(handler, "hyperfleet-api",
		otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
			// Custom span name: "METHOD /path"
			return r.Method + " " + r.URL.Path
		}),
	)

	// Extract trace_id and span_id and add to context
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		span := trace.SpanFromContext(ctx)

		// If span context is valid, extract trace_id and span_id
		if span.SpanContext().IsValid() {
			traceID := span.SpanContext().TraceID().String()
			spanID := span.SpanContext().SpanID().String()

			// Add to logger context
			ctx = logger.WithTraceID(ctx, traceID)
			ctx = logger.WithSpanID(ctx, spanID)
		}

		// Serve with updated context
		otelHandler.ServeHTTP(w, r.WithContext(ctx))
	})
}
