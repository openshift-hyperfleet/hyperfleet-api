package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/tenant"
)

// Tenant-scoped contexts for the isolation matrix. The tenant middleware is
// unit tested separately; these tests exercise the service and DAO layers
// with the resolved tenant already in context, which is exactly what the
// middleware produces.

func tenantCtx(t *testing.T, dims map[string]string) context.Context {
	return tenant.WithTenant(t.Context(), &tenant.ResolvedTenant{Dimensions: dims})
}

func systemCtx(t *testing.T) context.Context {
	return tenant.WithTenant(t.Context(), &tenant.ResolvedTenant{System: true})
}

func createTenantChannel(
	t *testing.T, svc services.ResourceService, ctx context.Context, name string,
) *api.Resource {
	created, svcErr := svc.Create(ctx, "Channel", newChannelResource(name), nil)
	Expect(svcErr).To(BeNil(), "channel creation should succeed")
	return created
}

func tenancyMap(t *testing.T, r *api.Resource) map[string]string {
	m := map[string]string{}
	if len(r.Tenancy) > 0 {
		Expect(json.Unmarshal(r.Tenancy, &m)).To(Succeed())
	}
	return m
}

func TestTenantIsolation_CreateInjectsTenancy(t *testing.T) {
	RegisterTestingT(t)
	svc, _ := setupResourceTest(t)

	acme := tenantCtx(t, map[string]string{"org": "acme", "project": "proj-1"})
	name := fmt.Sprintf("tenancy-create-%s", uuid.NewString()[:8])
	created := createTenantChannel(t, svc, acme, name)

	Expect(tenancyMap(t, created)).To(Equal(map[string]string{"org": "acme", "project": "proj-1"}))

	fetched, svcErr := svc.Get(acme, "Channel", created.ID)
	Expect(svcErr).To(BeNil())
	Expect(tenancyMap(t, fetched)).To(Equal(map[string]string{"org": "acme", "project": "proj-1"}))
}

func TestTenantIsolation_ListScopedBothDirections(t *testing.T) {
	RegisterTestingT(t)
	svc, _ := setupResourceTest(t)

	// Unique org names per run keep this test independent of leftover rows.
	orgA := "acme-" + uuid.NewString()[:8]
	orgB := "globex-" + uuid.NewString()[:8]
	acme := tenantCtx(t, map[string]string{"org": orgA})
	globex := tenantCtx(t, map[string]string{"org": orgB})

	a1 := createTenantChannel(t, svc, acme, fmt.Sprintf("iso-a1-%s", uuid.NewString()[:8]))
	a2 := createTenantChannel(t, svc, acme, fmt.Sprintf("iso-a2-%s", uuid.NewString()[:8]))
	b1 := createTenantChannel(t, svc, globex, fmt.Sprintf("iso-b1-%s", uuid.NewString()[:8]))

	args := &services.ListArguments{Page: 1, Size: 100}

	listA, metaA, svcErr := svc.List(acme, "Channel", args)
	Expect(svcErr).To(BeNil())
	idsA := map[string]bool{}
	for _, r := range listA {
		idsA[r.ID] = true
	}
	Expect(idsA).To(HaveKey(a1.ID))
	Expect(idsA).To(HaveKey(a2.ID))
	Expect(idsA).ToNot(HaveKey(b1.ID))
	Expect(metaA.Total).To(BeEquivalentTo(2), "list total must count only the tenant's resources")

	listB, metaB, svcErr := svc.List(globex, "Channel", args)
	Expect(svcErr).To(BeNil())
	idsB := map[string]bool{}
	for _, r := range listB {
		idsB[r.ID] = true
	}
	Expect(idsB).To(HaveKey(b1.ID))
	Expect(idsB).ToNot(HaveKey(a1.ID))
	Expect(metaB.Total).To(BeEquivalentTo(1))
}

func TestTenantIsolation_ContainmentGivesHierarchy(t *testing.T) {
	RegisterTestingT(t)
	svc, _ := setupResourceTest(t)

	org := "acme-" + uuid.NewString()[:8]
	proj1 := tenantCtx(t, map[string]string{"org": org, "project": "proj-1"})
	proj2 := tenantCtx(t, map[string]string{"org": org, "project": "proj-2"})
	orgOnly := tenantCtx(t, map[string]string{"org": org})

	p1 := createTenantChannel(t, svc, proj1, fmt.Sprintf("hier-p1-%s", uuid.NewString()[:8]))
	p2 := createTenantChannel(t, svc, proj2, fmt.Sprintf("hier-p2-%s", uuid.NewString()[:8]))

	// Org-scoped caller sees both projects.
	list, meta, svcErr := svc.List(orgOnly, "Channel", &services.ListArguments{Page: 1, Size: 100})
	Expect(svcErr).To(BeNil())
	ids := map[string]bool{}
	for _, r := range list {
		ids[r.ID] = true
	}
	Expect(ids).To(HaveKey(p1.ID))
	Expect(ids).To(HaveKey(p2.ID))
	Expect(meta.Total).To(BeEquivalentTo(2))

	// Project-scoped caller sees only its own project.
	list, _, svcErr = svc.List(proj1, "Channel", &services.ListArguments{Page: 1, Size: 100})
	Expect(svcErr).To(BeNil())
	ids = map[string]bool{}
	for _, r := range list {
		ids[r.ID] = true
	}
	Expect(ids).To(HaveKey(p1.ID))
	Expect(ids).ToNot(HaveKey(p2.ID))

	// Project-scoped caller can point-read its own resource but not a sibling
	// project's, even with the ID in hand.
	_, svcErr = svc.Get(proj1, "Channel", p1.ID)
	Expect(svcErr).To(BeNil())
	_, svcErr = svc.Get(proj1, "Channel", p2.ID)
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.Is404()).To(BeTrue())
}

func TestTenantIsolation_CrossTenantPointAccessIs404(t *testing.T) {
	RegisterTestingT(t)
	svc, _ := setupResourceTest(t)

	acme := tenantCtx(t, map[string]string{"org": "acme-" + uuid.NewString()[:8]})
	globex := tenantCtx(t, map[string]string{"org": "globex-" + uuid.NewString()[:8]})

	created := createTenantChannel(t, svc, acme, fmt.Sprintf("cross-%s", uuid.NewString()[:8]))

	// Get by kind+id.
	_, svcErr := svc.Get(globex, "Channel", created.ID)
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.Is404()).To(BeTrue(), "cross-tenant get must be 404, not 403")

	// GetByID (the path status handlers use to resolve parents).
	_, svcErr = svc.GetByID(globex, created.ID)
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.Is404()).To(BeTrue())

	// Patch.
	_, svcErr = svc.Patch(globex, "Channel", created.ID, &api.ResourcePatch{
		Labels: map[string]string{"env": "prod"},
	})
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.Is404()).To(BeTrue())

	// Delete.
	_, svcErr = svc.Delete(globex, "Channel", created.ID)
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.Is404()).To(BeTrue())

	// The owner still sees an untouched resource.
	fetched, svcErr := svc.Get(acme, "Channel", created.ID)
	Expect(svcErr).To(BeNil())
	Expect(fetched.DeletedTime).To(BeNil())
	Expect(labelsToMap(fetched.Labels)).ToNot(HaveKey("env"))
}

func TestTenantIsolation_SystemSeesEverything(t *testing.T) {
	RegisterTestingT(t)
	svc, _ := setupResourceTest(t)

	acme := tenantCtx(t, map[string]string{"org": "acme-" + uuid.NewString()[:8]})
	system := systemCtx(t)

	tenantOwned := createTenantChannel(t, svc, acme, fmt.Sprintf("sys-a-%s", uuid.NewString()[:8]))
	// Created without tenant context: empty tenancy map.
	unowned := createTenantChannel(t, svc, t.Context(), fmt.Sprintf("sys-u-%s", uuid.NewString()[:8]))
	Expect(tenancyMap(t, unowned)).To(BeEmpty())

	// System reads both.
	_, svcErr := svc.Get(system, "Channel", tenantOwned.ID)
	Expect(svcErr).To(BeNil())
	_, svcErr = svc.Get(system, "Channel", unowned.ID)
	Expect(svcErr).To(BeNil())

	list, _, svcErr := svc.List(system, "Channel", &services.ListArguments{Page: 1, Size: 100})
	Expect(svcErr).To(BeNil())
	ids := map[string]bool{}
	for _, r := range list {
		ids[r.ID] = true
	}
	Expect(ids).To(HaveKey(tenantOwned.ID))
	Expect(ids).To(HaveKey(unowned.ID))

	// The tenant can never see the unowned ({} tenancy) resource.
	_, svcErr = svc.Get(acme, "Channel", unowned.ID)
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.Is404()).To(BeTrue())
}

func TestTenantIsolation_PatchPreservesTenancy(t *testing.T) {
	RegisterTestingT(t)
	svc, _ := setupResourceTest(t)

	org := "acme-" + uuid.NewString()[:8]
	acme := tenantCtx(t, map[string]string{"org": org})

	created := createTenantChannel(t, svc, acme, fmt.Sprintf("patch-%s", uuid.NewString()[:8]))

	patched, svcErr := svc.Patch(acme, "Channel", created.ID, &api.ResourcePatch{
		Labels: map[string]string{"env": "prod"},
	})
	Expect(svcErr).To(BeNil())
	Expect(tenancyMap(t, patched)).To(Equal(map[string]string{"org": org}))
	Expect(labelsToMap(patched.Labels)).To(HaveKeyWithValue("env", "prod"))
}

func TestTenantIsolation_SearchComposesWithScope(t *testing.T) {
	RegisterTestingT(t)
	svc, _ := setupResourceTest(t)

	orgA := "acme-" + uuid.NewString()[:8]
	orgB := "globex-" + uuid.NewString()[:8]
	acme := tenantCtx(t, map[string]string{"org": orgA})
	globex := tenantCtx(t, map[string]string{"org": orgB})

	// A search matching both tenants' resources must only surface the caller's.
	shared := fmt.Sprintf("search-%s", uuid.NewString()[:8])
	mine := createTenantChannel(t, svc, acme, shared+"-a")
	theirs := createTenantChannel(t, svc, globex, shared+"-b")

	list, _, svcErr := svc.List(acme, "Channel", &services.ListArguments{
		Page: 1, Size: 100,
		Search: fmt.Sprintf("name in ['%s', '%s']", mine.Name, theirs.Name),
	})
	Expect(svcErr).To(BeNil())
	Expect(list).To(HaveLen(1))
	Expect(list[0].ID).To(Equal(mine.ID))

	// Label queries (EXISTS subquery path) compose with the tenant WHERE too.
	labeled, svcErr := svc.Patch(acme, "Channel", mine.ID, &api.ResourcePatch{
		Labels: map[string]string{"tier": "gold"},
	})
	Expect(svcErr).To(BeNil())
	Expect(labelsToMap(labeled.Labels)).To(HaveKeyWithValue("tier", "gold"))

	list, _, svcErr = svc.List(acme, "Channel", &services.ListArguments{
		Page: 1, Size: 100,
		Search: "labels.tier='gold'",
	})
	Expect(svcErr).To(BeNil())
	ids := map[string]bool{}
	for _, r := range list {
		ids[r.ID] = true
	}
	Expect(ids).To(HaveKey(mine.ID))

	list, _, svcErr = svc.List(globex, "Channel", &services.ListArguments{
		Page: 1, Size: 100,
		Search: "labels.tier='gold'",
	})
	Expect(svcErr).To(BeNil())
	Expect(list).To(BeEmpty(), "label search must not leak across tenants")
}

func TestTenantIsolation_OwnerScopedChildren(t *testing.T) {
	RegisterTestingT(t)
	svc, _ := setupResourceTest(t)

	org := "acme-" + uuid.NewString()[:8]
	acme := tenantCtx(t, map[string]string{"org": org})
	globex := tenantCtx(t, map[string]string{"org": "globex-" + uuid.NewString()[:8]})

	channel := createTenantChannel(t, svc, acme, fmt.Sprintf("owner-ch-%s", uuid.NewString()[:8]))
	version := newVersionResource(fmt.Sprintf("v1.0.0-%s", uuid.NewString()[:8]), channel.ID)
	createdVersion, svcErr := svc.Create(acme, "Version", version, nil)
	Expect(svcErr).To(BeNil())
	Expect(tenancyMap(t, createdVersion)).To(Equal(map[string]string{"org": org}))

	// Owner-scoped point read respects tenancy.
	_, svcErr = svc.GetByOwner(acme, "Version", createdVersion.ID, channel.ID)
	Expect(svcErr).To(BeNil())
	_, svcErr = svc.GetByOwner(globex, "Version", createdVersion.ID, channel.ID)
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.Is404()).To(BeTrue())

	// Owner-scoped list respects tenancy.
	list, _, svcErr := svc.ListByOwner(globex, "Version", channel.ID, &services.ListArguments{Page: 1, Size: 100})
	Expect(svcErr).To(BeNil())
	Expect(list).To(BeEmpty())
}
