package executor

import (
	"context"
	"time"

	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/metrics"
)

// HandlerFunc is a composable event handler. Build a broker-compatible handler with:
//
//	handler := AlwaysAck(WithMetrics(exec.CreateHandler(), metricsRecorder, log), log)
type HandlerFunc func(ctx context.Context, evt *event.Event) (*ExecutionResult, error)

// WithMetrics wraps a HandlerFunc to record Prometheus metrics after execution.
// A panic in metrics recording is recovered to prevent crashing the handler.
// If recorder is nil, the handler is returned unwrapped.
func WithMetrics(h HandlerFunc, recorder *metrics.Recorder, log logger.Logger) HandlerFunc {
	if recorder == nil {
		return h
	}
	return func(ctx context.Context, evt *event.Event) (*ExecutionResult, error) {
		start := time.Now()
		result, err := h(ctx, evt)
		duration := time.Since(start)

		resultForMetrics := result
		if err != nil && (result == nil || result.Status == StatusSuccess) {
			resultForMetrics = &ExecutionResult{Status: StatusFailed}
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Errorf(ctx, "panic in metrics recording (recovered): %v", r)
				}
			}()
			recordMetrics(recorder, resultForMetrics, duration)
		}()

		return result, err
	}
}

// AlwaysAck wraps a HandlerFunc into a broker compatible handler that always returns nil,
// preventing infinite retry loops for non-recoverable errors.
// Errors are logged at warn level before being discarded.
func AlwaysAck(h HandlerFunc, log logger.Logger) func(ctx context.Context, evt *event.Event) error {
	return func(ctx context.Context, evt *event.Event) error {
		result, err := h(ctx, evt)
		errCtx := logger.WithLogFields(ctx, logger.LogFields{
			"event_id":   evt.ID(),
			"event_type": evt.Type(),
		})
		if err != nil {
			errCtx = logger.WithErrorField(errCtx, err)
			log.Warn(errCtx, "event handler error (acked)")
		} else if result != nil && result.Status == StatusFailed {
			phases := make([]string, 0, len(result.Errors))
			for phase := range result.Errors {
				phases = append(phases, string(phase))
			}
			errCtx = logger.WithLogField(errCtx, "failed_phases", phases)
			log.Warn(errCtx, "event handler failed (acked)")
		}
		return nil
	}
}

// recordMetrics records Prometheus metrics based on the execution result.
func recordMetrics(recorder *metrics.Recorder, result *ExecutionResult, duration time.Duration) {
	if recorder == nil {
		return
	}

	recorder.ObserveProcessingDuration(duration)

	if result == nil {
		return
	}

	switch {
	case result.Status == StatusFailed:
		recorder.RecordEventProcessed("failed")
		for phase := range result.Errors {
			recorder.RecordError(string(phase))
		}
	case result.ResourcesSkipped:
		recorder.RecordEventProcessed("skipped")
	default:
		recorder.RecordEventProcessed("success")
	}
}
