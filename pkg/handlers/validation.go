package handlers

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

const (
	maxReasonLength     = 1024
	maxLabelKeyLen      = 317 // 253-char prefix + "/" + 63-char name
	maxLabelValueLen    = 63
	maxObservedTimeSkew = 5 * time.Minute  // tolerance for clock skew between adapter pods and API server
	maxObservedTimeAge  = 30 * time.Minute // matches Sentinel staleness health check window
)

// dnsLabelPattern matches a single DNS label segment in a Kubernetes label key prefix:
// lowercase alphanumeric with hyphens, must start and end with alphanumeric.
var dnsLabelPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// labelNamePattern matches the name portion of a Kubernetes label key:
// alphanumeric (upper or lower), may contain ., _, -.
var labelNamePattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$`)

// labelValuePattern enforces Kubernetes label value format: empty or alphanumeric with ._-.
var labelValuePattern = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?)?$`)

// isValidLabelKey validates a Kubernetes label key: optional DNS subdomain prefix (lowercase,
// dot-separated, each segment <=63 chars, total <=253) + "/" + required name (<=63 chars).
func isValidLabelKey(k string) bool {
	if len(k) == 0 || len(k) > maxLabelKeyLen {
		return false
	}
	prefix, name, hasPrefix := strings.Cut(k, "/")
	if hasPrefix {
		if !isValidLabelKeyPrefix(prefix) {
			return false
		}
	} else {
		name = prefix
	}
	return len(name) > 0 && len(name) <= 63 && labelNamePattern.MatchString(name)
}

// isValidLabelKeyPrefix validates the DNS subdomain prefix of a Kubernetes label key.
func isValidLabelKeyPrefix(s string) bool {
	if len(s) == 0 || len(s) > 253 {
		return false
	}
	for _, seg := range strings.Split(s, ".") {
		if len(seg) == 0 || len(seg) > 63 || !dnsLabelPattern.MatchString(seg) {
			return false
		}
	}
	return true
}

func validatePathID(id, name string) *errors.ServiceError {
	if _, err := uuid.Parse(id); err != nil {
		return errors.Validation("invalid %s format", name)
	}
	return nil
}

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

//nolint:unparam // fieldName is generic; currently only "Reason" but used for any field
func validateMaxLength(i interface{}, fieldName string, field string, maxLen int) validate {
	return func() *errors.ServiceError {
		value := reflect.ValueOf(i).Elem().FieldByName(fieldName)
		if value.Kind() == reflect.Ptr {
			if value.IsNil() {
				return nil
			}
			value = value.Elem()
		}
		if utf8.RuneCountInString(value.String()) > maxLen {
			return errors.Validation("%s must be at most %d characters", field, maxLen)
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

// validateLabels validates that all label keys and values conform to Kubernetes label format.
// Keys must match Kubernetes label key format (optional prefix/name).
// Values must be empty or match the same alphanumeric-with-._- pattern, max 63 chars.
// This catches XSS payloads, path traversal, null bytes, and other invalid inputs.
//
//nolint:unparam // fieldName is kept as parameter for flexibility even though currently only "Labels" is used
func validateLabels(i interface{}, fieldName string) validate {
	return func() *errors.ServiceError {
		value := reflect.ValueOf(i).Elem().FieldByName(fieldName)
		if value.Kind() == reflect.Ptr {
			if value.IsNil() {
				return nil
			}
			value = value.Elem()
		}
		if !value.IsValid() {
			return nil
		}

		labels, ok := value.Interface().(map[string]string)
		if !ok {
			return nil
		}

		var details []errors.ValidationDetail
		for k, v := range labels {
			if !isValidLabelKey(k) {
				details = append(details, errors.ValidationDetail{
					Field:      "labels",
					Value:      k,
					Constraint: "pattern",
					Message:    "label key must match Kubernetes label format (optional DNS prefix/name, alphanumeric with ._-)",
				})
			}
			if len(v) > maxLabelValueLen || !labelValuePattern.MatchString(v) {
				details = append(details, errors.ValidationDetail{
					Field:      fmt.Sprintf("labels[%s]", k),
					Value:      v,
					Constraint: "pattern",
					Message:    fmt.Sprintf("label value must be at most %d chars (alphanumeric with ._-)", maxLabelValueLen),
				})
			}
		}

		if len(details) > 0 {
			return errors.ValidationWithDetails("label validation failed", details)
		}
		return nil
	}
}

// validatePatchRequest validates that at least one patchable field (Spec or Labels) is provided
func validatePatchRequest(i interface{}) validate {
	return func() *errors.ServiceError {
		v := reflect.ValueOf(i).Elem()

		spec := v.FieldByName("Spec")
		labels := v.FieldByName("Labels")

		specPresent := spec.IsValid() && !spec.IsNil()
		labelsPresent := labels.IsValid() && !labels.IsNil()

		if !specPresent && !labelsPresent {
			return errors.BadRequest("at least one field must be provided for update")
		}
		return nil
	}
}

func validateObservedGeneration(req *openapi.AdapterStatusCreateRequest) validate {
	return func() *errors.ServiceError {
		if req.ObservedGeneration < 1 {
			return errors.Validation("observed_generation must be >= 1")
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

// validateObservedTimeRange rejects observed_time values that are more than maxObservedTimeSkew in
// the future or more than maxObservedTimeAge in the past — see HYPERFLEET-1239.
func validateObservedTimeRange(t *time.Time) validate {
	return func() *errors.ServiceError {
		if t == nil || t.IsZero() {
			return nil
		}
		now := time.Now()
		if t.After(now.Add(maxObservedTimeSkew)) {
			return errors.Validation("observed_time must not be more than %s in the future", maxObservedTimeSkew)
		}
		if t.Before(now.Add(-maxObservedTimeAge)) {
			return errors.Validation("observed_time must not be more than %s in the past", maxObservedTimeAge)
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
