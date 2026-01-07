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
	OpIDKey             contextKey = "operation_id"
	AccountIDCtxKey     contextKey = "account_id"
	TransactionIDCtxKey contextKey = "transaction_id"
	TraceIDCtxKey       contextKey = "trace_id"
	SpanIDCtxKey        contextKey = "span_id"
	ClusterIDCtxKey     contextKey = "cluster_id"
	ResourceTypeCtxKey  contextKey = "resource_type"
	ResourceIDCtxKey    contextKey = "resource_id"
)

// HTTP header names
const (
	OpIDHeader = "X-Operation-ID"
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

// WithAccountID adds account ID to context
func WithAccountID(ctx context.Context, accountID string) context.Context {
	return context.WithValue(ctx, AccountIDCtxKey, accountID)
}

// GetAccountID retrieves account ID from context
func GetAccountID(ctx context.Context) (string, bool) {
	accountID, ok := ctx.Value(AccountIDCtxKey).(string)
	return accountID, ok
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
