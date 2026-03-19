package services

import (
	"testing"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

func TestMapAdapterToConditionType(t *testing.T) {
	tests := []struct {
		adapter  string
		expected string
	}{
		{"validator", "ValidatorSuccessful"},
		{"dns", "DnsSuccessful"},
		{"gcp-provisioner", "GcpProvisionerSuccessful"},
		{"unknown-adapter", "UnknownAdapterSuccessful"},
		{"multi-word-adapter", "MultiWordAdapterSuccessful"},
		{"single", "SingleSuccessful"},
	}

	for _, tt := range tests {
		result := MapAdapterToConditionType(tt.adapter)
		if result != tt.expected {
			t.Errorf("MapAdapterToConditionType(%q) = %q, want %q",
				tt.adapter, result, tt.expected)
		}
	}
}

// Test custom suffix mapping (for future use)
func TestMapAdapterToConditionType_CustomSuffix(t *testing.T) {
	// Temporarily add a custom mapping
	adapterConditionSuffixMap["test-adapter"] = "Ready"
	defer delete(adapterConditionSuffixMap, "test-adapter")

	result := MapAdapterToConditionType("test-adapter")
	expected := "TestAdapterReady"
	if result != expected {
		t.Errorf("MapAdapterToConditionType(%q) = %q, want %q",
			"test-adapter", result, expected)
	}
}

// Test that default behavior still works after custom suffix is removed
func TestMapAdapterToConditionType_DefaultAfterCustom(t *testing.T) {
	// Add and then remove custom mapping
	adapterConditionSuffixMap["dns"] = "Ready"
	delete(adapterConditionSuffixMap, "dns")

	result := MapAdapterToConditionType("dns")
	expected := "DnsSuccessful"
	if result != expected {
		t.Errorf("MapAdapterToConditionType(%q) = %q, want %q (should revert to default)",
			"dns", result, expected)
	}
}

func TestValidateMandatoryConditions_AllPresent(t *testing.T) {
	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionFalse, LastTransitionTime: time.Now()},
	}

	errorType, conditionName := ValidateMandatoryConditions(conditions)
	if errorType != "" {
		t.Errorf("Expected no errors, got errorType: %s, conditionName: %s", errorType, conditionName)
	}
}

func TestValidateMandatoryConditions_MissingAvailable(t *testing.T) {
	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}

	errorType, conditionName := ValidateMandatoryConditions(conditions)
	if errorType != ConditionValidationErrorMissing {
		t.Errorf("Expected errorType ConditionValidationErrorMissing, got: %s", errorType)
	}
	if conditionName != api.ConditionTypeAvailable {
		t.Errorf("Expected missing condition %s, got: %s", api.ConditionTypeAvailable, conditionName)
	}
}

func TestValidateMandatoryConditions_MandatoryConditionUnknown(t *testing.T) {
	// Unknown status in Applied/Health is allowed; only Available=Unknown has special handling elsewhere
	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionUnknown, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}

	errorType, conditionName := ValidateMandatoryConditions(conditions)
	if errorType != "" {
		t.Errorf("Expected no errors (Unknown is allowed), got errorType: %s, conditionName: %s", errorType, conditionName)
	}
}

func TestValidateMandatoryConditions_WithCustomConditions(t *testing.T) {
	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: "CustomCondition", Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeReady, Status: api.AdapterConditionFalse, LastTransitionTime: time.Now()},
	}

	errorType, conditionName := ValidateMandatoryConditions(conditions)
	if errorType != "" {
		t.Errorf("Expected no errors, got errorType: %s, conditionName: %s", errorType, conditionName)
	}
}

func TestValidateMandatoryConditions_EmptyConditions(t *testing.T) {
	conditions := []api.AdapterCondition{}

	errorType, conditionName := ValidateMandatoryConditions(conditions)
	if errorType != ConditionValidationErrorMissing {
		t.Errorf("Expected errorType ConditionValidationErrorMissing, got: %s", errorType)
	}
	if conditionName != api.ConditionTypeAvailable {
		t.Errorf("Expected missing condition %s, got: %s", api.ConditionTypeAvailable, conditionName)
	}
}

// TestValidateMandatoryConditions_MissingMultiple tests that when multiple conditions are missing,
// the function returns the first missing one
func TestValidateMandatoryConditions_MissingMultiple(t *testing.T) {
	// Test: Only Available present, missing Applied and Health
	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionUnknown, LastTransitionTime: time.Now()},
	}

	errorType, conditionName := ValidateMandatoryConditions(conditions)

	// Should return missing condition
	if errorType != ConditionValidationErrorMissing {
		t.Errorf("Expected errorType ConditionValidationErrorMissing, got: %s", errorType)
	}
	if conditionName != api.ConditionTypeApplied && conditionName != api.ConditionTypeHealth {
		t.Errorf("Expected missing condition to be Applied or Health, got: %s", conditionName)
	}
}
