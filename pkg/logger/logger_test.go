package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
)

// TestParseLogLevel tests log level parsing with various inputs
func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  slog.Level
		expectErr bool
	}{
		{"debug", "debug", slog.LevelDebug, false},
		{"info", "info", slog.LevelInfo, false},
		{"warn", "warn", slog.LevelWarn, false},
		{"warning", "warning", slog.LevelWarn, false},
		{"error", "error", slog.LevelError, false},
		{"case insensitive", "DEBUG", slog.LevelDebug, false},
		{"with whitespace", "  info  ", slog.LevelInfo, false},
		{"invalid", "invalid", slog.LevelInfo, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, err := ParseLogLevel(tt.input)
			if tt.expectErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				if !strings.Contains(err.Error(), "unknown log level") {
					t.Errorf("unexpected error message: %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if level != tt.expected {
					t.Errorf("expected %v, got %v", tt.expected, level)
				}
			}
		})
	}
}

// TestParseLogFormat tests log format parsing with various inputs
func TestParseLogFormat(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  LogFormat
		expectErr bool
	}{
		{"text", "text", FormatText, false},
		{"json", "json", FormatJSON, false},
		{"case insensitive", "JSON", FormatJSON, false},
		{"invalid", "invalid", FormatText, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format, err := ParseLogFormat(tt.input)
			if tt.expectErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				if !strings.Contains(err.Error(), "unknown log format") {
					t.Errorf("unexpected error message: %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if format != tt.expected {
					t.Errorf("expected %v, got %v", tt.expected, format)
				}
			}
		})
	}
}

// TestParseLogOutput tests log output parsing with various inputs
func TestParseLogOutput(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  *os.File
		expectErr bool
	}{
		{"stdout", "stdout", os.Stdout, false},
		{"stderr", "stderr", os.Stderr, false},
		{"empty defaults to stdout", "", os.Stdout, false},
		{"invalid", "invalid", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := ParseLogOutput(tt.input)
			if tt.expectErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				if !strings.Contains(err.Error(), "unknown log output") {
					t.Errorf("unexpected error message: %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if output != tt.expected {
					t.Errorf("expected %v, got %v", tt.expected, output)
				}
			}
		})
	}
}

// TestJSONFormat_BasicLog tests basic JSON format output
func TestJSONFormat_BasicLog(t *testing.T) {
	var buf bytes.Buffer
	cfg := &LogConfig{
		Level:     slog.LevelInfo,
		Format:    FormatJSON,
		Output:    &buf,
		Component: "api",
		Version:   "test-version",
		Hostname:  "test-host",
	}

	InitGlobalLogger(cfg)
	ctx := context.Background()
	Info(ctx, "test message", "key", "value")

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse JSON log: %v", err)
	}

	// Check required fields
	if logEntry["message"] != "test message" {
		t.Errorf("expected message 'test message', got %v", logEntry["message"])
	}
	if logEntry["level"] != "info" {
		t.Errorf("expected level 'info', got %v", logEntry["level"])
	}
	if logEntry["component"] != "api" {
		t.Errorf("expected component 'api', got %v", logEntry["component"])
	}
	if logEntry["version"] != "test-version" {
		t.Errorf("expected version 'test-version', got %v", logEntry["version"])
	}
	if logEntry["hostname"] != "test-host" {
		t.Errorf("expected hostname 'test-host', got %v", logEntry["hostname"])
	}
	if logEntry["key"] != "value" {
		t.Errorf("expected key 'value', got %v", logEntry["key"])
	}
}

// TestTextFormat_BasicLog tests basic text format output
func TestTextFormat_BasicLog(t *testing.T) {
	var buf bytes.Buffer
	cfg := &LogConfig{
		Level:     slog.LevelInfo,
		Format:    FormatText,
		Output:    &buf,
		Component: "api",
		Version:   "test-version",
		Hostname:  "test-host",
	}

	InitGlobalLogger(cfg)
	ctx := context.Background()
	Info(ctx, "test message", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("expected output to contain 'test message', got: %s", output)
	}
	if !strings.Contains(output, "level=info") {
		t.Errorf("expected output to contain 'level=info', got: %s", output)
	}
	if !strings.Contains(output, "component=api") {
		t.Errorf("expected output to contain 'component=api', got: %s", output)
	}
}

// TestContextFields tests context field extraction for all supported fields
func TestContextFields(t *testing.T) {
	tests := []struct {
		name       string
		setupCtx   func(context.Context) context.Context
		fieldName  string
		fieldValue string
	}{
		{
			name:       "trace_id",
			setupCtx:   func(ctx context.Context) context.Context { return WithTraceID(ctx, "trace-123") },
			fieldName:  "trace_id",
			fieldValue: "trace-123",
		},
		{
			name:       "span_id",
			setupCtx:   func(ctx context.Context) context.Context { return WithSpanID(ctx, "span-456") },
			fieldName:  "span_id",
			fieldValue: "span-456",
		},
		{
			name: "request_id",
			setupCtx: func(ctx context.Context) context.Context {
				return context.WithValue(ctx, OpIDKey, "op-789")
			},
			fieldName:  "request_id",
			fieldValue: "op-789",
		},
		{
			name:       "cluster_id",
			setupCtx:   func(ctx context.Context) context.Context { return WithClusterID(ctx, "cluster-abc") },
			fieldName:  "cluster_id",
			fieldValue: "cluster-abc",
		},
		{
			name:       "resource_type",
			setupCtx:   func(ctx context.Context) context.Context { return WithResourceType(ctx, "managed-cluster") },
			fieldName:  "resource_type",
			fieldValue: "managed-cluster",
		},
		{
			name:       "resource_id",
			setupCtx:   func(ctx context.Context) context.Context { return WithResourceID(ctx, "resource-xyz") },
			fieldName:  "resource_id",
			fieldValue: "resource-xyz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := &LogConfig{
				Level:     slog.LevelInfo,
				Format:    FormatJSON,
				Output:    &buf,
				Component: "api",
				Version:   "test-version",
				Hostname:  "test-host",
			}
			InitGlobalLogger(cfg)

			ctx := tt.setupCtx(context.Background())
			Info(ctx, "test message")

			var logEntry map[string]interface{}
			if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
				t.Fatalf("failed to parse JSON log: %v", err)
			}

			if logEntry[tt.fieldName] != tt.fieldValue {
				t.Errorf("expected %s='%s', got %v", tt.fieldName, tt.fieldValue, logEntry[tt.fieldName])
			}
		})
	}
}

// TestContextFields_Multiple tests multiple context fields together
func TestContextFields_Multiple(t *testing.T) {
	var buf bytes.Buffer
	cfg := &LogConfig{
		Level:     slog.LevelInfo,
		Format:    FormatJSON,
		Output:    &buf,
		Component: "api",
		Version:   "test-version",
		Hostname:  "test-host",
	}

	InitGlobalLogger(cfg)
	ctx := context.Background()
	ctx = WithTraceID(ctx, "trace-123")
	ctx = WithSpanID(ctx, "span-456")
	ctx = WithClusterID(ctx, "cluster-abc")
	Info(ctx, "test message")

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse JSON log: %v", err)
	}

	if logEntry["trace_id"] != "trace-123" {
		t.Errorf("expected trace_id 'trace-123', got %v", logEntry["trace_id"])
	}
	if logEntry["span_id"] != "span-456" {
		t.Errorf("expected span_id 'span-456', got %v", logEntry["span_id"])
	}
	if logEntry["cluster_id"] != "cluster-abc" {
		t.Errorf("expected cluster_id 'cluster-abc', got %v", logEntry["cluster_id"])
	}
}

// TestStackTrace tests stack trace capture for different log levels
func TestStackTrace(t *testing.T) {
	tests := []struct {
		name             string
		level            slog.Level
		logFunc          func(context.Context, string, ...any)
		expectStackTrace bool
		expectErrorField bool
	}{
		{"error level has stack trace", slog.LevelError, Error, true, true},
		{"info level no stack trace", slog.LevelInfo, Info, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := &LogConfig{
				Level:     tt.level,
				Format:    FormatJSON,
				Output:    &buf,
				Component: "api",
				Version:   "test-version",
				Hostname:  "test-host",
			}
			InitGlobalLogger(cfg)

			ctx := context.Background()
			tt.logFunc(ctx, "test message")

			var logEntry map[string]interface{}
			if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
				t.Fatalf("failed to parse JSON log: %v", err)
			}

			if tt.expectStackTrace {
				if logEntry["stack_trace"] == nil {
					t.Error("expected stack_trace field for error level")
				}
				// Verify stack trace is an array
				stackTrace, ok := logEntry["stack_trace"].([]interface{})
				if !ok {
					t.Fatalf("expected stack_trace to be array, got %T", logEntry["stack_trace"])
				}
				if len(stackTrace) == 0 {
					t.Error("expected stack_trace to have at least one frame")
				}
			} else {
				if logEntry["stack_trace"] != nil {
					t.Error("expected no stack_trace field for non-error level")
				}
			}

			if tt.expectErrorField {
				if logEntry["error"] != "test message" {
					t.Errorf("expected error field 'test message', got %v", logEntry["error"])
				}
			} else {
				if logEntry["error"] != nil {
					t.Error("expected no error field for non-error level")
				}
			}
		})
	}
}

// TestLogLevelFiltering tests log level filtering
func TestLogLevelFiltering(t *testing.T) {
	tests := []struct {
		name        string
		configLevel slog.Level
		logFunc     func(context.Context, string, ...any)
		shouldLog   bool
	}{
		{"debug filtered at info level", slog.LevelInfo, Debug, false},
		{"info enabled at info level", slog.LevelInfo, Info, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := &LogConfig{
				Level:     tt.configLevel,
				Format:    FormatJSON,
				Output:    &buf,
				Component: "api",
				Version:   "test-version",
				Hostname:  "test-host",
			}
			InitGlobalLogger(cfg)

			ctx := context.Background()
			tt.logFunc(ctx, "test message")

			if tt.shouldLog && buf.Len() == 0 {
				t.Error("expected log output, got none")
			}
			if !tt.shouldLog && buf.Len() > 0 {
				t.Errorf("expected no log output, got: %s", buf.String())
			}
		})
	}
}

// TestGetLogger_Uninitialized tests GetLogger with uninitialized global logger
func TestGetLogger_Uninitialized(t *testing.T) {
	// Save and restore global logger
	saved := globalLogger
	defer func() { globalLogger = saved }()

	globalLogger = nil
	ctx := context.Background()
	logger := GetLogger(ctx)

	if logger == nil {
		t.Error("expected GetLogger to return non-nil logger")
	}

	// Should return default logger
	if logger != slog.Default() {
		t.Error("expected GetLogger to return slog.Default() when uninitialized")
	}
}

// TestConvenienceFunctions tests convenience functions (Debug, Info, Warn, Error)
func TestConvenienceFunctions(t *testing.T) {
	tests := []struct {
		name        string
		level       slog.Level
		logFunc     func(context.Context, string, ...any)
		expectedLvl string
	}{
		{"Debug", slog.LevelDebug, Debug, "debug"},
		{"Warn", slog.LevelWarn, Warn, "warn"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := &LogConfig{
				Level:     tt.level,
				Format:    FormatJSON,
				Output:    &buf,
				Component: "api",
				Version:   "test-version",
				Hostname:  "test-host",
			}
			InitGlobalLogger(cfg)

			ctx := context.Background()
			tt.logFunc(ctx, "test message", "key", "value")

			var logEntry map[string]interface{}
			if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
				t.Fatalf("failed to parse JSON log: %v", err)
			}

			if logEntry["level"] != tt.expectedLvl {
				t.Errorf("expected level '%s', got %v", tt.expectedLvl, logEntry["level"])
			}
			if logEntry["message"] != "test message" {
				t.Errorf("expected message 'test message', got %v", logEntry["message"])
			}
		})
	}
}

// TestFormattedFunctions tests printf-style formatted functions (Debugf, Infof, Warnf, Errorf)
func TestFormattedFunctions(t *testing.T) {
	tests := []struct {
		name             string
		level            slog.Level
		logFunc          func(context.Context, string, ...interface{})
		format           string
		args             []interface{}
		expectedMsg      string
		expectedLvl      string
		expectStackTrace bool
		expectErrorField bool
	}{
		{
			name:             "Debugf",
			level:            slog.LevelDebug,
			logFunc:          Debugf,
			format:           "debug message with %s and %d",
			args:             []interface{}{"string", 123},
			expectedMsg:      "debug message with string and 123",
			expectedLvl:      "debug",
			expectStackTrace: false,
			expectErrorField: false,
		},
		{
			name:             "Infof",
			level:            slog.LevelInfo,
			logFunc:          Infof,
			format:           "info message: cluster=%s, count=%d",
			args:             []interface{}{"test-cluster", 42},
			expectedMsg:      "info message: cluster=test-cluster, count=42",
			expectedLvl:      "info",
			expectStackTrace: false,
			expectErrorField: false,
		},
		{
			name:             "Warnf",
			level:            slog.LevelWarn,
			logFunc:          Warnf,
			format:           "warning: %s failed with code %d",
			args:             []interface{}{"operation", 500},
			expectedMsg:      "warning: operation failed with code 500",
			expectedLvl:      "warn",
			expectStackTrace: false,
			expectErrorField: false,
		},
		{
			name:             "Errorf",
			level:            slog.LevelError,
			logFunc:          Errorf,
			format:           "error: failed to process %s: %v",
			args:             []interface{}{"resource-123", "connection timeout"},
			expectedMsg:      "error: failed to process resource-123: connection timeout",
			expectedLvl:      "error",
			expectStackTrace: true,
			expectErrorField: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := &LogConfig{
				Level:     tt.level,
				Format:    FormatJSON,
				Output:    &buf,
				Component: "api",
				Version:   "test-version",
				Hostname:  "test-host",
			}
			InitGlobalLogger(cfg)

			ctx := context.Background()
			tt.logFunc(ctx, tt.format, tt.args...)

			var logEntry map[string]interface{}
			if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
				t.Fatalf("failed to parse JSON log: %v", err)
			}

			if logEntry["level"] != tt.expectedLvl {
				t.Errorf("expected level '%s', got %v", tt.expectedLvl, logEntry["level"])
			}
			if logEntry["message"] != tt.expectedMsg {
				t.Errorf("expected message '%s', got %v", tt.expectedMsg, logEntry["message"])
			}

			if tt.expectErrorField {
				if logEntry["error"] != tt.expectedMsg {
					t.Errorf("expected error field '%s', got %v", tt.expectedMsg, logEntry["error"])
				}
			}

			if tt.expectStackTrace {
				if logEntry["stack_trace"] == nil {
					t.Error("expected stack_trace field for error level")
				}
			}
		})
	}
}
