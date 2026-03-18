package presenters

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

const clusterKind = "Cluster"

func createEmptyConditionsClusterList() openapi.ClusterList {
	id := "cluster-empty"
	return openapi.ClusterList{
		Kind:  "ClusterList",
		Page:  1,
		Size:  1,
		Total: 1,
		Items: []openapi.Cluster{
			{Id: &id, Status: openapi.ClusterStatus{Conditions: []openapi.ResourceCondition{}}},
		},
	}
}

func createTestClusterList() openapi.ClusterList {
	id1 := "cluster-id1"
	id2 := "cluster-id2"
	kind := clusterKind

	labels1 := map[string]string{
		"env":  "prod",
		"team": "platform",
	}
	labels2 := map[string]string{
		"env": "dev",
	}

	now := time.Now()
	msg1 := "All checks passed"
	msg2 := "Some components unavailable"
	conditions := []openapi.ResourceCondition{
		{
			Type:               "Ready",
			Status:             openapi.ResourceConditionStatus("True"),
			Message:            &msg1,
			CreatedTime:        time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC),
			LastTransitionTime: now,
			LastUpdatedTime:    now,
		},
		{
			Type:               "Progressing",
			Status:             openapi.ResourceConditionStatus("False"),
			Message:            &msg2,
			CreatedTime:        time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC),
			LastTransitionTime: now,
			LastUpdatedTime:    now,
		},
	}

	return openapi.ClusterList{
		Kind:  "ClusterList",
		Page:  1,
		Size:  2,
		Total: 2,
		Items: []openapi.Cluster{
			{
				Id:          &id1,
				Kind:        &kind,
				Name:        "test-cluster",
				Generation:  1,
				Labels:      &labels1,
				CreatedTime: now,
				UpdatedTime: now,
				Spec:        openapi.ClusterSpec{"region": "us-east-1"},
				Status:      openapi.ClusterStatus{Conditions: conditions},
			},
			{
				Id:          &id2,
				Kind:        &kind,
				Name:        "development-cluster",
				Generation:  2,
				Labels:      &labels2,
				CreatedTime: now,
				UpdatedTime: now,
				Spec:        openapi.ClusterSpec{"region": "eu-west-1"},
				Status:      openapi.ClusterStatus{Conditions: conditions},
			},
		},
	}
}

func TestSliceFilter(t *testing.T) {
	tests := []struct {
		model    interface{}
		validate func(result *ProjectionList, err *errors.ServiceError)
		name     string
		fields   []string
	}{
		{
			name:   "include and exclude fields with different types",
			fields: []string{"id", "name", "generation"},
			model:  createTestClusterList(),
			validate: func(result *ProjectionList, err *errors.ServiceError) {
				Expect(err).To(BeNil())
				Expect(result).ToNot(BeNil())
				Expect(result.Kind).To(Equal("ClusterList"))
				Expect(result.Page).To(Equal(int32(1)))
				Expect(result.Size).To(Equal(int32(2)))
				Expect(result.Total).To(Equal(int32(2)))
				Expect(result.Items).To(HaveLen(2))

				// Included fields
				id1 := result.Items[0]["id"].(*string)
				Expect(*id1).To(Equal("cluster-id1"))
				Expect(result.Items[0]["name"]).To(Equal("test-cluster"))
				Expect(result.Items[0]["generation"]).To(Equal(int32(1)))

				id2 := result.Items[1]["id"].(*string)
				Expect(*id2).To(Equal("cluster-id2"))
				Expect(result.Items[1]["name"]).To(Equal("development-cluster"))
				Expect(result.Items[1]["generation"]).To(Equal(int32(2)))

				// Excluded fields
				Expect(result.Items[0]).ToNot(HaveKey("labels"))
				Expect(result.Items[0]).ToNot(HaveKey("spec"))
				Expect(result.Items[0]).ToNot(HaveKey("created_time"))
			},
		},
		{
			name:   "nested field handling",
			fields: []string{"id", "name", "labels", "spec"},
			model:  createTestClusterList(),
			validate: func(result *ProjectionList, err *errors.ServiceError) {
				Expect(err).To(BeNil())
				Expect(result).ToNot(BeNil())
				Expect(result.Items).To(HaveLen(2))

				id1 := result.Items[0]["id"].(*string)
				Expect(*id1).To(Equal("cluster-id1"))
				Expect(result.Items[0]["name"]).To(Equal("test-cluster"))

				// Verify nested labels map is included
				labels := result.Items[0]["labels"].(*map[string]string)
				Expect((*labels)["env"]).To(Equal("prod"))
				Expect((*labels)["team"]).To(Equal("platform"))
				Expect(result.Items[0]["spec"]).To(Equal(openapi.ClusterSpec{"region": "us-east-1"}))

				id2 := result.Items[1]["id"].(*string)
				Expect(*id2).To(Equal("cluster-id2"))
				labels2 := result.Items[1]["labels"].(*map[string]string)
				Expect((*labels2)["env"]).To(Equal("dev"))
				Expect(result.Items[1]["spec"]).To(Equal(openapi.ClusterSpec{"region": "eu-west-1"}))

				Expect(result.Items[0]).ToNot(HaveKey("generation"))
			},
		},
		{
			name:   "time field handling",
			fields: []string{"id", "created_time"},
			model:  createTestClusterList(),
			validate: func(result *ProjectionList, err *errors.ServiceError) {
				Expect(err).To(BeNil())
				Expect(result).ToNot(BeNil())
				Expect(result.Items).To(HaveLen(2))

				id1 := result.Items[0]["id"].(*string)
				Expect(*id1).To(Equal("cluster-id1"))

				createdTime, ok := result.Items[0]["created_time"].(string)
				Expect(ok).To(BeTrue(), "created_time should be a string")
				Expect(createdTime).ToNot(BeEmpty())

				// Verify it's valid RFC3339 format
				parsedTime, parseErr := time.Parse(time.RFC3339, createdTime)
				Expect(parseErr).To(BeNil(), "created_time should be valid RFC3339 format")
				Expect(parsedTime.IsZero()).To(BeFalse(), "parsed time should not be zero")
			},
		},
		{
			name:   "nil input",
			fields: []string{"id"},
			model:  nil,
			validate: func(result *ProjectionList, err *errors.ServiceError) {
				Expect(result).To(BeNil())
				Expect(err).ToNot(BeNil())
				Expect(err.Type).To(Equal(errors.ErrorTypeValidation))
				Expect(err.Error()).To(ContainSubstring("Empty model"))
			},
		},
		{
			name:   "empty field list",
			fields: []string{},
			model:  createTestClusterList(),
			validate: func(result *ProjectionList, err *errors.ServiceError) {
				Expect(err).To(BeNil())
				Expect(result).ToNot(BeNil())
				Expect(result.Kind).To(Equal("ClusterList"))
				Expect(result.Page).To(Equal(int32(1)))
				Expect(result.Items).To(HaveLen(2))

				// No fields requested, so items should be empty maps
				Expect(result.Items[0]).To(HaveLen(0))
				Expect(result.Items[1]).To(HaveLen(0))
			},
		},
		{
			name:   "nil field list",
			fields: nil,
			model:  createTestClusterList(),
			validate: func(result *ProjectionList, err *errors.ServiceError) {
				Expect(err).To(BeNil())
				Expect(result).ToNot(BeNil())
				Expect(result.Kind).To(Equal("ClusterList"))
				Expect(result.Items).To(HaveLen(2))
				Expect(result.Items[0]).To(HaveLen(0))
			},
		},
		{
			name:   "invalid field name",
			fields: []string{"id", "nonexistent_field"},
			model:  createTestClusterList(),
			validate: func(result *ProjectionList, err *errors.ServiceError) {
				Expect(result).To(BeNil())
				Expect(err).ToNot(BeNil())
				Expect(err.Type).To(Equal(errors.ErrorTypeValidation))
				Expect(err.Error()).To(ContainSubstring("doesn't exist"))
				Expect(err.Error()).To(ContainSubstring("nonexistent_field"))
			},
		},
		{
			name:   "valid sub-field of slice element",
			fields: []string{"id", "status.conditions.type"},
			model:  createTestClusterList(),
			validate: func(result *ProjectionList, err *errors.ServiceError) {
				Expect(err).To(BeNil())
				Expect(result).ToNot(BeNil())
				Expect(result.Items).To(HaveLen(2))
				status := result.Items[0]["status"].(map[string]interface{})
				conditions := status["conditions"].([]interface{})
				Expect(conditions).To(HaveLen(2))

				elem0 := conditions[0].(map[string]interface{})
				Expect(elem0["type"]).To(Equal("Ready"))
				Expect(elem0).ToNot(HaveKey("message"))

				elem1 := conditions[1].(map[string]interface{})
				Expect(elem1["type"]).To(Equal("Progressing"))
				Expect(elem1).ToNot(HaveKey("message"))
			},
		},
		{
			name:   "multiple sub-fields from same slice element",
			fields: []string{"id", "status.conditions.type", "status.conditions.status"},
			model:  createTestClusterList(),
			validate: func(result *ProjectionList, err *errors.ServiceError) {
				Expect(err).To(BeNil())
				Expect(result).ToNot(BeNil())
				Expect(result.Items).To(HaveLen(2))
				status := result.Items[0]["status"].(map[string]interface{})
				conditions := status["conditions"].([]interface{})
				Expect(conditions).To(HaveLen(2))

				elem0 := conditions[0].(map[string]interface{})
				Expect(elem0["type"]).To(Equal("Ready"))
				Expect(elem0["status"]).To(Equal(openapi.ResourceConditionStatus("True")))
				Expect(elem0).ToNot(HaveKey("message"))
				Expect(elem0).ToNot(HaveKey("last_transition_time"))

				elem1 := conditions[1].(map[string]interface{})
				Expect(elem1["type"]).To(Equal("Progressing"))
				Expect(elem1["status"]).To(Equal(openapi.ResourceConditionStatus("False")))
				Expect(elem1).ToNot(HaveKey("message"))
				Expect(elem1).ToNot(HaveKey("last_transition_time"))
			},
		},
		{
			name:   "time sub-field of slice element",
			fields: []string{"id", "status.conditions.created_time"},
			model:  createTestClusterList(),
			validate: func(result *ProjectionList, err *errors.ServiceError) {
				Expect(err).To(BeNil())
				Expect(result).ToNot(BeNil())
				status := result.Items[0]["status"].(map[string]interface{})
				conditions := status["conditions"].([]interface{})
				elem0 := conditions[0].(map[string]interface{})
				Expect(elem0).ToNot(HaveKey("type"))
				ltt0, ok := elem0["created_time"].(string)
				Expect(ok).To(BeTrue(), "created_time should be a string")
				Expect(ltt0).To(Equal("2026-01-25T00:00:00Z"))

				elem1 := conditions[1].(map[string]interface{})
				Expect(elem1).ToNot(HaveKey("type"))
				ltt1, ok := elem1["created_time"].(string)
				Expect(ok).To(BeTrue(), "created_time should be a string")
				Expect(ltt1).To(Equal("2026-03-11T00:00:00Z"))
			},
		},
		{
			name:   "invalid sub-field of slice element returns error",
			fields: []string{"id", "status.conditions.nonexistent_field"},
			model:  createTestClusterList(),
			validate: func(result *ProjectionList, err *errors.ServiceError) {
				Expect(result).To(BeNil())
				Expect(err).ToNot(BeNil())
				Expect(err.Type).To(Equal(errors.ErrorTypeValidation))
				Expect(err.Error()).To(ContainSubstring("doesn't exist"))
				Expect(err.Error()).To(ContainSubstring("nonexistent_field"))
			},
		},
		{
			name:   "invalid sub-field alongside slice star selector returns error",
			fields: []string{"id", "status.conditions.*", "status.conditions.nonexistent_field"},
			model:  createTestClusterList(),
			validate: func(result *ProjectionList, err *errors.ServiceError) {
				Expect(result).To(BeNil())
				Expect(err).ToNot(BeNil())
				Expect(err.Type).To(Equal(errors.ErrorTypeValidation))
				Expect(err.Error()).To(ContainSubstring("nonexistent_field"))
			},
		},
		{
			name:   "invalid sub-field alongside struct star selector returns error",
			fields: []string{"id", "status.*", "status.nonexistent_field"},
			model:  createTestClusterList(),
			validate: func(result *ProjectionList, err *errors.ServiceError) {
				Expect(result).To(BeNil())
				Expect(err).ToNot(BeNil())
				Expect(err.Type).To(Equal(errors.ErrorTypeValidation))
				Expect(err.Error()).To(ContainSubstring("nonexistent_field"))
			},
		},
		{
			name:   "parent star selector includes slice fields",
			fields: []string{"id", "status.*"},
			model:  createTestClusterList(),
			validate: func(result *ProjectionList, err *errors.ServiceError) {
				Expect(err).To(BeNil())
				Expect(result).ToNot(BeNil())
				Expect(result.Items).To(HaveLen(2))
				status := result.Items[0]["status"].(map[string]interface{})

				// status.* must include conditions slice with all element fields
				conditions, ok := status["conditions"].([]interface{})
				Expect(ok).To(BeTrue(), "conditions should be present under status.*")
				Expect(conditions).To(HaveLen(2))

				elem0 := conditions[0].(map[string]interface{})
				Expect(elem0).To(HaveLen(8))
				Expect(elem0["type"]).To(Equal("Ready"))
				Expect(elem0["status"]).To(Equal(openapi.ResourceConditionStatus("True")))
			},
		},
		{
			name:   "star selector for slice elements is valid",
			fields: []string{"id", "status.conditions.*"},
			model:  createTestClusterList(),
			validate: func(result *ProjectionList, err *errors.ServiceError) {
				Expect(err).To(BeNil())
				Expect(result).ToNot(BeNil())
				Expect(result.Items).To(HaveLen(2))
				status := result.Items[0]["status"].(map[string]interface{})
				conditions := status["conditions"].([]interface{})
				Expect(conditions).To(HaveLen(2))

				// All fields of ResourceCondition must be present
				elem0 := conditions[0].(map[string]interface{})
				Expect(elem0).To(HaveLen(8))
				Expect(elem0["type"]).To(Equal("Ready"))
				Expect(elem0["status"]).To(Equal(openapi.ResourceConditionStatus("True")))
				Expect(elem0).To(HaveKey("message"))

				elem1 := conditions[1].(map[string]interface{})
				Expect(elem1).To(HaveLen(8))
				Expect(elem1["type"]).To(Equal("Progressing"))
				Expect(elem1["status"]).To(Equal(openapi.ResourceConditionStatus("False")))
				Expect(elem1).To(HaveKey("message"))
			},
		},
		{
			name:   "requesting whole slice is valid",
			fields: []string{"id", "status.conditions"},
			model:  createTestClusterList(),
			validate: func(result *ProjectionList, err *errors.ServiceError) {
				Expect(err).To(BeNil())
				Expect(result).ToNot(BeNil())
				Expect(result.Items).To(HaveLen(2))
				// Bare slice request promotes to star, all fields must be returned
				status := result.Items[0]["status"].(map[string]interface{})
				conditions := status["conditions"].([]interface{})
				Expect(conditions).To(HaveLen(2))

				elem0 := conditions[0].(map[string]interface{})
				Expect(elem0).To(HaveLen(8))
				Expect(elem0).To(HaveKey("message"))
				Expect(elem0).To(HaveKey("last_transition_time"))

				elem1 := conditions[1].(map[string]interface{})
				Expect(elem1).To(HaveLen(8))
				Expect(elem1).To(HaveKey("message"))
				Expect(elem1).To(HaveKey("last_transition_time"))
			},
		},
		{
			name:   "requesting empty slice returns empty slice",
			fields: []string{"id", "status.conditions"},
			model:  createEmptyConditionsClusterList(),
			validate: func(result *ProjectionList, err *errors.ServiceError) {
				Expect(err).To(BeNil())
				Expect(result).ToNot(BeNil())
				status := result.Items[0]["status"].(map[string]interface{})
				Expect(status["conditions"]).To(Equal([]interface{}{}))
			},
		},
		{
			name:   "requesting sub-field of empty slice returns empty slice",
			fields: []string{"id", "status.conditions.type"},
			model:  createEmptyConditionsClusterList(),
			validate: func(result *ProjectionList, err *errors.ServiceError) {
				Expect(err).To(BeNil())
				Expect(result).ToNot(BeNil())
				status := result.Items[0]["status"].(map[string]interface{})
				Expect(status["conditions"]).To(Equal([]interface{}{}))
			},
		},
		{
			name:   "empty items - panic prevention",
			fields: []string{"id", "name"},
			model: openapi.ClusterList{
				Kind:  "ClusterList",
				Page:  1,
				Size:  0,
				Total: 0,
				Items: []openapi.Cluster{},
			},
			validate: func(result *ProjectionList, err *errors.ServiceError) {
				Expect(err).To(BeNil())
				Expect(result).ToNot(BeNil())
				Expect(result.Kind).To(Equal("ClusterList"))
				Expect(result.Page).To(Equal(int32(1)))
				Expect(result.Size).To(Equal(int32(0)))
				Expect(result.Total).To(Equal(int32(0)))
				Expect(result.Items).To(BeNil())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			result, err := SliceFilter(tt.fields, tt.model)
			tt.validate(result, err)
		})
	}
}
