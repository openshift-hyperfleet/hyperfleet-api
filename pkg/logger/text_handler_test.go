package logger

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

// TestHyperFleetTextHandler_BasicFormat tests basic text output format
func TestHyperFleetTextHandler_BasicFormat(t *testing.T) {
	var buf bytes.Buffer
	handler := NewHyperFleetTextHandler(&buf, "hyperfleet-api", "v1.2.3", "test-host", slog.LevelInfo)

	ctx := context.Background()
	logger := slog.New(handler)
	logger.InfoContext(ctx, "Test message", "key", "value")

	output := buf.String()

	// Check format: {timestamp} {LEVEL} [{component}] [{version}] [{hostname}] {message} {key=value}...
	if !strings.Contains(output, "INFO") {
		t.Errorf("expected uppercase level INFO, got: %s", output)
	}
	if !strings.Contains(output, "[hyperfleet-api]") {
		t.Errorf("expected [hyperfleet-api], got: %s", output)
	}
	if !strings.Contains(output, "[v1.2.3]") {
		t.Errorf("expected [v1.2.3], got: %s", output)
	}
	if !strings.Contains(output, "[test-host]") {
		t.Errorf("expected [test-host], got: %s", output)
	}
	if !strings.Contains(output, "Test message") {
		t.Errorf("expected 'Test message', got: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("expected key=value, got: %s", output)
	}
}

// TestHyperFleetTextHandler_ContextFields tests context field extraction
func TestHyperFleetTextHandler_ContextFields(t *testing.T) {
	var buf bytes.Buffer
	handler := NewHyperFleetTextHandler(&buf, "hyperfleet-api", "v1.2.3", "test-host", slog.LevelInfo)

	ctx := context.Background()
	ctx = context.WithValue(ctx, ReqIDKey, "test-request-123")
	ctx = context.WithValue(ctx, TraceIDCtxKey, "trace-456")
	ctx = context.WithValue(ctx, SpanIDCtxKey, "span-789")
	ctx = context.WithValue(ctx, ClusterIDCtxKey, "cluster-abc")

	logger := slog.New(handler)
	logger.InfoContext(ctx, "Processing request")

	output := buf.String()

	if !strings.Contains(output, "request_id=test-request-123") {
		t.Errorf("expected request_id=test-request-123, got: %s", output)
	}
	if !strings.Contains(output, "trace_id=trace-456") {
		t.Errorf("expected trace_id=trace-456, got: %s", output)
	}
	if !strings.Contains(output, "span_id=span-789") {
		t.Errorf("expected span_id=span-789, got: %s", output)
	}
	if !strings.Contains(output, "cluster_id=cluster-abc") {
		t.Errorf("expected cluster_id=cluster-abc, got: %s", output)
	}
}

// TestHyperFleetTextHandler_SpecialCharacters tests value quoting for special characters
func TestHyperFleetTextHandler_SpecialCharacters(t *testing.T) {
	var buf bytes.Buffer
	handler := NewHyperFleetTextHandler(&buf, "hyperfleet-api", "v1.2.3", "test-host", slog.LevelInfo)

	ctx := context.Background()
	logger := slog.New(handler)
	logger.InfoContext(ctx, "Test message",
		"simple", "value",
		"with_spaces", "hello world",
		"with_quotes", `contains "quotes"`)

	output := buf.String()

	// Simple value without quotes
	if !strings.Contains(output, "simple=value") {
		t.Errorf("expected simple=value, got: %s", output)
	}

	// Value with spaces should be quoted
	if !strings.Contains(output, `with_spaces="hello world"`) {
		t.Errorf("expected quoted value for spaces, got: %s", output)
	}

	// Value with internal quotes should be escaped and quoted
	hasQuotes := strings.Contains(output, `with_quotes="contains \"quotes\""`) ||
		strings.Contains(output, `with_quotes="contains \\\"quotes\\\""`)
	if !hasQuotes {
		t.Errorf("expected escaped quotes, got: %s", output)
	}
}

// TestHyperFleetTextHandler_LogLevels tests different log levels
func TestHyperFleetTextHandler_LogLevels(t *testing.T) {
	tests := []struct {
		name          string
		level         slog.Level
		logFunc       func(*slog.Logger, context.Context, string)
		expectedLevel string
		shouldLog     bool
	}{
		{
			"DEBUG enabled", slog.LevelDebug,
			func(l *slog.Logger, ctx context.Context, msg string) { l.DebugContext(ctx, msg) },
			"DEBUG", true,
		},
		{
			"INFO enabled", slog.LevelInfo,
			func(l *slog.Logger, ctx context.Context, msg string) { l.InfoContext(ctx, msg) },
			"INFO", true,
		},
		{
			"WARN enabled", slog.LevelWarn,
			func(l *slog.Logger, ctx context.Context, msg string) { l.WarnContext(ctx, msg) },
			"WARN", true,
		},
		{
			"ERROR enabled", slog.LevelError,
			func(l *slog.Logger, ctx context.Context, msg string) { l.ErrorContext(ctx, msg) },
			"ERROR", true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			handler := NewHyperFleetTextHandler(&buf, "hyperfleet-api", "v1.2.3", "test-host", tt.level)

			ctx := context.Background()
			logger := slog.New(handler)
			tt.logFunc(logger, ctx, "Test message")

			output := buf.String()

			if tt.shouldLog && !strings.Contains(output, tt.expectedLevel) {
				t.Errorf("expected level %s, got: %s", tt.expectedLevel, output)
			}
		})
	}
}

// TestHyperFleetTextHandler_LevelFiltering tests log level filtering
func TestHyperFleetTextHandler_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	// Set handler to INFO level
	handler := NewHyperFleetTextHandler(&buf, "hyperfleet-api", "v1.2.3", "test-host", slog.LevelInfo)

	ctx := context.Background()
	logger := slog.New(handler)

	// DEBUG should be filtered out
	logger.DebugContext(ctx, "Debug message")
	if buf.Len() > 0 {
		t.Errorf("expected DEBUG to be filtered, got output: %s", buf.String())
	}

	// INFO should pass through
	logger.InfoContext(ctx, "Info message")
	if buf.Len() == 0 {
		t.Error("expected INFO to be logged")
	}
}

// TestHyperFleetTextHandler_MessageOnly tests logging with message only (no attributes)
func TestHyperFleetTextHandler_MessageOnly(t *testing.T) {
	var buf bytes.Buffer
	handler := NewHyperFleetTextHandler(&buf, "hyperfleet-api", "v1.2.3", "test-host", slog.LevelInfo)

	ctx := context.Background()
	logger := slog.New(handler)
	logger.InfoContext(ctx, "Simple message")

	output := buf.String()

	if !strings.Contains(output, "Simple message") {
		t.Errorf("expected 'Simple message', got: %s", output)
	}
	// Should have system fields but no additional key=value pairs
	if !strings.Contains(output, "[hyperfleet-api]") {
		t.Errorf("expected system fields, got: %s", output)
	}
}

// TestHyperFleetTextHandler_MultipleAttributes tests logging with multiple attributes
func TestHyperFleetTextHandler_MultipleAttributes(t *testing.T) {
	var buf bytes.Buffer
	handler := NewHyperFleetTextHandler(&buf, "hyperfleet-api", "v1.2.3", "test-host", slog.LevelInfo)

	ctx := context.Background()
	logger := slog.New(handler)
	logger.InfoContext(ctx, "Multiple attributes",
		"attr1", "value1",
		"attr2", 42,
		"attr3", true)

	output := buf.String()

	if !strings.Contains(output, "attr1=value1") {
		t.Errorf("expected attr1=value1, got: %s", output)
	}
	if !strings.Contains(output, "attr2=42") {
		t.Errorf("expected attr2=42, got: %s", output)
	}
	if !strings.Contains(output, "attr3=true") {
		t.Errorf("expected attr3=true, got: %s", output)
	}
}

// TestHyperFleetTextHandler_EmptyContext tests logging with empty context
func TestHyperFleetTextHandler_EmptyContext(t *testing.T) {
	var buf bytes.Buffer
	handler := NewHyperFleetTextHandler(&buf, "hyperfleet-api", "v1.2.3", "test-host", slog.LevelInfo)

	ctx := context.Background()
	logger := slog.New(handler)
	logger.InfoContext(ctx, "No context fields")

	output := buf.String()

	// Should not have request_id, trace_id, etc.
	if strings.Contains(output, "request_id=") {
		t.Errorf("expected no request_id, got: %s", output)
	}
	if strings.Contains(output, "trace_id=") {
		t.Errorf("expected no trace_id, got: %s", output)
	}
}

// TestFormatValue tests the formatValue helper function
func TestFormatValue(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{"simple string", "hello", "hello"},
		{"string with spaces", "hello world", `"hello world"`},
		{"string with quotes", `say "hello"`, `"say \"hello\""`},
		{"number", 42, "42"},
		{"boolean", true, "true"},
		{"nil", nil, "null"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatValue(tt.input)
			if result != tt.expected {
				t.Errorf("formatValue(%v) = %s, expected %s", tt.input, result, tt.expected)
			}
		})
	}
}
