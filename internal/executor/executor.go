package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/configloader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleetapi"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/transportclient"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/metrics"
	pkgotel "github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// NewExecutor creates a new Executor with the given configuration
func NewExecutor(config *ExecutorConfig) (*Executor, error) {
	if err := validateExecutorConfig(config); err != nil {
		return nil, err
	}

	return &Executor{
		config:             config,
		precondExecutor:    newPreconditionExecutor(config),
		resourceExecutor:   newResourceExecutor(config),
		postActionExecutor: newPostActionExecutor(config),
		log:                config.Logger,
	}, nil
}

func validateExecutorConfig(config *ExecutorConfig) error {
	if config == nil {
		return fmt.Errorf("config is required")
	}

	if config.Config == nil {
		return fmt.Errorf("config is required")
	}

	requiredFields := []string{
		"APIClient",
		"Logger",
		"TransportClient"}

	for _, field := range requiredFields {
		if reflect.ValueOf(config).Elem().FieldByName(field).IsNil() {
			return fmt.Errorf("field %s is required", field)
		}
	}

	return nil
}

// Execute processes event data according to the adapter configuration
// The caller is responsible for:
// - Adding event ID to context for logging correlation using logger.WithEventID()
func (e *Executor) Execute(ctx context.Context, data interface{}) *ExecutionResult {
	// Start OTel span and add trace context to logs
	ctx, span := e.startTracedExecution(ctx)
	defer span.End()

	// Parse event data
	eventData, rawData, err := ParseEventData(data)
	if err != nil {
		parseErr := fmt.Errorf("failed to parse event data: %w", err)
		errCtx := logger.WithErrorField(ctx, parseErr)
		e.log.Errorf(errCtx, "Failed to parse event data")
		return &ExecutionResult{
			Status:       StatusFailed,
			CurrentPhase: PhaseParamExtraction,
			Errors:       map[ExecutionPhase]error{PhaseParamExtraction: parseErr},
		}
	}

	// This is intended to set OwnerReferences and ResourceID for the event when it exists
	// For example, when a NodePool event arrived
	// the logger will set the cluster_id=owner_id, nodepool_id=resource_id, resource_type=nodepool
	// but when a resource is cluster type, it will just record cluster_id=resource_id
	if eventData.OwnerReferences != nil {
		ctx = logger.WithResourceType(ctx, eventData.Kind)
		ctx = logger.WithDynamicResourceID(ctx, eventData.Kind, eventData.ID)
		ctx = logger.WithDynamicResourceID(ctx, eventData.OwnerReferences.Kind, eventData.OwnerReferences.ID)
	} else {
		ctx = logger.WithDynamicResourceID(ctx, eventData.Kind, eventData.ID)
	}

	execCtx := NewExecutionContext(ctx, rawData, e.config.Config)

	// Initialize execution result
	result := &ExecutionResult{
		Status:       StatusSuccess,
		Params:       make(map[string]interface{}),
		Errors:       make(map[ExecutionPhase]error),
		CurrentPhase: PhaseParamExtraction,
	}

	e.log.Info(ctx, "Processing event")

	// Phase 1: Parameter Extraction
	e.log.Infof(ctx, "Phase %s: RUNNING", result.CurrentPhase)
	if paramErr := e.executeParamExtraction(execCtx); paramErr != nil {
		result.Status = StatusFailed
		result.Errors[PhaseParamExtraction] = paramErr
		execCtx.SetError("ParameterExtractionFailed", paramErr.Error())
		resErr := fmt.Errorf("parameter extraction failed: %w", paramErr)
		errCtx := logger.WithErrorField(ctx, resErr)
		e.log.Errorf(errCtx, "Phase %s: FAILED", PhaseParamExtraction)
		result.ExecutionContext = execCtx
		result.Params = execCtx.Params
		return result
	}
	result.Params = execCtx.Params
	e.log.Debugf(ctx, "Parameter extraction completed: extracted %d params", len(execCtx.Params))

	// Phase 2: Preconditions
	result.CurrentPhase = PhasePreconditions
	preconditions := e.config.Config.Preconditions
	e.log.Infof(ctx, "Phase %s: RUNNING - %d configured", result.CurrentPhase, len(preconditions))
	precondOutcome := e.precondExecutor.ExecuteAll(ctx, preconditions, execCtx)
	result.PreconditionResults = precondOutcome.Results

	switch {
	case precondOutcome.Error != nil:
		// Process execution error: precondition evaluation failed
		result.Status = StatusFailed
		precondErr := fmt.Errorf("precondition evaluation failed: error=%w", precondOutcome.Error)
		result.Errors[result.CurrentPhase] = precondErr
		execCtx.SetError("PreconditionFailed", precondOutcome.Error.Error())
		errCtx := logger.WithErrorField(ctx, precondOutcome.Error)
		e.log.Errorf(errCtx, "Phase %s: FAILED", result.CurrentPhase)
		result.ResourcesSkipped = true
		result.SkipReason = "PreconditionFailed"
		// Set skip metadata on adapter context without overwriting the failed execution status
		// Note: SetSkipped() is NOT called here because it resets ExecutionStatus to "success",
		// which would mask the precondition failure in CEL expressions (e.g., Health condition)
		execCtx.Adapter.ResourcesSkipped = true
		execCtx.Adapter.SkipReason = precondOutcome.Error.Error()
		// Continue to post actions for error reporting
	case !precondOutcome.AllMatched:
		// Business outcome: precondition not satisfied
		result.ResourcesSkipped = true
		result.SkipReason = precondOutcome.NotMetReason
		execCtx.SetSkipped("PreconditionNotMet", precondOutcome.NotMetReason)
		e.log.Infof(ctx, "Phase %s: SUCCESS - NOT_MET - %s", result.CurrentPhase, precondOutcome.NotMetReason)
	default:
		// All preconditions matched
		e.log.Infof(ctx, "Phase %s: SUCCESS - MET - %d passed", result.CurrentPhase, len(precondOutcome.Results))
	}

	// Phase 3: Resources (skip if preconditions not met or previous error)
	result.CurrentPhase = PhaseResources
	resources := e.config.Config.Resources
	e.log.Infof(ctx, "Phase %s: RUNNING - %d configured", result.CurrentPhase, len(resources))
	if !result.ResourcesSkipped {
		resourceResults, resourceErr := e.resourceExecutor.ExecuteAll(ctx, resources, execCtx)
		result.ResourceResults = resourceResults

		if resourceErr != nil {
			result.Status = StatusFailed
			resErr := fmt.Errorf("resource execution failed: %w", resourceErr)
			result.Errors[result.CurrentPhase] = resErr
			execCtx.SetError("ResourceFailed", resourceErr.Error())
			errCtx := logger.WithErrorField(ctx, resourceErr)
			e.log.Errorf(errCtx, "Phase %s: FAILED", result.CurrentPhase)
			// Continue to post actions for error reporting
		} else {
			e.log.Infof(ctx, "Phase %s: SUCCESS - %d processed", result.CurrentPhase, len(resourceResults))
		}
	} else {
		e.log.Infof(ctx, "Phase %s: SKIPPED - %s", result.CurrentPhase, result.SkipReason)
	}

	// Phase 4: Post Actions (always execute for error reporting)
	result.CurrentPhase = PhasePostActions
	postConfig := e.config.Config.Post
	postActionCount := 0
	if postConfig != nil {
		postActionCount = len(postConfig.PostActions)
	}
	e.log.Infof(ctx, "Phase %s: RUNNING - %d configured", result.CurrentPhase, postActionCount)
	postResults, err := e.postActionExecutor.ExecuteAll(ctx, postConfig, execCtx)
	result.PostActionResults = postResults

	if err != nil {
		result.Status = StatusFailed
		postErr := fmt.Errorf("post action execution failed: %w", err)
		result.Errors[result.CurrentPhase] = postErr
		errCtx := logger.WithErrorField(ctx, err)
		e.log.Errorf(errCtx, "Phase %s: FAILED", result.CurrentPhase)
	} else {
		e.log.Infof(ctx, "Phase %s: SUCCESS - %d executed", result.CurrentPhase, len(postResults))
	}

	// Finalize
	result.ExecutionContext = execCtx

	if result.Status == StatusSuccess {
		e.log.Infof(ctx,
			"Event execution finished: event_execution_status=success resources_skipped=%t reason=%s",
			result.ResourcesSkipped, result.SkipReason)
	} else {
		// Combine all errors into a single error for logging
		var errMsgs []string
		for phase, err := range result.Errors {
			errMsgs = append(errMsgs, fmt.Sprintf("%s: %v", phase, err))
		}
		combinedErr := fmt.Errorf("execution failed: %s", strings.Join(errMsgs, "; "))
		errCtx := logger.WithErrorField(ctx, combinedErr)
		e.log.Errorf(errCtx, "Event execution finished: event_execution_status=failed")
	}
	return result
}

// executeParamExtraction extracts parameters from the event and environment
func (e *Executor) executeParamExtraction(execCtx *ExecutionContext) error {
	configMap, err := configToMap(e.config.Config)
	if err != nil {
		return NewExecutorError(PhaseParamExtraction, "config", "failed to marshal config", err)
	}

	// Use a redacted config map for template-accessible params to avoid exposing sensitive
	// values (e.g. TLS cert paths) in rendered manifests or logs.
	redactedMap, err := configToMap(e.config.Config.Redacted())
	if err != nil {
		return NewExecutorError(PhaseParamExtraction, "config", "failed to marshal redacted config", err)
	}

	addAdapterParams(e.config.Config, execCtx, redactedMap)

	// config.* param sources resolve against the real (unredacted) config so that
	// sensitive fields like cert paths can still be explicitly extracted when needed.
	return extractConfigParams(e.config.Config, execCtx, configMap)
}

// startTracedExecution creates an OTel span and adds trace context to logs.
// Returns the enriched context and span. Caller must call span.End() when done.
//
// This method:
//   - Creates an OTel span with trace_id and span_id (for distributed tracing)
//   - Adds trace_id and span_id to logger context (for log correlation)
//   - The trace context is automatically propagated to outgoing HTTP requests
func (e *Executor) startTracedExecution(ctx context.Context) (context.Context, trace.Span) {
	componentName := e.config.Config.Adapter.Name
	ctx, span := otel.Tracer(componentName).Start(ctx, "Execute")

	// Add trace_id and span_id to logger context for log correlation
	ctx = logger.WithOTelTraceContext(ctx)

	return ctx, span
}

// CreateHandler creates a HandlerFunc that executes the adapter task for a given CloudEvent
func (e *Executor) CreateHandler() HandlerFunc {
	return func(ctx context.Context, evt *event.Event) (*ExecutionResult, error) {
		// Add event ID to context for logging correlation
		ctx = logger.WithEventID(ctx, evt.ID())

		// Extract W3C trace context from CloudEvent extensions (if present)
		// This enables distributed tracing when upstream services (e.g., Sentinel)
		// include traceparent/tracestate in the CloudEvent
		ctx = pkgotel.ExtractTraceContextFromCloudEvent(ctx, evt)

		// Log event metadata
		e.log.Infof(ctx, "Event received: id=%s type=%s source=%s time=%s",
			evt.ID(), evt.Type(), evt.Source(), evt.Time())

		result := e.Execute(ctx, evt.Data())

		e.log.Infof(ctx, "Event processed: type=%s source=%s time=%s",
			evt.Type(), evt.Source(), evt.Time())

		return result, nil
	}
}

// ParseEventData parses event data from various input types into structured EventData and raw map.
// Accepts: []byte (JSON), map[string]interface{}, or any JSON-serializable type.
// Returns: structured EventData, raw map for flexible access, and any error.
func ParseEventData(data interface{}) (*EventData, map[string]interface{}, error) {
	if data == nil {
		return &EventData{}, make(map[string]interface{}), nil
	}

	var jsonBytes []byte
	var err error

	switch v := data.(type) {
	case []byte:
		if len(v) == 0 {
			return &EventData{}, make(map[string]interface{}), nil
		}
		jsonBytes = v
	case map[string]interface{}:
		// Already a map, marshal to JSON for struct conversion
		jsonBytes, err = json.Marshal(v)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal map data: error=%w", err)
		}
	default:
		// Try to marshal any other type
		jsonBytes, err = json.Marshal(v)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal data: error=%w", err)
		}
	}

	// Parse into structured EventData
	var eventData EventData
	if err := json.Unmarshal(jsonBytes, &eventData); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal to EventData: error=%w", err)
	}

	// Parse into raw map for flexible access
	var rawData map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &rawData); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal to map: error=%w", err)
	}

	return &eventData, rawData, nil
}

// ExecutorBuilder provides a fluent interface for building an Executor
type ExecutorBuilder struct {
	config *ExecutorConfig
}

// NewBuilder creates a new ExecutorBuilder
func NewBuilder() *ExecutorBuilder {
	return &ExecutorBuilder{
		config: &ExecutorConfig{},
	}
}

// WithConfig sets the unified configuration
func (b *ExecutorBuilder) WithConfig(config *configloader.Config) *ExecutorBuilder {
	b.config.Config = config
	return b
}

// WithAPIClient sets the HyperFleet API client
func (b *ExecutorBuilder) WithAPIClient(client hyperfleetapi.Client) *ExecutorBuilder {
	b.config.APIClient = client
	return b
}

// WithTransportClient sets the transport client for resource application (kubernetes or maestro)
func (b *ExecutorBuilder) WithTransportClient(client transportclient.TransportClient) *ExecutorBuilder {
	b.config.TransportClient = client
	return b
}

// WithLogger sets the logger
func (b *ExecutorBuilder) WithLogger(log logger.Logger) *ExecutorBuilder {
	b.config.Logger = log
	return b
}

// WithMetricsRecorder sets the optional Prometheus metrics recorder
func (b *ExecutorBuilder) WithMetricsRecorder(recorder *metrics.Recorder) *ExecutorBuilder {
	b.config.MetricsRecorder = recorder
	return b
}

// Build creates the Executor
func (b *ExecutorBuilder) Build() (*Executor, error) {
	return NewExecutor(b.config)
}
