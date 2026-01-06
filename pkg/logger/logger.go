package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// Global logger configuration
var (
	globalLogger *slog.Logger
	globalMu     sync.RWMutex
	initOnce     sync.Once

	// Build info set at initialization
	component = "api"
	version   = "unknown"
	hostname  = "unknown"
)

// Config holds the logger configuration
type Config struct {
	Level   string // debug, info, warn, error
	Format  string // text, json
	Output  string // stdout, stderr
	Version string // application version
}

// Initialize sets up the global logger with the given configuration
func Initialize(cfg Config) {
	globalMu.Lock()
	defer globalMu.Unlock()

	// Set version
	if cfg.Version != "" {
		version = cfg.Version
	}

	// Get hostname
	if h, err := os.Hostname(); err == nil {
		hostname = h
	}

	// Determine output writer
	var output io.Writer
	switch strings.ToLower(cfg.Output) {
	case "stderr":
		output = os.Stderr
	default:
		output = os.Stdout
	}

	// Determine log level
	var level slog.Level
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	// Create handler options
	opts := &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Rename time to timestamp for HyperFleet standard
			if a.Key == slog.TimeKey {
				a.Key = "timestamp"
			}
			// Rename msg to message for HyperFleet standard
			if a.Key == slog.MessageKey {
				a.Key = "message"
			}
			return a
		},
	}

	// Create handler based on format
	var handler slog.Handler
	switch strings.ToLower(cfg.Format) {
	case "json":
		handler = slog.NewJSONHandler(output, opts)
	default:
		handler = slog.NewTextHandler(output, opts)
	}

	// Wrap handler to add default attributes
	handler = &contextHandler{
		Handler: handler,
	}

	// Create and set global logger with base attributes
	globalLogger = slog.New(handler).With(
		slog.String("component", component),
		slog.String("version", version),
		slog.String("hostname", hostname),
	)

	// Set as default logger
	slog.SetDefault(globalLogger)
}

// contextHandler wraps a slog.Handler to add context-based attributes
type contextHandler struct {
	slog.Handler
}

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	// Add trace_id if present in context (OpenTelemetry)
	if traceID := GetTraceID(ctx); traceID != "" {
		r.AddAttrs(slog.String("trace_id", traceID))
	}

	// Add span_id if present in context (OpenTelemetry)
	if spanID := GetSpanID(ctx); spanID != "" {
		r.AddAttrs(slog.String("span_id", spanID))
	}

	// Add request_id if present in context
	if requestID := GetRequestID(ctx); requestID != "" {
		r.AddAttrs(slog.String("request_id", requestID))
	}

	// Add operation_id if present in context
	if opID := GetOperationID(ctx); opID != "" {
		r.AddAttrs(slog.String("operation_id", opID))
	}

	return h.Handler.Handle(ctx, r)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithGroup(name)}
}

// Default returns the global logger, initializing with defaults if necessary
func Default() *slog.Logger {
	globalMu.RLock()
	l := globalLogger
	globalMu.RUnlock()

	if l == nil {
		// Use sync.Once to ensure atomic initialization
		initOnce.Do(func() {
			Initialize(Config{
				Level:  "info",
				Format: "text",
				Output: "stdout",
			})
		})
		globalMu.RLock()
		l = globalLogger
		globalMu.RUnlock()
	}
	return l
}

// With returns a logger with the given attributes
func With(args ...any) *slog.Logger {
	return Default().With(args...)
}

// OCMLogger is the legacy interface for backward compatibility
type OCMLogger interface {
	V(level int32) OCMLogger
	Infof(format string, args ...interface{})
	Extra(key string, value interface{}) OCMLogger
	Info(message string)
	Warning(message string)
	Error(message string)
	Fatal(message string)
}

var _ OCMLogger = &legacyLogger{}

type legacyLogger struct {
	ctx    context.Context
	logger *slog.Logger
	level  int32
	attrs  []any
}

// NewOCMLogger creates a new logger instance with a default verbosity of 1
// This maintains backward compatibility with existing code
func NewOCMLogger(ctx context.Context) OCMLogger {
	l := Default()

	// Add context-based attributes that are not handled by contextHandler
	// Note: operation_id is added by contextHandler.Handle to avoid duplication
	attrs := []any{}

	if accountID := GetAccountID(ctx); accountID != "" {
		attrs = append(attrs, slog.String("account_id", accountID))
	}

	if txid := GetTransactionID(ctx); txid != 0 {
		attrs = append(attrs, slog.Int64("tx_id", txid))
	}

	if len(attrs) > 0 {
		l = l.With(attrs...)
	}

	return &legacyLogger{
		ctx:    ctx,
		logger: l,
		level:  1,
		attrs:  []any{},
	}
}

func (l *legacyLogger) V(level int32) OCMLogger {
	return &legacyLogger{
		ctx:    l.ctx,
		logger: l.logger,
		level:  level,
		attrs:  l.attrs,
	}
}

func (l *legacyLogger) Infof(format string, args ...interface{}) {
	// V() levels > 1 are treated as debug
	if l.level > 1 {
		l.logger.With(l.attrs...).DebugContext(l.ctx, sprintf(format, args...))
	} else {
		l.logger.With(l.attrs...).InfoContext(l.ctx, sprintf(format, args...))
	}
}

func (l *legacyLogger) Extra(key string, value interface{}) OCMLogger {
	newAttrs := make([]any, len(l.attrs), len(l.attrs)+2)
	copy(newAttrs, l.attrs)
	newAttrs = append(newAttrs, slog.Any(key, value))

	return &legacyLogger{
		ctx:    l.ctx,
		logger: l.logger,
		level:  l.level,
		attrs:  newAttrs,
	}
}

func (l *legacyLogger) Info(message string) {
	l.logger.With(l.attrs...).InfoContext(l.ctx, message)
}

func (l *legacyLogger) Warning(message string) {
	l.logger.With(l.attrs...).WarnContext(l.ctx, message)
}

func (l *legacyLogger) Error(message string) {
	l.logger.With(l.attrs...).ErrorContext(l.ctx, message)
}

func (l *legacyLogger) Fatal(message string) {
	l.logger.With(l.attrs...).ErrorContext(l.ctx, message)
	os.Exit(1)
}

// sprintf is a helper for formatting strings
func sprintf(format string, args ...interface{}) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}
