package logger

import (
	"context"

	"github.com/segmentio/ksuid"
)

// contextKey is an unexported type for keys defined in this package.
// This prevents collisions with keys defined in other packages.
type contextKey string

// Context keys for storing correlation fields
const (
	ReqIDKey            contextKey = "request_id"
	TransactionIDCtxKey contextKey = "transaction_id" // Database transaction ID
	TraceIDCtxKey       contextKey = "trace_id"
	SpanIDCtxKey        contextKey = "span_id"
	ClusterIDCtxKey     contextKey = "cluster_id"
	ResourceTypeCtxKey  contextKey = "resource_type"
	ResourceIDCtxKey    contextKey = "resource_id"
)

// HTTP header names
const (
	ReqIDHeader = "X-Request-ID"
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

// WithRequestID adds request ID to context
// If request ID already exists in context, it returns the context unchanged
// Otherwise, it generates a new KSUID and adds it to the context
func WithRequestID(ctx context.Context) context.Context {
	if ctx.Value(ReqIDKey) != nil {
		return ctx
	}
	reqID := ksuid.New().String()
	return context.WithValue(ctx, ReqIDKey, reqID)
}

// GetRequestID retrieves request ID from context
func GetRequestID(ctx context.Context) (string, bool) {
	reqID, ok := ctx.Value(ReqIDKey).(string)
	return reqID, ok
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

// ContextField defines metadata for a string-type context log field
type ContextField struct {
	Key    contextKey
	Name   string
	Getter func(context.Context) (string, bool)
}

// ContextFieldsRegistry defines all string-type context fields for logging
// This is the single source of truth for string field management
// Fields are ordered as per HyperFleet Logging Specification (docs/logging.md:384)
// Note: transaction_id (int64) is handled separately in logger.go
var ContextFieldsRegistry = []ContextField{
	{ReqIDKey, "request_id", GetRequestID},
	{TraceIDCtxKey, "trace_id", GetTraceID},
	{SpanIDCtxKey, "span_id", GetSpanID},
	{ClusterIDCtxKey, "cluster_id", GetClusterID},
	{ResourceTypeCtxKey, "resource_type", GetResourceType},
	{ResourceIDCtxKey, "resource_id", GetResourceID},
}

// WithTransactionID adds transaction ID to context
func WithTransactionID(ctx context.Context, transactionID int64) context.Context {
	return context.WithValue(ctx, TransactionIDCtxKey, transactionID)
}

// GetTransactionID retrieves transaction ID from context
func GetTransactionID(ctx context.Context) (int64, bool) {
	transactionID, ok := ctx.Value(TransactionIDCtxKey).(int64)
	return transactionID, ok
}
