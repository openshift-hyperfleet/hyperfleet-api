package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/configloader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/criteria"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleetapi"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/manifest"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/transportclient"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ExecutionPhase represents which phase of execution
type ExecutionPhase string

const (
	// PhaseParamExtraction is the parameter extraction phase
	PhaseParamExtraction ExecutionPhase = "param_extraction"
	// PhasePreconditions is the precondition evaluation phase
	PhasePreconditions ExecutionPhase = "preconditions"
	// PhaseResources is the resource creation/update phase
	PhaseResources ExecutionPhase = "resources"
	// PhasePostActions is the post-action execution phase
	PhasePostActions ExecutionPhase = "post_actions"
)

// ExecutionStatus represents the status of execution (runtime perspective)
type ExecutionStatus string

const (
	// StatusSuccess indicates successful execution (adapter ran successfully)
	StatusSuccess ExecutionStatus = "success"
	// StatusFailed indicates failed execution (process execution error: API timeout, parse error, K8s error, etc.)
	StatusFailed ExecutionStatus = "failed"
	// StatusSkipped indicates the action was intentionally skipped (e.g. when condition evaluated to false)
	StatusSkipped ExecutionStatus = "skipped"
)

// ResourceRef represents a reference to a HyperFleet resource
type ResourceRef struct {
	ID   string `json:"id,omitempty"`
	Kind string `json:"kind,omitempty"`
	Href string `json:"href,omitempty"`
}

// EventData represents the data payload of a HyperFleet CloudEvent
type EventData struct {
	OwnerReferences *ResourceRef `json:"owner_references,omitempty"`
	ID              string       `json:"id,omitempty"`
	Kind            string       `json:"kind,omitempty"`
	Href            string       `json:"href,omitempty"`
	Generation      int64        `json:"generation,omitempty"`
}

// ExecutorConfig holds configuration for the executor
type ExecutorConfig struct {
	// Config is the unified configuration (merged from deployment and task configs)
	Config *configloader.Config
	// APIClient is the HyperFleet API client
	APIClient hyperfleetapi.Client
	// TransportClient is the transport client for applying resources (kubernetes or maestro)
	TransportClient transportclient.TransportClient
	// Logger is the logger instance
	Logger logger.Logger
}

// Executor processes CloudEvents according to the adapter configuration
type Executor struct {
	config             *ExecutorConfig
	precondExecutor    *PreconditionExecutor
	resourceExecutor   *ResourceExecutor
	postActionExecutor *PostActionExecutor
	log                logger.Logger
}

// ExecutionResult contains the result of processing an event
type ExecutionResult struct {
	// ExecutionContext contains the full execution context (for testing and debugging)
	ExecutionContext *ExecutionContext
	// Params contains the extracted parameters
	Params map[string]interface{}
	// Errors contains errors keyed by the phase where they occurred
	Errors map[ExecutionPhase]error
	// SkipReason is why resources were skipped (e.g., "precondition not met")
	SkipReason string
	// Status is the overall execution status (runtime perspective)
	Status ExecutionStatus
	// CurrentPhase is the phase where execution ended (or is currently)
	CurrentPhase ExecutionPhase
	// PreconditionResults contains results of precondition evaluations
	PreconditionResults []PreconditionResult
	// ResourceResults contains results of resource operations
	ResourceResults []ResourceResult
	// PostActionResults contains results of post-action executions
	PostActionResults []PostActionResult
	// ResourcesSkipped indicates if resources were skipped (business outcome)
	ResourcesSkipped bool
}

// PreconditionResult contains the result of a single precondition evaluation
type PreconditionResult struct {
	// Error is the error if Status is StatusFailed
	Error error
	// CapturedFields contains fields captured from the API response
	CapturedFields map[string]interface{}
	// CELResult contains CEL evaluation result (if expression was used)
	CELResult *criteria.CELResult
	// Name is the precondition name
	Name string
	// Status is the result status
	Status ExecutionStatus
	// APIResponse contains the raw API response (if APICallMade)
	APIResponse []byte
	// ConditionResults contains individual condition evaluation results
	ConditionResults []criteria.EvaluationResult
	// Matched indicates if conditions were satisfied
	Matched bool
	// APICallMade indicates if an API call was made
	APICallMade bool
}

// ResourceResult contains the result of a single resource operation
type ResourceResult struct {
	// Error is the error if Status is StatusFailed
	Error error
	// DiscoveredState holds the resource state found before a delete operation.
	// Only populated for Operation == OperationDelete.
	DiscoveredState *unstructured.Unstructured
	// Name is the resource name from config
	Name string
	// Kind is the Kubernetes resource kind
	Kind string
	// Namespace is the resource namespace
	Namespace string
	// ResourceName is the actual K8s resource name
	ResourceName string
	// OperationReason explains why this operation was performed
	// Examples: "resource not found", "generation changed from 1 to 2",
	// "generation 1 unchanged", "recreate_on_change=true"
	OperationReason string
	// Status is the result status
	Status ExecutionStatus
	// Operation is the operation performed (create, update, recreate, skip, delete)
	Operation manifest.Operation
}

// PostActionResult contains the result of a single post-action execution
type PostActionResult struct {
	// Error is the error if Status is StatusFailed
	Error error
	// Name is the post-action name
	Name string
	// SkipReason is the reason for skipping
	SkipReason string
	// Status is the result status
	Status ExecutionStatus
	// APIResponse contains the raw API response (if APICallMade)
	APIResponse []byte
	// HTTPStatus is the HTTP status code of the API response
	HTTPStatus int
	// Skipped indicates if the action was skipped due to when condition
	Skipped bool
	// APICallMade indicates if an API call was made
	APICallMade bool
}

// ExecutionContext holds runtime context during execution
type ExecutionContext struct {
	// Ctx is the Go context
	Ctx context.Context
	// Config is the unified adapter configuration
	Config *configloader.Config
	// EventData is the parsed event data payload
	EventData map[string]interface{}
	// Params holds extracted parameters and captured fields
	// - Populated during param extraction phase with event/env data
	// - Populated during precondition phase with captured API response fields
	Params map[string]interface{}
	// Resources holds discovered resources keyed by resource name.
	// Nested discoveries are also added as top-level entries keyed by nested discovery name.
	// Values are expected to be *unstructured.Unstructured.
	Resources map[string]interface{}
	// Evaluations tracks all condition evaluations for debugging/auditing
	Evaluations []EvaluationRecord
	// Adapter holds adapter execution metadata
	Adapter AdapterMetadata
}

// EvaluationRecord tracks a single condition evaluation during execution
type EvaluationRecord struct {
	// FieldResults contains individual field evaluation results keyed by field path (for structured conditions)
	// Reuses criteria.EvaluationResult to avoid duplication
	FieldResults map[string]criteria.EvaluationResult
	// Timestamp is when the evaluation occurred
	Timestamp time.Time
	// Name is the name of the precondition/resource/action being evaluated
	Name string
	// Expression is the CEL expression or condition description
	Expression string
	// Phase is the execution phase where this evaluation occurred
	Phase ExecutionPhase
	// EvaluationType indicates what kind of evaluation was performed
	EvaluationType EvaluationType
	// Matched indicates whether the evaluation succeeded
	Matched bool
}

// EvaluationType indicates the type of evaluation performed
type EvaluationType string

const (
	// EvaluationTypeCEL indicates a CEL expression evaluation
	EvaluationTypeCEL EvaluationType = "cel"
	// EvaluationTypeConditions indicates structured conditions evaluation
	EvaluationTypeConditions EvaluationType = "conditions"
)

// AdapterMetadata holds adapter execution metadata for CEL expressions
type AdapterMetadata struct {
	// ExecutionError contains detailed error information for the first failure across all phases.
	// This is the primary error signal — post-action CEL expressions use adapter.?executionError
	// to detect any failure and surface a human-readable message.
	ExecutionError *ExecutionError `json:"executionError,omitempty"`
	// ResourceErrors holds per-resource error details from the resources phase.
	// Keyed by resource name. Populated alongside ExecutionError so that adapter authors
	// who need granular per-resource failure details can access them via
	// adapter.?resourceErrors.?myResource.?message without replacing the top-level signal.
	ResourceErrors map[string]ExecutionError `json:"resourceErrors,omitempty"`
	// ExecutionStatus is the overall execution status (runtime perspective: "success", "failed")
	ExecutionStatus string
	// ErrorReason is the error reason if failed (process execution errors only)
	ErrorReason string
	// ErrorMessage is the error message if failed (process execution errors only)
	ErrorMessage string
	// SkipReason is why resources were skipped (e.g., "precondition not met")
	SkipReason string `json:"skipReason,omitempty"`
	// ResourcesSkipped indicates if resources were skipped (business outcome)
	ResourcesSkipped bool `json:"resourcesSkipped,omitempty"`
}

// ExecutionError represents a structured execution error
type ExecutionError struct {
	// Phase is the execution phase where the error occurred
	Phase string `json:"phase"`
	// Step is the specific step (precondition/resource/action name) that failed
	Step string `json:"step"`
	// Message is the error message (includes all relevant details)
	Message string `json:"message"`
}

// NewExecutionContext creates a new execution context
func NewExecutionContext(
	ctx context.Context,
	eventData map[string]interface{},
	config *configloader.Config,
) *ExecutionContext {
	return &ExecutionContext{
		Ctx:         ctx,
		Config:      config,
		EventData:   eventData,
		Params:      make(map[string]interface{}),
		Resources:   make(map[string]interface{}),
		Evaluations: make([]EvaluationRecord, 0),
		Adapter: AdapterMetadata{
			ExecutionStatus: string(StatusSuccess),
		},
	}
}

// AddEvaluation records a condition evaluation result
func (ec *ExecutionContext) AddEvaluation(
	phase ExecutionPhase,
	name string,
	evalType EvaluationType,
	expression string,
	matched bool,
	fieldResults map[string]criteria.EvaluationResult,
) {
	ec.Evaluations = append(ec.Evaluations, EvaluationRecord{
		Phase:          phase,
		Name:           name,
		EvaluationType: evalType,
		Expression:     expression,
		Matched:        matched,
		FieldResults:   fieldResults,
		Timestamp:      time.Now(),
	})
}

// AddCELEvaluation is a convenience method for recording CEL expression evaluations
func (ec *ExecutionContext) AddCELEvaluation(phase ExecutionPhase, name, expression string, matched bool) {
	ec.AddEvaluation(phase, name, EvaluationTypeCEL, expression, matched, nil)
}

// AddConditionsEvaluation is a convenience method for recording structured conditions evaluations
func (ec *ExecutionContext) AddConditionsEvaluation(
	phase ExecutionPhase,
	name string,
	matched bool,
	fieldResults map[string]criteria.EvaluationResult,
) {
	ec.AddEvaluation(phase, name, EvaluationTypeConditions, "", matched, fieldResults)
}

// GetEvaluationsByPhase returns all evaluations for a specific phase
func (ec *ExecutionContext) GetEvaluationsByPhase(phase ExecutionPhase) []EvaluationRecord {
	var results []EvaluationRecord
	for _, eval := range ec.Evaluations {
		if eval.Phase == phase {
			results = append(results, eval)
		}
	}
	return results
}

// GetFailedEvaluations returns all evaluations that did not match
func (ec *ExecutionContext) GetFailedEvaluations() []EvaluationRecord {
	var results []EvaluationRecord
	for _, eval := range ec.Evaluations {
		if !eval.Matched {
			results = append(results, eval)
		}
	}
	return results
}

// SetError sets the error status in adapter metadata (for runtime failures).
// ExecutionError is only written when not already populated — callers like
// recordResourceError set a richer error (with Phase and Step) first, and
// SetError must not overwrite it with a coarser-grained fallback.
func (ec *ExecutionContext) SetError(reason, message string) {
	ec.Adapter.ExecutionStatus = string(StatusFailed)
	ec.Adapter.ErrorReason = reason
	ec.Adapter.ErrorMessage = message
	if ec.Adapter.ExecutionError == nil {
		ec.Adapter.ExecutionError = &ExecutionError{
			Phase:   reason,
			Message: message,
		}
	}
}

// SetSkipped sets the status to indicate execution was skipped (not an error)
func (ec *ExecutionContext) SetSkipped(reason, message string) {
	// Execution was successful, but resources were skipped due to business logic
	ec.Adapter.ExecutionStatus = string(StatusSuccess)
	ec.Adapter.ResourcesSkipped = true
	ec.Adapter.SkipReason = reason
	if message != "" {
		ec.Adapter.SkipReason = message // Use message if provided for more detail
	}
}

// GetCELVariables returns all variables for CEL evaluation.
// This includes Params, adapter metadata, and resources.
func (ec *ExecutionContext) GetCELVariables() map[string]interface{} {
	result := make(map[string]interface{})

	// Copy all params
	for k, v := range ec.Params {
		result[k] = v
	}

	// Add adapter metadata (use helper from utils.go)
	result["adapter"] = adapterMetadataToMap(&ec.Adapter)

	// Add resources (convert unstructured to maps for CEL evaluation).
	// Deleted resources (nil sentinels) are intentionally omitted from this map.
	// A missing key returns Optional.none() via optional access, so
	// "!resources.?clusterJob.hasValue()" correctly evaluates to true when a
	// resource is confirmed deleted. Use "adapter.?resourcesSkipped.orValue(false)"
	// to distinguish "deleted" from "never processed" in Finalized conditions.
	resources := make(map[string]interface{})
	for name, val := range ec.Resources {
		if val == nil {
			continue // deleted resources are absent from CEL; resources.?X.hasValue() returns false
		}
		switch v := val.(type) {
		case *unstructured.Unstructured:
			if v != nil {
				resources[name] = v.Object
			}
		case map[string]*unstructured.Unstructured:
			nested := make(map[string]interface{})
			for nestedName, nestedRes := range v {
				if nestedRes != nil {
					nested[nestedName] = nestedRes.Object
				}
			}
			resources[name] = nested
		}
	}
	result["resources"] = resources

	return result
}

// ExecutorError represents an error during execution
type ExecutorError struct {
	Err     error
	Phase   ExecutionPhase
	Step    string
	Message string
}

func (e *ExecutorError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %s: %v", e.Phase, e.Step, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s: %s", e.Phase, e.Step, e.Message)
}

func (e *ExecutorError) Unwrap() error {
	return e.Err
}

// NewExecutorError creates a new executor error
func NewExecutorError(phase ExecutionPhase, step, message string, err error) *ExecutorError {
	return &ExecutorError{
		Phase:   phase,
		Step:    step,
		Message: message,
		Err:     err,
	}
}

// PreconditionsOutcome represents the high-level result of precondition evaluation
type PreconditionsOutcome struct {
	// Error contains execution errors (API failures, parse errors, etc.)
	// nil if preconditions were evaluated successfully, even if not matched
	Error error
	// NotMetReason provides details when AllMatched is false
	NotMetReason string
	// Results contains individual precondition results
	Results []PreconditionResult
	// AllMatched indicates whether all preconditions were satisfied (business outcome)
	AllMatched bool
}
