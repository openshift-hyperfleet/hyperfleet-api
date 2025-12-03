package api

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

// TestResourcePhase_Constants verifies that ResourcePhase constants match OpenAPI equivalents
func TestResourcePhase_Constants(t *testing.T) {
	RegisterTestingT(t)

	// Verify constant values match expected strings
	Expect(string(PhaseNotReady)).To(Equal("NotReady"))
	Expect(string(PhaseReady)).To(Equal("Ready"))
	Expect(string(PhaseFailed)).To(Equal("Failed"))

	// These values should match openapi.NOT_READY, openapi.READY, openapi.FAILED
	// which are "NotReady", "Ready", "Failed" respectively
}

// TestResourcePhase_StringConversion tests type casting to/from string
func TestResourcePhase_StringConversion(t *testing.T) {
	RegisterTestingT(t)

	// Test converting string to ResourcePhase
	phase := ResourcePhase("NotReady")
	Expect(phase).To(Equal(PhaseNotReady))

	// Test converting ResourcePhase to string
	str := string(PhaseReady)
	Expect(str).To(Equal("Ready"))
}

// TestConditionStatus_Constants verifies that ConditionStatus constants match OpenAPI equivalents
func TestConditionStatus_Constants(t *testing.T) {
	RegisterTestingT(t)

	// Verify constant values match expected strings
	Expect(string(ConditionTrue)).To(Equal("True"))
	Expect(string(ConditionFalse)).To(Equal("False"))
	Expect(string(ConditionUnknown)).To(Equal("Unknown"))

	// These values should match openapi.TRUE, openapi.FALSE, openapi.UNKNOWN
	// which are "True", "False", "Unknown" respectively
}

// TestConditionStatus_StringConversion tests type casting to/from string
func TestConditionStatus_StringConversion(t *testing.T) {
	RegisterTestingT(t)

	// Test converting string to ConditionStatus
	status := ConditionStatus("True")
	Expect(status).To(Equal(ConditionTrue))

	// Test converting ConditionStatus to string
	str := string(ConditionFalse)
	Expect(str).To(Equal("False"))
}

// TestResourceCondition_JSONSerialization tests marshaling ResourceCondition to JSON
func TestResourceCondition_JSONSerialization(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now().UTC().Truncate(time.Second)
	reason := "TestReason"
	message := "Test message"

	// Test full struct with all fields
	fullCondition := ResourceCondition{
		ObservedGeneration: 5,
		CreatedTime:        now,
		LastUpdatedTime:    now,
		Type:               "Ready",
		Status:             ConditionTrue,
		Reason:             &reason,
		Message:            &message,
		LastTransitionTime: now,
	}

	jsonBytes, err := json.Marshal(fullCondition)
	Expect(err).To(BeNil())

	// Verify JSON contains expected fields
	var jsonMap map[string]interface{}
	err = json.Unmarshal(jsonBytes, &jsonMap)
	Expect(err).To(BeNil())

	Expect(jsonMap["observed_generation"]).To(BeNumerically("==", 5))
	Expect(jsonMap["type"]).To(Equal("Ready"))
	Expect(jsonMap["status"]).To(Equal("True"))
	Expect(jsonMap["reason"]).To(Equal("TestReason"))
	Expect(jsonMap["message"]).To(Equal("Test message"))

	// Test struct with nil optional fields
	minimalCondition := ResourceCondition{
		ObservedGeneration: 3,
		CreatedTime:        now,
		LastUpdatedTime:    now,
		Type:               "Available",
		Status:             ConditionFalse,
		Reason:             nil,
		Message:            nil,
		LastTransitionTime: now,
	}

	jsonBytes, err = json.Marshal(minimalCondition)
	Expect(err).To(BeNil())

	var minimalMap map[string]interface{}
	err = json.Unmarshal(jsonBytes, &minimalMap)
	Expect(err).To(BeNil())

	// Verify optional fields are omitted
	_, hasReason := minimalMap["reason"]
	_, hasMessage := minimalMap["message"]
	Expect(hasReason).To(BeFalse())
	Expect(hasMessage).To(BeFalse())
}

// TestResourceCondition_JSONDeserialization tests unmarshaling JSON to ResourceCondition
func TestResourceCondition_JSONDeserialization(t *testing.T) {
	RegisterTestingT(t)

	// Test JSON with all fields
	fullJSON := `{
		"observed_generation": 7,
		"created_time": "2023-01-01T00:00:00Z",
		"last_updated_time": "2023-01-01T01:00:00Z",
		"type": "Validated",
		"status": "True",
		"reason": "Success",
		"message": "Validation successful",
		"last_transition_time": "2023-01-01T02:00:00Z"
	}`

	var condition ResourceCondition
	err := json.Unmarshal([]byte(fullJSON), &condition)
	Expect(err).To(BeNil())

	Expect(condition.ObservedGeneration).To(Equal(int32(7)))
	Expect(condition.Type).To(Equal("Validated"))
	Expect(condition.Status).To(Equal(ConditionTrue))
	Expect(condition.Reason).ToNot(BeNil())
	Expect(*condition.Reason).To(Equal("Success"))
	Expect(condition.Message).ToNot(BeNil())
	Expect(*condition.Message).To(Equal("Validation successful"))

	// Test JSON with missing optional fields
	minimalJSON := `{
		"observed_generation": 2,
		"created_time": "2023-01-01T00:00:00Z",
		"last_updated_time": "2023-01-01T01:00:00Z",
		"type": "NotReady",
		"status": "Unknown",
		"last_transition_time": "2023-01-01T02:00:00Z"
	}`

	var minimalCondition ResourceCondition
	err = json.Unmarshal([]byte(minimalJSON), &minimalCondition)
	Expect(err).To(BeNil())

	Expect(minimalCondition.ObservedGeneration).To(Equal(int32(2)))
	Expect(minimalCondition.Type).To(Equal("NotReady"))
	Expect(minimalCondition.Status).To(Equal(ConditionUnknown))
	Expect(minimalCondition.Reason).To(BeNil())
	Expect(minimalCondition.Message).To(BeNil())
}

// TestResourceCondition_RoundTrip tests Marshal → Unmarshal to ensure no data loss
func TestResourceCondition_RoundTrip(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now().UTC().Truncate(time.Second)
	reason := "RoundTripReason"
	message := "Round trip message"

	original := ResourceCondition{
		ObservedGeneration: 10,
		CreatedTime:        now,
		LastUpdatedTime:    now,
		Type:               "HealthCheck",
		Status:             ConditionTrue,
		Reason:             &reason,
		Message:            &message,
		LastTransitionTime: now,
	}

	// Marshal
	jsonBytes, err := json.Marshal(original)
	Expect(err).To(BeNil())

	// Unmarshal
	var decoded ResourceCondition
	err = json.Unmarshal(jsonBytes, &decoded)
	Expect(err).To(BeNil())

	// Verify all fields match
	Expect(decoded.ObservedGeneration).To(Equal(original.ObservedGeneration))
	Expect(decoded.Type).To(Equal(original.Type))
	Expect(decoded.Status).To(Equal(original.Status))
	Expect(*decoded.Reason).To(Equal(*original.Reason))
	Expect(*decoded.Message).To(Equal(*original.Message))

	// Compare timestamps (truncated to second precision to avoid nanosecond differences)
	Expect(decoded.CreatedTime.Unix()).To(Equal(original.CreatedTime.Unix()))
	Expect(decoded.LastUpdatedTime.Unix()).To(Equal(original.LastUpdatedTime.Unix()))
	Expect(decoded.LastTransitionTime.Unix()).To(Equal(original.LastTransitionTime.Unix()))
}

// TestAdapterCondition_JSONSerialization tests marshaling AdapterCondition to JSON
func TestAdapterCondition_JSONSerialization(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now().UTC().Truncate(time.Second)
	reason := "AdapterReady"
	message := "Adapter is ready"

	// Test with all fields
	fullCondition := AdapterCondition{
		Type:               "Connected",
		Status:             ConditionTrue,
		Reason:             &reason,
		Message:            &message,
		LastTransitionTime: now,
	}

	jsonBytes, err := json.Marshal(fullCondition)
	Expect(err).To(BeNil())

	var jsonMap map[string]interface{}
	err = json.Unmarshal(jsonBytes, &jsonMap)
	Expect(err).To(BeNil())

	Expect(jsonMap["type"]).To(Equal("Connected"))
	Expect(jsonMap["status"]).To(Equal("True"))
	Expect(jsonMap["reason"]).To(Equal("AdapterReady"))
	Expect(jsonMap["message"]).To(Equal("Adapter is ready"))

	// Test without optional fields
	minimalCondition := AdapterCondition{
		Type:               "Disconnected",
		Status:             ConditionFalse,
		Reason:             nil,
		Message:            nil,
		LastTransitionTime: now,
	}

	jsonBytes, err = json.Marshal(minimalCondition)
	Expect(err).To(BeNil())

	var minimalMap map[string]interface{}
	err = json.Unmarshal(jsonBytes, &minimalMap)
	Expect(err).To(BeNil())

	_, hasReason := minimalMap["reason"]
	_, hasMessage := minimalMap["message"]
	Expect(hasReason).To(BeFalse())
	Expect(hasMessage).To(BeFalse())
}

// TestAdapterCondition_JSONDeserialization tests unmarshaling JSON to AdapterCondition
func TestAdapterCondition_JSONDeserialization(t *testing.T) {
	RegisterTestingT(t)

	// Test JSON with all fields
	fullJSON := `{
		"type": "Synced",
		"status": "True",
		"reason": "SyncSuccessful",
		"message": "Data synchronized",
		"last_transition_time": "2023-01-01T12:00:00Z"
	}`

	var condition AdapterCondition
	err := json.Unmarshal([]byte(fullJSON), &condition)
	Expect(err).To(BeNil())

	Expect(condition.Type).To(Equal("Synced"))
	Expect(condition.Status).To(Equal(ConditionTrue))
	Expect(condition.Reason).ToNot(BeNil())
	Expect(*condition.Reason).To(Equal("SyncSuccessful"))
	Expect(condition.Message).ToNot(BeNil())
	Expect(*condition.Message).To(Equal("Data synchronized"))

	// Test JSON without optional fields
	minimalJSON := `{
		"type": "Error",
		"status": "False",
		"last_transition_time": "2023-01-01T12:00:00Z"
	}`

	var minimalCondition AdapterCondition
	err = json.Unmarshal([]byte(minimalJSON), &minimalCondition)
	Expect(err).To(BeNil())

	Expect(minimalCondition.Type).To(Equal("Error"))
	Expect(minimalCondition.Status).To(Equal(ConditionFalse))
	Expect(minimalCondition.Reason).To(BeNil())
	Expect(minimalCondition.Message).To(BeNil())
}

// TestAdapterCondition_RoundTrip tests Marshal → Unmarshal to ensure no data loss
func TestAdapterCondition_RoundTrip(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now().UTC().Truncate(time.Second)
	reason := "TestReason"
	message := "Test message for round trip"

	original := AdapterCondition{
		Type:               "Provisioned",
		Status:             ConditionTrue,
		Reason:             &reason,
		Message:            &message,
		LastTransitionTime: now,
	}

	// Marshal
	jsonBytes, err := json.Marshal(original)
	Expect(err).To(BeNil())

	// Unmarshal
	var decoded AdapterCondition
	err = json.Unmarshal(jsonBytes, &decoded)
	Expect(err).To(BeNil())

	// Verify all fields match
	Expect(decoded.Type).To(Equal(original.Type))
	Expect(decoded.Status).To(Equal(original.Status))
	Expect(*decoded.Reason).To(Equal(*original.Reason))
	Expect(*decoded.Message).To(Equal(*original.Message))
	Expect(decoded.LastTransitionTime.Unix()).To(Equal(original.LastTransitionTime.Unix()))
}
