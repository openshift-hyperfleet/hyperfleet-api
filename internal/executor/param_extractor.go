package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/configloader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/criteria"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleetapi"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/utils"
)

// extractConfigParams extracts all configured parameters and populates execCtx.Params
func extractConfigParams(
	ctx context.Context,
	config *configloader.Config,
	execCtx *ExecutionContext,
	configMap map[string]interface{},
	apiClient hyperfleetapi.Client,
	log logger.Logger,
) error {
	for _, param := range config.Params {
		value, err := extractParam(ctx, param, execCtx, configMap, apiClient, log)
		if err != nil {
			if param.Required {
				return NewExecutorError(PhaseParamExtraction, param.Name,
					fmt.Sprintf("failed to extract required parameter '%s' from source '%s'",
						param.Name, param.Source.Describe()), err)
			}
			if param.Default != nil {
				execCtx.Params[param.Name] = param.Default
			}
			continue
		}

		// Apply default if value is nil or (for strings) empty
		isEmpty := value == nil
		if s, ok := value.(string); ok && s == "" {
			isEmpty = true
		}
		if isEmpty && param.Default != nil {
			value = param.Default
		}

		if value != nil && param.Type != "" && (param.Source.IsString() || param.Source.IsFile()) {
			converted, convErr := convertParamType(value, param.Type)
			if convErr != nil {
				if param.Required {
					return NewExecutorError(PhaseParamExtraction, param.Name,
						fmt.Sprintf("failed to convert parameter '%s' to type '%s'", param.Name, param.Type), convErr)
				}
				if param.Default != nil {
					execCtx.Params[param.Name] = param.Default
				}
				continue
			}
			value = converted
		}

		if value != nil {
			execCtx.Params[param.Name] = value
		}
	}

	return nil
}

// extractParam resolves a single parameter based on its source kind
func extractParam(
	ctx context.Context,
	param configloader.Parameter,
	execCtx *ExecutionContext,
	configMap map[string]interface{},
	apiClient hyperfleetapi.Client,
	log logger.Logger,
) (interface{}, error) {
	switch {
	case param.Source.IsAPICall():
		return extractFromAPICall(ctx, param, execCtx, apiClient, log)
	case param.Source.IsExpression():
		return extractFromCELExpression(ctx, param, execCtx, log)
	case param.Source.IsFile():
		return extractFromFile(param)
	case param.Source.IsString():
		return extractFromStringSource(param, execCtx.EventData, configMap, execCtx.Params)
	default:
		return param.Default, nil
	}
}

// extractFromStringSource handles env.*, event.*, config.*, and dot-notation param derivation
func extractFromStringSource(
	param configloader.Parameter,
	eventData map[string]interface{},
	configMap map[string]interface{},
	resolvedParams map[string]interface{},
) (interface{}, error) {
	source := param.Source.StringVal
	switch {
	case strings.HasPrefix(source, "env."):
		return extractFromEnv(source[4:])
	case strings.HasPrefix(source, "event."):
		return utils.GetNestedValue(eventData, source[6:])
	case strings.HasPrefix(source, "config."):
		return utils.GetNestedValue(configMap, source[7:])
	case source == "":
		return param.Default, nil
	default:
		// Check if the first path segment is a previously resolved param.
		parts := strings.SplitN(source, ".", 2)
		if baseVal, ok := resolvedParams[parts[0]]; ok {
			if len(parts) == 1 {
				return baseVal, nil
			}
			m, ok := baseVal.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("param %q is not a map (got %T), cannot derive %q from it",
					parts[0], baseVal, source)
			}
			return utils.GetNestedValue(m, parts[1])
		}
		// Fallback: treat as bare event path (preserves old behavior for unqualified field names)
		return utils.GetNestedValue(eventData, source)
	}
}

// extractFromAPICall makes an HTTP call, stores the parsed JSON response map as the param value
func extractFromAPICall(
	ctx context.Context,
	param configloader.Parameter,
	execCtx *ExecutionContext,
	apiClient hyperfleetapi.Client,
	log logger.Logger,
) (interface{}, error) {
	ac := param.Source.APICall
	if ac == nil {
		return nil, fmt.Errorf("param %q: api_call source has nil configuration", param.Name)
	}
	resp, renderedURL, err := ExecuteAPICall(ctx, ac, execCtx, apiClient, log)
	if validationErr := ValidateAPIResponse(resp, err, ac.Method, renderedURL); validationErr != nil {
		return nil, validationErr
	}
	var responseData map[string]interface{}
	if jsonErr := json.Unmarshal(resp.Body, &responseData); jsonErr != nil {
		return nil, fmt.Errorf("param %q: failed to parse API response as JSON: %w", param.Name, jsonErr)
	}
	return responseData, nil
}

// extractFromCELExpression evaluates a CEL expression over already-resolved params
func extractFromCELExpression(
	ctx context.Context,
	param configloader.Parameter,
	execCtx *ExecutionContext,
	log logger.Logger,
) (interface{}, error) {
	evalCtx := criteria.NewEvaluationContext()
	evalCtx.SetVariablesFromMap(execCtx.GetCELVariables())
	evaluator, err := criteria.NewEvaluator(ctx, evalCtx, log)
	if err != nil {
		return nil, fmt.Errorf("param %q: failed to create CEL evaluator: %w", param.Name, err)
	}
	result, err := evaluator.EvaluateCEL(strings.TrimSpace(param.Source.Expression))
	if err != nil {
		return nil, fmt.Errorf("param %q: CEL evaluation failed: %w", param.Name, err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("param %q: CEL expression error: %w", param.Name, result.Error)
	}
	return result.Value, nil
}

// maxFileSourceSize is a defensive cap for file-based parameter sources (1 MB).
const maxFileSourceSize = 1 << 20

// extractFromFile reads a parameter value from a filesystem path.
// The file is read fresh on every call (not cached between reconciliations).
func extractFromFile(param configloader.Parameter) (interface{}, error) {
	fs := param.Source.File
	if fs == nil {
		return nil, fmt.Errorf("param %q: file source has nil configuration", param.Name)
	}

	f, err := os.Open(fs.Path)
	if err != nil {
		return nil, fmt.Errorf("param %q: opening %q: %w", param.Name, fs.Path, err)
	}
	defer f.Close() //nolint:errcheck // best-effort close on read-only file

	limited := io.LimitReader(f, maxFileSourceSize+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("param %q: reading %q: %w", param.Name, fs.Path, err)
	}
	if len(raw) > maxFileSourceSize {
		return nil, fmt.Errorf("param %q: file %q exceeds maximum size of %d bytes", param.Name, fs.Path, maxFileSourceSize)
	}

	content := string(raw)
	if fs.Trim == nil || *fs.Trim {
		content = strings.TrimSpace(content)
	}

	if param.Required && strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("param %q: file %q is empty", param.Name, fs.Path)
	}

	return content, nil
}

// configToMap converts a Config to map[string]interface{} using the yaml struct tags for key names.
// mapstructure reads the "yaml" tag for key names but ignores the omitempty option, so zero-valued
// fields like debug_config=false are preserved in the resulting map.
func configToMap(cfg *configloader.Config) (map[string]interface{}, error) {
	var m map[string]interface{}
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName: "yaml",
		Result:  &m,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create config decoder: %w", err)
	}
	if err := decoder.Decode(cfg); err != nil {
		return nil, fmt.Errorf("failed to convert config to map: %w", err)
	}
	return m, nil
}

// extractFromEnv extracts a value from environment variables
func extractFromEnv(envVar string) (interface{}, error) {
	value, exists := os.LookupEnv(envVar)
	if !exists {
		return nil, fmt.Errorf("environment variable %s not set", envVar)
	}
	return value, nil
}

// addAdapterParams adds adapter info and the full config map to execCtx.Params
func addAdapterParams(config *configloader.Config, execCtx *ExecutionContext, configMap map[string]interface{}) {
	execCtx.Params["adapter"] = map[string]interface{}{
		"name":    config.Adapter.Name,
		"version": config.Adapter.Version,
	}
	execCtx.Params["config"] = configMap
}

// convertParamType converts a value to the specified type.
// Supported types: string, int, int64, float, float64, bool
func convertParamType(value interface{}, targetType string) (interface{}, error) {
	return utils.ConvertToType(value, targetType)
}

//nolint:unparam // error kept for API consistency with convertToInt64
func convertToString(value interface{}) (string, error) {
	return utils.ConvertToString(value)
}

func convertToInt64(value interface{}) (int64, error) {
	return utils.ConvertToInt64(value)
}

func convertToFloat64(value interface{}) (float64, error) {
	return utils.ConvertToFloat64(value)
}

func convertToBool(value interface{}) (bool, error) {
	return utils.ConvertToBool(value)
}
