package middleware

import (
	"net/http"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// OTelMiddleware extracts W3C trace context and enriches logger context
//
// Flow:
// 1. otelhttp.NewHandler creates/continues span from W3C headers
// 2. enrichedHandler extracts trace_id/span_id from span context
// 3. Adds trace identifiers to logger context for log correlation
//
// Satisfies HyperFleet Logging Specification:
// "Components must extract trace context from W3C headers"
func OTelMiddleware(handler http.Handler) http.Handler {
	enrichedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		span := trace.SpanFromContext(ctx)

		if span.SpanContext().IsValid() {
			traceID := span.SpanContext().TraceID().String()
			spanID := span.SpanContext().SpanID().String()
			ctx = logger.WithTraceID(ctx, traceID)
			ctx = logger.WithSpanID(ctx, spanID)
		}

		r = r.WithContext(ctx)
		handler.ServeHTTP(w, r)
	})

	otelHandler := otelhttp.NewHandler(enrichedHandler, "hyperfleet-api",
		otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
			// Use route template to prevent cardinality explosion
			// Example: "GET /api/clusters/{id}" instead of "GET /api/clusters/uuid-123"
			if r.Pattern != "" {
				return r.Method + " " + stripMethodPrefix(r.Pattern)
			}
			// Fallback to full path if route template unavailable
			return r.Method + " " + r.URL.Path
		}),
	)

	return otelHandler
}

// stripMethodPrefix removes the leading "METHOD " prefix from a Go 1.22+
// http.Request.Pattern (e.g. "GET /clusters/{id}" -> "/clusters/{id}"),
// leaving method-less patterns (e.g. "/") unchanged.
func stripMethodPrefix(pattern string) string {
	if _, path, ok := strings.Cut(pattern, " "); ok {
		return path
	}
	return pattern
}
