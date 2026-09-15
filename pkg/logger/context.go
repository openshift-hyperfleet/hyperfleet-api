package logger

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	hfl "github.com/openshift-hyperfleet/hyperfleet-logger"
)

// Context keys for API-specific correlation fields. Trace, span, resource type,
// and resource ID keys are owned by the shared logger package.
var ReqIDKey = hfl.NewKey[string](FieldRequestID)

// AdapterKey identifies the adapter whose report is being processed.
var AdapterKey = hfl.NewKey[string](FieldAdapter)

// WithAdapter derives a context carrying adapter identity without losing request correlation.
func WithAdapter(ctx context.Context, adapter string) context.Context {
	return hfl.Set(ctx, AdapterKey, adapter)
}

// HTTP header names
const (
	ReqIDHeader = "X-Request-ID"
)

// WithRequestID adds request ID to context
// If request ID already exists in context, it returns the context unchanged
// Otherwise, it generates a new UUID v7 and adds it to the context
// Returns an error if UUID generation fails (extremely unlikely in practice)
func WithRequestID(ctx context.Context) (context.Context, error) {
	if _, ok := hfl.Get(ctx, ReqIDKey); ok {
		return ctx, nil
	}

	reqID, err := uuid.NewV7()
	if err != nil {
		return ctx, fmt.Errorf("failed to generate request ID: %w", err)
	}

	return hfl.Set(ctx, ReqIDKey, reqID.String()), nil
}

// GetRequestID retrieves request ID from context
func GetRequestID(ctx context.Context) (string, bool) {
	return hfl.Get(ctx, ReqIDKey)
}

// ContextFields returns API-specific fields for the shared handler. Standard
// correlation fields (trace_id, span_id, resource_type, resource_id) are
// registered by hyperfleet-logger itself.
func ContextFields() []hfl.ContextField {
	return []hfl.ContextField{
		hfl.StringField(ReqIDKey),
		hfl.StringField(AdapterKey),
	}
}
