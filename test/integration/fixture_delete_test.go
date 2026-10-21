package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/util"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

// HYPERFLEET-1730: delete-policy capabilities are proven over the synthetic
// SoftProbe and RestrictParent/RestrictChild kinds (declared in
// defaultTestEntities, test/helper.go) so the coverage survives changes to any
// production descriptor. The capability-to-test mapping lives on the ticket.

const (
	softProbesPath      = "/softprobes"
	restrictParentsPath = "/restrictparents"
)

// createSoftProbe POSTs a SoftProbe through its own flat route and returns the
// created resource envelope. SoftProbe requires the "validation" adapter, so
// deleting it takes the soft-delete path.
func createSoftProbe(t *testing.T, ctx context.Context, h *test.Helper, name string) openapi.Resource {
	t.Helper()
	input := openapi.ResourceCreateRequest{
		Kind: "SoftProbe",
		Name: name,
		Spec: map[string]interface{}{"probe": "value"},
	}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal SoftProbe create request: %v", err)
	}
	resp, err := rootResourceRequest(ctx).
		SetBody(body).
		Post(h.RestURL(softProbesPath))
	if err != nil {
		t.Fatalf("failed to POST softprobe: %v", err)
	}
	if resp.StatusCode() != http.StatusCreated {
		t.Fatalf("POST %s returned %d: %s", softProbesPath, resp.StatusCode(), resp.Body())
	}
	var created openapi.Resource
	if err := json.Unmarshal(resp.Body(), &created); err != nil {
		t.Fatalf("failed to unmarshal SoftProbe create response: %v", err)
	}
	return created
}

// TestFixtureSoftDeleteVisibility proves that a soft-deleted resource stays
// visible: DELETE returns 202 with deleted_time set, GET still returns 200 with
// deleted_time populated, and LIST includes soft-deleted rows by default.
func TestFixtureSoftDeleteVisibility(t *testing.T) {
	RegisterTestingT(t)
	_, h := setupResourceTest(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	active := createSoftProbe(t, ctx, h, fmt.Sprintf("soft-active-%s", uuid.NewString()[:8]))
	doomed := createSoftProbe(t, ctx, h, fmt.Sprintf("soft-doomed-%s", uuid.NewString()[:8]))

	resp, err := rootResourceRequest(ctx).
		Delete(h.RestURL(fmt.Sprintf("%s/%s", softProbesPath, doomed.Id)))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusAccepted))

	var deleted openapi.Resource
	Expect(json.Unmarshal(resp.Body(), &deleted)).To(Succeed())
	Expect(deleted.DeletedTime).NotTo(BeNil(), "DELETE response should carry deleted_time")

	t.Run("GetReturns200WithDeletedTime", func(t *testing.T) {
		RegisterTestingT(t)
		resp, err := rootResourceRequest(ctx).
			Get(h.RestURL(fmt.Sprintf("%s/%s", softProbesPath, doomed.Id)))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusOK),
			"GET on soft-deleted resource should return 200, not 404")

		var fetched openapi.Resource
		Expect(json.Unmarshal(resp.Body(), &fetched)).To(Succeed())
		Expect(fetched.Id).To(Equal(doomed.Id))
		Expect(fetched.DeletedTime).NotTo(BeNil(),
			"soft-deleted resource should have deleted_time in GET response")
	})

	t.Run("ListIncludesSoftDeletedByDefault", func(t *testing.T) {
		RegisterTestingT(t)
		resp, err := rootResourceRequest(ctx).
			Get(h.RestURL(softProbesPath))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusOK))

		var list openapi.ResourceList
		Expect(json.Unmarshal(resp.Body(), &list)).To(Succeed())

		var foundActive, foundDeleted bool
		for _, item := range list.Items {
			if item.Id == active.Id {
				Expect(item.DeletedTime).To(BeNil(), "active softprobe should not have deleted_time")
				foundActive = true
			}
			if item.Id == doomed.Id {
				Expect(item.DeletedTime).NotTo(BeNil(), "soft-deleted softprobe should have deleted_time")
				foundDeleted = true
			}
		}
		Expect(foundActive).To(BeTrue(), "active softprobe should appear in LIST")
		Expect(foundDeleted).To(BeTrue(), "soft-deleted softprobe should appear in LIST by default")
	})
}

// newRestrictParentResource builds a RestrictParent resource struct. Does NOT
// persist to database — use svc.Create() to persist.
func newRestrictParentResource(name string) *api.Resource {
	return &api.Resource{
		Kind:      "RestrictParent",
		Name:      name,
		Spec:      []byte(`{"key": "value"}`),
		CreatedBy: "test@example.com",
		UpdatedBy: "test@example.com",
	}
}

// newRestrictChildResource builds a RestrictChild owned by the given parent.
// Does NOT persist to database — use svc.Create() to persist.
func newRestrictChildResource(name, parentID string) *api.Resource {
	ownerKind := "RestrictParent"
	return &api.Resource{
		Kind:      "RestrictChild",
		Name:      name,
		OwnerID:   util.ToPtr(parentID),
		OwnerKind: util.ToPtr(ownerKind),
		Spec:      []byte(`{"key": "value"}`),
		CreatedBy: "test@example.com",
		UpdatedBy: "test@example.com",
	}
}

// TestFixtureDelete_RestrictPolicy proves that the restrict delete policy
// refuses parent deletion with 409 while an active child exists, and that the
// parent deletes once the child is gone.
func TestFixtureDelete_RestrictPolicy(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupResourceTest(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	parent, svcErr := svc.Create(t.Context(), "RestrictParent",
		newRestrictParentResource(fmt.Sprintf("restrict-parent-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())

	child, svcErr := svc.Create(t.Context(), "RestrictChild",
		newRestrictChildResource(fmt.Sprintf("restrict-child-%s", uuid.NewString()[:8]), parent.ID), nil)
	Expect(svcErr).To(BeNil())

	t.Run("ActiveChildBlocksParentDelete", func(t *testing.T) {
		RegisterTestingT(t)
		resp, err := rootResourceRequest(ctx).
			Delete(h.RestURL(fmt.Sprintf("%s/%s", restrictParentsPath, parent.ID)))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusConflict),
			"parent deletion must be refused with 409 while an active child exists")

		var problem openapi.ProblemDetails
		Expect(json.Unmarshal(resp.Body(), &problem)).To(Succeed())
		Expect(problem.Status).To(Equal(http.StatusConflict))
	})

	t.Run("ParentDeletesAfterChildrenGone", func(t *testing.T) {
		RegisterTestingT(t)
		_, svcErr := svc.Delete(t.Context(), "RestrictChild", child.ID)
		Expect(svcErr).To(BeNil())

		resp, err := rootResourceRequest(ctx).
			Delete(h.RestURL(fmt.Sprintf("%s/%s", restrictParentsPath, parent.ID)))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusAccepted),
			"parent deletion should succeed once no children remain")
	})
}

// TestFixtureDelete_NoRequiredAdaptersHardDeleted proves that a kind with no
// required adapters is hard-deleted inside the DELETE transaction: the request
// succeeds, the row is gone from the database immediately, and GET returns 404.
func TestFixtureDelete_NoRequiredAdaptersHardDeleted(t *testing.T) {
	RegisterTestingT(t)
	_, h := setupResourceTest(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	created := createCrudFixture(t, ctx, h, fmt.Sprintf("hard-del-%s", uuid.NewString()[:8]))

	resp, err := rootResourceRequest(ctx).
		Delete(h.RestURL(fmt.Sprintf("%s/%s", crudFixturesPath, created.Id)))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusAccepted))

	getResp, err := rootResourceRequest(ctx).
		Get(h.RestURL(fmt.Sprintf("%s/%s", crudFixturesPath, created.Id)))
	Expect(err).NotTo(HaveOccurred())
	Expect(getResp.StatusCode()).To(Equal(http.StatusNotFound),
		"hard-deleted resource must not be retrievable")

	// The row left the database inside the DELETE request — no adapter
	// finalization step is involved for a kind without required adapters.
	err = checkResourceCount(ctx, h, []string{created.Id}, 0)
	Expect(err).To(BeNil(), "hard-deleted crudfixture should be gone from the database")
}
