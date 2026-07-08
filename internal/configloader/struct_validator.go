package configloader

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/criteria"
)

// -----------------------------------------------------------------------------
// Struct Validator (go-playground/validator integration)
// -----------------------------------------------------------------------------

var (
	structValidator     *validator.Validate
	structValidatorOnce sync.Once
	// fieldNameCache maps Go struct field names to yaml tag names (built via reflection)
	fieldNameCache = make(map[string]string)
)

// resourceNamePattern validates resource names for CEL compatibility.
// Must start with lowercase letter, can contain letters, numbers, underscores.
// Hyphens (kebab-case) are NOT allowed as they conflict with CEL's minus operator.
var resourceNamePattern = regexp.MustCompile(`^[a-z][a-zA-Z0-9_]*$`)

// extractYamlTagName extracts the yaml tag name from a struct field.
// Returns the Go field name if no yaml tag is defined.
func extractYamlTagName(fld reflect.StructField) string {
	name := strings.SplitN(fld.Tag.Get("yaml"), ",", 2)[0]
	if name == "-" || name == "" {
		return fld.Name
	}
	return name
}

// buildFieldNameCache recursively scans a type and caches Go field name -> yaml tag name mappings
func buildFieldNameCache(t reflect.Type, visited map[reflect.Type]bool) {
	switch t.Kind() { //nolint:exhaustive // only handling types that contain nested fields
	case reflect.Ptr:
		buildFieldNameCache(t.Elem(), visited)
	case reflect.Slice, reflect.Array, reflect.Map:
		buildFieldNameCache(t.Elem(), visited)
	case reflect.Struct:
		if visited[t] {
			return
		}
		visited[t] = true

		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			fieldNameCache[field.Name] = extractYamlTagName(field)
			buildFieldNameCache(field.Type, visited)
		}
	}
}

// getStructValidator returns a singleton validator instance with custom validations registered
func getStructValidator() *validator.Validate {
	structValidatorOnce.Do(func() {
		structValidator = validator.New()

		// Register custom field-level validations
		if err := structValidator.RegisterValidation(
			"resourcename", validateResourceName); err != nil {
			panic(fmt.Sprintf(
				"failed to register resourcename validation: %v", err))
		}
		if err := structValidator.RegisterValidation(
			"validoperator", validateOperator); err != nil {
			panic(fmt.Sprintf(
				"failed to register validoperator validation: %v", err))
		}

		// Register custom struct-level validations
		structValidator.RegisterStructValidation(validateParameterEnvRequired, Parameter{})

		// Use yaml tag names for field names in errors
		structValidator.RegisterTagNameFunc(extractYamlTagName)

		// Build field name cache by reflecting on all config types
		visited := make(map[reflect.Type]bool)
		buildFieldNameCache(reflect.TypeOf(AdapterConfig{}), visited)
		buildFieldNameCache(reflect.TypeOf(AdapterTaskConfig{}), visited)
		buildFieldNameCache(reflect.TypeOf(Config{}), visited)
	})
	return structValidator
}

// validateResourceName is a custom validator for CEL-compatible resource names
func validateResourceName(fl validator.FieldLevel) bool {
	return resourceNamePattern.MatchString(fl.Field().String())
}

// validateOperator is a custom validator for condition operators
func validateOperator(fl validator.FieldLevel) bool {
	return criteria.IsValidOperator(fl.Field().String())
}

// validateParameterEnvRequired is a struct-level validator for Parameter.
// Checks that source is set and, for required env params, that the env var exists.
func validateParameterEnvRequired(sl validator.StructLevel) {
	// type is guaranteed by RegisterStructValidation
	//nolint:errcheck
	param := sl.Current().Interface().(Parameter)

	if param.Source.IsZero() || (param.Source.IsString() && strings.TrimSpace(param.Source.StringVal) == "") {
		sl.ReportError(param.Source, "source", "Source", "required", "")
		return
	}

	if !param.Required || !param.Source.IsString() ||
		!strings.HasPrefix(param.Source.StringVal, "env.") {
		return
	}

	envName := strings.TrimPrefix(param.Source.StringVal, "env.")
	envValue := os.Getenv(envName)

	// Error if env var is not set and no default provided
	if envValue == "" && param.Default == nil {
		sl.ReportError(param.Source, "source", "Source", "envrequired", envName)
	}
}

// ValidateStruct validates a struct using go-playground/validator tags.
// Returns a ValidationErrors with all validation failures.
func ValidateStruct(s interface{}) *ValidationErrors {
	v := getStructValidator()
	err := v.Struct(s)
	if err == nil {
		return nil
	}

	validationErrors := &ValidationErrors{}

	if errs, ok := err.(validator.ValidationErrors); ok {
		for _, e := range errs {
			// Format as "path message" to match existing error format
			msg := formatFullErrorMessage(e)
			validationErrors.Add("", msg)
		}
	} else {
		validationErrors.Add("", err.Error())
	}

	return validationErrors
}

// formatFullErrorMessage creates a complete error message matching existing format
// e.g., "apiVersion is required" or "spec.adapter.version is required"
func formatFullErrorMessage(e validator.FieldError) string {
	path := formatFieldPath(e.Namespace())
	field := e.Field()

	switch e.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", path)
	case "required_without":
		// e.g., "field is required when expression is not set"
		otherField := yamlFieldName(e.Param())
		return fmt.Sprintf("%s: must have either '%s' or '%s' set", parentPath(path), field, otherField)
	case "excluded_with":
		// e.g., "field and expression cannot both be set"
		otherField := yamlFieldName(e.Param())
		return fmt.Sprintf("%s: '%s' and '%s' are mutually exclusive", parentPath(path), field, otherField)
	case "eq":
		return fmt.Sprintf("invalid %s %q (expected: %q)", path, e.Value(), e.Param())
	case "oneof":
		return fmt.Sprintf("%s %q is invalid (allowed: %s)",
			path, e.Value(), strings.ReplaceAll(e.Param(), " ", ", "))
	case "resourcename":
		return fmt.Sprintf(
			"%s %q: must start with lowercase letter and contain only letters, numbers, underscores (no hyphens)",
			path, e.Value(),
		)
	case "validoperator":
		return fmt.Sprintf("%s: invalid operator %q, must be one of: %s",
			path, e.Value(), strings.Join(criteria.OperatorStrings(), ", "))
	case "required_without_all":
		// e.g., "must specify apiCall, expression, or conditions"
		// Convert params like "ActionBase.APICall Expression Conditions"
		// to "apiCall, expression, or conditions"
		params := strings.Split(e.Param(), " ")
		cleanParams := make([]string, 0, len(params))
		for _, p := range params {
			// Remove embedded struct prefix (e.g., "ActionBase.APICall" -> "APICall")
			if idx := strings.LastIndex(p, "."); idx >= 0 {
				p = p[idx+1:]
			}
			cleanParams = append(cleanParams, yamlFieldName(p))
		}
		return fmt.Sprintf("%s: must specify %s", parentPath(path), strings.Join(cleanParams, ", "))
	case "min":
		return fmt.Sprintf("%s: must have at least %s element(s)", path, e.Param())
	case "unique":
		// e.g., "spec.resources: contains duplicate name values"
		return fmt.Sprintf("%s: contains duplicate %s values", path, yamlFieldName(e.Param()))
	case "envrequired":
		// e.g., "spec.params[0]: required environment variable MY_VAR is not set"
		return fmt.Sprintf("%s: required environment variable %s is not set", parentPath(path), e.Param())
	default:
		return fmt.Sprintf("%s: failed validation %s", path, e.Tag())
	}
}

// parentPath returns the parent path (removes last segment)
// e.g., "spec.preconditions[0].capture[0].field" -> "spec.preconditions[0].capture[0]"
func parentPath(path string) string {
	lastDot := strings.LastIndex(path, ".")
	if lastDot == -1 {
		return path
	}
	return path[:lastDot]
}

// yamlFieldName returns the yaml tag name for a Go struct field name.
// Uses the cache built from reflecting on AdapterConfig.
// Falls back to lowercasing the first character if not in the cache.
func yamlFieldName(goFieldName string) string {
	// Ensure cache is populated
	getStructValidator()

	if yamlName, ok := fieldNameCache[goFieldName]; ok {
		return yamlName
	}
	// Fallback: lowercase first character (common convention)
	if goFieldName == "" {
		return goFieldName
	}
	return strings.ToLower(goFieldName[:1]) + goFieldName[1:]
}

// embeddedStructNames contains Go embedded struct names that should be removed from error paths.
// These are internal implementation details, not actual YAML field names.
var embeddedStructNames = map[string]bool{
	"ActionBase":         true, // Embedded in Precondition and PostAction
	"FieldExpressionDef": true, // Embedded in ValueDef
}

// formatFieldPath converts validator namespace to our path format
// e.g., "AdapterConfig.Spec.Resources[0].Name" -> "spec.resources[0].name"
// Also handles embedded structs by removing the embedded type name
// e.g., "AdapterConfig.Spec.Preconditions[0].ActionBase.Name"
// becomes "spec.preconditions[0].name"
func formatFieldPath(namespace string) string {
	// Remove the root struct name (e.g., "AdapterConfig.")
	parts := strings.SplitN(namespace, ".", 2)
	if len(parts) < 2 {
		return strings.ToLower(namespace)
	}
	path := parts[1]

	// Remove embedded struct names (Go implementation detail, not in YAML)
	// Keep real YAML path components like "spec", "preconditions", etc.
	pathParts := strings.Split(path, ".")
	var cleanParts []string
	for _, part := range pathParts {
		// Skip embedded struct names (internal Go structs, not YAML fields)
		if embeddedStructNames[part] {
			continue
		}
		// Convert array-indexed parts to lowercase
		// e.g., "Preconditions[0]" -> "preconditions[0]"
		if idx := strings.Index(part, "["); idx > 0 {
			part = strings.ToLower(part[:idx]) + part[idx:]
		}
		cleanParts = append(cleanParts, part)
	}

	return strings.Join(cleanParts, ".")
}
