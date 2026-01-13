package presenters

import (
	"encoding/json"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	. "github.com/onsi/gomega"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/util"
)

// Helper function to create test ClusterCreateRequest
func createTestClusterRequest() *openapi.ClusterCreateRequest {
	labels := map[string]string{"env": "test"}

	return &openapi.ClusterCreateRequest{
		Labels: &labels,
		Kind:   util.PtrString("Cluster"),
		Name:   "test-cluster",
		Spec: map[string]interface{}{
			"region":   "us-central1",
			"provider": "gcp",
		},
	}
}

// TestConvertCluster_Complete tests conversion with all fields populated
func TestConvertCluster_Complete(t *testing.T) {
	RegisterTestingT(t)

	req := createTestClusterRequest()
	createdBy := "user123"

	result, err := ConvertCluster(req, createdBy)
	Expect(err).To(BeNil())

	// Verify basic fields
	Expect(result.Kind).To(Equal("Cluster"))
	Expect(result.Name).To(Equal("test-cluster"))
	Expect(result.CreatedBy).To(Equal(createdBy))
	Expect(result.UpdatedBy).To(Equal(createdBy))

	// Verify defaults
	Expect(result.Generation).To(Equal(int32(1)))
	Expect(result.StatusPhase).To(Equal("NotReady"))
	Expect(result.StatusObservedGeneration).To(Equal(int32(0)))

	// Verify Spec marshaled correctly
	var spec map[string]interface{}
	err = json.Unmarshal(result.Spec, &spec)
	Expect(err).To(BeNil())
	Expect(spec["region"]).To(Equal("us-central1"))
	Expect(spec["provider"]).To(Equal("gcp"))

	// Verify Labels marshaled correctly
	var labels map[string]string
	err = json.Unmarshal(result.Labels, &labels)
	Expect(err).To(BeNil())
	Expect(labels["env"]).To(Equal("test"))

	// Verify StatusConditions initialized as empty array
	var conditions []api.ResourceCondition
	err = json.Unmarshal(result.StatusConditions, &conditions)
	Expect(err).To(BeNil())
	Expect(len(conditions)).To(Equal(0))
}

// TestConvertCluster_WithLabels tests conversion with labels
func TestConvertCluster_WithLabels(t *testing.T) {
	RegisterTestingT(t)

	labels := map[string]string{
		"env":  "production",
		"team": "platform",
	}

	req := &openapi.ClusterCreateRequest{
		Labels: &labels,
		Kind:   util.PtrString("Cluster"),
		Name:   "labeled-cluster",
		Spec:   map[string]interface{}{"test": "spec"},
	}

	result, err := ConvertCluster(req, "user456")
	Expect(err).To(BeNil())

	var resultLabels map[string]string
	err = json.Unmarshal(result.Labels, &resultLabels)
	Expect(err).To(BeNil())
	Expect(resultLabels["env"]).To(Equal("production"))
	Expect(resultLabels["team"]).To(Equal("platform"))
}

// TestConvertCluster_WithoutLabels tests conversion with nil labels
func TestConvertCluster_WithoutLabels(t *testing.T) {
	RegisterTestingT(t)

	req := &openapi.ClusterCreateRequest{
		Labels: nil, // Nil labels
		Kind:   util.PtrString("Cluster"),
		Name:   "unlabeled-cluster",
		Spec:   map[string]interface{}{"test": "spec"},
	}

	result, err := ConvertCluster(req, "user789")
	Expect(err).To(BeNil())

	var resultLabels map[string]string
	err = json.Unmarshal(result.Labels, &resultLabels)
	Expect(err).To(BeNil())
	Expect(len(resultLabels)).To(Equal(0)) // Empty map
}

// TestConvertCluster_SpecMarshaling tests complex spec with nested objects
func TestConvertCluster_SpecMarshaling(t *testing.T) {
	RegisterTestingT(t)

	complexSpec := map[string]interface{}{
		"provider": "gcp",
		"region":   "us-east1",
		"config": map[string]interface{}{
			"nodes": 3,
			"networking": map[string]interface{}{
				"cidr": "10.0.0.0/16",
			},
		},
		"tags": []string{"production", "critical"},
	}

	req := &openapi.ClusterCreateRequest{
		Kind: util.PtrString("Cluster"),
		Name: "complex-cluster",
		Spec: complexSpec,
	}

	result, err := ConvertCluster(req, "user000")
	Expect(err).To(BeNil())

	var resultSpec map[string]interface{}
	err = json.Unmarshal(result.Spec, &resultSpec)
	Expect(err).To(BeNil())
	Expect(resultSpec["provider"]).To(Equal("gcp"))
	Expect(resultSpec["region"]).To(Equal("us-east1"))

	// Verify nested config
	config := resultSpec["config"].(map[string]interface{})
	Expect(config["nodes"]).To(BeNumerically("==", 3))

	networking := config["networking"].(map[string]interface{})
	Expect(networking["cidr"]).To(Equal("10.0.0.0/16"))

	// Verify tags array
	tags := resultSpec["tags"].([]interface{})
	Expect(len(tags)).To(Equal(2))
}

// TestPresentCluster_Complete tests presentation with all fields
func TestPresentCluster_Complete(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	reason := "Ready"
	message := "Cluster is ready"

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

	spec := map[string]interface{}{"region": "us-west1"}
	specJSON, _ := json.Marshal(spec)

	labels := map[string]string{"env": "staging"}
	labelsJSON, _ := json.Marshal(labels)

	cluster := &api.Cluster{
		Kind:                     "Cluster",
		Href:                     "/api/hyperfleet/v1/clusters/cluster-abc123",
		Name:                     "presented-cluster",
		Spec:                     specJSON,
		Labels:                   labelsJSON,
		Generation:               10,
		StatusPhase:              "Ready",
		StatusObservedGeneration: 5,
		StatusConditions:         conditionsJSON,
		StatusLastTransitionTime: &now,
		StatusLastUpdatedTime:    &now,
		CreatedBy:                "user123@example.com",
		UpdatedBy:                "user456@example.com",
	}
	cluster.ID = "cluster-abc123"
	cluster.CreatedTime = now
	cluster.UpdatedTime = now

	result, err := PresentCluster(cluster)
	Expect(err).To(BeNil())

	// Verify basic fields
	Expect(*result.Id).To(Equal("cluster-abc123"))
	Expect(*result.Kind).To(Equal("Cluster"))
	Expect(*result.Href).To(Equal("/api/hyperfleet/v1/clusters/cluster-abc123"))
	Expect(result.Name).To(Equal("presented-cluster"))
	Expect(result.Generation).To(Equal(int32(10)))
	Expect(result.CreatedBy).To(Equal(openapi_types.Email("user123@example.com")))
	Expect(result.UpdatedBy).To(Equal(openapi_types.Email("user456@example.com")))

	// Verify Spec unmarshaled correctly
	Expect(result.Spec["region"]).To(Equal("us-west1"))

	// Verify Labels unmarshaled correctly
	Expect((*result.Labels)["env"]).To(Equal("staging"))

	// Verify Status
	Expect(result.Status.Phase).To(Equal(openapi.Ready))
	Expect(result.Status.ObservedGeneration).To(Equal(int32(5)))
	Expect(len(result.Status.Conditions)).To(Equal(1))
	Expect(result.Status.Conditions[0].Type).To(Equal("Available"))
	Expect(result.Status.Conditions[0].Status).To(Equal(openapi.True))
	Expect(*result.Status.Conditions[0].Reason).To(Equal("Ready"))

	// Verify timestamps
	Expect(result.CreatedTime.Unix()).To(Equal(now.Unix()))
	Expect(result.UpdatedTime.Unix()).To(Equal(now.Unix()))
	Expect(result.Status.LastTransitionTime.Unix()).To(Equal(now.Unix()))
	Expect(result.Status.LastUpdatedTime.Unix()).To(Equal(now.Unix()))
}

// TestPresentCluster_HrefGeneration tests that Href is generated if not set
func TestPresentCluster_HrefGeneration(t *testing.T) {
	RegisterTestingT(t)

	cluster := &api.Cluster{
		Kind:             "Cluster",
		Href:             "", // Empty Href
		Name:             "href-test",
		Spec:             []byte("{}"),
		Labels:           []byte("{}"),
		StatusConditions: []byte("[]"),
	}
	cluster.ID = "cluster-xyz789"

	result, err := PresentCluster(cluster)
	Expect(err).To(BeNil())

	Expect(*result.Href).To(Equal("/api/hyperfleet/v1/clusters/cluster-xyz789"))
}

// TestPresentCluster_EmptyStatusPhase tests handling of empty StatusPhase
func TestPresentCluster_EmptyStatusPhase(t *testing.T) {
	RegisterTestingT(t)

	cluster := &api.Cluster{
		Kind:             "Cluster",
		Name:             "empty-phase-test",
		Spec:             []byte("{}"),
		Labels:           []byte("{}"),
		StatusPhase:      "", // Empty status phase
		StatusConditions: []byte("[]"),
	}
	cluster.ID = "cluster-empty-phase"

	result, err := PresentCluster(cluster)
	Expect(err).To(BeNil())

	// Should use NOT_READY as default
	Expect(result.Status.Phase).To(Equal(openapi.NotReady))
}

// TestPresentCluster_NilStatusTimestamps tests handling of nil timestamps
func TestPresentCluster_NilStatusTimestamps(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	cluster := &api.Cluster{
		Kind:                     "Cluster",
		Name:                     "nil-timestamps-test",
		Spec:                     []byte("{}"),
		Labels:                   []byte("{}"),
		StatusConditions:         []byte("[]"),
		StatusLastTransitionTime: nil, // Nil timestamp
		StatusLastUpdatedTime:    nil, // Nil timestamp
	}
	cluster.ID = "cluster-nil-timestamps"
	cluster.CreatedTime = now
	cluster.UpdatedTime = now

	result, err := PresentCluster(cluster)
	Expect(err).To(BeNil())

	// Verify zero time.Time is returned (not nil)
	Expect(result.Status.LastTransitionTime.IsZero()).To(BeTrue())
	Expect(result.Status.LastUpdatedTime.IsZero()).To(BeTrue())
}

// TestPresentCluster_StatusConditionsConversion tests condition conversion
func TestPresentCluster_StatusConditionsConversion(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	reason1 := "Ready"
	message1 := "All systems operational"
	reason2 := "Degraded"
	message2 := "Some components unavailable"

	// Create multiple domain ResourceConditions
	conditions := []api.ResourceCondition{
		{
			ObservedGeneration: 3,
			CreatedTime:        now,
			LastUpdatedTime:    now,
			Type:               "Available",
			Status:             api.ConditionTrue,
			Reason:             &reason1,
			Message:            &message1,
			LastTransitionTime: now,
		},
		{
			ObservedGeneration: 3,
			CreatedTime:        now,
			LastUpdatedTime:    now,
			Type:               "Progressing",
			Status:             api.ConditionFalse,
			Reason:             &reason2,
			Message:            &message2,
			LastTransitionTime: now,
		},
	}
	conditionsJSON, _ := json.Marshal(conditions)

	cluster := &api.Cluster{
		Kind:             "Cluster",
		Name:             "multi-conditions-test",
		Spec:             []byte("{}"),
		Labels:           []byte("{}"),
		StatusConditions: conditionsJSON,
	}
	cluster.ID = "cluster-multi-conditions"
	cluster.CreatedTime = now
	cluster.UpdatedTime = now

	result, err := PresentCluster(cluster)
	Expect(err).To(BeNil())

	// Verify both conditions converted correctly
	Expect(len(result.Status.Conditions)).To(Equal(2))

	// First condition
	Expect(result.Status.Conditions[0].Type).To(Equal("Available"))
	Expect(result.Status.Conditions[0].Status).To(Equal(openapi.True))
	Expect(*result.Status.Conditions[0].Reason).To(Equal("Ready"))
	Expect(*result.Status.Conditions[0].Message).To(Equal("All systems operational"))

	// Second condition
	Expect(result.Status.Conditions[1].Type).To(Equal("Progressing"))
	Expect(result.Status.Conditions[1].Status).To(Equal(openapi.False))
	Expect(*result.Status.Conditions[1].Reason).To(Equal("Degraded"))
	Expect(*result.Status.Conditions[1].Message).To(Equal("Some components unavailable"))
}

// TestConvertAndPresentCluster_RoundTrip tests data integrity through convert and present
func TestConvertAndPresentCluster_RoundTrip(t *testing.T) {
	RegisterTestingT(t)

	originalReq := createTestClusterRequest()
	createdBy := "user999@example.com"

	// Convert from OpenAPI request to domain
	cluster, err := ConvertCluster(originalReq, createdBy)
	Expect(err).To(BeNil())

	// Simulate database fields (ID, timestamps)
	cluster.ID = "cluster-roundtrip-123"
	now := time.Now()
	cluster.CreatedTime = now
	cluster.UpdatedTime = now

	// Present from domain back to OpenAPI
	result, err := PresentCluster(cluster)
	Expect(err).To(BeNil())

	// Verify data integrity
	Expect(*result.Id).To(Equal("cluster-roundtrip-123"))
	Expect(result.Kind).To(Equal(originalReq.Kind))
	Expect(result.Name).To(Equal(originalReq.Name))
	Expect(result.CreatedBy).To(Equal(openapi_types.Email(createdBy)))
	Expect(result.UpdatedBy).To(Equal(openapi_types.Email(createdBy)))

	// Verify Spec preserved
	Expect(result.Spec["region"]).To(Equal(originalReq.Spec["region"]))
	Expect(result.Spec["provider"]).To(Equal(originalReq.Spec["provider"]))

	// Verify Labels preserved
	Expect((*result.Labels)["env"]).To(Equal((*originalReq.Labels)["env"]))

	// Verify Status defaults
	Expect(result.Status.Phase).To(Equal(openapi.NotReady))
	Expect(result.Status.ObservedGeneration).To(Equal(int32(0)))
	Expect(len(result.Status.Conditions)).To(Equal(0))
}

// TestPresentCluster_MalformedSpec tests error handling for malformed Spec JSON
func TestPresentCluster_MalformedSpec(t *testing.T) {
	RegisterTestingT(t)

	cluster := &api.Cluster{
		Kind:             "Cluster",
		Name:             "malformed-spec-cluster",
		Spec:             []byte("{invalid json}"), // Malformed JSON
		Labels:           []byte("{}"),
		StatusConditions: []byte("[]"),
	}
	cluster.ID = "cluster-malformed-spec"

	_, err := PresentCluster(cluster)

	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("failed to unmarshal cluster spec"))
}

// TestPresentCluster_MalformedLabels tests error handling for malformed Labels JSON
func TestPresentCluster_MalformedLabels(t *testing.T) {
	RegisterTestingT(t)

	cluster := &api.Cluster{
		Kind:             "Cluster",
		Name:             "malformed-labels-cluster",
		Spec:             []byte("{}"),
		Labels:           []byte("{not valid json"), // Malformed JSON
		StatusConditions: []byte("[]"),
	}
	cluster.ID = "cluster-malformed-labels"

	_, err := PresentCluster(cluster)

	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("failed to unmarshal cluster labels"))
}

// TestPresentCluster_MalformedStatusConditions tests error handling for malformed StatusConditions JSON
func TestPresentCluster_MalformedStatusConditions(t *testing.T) {
	RegisterTestingT(t)

	cluster := &api.Cluster{
		Kind:             "Cluster",
		Name:             "malformed-conditions-cluster",
		Spec:             []byte("{}"),
		Labels:           []byte("{}"),
		StatusConditions: []byte("[{incomplete"), // Malformed JSON
	}
	cluster.ID = "cluster-malformed-conditions"

	_, err := PresentCluster(cluster)

	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("failed to unmarshal cluster status conditions"))
}
