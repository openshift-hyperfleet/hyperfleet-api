package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	hfl "github.com/openshift-hyperfleet/hyperfleet-logger"
)

func TestHandlerAddsAPICorrelationFields(t *testing.T) {
	var output bytes.Buffer
	ctx := hfl.WithTraceID(t.Context(), "trace-1")
	ctx = hfl.WithSpanID(ctx, "span-1")
	ctx = hfl.WithResourceType(ctx, "Cluster")
	ctx = hfl.WithResourceID(ctx, "cluster-1")
	ctx, err := WithRequestID(ctx)
	if err != nil {
		t.Fatal(err)
	}

	NewLogger("test-version", HandlerConfig{
		Level: slog.LevelInfo, Format: hfl.FormatJSON, Output: &output, Hostname: "test-host",
	}).InfoContext(ctx, "cluster updated")

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"component":     Component,
		"version":       "test-version",
		"hostname":      "test-host",
		"trace_id":      "trace-1",
		"span_id":       "span-1",
		"resource_type": "Cluster",
		"resource_id":   "cluster-1",
	} {
		if got := record[key]; got != want {
			t.Errorf("%s = %v, want %q", key, got, want)
		}
	}
	if _, ok := record["request_id"].(string); !ok {
		t.Errorf("request_id = %T, want string", record["request_id"])
	}
}

func TestHandlerSanitizesText(t *testing.T) {
	var output bytes.Buffer
	log := NewLogger("test", HandlerConfig{
		Level: slog.LevelInfo, Format: hfl.FormatText, Output: &output, Hostname: "test-host",
	})
	log.ErrorContext(t.Context(), "expected\nerror")
	if strings.Contains(output.String(), "expected\nerror") {
		t.Fatal("text output must sanitize embedded control characters")
	}
}

func TestHandlerPreservesStackCaptureByFormat(t *testing.T) {
	for _, format := range []struct {
		name  string
		value hfl.Format
	}{
		{name: "json", value: hfl.FormatJSON},
		{name: "text", value: hfl.FormatText},
	} {
		for _, configured := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelError} {
			for _, emitted := range []slog.Level{slog.LevelInfo, slog.LevelWarn, slog.LevelError, slog.LevelError + 4} {
				t.Run(format.name+"/"+configured.String()+"/"+emitted.String(), func(t *testing.T) {
					var output bytes.Buffer
					NewLogger("test", HandlerConfig{
						Level: configured, Format: format.value, Output: &output,
					}).Log(t.Context(), emitted, "test record")
					if emitted < configured {
						if output.Len() != 0 {
							t.Fatal("disabled records must not be emitted")
						}
						return
					}
					hasStack := bytes.Contains(output.Bytes(), []byte("stack_trace"))
					wantStack := format.value == hfl.FormatJSON && emitted >= slog.LevelError
					if hasStack != wantStack {
						t.Errorf("stack present = %v, want %v: %s", hasStack, wantStack, output.String())
					}
				})
			}
		}
	}
}
