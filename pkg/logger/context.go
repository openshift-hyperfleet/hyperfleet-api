package logger

import (
	"context"
)

// Context keys for logging fields
type contextKey string

const (
	// TraceIDKey is the context key for distributed trace ID (OpenTelemetry)
	TraceIDKey contextKey = "trace_id"
	// SpanIDKey is the context key for current span ID (OpenTelemetry)
	SpanIDKey contextKey = "span_id"
	// RequestIDKey is the context key for HTTP request ID
	RequestIDKey contextKey = "request_id"
	// AccountIDKey is the context key for account ID
	AccountIDKey contextKey = "accountID"
	// TransactionIDKey is the context key for transaction ID
	TransactionIDKey contextKey = "txid"
)

// WithTraceID adds a trace ID to the context
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}

// GetTraceID retrieves the trace ID from context
func GetTraceID(ctx context.Context) string {
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok {
		return traceID
	}
	return ""
}

// WithSpanID adds a span ID to the context
func WithSpanID(ctx context.Context, spanID string) context.Context {
	return context.WithValue(ctx, SpanIDKey, spanID)
}

// GetSpanID retrieves the span ID from context
func GetSpanID(ctx context.Context) string {
	if spanID, ok := ctx.Value(SpanIDKey).(string); ok {
		return spanID
	}
	return ""
}

// WithRequestID adds a request ID to the context
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// GetRequestID retrieves the request ID from context
func GetRequestID(ctx context.Context) string {
	if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
		return requestID
	}
	return ""
}

// NOTE: WithOperationID and GetOperationID are defined in operationid_middleware.go
// The operation ID uses OpIDKey from that file for context storage

// WithAccountID adds an account ID to the context
func WithAccountID(ctx context.Context, accountID string) context.Context {
	return context.WithValue(ctx, AccountIDKey, accountID)
}

// GetAccountID retrieves the account ID from context
func GetAccountID(ctx context.Context) string {
	if accountID := ctx.Value(AccountIDKey); accountID != nil {
		if s, ok := accountID.(string); ok {
			return s
		}
	}
	return ""
}

// WithTransactionID adds a transaction ID to the context
func WithTransactionID(ctx context.Context, txid int64) context.Context {
	return context.WithValue(ctx, TransactionIDKey, txid)
}

// GetTransactionID retrieves the transaction ID from context
func GetTransactionID(ctx context.Context) int64 {
	if txid, ok := ctx.Value(TransactionIDKey).(int64); ok {
		return txid
	}
	return 0
}
