package services

import (
	"encoding/json"
	"testing"
	"time"

	"gorm.io/datatypes"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

// makeConditionsJSON marshals a slice of {Type, Status} pairs into datatypes.JSON.
func makeConditionsJSON(t *testing.T, conditions []struct{ Type, Status string }) datatypes.JSON {
	t.Helper()
	b, err := json.Marshal(conditions)
	if err != nil {
		t.Fatalf("failed to marshal conditions: %v", err)
	}
	return datatypes.JSON(b)
}

// makeAdapterStatus builds an AdapterStatus with the given fields.
func makeAdapterStatus(
	adapter string, gen int32, lastReportTime *time.Time, conditionsJSON datatypes.JSON,
) *api.AdapterStatus {
	return &api.AdapterStatus{
		Adapter:            adapter,
		ObservedGeneration: gen,
		LastReportTime:     lastReportTime,
		Conditions:         conditionsJSON,
	}
}

func ptr(t time.Time) *time.Time { return &t }

func TestComputeReadyLastUpdated_NotReady(t *testing.T) {
	now := time.Now()
	// When isReady=false the function must return now regardless of adapter state.
	result := computeReadyLastUpdated(nil, []string{"dns"}, 1, now, false)
	if !result.Equal(now) {
		t.Errorf("expected now, got %v", result)
	}
}

func TestComputeReadyLastUpdated_MissingAdapter(t *testing.T) {
	now := time.Now()
	statuses := api.AdapterStatusList{
		makeAdapterStatus("validator", 1, ptr(now.Add(-5*time.Second)), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, "True"},
		})),
	}
	// "dns" is required but not in the list → safety fallback to now.
	result := computeReadyLastUpdated(statuses, []string{"validator", "dns"}, 1, now, true)
	if !result.Equal(now) {
		t.Errorf("expected now (missing adapter), got %v", result)
	}
}

func TestComputeReadyLastUpdated_NilLastReportTime(t *testing.T) {
	now := time.Now()
	statuses := api.AdapterStatusList{
		makeAdapterStatus("dns", 1, nil, makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, "True"},
		})),
	}
	result := computeReadyLastUpdated(statuses, []string{"dns"}, 1, now, true)
	if !result.Equal(now) {
		t.Errorf("expected now (nil LastReportTime), got %v", result)
	}
}

func TestComputeReadyLastUpdated_WrongGeneration(t *testing.T) {
	now := time.Now()
	reportTime := now.Add(-10 * time.Second)
	statuses := api.AdapterStatusList{
		// ObservedGeneration=1 but resourceGeneration=2 — skipped.
		makeAdapterStatus("dns", 1, ptr(reportTime), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, "True"},
		})),
	}
	// All adapters skipped → minTime is nil → fallback to now.
	result := computeReadyLastUpdated(statuses, []string{"dns"}, 2, now, true)
	if !result.Equal(now) {
		t.Errorf("expected now (wrong generation), got %v", result)
	}
}

func TestComputeReadyLastUpdated_AvailableFalse(t *testing.T) {
	now := time.Now()
	reportTime := now.Add(-10 * time.Second)
	statuses := api.AdapterStatusList{
		makeAdapterStatus("dns", 1, ptr(reportTime), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, "False"},
		})),
	}
	// Available=False → skipped → fallback to now.
	result := computeReadyLastUpdated(statuses, []string{"dns"}, 1, now, true)
	if !result.Equal(now) {
		t.Errorf("expected now (Available=False), got %v", result)
	}
}

func TestComputeReadyLastUpdated_SingleQualifyingAdapter(t *testing.T) {
	now := time.Now()
	reportTime := now.Add(-30 * time.Second)
	statuses := api.AdapterStatusList{
		makeAdapterStatus("dns", 1, ptr(reportTime), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, "True"},
		})),
	}
	result := computeReadyLastUpdated(statuses, []string{"dns"}, 1, now, true)
	if !result.Equal(reportTime) {
		t.Errorf("expected %v, got %v", reportTime, result)
	}
}

func TestComputeReadyLastUpdated_MultipleAdapters_ReturnsMinimum(t *testing.T) {
	now := time.Now()
	older := now.Add(-60 * time.Second)
	newer := now.Add(-10 * time.Second)

	statuses := api.AdapterStatusList{
		makeAdapterStatus("validator", 2, ptr(newer), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, "True"},
		})),
		makeAdapterStatus("dns", 2, ptr(older), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, "True"},
		})),
	}
	result := computeReadyLastUpdated(statuses, []string{"validator", "dns"}, 2, now, true)
	if !result.Equal(older) {
		t.Errorf("expected minimum timestamp %v, got %v", older, result)
	}
}

// TestBuildSyntheticConditions_ReadyLastUpdatedThreaded verifies the full chain:
// when Ready=True, Ready.LastUpdatedTime equals the adapter's LastReportTime,
// not the evaluation time.
func TestBuildSyntheticConditions_ReadyLastUpdatedThreaded(t *testing.T) {
	now := time.Now()
	reportTime := now.Add(-30 * time.Second)
	adapterStatuses := api.AdapterStatusList{
		makeAdapterStatus("dns", 1, ptr(reportTime), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, "True"},
		})),
	}
	requiredAdapters := []string{"dns"}
	resourceGeneration := int32(1)

	_, readyCondition := BuildSyntheticConditions(
		[]byte("[]"), adapterStatuses, requiredAdapters, resourceGeneration, now,
	)

	if !readyCondition.LastUpdatedTime.Equal(reportTime) {
		t.Errorf("Ready.LastUpdatedTime = %v, want reportTime %v",
			readyCondition.LastUpdatedTime, reportTime)
	}
}

// TestBuildSyntheticConditions_AvailableLastUpdatedTime_Stable verifies that
// Available's LastUpdatedTime is NOT refreshed on every evaluation cycle when
// the status and observed generation are unchanged.
func TestBuildSyntheticConditions_AvailableLastUpdatedTime_Stable(t *testing.T) {
	originalLastUpdated := time.Now().Add(-5 * time.Minute)
	now := time.Now()

	adapterStatuses := api.AdapterStatusList{
		makeAdapterStatus("dns", 1, ptr(now.Add(-10*time.Second)), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, "True"},
		})),
	}
	requiredAdapters := []string{"dns"}
	resourceGeneration := int32(1)

	// Simulate an existing Available condition with a stable LastUpdatedTime.
	existingConditions := []api.ResourceCondition{
		{
			Type:               api.ConditionTypeAvailable,
			Status:             api.ConditionTrue,
			ObservedGeneration: 1,
			LastUpdatedTime:    originalLastUpdated,
			LastTransitionTime: originalLastUpdated,
			CreatedTime:        originalLastUpdated,
		},
	}
	existingJSON, err := json.Marshal(existingConditions)
	if err != nil {
		t.Fatalf("failed to marshal existing conditions: %v", err)
	}

	availableCondition, _ := BuildSyntheticConditions(
		existingJSON, adapterStatuses, requiredAdapters, resourceGeneration, now)

	if !availableCondition.LastUpdatedTime.Equal(originalLastUpdated) {
		t.Errorf("Available.LastUpdatedTime = %v, want %v (must not refresh when status is unchanged)",
			availableCondition.LastUpdatedTime, originalLastUpdated)
	}
}

// TestBuildSyntheticConditions_AvailableLastUpdatedTime_UpdatesOnChange verifies that
// Available's LastUpdatedTime is refreshed when the status changes.
func TestBuildSyntheticConditions_AvailableLastUpdatedTime_UpdatesOnChange(t *testing.T) {
	originalLastUpdated := time.Now().Add(-5 * time.Minute)
	now := time.Now()

	// Adapter now reports Available=False (changed from True).
	adapterStatuses := api.AdapterStatusList{
		makeAdapterStatus("dns", 1, ptr(now.Add(-10*time.Second)), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, "False"},
		})),
	}
	requiredAdapters := []string{"dns"}
	resourceGeneration := int32(1)

	existingConditions := []api.ResourceCondition{
		{
			Type:               api.ConditionTypeAvailable,
			Status:             api.ConditionTrue, // was True, now False
			ObservedGeneration: 1,
			LastUpdatedTime:    originalLastUpdated,
			LastTransitionTime: originalLastUpdated,
			CreatedTime:        originalLastUpdated,
		},
	}
	existingJSON, err := json.Marshal(existingConditions)
	if err != nil {
		t.Fatalf("failed to marshal existing conditions: %v", err)
	}

	availableCondition, _ := BuildSyntheticConditions(
		existingJSON, adapterStatuses, requiredAdapters, resourceGeneration, now)

	if !availableCondition.LastUpdatedTime.Equal(now) {
		t.Errorf("Available.LastUpdatedTime = %v, want %v (must refresh when status changes)",
			availableCondition.LastUpdatedTime, now)
	}
}

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

func TestValidateMandatoryConditions_DuplicateCondition(t *testing.T) {
	// Test: If Available appears twice, should reject as duplicate
	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionUnknown, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()}, // Duplicate!
	}

	errorType, conditionName := ValidateMandatoryConditions(conditions)

	if errorType != ConditionValidationErrorDuplicate {
		t.Errorf("Expected errorType ConditionValidationErrorDuplicate, got: %s", errorType)
	}
	if conditionName != api.ConditionTypeAvailable {
		t.Errorf("Expected duplicate condition %s, got: %s", api.ConditionTypeAvailable, conditionName)
	}
}

func TestValidateMandatoryConditions_DuplicateCustomCondition(t *testing.T) {
	// Test: Duplicate custom condition should also be rejected
	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: "CustomCondition", Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: "CustomCondition", Status: api.AdapterConditionFalse, LastTransitionTime: time.Now()}, // Duplicate!
	}

	errorType, conditionName := ValidateMandatoryConditions(conditions)

	if errorType != ConditionValidationErrorDuplicate {
		t.Errorf("Expected errorType ConditionValidationErrorDuplicate, got: %s", errorType)
	}
	if conditionName != "CustomCondition" {
		t.Errorf("Expected duplicate condition CustomCondition, got: %s", conditionName)
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

func TestValidateMandatoryConditions_EmptyConditionType(t *testing.T) {
	// Test: Condition with empty type should be rejected
	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: "", Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()}, // Empty type
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}

	errorType, conditionName := ValidateMandatoryConditions(conditions)

	if errorType != ConditionValidationErrorMissing {
		t.Errorf("Expected errorType ConditionValidationErrorMissing, got: %s", errorType)
	}
	if conditionName != "<empty type>" {
		t.Errorf("Expected conditionName '<empty type>', got: %s", conditionName)
	}
}
