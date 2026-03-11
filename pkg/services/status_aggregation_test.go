package services

import (
	"context"
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

func TestComputeReadyLastUpdated_NotReady_NoAdapters(t *testing.T) {
	now := time.Now()
	// When isReady=false and a required adapter has no stored status, fall back to now.
	result := computeReadyLastUpdated(context.Background(),nil, []string{"dns"}, 1, now, false)
	if !result.Equal(now) {
		t.Errorf("expected now (missing adapter fallback), got %v", result)
	}
}

func TestComputeReadyLastUpdated_NotReady_WithAdapters(t *testing.T) {
	now := time.Now()
	older := now.Add(-30 * time.Second)
	newer := now.Add(-10 * time.Second)

	// Both required adapters present; one is False → isReady=false.
	// LUT must be min(LRTs) = older, not now.
	statuses := api.AdapterStatusList{
		makeAdapterStatus("dns", 1, ptr(older), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionFalse)},
		})),
		makeAdapterStatus("validator", 1, ptr(newer), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionTrue)},
		})),
	}

	result := computeReadyLastUpdated(context.Background(),statuses, []string{"dns", "validator"}, 1, now, false)
	if !result.Equal(older) {
		t.Errorf("expected min(LRTs)=%v, got %v", older, result)
	}
}

func TestComputeReadyLastUpdated_MissingAdapter(t *testing.T) {
	now := time.Now()
	statuses := api.AdapterStatusList{
		makeAdapterStatus("validator", 1, ptr(now.Add(-5*time.Second)), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionTrue)},
		})),
	}
	// "dns" is required but not in the list → safety fallback to now.
	result := computeReadyLastUpdated(context.Background(),statuses, []string{"validator", "dns"}, 1, now, true)
	if !result.Equal(now) {
		t.Errorf("expected now (missing adapter), got %v", result)
	}
}

func TestComputeReadyLastUpdated_NilLastReportTime(t *testing.T) {
	now := time.Now()
	statuses := api.AdapterStatusList{
		makeAdapterStatus("dns", 1, nil, makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionTrue)},
		})),
	}
	result := computeReadyLastUpdated(context.Background(),statuses, []string{"dns"}, 1, now, true)
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
			{api.ConditionTypeAvailable, string(api.AdapterConditionTrue)},
		})),
	}
	// All adapters skipped → minTime is nil → fallback to now.
	result := computeReadyLastUpdated(context.Background(),statuses, []string{"dns"}, 2, now, true)
	if !result.Equal(now) {
		t.Errorf("expected now (wrong generation), got %v", result)
	}
}

func TestComputeReadyLastUpdated_AvailableFalse(t *testing.T) {
	now := time.Now()
	reportTime := now.Add(-10 * time.Second)
	statuses := api.AdapterStatusList{
		makeAdapterStatus("dns", 1, ptr(reportTime), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionFalse)},
		})),
	}
	// Available=False → skipped → fallback to now.
	result := computeReadyLastUpdated(context.Background(),statuses, []string{"dns"}, 1, now, true)
	if !result.Equal(now) {
		t.Errorf("expected now (Available=False), got %v", result)
	}
}

func TestComputeReadyLastUpdated_SingleQualifyingAdapter(t *testing.T) {
	now := time.Now()
	reportTime := now.Add(-30 * time.Second)
	statuses := api.AdapterStatusList{
		makeAdapterStatus("dns", 1, ptr(reportTime), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionTrue)},
		})),
	}
	result := computeReadyLastUpdated(context.Background(),statuses, []string{"dns"}, 1, now, true)
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
			{api.ConditionTypeAvailable, string(api.AdapterConditionTrue)},
		})),
		makeAdapterStatus("dns", 2, ptr(older), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionTrue)},
		})),
	}
	result := computeReadyLastUpdated(context.Background(),statuses, []string{"validator", "dns"}, 2, now, true)
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
			{api.ConditionTypeAvailable, string(api.AdapterConditionTrue)},
		})),
	}
	requiredAdapters := []string{"dns"}
	resourceGeneration := int32(1)

	_, readyCondition := BuildSyntheticConditions(
		context.Background(), []byte("[]"), adapterStatuses, requiredAdapters, resourceGeneration, now, now, false,
	)

	if !readyCondition.LastUpdatedTime.Equal(reportTime) {
		t.Errorf("Ready.LastUpdatedTime = %v, want reportTime %v",
			readyCondition.LastUpdatedTime, reportTime)
	}
}

// TestBuildSyntheticConditions_AvailableLastUpdatedTime_Stable verifies that
// Available's LastUpdatedTime is updated to min_lut (the adapter's LastReportTime)
// when all_at_X=true and the status stays True. Per spec §5.2, lut=min_lut for
// all cases except True→False transitions.
func TestBuildSyntheticConditions_AvailableLastUpdatedTime_Stable(t *testing.T) {
	originalLastUpdated := time.Now().Add(-5 * time.Minute)
	now := time.Now()
	reportTime := now.Add(-10 * time.Second)

	adapterStatuses := api.AdapterStatusList{
		makeAdapterStatus("dns", 1, ptr(reportTime), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionTrue)},
		})),
	}
	requiredAdapters := []string{"dns"}
	resourceGeneration := int32(1)

	// Simulate an existing Available condition.
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
		context.Background(), existingJSON, adapterStatuses, requiredAdapters, resourceGeneration, now, now, false)

	// Per spec §5.2: when all_at_X=true and status stays True, lut=min_lut=adapter's LastReportTime.
	if !availableCondition.LastUpdatedTime.Equal(reportTime) {
		t.Errorf("Available.LastUpdatedTime = %v, want %v (min_lut from adapter's LastReportTime)",
			availableCondition.LastUpdatedTime, reportTime)
	}
}

// TestBuildSyntheticConditions_AvailableLastUpdatedTime_UpdatesOnChange verifies that
// on a True→False transition, Available's LastUpdatedTime is set to the triggering
// adapter's observed_time (not now), per spec.
func TestBuildSyntheticConditions_AvailableLastUpdatedTime_UpdatesOnChange(t *testing.T) {
	originalLastUpdated := time.Now().Add(-5 * time.Minute)
	now := time.Now()
	adapterReportTime := now.Add(-10 * time.Second)

	// Adapter now reports Available=False (changed from True).
	adapterStatuses := api.AdapterStatusList{
		makeAdapterStatus("dns", 1, ptr(adapterReportTime), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionFalse)},
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

	// observedTime = adapter's LastReportTime (the triggering adapter's observed_time).
	availableCondition, _ := BuildSyntheticConditions(
		context.Background(), existingJSON, adapterStatuses, requiredAdapters,
		resourceGeneration, now, adapterReportTime, false)

	// Per spec: True→False transition → lut=observed_time (adapter's report time).
	if !availableCondition.LastUpdatedTime.Equal(adapterReportTime) {
		t.Errorf("Available.LastUpdatedTime = %v, want %v (observed_time on True→False transition)",
			availableCondition.LastUpdatedTime, adapterReportTime)
	}
}

// TestBuildSyntheticConditions_Available_MixedGenerations verifies that Available stays False
// when required adapters report at different observed generations (all_at_X=false per spec §5.2).
// With no existing Available state and mixed adapter generations, the condition defaults to False.
func TestBuildSyntheticConditions_Available_MixedGenerations(t *testing.T) {
	now := time.Now()
	adapterStatuses := api.AdapterStatusList{
		makeAdapterStatus("dns", 1, ptr(now.Add(-10*time.Second)), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionTrue)},
		})),
		makeAdapterStatus("validator", 2, ptr(now.Add(-5*time.Second)), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionTrue)},
		})),
	}

	// No existing Available condition; adapters at different generations → all_at_X=false.
	availableCondition, _ := BuildSyntheticConditions(
		context.Background(), []byte("[]"), adapterStatuses, []string{"dns", "validator"}, 2, now, now, false)

	// Per spec §5.2: all_at_X=false → no change. With no existing state, defaults to False.
	if availableCondition.Status != api.ConditionFalse {
		t.Errorf("Available.Status = %v, want False (all_at_X=false with mixed generations, no existing state)",
			availableCondition.Status)
	}
	if availableCondition.ObservedGeneration != 2 {
		t.Errorf("Available.ObservedGeneration = %v, want 2 (resource generation when all_at_X=false)",
			availableCondition.ObservedGeneration)
	}
}

// TestBuildSyntheticConditions_AvailableLastUpdatedTime_UpdatesOnGenerationChange verifies that
// Available's LastUpdatedTime is set to min_lut (the adapter's LastReportTime) when the observed
// generation advances and status stays True. Per spec §5.2, lut=min_lut when all_at_X=true and
// the status stays True.
func TestBuildSyntheticConditions_AvailableLastUpdatedTime_UpdatesOnGenerationChange(t *testing.T) {
	originalLastUpdated := time.Now().Add(-5 * time.Minute)
	now := time.Now()
	reportTime := now.Add(-10 * time.Second)

	// Adapter advances from gen1 to gen2; status stays True.
	adapterStatuses := api.AdapterStatusList{
		makeAdapterStatus("dns", 2, ptr(reportTime), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionTrue)},
		})),
	}

	existingConditions := []api.ResourceCondition{
		{
			Type:               api.ConditionTypeAvailable,
			Status:             api.ConditionTrue,
			ObservedGeneration: 1, // was gen1, now gen2
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
		context.Background(), existingJSON, adapterStatuses, []string{"dns"}, 2, now, now, false)

	if availableCondition.Status != api.ConditionTrue {
		t.Errorf("Available.Status = %v, want True", availableCondition.Status)
	}
	// Per spec §5.2: all_at_X=true, stays True → lut=min_lut=adapter's LastReportTime.
	if !availableCondition.LastUpdatedTime.Equal(reportTime) {
		t.Errorf("Available.LastUpdatedTime = %v, want %v (min_lut from adapter's LastReportTime when generation advances)",
			availableCondition.LastUpdatedTime, reportTime)
	}
}

func TestValidateMandatoryConditions_InvalidStatus(t *testing.T) {
	// P2: Available status not in {True, False, Unknown} → reject.
	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: "InvalidValue", LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}

	errorType, conditionName := ValidateMandatoryConditions(conditions)

	if errorType != ConditionValidationErrorInvalidStatus {
		t.Errorf("Expected errorType ConditionValidationErrorInvalidStatus, got: %s", errorType)
	}
	if conditionName != api.ConditionTypeAvailable {
		t.Errorf("Expected conditionName %s, got: %s", api.ConditionTypeAvailable, conditionName)
	}
}

func TestValidateMandatoryConditions_InvalidStatusApplied(t *testing.T) {
	// P2: Applied status not in {True, False, Unknown} → reject.
	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: "bad-value", LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}

	errorType, conditionName := ValidateMandatoryConditions(conditions)

	if errorType != ConditionValidationErrorInvalidStatus {
		t.Errorf("Expected errorType ConditionValidationErrorInvalidStatus, got: %s", errorType)
	}
	if conditionName != api.ConditionTypeApplied {
		t.Errorf("Expected conditionName %s, got: %s", api.ConditionTypeApplied, conditionName)
	}
}

// TestBuildSyntheticConditions_Available_AllAtX_True verifies that Available becomes True
// when all required adapters are at the same generation X and all report True (spec §5.2 T6).
func TestBuildSyntheticConditions_Available_AllAtX_True(t *testing.T) {
	now := time.Now()
	reportTime1 := now.Add(-20 * time.Second)
	reportTime2 := now.Add(-10 * time.Second)

	// Both adapters at gen=1 (same X), both True.
	adapterStatuses := api.AdapterStatusList{
		makeAdapterStatus("dns", 1, ptr(reportTime1), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionTrue)},
		})),
		makeAdapterStatus("validator", 1, ptr(reportTime2), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionTrue)},
		})),
	}

	availableCondition, _ := BuildSyntheticConditions(
		context.Background(), []byte("[]"), adapterStatuses, []string{"dns", "validator"}, 2, now, now, false)

	// Per spec §5.2: all_at_X=true for X=1, all True → Available=True@1, lut=min_lut.
	if availableCondition.Status != api.ConditionTrue {
		t.Errorf("Available.Status = %v, want True (all_at_X=true, all True)", availableCondition.Status)
	}
	if availableCondition.ObservedGeneration != 1 {
		t.Errorf("Available.ObservedGeneration = %v, want 1 (the common generation X)", availableCondition.ObservedGeneration)
	}
	// min_lut = min(reportTime1, reportTime2) = reportTime1 (earlier).
	if !availableCondition.LastUpdatedTime.Equal(reportTime1) {
		t.Errorf("Available.LastUpdatedTime = %v, want %v (min_lut)", availableCondition.LastUpdatedTime, reportTime1)
	}
}

// TestBuildSyntheticConditions_Available_PreservesExistingTrueOnMixedGens verifies that
// when adapters are at different generations (all_at_X=false), an existing Available=True
// is preserved unchanged (spec §5.2 "True | false → no change").
func TestBuildSyntheticConditions_Available_PreservesExistingTrueOnMixedGens(t *testing.T) {
	originalLastUpdated := time.Now().Add(-2 * time.Minute)
	now := time.Now()

	// dns at gen=1, validator at gen=2 → all_at_X=false.
	adapterStatuses := api.AdapterStatusList{
		makeAdapterStatus("dns", 1, ptr(now.Add(-30*time.Second)), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionTrue)},
		})),
		makeAdapterStatus("validator", 2, ptr(now.Add(-10*time.Second)),
			makeConditionsJSON(t, []struct{ Type, Status string }{
				{api.ConditionTypeAvailable, string(api.AdapterConditionTrue)},
			})),
	}

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
		context.Background(), existingJSON, adapterStatuses, []string{"dns", "validator"}, 2, now, now, false)

	// Per spec §5.2: all_at_X=false → no change → preserve existing True@gen1.
	if availableCondition.Status != api.ConditionTrue {
		t.Errorf("Available.Status = %v, want True (preserved, all_at_X=false)", availableCondition.Status)
	}
	if availableCondition.ObservedGeneration != 1 {
		t.Errorf("Available.ObservedGeneration = %v, want 1 (preserved)", availableCondition.ObservedGeneration)
	}
	if !availableCondition.LastUpdatedTime.Equal(originalLastUpdated) {
		t.Errorf("Available.LastUpdatedTime = %v, want %v (preserved when all_at_X=false)",
			availableCondition.LastUpdatedTime, originalLastUpdated)
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

// TestBuildSyntheticConditions_ZeroAdapters_BothTrue verifies that when no required adapters
// are configured, both Available and Ready are trivially True (spec: zero required → satisfied).
func TestBuildSyntheticConditions_ZeroAdapters_BothTrue(t *testing.T) {
	now := time.Now()

	available, ready := BuildSyntheticConditions(
		context.Background(), nil, nil, nil, 1, now, now, false)

	if available.Status != api.ConditionTrue {
		t.Errorf("Available.Status = %v, want True (zero required adapters → trivially satisfied)", available.Status)
	}
	if ready.Status != api.ConditionTrue {
		t.Errorf("Ready.Status = %v, want True (zero required adapters → trivially satisfied)", ready.Status)
	}
	if available.ObservedGeneration != 1 {
		t.Errorf("Available.ObservedGeneration = %v, want 1", available.ObservedGeneration)
	}
	if ready.ObservedGeneration != 1 {
		t.Errorf("Ready.ObservedGeneration = %v, want 1", ready.ObservedGeneration)
	}
}

// TestBuildSyntheticConditions_LifecycleChange_AvailableFrozen verifies that on a lifecycle
// change (Create/Replace), Available is completely frozen and Ready resets with lut=now.
// False→False transition: Ready.ltt is preserved from existing.
func TestBuildSyntheticConditions_LifecycleChange_AvailableFrozen(t *testing.T) {
	fixedTime := time.Now().Add(-5 * time.Minute)
	now := time.Now()

	existingConditions := []api.ResourceCondition{
		{
			Type:               api.ConditionTypeAvailable,
			Status:             api.ConditionFalse,
			ObservedGeneration: 1,
			LastUpdatedTime:    fixedTime,
			LastTransitionTime: fixedTime,
			CreatedTime:        fixedTime,
		},
		{
			Type:               api.ConditionTypeReady,
			Status:             api.ConditionFalse,
			ObservedGeneration: 1,
			LastUpdatedTime:    fixedTime,
			LastTransitionTime: fixedTime,
			CreatedTime:        fixedTime,
		},
	}
	existingJSON, err := json.Marshal(existingConditions)
	if err != nil {
		t.Fatalf("failed to marshal existing conditions: %v", err)
	}

	// Adapter reports (should be ignored for Available on lifecycle change).
	adapterStatuses := api.AdapterStatusList{
		makeAdapterStatus("dns", 2, ptr(now.Add(-1*time.Second)), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionTrue)},
		})),
	}

	available, ready := BuildSyntheticConditions(
		context.Background(), existingJSON, adapterStatuses, []string{"dns"}, 2, now, now, true)

	// Available must be completely frozen (unchanged from existing).
	if available.Status != api.ConditionFalse {
		t.Errorf("Available.Status = %v, want False (frozen on lifecycle change)", available.Status)
	}
	if available.ObservedGeneration != 1 {
		t.Errorf("Available.ObservedGeneration = %v, want 1 (frozen)", available.ObservedGeneration)
	}
	if !available.LastUpdatedTime.Equal(fixedTime) {
		t.Errorf("Available.LastUpdatedTime = %v, want %v (frozen)", available.LastUpdatedTime, fixedTime)
	}
	if !available.LastTransitionTime.Equal(fixedTime) {
		t.Errorf("Available.LastTransitionTime = %v, want %v (frozen)", available.LastTransitionTime, fixedTime)
	}

	// Ready must reset: status=False at new generation, lut=now.
	if ready.Status != api.ConditionFalse {
		t.Errorf("Ready.Status = %v, want False", ready.Status)
	}
	if ready.ObservedGeneration != 2 {
		t.Errorf("Ready.ObservedGeneration = %v, want 2 (new generation)", ready.ObservedGeneration)
	}
	if !ready.LastUpdatedTime.Equal(now) {
		t.Errorf("Ready.LastUpdatedTime = %v, want now=%v (lifecycle reset)", ready.LastUpdatedTime, now)
	}
	// False→False: ltt preserved from existing.
	if !ready.LastTransitionTime.Equal(fixedTime) {
		t.Errorf("Ready.LastTransitionTime = %v, want %v (preserved on False→False)", ready.LastTransitionTime, fixedTime)
	}
	// CreatedTime preserved from existing.
	if !ready.CreatedTime.Equal(fixedTime) {
		t.Errorf("Ready.CreatedTime = %v, want %v (preserved)", ready.CreatedTime, fixedTime)
	}
}

// TestBuildSyntheticConditions_LifecycleChange_ReadyTrueToFalse verifies that on a lifecycle
// change when Ready was True, Ready resets to False with lut=now and ltt=observedTime (now).
func TestBuildSyntheticConditions_LifecycleChange_ReadyTrueToFalse(t *testing.T) {
	fixedTime := time.Now().Add(-5 * time.Minute)
	now := time.Now()

	existingConditions := []api.ResourceCondition{
		{
			Type:               api.ConditionTypeAvailable,
			Status:             api.ConditionTrue,
			ObservedGeneration: 1,
			LastUpdatedTime:    fixedTime,
			LastTransitionTime: fixedTime,
			CreatedTime:        fixedTime,
		},
		{
			Type:               api.ConditionTypeReady,
			Status:             api.ConditionTrue,
			ObservedGeneration: 1,
			LastUpdatedTime:    fixedTime,
			LastTransitionTime: fixedTime,
			CreatedTime:        fixedTime,
		},
	}
	existingJSON, err := json.Marshal(existingConditions)
	if err != nil {
		t.Fatalf("failed to marshal existing conditions: %v", err)
	}

	available, ready := BuildSyntheticConditions(
		context.Background(), existingJSON, nil, []string{"dns"}, 2, now, now, true)

	// Available frozen at True@1.
	if available.Status != api.ConditionTrue {
		t.Errorf("Available.Status = %v, want True (frozen)", available.Status)
	}
	if available.ObservedGeneration != 1 {
		t.Errorf("Available.ObservedGeneration = %v, want 1 (frozen)", available.ObservedGeneration)
	}

	// Ready: True→False, lut=now, ltt=observedTime=now.
	if ready.Status != api.ConditionFalse {
		t.Errorf("Ready.Status = %v, want False", ready.Status)
	}
	if ready.ObservedGeneration != 2 {
		t.Errorf("Ready.ObservedGeneration = %v, want 2", ready.ObservedGeneration)
	}
	if !ready.LastUpdatedTime.Equal(now) {
		t.Errorf("Ready.LastUpdatedTime = %v, want now=%v", ready.LastUpdatedTime, now)
	}
	// True→False: ltt=observedTime=now.
	if !ready.LastTransitionTime.Equal(now) {
		t.Errorf("Ready.LastTransitionTime = %v, want now=%v (True→False transition)", ready.LastTransitionTime, now)
	}
	// CreatedTime preserved from existing.
	if !ready.CreatedTime.Equal(fixedTime) {
		t.Errorf("Ready.CreatedTime = %v, want %v (preserved)", ready.CreatedTime, fixedTime)
	}
}

// TestBuildSyntheticConditions_Available_TrueToFalse_Ltt verifies that on a True→False
// transition, Available's LastTransitionTime is set to observedTime (spec: ltt=obs_time).
// Complements TestBuildSyntheticConditions_AvailableLastUpdatedTime_UpdatesOnChange which
// only checked LastUpdatedTime.
func TestBuildSyntheticConditions_Available_TrueToFalse_Ltt(t *testing.T) {
	fixedLtt := time.Now().Add(-5 * time.Minute)
	now := time.Now()
	adapterReportTime := now.Add(-10 * time.Second)

	adapterStatuses := api.AdapterStatusList{
		makeAdapterStatus("dns", 1, ptr(adapterReportTime), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionFalse)},
		})),
	}

	existingConditions := []api.ResourceCondition{
		{
			Type:               api.ConditionTypeAvailable,
			Status:             api.ConditionTrue,
			ObservedGeneration: 1,
			LastUpdatedTime:    fixedLtt,
			LastTransitionTime: fixedLtt,
			CreatedTime:        fixedLtt,
		},
	}
	existingJSON, err := json.Marshal(existingConditions)
	if err != nil {
		t.Fatalf("failed to marshal existing conditions: %v", err)
	}

	availableCondition, _ := BuildSyntheticConditions(
		context.Background(), existingJSON, adapterStatuses, []string{"dns"}, 1, now, adapterReportTime, false)

	if availableCondition.Status != api.ConditionFalse {
		t.Errorf("Available.Status = %v, want False", availableCondition.Status)
	}
	// True→False: ltt=observedTime=adapterReportTime.
	if !availableCondition.LastTransitionTime.Equal(adapterReportTime) {
		t.Errorf("Available.LastTransitionTime = %v, want %v (observedTime on True→False)",
			availableCondition.LastTransitionTime, adapterReportTime)
	}
	// True→False: lut=observedTime=adapterReportTime.
	if !availableCondition.LastUpdatedTime.Equal(adapterReportTime) {
		t.Errorf("Available.LastUpdatedTime = %v, want %v (observedTime on True→False)",
			availableCondition.LastUpdatedTime, adapterReportTime)
	}
}

// TestBuildSyntheticConditions_Available_FalseToTrue_Ltt verifies that on a False→True
// transition, Available's LastTransitionTime is set to observedTime and LastUpdatedTime
// is set to min(LRTs) (spec: ltt=obs_time, lut=min_lut on False→True).
func TestBuildSyntheticConditions_Available_FalseToTrue_Ltt(t *testing.T) {
	fixedLtt := time.Now().Add(-5 * time.Minute)
	now := time.Now()
	reportTime1 := now.Add(-20 * time.Second) // earlier — will be min(LRTs)
	reportTime2 := now.Add(-10 * time.Second)
	observedTime := now.Add(-5 * time.Second) // distinct from both LRTs

	adapterStatuses := api.AdapterStatusList{
		makeAdapterStatus("dns", 1, ptr(reportTime1), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionTrue)},
		})),
		makeAdapterStatus("validator", 1, ptr(reportTime2), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionTrue)},
		})),
	}

	existingConditions := []api.ResourceCondition{
		{
			Type:               api.ConditionTypeAvailable,
			Status:             api.ConditionFalse,
			ObservedGeneration: 1,
			LastUpdatedTime:    fixedLtt,
			LastTransitionTime: fixedLtt,
			CreatedTime:        fixedLtt,
		},
	}
	existingJSON, err := json.Marshal(existingConditions)
	if err != nil {
		t.Fatalf("failed to marshal existing conditions: %v", err)
	}

	availableCondition, _ := BuildSyntheticConditions(
		context.Background(), existingJSON, adapterStatuses, []string{"dns", "validator"}, 1, now, observedTime, false)

	if availableCondition.Status != api.ConditionTrue {
		t.Errorf("Available.Status = %v, want True (all at same gen, all True)", availableCondition.Status)
	}
	// False→True: ltt=observedTime (status transition).
	if !availableCondition.LastTransitionTime.Equal(observedTime) {
		t.Errorf("Available.LastTransitionTime = %v, want %v (observedTime on False→True)",
			availableCondition.LastTransitionTime, observedTime)
	}
	// False→True: lut=min(LRTs)=reportTime1.
	if !availableCondition.LastUpdatedTime.Equal(reportTime1) {
		t.Errorf("Available.LastUpdatedTime = %v, want %v (min_lut on False→True)",
			availableCondition.LastUpdatedTime, reportTime1)
	}
}

// TestBuildSyntheticConditions_Available_TrueToTrue_LttPreserved verifies that when
// Available stays True, LastTransitionTime is preserved from the existing condition
// (spec: ltt=— on True→True, no change).
func TestBuildSyntheticConditions_Available_TrueToTrue_LttPreserved(t *testing.T) {
	fixedLtt := time.Now().Add(-5 * time.Minute)
	now := time.Now()
	reportTime := now.Add(-10 * time.Second)

	adapterStatuses := api.AdapterStatusList{
		makeAdapterStatus("dns", 1, ptr(reportTime), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionTrue)},
		})),
	}

	existingConditions := []api.ResourceCondition{
		{
			Type:               api.ConditionTypeAvailable,
			Status:             api.ConditionTrue,
			ObservedGeneration: 1,
			LastUpdatedTime:    fixedLtt,
			LastTransitionTime: fixedLtt,
			CreatedTime:        fixedLtt,
		},
	}
	existingJSON, err := json.Marshal(existingConditions)
	if err != nil {
		t.Fatalf("failed to marshal existing conditions: %v", err)
	}

	availableCondition, _ := BuildSyntheticConditions(
		context.Background(), existingJSON, adapterStatuses, []string{"dns"}, 1, now, now, false)

	if availableCondition.Status != api.ConditionTrue {
		t.Errorf("Available.Status = %v, want True", availableCondition.Status)
	}
	// True→True: ltt preserved from existing.
	if !availableCondition.LastTransitionTime.Equal(fixedLtt) {
		t.Errorf("Available.LastTransitionTime = %v, want %v (preserved on True→True)",
			availableCondition.LastTransitionTime, fixedLtt)
	}
	// True→True: lut=min(LRTs)=reportTime.
	if !availableCondition.LastUpdatedTime.Equal(reportTime) {
		t.Errorf("Available.LastUpdatedTime = %v, want %v (min_lut on True→True)",
			availableCondition.LastUpdatedTime, reportTime)
	}
}

// TestBuildSyntheticConditions_Available_FalseToFalse_LttPreserved verifies that when
// Available stays False with all_at_X=true (consistent gen, adapter reports False),
// LastTransitionTime is preserved from the existing condition (spec: ltt=— on False→False).
// This is distinct from the all_at_X=false path (TestBuildSyntheticConditions_Available_MixedGenerations)
// where buildAvailableCondition is never reached.
func TestBuildSyntheticConditions_Available_FalseToFalse_LttPreserved(t *testing.T) {
	fixedLtt := time.Now().Add(-5 * time.Minute)
	now := time.Now()
	reportTime := now.Add(-10 * time.Second)

	// Single adapter at gen=1 reports False → all_at_X=true, consistent=true, newStatus=False.
	adapterStatuses := api.AdapterStatusList{
		makeAdapterStatus("dns", 1, ptr(reportTime), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionFalse)},
		})),
	}

	existingConditions := []api.ResourceCondition{
		{
			Type:               api.ConditionTypeAvailable,
			Status:             api.ConditionFalse,
			ObservedGeneration: 1,
			LastUpdatedTime:    fixedLtt,
			LastTransitionTime: fixedLtt,
			CreatedTime:        fixedLtt,
		},
	}
	existingJSON, err := json.Marshal(existingConditions)
	if err != nil {
		t.Fatalf("failed to marshal existing conditions: %v", err)
	}

	availableCondition, _ := BuildSyntheticConditions(
		context.Background(), existingJSON, adapterStatuses, []string{"dns"}, 1, now, now, false)

	if availableCondition.Status != api.ConditionFalse {
		t.Errorf("Available.Status = %v, want False", availableCondition.Status)
	}
	// False→False: ltt preserved from existing (no status transition occurred).
	if !availableCondition.LastTransitionTime.Equal(fixedLtt) {
		t.Errorf("Available.LastTransitionTime = %v, want %v (preserved on False→False, all_at_X=true path)",
			availableCondition.LastTransitionTime, fixedLtt)
	}
}

// TestComputeReadyLastUpdated_NotReady_NilLastReportTime verifies that when isReady=false
// and a required adapter has nil LastReportTime, the function falls back to now.
// Complements TestComputeReadyLastUpdated_NilLastReportTime which only tests isReady=true.
func TestComputeReadyLastUpdated_NotReady_NilLastReportTime(t *testing.T) {
	now := time.Now()
	statuses := api.AdapterStatusList{
		makeAdapterStatus("dns", 1, nil, makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionFalse)},
		})),
	}
	result := computeReadyLastUpdated(context.Background(),statuses, []string{"dns"}, 1, now, false)
	if !result.Equal(now) {
		t.Errorf("expected now (nil LastReportTime, isReady=false), got %v", result)
	}
}

// TestBuildSyntheticConditions_ReadyStaysFalse_LutIsMinLRT verifies that when Ready stays
// False and all required adapters have real LRTs, Ready.LastUpdatedTime equals min(LRTs)
// rather than now (spec: lut=min(LRTs) when Ready stays False with real adapter LRTs).
func TestBuildSyntheticConditions_ReadyStaysFalse_LutIsMinLRT(t *testing.T) {
	now := time.Now()
	dnsLRT := now.Add(-30 * time.Second)      // earlier — will be min(LRTs)
	validatorLRT := now.Add(-10 * time.Second) // later

	// dns is Available=False → Ready cannot be True.
	adapterStatuses := api.AdapterStatusList{
		makeAdapterStatus("dns", 1, ptr(dnsLRT), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionFalse)},
		})),
		makeAdapterStatus("validator", 1, ptr(validatorLRT), makeConditionsJSON(t, []struct{ Type, Status string }{
			{api.ConditionTypeAvailable, string(api.AdapterConditionTrue)},
		})),
	}

	_, readyCondition := BuildSyntheticConditions(
		context.Background(), []byte("[]"), adapterStatuses, []string{"dns", "validator"}, 1, now, now, false)

	if readyCondition.Status != api.ConditionFalse {
		t.Errorf("Ready.Status = %v, want False (dns is Available=False)", readyCondition.Status)
	}
	// Ready stays False: lut=min(all LRTs)=dnsLRT.
	if !readyCondition.LastUpdatedTime.Equal(dnsLRT) {
		t.Errorf("Ready.LastUpdatedTime = %v, want %v (min(LRTs) when Ready stays False)",
			readyCondition.LastUpdatedTime, dnsLRT)
	}
}
