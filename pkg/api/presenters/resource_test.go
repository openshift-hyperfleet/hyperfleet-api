package presenters

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"gorm.io/datatypes"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
)

func TestConvertResource(t *testing.T) {
	RegisterTestingT(t)

	labels := map[string]string{"env": "prod"}
	req := &openapi.ResourceCreateRequest{
		Kind:   "Channel",
		Name:   "stable",
		Spec:   map[string]any{"is_default": true, "enabled_regex": "4\\.17\\..*"},
		Labels: &labels,
	}

	resource, err := ConvertResource(req)
	Expect(err).NotTo(HaveOccurred())
	Expect(resource.Kind).To(Equal("Channel"))
	Expect(resource.Name).To(Equal("stable"))
	Expect(resource.Spec).NotTo(BeEmpty())
	Expect(string(resource.Labels)).To(ContainSubstring("env"))
}

func TestConvertResource_NilLabels(t *testing.T) {
	RegisterTestingT(t)

	req := &openapi.ResourceCreateRequest{
		Kind: "Channel",
		Name: "test",
		Spec: map[string]any{"key": "value"},
	}

	resource, err := ConvertResource(req)
	Expect(err).NotTo(HaveOccurred())
	Expect(string(resource.Labels)).To(Equal("{}"))
}

func TestConvertResourceWithOwner(t *testing.T) {
	RegisterTestingT(t)

	req := &openapi.ResourceCreateRequest{
		Kind: "Version",
		Name: "4-17-3",
		Spec: map[string]any{"raw_version": "4.17.3"},
	}

	resource, err := ConvertResourceWithOwner(
		req,
		"parent-id", "Channel", "/api/hyperfleet/v1/channels/parent-id",
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(*resource.OwnerID).To(Equal("parent-id"))
	Expect(*resource.OwnerKind).To(Equal("Channel"))
	Expect(*resource.OwnerHref).To(Equal("/api/hyperfleet/v1/channels/parent-id"))
}

func TestPresentResource(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	resource := &api.Resource{
		Meta:       api.Meta{ID: "test-id", CreatedTime: now, UpdatedTime: now},
		Kind:       "Channel",
		Name:       "stable",
		Href:       "/api/hyperfleet/v1/channels/test-id",
		Spec:       datatypes.JSON(`{"is_default":true}`),
		Labels:     datatypes.JSON(`{"env":"prod"}`),
		Generation: 1,
		CreatedBy:  "user@test.com",
		UpdatedBy:  "user@test.com",
	}

	resp := PresentResource(resource)
	Expect(resp.Id).To(Equal("test-id"))
	Expect(resp.Kind).To(Equal("Channel"))
	Expect(resp.Name).To(Equal("stable"))
	Expect(*resp.Href).To(Equal("/api/hyperfleet/v1/channels/test-id"))
	Expect(resp.Spec).To(HaveKeyWithValue("is_default", true))
	Expect(*resp.Labels).To(HaveKeyWithValue("env", "prod"))
	Expect(resp.Generation).To(Equal(int32(1)))
	Expect(string(resp.CreatedBy)).To(Equal("user@test.com"))
	Expect(resp.OwnerReferences).To(BeNil())
	Expect(resp.Status.Conditions).NotTo(BeNil())
	Expect(resp.Status.Conditions).To(BeEmpty())
}

func TestPresentResource_StatusConditionsJSONEmptyArray(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	resource := &api.Resource{
		Meta:      api.Meta{ID: "wif-id", CreatedTime: now, UpdatedTime: now},
		Kind:      "Wifconfig",
		Name:      "test-wif",
		Spec:      datatypes.JSON(`{"project_id":"p1"}`),
		CreatedBy: "user@test.com",
		UpdatedBy: "user@test.com",
	}

	resp := PresentResource(resource)
	body, err := json.Marshal(resp)
	Expect(err).NotTo(HaveOccurred())
	Expect(string(body)).To(ContainSubstring(`"status":{"conditions":[]}`))
}

func TestPresentResource_WithOwner(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	ownerID := "parent-id"
	ownerKind := "Channel"
	ownerHref := "/api/hyperfleet/v1/channels/parent-id"
	resource := &api.Resource{
		Meta:      api.Meta{ID: "child-id", CreatedTime: now, UpdatedTime: now},
		Kind:      "Version",
		Name:      "4-17-1",
		Spec:      datatypes.JSON(`{}`),
		OwnerID:   &ownerID,
		OwnerKind: &ownerKind,
		OwnerHref: &ownerHref,
		CreatedBy: "user@test.com",
		UpdatedBy: "user@test.com",
	}

	resp := PresentResource(resource)
	Expect(resp.OwnerReferences).NotTo(BeNil())
	Expect(*resp.OwnerReferences.Id).To(Equal("parent-id"))
	Expect(resp.OwnerReferences.Kind).To(Equal("Channel"))
}

func TestPresentResource_EmptySpec(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	resource := &api.Resource{
		Meta:      api.Meta{ID: "id", CreatedTime: now, UpdatedTime: now},
		Kind:      "Channel",
		Name:      "test",
		Spec:      datatypes.JSON(`{}`),
		CreatedBy: "user@test.com",
		UpdatedBy: "user@test.com",
	}

	resp := PresentResource(resource)
	Expect(resp.Spec).To(BeEmpty())
}

func TestPresentResource_WithConditions(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now().Truncate(time.Microsecond)
	reason := "AllAdaptersReporting"
	message := "All adapters are available"
	resource := &api.Resource{
		Meta:      api.Meta{ID: "cond-id", CreatedTime: now, UpdatedTime: now},
		Kind:      "Channel",
		Name:      "test",
		Spec:      datatypes.JSON(`{}`),
		CreatedBy: "user@test.com",
		UpdatedBy: "user@test.com",
		Conditions: []api.ResourceCondition{
			{
				Type:               "Available",
				Status:             api.ConditionTrue,
				Reason:             &reason,
				Message:            &message,
				ObservedGeneration: 3,
				CreatedTime:        now,
				LastUpdatedTime:    now,
				LastTransitionTime: now,
			},
		},
	}

	resp := PresentResource(resource)
	Expect(resp.Status.Conditions).To(HaveLen(1))

	cond := resp.Status.Conditions[0]
	Expect(cond.Type).To(Equal("Available"))
	Expect(cond.Status).To(Equal(openapi.ResourceConditionStatusTrue))
	Expect(*cond.Reason).To(Equal("AllAdaptersReporting"))
	Expect(*cond.Message).To(Equal("All adapters are available"))
	Expect(cond.ObservedGeneration).To(Equal(int32(3)))
	Expect(cond.CreatedTime).To(BeTemporally("==", now))
	Expect(cond.LastTransitionTime).To(BeTemporally("==", now))
}

func TestPresentResource_WithEmptyConditions(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	resource := &api.Resource{
		Meta:       api.Meta{ID: "empty-cond-id", CreatedTime: now, UpdatedTime: now},
		Kind:       "Channel",
		Name:       "test",
		Spec:       datatypes.JSON(`{}`),
		CreatedBy:  "user@test.com",
		UpdatedBy:  "user@test.com",
		Conditions: []api.ResourceCondition{},
	}

	resp := PresentResource(resource)
	body, err := json.Marshal(resp)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.Status.Conditions).NotTo(BeNil())
	Expect(resp.Status.Conditions).To(BeEmpty())
	Expect(string(body)).To(ContainSubstring(`"status":{"conditions":[]}`))
}

func TestPresentResourceList(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	resources := api.ResourceList{
		&api.Resource{
			Meta: api.Meta{ID: "id1", CreatedTime: now, UpdatedTime: now},
			Kind: "Channel", Name: "stable",
			Spec: datatypes.JSON(`{}`), CreatedBy: "u@t.com", UpdatedBy: "u@t.com",
		},
		&api.Resource{
			Meta: api.Meta{ID: "id2", CreatedTime: now, UpdatedTime: now},
			Kind: "Channel", Name: "candidate",
			Spec: datatypes.JSON(`{}`), CreatedBy: "u@t.com", UpdatedBy: "u@t.com",
		},
	}
	paging := &api.PagingMeta{Page: 1, Size: 2, Total: 2}

	result := PresentResourceList(resources, paging)
	Expect(result.Items).To(HaveLen(2))
	Expect(result.Page).To(Equal(int32(1)))
	Expect(result.Size).To(Equal(int32(2)))
	Expect(result.Total).To(Equal(int64(2)))
}
