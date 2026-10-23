package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/tenant"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

const crudFixturesPath = "/crudfixtures"

// HYPERFLEET-1730: the generic resource capabilities the per-kind E2E suites used
// to prove are re-proven here over the synthetic CrudFixture kind (declared in
// defaultTestEntities, test/helper.go), so the coverage survives changes to any
// production descriptor. The capability-to-test mapping lives on the ticket.

// newCrudFixtureInput returns a create request whose spec satisfies
// CrudFixtureSpec (test/validation-schema.yaml requires fixture_key).
func newCrudFixtureInput(name string) openapi.ResourceCreateRequest {
	return openapi.ResourceCreateRequest{
		Kind: "CrudFixture",
		Name: name,
		Spec: map[string]interface{}{"fixture_key": "initial"},
	}
}

// createCrudFixture POSTs a valid CrudFixture through its own config-driven
// flat route and returns the created resource envelope.
func createCrudFixture(t *testing.T, ctx context.Context, h *test.Helper, name string) openapi.Resource {
	t.Helper()
	body, err := json.Marshal(newCrudFixtureInput(name))
	if err != nil {
		t.Fatalf("failed to marshal CrudFixture create request: %v", err)
	}
	resp, err := rootResourceRequest(ctx).
		SetBody(body).
		Post(h.RestURL(crudFixturesPath))
	if err != nil {
		t.Fatalf("failed to POST crudfixture: %v", err)
	}
	if resp.StatusCode() != http.StatusCreated {
		t.Fatalf("POST %s returned %d: %s", crudFixturesPath, resp.StatusCode(), resp.Body())
	}
	var created openapi.Resource
	if err := json.Unmarshal(resp.Body(), &created); err != nil {
		t.Fatalf("failed to unmarshal CrudFixture create response: %v", err)
	}
	return created
}

// TestFixtureFlatRouteCRUD proves the config-driven flat route wiring and the
// generic synchronous CRUD round trip — create, get, list, patch, delete —
// including the response envelope shape, over a synthetic kind.
func TestFixtureFlatRouteCRUD(t *testing.T) {
	RegisterTestingT(t)
	_, h := setupResourceTest(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	name := fmt.Sprintf("flat-fixture-%s", uuid.NewString()[:8])
	created := createCrudFixture(t, ctx, h, name)

	// Envelope shape on create.
	Expect(created.Id).NotTo(BeEmpty())
	Expect(created.Kind).To(Equal("CrudFixture"))
	Expect(created.Name).To(Equal(name))
	Expect(created.Generation).To(Equal(int32(1)))
	Expect(created.CreatedTime).NotTo(BeZero())
	Expect(created.UpdatedTime).NotTo(BeZero())
	Expect(created.DeletedTime).To(BeNil())

	t.Run("Get", func(t *testing.T) {
		RegisterTestingT(t)
		resp, err := rootResourceRequest(ctx).
			Get(h.RestURL(fmt.Sprintf("%s/%s", crudFixturesPath, created.Id)))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusOK))

		var fetched openapi.Resource
		Expect(json.Unmarshal(resp.Body(), &fetched)).To(Succeed())
		Expect(fetched.Id).To(Equal(created.Id))
		Expect(fetched.Kind).To(Equal("CrudFixture"))
		Expect(fetched.Name).To(Equal(name))
		Expect(fetched.Generation).To(Equal(int32(1)))
	})

	t.Run("List", func(t *testing.T) {
		RegisterTestingT(t)
		resp, err := rootResourceRequest(ctx).
			Get(h.RestURL(crudFixturesPath))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusOK))

		var list openapi.ResourceList
		Expect(json.Unmarshal(resp.Body(), &list)).To(Succeed())
		found := false
		for _, item := range list.Items {
			if item.Id == created.Id {
				Expect(item.Kind).To(Equal("CrudFixture"))
				found = true
			}
		}
		Expect(found).To(BeTrue(), "created crudfixture should appear in the /crudfixtures list")
	})

	t.Run("Patch", func(t *testing.T) {
		RegisterTestingT(t)
		patchBody := `{"spec": {"fixture_key": "updated"}}`
		resp, err := rootResourceRequest(ctx).
			SetBody(patchBody).
			Patch(h.RestURL(fmt.Sprintf("%s/%s", crudFixturesPath, created.Id)))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusOK))

		var patched openapi.Resource
		Expect(json.Unmarshal(resp.Body(), &patched)).To(Succeed())
		Expect(patched.Generation).To(Equal(int32(2)), "generation should increment after PATCH")
		Expect(patched.Spec["fixture_key"]).To(Equal("updated"))

		fetchedResp, err := rootResourceRequest(ctx).
			Get(h.RestURL(fmt.Sprintf("%s/%s", crudFixturesPath, created.Id)))
		Expect(err).NotTo(HaveOccurred())
		Expect(fetchedResp.StatusCode()).To(Equal(http.StatusOK))

		var fetched openapi.Resource
		Expect(json.Unmarshal(fetchedResp.Body(), &fetched)).To(Succeed())
		Expect(fetched.Spec["fixture_key"]).To(Equal("updated"), "patched spec should persist across a GET")
	})

	// Own fixture so the generation count starts at 1 and does not depend on
	// the Patch subtest above.
	t.Run("PatchLabels", func(t *testing.T) {
		RegisterTestingT(t)
		labeled := createCrudFixture(t, ctx, h, fmt.Sprintf("label-fixture-%s", uuid.NewString()[:8]))

		patchReq := openapi.ResourcePatchRequest{
			Labels: &map[string]string{"crud": labeled.Id},
		}
		patchBody, err := json.Marshal(patchReq)
		Expect(err).NotTo(HaveOccurred())

		resp, err := rootResourceRequest(ctx).
			SetBody(patchBody).
			Patch(h.RestURL(fmt.Sprintf("%s/%s", crudFixturesPath, labeled.Id)))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusOK))

		var patched openapi.Resource
		Expect(json.Unmarshal(resp.Body(), &patched)).To(Succeed())
		Expect(patched.Generation).To(Equal(int32(2)), "labels-only PATCH should increment generation")
		Expect(patched.Labels).NotTo(BeNil())
		Expect((*patched.Labels)["crud"]).To(Equal(labeled.Id))

		listResp, err := rootResourceRequest(ctx).
			SetQueryParam("search", fmt.Sprintf("labels.crud='%s'", labeled.Id)).
			Get(h.RestURL(crudFixturesPath))
		Expect(err).NotTo(HaveOccurred())
		Expect(listResp.StatusCode()).To(Equal(http.StatusOK))

		var list openapi.ResourceList
		Expect(json.Unmarshal(listResp.Body(), &list)).To(Succeed())
		Expect(list.Items).To(HaveLen(1), "label search should return exactly the labeled fixture")
		Expect(list.Items[0].Id).To(Equal(labeled.Id))
	})

	t.Run("Delete", func(t *testing.T) {
		RegisterTestingT(t)
		resp, err := rootResourceRequest(ctx).
			Delete(h.RestURL(fmt.Sprintf("%s/%s", crudFixturesPath, created.Id)))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusAccepted))

		// The 202 body echoes the deleted resource's envelope: the service
		// marks it deleted (deleted_time and deleted_by set) before the row
		// leaves the database.
		var deleted openapi.Resource
		Expect(json.Unmarshal(resp.Body(), &deleted)).To(Succeed())
		Expect(deleted.Id).To(Equal(created.Id))
		Expect(deleted.Kind).To(Equal("CrudFixture"))
		Expect(deleted.Name).To(Equal(name))
		Expect(deleted.DeletedTime).NotTo(BeNil(), "DELETE response should carry deleted_time")
		Expect(deleted.DeletedBy).NotTo(BeNil())
		Expect(*deleted.DeletedBy).To(Equal(account.Email),
			"DELETE response should carry the identity that requested deletion")
	})
}

// newCrudFixtureResource builds a CrudFixture resource struct whose spec
// satisfies CrudFixtureSpec. Does NOT persist — use svc.Create() to persist.
func newCrudFixtureResource(name string) *api.Resource {
	return &api.Resource{
		Kind:      "CrudFixture",
		Name:      name,
		Spec:      []byte(`{"fixture_key": "initial"}`),
		CreatedBy: "test@example.com",
		UpdatedBy: "test@example.com",
	}
}

// TestFixtureNameUniqueness proves that a duplicate name within the same tenant
// is refused with 409 while a different tenant may own the same name.
func TestFixtureNameUniqueness(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupResourceTest(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	name := fmt.Sprintf("unique-fixture-%s", uuid.NewString()[:8])
	created := createCrudFixture(t, ctx, h, name)
	Expect(created.Name).To(Equal(name))

	t.Run("DuplicateNameInSameTenantReturns409", func(t *testing.T) {
		RegisterTestingT(t)
		body, err := json.Marshal(newCrudFixtureInput(name))
		Expect(err).NotTo(HaveOccurred())

		resp, err := rootResourceRequest(ctx).
			SetBody(body).
			Post(h.RestURL(crudFixturesPath))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusConflict))

		var problem openapi.ProblemDetails
		Expect(json.Unmarshal(resp.Body(), &problem)).To(Succeed())
		Expect(problem.Status).To(Equal(http.StatusConflict))
	})

	// Tenant scoping is proven at the service layer with explicit tenant
	// contexts, mirroring TestClusterNameUniquenessIsTenantScoped: the
	// tenant-resolution middleware is not yet wired into the HTTP path, so two
	// HTTP callers today share the empty tenancy and legitimately conflict.
	// The service handle is reused from the parent setup — a second
	// setupResourceTest would ResetDB mid-test and discard the fixture created
	// above (its empty-tenancy row does not collide with the acme/globex rows
	// under the tenant-scoped unique index).
	t.Run("SameNameAllowedInDifferentTenant", func(t *testing.T) {
		RegisterTestingT(t)

		ctxAcme := tenant.WithTenant(context.Background(),
			&tenant.ResolvedTenant{Dimensions: map[string]string{"org": "acme"}})
		ctxGlobex := tenant.WithTenant(context.Background(),
			&tenant.ResolvedTenant{Dimensions: map[string]string{"org": "globex"}})

		acme, svcErr := svc.Create(ctxAcme, "CrudFixture", newCrudFixtureResource(name), nil)
		Expect(svcErr).To(BeNil())
		Expect(acme.Tenancy).To(MatchJSON(`{"org":"acme"}`))

		globex, svcErr := svc.Create(ctxGlobex, "CrudFixture", newCrudFixtureResource(name), nil)
		Expect(svcErr).To(BeNil(), "cross-tenant same-name create should succeed")
		Expect(globex.Tenancy).To(MatchJSON(`{"org":"globex"}`))
	})
}

// TestFixturePatchGeneration proves that generation increments only when the
// spec actually changes: an identical-spec PATCH is a no-op.
func TestFixturePatchGeneration(t *testing.T) {
	RegisterTestingT(t)
	_, h := setupResourceTest(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	created := createCrudFixture(t, ctx, h, fmt.Sprintf("gen-fixture-%s", uuid.NewString()[:8]))
	Expect(created.Generation).To(Equal(int32(1)))

	patchBody := `{"spec": {"fixture_key": "changed"}}`

	t.Run("SpecChangeIncrementsGeneration", func(t *testing.T) {
		RegisterTestingT(t)
		resp, err := rootResourceRequest(ctx).
			SetBody(patchBody).
			Patch(h.RestURL(fmt.Sprintf("%s/%s", crudFixturesPath, created.Id)))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusOK))

		var patched openapi.Resource
		Expect(json.Unmarshal(resp.Body(), &patched)).To(Succeed())
		Expect(patched.Generation).To(Equal(int32(2)))
	})

	t.Run("NoOpPatchDoesNotIncrement", func(t *testing.T) {
		RegisterTestingT(t)
		resp, err := rootResourceRequest(ctx).
			SetBody(patchBody).
			Patch(h.RestURL(fmt.Sprintf("%s/%s", crudFixturesPath, created.Id)))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusOK))

		var patched openapi.Resource
		Expect(json.Unmarshal(resp.Body(), &patched)).To(Succeed())
		Expect(patched.Generation).To(Equal(int32(2)),
			"replaying an identical spec PATCH must not increment generation")
	})
}

// TestFixtureSpecSchemaValidation proves that spec-schema validation rejects an
// invalid spec on the synthetic kind's own route: a non-object spec and specs
// violating the CrudFixtureSpec schema (missing or non-string fixture_key)
// return 400 with the RFC 9457 problem-details shape.
func TestFixtureSpecSchemaValidation(t *testing.T) {
	RegisterTestingT(t)
	_, h := setupResourceTest(t)

	// CrudFixtureSpec exists only in test/validation-schema.yaml: under another
	// schema file the validator skips the kind and the 400s below cannot be
	// produced (skip); under the default test schema a missing CrudFixtureSpec
	// is broken setup and must fail loudly.
	hasSchema := h.Container.SchemaValidator().HasSchema("crudfixtures")
	if strings.HasSuffix(os.Getenv("HYPERFLEET_SERVER_OPENAPI_SCHEMA_PATH"), "validation-schema.yaml") {
		if !hasSchema {
			t.Fatalf("CrudFixtureSpec missing from the test schema - crudfixtures spec validation would be silently disabled")
		}
	} else if !hasSchema {
		t.Skip("Skipping CrudFixture spec-schema validation - no CrudFixtureSpec in the configured schema")
	}

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	t.Run("RejectsNonObjectSpec", func(t *testing.T) {
		RegisterTestingT(t)
		body := fmt.Sprintf(
			`{"kind":"CrudFixture","name":"bad-type-%s","spec":"not-an-object"}`,
			uuid.NewString()[:8],
		)
		resp, err := rootResourceRequest(ctx).
			SetBody(body).
			Post(h.RestURL(crudFixturesPath))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusBadRequest))

		var problem openapi.ProblemDetails
		Expect(json.Unmarshal(resp.Body(), &problem)).To(Succeed())
		Expect(problem.Status).To(Equal(http.StatusBadRequest))
	})

	t.Run("RejectsSpecMissingRequiredField", func(t *testing.T) {
		RegisterTestingT(t)
		input := openapi.ResourceCreateRequest{
			Kind: "CrudFixture",
			Name: fmt.Sprintf("missing-key-%s", uuid.NewString()[:8]),
			Spec: map[string]interface{}{"unrelated": "value"},
		}
		body, err := json.Marshal(input)
		Expect(err).NotTo(HaveOccurred())

		resp, err := rootResourceRequest(ctx).
			SetBody(body).
			Post(h.RestURL(crudFixturesPath))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusBadRequest),
			"spec missing required fixture_key must be rejected")

		var problem openapi.ProblemDetails
		Expect(json.Unmarshal(resp.Body(), &problem)).To(Succeed())
		Expect(problem.Code).NotTo(BeNil())
		foundFieldError := false
		if problem.Errors != nil {
			for _, detail := range *problem.Errors {
				if strings.Contains(detail.Field, "fixture_key") {
					foundFieldError = true
					break
				}
			}
		}
		Expect(foundFieldError).To(BeTrue(), "error details should mention missing 'fixture_key' field")
	})

	t.Run("RejectsNonStringFixtureKey", func(t *testing.T) {
		RegisterTestingT(t)
		body := fmt.Sprintf(
			`{"kind":"CrudFixture","name":"bad-key-type-%s","spec":{"fixture_key":42}}`,
			uuid.NewString()[:8],
		)
		resp, err := rootResourceRequest(ctx).
			SetBody(body).
			Post(h.RestURL(crudFixturesPath))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusBadRequest),
			"spec with a non-string fixture_key must be rejected")

		var problem openapi.ProblemDetails
		Expect(json.Unmarshal(resp.Body(), &problem)).To(Succeed())
		Expect(problem.Status).To(Equal(http.StatusBadRequest))
	})

	t.Run("PatchRejectsInvalidSpec", func(t *testing.T) {
		RegisterTestingT(t)
		created := createCrudFixture(t, ctx, h, fmt.Sprintf("patch-inv-%s", uuid.NewString()[:8]))

		patchBody := `{"spec": {"unrelated": "value"}}`
		resp, err := rootResourceRequest(ctx).
			SetBody(patchBody).
			Patch(h.RestURL(fmt.Sprintf("%s/%s", crudFixturesPath, created.Id)))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusBadRequest),
			"PATCH with spec missing required fixture_key must be rejected")

		var problem openapi.ProblemDetails
		Expect(json.Unmarshal(resp.Body(), &problem)).To(Succeed())
		Expect(problem.Status).To(Equal(http.StatusBadRequest))
	})

	t.Run("PatchRejectsNonStringFixtureKey", func(t *testing.T) {
		RegisterTestingT(t)
		created := createCrudFixture(t, ctx, h, fmt.Sprintf("patch-bad-key-%s", uuid.NewString()[:8]))

		patchBody := `{"spec": {"fixture_key": 42}}`
		resp, err := rootResourceRequest(ctx).
			SetBody(patchBody).
			Patch(h.RestURL(fmt.Sprintf("%s/%s", crudFixturesPath, created.Id)))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusBadRequest),
			"PATCH with a non-string fixture_key must be rejected")

		var problem openapi.ProblemDetails
		Expect(json.Unmarshal(resp.Body(), &problem)).To(Succeed())
		Expect(problem.Status).To(Equal(http.StatusBadRequest))
	})
}
