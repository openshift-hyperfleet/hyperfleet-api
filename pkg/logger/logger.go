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
// Returns slog.Handler interface to support both HyperFleetHandler (JSON) and HyperFleetTextHandler (Text)
func NewHyperFleetHandler(cfg *LogConfig) slog.Handler {
	if cfg.Format == FormatText {
		return NewHyperFleetTextHandler(cfg.Output, cfg.Component, cfg.Version, cfg.Hostname, cfg.Level)
	}

	var baseHandler slog.Handler
	opts := &slog.HandlerOptions{
		Level:     cfg.Level,
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{Key: "timestamp", Value: a.Value}
			}
			if a.Key == slog.LevelKey {
				if level, ok := a.Value.Any().(slog.Level); ok {
					return slog.Attr{Key: "level", Value: slog.StringValue(strings.ToLower(level.String()))}
				}
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

	baseHandler = slog.NewJSONHandler(cfg.Output, opts)

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

	for _, field := range ContextFieldsRegistry {
		if val, ok := field.Getter(ctx); ok {
			r.AddAttrs(slog.String(field.Name, val))
		}
	}

	if transactionID, ok := GetTransactionID(ctx); ok {
		r.AddAttrs(slog.Int64("transaction_id", transactionID))
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

// InitGlobalLogger initializes the global logger with the given configuration.
// This function is idempotent (safe to call multiple times).
//
// Concurrency note: This function uses atomic.Value without sync.Once intentionally.
// Current call sites are serialized (single-threaded main() and sync.Once in tests).
// If concurrent initialization occurs, the last Store() wins, which is acceptable.
// For stricter guarantees in future use cases, callers should wrap this with sync.Once
// or equivalent synchronization.
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

// Debug logs at Debug level with context fields only.
// For temporary fields, use With().
func Debug(ctx context.Context, msg string) {
	GetLogger().DebugContext(ctx, msg)
}

// Info logs at Info level with context fields only.
// For temporary fields, use With().
func Info(ctx context.Context, msg string) {
	GetLogger().InfoContext(ctx, msg)
}

// Warn logs at Warn level with context fields only.
// For temporary fields, use With().
func Warn(ctx context.Context, msg string) {
	GetLogger().WarnContext(ctx, msg)
}

// Error logs at Error level with context fields only.
// For temporary fields, use With().
func Error(ctx context.Context, msg string) {
	GetLogger().ErrorContext(ctx, msg)
}

// ContextLogger wraps a context with additional temporary key-value pairs for logging.
type ContextLogger struct {
	ctx    context.Context
	logger *slog.Logger
}

// With creates a new ContextLogger with temporary key-value pairs.
func With(ctx context.Context, args ...any) *ContextLogger {
	return &ContextLogger{
		ctx:    ctx,
		logger: GetLogger().With(args...),
	}
}

// WithError is a convenience function for adding error field to logs.
func WithError(ctx context.Context, err error) *ContextLogger {
	if err == nil {
		return With(ctx)
	}
	return With(ctx, "error", err.Error())
}

// WithError adds an error field to the logger and can be chained.
func (l *ContextLogger) WithError(err error) *ContextLogger {
	if err == nil {
		return l
	}
	return &ContextLogger{
		ctx:    l.ctx,
		logger: l.logger.With("error", err.Error()),
	}
}

// With adds additional temporary fields to the logger and can be chained.
func (l *ContextLogger) With(args ...any) *ContextLogger {
	return &ContextLogger{
		ctx:    l.ctx,
		logger: l.logger.With(args...),
	}
}

func (l *ContextLogger) Debug(msg string) {
	l.logger.DebugContext(l.ctx, msg)
}

func (l *ContextLogger) Info(msg string) {
	l.logger.InfoContext(l.ctx, msg)
}

func (l *ContextLogger) Warn(msg string) {
	l.logger.WarnContext(l.ctx, msg)
}

func (l *ContextLogger) Error(msg string) {
	l.logger.ErrorContext(l.ctx, msg)
}
