package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"time"
)

// HyperFleetTextHandler implements the HyperFleet Logging Specification text format:
// {timestamp} {LEVEL} [{component}] [{version}] [{hostname}] {message} {key=value}...
//
// Example output:
// 2026-01-09T12:30:45Z INFO [hyperfleet-api] [v1.2.3] [pod-abc] Processing request request_id=xyz cluster_id=abc123
type HyperFleetTextHandler struct {
	w         io.Writer
	component string
	version   string
	hostname  string
	level     slog.Level
	attrs     []slog.Attr
	mu        sync.Mutex
}

// NewHyperFleetTextHandler creates a new text handler conforming to HyperFleet Logging Specification
func NewHyperFleetTextHandler(w io.Writer, component, version, hostname string, level slog.Level) *HyperFleetTextHandler {
	return &HyperFleetTextHandler{
		w:         w,
		component: component,
		version:   version,
		hostname:  hostname,
		level:     level,
	}
}

// Enabled reports whether the handler handles records at the given level
func (h *HyperFleetTextHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *HyperFleetTextHandler) Handle(ctx context.Context, r slog.Record) error {
	var buf strings.Builder

	buf.WriteString(r.Time.Format(time.RFC3339))
	buf.WriteByte(' ')
	buf.WriteString(strings.ToUpper(r.Level.String()))
	buf.WriteByte(' ')
	fmt.Fprintf(&buf, "[%s] [%s] [%s] ", h.component, h.version, h.hostname)
	buf.WriteString(r.Message)

	for _, field := range ContextFieldsRegistry {
		if val, ok := field.Getter(ctx); ok {
			fmt.Fprintf(&buf, " %s=%s", field.Name, formatValue(val))
		}
	}

	for _, attr := range h.attrs {
		fmt.Fprintf(&buf, " %s=%s", attr.Key, formatValue(attr.Value.Any()))
	}

	var stackTrace []string
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "stack_trace" {
			if frames, ok := a.Value.Any().([]runtime.Frame); ok {
				stackTrace = formatStackTrace(frames)
				return true
			}
		}
		fmt.Fprintf(&buf, " %s=%s", a.Key, formatValue(a.Value.Any()))
		return true
	})

	buf.WriteByte('\n')

	if len(stackTrace) > 0 {
		buf.WriteString("  stack_trace:\n")
		for _, frame := range stackTrace {
			fmt.Fprintf(&buf, "    %s\n", frame)
		}
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.w.Write([]byte(buf.String()))
	return err
}

func (h *HyperFleetTextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)

	return &HyperFleetTextHandler{
		w:         h.w,
		component: h.component,
		version:   h.version,
		hostname:  h.hostname,
		level:     h.level,
		attrs:     newAttrs,
	}
}

// WithGroup returns a new handler with a group name
func (h *HyperFleetTextHandler) WithGroup(name string) slog.Handler {
	// For simplicity, return self (groups not needed for HyperFleet spec)
	return h
}

func formatValue(v interface{}) string {
	if v == nil {
		return "null"
	}

	str := fmt.Sprintf("%v", v)
	if strings.ContainsAny(str, " \t\n\"") {
		str = strings.ReplaceAll(str, `"`, `\"`)
		return fmt.Sprintf(`"%s"`, str)
	}
	return str
}

func formatStackTrace(frames []runtime.Frame) []string {
	result := make([]string, 0, len(frames))
	for _, frame := range frames {
		result = append(result, fmt.Sprintf("%s:%d %s", frame.File, frame.Line, frame.Function))
	}
	return result
}
