package presenters

import (
	"encoding/json"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	. "github.com/onsi/gomega"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
)

// Helper function to create test NodePoolCreateRequest
func createTestNodePoolRequest() *openapi.NodePoolCreateRequest {
	labels := map[string]string{"env": "test"}
	kind := "NodePool"

	return &openapi.NodePoolCreateRequest{
		Kind: &kind,
		Name: "test-nodepool",
		Spec: map[string]interface{}{
			"replicas":     3,
			"instanceType": "n1-standard-4",
		},
		Labels: &labels,
	}
}

// TestConvertNodePool_Complete tests conversion with all fields populated
func TestConvertNodePool_Complete(t *testing.T) {
	RegisterTestingT(t)

	req := createTestNodePoolRequest()
	ownerID := "cluster-owner-123"
	createdBy := "user456"

	result, err := ConvertNodePool(req, ownerID, createdBy)
	Expect(err).To(BeNil())

	// Verify basic fields
	Expect(result.Kind).To(Equal("NodePool"))
	Expect(result.Name).To(Equal("test-nodepool"))
	Expect(result.OwnerID).To(Equal("cluster-owner-123"))
	Expect(result.OwnerKind).To(Equal("Cluster"))
	Expect(result.CreatedBy).To(Equal("user456"))
	Expect(result.UpdatedBy).To(Equal("user456"))

	// Verify Spec marshaled correctly
	var spec map[string]interface{}
	err = json.Unmarshal(result.Spec, &spec)
	Expect(err).To(BeNil())
	Expect(spec["replicas"]).To(BeNumerically("==", 3))
	Expect(spec["instanceType"]).To(Equal("n1-standard-4"))

	// Verify Labels marshaled correctly
	var labels map[string]string
	err = json.Unmarshal(result.Labels, &labels)
	Expect(err).To(BeNil())
	Expect(labels["env"]).To(Equal("test"))

	// StatusConditions initialization is handled by the service layer on create, not presenters.
	Expect(len(result.StatusConditions)).To(Equal(0))
}

// TestConvertNodePool_WithKind tests conversion with Kind specified
func TestConvertNodePool_WithKind(t *testing.T) {
	RegisterTestingT(t)

	customKind := "CustomNodePool"
	req := &openapi.NodePoolCreateRequest{
		Kind:   &customKind,
		Name:   "custom-nodepool",
		Spec:   map[string]interface{}{"test": "spec"},
		Labels: nil,
	}

	result, err := ConvertNodePool(req, "cluster-123", "user789")
	Expect(err).To(BeNil())

	Expect(result.Kind).To(Equal("CustomNodePool"))
}

// TestConvertNodePool_WithoutKind tests conversion with nil Kind (uses default)
func TestConvertNodePool_WithoutKind(t *testing.T) {
	RegisterTestingT(t)

	req := &openapi.NodePoolCreateRequest{
		Kind:   nil, // Nil Kind
		Name:   "default-kind-nodepool",
		Spec:   map[string]interface{}{"test": "spec"},
		Labels: nil,
	}

	result, err := ConvertNodePool(req, "cluster-456", "user000")
	Expect(err).To(BeNil())

	Expect(result.Kind).To(Equal("NodePool")) // Default value
}

// TestConvertNodePool_WithLabels tests conversion with labels
func TestConvertNodePool_WithLabels(t *testing.T) {
	RegisterTestingT(t)

	labels := map[string]string{
		"environment": "production",
		"team":        "platform",
		"region":      "us-east",
	}

	req := &openapi.NodePoolCreateRequest{
		Name:   "labeled-nodepool",
		Spec:   map[string]interface{}{"test": "spec"},
		Labels: &labels,
	}

	result, err := ConvertNodePool(req, "cluster-789", "user111")
	Expect(err).To(BeNil())

	var resultLabels map[string]string
	err = json.Unmarshal(result.Labels, &resultLabels)
	Expect(err).To(BeNil())
	Expect(resultLabels["environment"]).To(Equal("production"))
	Expect(resultLabels["team"]).To(Equal("platform"))
	Expect(resultLabels["region"]).To(Equal("us-east"))
}

// TestConvertNodePool_WithoutLabels tests conversion with nil labels
func TestConvertNodePool_WithoutLabels(t *testing.T) {
	RegisterTestingT(t)

	req := &openapi.NodePoolCreateRequest{
		Name:   "unlabeled-nodepool",
		Spec:   map[string]interface{}{"test": "spec"},
		Labels: nil, // Nil labels
	}

	result, err := ConvertNodePool(req, "cluster-xyz", "user222")
	Expect(err).To(BeNil())

	var resultLabels map[string]string
	err = json.Unmarshal(result.Labels, &resultLabels)
	Expect(err).To(BeNil())
	Expect(len(resultLabels)).To(Equal(0)) // Empty map
}

// TestPresentNodePool_Complete tests presentation with all fields
func TestPresentNodePool_Complete(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	reason := "Ready"
	message := "NodePool is ready"

	// Create domain ResourceCondition
	conditions := []api.ResourceCondition{
		{
			ObservedGeneration: 5,
			CreatedTime:        now,
			LastUpdatedTime:    now,
			Type:               "Available",
			Status:             api.ConditionTrue,
			Reason:             &reason,
			Message:            &message,
			LastTransitionTime: now,
		},
	}
	conditionsJSON, _ := json.Marshal(conditions)

	spec := map[string]interface{}{"replicas": 5}
	specJSON, _ := json.Marshal(spec)

	labels := map[string]string{"env": "staging"}
	labelsJSON, _ := json.Marshal(labels)

	nodePool := &api.NodePool{
		Kind:             "NodePool",
		Href:             "/api/hyperfleet/v1/clusters/cluster-abc/nodepools/nodepool-xyz",
		Name:             "presented-nodepool",
		Spec:             specJSON,
		Labels:           labelsJSON,
		OwnerID:          "cluster-abc",
		OwnerKind:        "Cluster",
		OwnerHref:        "/api/hyperfleet/v1/clusters/cluster-abc",
		StatusConditions: conditionsJSON,
		CreatedBy:        "user123@example.com",
		UpdatedBy:        "user456@example.com",
	}
	nodePool.ID = "nodepool-xyz"
	nodePool.CreatedTime = now
	nodePool.UpdatedTime = now

	result, err := PresentNodePool(nodePool)
	Expect(err).To(BeNil())

	// Verify basic fields
	Expect(*result.Id).To(Equal("nodepool-xyz"))
	Expect(*result.Kind).To(Equal("NodePool"))
	Expect(*result.Href).To(Equal("/api/hyperfleet/v1/clusters/cluster-abc/nodepools/nodepool-xyz"))
	Expect(result.Name).To(Equal("presented-nodepool"))
	Expect(result.CreatedBy).To(Equal(openapi_types.Email("user123@example.com")))
	Expect(result.UpdatedBy).To(Equal(openapi_types.Email("user456@example.com")))

	// Verify Spec unmarshaled correctly
	Expect(result.Spec["replicas"]).To(BeNumerically("==", 5))

	// Verify Labels unmarshaled correctly
	Expect((*result.Labels)["env"]).To(Equal("staging"))

	// Verify OwnerReferences
	Expect(*result.OwnerReferences.Id).To(Equal("cluster-abc"))
	Expect(*result.OwnerReferences.Kind).To(Equal("Cluster"))
	Expect(*result.OwnerReferences.Href).To(Equal("/api/hyperfleet/v1/clusters/cluster-abc"))

	// Verify Status
	Expect(len(result.Status.Conditions)).To(Equal(1))
	Expect(result.Status.Conditions[0].Type).To(Equal("Available"))
	Expect(result.Status.Conditions[0].Status).To(Equal(openapi.ResourceConditionStatusTrue))

	// Verify timestamps
	Expect(result.CreatedTime.Unix()).To(Equal(now.Unix()))
	Expect(result.UpdatedTime.Unix()).To(Equal(now.Unix()))
}

// TestPresentNodePool_HrefGeneration tests that Href is generated if not set
func TestPresentNodePool_HrefGeneration(t *testing.T) {
	RegisterTestingT(t)

	nodePool := &api.NodePool{
		Kind:             "NodePool",
		Href:             "", // Empty Href
		Name:             "href-test",
		Spec:             []byte("{}"),
		Labels:           []byte("{}"),
		OwnerID:          "cluster-owner-456",
		StatusConditions: []byte("[]"),
	}
	nodePool.ID = "nodepool-test-123"

	result, err := PresentNodePool(nodePool)
	Expect(err).To(BeNil())

	Expect(*result.Href).To(Equal("/api/hyperfleet/v1/clusters/cluster-owner-456/nodepools/nodepool-test-123"))
}

// TestPresentNodePool_OwnerHrefGeneration tests that OwnerHref is generated if not set
func TestPresentNodePool_OwnerHrefGeneration(t *testing.T) {
	RegisterTestingT(t)

	nodePool := &api.NodePool{
		Kind:             "NodePool",
		Name:             "owner-href-test",
		Spec:             []byte("{}"),
		Labels:           []byte("{}"),
		OwnerID:          "cluster-owner-789",
		OwnerHref:        "", // Empty OwnerHref
		StatusConditions: []byte("[]"),
	}
	nodePool.ID = "nodepool-owner-test"

	result, err := PresentNodePool(nodePool)
	Expect(err).To(BeNil())

	Expect(*result.OwnerReferences.Href).To(Equal("/api/hyperfleet/v1/clusters/cluster-owner-789"))
}

// TestPresentNodePool_OwnerReferences tests OwnerReferences are set correctly
func TestPresentNodePool_OwnerReferences(t *testing.T) {
	RegisterTestingT(t)

	nodePool := &api.NodePool{
		Kind:             "NodePool",
		Name:             "owner-ref-test",
		Spec:             []byte("{}"),
		Labels:           []byte("{}"),
		OwnerID:          "cluster-ref-123",
		OwnerKind:        "Cluster",
		StatusConditions: []byte("[]"),
	}
	nodePool.ID = "nodepool-ref-456"

	result, err := PresentNodePool(nodePool)
	Expect(err).To(BeNil())

	Expect(result.OwnerReferences.Id).ToNot(BeNil())
	Expect(*result.OwnerReferences.Id).To(Equal("cluster-ref-123"))
	Expect(result.OwnerReferences.Kind).ToNot(BeNil())
	Expect(*result.OwnerReferences.Kind).To(Equal("Cluster"))
	Expect(result.OwnerReferences.Href).ToNot(BeNil())
}

// TestPresentNodePool_StatusConditionsConversion tests condition conversion
func TestPresentNodePool_StatusConditionsConversion(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	reason1 := "Scaling"
	message1 := "Scaling in progress"
	reason2 := "Healthy"
	message2 := "All nodes healthy"

	// Create multiple domain ResourceConditions
	conditions := []api.ResourceCondition{
		{
			ObservedGeneration: 2,
			CreatedTime:        now,
			LastUpdatedTime:    now,
			Type:               "Progressing",
			Status:             api.ConditionTrue,
			Reason:             &reason1,
			Message:            &message1,
			LastTransitionTime: now,
		},
		{
			ObservedGeneration: 2,
			CreatedTime:        now,
			LastUpdatedTime:    now,
			Type:               "Healthy",
			Status:             api.ConditionTrue,
			Reason:             &reason2,
			Message:            &message2,
			LastTransitionTime: now,
		},
	}
	conditionsJSON, _ := json.Marshal(conditions)

	nodePool := &api.NodePool{
		Kind:             "NodePool",
		Name:             "multi-conditions-test",
		Spec:             []byte("{}"),
		Labels:           []byte("{}"),
		OwnerID:          "cluster-conditions",
		StatusConditions: conditionsJSON,
	}
	nodePool.ID = "nodepool-multi-conditions"
	nodePool.CreatedTime = now
	nodePool.UpdatedTime = now

	result, err := PresentNodePool(nodePool)
	Expect(err).To(BeNil())

	// Verify both conditions converted correctly
	Expect(len(result.Status.Conditions)).To(Equal(2))

	// First condition
	Expect(result.Status.Conditions[0].Type).To(Equal("Progressing"))
	Expect(result.Status.Conditions[0].Status).To(Equal(openapi.ResourceConditionStatusTrue))
	Expect(*result.Status.Conditions[0].Reason).To(Equal("Scaling"))
	Expect(*result.Status.Conditions[0].Message).To(Equal("Scaling in progress"))

	// Second condition
	Expect(result.Status.Conditions[1].Type).To(Equal("Healthy"))
	Expect(result.Status.Conditions[1].Status).To(Equal(openapi.ResourceConditionStatusTrue))
	Expect(*result.Status.Conditions[1].Reason).To(Equal("Healthy"))
	Expect(*result.Status.Conditions[1].Message).To(Equal("All nodes healthy"))
}

// TestConvertAndPresentNodePool_RoundTrip tests data integrity through convert and present
func TestConvertAndPresentNodePool_RoundTrip(t *testing.T) {
	RegisterTestingT(t)

	originalReq := createTestNodePoolRequest()
	ownerID := "cluster-roundtrip-789"
	createdBy := "user-roundtrip@example.com"

	// Convert from OpenAPI request to domain
	nodePool, err := ConvertNodePool(originalReq, ownerID, createdBy)
	Expect(err).To(BeNil())

	// Simulate database fields (ID, timestamps)
	nodePool.ID = "nodepool-roundtrip-123"
	now := time.Now()
	nodePool.CreatedTime = now
	nodePool.UpdatedTime = now

	// Present from domain back to OpenAPI
	result, err := PresentNodePool(nodePool)
	Expect(err).To(BeNil())

	// Verify data integrity
	Expect(*result.Id).To(Equal("nodepool-roundtrip-123"))
	Expect(*result.Kind).To(Equal(*originalReq.Kind))
	Expect(result.Name).To(Equal(originalReq.Name))
	Expect(result.CreatedBy).To(Equal(openapi_types.Email(createdBy)))
	Expect(result.UpdatedBy).To(Equal(openapi_types.Email(createdBy)))

	// Verify Spec preserved
	Expect(result.Spec["replicas"]).To(BeNumerically("==", originalReq.Spec["replicas"]))
	Expect(result.Spec["instanceType"]).To(Equal(originalReq.Spec["instanceType"]))

	// Verify Labels preserved
	Expect((*result.Labels)["env"]).To(Equal((*originalReq.Labels)["env"]))

	// Verify OwnerReferences set
	Expect(*result.OwnerReferences.Id).To(Equal(ownerID))
	Expect(*result.OwnerReferences.Kind).To(Equal("Cluster"))

	// Status initialization is handled by the service layer on create, not presenters.
	Expect(len(result.Status.Conditions)).To(Equal(0))
}

// TestPresentNodePool_MalformedSpec tests error handling for malformed Spec JSON
func TestPresentNodePool_MalformedSpec(t *testing.T) {
	RegisterTestingT(t)

	nodePool := &api.NodePool{
		Kind:             "NodePool",
		Name:             "malformed-spec-nodepool",
		Spec:             []byte("{invalid json}"), // Malformed JSON
		Labels:           []byte("{}"),
		OwnerID:          "cluster-123",
		StatusConditions: []byte("[]"),
	}
	nodePool.ID = "nodepool-malformed-spec"

	_, err := PresentNodePool(nodePool)

	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("failed to unmarshal nodepool spec"))
}

// TestPresentNodePool_MalformedLabels tests error handling for malformed Labels JSON
func TestPresentNodePool_MalformedLabels(t *testing.T) {
	RegisterTestingT(t)

	nodePool := &api.NodePool{
		Kind:             "NodePool",
		Name:             "malformed-labels-nodepool",
		Spec:             []byte("{}"),
		Labels:           []byte("{not valid json"), // Malformed JSON
		OwnerID:          "cluster-456",
		StatusConditions: []byte("[]"),
	}
	nodePool.ID = "nodepool-malformed-labels"

	_, err := PresentNodePool(nodePool)

	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("failed to unmarshal nodepool labels"))
}

// TestPresentNodePool_MalformedStatusConditions tests error handling for malformed StatusConditions JSON
func TestPresentNodePool_MalformedStatusConditions(t *testing.T) {
	RegisterTestingT(t)

	nodePool := &api.NodePool{
		Kind:             "NodePool",
		Name:             "malformed-conditions-nodepool",
		Spec:             []byte("{}"),
		Labels:           []byte("{}"),
		OwnerID:          "cluster-789",
		StatusConditions: []byte("[{incomplete"), // Malformed JSON
	}
	nodePool.ID = "nodepool-malformed-conditions"

	_, err := PresentNodePool(nodePool)

	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("failed to unmarshal nodepool status conditions"))
}
