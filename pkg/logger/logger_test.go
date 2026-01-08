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

// TestBasicLogFormat tests basic log format output for both JSON and text formats
func TestBasicLogFormat(t *testing.T) {
	tests := []struct {
		name   string
		format LogFormat
	}{
		{"JSON format", FormatJSON},
		{"Text format", FormatText},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetForTesting()
			var buf bytes.Buffer
			cfg := &LogConfig{
				Level:     slog.LevelInfo,
				Format:    tt.format,
				Output:    &buf,
				Component: "api",
				Version:   "test-version",
				Hostname:  "test-host",
			}

			InitGlobalLogger(cfg)
			ctx := context.Background()
			With(ctx, "key", "value").Info("test message")

			output := buf.String()
			if output == "" {
				t.Fatal("expected log output, got none")
			}

			if tt.format == FormatJSON {
				// Parse the last JSON line (handles multi-line output like stack traces)
				output := strings.TrimSpace(buf.String())
				lines := strings.Split(output, "\n")
				lastLine := lines[len(lines)-1]

				var logEntry map[string]interface{}
				if err := json.Unmarshal([]byte(lastLine), &logEntry); err != nil {
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
			} else {
				// Text format - check for stable invariants only
				if !strings.Contains(output, "test message") {
					t.Errorf("expected output to contain 'test message', got: %s", output)
				}
				// Check for log level (case-insensitive)
				outputLower := strings.ToLower(output)
				if !strings.Contains(outputLower, "info") {
					t.Errorf("expected output to contain log level 'info', got: %s", output)
				}
			}
		})
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
				return context.WithValue(ctx, ReqIDKey, "req-789")
			},
			fieldName:  "request_id",
			fieldValue: "req-789",
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
			resetForTesting()
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

			// Parse the last JSON line (handles multi-line output like stack traces)
			output := strings.TrimSpace(buf.String())
			lines := strings.Split(output, "\n")
			lastLine := lines[len(lines)-1]

			var logEntry map[string]interface{}
			if err := json.Unmarshal([]byte(lastLine), &logEntry); err != nil {
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
	resetForTesting()
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

	// Parse the last JSON line (handles multi-line output like stack traces)
	output := strings.TrimSpace(buf.String())
	lines := strings.Split(output, "\n")
	lastLine := lines[len(lines)-1]

	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(lastLine), &logEntry); err != nil {
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
		logFunc          func(context.Context, string)
		expectStackTrace bool
	}{
		{"error level has stack trace", slog.LevelError, Error, true},
		{"info level no stack trace", slog.LevelInfo, Info, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetForTesting()
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

			// Parse the last JSON line (handles multi-line output like stack traces)
			output := strings.TrimSpace(buf.String())
			lines := strings.Split(output, "\n")
			lastLine := lines[len(lines)-1]

			var logEntry map[string]interface{}
			if err := json.Unmarshal([]byte(lastLine), &logEntry); err != nil {
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
		})
	}
}

// TestLogLevelFiltering tests log level filtering
func TestLogLevelFiltering(t *testing.T) {
	tests := []struct {
		name        string
		configLevel slog.Level
		logFunc     func(context.Context, string)
		shouldLog   bool
	}{
		{"debug filtered at info level", slog.LevelInfo, Debug, false},
		{"info enabled at info level", slog.LevelInfo, Info, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetForTesting()
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
	saved := globalLogger.Load()
	defer func() { globalLogger.Store(saved) }()

	globalLogger.Store((*slog.Logger)(nil))
	logger := GetLogger()

	if logger == nil {
		t.Error("expected GetLogger to return non-nil logger")
	}

	// Should return default logger
	if logger != slog.Default() {
		t.Error("expected GetLogger to return slog.Default() when uninitialized")
	}
}

// TestConvenienceFunctions tests all convenience functions (Debug, Info, Warn, Error)
func TestConvenienceFunctions(t *testing.T) {
	tests := []struct {
		name        string
		level       slog.Level
		logFunc     func(context.Context, string)
		expectedLvl string
	}{
		{"Debug", slog.LevelDebug, Debug, "debug"},
		{"Info", slog.LevelInfo, Info, "info"},
		{"Warn", slog.LevelWarn, Warn, "warn"},
		{"Error", slog.LevelError, Error, "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetForTesting()
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

			// Parse the last JSON line (handles multi-line output like stack traces)
			output := strings.TrimSpace(buf.String())
			lines := strings.Split(output, "\n")
			lastLine := lines[len(lines)-1]

			var logEntry map[string]interface{}
			if err := json.Unmarshal([]byte(lastLine), &logEntry); err != nil {
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

// TestWithError tests the WithError convenience function
func TestWithError(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		expectError   bool
		expectedValue string
	}{
		{
			name:          "non-nil error",
			err:           bytes.ErrTooLarge,
			expectError:   true,
			expectedValue: "bytes.Buffer: too large",
		},
		{
			name:        "nil error",
			err:         nil,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetForTesting()
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
			WithError(ctx, tt.err).Info("test message")

			// Parse the last JSON line
			output := strings.TrimSpace(buf.String())
			lines := strings.Split(output, "\n")
			lastLine := lines[len(lines)-1]

			var logEntry map[string]interface{}
			if err := json.Unmarshal([]byte(lastLine), &logEntry); err != nil {
				t.Fatalf("failed to parse JSON log: %v", err)
			}

			// Verify message is always present
			if logEntry["message"] != "test message" {
				t.Errorf("expected message 'test message', got %v", logEntry["message"])
			}

			// Check error field presence
			if tt.expectError {
				if logEntry["error"] == nil {
					t.Error("expected error field to be present")
				}
				if logEntry["error"] != tt.expectedValue {
					t.Errorf("expected error '%s', got %v", tt.expectedValue, logEntry["error"])
				}
			} else {
				if logEntry["error"] != nil {
					t.Errorf("expected no error field for nil error, got %v", logEntry["error"])
				}
			}
		})
	}
}

// TestWithError_MethodChaining tests that WithError returns a ContextLogger for method chaining
func TestWithError_MethodChaining(t *testing.T) {
	resetForTesting()
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
	testErr := bytes.ErrTooLarge

	// Test method chaining works at different log levels
	WithError(ctx, testErr).Info("info message")
	WithError(ctx, testErr).Warn("warn message")
	WithError(ctx, testErr).Error("error message")

	output := buf.String()
	if !strings.Contains(output, "info message") {
		t.Error("expected output to contain 'info message'")
	}
	if !strings.Contains(output, "warn message") {
		t.Error("expected output to contain 'warn message'")
	}
	if !strings.Contains(output, "error message") {
		t.Error("expected output to contain 'error message'")
	}
}

// TestContextLogger_With_Single tests single With call
func TestContextLogger_With_Single(t *testing.T) {
	resetForTesting()
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
	With(ctx, "user_id", "user123").With("action", "login").Info("User action")

	// Parse the last JSON line
	output := strings.TrimSpace(buf.String())
	lines := strings.Split(output, "\n")
	lastLine := lines[len(lines)-1]

	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(lastLine), &logEntry); err != nil {
		t.Fatalf("failed to parse JSON log: %v", err)
	}

	if logEntry["user_id"] != "user123" {
		t.Errorf("expected user_id 'user123', got %v", logEntry["user_id"])
	}
	if logEntry["action"] != "login" {
		t.Errorf("expected action 'login', got %v", logEntry["action"])
	}
	if logEntry["message"] != "User action" {
		t.Errorf("expected message 'User action', got %v", logEntry["message"])
	}
}

// TestContextLogger_With_Chaining tests multiple With calls chained together
func TestContextLogger_With_Chaining(t *testing.T) {
	resetForTesting()
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
	With(ctx, "field1", "value1").
		With("field2", "value2").
		With("field3", "value3").
		Info("Test chaining")

	// Parse the last JSON line
	output := strings.TrimSpace(buf.String())
	lines := strings.Split(output, "\n")
	lastLine := lines[len(lines)-1]

	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(lastLine), &logEntry); err != nil {
		t.Fatalf("failed to parse JSON log: %v", err)
	}

	if logEntry["field1"] != "value1" {
		t.Errorf("expected field1 'value1', got %v", logEntry["field1"])
	}
	if logEntry["field2"] != "value2" {
		t.Errorf("expected field2 'value2', got %v", logEntry["field2"])
	}
	if logEntry["field3"] != "value3" {
		t.Errorf("expected field3 'value3', got %v", logEntry["field3"])
	}
}

// TestContextLogger_With_AndWithError tests combining With and WithError
func TestContextLogger_With_AndWithError(t *testing.T) {
	resetForTesting()
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
	testErr := bytes.ErrTooLarge
	With(ctx, "user_id", "user456").
		With("operation", "upload").
		WithError(testErr).
		Error("Upload failed")

	// Parse the last JSON line
	output := strings.TrimSpace(buf.String())
	lines := strings.Split(output, "\n")
	lastLine := lines[len(lines)-1]

	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(lastLine), &logEntry); err != nil {
		t.Fatalf("failed to parse JSON log: %v", err)
	}

	if logEntry["user_id"] != "user456" {
		t.Errorf("expected user_id 'user456', got %v", logEntry["user_id"])
	}
	if logEntry["operation"] != "upload" {
		t.Errorf("expected operation 'upload', got %v", logEntry["operation"])
	}
	if logEntry["error"] != "bytes.Buffer: too large" {
		t.Errorf("expected error 'bytes.Buffer: too large', got %v", logEntry["error"])
	}
}

// TestContextLogger_With_Empty tests With with no arguments
func TestContextLogger_With_Empty(t *testing.T) {
	resetForTesting()
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
	// With() with no arguments should still work
	With(ctx, "field1", "value1").With().Info("Test empty With")

	// Parse the last JSON line
	output := strings.TrimSpace(buf.String())
	lines := strings.Split(output, "\n")
	lastLine := lines[len(lines)-1]

	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(lastLine), &logEntry); err != nil {
		t.Fatalf("failed to parse JSON log: %v", err)
	}

	if logEntry["field1"] != "value1" {
		t.Errorf("expected field1 'value1', got %v", logEntry["field1"])
	}
	if logEntry["message"] != "Test empty With" {
		t.Errorf("expected message 'Test empty With', got %v", logEntry["message"])
	}
}

// TestContextLogger_With_SlogAttr tests With combined with slog.Attr
func TestContextLogger_With_SlogAttr(t *testing.T) {
	resetForTesting()
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
	With(ctx,
		slog.String("string_field", "string_value"),
		slog.Int("int_field", 42),
	).With(
		slog.Bool("bool_field", true),
	).Info("Test slog.Attr")

	// Parse the last JSON line
	output := strings.TrimSpace(buf.String())
	lines := strings.Split(output, "\n")
	lastLine := lines[len(lines)-1]

	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(lastLine), &logEntry); err != nil {
		t.Fatalf("failed to parse JSON log: %v", err)
	}

	if logEntry["string_field"] != "string_value" {
		t.Errorf("expected string_field 'string_value', got %v", logEntry["string_field"])
	}
	// JSON numbers are float64 by default
	if intField, ok := logEntry["int_field"].(float64); !ok || int(intField) != 42 {
		t.Errorf("expected int_field 42, got %v", logEntry["int_field"])
	}
	if logEntry["bool_field"] != true {
		t.Errorf("expected bool_field true, got %v", logEntry["bool_field"])
	}
}

// TestContextLogger_With_HTTPHelpers tests With combined with HTTP helper functions
func TestContextLogger_With_HTTPHelpers(t *testing.T) {
	resetForTesting()
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
	With(ctx,
		HTTPMethod("POST"),
		HTTPPath("/api/v1/users"),
	).With(
		HTTPStatusCode(201),
	).Info("HTTP request handled")

	// Parse the last JSON line
	output := strings.TrimSpace(buf.String())
	lines := strings.Split(output, "\n")
	lastLine := lines[len(lines)-1]

	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(lastLine), &logEntry); err != nil {
		t.Fatalf("failed to parse JSON log: %v", err)
	}

	if logEntry["method"] != "POST" {
		t.Errorf("expected method 'POST', got %v", logEntry["method"])
	}
	if logEntry["path"] != "/api/v1/users" {
		t.Errorf("expected path '/api/v1/users', got %v", logEntry["path"])
	}
	// JSON numbers are float64 by default
	if statusCode, ok := logEntry["status_code"].(float64); !ok || int(statusCode) != 201 {
		t.Errorf("expected status_code 201, got %v", logEntry["status_code"])
	}
}
