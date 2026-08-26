package integration

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/tenant"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

const scopeNodePoolKind = "NodePool"

// twoTenantContexts returns two distinct org-scoped contexts plus a system context.
func twoTenantContexts(h *test.Helper) (ctxA, ctxB, ctxSystem context.Context) {
	orgA := "acme-" + h.NewID()
	orgB := "globex-" + h.NewID()
	return tenancyCtx(map[string]string{tenancyOrgKey: orgA}),
		tenancyCtx(map[string]string{tenancyOrgKey: orgB}),
		systemCtx()
}

// assertScopedVisibility checks that same-tenant and system see the resource,
// while cross-tenant gets 404.
func assertScopedVisibility(
	t *testing.T, ctxA, ctxB, ctxSystem context.Context, wantID string,
	fetch func(ctx context.Context) (*api.Resource, *errors.ServiceError),
) {
	t.Helper()
	cases := []struct {
		ctx         context.Context
		name        string
		wantVisible bool
	}{
		{ctxA, "same-tenant sees it", true},
		{ctxSystem, "system identity bypasses scoping", true},
		{ctxB, "cross-tenant returns 404, not 403 - no existence leak", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			got, err := fetch(tc.ctx)
			if tc.wantVisible {
				Expect(err).To(BeNil())
				Expect(got.ID).To(Equal(wantID))
			} else {
				Expect(err).NotTo(BeNil())
				Expect(err.Is404()).To(BeTrue())
			}
		})
	}
}

// newTenancyNodePool builds a NodePool resource owned by the given cluster (not persisted).
func newTenancyNodePool(name, clusterID string) *api.Resource {
	ownerKind := tenancyClusterKind
	return &api.Resource{
		Kind:      scopeNodePoolKind,
		Name:      name,
		OwnerID:   &clusterID,
		OwnerKind: &ownerKind,
		Spec:      []byte(`{"replicas": 1}`),
		CreatedBy: tenancyTestActor,
		UpdatedBy: tenancyTestActor,
	}
}

// createResourceInTx runs svc.Create for an arbitrary kind inside a write transaction.
func createResourceInTx(
	base context.Context, sf db.SessionFactory, svc services.ResourceService, kind string, r *api.Resource,
) (*api.Resource, *errors.ServiceError) {
	txCtx, err := db.NewContext(base, sf)
	Expect(err).NotTo(HaveOccurred())
	defer db.Resolve(txCtx)
	return svc.Create(txCtx, kind, r, nil)
}

// patchInTx runs svc.Patch inside a write transaction.
func patchInTx(
	base context.Context, sf db.SessionFactory, svc services.ResourceService,
	kind, id string, patch *api.ResourcePatch,
) (*api.Resource, *errors.ServiceError) {
	txCtx, err := db.NewContext(base, sf)
	Expect(err).NotTo(HaveOccurred())
	defer db.Resolve(txCtx)
	return svc.Patch(txCtx, kind, id, patch)
}

// TestResourceScopeCrossTenantAccess verifies Get, List, Patch, and Delete are
// tenant-scoped: cross-tenant gets 404, same-tenant and system retain access.
func TestResourceScopeCrossTenantAccess(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	svc := h.Container.ResourceService()
	sf := h.Container.SessionFactory()

	ctxA, ctxB, ctxSystem := twoTenantContexts(h)

	created, svcErr := createInTx(ctxA, sf, svc, newTenancyCluster("scope-cross-tenant"))
	Expect(svcErr).To(BeNil())

	assertScopedVisibility(t, ctxA, ctxB, ctxSystem, created.ID,
		func(ctx context.Context) (*api.Resource, *errors.ServiceError) {
			return svc.Get(ctx, tenancyClusterKind, created.ID)
		})

	// Cross-tenant List omits the resource entirely.
	list, _, svcErr := svc.List(ctxB, tenancyClusterKind, services.NewListArguments())
	Expect(svcErr).To(BeNil())
	Expect(idsOf(list)).NotTo(ContainElement(created.ID))

	// Same-tenant List includes the resource.
	list, _, svcErr = svc.List(ctxA, tenancyClusterKind, services.NewListArguments())
	Expect(svcErr).To(BeNil())
	Expect(idsOf(list)).To(ContainElement(created.ID))

	// Same-tenant Patch succeeds (regression guard: if the Patch path changes to
	// use a different lookup, this catches same-tenant breakage that the
	// cross-tenant 404 assertion below would miss).
	patched, svcErr := patchInTx(ctxA, sf, svc, tenancyClusterKind, created.ID,
		&api.ResourcePatch{Labels: map[string]string{"env": "staging"}})
	Expect(svcErr).To(BeNil())
	Expect(patched.ID).To(Equal(created.ID))

	// Cross-tenant Patch and Delete fail the same way as Get above: the scoped
	// GetForUpdate lookup misses before the operation's own logic runs, so each
	// returns 404. Delete must stay last since it's the only mutating case here.
	crossTenantOps := []struct {
		run  func() *errors.ServiceError
		name string
	}{
		{func() *errors.ServiceError {
			_, err := patchInTx(ctxB, sf, svc, tenancyClusterKind, created.ID,
				&api.ResourcePatch{Labels: map[string]string{"env": "test"}})
			return err
		}, "Patch"},
		{func() *errors.ServiceError {
			return deleteInTx(ctxB, sf, svc, created.ID)
		}, "Delete"},
	}
	for _, tc := range crossTenantOps {
		t.Run(tc.name+" returns 404 for cross-tenant caller", func(t *testing.T) {
			RegisterTestingT(t)
			opErr := tc.run()
			Expect(opErr).NotTo(BeNil())
			Expect(opErr.Is404()).To(BeTrue())
		})
	}

	// Same-tenant Delete succeeds.
	svcErr = deleteInTx(ctxA, sf, svc, created.ID)
	Expect(svcErr).To(BeNil())
}

// TestResourceScopeParentChildHierarchy verifies parent-child tenancy inheritance
// and cross-tenant isolation on child resources.
func TestResourceScopeParentChildHierarchy(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	svc := h.Container.ResourceService()
	sf := h.Container.SessionFactory()

	ctxA, ctxB, _ := twoTenantContexts(h)

	cluster, svcErr := createInTx(ctxA, sf, svc, newTenancyCluster("scope-parent"))
	Expect(svcErr).To(BeNil())

	nodePool, svcErr := createResourceInTx(ctxA, sf, svc, scopeNodePoolKind, newTenancyNodePool("scope-child", cluster.ID))
	Expect(svcErr).To(BeNil())
	Expect(nodePool.Tenancy).To(MatchJSON(tenant.TenancyJSON(ctxA)))

	// Same-tenant reads succeed.
	_, svcErr = svc.GetByOwner(ctxA, scopeNodePoolKind, nodePool.ID, cluster.ID)
	Expect(svcErr).To(BeNil())

	// Cross-tenant read of the child returns 404.
	_, svcErr = svc.GetByOwner(ctxB, scopeNodePoolKind, nodePool.ID, cluster.ID)
	Expect(svcErr).NotTo(BeNil())
	Expect(svcErr.Is404()).To(BeTrue())

	// Cross-tenant ListByOwner returns an empty list rather than leaking the parent's children.
	children, _, svcErr := svc.ListByOwner(ctxB, scopeNodePoolKind, cluster.ID, services.NewListArguments())
	Expect(svcErr).To(BeNil())
	Expect(children).To(BeEmpty())

	// Same-tenant ListByOwner sees the child.
	children, _, svcErr = svc.ListByOwner(ctxA, scopeNodePoolKind, cluster.ID, services.NewListArguments())
	Expect(svcErr).To(BeNil())
	Expect(idsOf(children)).To(ContainElement(nodePool.ID))
}

// TestResourceScopeListTotalReflectsScopedCount verifies that List totals reflect
// only the caller's tenant, not global row count.
func TestResourceScopeListTotalReflectsScopedCount(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	svc := h.Container.ResourceService()
	sf := h.Container.SessionFactory()

	ctxA, ctxB, _ := twoTenantContexts(h)

	_, svcErr := createInTx(ctxA, sf, svc, newTenancyCluster("scope-total-a1"))
	Expect(svcErr).To(BeNil())
	_, svcErr = createInTx(ctxA, sf, svc, newTenancyCluster("scope-total-a2"))
	Expect(svcErr).To(BeNil())
	_, svcErr = createInTx(ctxB, sf, svc, newTenancyCluster("scope-total-b1"))
	Expect(svcErr).To(BeNil())

	_, paging, svcErr := svc.List(ctxA, tenancyClusterKind, services.NewListArguments())
	Expect(svcErr).To(BeNil())
	Expect(paging.Total).To(Equal(int64(2)), "total should count only tenant A's resources, not tenant B's")
}

// TestResourceScopeAppliesToAllKinds verifies tenant scoping applies uniformly
// across different entity kinds.
func TestResourceScopeAppliesToAllKinds(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	svc := h.Container.ResourceService()
	sf := h.Container.SessionFactory()

	ctxA, ctxB, ctxSystem := twoTenantContexts(h)

	kinds := []struct {
		resource *api.Resource
		kind     string
	}{
		{newTenancyCluster("scope-multi-kind"), tenancyClusterKind},
		{newChannelResource("scope-multi-kind"), "Channel"},
	}

	for _, k := range kinds {
		t.Run(k.kind, func(t *testing.T) {
			RegisterTestingT(t)

			created, svcErr := createResourceInTx(ctxA, sf, svc, k.kind, k.resource)
			Expect(svcErr).To(BeNil())

			assertScopedVisibility(t, ctxA, ctxB, ctxSystem, created.ID,
				func(ctx context.Context) (*api.Resource, *errors.ServiceError) {
					return svc.Get(ctx, k.kind, created.ID)
				})

			// Cross-tenant list omits the resource.
			list, _, svcErr := svc.List(ctxB, k.kind, services.NewListArguments())
			Expect(svcErr).To(BeNil())
			Expect(idsOf(list)).NotTo(ContainElement(created.ID))
		})
	}
}

// TestResourceScopeGetByID verifies that kind-less GetByID respects tenant scoping.
func TestResourceScopeGetByID(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	svc := h.Container.ResourceService()
	sf := h.Container.SessionFactory()

	ctxA, ctxB, ctxSystem := twoTenantContexts(h)

	created, svcErr := createInTx(ctxA, sf, svc, newTenancyCluster("scope-get-by-id"))
	Expect(svcErr).To(BeNil())

	assertScopedVisibility(t, ctxA, ctxB, ctxSystem, created.ID,
		func(ctx context.Context) (*api.Resource, *errors.ServiceError) {
			return svc.GetByID(ctx, created.ID)
		})
}

// TestResourceScopeListAll verifies that cross-kind ListAll respects tenant scoping.
func TestResourceScopeListAll(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	svc := h.Container.ResourceService()
	sf := h.Container.SessionFactory()

	ctxA, ctxB, ctxSystem := twoTenantContexts(h)

	created, svcErr := createInTx(ctxA, sf, svc, newTenancyCluster("scope-list-all"))
	Expect(svcErr).To(BeNil())

	list, _, svcErr := svc.ListAll(ctxA, services.NewListArguments())
	Expect(svcErr).To(BeNil())
	Expect(idsOf(list)).To(ContainElement(created.ID))

	list, _, svcErr = svc.ListAll(ctxSystem, services.NewListArguments())
	Expect(svcErr).To(BeNil())
	Expect(idsOf(list)).To(ContainElement(created.ID))

	list, _, svcErr = svc.ListAll(ctxB, services.NewListArguments())
	Expect(svcErr).To(BeNil())
	Expect(idsOf(list)).NotTo(ContainElement(created.ID))
}

// deleteResourceInTx runs svc.Delete for an arbitrary kind inside a write transaction.
func deleteResourceInTx(
	base context.Context, sf db.SessionFactory, svc services.ResourceService, kind, id string,
) *errors.ServiceError {
	txCtx, err := db.NewContext(base, sf)
	Expect(err).NotTo(HaveOccurred())
	defer db.Resolve(txCtx)
	_, svcErr := svc.Delete(txCtx, kind, id)
	return svcErr
}

// TestResourceScopeDaoFindAndExistsPaths verifies DAO methods not directly exposed
// through the service layer (FindByKind, ExistsByOwner, etc.) are tenant-scoped.
func TestResourceScopeDaoFindAndExistsPaths(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	svc := h.Container.ResourceService()
	sf := h.Container.SessionFactory()
	resourceDao := h.Container.ResourceDao()

	ctxA, ctxB, ctxSystem := twoTenantContexts(h)

	cluster, svcErr := createInTx(ctxA, sf, svc, newTenancyCluster("scope-dao-parent"))
	Expect(svcErr).To(BeNil())

	np := newTenancyNodePool("scope-dao-child", cluster.ID)
	nodePool, svcErr := createResourceInTx(ctxA, sf, svc, scopeNodePoolKind, np)
	Expect(svcErr).To(BeNil())

	registerRefTestDescriptors()
	target, svcErr := createResourceInTx(ctxA, sf, svc, "RefTarget",
		newRefTestResource("RefTarget", "scope-dao-reftarget-"+h.NewID()))
	Expect(svcErr).To(BeNil())

	refs := makeRefs("dep", struct{ id, kind string }{target.ID, "RefTarget"})
	refTxCtx, err := db.NewContext(ctxA, sf)
	Expect(err).NotTo(HaveOccurred())
	source, svcErr := svc.Create(refTxCtx, "RefSource",
		newRefTestResource("RefSource", "scope-dao-refsource-"+h.NewID()), refs)
	db.Resolve(refTxCtx)
	Expect(svcErr).To(BeNil())

	checkCases := []struct {
		fetch func(ctx context.Context) ([]string, error)
		name  string
		want  string
	}{
		{name: "FindByKind", want: nodePool.ID, fetch: func(ctx context.Context) ([]string, error) {
			list, err := resourceDao.FindByKind(ctx, scopeNodePoolKind)
			return idsOf(list), err
		}},
		{name: "FindByKindAndOwner", want: nodePool.ID, fetch: func(ctx context.Context) ([]string, error) {
			list, err := resourceDao.FindByKindAndOwner(ctx, scopeNodePoolKind, cluster.ID)
			return idsOf(list), err
		}},
		{name: "FindByKindAndOwnerForUpdate", want: nodePool.ID, fetch: func(ctx context.Context) ([]string, error) {
			list, err := resourceDao.FindByKindAndOwnerForUpdate(ctx, scopeNodePoolKind, cluster.ID)
			return idsOf(list), err
		}},
		{name: "FindReferencers", want: source.Name, fetch: func(ctx context.Context) ([]string, error) {
			summaries, err := resourceDao.FindReferencers(ctx, target.ID)
			return namesOf(summaries), err
		}},
	}
	for _, tc := range checkCases {
		t.Run(tc.name+" omits cross-tenant rows", func(t *testing.T) {
			RegisterTestingT(t)
			listA, err := tc.fetch(ctxA)
			Expect(err).NotTo(HaveOccurred())
			Expect(listA).To(ContainElement(tc.want))

			listSystem, err := tc.fetch(ctxSystem)
			Expect(err).NotTo(HaveOccurred())
			Expect(listSystem).To(ContainElement(tc.want))

			listB, err := tc.fetch(ctxB)
			Expect(err).NotTo(HaveOccurred())
			Expect(listB).NotTo(ContainElement(tc.want))
		})
	}

	existsCases := []struct {
		setup func()
		fetch func(ctx context.Context) (bool, error)
		name  string
	}{
		{
			name: "ExistsByOwner",
			fetch: func(ctx context.Context) (bool, error) {
				return resourceDao.ExistsByOwner(ctx, scopeNodePoolKind, cluster.ID)
			},
		},
		{
			name: "ExistsSoftDeletedByOwner",
			setup: func() {
				deletedChild, svcErr := createResourceInTx(
					ctxA, sf, svc, scopeNodePoolKind, newTenancyNodePool("scope-dao-deleted-child", cluster.ID))
				Expect(svcErr).To(BeNil())
				Expect(deleteResourceInTx(ctxA, sf, svc, scopeNodePoolKind, deletedChild.ID)).To(BeNil())
			},
			fetch: func(ctx context.Context) (bool, error) {
				return resourceDao.ExistsSoftDeletedByOwner(ctx, []string{scopeNodePoolKind}, cluster.ID)
			},
		},
	}
	for _, tc := range existsCases {
		t.Run(tc.name+" hides cross-tenant children", func(t *testing.T) {
			RegisterTestingT(t)
			if tc.setup != nil {
				tc.setup()
			}
			existsA, err := tc.fetch(ctxA)
			Expect(err).NotTo(HaveOccurred())
			Expect(existsA).To(BeTrue())

			existsSystem, err := tc.fetch(ctxSystem)
			Expect(err).NotTo(HaveOccurred())
			Expect(existsSystem).To(BeTrue())

			existsB, err := tc.fetch(ctxB)
			Expect(err).NotTo(HaveOccurred())
			Expect(existsB).To(BeFalse(), "cross-tenant caller must not observe existence")
		})
	}

	t.Run("non-system caller with zero dimensions is denied without erroring", func(t *testing.T) {
		RegisterTestingT(t)
		ctxDenied := tenancyCtx(map[string]string{})

		list, err := resourceDao.FindByKind(ctxDenied, scopeNodePoolKind)
		Expect(err).NotTo(HaveOccurred(), "deny-all clause must not error against a real driver")
		Expect(list).To(BeEmpty())

		exists, err := resourceDao.ExistsByOwner(ctxDenied, scopeNodePoolKind, cluster.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(exists).To(BeFalse())
	})
}

// namesOf extracts resource names from a ResourceSummary slice.
func namesOf(summaries []api.ResourceSummary) []string {
	names := make([]string, len(summaries))
	for i, s := range summaries {
		names[i] = s.Name
	}
	return names
}

// idsOf extracts resource IDs from a slice.
func idsOf(list api.ResourceList) []string {
	ids := make([]string, len(list))
	for i, r := range list {
		ids[i] = r.ID
	}
	return ids
}
