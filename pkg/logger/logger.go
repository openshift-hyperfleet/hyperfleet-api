package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"

	"github.com/golang/glog"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/util"
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

// NewHyperFleetHandler creates a custom slog handler with HyperFleet-specific fields
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
				level := a.Value.Any().(slog.Level)
				return slog.Attr{Key: "level", Value: slog.StringValue(strings.ToLower(level.String()))}
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

	if traceID, ok := ctx.Value(TraceIDCtxKey).(string); ok && traceID != "" {
		r.AddAttrs(slog.String("trace_id", traceID))
	}
	if spanID, ok := ctx.Value(SpanIDCtxKey).(string); ok && spanID != "" {
		r.AddAttrs(slog.String("span_id", spanID))
	}
	if requestID, ok := ctx.Value(OpIDKey).(string); ok && requestID != "" {
		r.AddAttrs(slog.String("request_id", requestID))
	}
	if clusterID, ok := ctx.Value(ClusterIDCtxKey).(string); ok && clusterID != "" {
		r.AddAttrs(slog.String("cluster_id", clusterID))
	}
	if resourceType, ok := ctx.Value(ResourceTypeCtxKey).(string); ok && resourceType != "" {
		r.AddAttrs(slog.String("resource_type", resourceType))
	}
	if resourceID, ok := ctx.Value(ResourceIDCtxKey).(string); ok && resourceID != "" {
		r.AddAttrs(slog.String("resource_id", resourceID))
	}

	if r.Level >= slog.LevelError {
		stackTrace := captureStackTrace(4)
		r.AddAttrs(slog.Any("stack_trace", stackTrace))
		r.AddAttrs(slog.String("error", r.Message))
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

var globalLogger *slog.Logger

// InitGlobalLogger initializes the global logger with the given configuration
func InitGlobalLogger(cfg *LogConfig) {
	handler := NewHyperFleetHandler(cfg)
	globalLogger = slog.New(handler)
	slog.SetDefault(globalLogger)
}

// GetLogger returns a context-aware logger
func GetLogger(ctx context.Context) *slog.Logger {
	if globalLogger == nil {
		return slog.Default()
	}
	return globalLogger
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
	GetLogger(ctx).DebugContext(ctx, msg, args...)
}

func Info(ctx context.Context, msg string, args ...any) {
	GetLogger(ctx).InfoContext(ctx, msg, args...)
}

func Warn(ctx context.Context, msg string, args ...any) {
	GetLogger(ctx).WarnContext(ctx, msg, args...)
}

func Error(ctx context.Context, msg string, args ...any) {
	GetLogger(ctx).ErrorContext(ctx, msg, args...)
}

func Debugf(ctx context.Context, format string, args ...interface{}) {
	GetLogger(ctx).DebugContext(ctx, fmt.Sprintf(format, args...))
}

func Infof(ctx context.Context, format string, args ...interface{}) {
	GetLogger(ctx).InfoContext(ctx, fmt.Sprintf(format, args...))
}

func Warnf(ctx context.Context, format string, args ...interface{}) {
	GetLogger(ctx).WarnContext(ctx, fmt.Sprintf(format, args...))
}

func Errorf(ctx context.Context, format string, args ...interface{}) {
	GetLogger(ctx).ErrorContext(ctx, fmt.Sprintf(format, args...))
}

// Legacy glog-based logger for backward compatibility
// DEPRECATED: Will be removed in PR 3
type OCMLogger interface {
	V(level int32) OCMLogger
	Infof(format string, args ...interface{})
	Extra(key string, value interface{}) OCMLogger
	Info(message string)
	Warning(message string)
	Error(message string)
	Fatal(message string)
}

var _ OCMLogger = &ocmLogger{}

type extra map[string]interface{}

type ocmLogger struct {
	context   context.Context
	level     int32
	accountID string
	username  string
	extra     extra
}

// NewOCMLogger creates a legacy logger instance
// DEPRECATED: Use slog-based logger instead
func NewOCMLogger(ctx context.Context) OCMLogger {
	logger := &ocmLogger{
		context:   ctx,
		level:     1,
		extra:     make(extra),
		accountID: util.GetAccountIDFromContext(ctx),
	}
	return logger
}

func (l *ocmLogger) prepareLogPrefix(message string, extra extra) string {
	prefix := " "

	if txid, ok := l.context.Value("txid").(int64); ok {
		prefix = fmt.Sprintf("[tx_id=%d]%s", txid, prefix)
	}

	if l.accountID != "" {
		prefix = fmt.Sprintf("[accountID=%s]%s", l.accountID, prefix)
	}

	if opid, ok := l.context.Value(OpIDKey).(string); ok {
		prefix = fmt.Sprintf("[opid=%s]%s", opid, prefix)
	}

	var args []string
	for k, v := range extra {
		args = append(args, fmt.Sprintf("%s=%v", k, v))
	}

	return fmt.Sprintf("%s %s %s", prefix, message, strings.Join(args, " "))
}

func (l *ocmLogger) prepareLogPrefixf(format string, args ...interface{}) string {
	orig := fmt.Sprintf(format, args...)
	prefix := " "

	if txid, ok := l.context.Value("txid").(int64); ok {
		prefix = fmt.Sprintf("[tx_id=%d]%s", txid, prefix)
	}

	if l.accountID != "" {
		prefix = fmt.Sprintf("[accountID=%s]%s", l.accountID, prefix)
	}

	if opid, ok := l.context.Value(OpIDKey).(string); ok {
		prefix = fmt.Sprintf("[opid=%s]%s", opid, prefix)
	}

	return fmt.Sprintf("%s%s", prefix, orig)
}

func (l *ocmLogger) V(level int32) OCMLogger {
	return &ocmLogger{
		context:   l.context,
		accountID: l.accountID,
		username:  l.username,
		level:     level,
	}
}

func (l *ocmLogger) Infof(format string, args ...interface{}) {
	prefixed := l.prepareLogPrefixf(format, args...)
	glog.V(glog.Level(l.level)).Infof("%s", prefixed)
}

func (l *ocmLogger) Extra(key string, value interface{}) OCMLogger {
	l.extra[key] = value
	return l
}

func (l *ocmLogger) Info(message string) {
	l.log(message, glog.V(glog.Level(l.level)).Infoln)
}

func (l *ocmLogger) Warning(message string) {
	l.log(message, glog.Warningln)
}

func (l *ocmLogger) Error(message string) {
	l.log(message, glog.Errorln)
}

func (l *ocmLogger) Fatal(message string) {
	l.log(message, glog.Fatalln)
}

func (l *ocmLogger) log(message string, glogFunc func(args ...interface{})) {
	prefixed := l.prepareLogPrefix(message, l.extra)
	glogFunc(prefixed)
}
