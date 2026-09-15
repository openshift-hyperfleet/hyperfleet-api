package integration

import (
	"context"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/tenant"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

const (
	tenancyClusterKind = "Cluster"
	tenancyClusterName = "prod"
	tenancyTestActor   = "test@example.com"
	tenancyOrgKey      = "org"
)

// tenancyCtx returns a context carrying a resolved tenant with the given dimensions.
func tenancyCtx(dimensions map[string]string) context.Context {
	return tenant.WithTenant(context.Background(), &tenant.ResolvedTenant{Dimensions: dimensions})
}

// newTenancyCluster builds a fresh Cluster resource. Create mutates the resource
// (ID, timestamps, tenancy), so each attempt needs its own instance.
func newTenancyCluster(name string) *api.Resource {
	return &api.Resource{
		Kind:      tenancyClusterKind,
		Name:      name,
		Spec:      []byte(`{"region": "us-central1", "provider": "gcp"}`),
		CreatedBy: tenancyTestActor,
		UpdatedBy: tenancyTestActor,
	}
}

// TestClusterNameUniquenessIsTenantScoped verifies that root-resource name uniqueness is
// scoped to the caller's tenancy: two tenants may own a cluster with the same name, while a
// same-name create within a single tenancy still conflicts. It drives the service layer
// directly with distinct tenant contexts because the tenant-resolution middleware is not yet
// wired into the HTTP path.
func TestClusterNameUniquenessIsTenantScoped(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	svc := h.Container.ResourceService()

	ctxAcme := tenancyCtx(map[string]string{tenancyOrgKey: "acme"})
	ctxGlobex := tenancyCtx(map[string]string{tenancyOrgKey: "globex"})

	// Tenant "acme" creates a cluster named "prod". The tenancy is stamped from the context.
	acme, svcErr := svc.Create(ctxAcme, tenancyClusterKind, newTenancyCluster(tenancyClusterName), nil)
	Expect(svcErr).To(BeNil())
	Expect(acme.Tenancy).To(MatchJSON(`{"org":"acme"}`))

	// Tenant "globex" creates a cluster with the SAME name — allowed, different tenancy.
	globex, svcErr := svc.Create(ctxGlobex, tenancyClusterKind, newTenancyCluster(tenancyClusterName), nil)
	Expect(svcErr).To(BeNil(), "cross-tenant same-name create should succeed")
	Expect(globex.Tenancy).To(MatchJSON(`{"org":"globex"}`))

	// Tenant "acme" creates "prod" again — conflict within its own tenancy.
	_, svcErr = svc.Create(ctxAcme, tenancyClusterKind, newTenancyCluster(tenancyClusterName), nil)
	Expect(svcErr).NotTo(BeNil(), "same-tenant duplicate name should be rejected")
	Expect(svcErr.IsConflict()).To(BeTrue())
	Expect(svcErr.HTTPCode).To(Equal(http.StatusConflict))
}

// TestClusterRecreateAfterDeleteWithTenancy verifies the soft-delete-and-recreate flow still
// works under the tenant-scoped unique index: a name freed by deletion can be reused within
// the same tenancy.
func TestClusterRecreateAfterDeleteWithTenancy(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	svc := h.Container.ResourceService()

	ctxAcme := tenancyCtx(map[string]string{tenancyOrgKey: "acme"})

	created, svcErr := svc.Create(ctxAcme, tenancyClusterKind, newTenancyCluster(tenancyClusterName), nil)
	Expect(svcErr).To(BeNil())

	// A second create with the same name in the same tenancy conflicts.
	_, svcErr = svc.Create(ctxAcme, tenancyClusterKind, newTenancyCluster(tenancyClusterName), nil)
	Expect(svcErr).NotTo(BeNil(), "same-tenant duplicate name should be rejected")
	Expect(svcErr.IsConflict()).To(BeTrue())
	Expect(svcErr.HTTPCode).To(Equal(http.StatusConflict))

	// Delete the original, then recreate with the same name — the deleted row no longer
	// participates in the unique index.
	_, svcErr = svc.Delete(ctxAcme, tenancyClusterKind, created.ID)
	Expect(svcErr).To(BeNil())

	_, svcErr = svc.Create(ctxAcme, tenancyClusterKind, newTenancyCluster(tenancyClusterName), nil)
	Expect(svcErr).To(BeNil(), "recreate after delete should succeed")
}
