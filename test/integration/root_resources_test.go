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

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

const resourcesPath = "/resources"

func rootResourceRequest(ctx context.Context) *resty.Request {
	jwtToken := test.GetAccessTokenFromContext(ctx)
	return resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken))
}

func TestRootResourceList(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupResourceTest(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	ch1 := createChannel(t, svc, fmt.Sprintf("list-ch1-%s", uuid.NewString()[:8]))
	ch2 := createChannel(t, svc, fmt.Sprintf("list-ch2-%s", uuid.NewString()[:8]))

	t.Run("ListsAllKinds", func(t *testing.T) {
		RegisterTestingT(t)
		resp, err := rootResourceRequest(ctx).
			Get(h.RestURL(resourcesPath))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusOK))

		var result openapi.ResourceList
		Expect(json.Unmarshal(resp.Body(), &result)).To(Succeed())
		Expect(result.Size).To(BeNumerically(">=", 2))

		ids := make(map[string]bool)
		for _, item := range result.Items {
			ids[item.Id] = true
		}
		Expect(ids).To(HaveKey(ch1.ID))
		Expect(ids).To(HaveKey(ch2.ID))
	})

	t.Run("FiltersByKind", func(t *testing.T) {
		RegisterTestingT(t)
		resp, err := rootResourceRequest(ctx).
			Get(h.RestURL(resourcesPath + "?kind=Channel"))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusOK))

		var result openapi.ResourceList
		Expect(json.Unmarshal(resp.Body(), &result)).To(Succeed())
		for _, item := range result.Items {
			Expect(item.Kind).To(Equal("Channel"))
		}
	})

	t.Run("RejectsUnknownKind", func(t *testing.T) {
		RegisterTestingT(t)
		resp, err := rootResourceRequest(ctx).
			Get(h.RestURL(resourcesPath + "?kind=NonExistent"))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusBadRequest))
	})
}

func TestRootResourceGetByID(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupResourceTest(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	channel := createChannel(t, svc, fmt.Sprintf("get-ch-%s", uuid.NewString()[:8]))

	t.Run("FetchesByID", func(t *testing.T) {
		RegisterTestingT(t)
		resp, err := rootResourceRequest(ctx).
			Get(h.RestURL(fmt.Sprintf("%s/%s", resourcesPath, channel.ID)))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusOK))

		var resource openapi.Resource
		Expect(json.Unmarshal(resp.Body(), &resource)).To(Succeed())
		Expect(resource.Id).To(Equal(channel.ID))
		Expect(resource.Kind).To(Equal("Channel"))
	})

	t.Run("NotFoundReturns404", func(t *testing.T) {
		RegisterTestingT(t)
		resp, err := rootResourceRequest(ctx).
			Get(h.RestURL(fmt.Sprintf("%s/%s", resourcesPath, uuid.NewString())))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusNotFound))
	})
}

func TestRootResourceCreate(t *testing.T) {
	RegisterTestingT(t)
	_, h := setupResourceTest(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	t.Run("CreatesTopLevelEntity", func(t *testing.T) {
		RegisterTestingT(t)
		input := openapi.ResourceCreateRequest{
			Kind: "Channel",
			Name: fmt.Sprintf("root-create-%s", uuid.NewString()[:8]),
			Spec: map[string]interface{}{
				"is_default":    false,
				"enabled_regex": ".*",
			},
		}
		body, err := json.Marshal(input)
		Expect(err).NotTo(HaveOccurred())

		resp, err := rootResourceRequest(ctx).
			SetBody(body).
			Post(h.RestURL(resourcesPath))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusCreated))

		var created map[string]interface{}
		Expect(json.Unmarshal(resp.Body(), &created)).To(Succeed())
		Expect(created["id"]).NotTo(BeEmpty())
		Expect(created["kind"]).To(Equal("Channel"))
		Expect(created["name"]).To(HavePrefix("root-create-"))
	})
}

func TestRootResourceCreateChildKindReturns422(t *testing.T) {
	RegisterTestingT(t)
	_, h := setupResourceTest(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	input := openapi.ResourceCreateRequest{
		Kind: "Version",
		Name: fmt.Sprintf("child-reject-%s", uuid.NewString()[:8]),
		Spec: map[string]interface{}{
			"raw_version":   "4.17.0",
			"enabled":       true,
			"is_default":    false,
			"release_image": "quay.io/openshift-release-dev/ocp-release:4.17.0",
		},
	}
	body, err := json.Marshal(input)
	Expect(err).NotTo(HaveOccurred())

	resp, err := rootResourceRequest(ctx).
		SetBody(body).
		Post(h.RestURL(resourcesPath))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusUnprocessableEntity))

	var problem openapi.ProblemDetails
	Expect(json.Unmarshal(resp.Body(), &problem)).To(Succeed())
	Expect(*problem.Detail).To(ContainSubstring("channels"))
}

func TestRootResourcePatch(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupResourceTest(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	channel := createChannel(t, svc, fmt.Sprintf("patch-ch-%s", uuid.NewString()[:8]))

	patchBody := `{"spec": {"is_default": true, "enabled_regex": ".*"}}`
	resp, err := rootResourceRequest(ctx).
		SetBody(patchBody).
		Patch(h.RestURL(fmt.Sprintf("%s/%s", resourcesPath, channel.ID)))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusOK))

	var patched map[string]interface{}
	Expect(json.Unmarshal(resp.Body(), &patched)).To(Succeed())
	spec := patched["spec"].(map[string]interface{})
	Expect(spec["is_default"]).To(BeTrue())
}

func TestRootResourceDelete(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupResourceTest(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	channel := createChannel(t, svc, fmt.Sprintf("delete-ch-%s", uuid.NewString()[:8]))

	resp, err := rootResourceRequest(ctx).
		Delete(h.RestURL(fmt.Sprintf("%s/%s", resourcesPath, channel.ID)))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusAccepted))
}

func TestRootResourcePatchSoftDeletedReturns404(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupResourceTest(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	channel := createChannel(t, svc, fmt.Sprintf("del409-ch-%s", uuid.NewString()[:8]))
	_, delErr := svc.Delete(t.Context(), "Channel", channel.ID)
	Expect(delErr).To(BeNil())

	patchBody := `{"spec": {"is_default": true, "enabled_regex": ".*"}}`
	resp, err := rootResourceRequest(ctx).
		SetBody(patchBody).
		Patch(h.RestURL(fmt.Sprintf("%s/%s", resourcesPath, channel.ID)))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusNotFound))
}

func TestRootResourceForceDeleteRoute(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupResourceTest(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	channel := createChannel(t, svc, fmt.Sprintf("fdel-ch-%s", uuid.NewString()[:8]))

	t.Run("MissingReasonReturns400", func(t *testing.T) {
		RegisterTestingT(t)
		resp, err := rootResourceRequest(ctx).
			SetBody(`{}`).
			Post(h.RestURL(fmt.Sprintf("%s/%s/force-delete", resourcesPath, channel.ID)))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusBadRequest))
	})

	t.Run("ReturnsExpectedStatus", func(t *testing.T) {
		RegisterTestingT(t)
		resp, err := rootResourceRequest(ctx).
			SetBody(`{"reason": "cleanup"}`).
			Post(h.RestURL(fmt.Sprintf("%s/%s/force-delete", resourcesPath, channel.ID)))
		Expect(err).NotTo(HaveOccurred())
		// 409 because Channel has no required adapters (never soft-deleted/finalizing)
		// Confirms the route is wired and reaches ForceDelete service method
		Expect(resp.StatusCode()).To(Equal(http.StatusConflict))
	})
}

func TestFlatChildRouteList(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupResourceTest(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	ch := createChannel(t, svc, fmt.Sprintf("flat-parent-%s", uuid.NewString()[:8]))
	v1Name := fmt.Sprintf("flat-v1-%s", uuid.NewString()[:8])
	v1 := newVersionResource(v1Name, ch.ID)
	created1, svcErr := svc.Create(t.Context(), "Version", v1, nil)
	Expect(svcErr).To(BeNil())

	resp, err := rootResourceRequest(ctx).
		Get(h.RestURL("/versions"))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusOK))

	var result openapi.ResourceList
	Expect(json.Unmarshal(resp.Body(), &result)).To(Succeed())

	ids := make(map[string]bool)
	for _, item := range result.Items {
		ids[item.Id] = true
		Expect(item.Kind).To(Equal("Version"))
	}
	Expect(ids).To(HaveKey(created1.ID))
}

func TestFlatChildRouteGetByID(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupResourceTest(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	ch := createChannel(t, svc, fmt.Sprintf("flat-get-parent-%s", uuid.NewString()[:8]))
	v := newVersionResource(fmt.Sprintf("flat-get-v-%s", uuid.NewString()[:8]), ch.ID)
	created, svcErr := svc.Create(t.Context(), "Version", v, nil)
	Expect(svcErr).To(BeNil())

	resp, err := rootResourceRequest(ctx).
		Get(h.RestURL(fmt.Sprintf("/versions/%s", created.ID)))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusOK))

	var resource openapi.Resource
	Expect(json.Unmarshal(resp.Body(), &resource)).To(Succeed())
	Expect(resource.Id).To(Equal(created.ID))
	Expect(resource.Kind).To(Equal("Version"))
}

func TestFlatChildRoutePatch(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupResourceTest(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	ch := createChannel(t, svc, fmt.Sprintf("flat-patch-parent-%s", uuid.NewString()[:8]))
	v := newVersionResource(fmt.Sprintf("flat-patch-v-%s", uuid.NewString()[:8]), ch.ID)
	created, svcErr := svc.Create(t.Context(), "Version", v, nil)
	Expect(svcErr).To(BeNil())

	patchSpec := map[string]interface{}{
		"raw_version":   "4.18.0",
		"enabled":       true,
		"is_default":    false,
		"release_image": "quay.io/ocp-release:4.18.0",
	}
	patchBody, marshalErr := json.Marshal(map[string]interface{}{"spec": patchSpec})
	Expect(marshalErr).NotTo(HaveOccurred())
	resp, err := rootResourceRequest(ctx).
		SetBody(patchBody).
		Patch(h.RestURL(fmt.Sprintf("/versions/%s", created.ID)))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusOK))

	var patched map[string]interface{}
	Expect(json.Unmarshal(resp.Body(), &patched)).To(Succeed())
	spec := patched["spec"].(map[string]interface{})
	Expect(spec["raw_version"]).To(Equal("4.18.0"))
}

func TestFlatChildRouteDelete(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupResourceTest(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	ch := createChannel(t, svc, fmt.Sprintf("flat-del-parent-%s", uuid.NewString()[:8]))
	v := newVersionResource(fmt.Sprintf("flat-del-v-%s", uuid.NewString()[:8]), ch.ID)
	_, svcErr := svc.Create(t.Context(), "Version", v, nil)
	Expect(svcErr).To(BeNil())

	retrieved, getErr := svc.Get(t.Context(), "Version", v.ID)
	Expect(getErr).To(BeNil())

	resp, err := rootResourceRequest(ctx).
		Delete(h.RestURL(fmt.Sprintf("/versions/%s", retrieved.ID)))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusAccepted))
}

func TestFlatChildRoutePostReturns422(t *testing.T) {
	RegisterTestingT(t)
	_, h := setupResourceTest(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	input := openapi.ResourceCreateRequest{
		Kind: "Version",
		Name: fmt.Sprintf("flat-post-%s", uuid.NewString()[:8]),
		Spec: map[string]interface{}{
			"raw_version":   "4.17.0",
			"enabled":       true,
			"is_default":    false,
			"release_image": "quay.io/openshift-release-dev/ocp-release:4.17.0",
		},
	}
	body, err := json.Marshal(input)
	Expect(err).NotTo(HaveOccurred())

	resp, err := rootResourceRequest(ctx).
		SetBody(body).
		Post(h.RestURL("/versions"))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusUnprocessableEntity))

	var problem openapi.ProblemDetails
	Expect(json.Unmarshal(resp.Body(), &problem)).To(Succeed())
	Expect(*problem.Detail).To(ContainSubstring("channels"))
}
