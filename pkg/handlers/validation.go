package handlers

import (
	"reflect"
	"regexp"

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
func validateSpec(i interface{}, fieldName string, field string) validate {
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
