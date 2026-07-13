package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	"gopkg.in/resty.v1"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

const channelsPath = "/channels"

func createTestChannelForStatus(t *testing.T, svc services.ResourceService) *api.Resource {
	t.Helper()
	name := fmt.Sprintf("status-ch-%s", uuid.NewString()[:8])
	channel := newChannelResource(name)
	created, svcErr := svc.Create(context.Background(), "Channel", channel, nil)
	if svcErr != nil {
		t.Fatalf("Failed to create channel for status test: %v", svcErr)
	}
	return created
}

func TestResourceStatus_PutAndGet(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupResourceTest(t)

	channel := createTestChannelForStatus(t, svc)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	token := test.GetAccessTokenFromContext(ctx)

	// PUT adapter status
	statusReq := newAdapterStatusRequest(
		"test-adapter", channel.Generation,
		[]openapi.ConditionRequest{
			{Type: api.AdapterConditionTypeAvailable, Status: openapi.AdapterConditionStatusTrue},
			{Type: api.AdapterConditionTypeApplied, Status: openapi.AdapterConditionStatusTrue},
			{Type: api.AdapterConditionTypeHealth, Status: openapi.AdapterConditionStatusTrue},
		},
		nil,
	)
	body, err := json.Marshal(statusReq)
	Expect(err).NotTo(HaveOccurred())

	putResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", token)).
		SetBody(body).
		Put(h.RestURL(channelsPath + "/" + channel.ID + "/statuses"))
	Expect(err).NotTo(HaveOccurred())
	Expect(putResp.StatusCode()).To(Equal(http.StatusCreated))

	// GET adapter statuses
	getResp, err := resty.R().
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", token)).
		Get(h.RestURL(channelsPath + "/" + channel.ID + "/statuses"))
	Expect(err).NotTo(HaveOccurred())
	Expect(getResp.StatusCode()).To(Equal(http.StatusOK))

	var statusList openapi.AdapterStatusList
	Expect(json.Unmarshal(getResp.Body(), &statusList)).To(Succeed())
	Expect(statusList.Items).To(HaveLen(1))
	Expect(statusList.Items[0].Adapter).To(Equal("test-adapter"))
}

func TestResourceStatus_PutTriggersConditionsAggregation(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupResourceTest(t)

	channel := createTestChannelForStatus(t, svc)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	token := test.GetAccessTokenFromContext(ctx)

	statusReq := newAdapterStatusRequest(
		"test-adapter", channel.Generation,
		[]openapi.ConditionRequest{
			{Type: api.AdapterConditionTypeAvailable, Status: openapi.AdapterConditionStatusTrue},
			{Type: api.AdapterConditionTypeApplied, Status: openapi.AdapterConditionStatusTrue},
			{Type: api.AdapterConditionTypeHealth, Status: openapi.AdapterConditionStatusTrue},
		},
		nil,
	)
	body, _ := json.Marshal(statusReq)

	putResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", token)).
		SetBody(body).
		Put(h.RestURL(channelsPath + "/" + channel.ID + "/statuses"))
	Expect(err).NotTo(HaveOccurred())
	Expect(putResp.StatusCode()).To(Equal(http.StatusCreated))

	// GET the resource and verify conditions are populated
	resource, svcErr := svc.Get(context.Background(), "Channel", channel.ID)
	Expect(svcErr).To(BeNil())
	Expect(resource.Conditions).ToNot(BeEmpty(), "Conditions should be populated after adapter status report")
}

func TestResourceStatus_NonExistentResource_Returns404(t *testing.T) {
	RegisterTestingT(t)
	_, h := setupResourceTest(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	token := test.GetAccessTokenFromContext(ctx)

	// GET statuses for non-existent resource
	getResp, err := resty.R().
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", token)).
		Get(h.RestURL(channelsPath + "/nonexistent-id/statuses"))
	Expect(err).NotTo(HaveOccurred())
	Expect(getResp.StatusCode()).To(Equal(http.StatusNotFound))
}

func TestResourceStatus_NestedEntityStatuses(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupResourceTest(t)

	channel := createTestChannelForStatus(t, svc)

	versionName := fmt.Sprintf("status-ver-%s", uuid.NewString()[:8])
	version := newVersionResource(versionName, channel.ID)
	createdVersion, svcErr := svc.Create(context.Background(), "Version", version, nil)
	Expect(svcErr).To(BeNil())

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	token := test.GetAccessTokenFromContext(ctx)

	statusReq := newAdapterStatusRequest(
		"version-adapter", createdVersion.Generation,
		[]openapi.ConditionRequest{
			{Type: api.AdapterConditionTypeAvailable, Status: openapi.AdapterConditionStatusTrue},
			{Type: api.AdapterConditionTypeApplied, Status: openapi.AdapterConditionStatusTrue},
			{Type: api.AdapterConditionTypeHealth, Status: openapi.AdapterConditionStatusTrue},
		},
		nil,
	)
	body, _ := json.Marshal(statusReq)

	// PUT via nested path
	nestedPath := fmt.Sprintf("%s/%s/versions/%s/statuses", channelsPath, channel.ID, createdVersion.ID)
	putResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", token)).
		SetBody(body).
		Put(h.RestURL(nestedPath))
	Expect(err).NotTo(HaveOccurred())
	Expect(putResp.StatusCode()).To(Equal(http.StatusCreated))

	// GET via nested path
	getResp, err := resty.R().
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", token)).
		Get(h.RestURL(nestedPath))
	Expect(err).NotTo(HaveOccurred())
	Expect(getResp.StatusCode()).To(Equal(http.StatusOK))

	var statusList openapi.AdapterStatusList
	Expect(json.Unmarshal(getResp.Body(), &statusList)).To(Succeed())
	Expect(statusList.Items).To(HaveLen(1))
	Expect(statusList.Items[0].Adapter).To(Equal("version-adapter"))
}

func TestResourceStatus_DiscardedUpdate_Returns204(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupResourceTest(t)

	channel := createTestChannelForStatus(t, svc)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	token := test.GetAccessTokenFromContext(ctx)

	// PUT with future generation → discarded → 204
	statusReq := newAdapterStatusRequest(
		"test-adapter", 999,
		[]openapi.ConditionRequest{
			{Type: api.AdapterConditionTypeAvailable, Status: openapi.AdapterConditionStatusTrue},
			{Type: api.AdapterConditionTypeApplied, Status: openapi.AdapterConditionStatusTrue},
			{Type: api.AdapterConditionTypeHealth, Status: openapi.AdapterConditionStatusTrue},
		},
		nil,
	)
	body, _ := json.Marshal(statusReq)

	putResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", token)).
		SetBody(body).
		Put(h.RestURL(channelsPath + "/" + channel.ID + "/statuses"))
	Expect(err).NotTo(HaveOccurred())
	Expect(putResp.StatusCode()).To(Equal(http.StatusNoContent))
}
