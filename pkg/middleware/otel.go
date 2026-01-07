package middleware

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// OTelMiddleware wraps HTTP handlers with OpenTelemetry instrumentation
// Extracts trace_id and span_id from OTel span and adds them to logger context
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
			return r.Method + " " + r.URL.Path
		}),
	)

	return otelHandler
}
