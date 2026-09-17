package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	hfl "github.com/openshift-hyperfleet/hyperfleet-logger"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

func captureServiceLogs(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(logger.NewLogger("test", logger.HandlerConfig{
		Level: level, Format: hfl.FormatJSON, Output: &output,
	}))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &output
}

func TestProcessAdapterStatus_ConcurrentReportsRetainLogCorrelation(t *testing.T) {
	setupAdapterStatusDescriptors()
	mockDao := newMockResourceDao()
	svc, _, _, _ := newTestResourceServiceWithAdapterStatus(mockDao)
	resource := testResource("TestResource", "r-1", "test")
	resource.Generation = 1
	mockDao.addResource(resource)
	output := captureServiceLogs(t, slog.LevelDebug)
	parent := hfl.WithTraceID(t.Context(), "trace-1")
	parent = hfl.WithSpanID(parent, "span-1")
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Go(func() {
			<-start
			ctx := hfl.Set(parent, logger.ReqIDKey, fmt.Sprintf("request-%d", i))
			status := testAdapterStatusRequest(99)
			status.Adapter = fmt.Sprintf("adapter-%d", i)
			reportCtx := logger.WithAdapter(ctx, status.Adapter)
			result, err := svc.ProcessAdapterStatus(reportCtx, resource.Kind, resource.ID, status)
			if err != nil || result != nil {
				t.Errorf("future report: result = %v, error = %v", result, err)
			}
			if _, ok := hfl.Get(ctx, logger.AdapterKey); ok {
				t.Error("processing must not change the caller's adapter context")
			}
		})
	}
	close(start)
	wg.Wait()

	lines := bytes.Split(bytes.TrimSpace(output.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("got %d log records, want 2: %s", len(lines), output.String())
	}
	seen := make(map[string]bool)
	for _, line := range lines {
		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatal(err)
		}
		requestID, ok := record["request_id"].(string)
		if !ok {
			t.Fatalf("request_id missing: %s", line)
		}
		seen[requestID] = true
		wantAdapter := map[string]string{"request-0": "adapter-0", "request-1": "adapter-1"}[requestID]
		for key, want := range map[string]string{
			"adapter": wantAdapter, "resource_type": resource.Kind, "resource_id": resource.ID,
			"trace_id": "trace-1", "span_id": "span-1",
			"message": "Discarding adapter status update: future generation",
		} {
			if record[key] != want {
				t.Errorf("%s = %v, want %q", key, record[key], want)
			}
		}
	}
	if !seen["request-0"] || !seen["request-1"] {
		t.Errorf("missing report logs: %v", seen)
	}
}

func TestAdapterDiagnostics_IdentifyAdapterAndClassifyStacks(t *testing.T) {
	output := captureServiceLogs(t, slog.LevelInfo)
	ctx := logger.WithAdapter(t.Context(), "reporting-adapter")
	parsePrevConditions(ctx, []byte("invalid JSON"))
	normalizeAdapterReportsForAggregation(ctx, api.AdapterStatusList{
		makeAdapterStatus("other-adapter", time.Now(), 1, []byte("invalid JSON")),
	}, []string{"other-adapter"}, 1)
	adapterStatusToMapWithUnknownCheck(ctx, &api.AdapterStatus{
		Adapter: "mapper-adapter", Conditions: []byte("invalid JSON"), Data: []byte("invalid JSON"),
	})

	lines := bytes.Split(bytes.TrimSpace(output.Bytes()), []byte("\n"))
	if len(lines) != 4 {
		t.Fatalf("got %d records, want 4: %s", len(lines), output.String())
	}
	for i, line := range lines {
		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatal(err)
		}
		if _, hasStack := record["stack_trace"]; hasStack != (i < 2) {
			t.Errorf("only ERROR diagnostics must include a stack: %s", line)
		}
		wantAdapter := []string{
			"reporting-adapter", "other-adapter", "mapper-adapter", "mapper-adapter",
		}[i]
		if record["adapter"] != wantAdapter || bytes.Count(line, []byte(`"adapter":`)) != 1 {
			t.Errorf("record must identify %q once: %s", wantAdapter, line)
		}
	}
}
