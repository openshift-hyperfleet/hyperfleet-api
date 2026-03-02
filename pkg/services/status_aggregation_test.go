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

	missing, unknown := ValidateMandatoryConditions(conditions)
	if missing != "" {
		t.Errorf("Expected no missing conditions, got missing: %s", missing)
	}
	if unknown != "" {
		t.Errorf("Expected no unknown conditions, got unknown: %s", unknown)
	}
}

func TestValidateMandatoryConditions_MissingAvailable(t *testing.T) {
	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}

	missing, unknown := ValidateMandatoryConditions(conditions)
	if missing != api.ConditionTypeAvailable {
		t.Errorf("Expected missing condition %s, got: %s", api.ConditionTypeAvailable, missing)
	}
	if unknown != "" {
		t.Errorf("Expected no unknown conditions, got: %s", unknown)
	}
}

func TestValidateMandatoryConditions_MandatoryConditionUnknown(t *testing.T) {
	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionUnknown, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}

	missing, unknown := ValidateMandatoryConditions(conditions)
	if missing != "" {
		t.Errorf("Expected no missing conditions, got: %s", missing)
	}
	if unknown != api.ConditionTypeAvailable {
		t.Errorf("Expected unknown condition %s, got: %s", api.ConditionTypeAvailable, unknown)
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

	missing, unknown := ValidateMandatoryConditions(conditions)
	if missing != "" {
		t.Errorf("Expected no missing conditions, got: %s", missing)
	}
	if unknown != "" {
		t.Errorf("Expected no unknown conditions, got: %s", unknown)
	}
}

func TestValidateMandatoryConditions_EmptyConditions(t *testing.T) {
	conditions := []api.AdapterCondition{}

	missing, unknown := ValidateMandatoryConditions(conditions)
	if missing != api.ConditionTypeAvailable {
		t.Errorf("Expected missing condition %s, got: %s", api.ConditionTypeAvailable, missing)
	}
	if unknown != "" {
		t.Errorf("Expected no unknown conditions, got: %s", unknown)
	}
}

func TestValidateMandatoryConditions_DuplicateCondition_UnknownThenTrue(t *testing.T) {
	// Test: If Available appears twice (Unknown first, True second),
	// should detect Unknown (Unknown has highest priority)
	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionUnknown, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()}, // Duplicate!
	}

	missing, unknown := ValidateMandatoryConditions(conditions)

	// Should detect Unknown even though True comes later
	if missing != "" {
		t.Errorf("Expected no missing conditions, got: %s", missing)
	}
	if unknown != api.ConditionTypeAvailable {
		t.Errorf("Expected unknown condition %s (Unknown should be preserved), got: %s",
			api.ConditionTypeAvailable, unknown)
	}
}

func TestValidateMandatoryConditions_DuplicateCondition_TrueThenUnknown(t *testing.T) {
	// Test: If Available appears twice (True first, Unknown second),
	// should detect Unknown (Unknown has highest priority)
	conditions := []api.AdapterCondition{
		// First: True
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		// Duplicate: Unknown!
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionUnknown, LastTransitionTime: time.Now()},
	}

	missing, unknown := ValidateMandatoryConditions(conditions)

	// Should detect Unknown even though True comes first
	if missing != "" {
		t.Errorf("Expected no missing conditions, got: %s", missing)
	}
	if unknown != api.ConditionTypeAvailable {
		t.Errorf("Expected unknown condition %s (Unknown should overwrite True), got: %s",
			api.ConditionTypeAvailable, unknown)
	}
}
