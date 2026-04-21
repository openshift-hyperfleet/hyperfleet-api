package handlers

import (
	"reflect"
	"regexp"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

// Cluster/NodePool name pattern: compliant with Kubernetes DNS Subdomain Names (RFC 1123)
// Must start and end with alphanumeric, can contain hyphens in the middle
var namePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

func validateNotEmpty(i interface{}, fieldName string, field string) validate {
	return func() *errors.ServiceError {
		value := reflect.ValueOf(i).Elem().FieldByName(fieldName)
		if value.Kind() == reflect.Ptr {
			if value.IsNil() {
				return errors.Validation("%s is required", field)
			}
			value = value.Elem()
		}
		if len(value.String()) == 0 {
			return errors.Validation("%s is required", field)
		}
		return nil
	}
}

func validateEmpty(i interface{}, fieldName string, field string) validate {
	return func() *errors.ServiceError {
		value := reflect.ValueOf(i).Elem().FieldByName(fieldName)
		if value.Kind() == reflect.Ptr {
			if value.IsNil() {
				return nil
			}
			value = value.Elem()
		}
		if len(value.String()) != 0 {
			return errors.Validation("%s must be empty", field)
		}
		return nil
	}
}

// validateName validates that a name field matches the pattern ^[a-z0-9-]+$ and length constraints
//
//nolint:unparam // fieldName is kept as parameter for flexibility even though currently only "Name" is used
func validateName(i interface{}, fieldName string, field string, minLen, maxLen int) validate {
	return func() *errors.ServiceError {
		value := reflect.ValueOf(i).Elem().FieldByName(fieldName)
		if value.Kind() == reflect.Ptr {
			if value.IsNil() {
				return errors.Validation("%s is required", field)
			}
			value = value.Elem()
		}

		name := value.String()

		// Check if empty
		if len(name) == 0 {
			return errors.Validation("%s is required", field)
		}

		// Check minimum length
		if len(name) < minLen {
			return errors.Validation("%s must be at least %d characters", field, minLen)
		}

		// Check maximum length
		if len(name) > maxLen {
			return errors.Validation("%s must be at most %d characters", field, maxLen)
		}

		// Check pattern: lowercase alphanumeric and hyphens only
		if !namePattern.MatchString(name) {
			return errors.Validation(
				"%s must start and end with lowercase letter or number, and contain only "+
					"lowercase letters, numbers, and hyphens", field,
			)
		}

		return nil
	}
}

// validateKind validates that the kind field matches the expected value
//
//nolint:unparam // fieldName is kept as parameter for flexibility even though currently only "Kind" is used
func validateKind(i interface{}, fieldName string, field string, expectedKind string) validate {
	return func() *errors.ServiceError {
		value := reflect.ValueOf(i).Elem().FieldByName(fieldName)
		if value.Kind() == reflect.Ptr {
			if value.IsNil() {
				return errors.Validation("%s is required", field)
			}
			value = value.Elem()
		}

		kind := value.String()

		// Check if empty
		if len(kind) == 0 {
			return errors.Validation("%s is required", field)
		}

		// Check if matches expected kind
		if kind != expectedKind {
			return errors.Validation("%s must be '%s'", field, expectedKind)
		}

		return nil
	}
}

// validateSpec validates that the spec field is not nil
func validateSpec(i interface{}, fieldName string, field string) validate { //nolint:unparam
	return func() *errors.ServiceError {
		value := reflect.ValueOf(i).Elem().FieldByName(fieldName)

		if value.Kind() == reflect.Ptr {
			if value.IsNil() {
				return errors.Validation("%s is required", field)
			}
			value = value.Elem()
		}

		if !value.IsValid() || value.IsNil() {
			return errors.Validation("%s is required", field)
		}

		return nil
	}
}

// validatePatchRequest validates that at least one patchable field (Spec or Labels) is provided
func validatePatchRequest(i interface{}) validate {
	return func() *errors.ServiceError {
		v := reflect.ValueOf(i).Elem()

		spec := v.FieldByName("Spec")
		if spec.Kind() == reflect.Ptr {
			spec = spec.Elem()
		}

		labels := v.FieldByName("Labels")
		if labels.Kind() == reflect.Ptr {
			labels = labels.Elem()
		}

		if !spec.IsValid() && !labels.IsValid() {
			return errors.BadRequest("at least one field must be provided for update")
		}
		return nil
	}
}

// validateConditions validates condition type and status fields
// - Type must not be empty
// - Type must not be duplicated
// - Status must be one of: "True", "False", "Unknown"
//
//nolint:unparam // fieldName is kept as parameter for consistency with other validation functions
func validateConditions(i interface{}, fieldName string) validate {
	return func() *errors.ServiceError {
		value := reflect.ValueOf(i).Elem().FieldByName(fieldName)
		if value.Kind() != reflect.Slice {
			return nil
		}

		conditions, ok := value.Interface().([]openapi.ConditionRequest)
		if !ok {
			return nil
		}

		seen := make(map[string]bool)

		for idx, cond := range conditions {
			if cond.Type == "" {
				return errors.Validation("condition type cannot be empty at index %d", idx)
			}

			if seen[cond.Type] {
				return errors.Validation("duplicate condition type '%s' at index %d", cond.Type, idx)
			}
			seen[cond.Type] = true

			if !isValidAdapterConditionStatus(cond.Status) {
				return errors.Validation(
					"invalid status value '%s' for condition at index %d (type: %s): must be 'True', 'False', or 'Unknown'",
					cond.Status, idx, cond.Type,
				)
			}
		}
		return nil
	}
}

// isValidAdapterConditionStatus checks if a status value is one of the allowed enum values
func isValidAdapterConditionStatus(status openapi.AdapterConditionStatus) bool {
	switch status {
	case openapi.AdapterConditionStatusTrue,
		openapi.AdapterConditionStatusFalse,
		openapi.AdapterConditionStatusUnknown:
		return true
	default:
		return false
	}
}
