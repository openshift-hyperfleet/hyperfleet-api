package logger

import (
	"context"

	"github.com/segmentio/ksuid"
)

// Context key types for type safety
type OperationIDKey string
type TraceIDKey string
type SpanIDKey string
type ClusterIDKey string
type ResourceTypeKey string
type ResourceIDKey string

// Context keys for storing correlation fields
const (
	OpIDKey          OperationIDKey   = "opID"           // Keep existing value for compatibility
	OpIDHeader       OperationIDKey   = "X-Operation-ID" // HTTP header for operation ID
	TraceIDCtxKey    TraceIDKey       = "trace_id"
	SpanIDCtxKey     SpanIDKey        = "span_id"
	ClusterIDCtxKey  ClusterIDKey     = "cluster_id"
	ResourceTypeCtxKey ResourceTypeKey = "resource_type"
	ResourceIDCtxKey ResourceIDKey    = "resource_id"
)

// WithTraceID adds trace ID to context
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDCtxKey, traceID)
}

// WithSpanID adds span ID to context
func WithSpanID(ctx context.Context, spanID string) context.Context {
	return context.WithValue(ctx, SpanIDCtxKey, spanID)
}

// WithClusterID adds cluster ID to context
func WithClusterID(ctx context.Context, clusterID string) context.Context {
	return context.WithValue(ctx, ClusterIDCtxKey, clusterID)
}

// WithResourceType adds resource type to context
func WithResourceType(ctx context.Context, resourceType string) context.Context {
	return context.WithValue(ctx, ResourceTypeCtxKey, resourceType)
}

// WithResourceID adds resource ID to context
func WithResourceID(ctx context.Context, resourceID string) context.Context {
	return context.WithValue(ctx, ResourceIDCtxKey, resourceID)
}

// WithOpID adds operation ID to context
// If operation ID already exists in context, it returns the context unchanged
// Otherwise, it generates a new KSUID and adds it to the context
func WithOpID(ctx context.Context) context.Context {
	if ctx.Value(OpIDKey) != nil {
		return ctx
	}
	opID := ksuid.New().String()
	return context.WithValue(ctx, OpIDKey, opID)
}

// GetOperationID retrieves operation ID from context
func GetOperationID(ctx context.Context) string {
	if opID, ok := ctx.Value(OpIDKey).(string); ok {
		return opID
	}
	return ""
}

// GetTraceID retrieves trace ID from context
func GetTraceID(ctx context.Context) (string, bool) {
	traceID, ok := ctx.Value(TraceIDCtxKey).(string)
	return traceID, ok
}

// GetSpanID retrieves span ID from context
func GetSpanID(ctx context.Context) (string, bool) {
	spanID, ok := ctx.Value(SpanIDCtxKey).(string)
	return spanID, ok
}

// GetClusterID retrieves cluster ID from context
func GetClusterID(ctx context.Context) (string, bool) {
	clusterID, ok := ctx.Value(ClusterIDCtxKey).(string)
	return clusterID, ok
}

// GetResourceType retrieves resource type from context
func GetResourceType(ctx context.Context) (string, bool) {
	resourceType, ok := ctx.Value(ResourceTypeCtxKey).(string)
	return resourceType, ok
}

// GetResourceID retrieves resource ID from context
func GetResourceID(ctx context.Context) (string, bool) {
	resourceID, ok := ctx.Value(ResourceIDCtxKey).(string)
	return resourceID, ok
}
