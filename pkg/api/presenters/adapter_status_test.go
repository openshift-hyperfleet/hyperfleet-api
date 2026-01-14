package presenters

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
)

// Helper function to create test AdapterStatusCreateRequest
func createTestAdapterStatusRequest() *openapi.AdapterStatusCreateRequest {
	reason := "TestReason"
	message := "Test message"
	observedTime := time.Now()

	return &openapi.AdapterStatusCreateRequest{
		Adapter:            "test-adapter",
		ObservedGeneration: 5,
		Data: &map[string]interface{}{
			"section1": map[string]interface{}{"key": "value"},
		},
		Metadata: &struct {
			Attempt       *int32     `json:"attempt,omitempty"`
			CompletedTime *time.Time `json:"completed_time,omitempty"`
			Duration      *string    `json:"duration,omitempty"`
			JobName       *string    `json:"job_name,omitempty"`
			JobNamespace  *string    `json:"job_namespace,omitempty"`
			StartedTime   *time.Time `json:"started_time,omitempty"`
		}{
			JobName: strPtr("test-job"),
		},
		Conditions: []openapi.ConditionRequest{
			{
				Type:    "Ready",
				Status:  openapi.True,
				Reason:  &reason,
				Message: &message,
			},
		},
		ObservedTime: observedTime,
	}
}

func strPtr(s string) *string {
	return &s
}

// TestConvertAdapterStatus_Complete tests conversion with all fields populated
func TestConvertAdapterStatus_Complete(t *testing.T) {
	RegisterTestingT(t)

	req := createTestAdapterStatusRequest()
	resourceType := "Cluster"
	resourceID := "test-cluster-id"

	result, err := ConvertAdapterStatus(resourceType, resourceID, req)
	Expect(err).To(BeNil())

	// Verify basic fields
	Expect(result.ResourceType).To(Equal(resourceType))
	Expect(result.ResourceID).To(Equal(resourceID))
	Expect(result.Adapter).To(Equal("test-adapter"))
	Expect(result.ObservedGeneration).To(Equal(int32(5)))

	// Verify Conditions marshaled correctly
	var conditions []api.AdapterCondition
	err = json.Unmarshal(result.Conditions, &conditions)
	Expect(err).To(BeNil())
	Expect(len(conditions)).To(Equal(1))
	Expect(conditions[0].Type).To(Equal("Ready"))
	Expect(conditions[0].Status).To(Equal(api.ConditionTrue))
	Expect(*conditions[0].Reason).To(Equal("TestReason"))
	Expect(*conditions[0].Message).To(Equal("Test message"))

	// Verify Data marshaled correctly
	var data map[string]map[string]interface{}
	err = json.Unmarshal(result.Data, &data)
	Expect(err).To(BeNil())
	Expect(data["section1"]["key"]).To(Equal("value"))

	// Verify Metadata marshaled correctly
	Expect(result.Metadata).ToNot(BeNil())
	var metadata struct {
		JobName *string `json:"job_name,omitempty"`
	}
	err = json.Unmarshal(result.Metadata, &metadata)
	Expect(err).To(BeNil())
	Expect(*metadata.JobName).To(Equal("test-job"))

	// Verify timestamps
	Expect(result.CreatedTime).ToNot(BeNil())
	Expect(result.LastReportTime).ToNot(BeNil())
	Expect(result.CreatedTime).To(Equal(result.LastReportTime))
}

// TestConvertAdapterStatus_WithObservedTime tests that ObservedTime is used when provided
func TestConvertAdapterStatus_WithObservedTime(t *testing.T) {
	RegisterTestingT(t)

	specificTime := time.Date(2023, 6, 15, 12, 0, 0, 0, time.UTC)
	req := &openapi.AdapterStatusCreateRequest{
		Adapter:            "test-adapter",
		ObservedGeneration: 3,
		Conditions:         []openapi.ConditionRequest{},
		ObservedTime:       specificTime,
	}

	result, err := ConvertAdapterStatus("Cluster", "cluster-123", req)
	Expect(err).To(BeNil())

	Expect(result.CreatedTime).ToNot(BeNil())
	Expect(result.LastReportTime).ToNot(BeNil())
	Expect(result.CreatedTime.Unix()).To(Equal(specificTime.Unix()))
	Expect(result.LastReportTime.Unix()).To(Equal(specificTime.Unix()))
}

// TestConvertAdapterStatus_WithoutObservedTime tests that time.Now() is used when ObservedTime is zero
func TestConvertAdapterStatus_WithoutObservedTime(t *testing.T) {
	RegisterTestingT(t)

	req := &openapi.AdapterStatusCreateRequest{
		Adapter:            "test-adapter",
		ObservedGeneration: 2,
		Conditions:         []openapi.ConditionRequest{},
		ObservedTime:       time.Time{}, // Zero time
	}

	before := time.Now()
	result, err := ConvertAdapterStatus("Cluster", "cluster-456", req)
	after := time.Now()

	Expect(err).To(BeNil())
	Expect(result.CreatedTime).ToNot(BeNil())
	Expect(result.LastReportTime).ToNot(BeNil())

	// Verify time is approximately now (within a few seconds)
	Expect(result.CreatedTime.Unix()).To(BeNumerically(">=", before.Unix()-1))
	Expect(result.CreatedTime.Unix()).To(BeNumerically("<=", after.Unix()+1))
}

// TestConvertAdapterStatus_EmptyConditions tests conversion with empty conditions array
func TestConvertAdapterStatus_EmptyConditions(t *testing.T) {
	RegisterTestingT(t)

	req := &openapi.AdapterStatusCreateRequest{
		Adapter:            "test-adapter",
		ObservedGeneration: 1,
		Conditions:         []openapi.ConditionRequest{}, // Empty
	}

	result, err := ConvertAdapterStatus("NodePool", "nodepool-789", req)
	Expect(err).To(BeNil())

	var conditions []api.AdapterCondition
	err = json.Unmarshal(result.Conditions, &conditions)
	Expect(err).To(BeNil())
	Expect(len(conditions)).To(Equal(0))
}

// TestConvertAdapterStatus_NilData tests conversion with nil Data field
func TestConvertAdapterStatus_NilData(t *testing.T) {
	RegisterTestingT(t)

	req := &openapi.AdapterStatusCreateRequest{
		Adapter:            "test-adapter",
		ObservedGeneration: 1,
		Data:               nil, // Nil data
		Conditions:         []openapi.ConditionRequest{},
	}

	result, err := ConvertAdapterStatus("Cluster", "cluster-000", req)
	Expect(err).To(BeNil())

	var data map[string]map[string]interface{}
	err = json.Unmarshal(result.Data, &data)
	Expect(err).To(BeNil())
	Expect(len(data)).To(Equal(0)) // Empty map marshaled
}

// TestConvertAdapterStatus_NilMetadata tests conversion with nil Metadata field
func TestConvertAdapterStatus_NilMetadata(t *testing.T) {
	RegisterTestingT(t)

	req := &openapi.AdapterStatusCreateRequest{
		Adapter:            "test-adapter",
		ObservedGeneration: 1,
		Metadata:           nil, // Nil metadata
		Conditions:         []openapi.ConditionRequest{},
	}

	result, err := ConvertAdapterStatus("Cluster", "cluster-111", req)
	Expect(err).To(BeNil())

	// Metadata should be nil or empty
	Expect(len(result.Metadata)).To(Equal(0))
}

// TestConvertAdapterStatus_ConditionStatusConversion tests status conversion for all values
func TestConvertAdapterStatus_ConditionStatusConversion(t *testing.T) {
	RegisterTestingT(t)

	testCases := []struct {
		openapiStatus  openapi.ConditionStatus
		expectedDomain api.ConditionStatus
	}{
		{openapi.True, api.ConditionTrue},
		{openapi.False, api.ConditionFalse},
		{openapi.Unknown, api.ConditionUnknown},
	}

	for _, tc := range testCases {
		req := &openapi.AdapterStatusCreateRequest{
			Adapter:            "test-adapter",
			ObservedGeneration: 1,
			Conditions: []openapi.ConditionRequest{
				{
					Type:   "TestCondition",
					Status: tc.openapiStatus,
				},
			},
		}

		result, err := ConvertAdapterStatus("Cluster", "test-id", req)
		Expect(err).To(BeNil())

		var conditions []api.AdapterCondition
		err = json.Unmarshal(result.Conditions, &conditions)
		Expect(err).To(BeNil())
		Expect(conditions[0].Status).To(Equal(tc.expectedDomain))
	}
}

// TestPresentAdapterStatus_Complete tests presentation with all fields
func TestPresentAdapterStatus_Complete(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	reason := "Success"
	message := "All good"

	// Create domain AdapterCondition
	conditions := []api.AdapterCondition{
		{
			Type:               "Ready",
			Status:             api.ConditionTrue,
			Reason:             &reason,
			Message:            &message,
			LastTransitionTime: now,
		},
	}
	conditionsJSON, _ := json.Marshal(conditions)

	data := map[string]map[string]interface{}{
		"metrics": {"cpu": "50%"},
	}
	dataJSON, _ := json.Marshal(data)

	metadata := &struct {
		Attempt       *int32     `json:"attempt,omitempty"`
		CompletedTime *time.Time `json:"completed_time,omitempty"`
		Duration      *string    `json:"duration,omitempty"`
		JobName       *string    `json:"job_name,omitempty"`
		JobNamespace  *string    `json:"job_namespace,omitempty"`
		StartedTime   *time.Time `json:"started_time,omitempty"`
	}{
		JobName: strPtr("adapter-job"),
	}
	metadataJSON, _ := json.Marshal(metadata)

	adapterStatus := &api.AdapterStatus{
		ResourceType:       "Cluster",
		ResourceID:         "cluster-abc",
		Adapter:            "aws-adapter",
		ObservedGeneration: 10,
		Conditions:         conditionsJSON,
		Data:               dataJSON,
		Metadata:           metadataJSON,
		CreatedTime:        &now,
		LastReportTime:     &now,
	}

	result, err := PresentAdapterStatus(adapterStatus)
	Expect(err).To(BeNil())

	// Verify basic fields
	Expect(result.Adapter).To(Equal("aws-adapter"))
	Expect(result.ObservedGeneration).To(Equal(int32(10)))

	// Verify conditions converted correctly
	Expect(len(result.Conditions)).To(Equal(1))
	Expect(result.Conditions[0].Type).To(Equal("Ready"))
	Expect(result.Conditions[0].Status).To(Equal(openapi.True))
	Expect(*result.Conditions[0].Reason).To(Equal("Success"))

	// Verify data unmarshaled correctly
	Expect(result.Data).ToNot(BeNil())
	Expect((*result.Data)["metrics"]).To(BeAssignableToTypeOf(map[string]interface{}{}))
	metrics := (*result.Data)["metrics"].(map[string]interface{})
	Expect(metrics["cpu"]).To(Equal("50%"))

	// Verify metadata unmarshaled correctly
	Expect(result.Metadata).ToNot(BeNil())
	Expect(*result.Metadata.JobName).To(Equal("adapter-job"))

	// Verify timestamps
	Expect(result.CreatedTime.Unix()).To(Equal(now.Unix()))
	Expect(result.LastReportTime.Unix()).To(Equal(now.Unix()))
}

// TestPresentAdapterStatus_NilTimestamps tests handling of nil timestamps
func TestPresentAdapterStatus_NilTimestamps(t *testing.T) {
	RegisterTestingT(t)

	adapterStatus := &api.AdapterStatus{
		ResourceType:       "Cluster",
		ResourceID:         "cluster-xyz",
		Adapter:            "test-adapter",
		ObservedGeneration: 5,
		Conditions:         []byte("[]"),
		Data:               []byte("{}"),
		CreatedTime:        nil, // Nil timestamp
		LastReportTime:     nil, // Nil timestamp
	}

	result, err := PresentAdapterStatus(adapterStatus)
	Expect(err).To(BeNil())

	// Verify zero time.Time is returned (not nil)
	Expect(result.CreatedTime.IsZero()).To(BeTrue())
	Expect(result.LastReportTime.IsZero()).To(BeTrue())
}

// TestPresentAdapterStatus_EmptyConditions tests handling of empty conditions JSON
func TestPresentAdapterStatus_EmptyConditions(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	adapterStatus := &api.AdapterStatus{
		ResourceType:       "NodePool",
		ResourceID:         "nodepool-123",
		Adapter:            "test-adapter",
		ObservedGeneration: 2,
		Conditions:         []byte("[]"), // Empty array JSON
		Data:               []byte("{}"),
		CreatedTime:        &now,
		LastReportTime:     &now,
	}

	result, err := PresentAdapterStatus(adapterStatus)
	Expect(err).To(BeNil())

	Expect(result.Conditions).ToNot(BeNil())
	Expect(len(result.Conditions)).To(Equal(0))
}

// TestPresentAdapterStatus_EmptyData tests handling of empty data JSON
func TestPresentAdapterStatus_EmptyData(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	adapterStatus := &api.AdapterStatus{
		ResourceType:       "Cluster",
		ResourceID:         "cluster-empty",
		Adapter:            "test-adapter",
		ObservedGeneration: 3,
		Conditions:         []byte("[]"),
		Data:               []byte("{}"), // Empty object JSON
		CreatedTime:        &now,
		LastReportTime:     &now,
	}

	result, err := PresentAdapterStatus(adapterStatus)
	Expect(err).To(BeNil())

	Expect(result.Data).ToNot(BeNil())
	Expect(len(*result.Data)).To(Equal(0))
}

// TestPresentAdapterStatus_ConditionStatusConversion tests status conversion from domain to openapi
func TestPresentAdapterStatus_ConditionStatusConversion(t *testing.T) {
	RegisterTestingT(t)

	testCases := []struct {
		domainStatus    api.ConditionStatus
		expectedOpenAPI openapi.ConditionStatus
	}{
		{api.ConditionTrue, openapi.True},
		{api.ConditionFalse, openapi.False},
		{api.ConditionUnknown, openapi.Unknown},
	}

	now := time.Now()
	for _, tc := range testCases {
		conditions := []api.AdapterCondition{
			{
				Type:               "TestCondition",
				Status:             tc.domainStatus,
				LastTransitionTime: now,
			},
		}
		conditionsJSON, _ := json.Marshal(conditions)

		adapterStatus := &api.AdapterStatus{
			ResourceType:       "Cluster",
			ResourceID:         "test-id",
			Adapter:            "test-adapter",
			ObservedGeneration: 1,
			Conditions:         conditionsJSON,
			Data:               []byte("{}"),
			CreatedTime:        &now,
			LastReportTime:     &now,
		}

		result, err := PresentAdapterStatus(adapterStatus)
		Expect(err).To(BeNil())

		Expect(len(result.Conditions)).To(Equal(1))
		Expect(result.Conditions[0].Status).To(Equal(tc.expectedOpenAPI))
	}
}

// TestConvertAndPresent_RoundTrip tests data integrity through convert and present
func TestConvertAndPresent_RoundTrip(t *testing.T) {
	RegisterTestingT(t)

	originalReq := createTestAdapterStatusRequest()

	// Convert from OpenAPI request to domain
	adapterStatus, err := ConvertAdapterStatus("Cluster", "cluster-roundtrip", originalReq)
	Expect(err).To(BeNil())

	// Present from domain back to OpenAPI
	result, err := PresentAdapterStatus(adapterStatus)
	Expect(err).To(BeNil())

	// Verify data integrity
	Expect(result.Adapter).To(Equal(originalReq.Adapter))
	Expect(result.ObservedGeneration).To(Equal(originalReq.ObservedGeneration))

	// Verify conditions preserved
	Expect(len(result.Conditions)).To(Equal(len(originalReq.Conditions)))
	Expect(result.Conditions[0].Type).To(Equal(originalReq.Conditions[0].Type))
	Expect(result.Conditions[0].Status).To(Equal(originalReq.Conditions[0].Status))
	Expect(*result.Conditions[0].Reason).To(Equal(*originalReq.Conditions[0].Reason))
	Expect(*result.Conditions[0].Message).To(Equal(*originalReq.Conditions[0].Message))

	// Verify data preserved
	Expect(result.Data).ToNot(BeNil())
	Expect(originalReq.Data).ToNot(BeNil())
	resultSection := (*result.Data)["section1"].(map[string]interface{})
	originalSection := (*originalReq.Data)["section1"].(map[string]interface{})
	Expect(resultSection["key"]).To(Equal(originalSection["key"]))

	// Verify metadata preserved
	Expect(result.Metadata).ToNot(BeNil())
	Expect(*result.Metadata.JobName).To(Equal(*originalReq.Metadata.JobName))
}

// TestPresentAdapterStatus_MalformedConditions tests error handling for malformed Conditions JSON
func TestPresentAdapterStatus_MalformedConditions(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	adapterStatus := &api.AdapterStatus{
		ResourceType:       "Cluster",
		ResourceID:         "cluster-123",
		Adapter:            "test-adapter",
		ObservedGeneration: 5,
		Conditions:         []byte("{invalid json}"), // Malformed JSON
		Data:               []byte("{}"),
		CreatedTime:        &now,
		LastReportTime:     &now,
	}
	adapterStatus.ID = "adapter-status-malformed-conditions"

	_, err := PresentAdapterStatus(adapterStatus)

	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("failed to unmarshal adapter status conditions"))
}

// TestPresentAdapterStatus_MalformedData tests error handling for malformed Data JSON
func TestPresentAdapterStatus_MalformedData(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	adapterStatus := &api.AdapterStatus{
		ResourceType:       "Cluster",
		ResourceID:         "cluster-456",
		Adapter:            "test-adapter",
		ObservedGeneration: 5,
		Conditions:         []byte("[]"),
		Data:               []byte("{not valid json"), // Malformed JSON
		CreatedTime:        &now,
		LastReportTime:     &now,
	}
	adapterStatus.ID = "adapter-status-malformed-data"

	_, err := PresentAdapterStatus(adapterStatus)

	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("failed to unmarshal adapter status data"))
}

// TestPresentAdapterStatus_MalformedMetadata tests error handling for malformed Metadata JSON
func TestPresentAdapterStatus_MalformedMetadata(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	adapterStatus := &api.AdapterStatus{
		ResourceType:       "Cluster",
		ResourceID:         "cluster-789",
		Adapter:            "test-adapter",
		ObservedGeneration: 5,
		Conditions:         []byte("[]"),
		Data:               []byte("{}"),
		Metadata:           []byte("[{incomplete"), // Malformed JSON
		CreatedTime:        &now,
		LastReportTime:     &now,
	}
	adapterStatus.ID = "adapter-status-malformed-metadata"

	_, err := PresentAdapterStatus(adapterStatus)

	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("failed to unmarshal adapter status metadata"))
}
