package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
)

// LogFormat enumeration for output format
type LogFormat int

const (
	// FormatText outputs logs in human-readable text format
	FormatText LogFormat = iota
	// FormatJSON outputs logs in JSON format for structured logging
	FormatJSON
)

// LogConfig holds the configuration for the logger
type LogConfig struct {
	Level     slog.Level
	Format    LogFormat
	Output    io.Writer
	Component string
	Version   string
	Hostname  string
}

// HyperFleetHandler implements slog.Handler interface
// Adds HyperFleet-specific fields: component, version, hostname, trace_id, span_id, etc.
type HyperFleetHandler struct {
	handler   slog.Handler
	component string
	version   string
	hostname  string
}

// NewHyperFleetHandler creates a HyperFleet logger handler
func NewHyperFleetHandler(cfg *LogConfig) *HyperFleetHandler {
	var baseHandler slog.Handler
	opts := &slog.HandlerOptions{
		Level:     cfg.Level,
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{Key: "timestamp", Value: a.Value}
			}
			if a.Key == slog.LevelKey {
				// Safe type assertion to avoid panic
				if level, ok := a.Value.Any().(slog.Level); ok {
					return slog.Attr{Key: "level", Value: slog.StringValue(strings.ToLower(level.String()))}
				}
				// Fallback: convert to string if type assertion fails
				return slog.Attr{Key: "level", Value: slog.StringValue(strings.ToLower(fmt.Sprint(a.Value.Any())))}
			}
			if a.Key == slog.MessageKey {
				return slog.Attr{Key: "message", Value: a.Value}
			}
			if a.Key == slog.SourceKey {
				return a
			}
			return a
		},
	}

	if cfg.Format == FormatJSON {
		baseHandler = slog.NewJSONHandler(cfg.Output, opts)
	} else {
		baseHandler = slog.NewTextHandler(cfg.Output, opts)
	}

	return &HyperFleetHandler{
		handler:   baseHandler,
		component: cfg.Component,
		version:   cfg.Version,
		hostname:  cfg.Hostname,
	}
}

// Handle implements slog.Handler interface
func (h *HyperFleetHandler) Handle(ctx context.Context, r slog.Record) error {
	r.AddAttrs(
		slog.String("component", h.component),
		slog.String("version", h.version),
		slog.String("hostname", h.hostname),
	)

	if traceID, ok := GetTraceID(ctx); ok && traceID != "" {
		r.AddAttrs(slog.String("trace_id", traceID))
	}
	if spanID, ok := GetSpanID(ctx); ok && spanID != "" {
		r.AddAttrs(slog.String("span_id", spanID))
	}
	if operationID := GetOperationID(ctx); operationID != "" {
		r.AddAttrs(slog.String("operation_id", operationID))
	}
	if accountID, ok := GetAccountID(ctx); ok && accountID != "" {
		r.AddAttrs(slog.String("account_id", accountID))
	}
	if transactionID, ok := GetTransactionID(ctx); ok && transactionID != 0 {
		r.AddAttrs(slog.Int64("transaction_id", transactionID))
	}
	if clusterID, ok := GetClusterID(ctx); ok && clusterID != "" {
		r.AddAttrs(slog.String("cluster_id", clusterID))
	}
	if resourceType, ok := GetResourceType(ctx); ok && resourceType != "" {
		r.AddAttrs(slog.String("resource_type", resourceType))
	}
	if resourceID, ok := GetResourceID(ctx); ok && resourceID != "" {
		r.AddAttrs(slog.String("resource_id", resourceID))
	}

	if r.Level >= slog.LevelError {
		stackTrace := captureStackTrace(4)
		r.AddAttrs(slog.Any("stack_trace", stackTrace))
	}

	return h.handler.Handle(ctx, r)
}

// Enabled implements slog.Handler interface
func (h *HyperFleetHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

// WithAttrs implements slog.Handler interface
func (h *HyperFleetHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &HyperFleetHandler{
		handler:   h.handler.WithAttrs(attrs),
		component: h.component,
		version:   h.version,
		hostname:  h.hostname,
	}
}

// WithGroup implements slog.Handler interface
func (h *HyperFleetHandler) WithGroup(name string) slog.Handler {
	return &HyperFleetHandler{
		handler:   h.handler.WithGroup(name),
		component: h.component,
		version:   h.version,
		hostname:  h.hostname,
	}
}

func captureStackTrace(skip int) []string {
	const maxFrames = 15
	pcs := make([]uintptr, maxFrames)
	n := runtime.Callers(skip, pcs)

	frames := runtime.CallersFrames(pcs[:n])
	var stackTrace []string
	for {
		frame, more := frames.Next()
		if !strings.Contains(frame.Function, "runtime.") &&
			!strings.Contains(frame.Function, "testing.") {
			stackTrace = append(stackTrace,
				fmt.Sprintf("%s:%d %s", frame.File, frame.Line, frame.Function))
		}
		if !more {
			break
		}
	}
	return stackTrace
}

var globalLogger atomic.Value // stores *slog.Logger

// InitGlobalLogger initializes the global logger with the given configuration
// This function is thread-safe and idempotent (safe to call multiple times)
func InitGlobalLogger(cfg *LogConfig) {
	if v := globalLogger.Load(); v != nil {
		if logger, ok := v.(*slog.Logger); ok && logger != nil {
			return
		}
	}

	handler := NewHyperFleetHandler(cfg)
	logger := slog.New(handler)
	globalLogger.Store(logger)
	slog.SetDefault(logger)
}

// ReconfigureGlobalLogger reconfigures the global logger with new configuration
// Unlike InitGlobalLogger, this can be called multiple times to update configuration
// This is useful when environment configuration is loaded after initial logger setup
func ReconfigureGlobalLogger(cfg *LogConfig) {
	handler := NewHyperFleetHandler(cfg)
	newLogger := slog.New(handler)
	globalLogger.Store(newLogger)
	slog.SetDefault(newLogger)
}

// resetForTesting resets the global logger for testing purposes
// This function should ONLY be used in tests
func resetForTesting() {
	globalLogger.Store((*slog.Logger)(nil))
}

// GetLogger returns the global logger instance
func GetLogger() *slog.Logger {
	if v := globalLogger.Load(); v != nil {
		if logger, ok := v.(*slog.Logger); ok && logger != nil {
			return logger
		}
	}
	return slog.Default()
}

// ParseLogLevel converts string to slog.Level
func ParseLogLevel(level string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level: %s (valid: debug, info, warn, error)", level)
	}
}

// ParseLogFormat converts string to LogFormat
func ParseLogFormat(format string) (LogFormat, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "text":
		return FormatText, nil
	case "json":
		return FormatJSON, nil
	default:
		return FormatText, fmt.Errorf("unknown log format: %s (valid: text, json)", format)
	}
}

// ParseLogOutput converts string to io.Writer
func ParseLogOutput(output string) (io.Writer, error) {
	switch strings.ToLower(strings.TrimSpace(output)) {
	case "stdout", "":
		return os.Stdout, nil
	case "stderr":
		return os.Stderr, nil
	default:
		return nil, fmt.Errorf("unknown log output: %s (valid: stdout, stderr)", output)
	}
}

func Debug(ctx context.Context, msg string, args ...any) {
	GetLogger().DebugContext(ctx, msg, args...)
}

func Info(ctx context.Context, msg string, args ...any) {
	GetLogger().InfoContext(ctx, msg, args...)
}

func Warn(ctx context.Context, msg string, args ...any) {
	GetLogger().WarnContext(ctx, msg, args...)
}

func Error(ctx context.Context, msg string, args ...any) {
	GetLogger().ErrorContext(ctx, msg, args...)
}

func Debugf(ctx context.Context, format string, args ...interface{}) {
	GetLogger().DebugContext(ctx, fmt.Sprintf(format, args...))
}

func Infof(ctx context.Context, format string, args ...interface{}) {
	GetLogger().InfoContext(ctx, fmt.Sprintf(format, args...))
}

func Warnf(ctx context.Context, format string, args ...interface{}) {
	GetLogger().WarnContext(ctx, fmt.Sprintf(format, args...))
}

func Errorf(ctx context.Context, format string, args ...interface{}) {
	GetLogger().ErrorContext(ctx, fmt.Sprintf(format, args...))
}
