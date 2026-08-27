package integration

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"gorm.io/datatypes"

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

// TestResourceScopeForceDeleteCrossTenant verifies that ForceDelete (hard delete) is
// scoped: a cross-tenant caller gets 404 because GetForUpdate misses before the delete
// logic runs. This guards a distinct destructive code path from the soft-delete test above.
func TestResourceScopeForceDeleteCrossTenant(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	svc := h.Container.ResourceService()
	sf := h.Container.SessionFactory()

	ctxA, ctxB, _ := twoTenantContexts(h)

	created, svcErr := createInTx(ctxA, sf, svc, newTenancyCluster("scope-force-delete"))
	Expect(svcErr).To(BeNil())

	// Soft-delete first (ForceDelete requires Finalizing state).
	svcErr = deleteInTx(ctxA, sf, svc, created.ID)
	Expect(svcErr).To(BeNil())

	t.Run("cross-tenant ForceDelete returns 404", func(t *testing.T) {
		RegisterTestingT(t)
		txCtx, err := db.NewContext(ctxB, sf)
		Expect(err).NotTo(HaveOccurred())
		defer db.Resolve(txCtx)
		svcErr := svc.ForceDelete(txCtx, tenancyClusterKind, created.ID, "cross-tenant attempt")
		Expect(svcErr).NotTo(BeNil())
		Expect(svcErr.Is404()).To(BeTrue())
	})

	t.Run("same-tenant ForceDelete succeeds", func(t *testing.T) {
		RegisterTestingT(t)
		txCtx, err := db.NewContext(ctxA, sf)
		Expect(err).NotTo(HaveOccurred())
		defer db.Resolve(txCtx)
		svcErr := svc.ForceDelete(txCtx, tenancyClusterKind, created.ID, "owner cleanup")
		Expect(svcErr).To(BeNil())
	})
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
	children, paging, svcErr := svc.ListByOwner(ctxB, scopeNodePoolKind, cluster.ID, services.NewListArguments())
	Expect(svcErr).To(BeNil())
	Expect(children).To(BeEmpty())
	Expect(paging.Total).To(Equal(int64(0)))

	// Same-tenant ListByOwner sees the child.
	children, paging, svcErr = svc.ListByOwner(ctxA, scopeNodePoolKind, cluster.ID, services.NewListArguments())
	Expect(svcErr).To(BeNil())
	Expect(idsOf(children)).To(ContainElement(nodePool.ID))
	Expect(paging.Total).To(Equal(int64(1)))
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

	_, paging, svcErr = svc.List(ctxB, tenancyClusterKind, services.NewListArguments())
	Expect(svcErr).To(BeNil())
	Expect(paging.Total).To(Equal(int64(1)), "total should count only tenant B's resources, not tenant A's")
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
	source, svcErr := func() (*api.Resource, *errors.ServiceError) {
		refTxCtx, err := db.NewContext(ctxA, sf)
		Expect(err).NotTo(HaveOccurred())
		defer db.Resolve(refTxCtx)
		return svc.Create(refTxCtx, "RefSource",
			newRefTestResource("RefSource", "scope-dao-refsource-"+h.NewID()), refs)
	}()
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

		refs, err := resourceDao.FindReferencers(ctxDenied, target.ID)
		Expect(err).NotTo(HaveOccurred(), "deny-all in FindReferencers must not produce invalid SQL")
		Expect(refs).To(BeEmpty())

		svcList, paging, svcErr := svc.List(ctxDenied, tenancyClusterKind, services.NewListArguments())
		Expect(svcErr).To(BeNil())
		Expect(svcList).To(BeEmpty())
		Expect(paging.Total).To(Equal(int64(0)))
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

// TestResourceScopeTenancyImmutableThroughPatch verifies that patching spec or labels
// does not alter the stored tenancy. ResourcePatch structurally excludes tenancy (no field
// for it), so this test is a regression guard: if someone accidentally adds tenancy to the
// patch path or the Save overwrites it, this catches it.
func TestResourceScopeTenancyImmutableThroughPatch(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	svc := h.Container.ResourceService()
	sf := h.Container.SessionFactory()

	ctxA, _, _ := twoTenantContexts(h)

	created, svcErr := createInTx(ctxA, sf, svc, newTenancyCluster("scope-immutable-tenancy"))
	Expect(svcErr).To(BeNil())
	originalTenancy := created.Tenancy

	t.Run("spec patch preserves tenancy", func(t *testing.T) {
		RegisterTestingT(t)
		patched, svcErr := patchInTx(ctxA, sf, svc, tenancyClusterKind, created.ID,
			&api.ResourcePatch{Spec: map[string]interface{}{"region": "eu-west1"}})
		Expect(svcErr).To(BeNil())
		Expect(patched.Tenancy).To(MatchJSON(originalTenancy))
	})

	t.Run("labels patch preserves tenancy", func(t *testing.T) {
		RegisterTestingT(t)
		patched, svcErr := patchInTx(ctxA, sf, svc, tenancyClusterKind, created.ID,
			&api.ResourcePatch{Labels: map[string]string{"env": "staging"}})
		Expect(svcErr).To(BeNil())
		Expect(patched.Tenancy).To(MatchJSON(originalTenancy))
	})

	t.Run("re-read from DB confirms tenancy unchanged", func(t *testing.T) {
		RegisterTestingT(t)
		got, svcErr := svc.Get(ctxA, tenancyClusterKind, created.ID)
		Expect(svcErr).To(BeNil())
		Expect(got.Tenancy).To(MatchJSON(originalTenancy))
	})
}

// newTestAdapterStatus builds an AdapterStatus with the three mandatory conditions
// (Available, Applied, Health) for use in ProcessAdapterStatus calls.
func newTestAdapterStatus(t *testing.T, adapter string, generation int32) *api.AdapterStatus {
	t.Helper()
	now := time.Now()
	return &api.AdapterStatus{
		Adapter:            adapter,
		ObservedGeneration: generation,
		LastReportTime:     now,
		Conditions:         mandatoryAdapterConditionsJSON(t, api.AdapterConditionTrue),
	}
}

// TestResourceScopeStatusSubResourceCrossTenant verifies that ProcessAdapterStatus
// rejects cross-tenant callers with 404: the parent resource lookup (GetForUpdate) is
// scoped, so a tenant caller targeting another tenant's resource gets "not found" before
// any status logic runs.
func TestResourceScopeStatusSubResourceCrossTenant(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	svc := h.Container.ResourceService()
	sf := h.Container.SessionFactory()

	ctxA, ctxB, ctxSystem := twoTenantContexts(h)

	created, svcErr := createInTx(ctxA, sf, svc, newTenancyCluster("scope-status-parent"))
	Expect(svcErr).To(BeNil())

	adapterStatus := newTestAdapterStatus(t, "test-adapter", created.Generation)

	t.Run("same-tenant status write succeeds", func(t *testing.T) {
		RegisterTestingT(t)
		txCtx, err := db.NewContext(ctxA, sf)
		Expect(err).NotTo(HaveOccurred())
		defer db.Resolve(txCtx)
		result, svcErr := svc.ProcessAdapterStatus(txCtx, tenancyClusterKind, created.ID, adapterStatus)
		Expect(svcErr).To(BeNil())
		Expect(result).NotTo(BeNil())
	})

	t.Run("system identity status write succeeds", func(t *testing.T) {
		RegisterTestingT(t)
		txCtx, err := db.NewContext(ctxSystem, sf)
		Expect(err).NotTo(HaveOccurred())
		defer db.Resolve(txCtx)
		result, svcErr := svc.ProcessAdapterStatus(txCtx, tenancyClusterKind, created.ID, adapterStatus)
		Expect(svcErr).To(BeNil())
		Expect(result).NotTo(BeNil())
	})

	t.Run("cross-tenant status write returns 404", func(t *testing.T) {
		RegisterTestingT(t)
		txCtx, err := db.NewContext(ctxB, sf)
		Expect(err).NotTo(HaveOccurred())
		defer db.Resolve(txCtx)
		_, svcErr := svc.ProcessAdapterStatus(txCtx, tenancyClusterKind, created.ID, adapterStatus)
		Expect(svcErr).NotTo(BeNil())
		Expect(svcErr.Is404()).To(BeTrue())
	})
}

// TestResourceScopeSearchFiltersComposeWithTenancy verifies that TSL search and label
// filters compose with tenant scoping: a tenant caller searching for a label that exists
// on both their own and another tenant's resources sees only their own matches, and the
// paging total reflects the scoped+filtered count.
func TestResourceScopeSearchFiltersComposeWithTenancy(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	svc := h.Container.ResourceService()
	sf := h.Container.SessionFactory()

	ctxA, ctxB, _ := twoTenantContexts(h)

	// Both tenants create a resource with the same label value.
	clusterA, svcErr := createInTx(ctxA, sf, svc, newTenancyCluster("scope-search-a"))
	Expect(svcErr).To(BeNil())
	_, svcErr = patchInTx(ctxA, sf, svc, tenancyClusterKind, clusterA.ID,
		&api.ResourcePatch{Labels: map[string]string{"env": "prod"}})
	Expect(svcErr).To(BeNil())

	clusterB, svcErr := createInTx(ctxB, sf, svc, newTenancyCluster("scope-search-b"))
	Expect(svcErr).To(BeNil())
	_, svcErr = patchInTx(ctxB, sf, svc, tenancyClusterKind, clusterB.ID,
		&api.ResourcePatch{Labels: map[string]string{"env": "prod"}})
	Expect(svcErr).To(BeNil())

	searchArgs := &services.ListArguments{
		Page:   1,
		Size:   100,
		Search: "labels.env='prod'",
	}

	t.Run("tenant A search sees only its own matching resource", func(t *testing.T) {
		RegisterTestingT(t)
		list, paging, svcErr := svc.List(ctxA, tenancyClusterKind, searchArgs)
		Expect(svcErr).To(BeNil())
		Expect(idsOf(list)).To(ContainElement(clusterA.ID))
		Expect(idsOf(list)).NotTo(ContainElement(clusterB.ID))
		Expect(paging.Total).To(Equal(int64(1)))
	})

	t.Run("tenant B search sees only its own matching resource", func(t *testing.T) {
		RegisterTestingT(t)
		list, paging, svcErr := svc.List(ctxB, tenancyClusterKind, searchArgs)
		Expect(svcErr).To(BeNil())
		Expect(idsOf(list)).To(ContainElement(clusterB.ID))
		Expect(idsOf(list)).NotTo(ContainElement(clusterA.ID))
		Expect(paging.Total).To(Equal(int64(1)))
	})
}

// TestResourceScopeContainmentHierarchy verifies that JSONB containment gives hierarchy
// for free: an org-scoped caller sees resources across all projects within that org,
// a project-scoped caller sees only its own project, and a different org sees neither.
func TestResourceScopeContainmentHierarchy(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	svc := h.Container.ResourceService()
	sf := h.Container.SessionFactory()

	orgID := "acme-" + h.NewID()
	projectAlpha := "alpha-" + h.NewID()
	projectBeta := "beta-" + h.NewID()
	otherOrg := "globex-" + h.NewID()

	// Project-level contexts (two projects under the same org).
	ctxAlpha := tenancyCtx(map[string]string{"org": orgID, "project": projectAlpha})
	ctxBeta := tenancyCtx(map[string]string{"org": orgID, "project": projectBeta})
	// Org-level context (sees across both projects).
	ctxOrg := tenancyCtx(map[string]string{"org": orgID})
	// Different org (sees neither).
	ctxOther := tenancyCtx(map[string]string{"org": otherOrg})

	clusterAlpha, svcErr := createInTx(ctxAlpha, sf, svc, newTenancyCluster("hierarchy-alpha"))
	Expect(svcErr).To(BeNil())

	clusterBeta, svcErr := createInTx(ctxBeta, sf, svc, newTenancyCluster("hierarchy-beta"))
	Expect(svcErr).To(BeNil())

	t.Run("org-scoped caller sees resources across both projects", func(t *testing.T) {
		RegisterTestingT(t)
		list, _, svcErr := svc.List(ctxOrg, tenancyClusterKind, services.NewListArguments())
		Expect(svcErr).To(BeNil())
		ids := idsOf(list)
		Expect(ids).To(ContainElement(clusterAlpha.ID))
		Expect(ids).To(ContainElement(clusterBeta.ID))
	})

	t.Run("project-scoped caller sees only its own project", func(t *testing.T) {
		RegisterTestingT(t)
		list, _, svcErr := svc.List(ctxAlpha, tenancyClusterKind, services.NewListArguments())
		Expect(svcErr).To(BeNil())
		ids := idsOf(list)
		Expect(ids).To(ContainElement(clusterAlpha.ID))
		Expect(ids).NotTo(ContainElement(clusterBeta.ID))
	})

	t.Run("different org sees neither", func(t *testing.T) {
		RegisterTestingT(t)
		list, _, svcErr := svc.List(ctxOther, tenancyClusterKind, services.NewListArguments())
		Expect(svcErr).To(BeNil())
		ids := idsOf(list)
		Expect(ids).NotTo(ContainElement(clusterAlpha.ID))
		Expect(ids).NotTo(ContainElement(clusterBeta.ID))
	})
}

// TestResourceScopeEmptyTenancyVisibility verifies that resources with an empty tenancy
// map ({}) are visible to system callers but invisible to any tenant-scoped caller.
// Empty-tenancy rows represent pre-migration or system-seeded data; the JSONB containment
// predicate naturally excludes them because {} does not contain any non-empty dimension set.
func TestResourceScopeEmptyTenancyVisibility(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	svc := h.Container.ResourceService()
	sf := h.Container.SessionFactory()

	ctxA, _, ctxSystem := twoTenantContexts(h)

	// Create as tenant A (system identity can't create per HYPERFLEET-1472), then
	// overwrite tenancy to {} at the DB level to simulate a legacy/unscoped row.
	created, svcErr := createInTx(ctxA, sf, svc, newTenancyCluster("scope-empty-tenancy"))
	Expect(svcErr).To(BeNil())

	g := sf.New(context.Background())
	Expect(g.Model(&api.Resource{}).Where("id = ?", created.ID).
		Update("tenancy", datatypes.JSON([]byte("{}"))).Error).To(Succeed())

	t.Run("system caller sees the empty-tenancy row via Get", func(t *testing.T) {
		RegisterTestingT(t)
		got, svcErr := svc.Get(ctxSystem, tenancyClusterKind, created.ID)
		Expect(svcErr).To(BeNil())
		Expect(got.ID).To(Equal(created.ID))
	})

	t.Run("system caller includes it in List", func(t *testing.T) {
		RegisterTestingT(t)
		list, _, svcErr := svc.List(ctxSystem, tenancyClusterKind, services.NewListArguments())
		Expect(svcErr).To(BeNil())
		Expect(idsOf(list)).To(ContainElement(created.ID))
	})

	t.Run("tenant caller gets 404 on Get", func(t *testing.T) {
		RegisterTestingT(t)
		_, svcErr := svc.Get(ctxA, tenancyClusterKind, created.ID)
		Expect(svcErr).NotTo(BeNil())
		Expect(svcErr.Is404()).To(BeTrue())
	})

	t.Run("tenant caller List omits it and total excludes it", func(t *testing.T) {
		RegisterTestingT(t)
		list, paging, svcErr := svc.List(ctxA, tenancyClusterKind, services.NewListArguments())
		Expect(svcErr).To(BeNil())
		Expect(idsOf(list)).NotTo(ContainElement(created.ID))
		Expect(paging.Total).To(Equal(int64(0)),
			"total must exclude empty-tenancy rows for tenant callers")
	})
}
