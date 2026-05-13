package executor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/configloader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/criteria"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleetapi"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/utils"
)

// PostActionExecutor executes post-processing actions
type PostActionExecutor struct {
	apiClient hyperfleetapi.Client
	log       logger.Logger
}

// newPostActionExecutor creates a new post-action executor
// NOTE: Caller (NewExecutor) is responsible for config validation
func newPostActionExecutor(config *ExecutorConfig) *PostActionExecutor {
	return &PostActionExecutor{
		apiClient: config.APIClient,
		log:       config.Logger,
	}
}

// ExecuteAll executes all post-processing actions
// First builds payloads from post.payloads, then executes post.postActions
func (pae *PostActionExecutor) ExecuteAll(
	ctx context.Context,
	postConfig *configloader.PostConfig,
	execCtx *ExecutionContext,
) ([]PostActionResult, error) {
	if postConfig == nil {
		return []PostActionResult{}, nil
	}

	// Step 1: Build post payloads (like clusterStatusPayload)
	if len(postConfig.Payloads) > 0 {
		pae.log.Infof(ctx, "Building %d post payloads", len(postConfig.Payloads))
		if err := pae.buildPostPayloads(ctx, postConfig.Payloads, execCtx); err != nil {
			errCtx := logger.WithErrorField(ctx, err)
			pae.log.Errorf(errCtx, "Failed to build post payloads")
			execCtx.Adapter.ExecutionError = &ExecutionError{
				Phase:   string(PhasePostActions),
				Step:    "build_payloads",
				Message: err.Error(),
			}
			return []PostActionResult{}, NewExecutorError(
				PhasePostActions, "build_payloads", "failed to build post payloads", err)
		}
		for _, payload := range postConfig.Payloads {
			pae.log.Debugf(ctx, "payload[%s] built successfully", payload.Name)
		}
	}

	// Step 2: Execute post actions (sequential - stop on first failure)
	results := make([]PostActionResult, 0, len(postConfig.PostActions))
	for _, action := range postConfig.PostActions {
		result, err := pae.executePostAction(ctx, action, execCtx)
		results = append(results, result)

		if err != nil {
			errCtx := logger.WithErrorField(ctx, err)
			pae.log.Errorf(errCtx, "PostAction[%s] processed: FAILED", action.Name)

			// Set ExecutionError for failed post action
			execCtx.Adapter.ExecutionError = &ExecutionError{
				Phase:   string(PhasePostActions),
				Step:    action.Name,
				Message: err.Error(),
			}

			// Stop execution - don't run remaining post actions
			return results, err
		}
		if result.Skipped {
			pae.log.Infof(ctx, "PostAction[%s] processed: SKIPPED - reason=%s", action.Name, result.SkipReason)
		} else {
			pae.log.Infof(ctx, "PostAction[%s] processed: SUCCESS - status=%s", action.Name, result.Status)
		}
	}

	return results, nil
}

// buildPostPayloads builds all post payloads and stores them in execCtx.Params
// Payloads are complex structures built from CEL expressions and templates
func (pae *PostActionExecutor) buildPostPayloads(
	ctx context.Context,
	payloads []configloader.Payload,
	execCtx *ExecutionContext,
) error {
	// Create evaluation context with all CEL variables (params, adapter, resources)
	evalCtx := criteria.NewEvaluationContext()
	evalCtx.SetVariablesFromMap(execCtx.GetCELVariables())

	evaluator, err := criteria.NewEvaluator(ctx, evalCtx, pae.log)
	if err != nil {
		return fmt.Errorf("failed to create evaluator: %w", err)
	}

	for _, payload := range payloads {
		// Determine build source (inline Build or BuildRef)
		var buildDef any
		switch {
		case payload.Build != nil:
			buildDef = payload.Build
		case payload.BuildRefContent != nil:
			buildDef = payload.BuildRefContent
		default:
			return fmt.Errorf("payload '%s' has neither Build nor BuildRefContent", payload.Name)
		}

		// Build the payload
		builtPayload, err := pae.buildPayload(ctx, buildDef, evaluator, execCtx.Params)
		if err != nil {
			return fmt.Errorf("failed to build payload '%s': %w", payload.Name, err)
		}

		// Convert to JSON for template rendering (templates will render maps as "map[...]" otherwise)
		jsonBytes, err := json.Marshal(builtPayload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload '%s' to JSON: %w", payload.Name, err)
		}

		// Store as JSON string in params for use in post action templates
		execCtx.Params[payload.Name] = string(jsonBytes)
	}

	return nil
}

// buildPayload builds a payload from a build definition
// The build definition can contain expressions that need to be evaluated
func (pae *PostActionExecutor) buildPayload(
	ctx context.Context,
	build any,
	evaluator *criteria.Evaluator,
	params map[string]any,
) (any, error) {
	switch v := build.(type) {
	case map[string]any:
		return pae.buildMapPayload(ctx, v, evaluator, params)
	case map[any]any:
		converted := utils.ConvertToStringKeyMap(v)
		return pae.buildMapPayload(ctx, converted, evaluator, params)
	default:
		return build, nil
	}
}

// buildMapPayload builds a map payload, evaluating expressions as needed
func (pae *PostActionExecutor) buildMapPayload(
	ctx context.Context,
	m map[string]any,
	evaluator *criteria.Evaluator,
	params map[string]any,
) (map[string]any, error) {
	result := make(map[string]any)

	for k, v := range m {
		// Render the key
		renderedKey, err := utils.RenderTemplate(k, params)
		if err != nil {
			return nil, fmt.Errorf("failed to render key '%s': %w", k, err)
		}

		// Process the value
		processedValue, err := pae.processValue(ctx, v, evaluator, params)
		if err != nil {
			return nil, fmt.Errorf("failed to process value for key '%s': %w", k, err)
		}

		result[renderedKey] = processedValue
	}

	return result, nil
}

// processValue processes a value, evaluating expressions as needed
func (pae *PostActionExecutor) processValue(
	ctx context.Context,
	v any,
	evaluator *criteria.Evaluator,
	params map[string]any,
) (any, error) {
	switch val := v.(type) {
	case map[string]any:
		// Check if this is a value definition: { field: "...", default: ... } or { expression: "...", default: ... }
		if valueDef, ok := configloader.ParseValueDef(val); ok {
			result, err := evaluator.ExtractValue(valueDef.Field, valueDef.Expression)
			// err indicates parse error - fail fast (bug in config)
			if err != nil {
				return nil, err
			}
			// If value is nil (field not found or empty), use default
			if result.Value == nil {
				if result.Error != nil && valueDef.Default == nil {
					pae.log.Warnf(ctx, "Field '%s' not found in payload: %v", result.Source, result.Error)
				} else if valueDef.Default != nil {
					pae.log.Debugf(ctx, "Using default value for '%s': %v", result.Source, valueDef.Default)
				}
				return valueDef.Default, nil
			}
			return result.Value, nil
		}

		// Recursively process nested maps
		return pae.buildMapPayload(ctx, val, evaluator, params)

	case map[any]any:
		converted := utils.ConvertToStringKeyMap(val)
		return pae.processValue(ctx, converted, evaluator, params)

	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			processed, err := pae.processValue(ctx, item, evaluator, params)
			if err != nil {
				return nil, err
			}
			result[i] = processed
		}
		return result, nil

	case string:
		return utils.RenderTemplate(val, params)

	default:
		return v, nil
	}
}

// executePostAction executes a single post-action
func (pae *PostActionExecutor) executePostAction(
	ctx context.Context,
	action configloader.PostAction,
	execCtx *ExecutionContext,
) (PostActionResult, error) {
	result := PostActionResult{
		Name:   action.Name,
		Status: StatusSuccess,
	}

	// Evaluate when condition if configured
	if action.When != nil {
		evalCtx := criteria.NewEvaluationContext()
		evalCtx.SetVariablesFromMap(execCtx.GetCELVariables())
		evaluator, err := criteria.NewEvaluator(ctx, evalCtx, pae.log)
		if err != nil {
			execErr := NewExecutorError(PhasePostActions, action.Name, "failed to create evaluator for when condition", err)
			result.Status = StatusFailed
			result.Error = execErr
			return result, execErr
		}
		celResult, err := evaluator.EvaluateCEL(action.When.Expression)
		if err != nil {
			execErr := NewExecutorError(PhasePostActions, action.Name, "failed to evaluate when condition", err)
			result.Status = StatusFailed
			result.Error = execErr
			return result, execErr
		}
		if celResult.HasError() {
			execErr := NewExecutorError(PhasePostActions, action.Name, "failed to evaluate when condition", celResult.Error)
			result.Status = StatusFailed
			result.Error = execErr
			return result, execErr
		}
		if !celResult.Matched {
			result.Skipped = true
			result.Status = StatusSkipped
			result.SkipReason = fmt.Sprintf("when condition evaluated to false: %s", action.When.Expression)
			pae.log.Infof(ctx, "PostAction[%s] skipped: when condition is false", action.Name)
			return result, nil
		}
	}

	// Execute log action if configured
	if action.Log != nil {
		ExecuteLogAction(ctx, action.Log, execCtx, pae.log)
	}

	// Execute API call if configured
	if action.APICall != nil {
		if err := pae.executeAPICall(ctx, action.APICall, execCtx, &result); err != nil {
			return result, err
		}
	}

	return result, nil
}

// executeAPICall executes an API call and populates the result with response details
func (pae *PostActionExecutor) executeAPICall(
	ctx context.Context,
	apiCall *configloader.APICall,
	execCtx *ExecutionContext,
	result *PostActionResult,
) error {
	resp, url, err := ExecuteAPICall(ctx, apiCall, execCtx, pae.apiClient, pae.log)
	result.APICallMade = true

	// Capture response details if available (even if err != nil)
	if resp != nil {
		result.APIResponse = resp.Body
		result.HTTPStatus = resp.StatusCode
	}

	// Validate response - returns APIError with full metadata if validation fails
	if validationErr := ValidateAPIResponse(resp, err, apiCall.Method, url); validationErr != nil {
		result.Status = StatusFailed
		result.Error = validationErr

		// Determine error context
		errorContext := "API call failed"
		if err == nil && resp != nil && !resp.IsSuccess() {
			errorContext = "API call returned non-success status"
		}

		return NewExecutorError(PhasePostActions, result.Name, errorContext, validationErr)
	}

	return nil
}
