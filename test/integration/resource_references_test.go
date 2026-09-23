package integration

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/util"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

var registerRefsOnce sync.Once

// --- Helper Functions ---

// registerRefTestDescriptors registers entity descriptors needed for resource
// reference integration tests.  Must be called before setupResourceTest so
// that the registry knows about these kinds when services/DAO try to resolve them.
func registerRefTestDescriptors() {
	registerRefsOnce.Do(func() {
		registry.Register(registry.EntityDescriptor{
			Kind:   "RefTarget",
			Plural: "reftargets",
		})
		registry.Register(registry.EntityDescriptor{
			Kind:   "RefSource",
			Plural: "refsources",
			References: []registry.ReferenceDescriptor{
				{RefType: "dep", TargetKind: "RefTarget", Min: 1, Max: 1},
			},
		})
		registry.Register(registry.EntityDescriptor{
			Kind:   "OptSource",
			Plural: "optsources",
			References: []registry.ReferenceDescriptor{
				{RefType: "link", TargetKind: "RefTarget", Min: 0, Max: 0},
			},
		})
		registry.Register(registry.EntityDescriptor{
			Kind:   "MultiSource",
			Plural: "multisources",
			References: []registry.ReferenceDescriptor{
				{RefType: "dep", TargetKind: "RefTarget", Min: 1, Max: 0},
			},
		})
		registry.Register(registry.EntityDescriptor{Kind: "RefTree", Plural: "reftrees"})
		registry.Register(registry.EntityDescriptor{
			Kind:           "RefChild",
			Plural:         "refchildren",
			ParentKind:     "RefTree",
			OnParentDelete: registry.OnParentDeleteCascade,
			References: []registry.ReferenceDescriptor{
				{RefType: "peer", TargetKind: "RefChild", Min: 0, Max: 1},
			},
		})
		registry.Register(registry.EntityDescriptor{
			Kind: "RefGrandchild", Plural: "refgrandchildren", ParentKind: "RefChild",
			OnParentDelete: registry.OnParentDeleteCascade,
		})
		registry.Register(registry.EntityDescriptor{
			Kind:   "RefChildSource",
			Plural: "refchildsources",
			References: []registry.ReferenceDescriptor{
				{RefType: "child", TargetKind: "RefChild", Min: 0, Max: 1},
			},
		})
	})
}

func setupRefTest(t *testing.T) (services.ResourceService, *test.Helper) {
	t.Helper()
	registerRefTestDescriptors()
	return setupResourceTest(t)
}

func newRefTestResource(kind, name string) *api.Resource {
	return &api.Resource{
		Kind:      kind,
		Name:      name,
		Spec:      []byte(`{"key": "value"}`),
		CreatedBy: "test@example.com",
		UpdatedBy: "test@example.com",
	}
}

func newOwnedRefTestResource(kind, name, ownerKind, ownerID string) *api.Resource {
	resource := newRefTestResource(kind, name)
	resource.OwnerID = util.ToPtr(ownerID)
	resource.OwnerKind = util.ToPtr(ownerKind)
	return resource
}

// makeRefs builds a reference map with a single ref type and target(s).
func makeRefs(refType string, targets ...struct{ id, kind string }) api.ReferenceMap {
	refs := make([]openapi.ObjectReference, len(targets))
	for i, t := range targets {
		refs[i] = openapi.ObjectReference{Id: util.ToPtr(t.id), Kind: t.kind}
	}
	return api.ReferenceMap{refType: refs}
}

// setupWifConfigRef temporarily adds a wif_config reference to the Cluster
// descriptor so Cluster → WifConfig reference tests
func setupWifConfigRef(t *testing.T) {
	t.Helper()
	registry.UpdateDescriptor("Cluster", func(d *registry.EntityDescriptor) {
		d.References = []registry.ReferenceDescriptor{
			{RefType: "wif_config", TargetKind: "WifConfig", Min: 1, Max: 1},
		}
	})
	t.Cleanup(func() {
		registry.UpdateDescriptor("Cluster", func(d *registry.EntityDescriptor) {
			d.References = nil
		})
	})
}

// --- Create ---

func TestResourceReferences_CreateWithValidRef(t *testing.T) {
	RegisterTestingT(t)
	svc, _ := setupRefTest(t)

	targetName := fmt.Sprintf("target-%s", uuid.NewString()[:8])
	target, svcErr := svc.Create(t.Context(), "RefTarget", newRefTestResource("RefTarget", targetName), nil)
	Expect(svcErr).To(BeNil(), "creating target should succeed")

	sourceName := fmt.Sprintf("source-%s", uuid.NewString()[:8])
	refs := makeRefs("dep", struct{ id, kind string }{target.ID, "RefTarget"})
	source, svcErr := svc.Create(t.Context(), "RefSource", newRefTestResource("RefSource", sourceName), refs)
	Expect(svcErr).To(BeNil(), "creating source with valid ref should succeed")
	Expect(source.ID).NotTo(BeEmpty())

	// GET should return the resource with references preloaded.
	retrieved, svcErr := svc.Get(t.Context(), "RefSource", source.ID)
	Expect(svcErr).To(BeNil(), "get source should succeed")
	Expect(retrieved.References).To(HaveLen(1), "should have exactly one reference row")
	Expect(retrieved.References[0].RefType).To(Equal("dep"))
	Expect(retrieved.References[0].TargetID).To(Equal(target.ID))
	Expect(retrieved.References[0].TargetKind).To(Equal("RefTarget"))
	Expect(retrieved.References[0].SourceID).To(Equal(source.ID))
}

func TestResourceReferences_CreateMissingRequiredRef(t *testing.T) {
	RegisterTestingT(t)
	svc, _ := setupRefTest(t)

	sourceName := fmt.Sprintf("source-noreq-%s", uuid.NewString()[:8])
	// RefSource has Min=1 on "dep", so creating without refs should fail.
	_, svcErr := svc.Create(t.Context(), "RefSource", newRefTestResource("RefSource", sourceName), nil)
	Expect(svcErr).NotTo(BeNil(), "create without required ref should fail")
	Expect(svcErr.HTTPCode).To(Equal(400))
}

func TestResourceReferences_CreateRefToNonExistentTarget(t *testing.T) {
	RegisterTestingT(t)
	svc, _ := setupRefTest(t)

	fakeID := uuid.NewString()
	sourceName := fmt.Sprintf("source-ghost-%s", uuid.NewString()[:8])
	refs := makeRefs("dep", struct{ id, kind string }{fakeID, "RefTarget"})

	_, svcErr := svc.Create(t.Context(), "RefSource", newRefTestResource("RefSource", sourceName), refs)
	Expect(svcErr).NotTo(BeNil(), "ref to non-existent target should fail")
	Expect(svcErr.HTTPCode).To(Equal(400))
}

func TestResourceReferences_CreateTooManyRefs(t *testing.T) {
	RegisterTestingT(t)
	svc, _ := setupRefTest(t)

	// Create two targets.
	target1, svcErr := svc.Create(t.Context(), "RefTarget",
		newRefTestResource("RefTarget", fmt.Sprintf("target1-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())

	target2, svcErr := svc.Create(t.Context(), "RefTarget",
		newRefTestResource("RefTarget", fmt.Sprintf("target2-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())

	// RefSource has Max=1 on "dep" — supplying 2 should fail.
	sourceName := fmt.Sprintf("source-toomany-%s", uuid.NewString()[:8])
	refs := makeRefs("dep",
		struct{ id, kind string }{target1.ID, "RefTarget"},
		struct{ id, kind string }{target2.ID, "RefTarget"},
	)

	_, svcErr = svc.Create(t.Context(), "RefSource", newRefTestResource("RefSource", sourceName), refs)
	Expect(svcErr).NotTo(BeNil(), "exceeding Max refs should fail")
	Expect(svcErr.HTTPCode).To(Equal(400))
}

// --- Patch ---

func TestResourceReferences_PatchReplacesAtomically(t *testing.T) {
	RegisterTestingT(t)
	svc, _ := setupRefTest(t)

	target1, svcErr := svc.Create(t.Context(), "RefTarget",
		newRefTestResource("RefTarget", fmt.Sprintf("target1-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())

	target2, svcErr := svc.Create(t.Context(), "RefTarget",
		newRefTestResource("RefTarget", fmt.Sprintf("target2-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())

	// Create source pointing to target1.
	sourceName := fmt.Sprintf("source-swap-%s", uuid.NewString()[:8])
	refs := makeRefs("dep", struct{ id, kind string }{target1.ID, "RefTarget"})
	source, svcErr := svc.Create(t.Context(), "RefSource", newRefTestResource("RefSource", sourceName), refs)
	Expect(svcErr).To(BeNil())

	// Patch to point to target2.
	patchRefs := api.ReferenceMap{
		"dep": {{Id: util.ToPtr(target2.ID), Kind: "RefTarget"}},
	}
	_, svcErr = svc.Patch(t.Context(), "RefSource", source.ID, &api.ResourcePatch{
		References: patchRefs,
	})
	Expect(svcErr).To(BeNil(), "patch should succeed")

	// GET should now show only target2.
	retrieved, svcErr := svc.Get(t.Context(), "RefSource", source.ID)
	Expect(svcErr).To(BeNil())
	Expect(retrieved.References).To(HaveLen(1))
	Expect(retrieved.References[0].TargetID).To(Equal(target2.ID))
}

func TestResourceReferences_PatchNilRefsIsNoOp(t *testing.T) {
	RegisterTestingT(t)
	svc, _ := setupRefTest(t)

	target, svcErr := svc.Create(t.Context(), "RefTarget",
		newRefTestResource("RefTarget", fmt.Sprintf("target-noop-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())

	sourceName := fmt.Sprintf("source-noop-%s", uuid.NewString()[:8])
	refs := makeRefs("dep", struct{ id, kind string }{target.ID, "RefTarget"})
	source, svcErr := svc.Create(t.Context(), "RefSource", newRefTestResource("RefSource", sourceName), refs)
	Expect(svcErr).To(BeNil())

	// Patch with spec change only — no references field (nil).
	_, svcErr = svc.Patch(t.Context(), "RefSource", source.ID, &api.ResourcePatch{
		Spec: map[string]interface{}{"key": "updated"},
	})
	Expect(svcErr).To(BeNil(), "patch spec-only should succeed")

	// References should be unchanged.
	retrieved, svcErr := svc.Get(t.Context(), "RefSource", source.ID)
	Expect(svcErr).To(BeNil())
	Expect(retrieved.References).To(HaveLen(1))
	Expect(retrieved.References[0].TargetID).To(Equal(target.ID))
}

func TestResourceReferences_PatchEmptyMapViolatesMin(t *testing.T) {
	RegisterTestingT(t)
	svc, _ := setupRefTest(t)

	target, svcErr := svc.Create(t.Context(), "RefTarget",
		newRefTestResource("RefTarget", fmt.Sprintf("target-empty-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())

	sourceName := fmt.Sprintf("source-empty-%s", uuid.NewString()[:8])
	refs := makeRefs("dep", struct{ id, kind string }{target.ID, "RefTarget"})
	source, svcErr := svc.Create(t.Context(), "RefSource", newRefTestResource("RefSource", sourceName), refs)
	Expect(svcErr).To(BeNil())

	// Patch with empty references map — Min=1 should reject.
	_, svcErr = svc.Patch(t.Context(), "RefSource", source.ID, &api.ResourcePatch{
		References: api.ReferenceMap{},
	})
	Expect(svcErr).NotTo(BeNil(), "clearing required refs should fail")
	Expect(svcErr.HTTPCode).To(Equal(400))
}

// --- Delete ---

func TestResourceReferences_Delete(t *testing.T) {
	suffix := func(label string) string {
		return fmt.Sprintf("%s-%s", label, uuid.NewString()[:8])
	}

	type refPair struct {
		setup   func(t *testing.T)
		name    string
		target  string
		source  string
		refType string
	}

	pairs := []refPair{
		{
			name:    "RefTarget_RefSource",
			target:  "RefTarget",
			source:  "RefSource",
			refType: "dep",
		},
		{
			name:    "WifConfig_Cluster",
			target:  "WifConfig",
			source:  "Cluster",
			refType: "wif_config",
			setup:   setupWifConfigRef,
		},
	}

	for _, p := range pairs {
		t.Run(p.name, func(t *testing.T) {
			if p.setup != nil {
				p.setup(t)
			}

			t.Run("UnreferencedTarget_Succeeds", func(t *testing.T) {
				RegisterTestingT(t)
				svc, h := setupRefTest(t)

				target, svcErr := svc.Create(t.Context(), p.target,
					newRefTestResource(p.target, suffix("target-unref")), nil)
				Expect(svcErr).To(BeNil())

				deleted, svcErr := svc.Delete(t.Context(), p.target, target.ID)
				Expect(svcErr).To(BeNil(), "delete of unreferenced target should succeed")
				Expect(deleted.DeletedTime).NotTo(BeNil())

				dbErr := checkResourceCount(t.Context(), h, []string{target.ID}, 0)
				Expect(dbErr).To(BeNil(), "target should be hard-deleted from DB")
			})

			t.Run("ReferencedTarget_Returns409", func(t *testing.T) {
				RegisterTestingT(t)
				svc, _ := setupRefTest(t)

				target, svcErr := svc.Create(t.Context(), p.target,
					newRefTestResource(p.target, suffix("target-block")), nil)
				Expect(svcErr).To(BeNil())

				refs := makeRefs(p.refType, struct{ id, kind string }{target.ID, p.target})
				_, svcErr = svc.Create(t.Context(), p.source,
					newRefTestResource(p.source, suffix("source-block")), refs)
				Expect(svcErr).To(BeNil())

				_, svcErr = svc.Delete(t.Context(), p.target, target.ID)
				Expect(svcErr).NotTo(BeNil(), "delete of referenced target should fail")
				Expect(svcErr.HTTPCode).To(Equal(409))
			})

			t.Run("DeleteSourceThenTarget_Succeeds", func(t *testing.T) {
				RegisterTestingT(t)
				svc, h := setupRefTest(t)

				target, svcErr := svc.Create(t.Context(), p.target,
					newRefTestResource(p.target, suffix("target-delsrc")), nil)
				Expect(svcErr).To(BeNil())

				refs := makeRefs(p.refType, struct{ id, kind string }{target.ID, p.target})
				source, svcErr := svc.Create(t.Context(), p.source,
					newRefTestResource(p.source, suffix("source-delsrc")), refs)
				Expect(svcErr).To(BeNil())

				deleted, svcErr := svc.Delete(t.Context(), p.source, source.ID)
				Expect(svcErr).To(BeNil(), "delete source should succeed")
				Expect(deleted.DeletedTime).NotTo(BeNil())

				deletedTarget, svcErr := svc.Delete(t.Context(), p.target, target.ID)
				Expect(svcErr).To(BeNil(), "delete target after source removal should succeed")
				Expect(deletedTarget.DeletedTime).NotTo(BeNil())

				dbErr := checkResourceCount(t.Context(), h, []string{target.ID}, 0)
				Expect(dbErr).To(BeNil(), "target should be hard-deleted from DB")
			})

			t.Run("ForceDeleteReferencedRequiredTarget_Returns409", func(t *testing.T) {
				RegisterTestingT(t)
				svc, h := setupRefTest(t)

				target, svcErr := svc.Create(t.Context(), p.target,
					newRefTestResource(p.target, suffix("target-fd")), nil)
				Expect(svcErr).To(BeNil())

				refs := makeRefs(p.refType, struct{ id, kind string }{target.ID, p.target})
				source, svcErr := svc.Create(t.Context(), p.source,
					newRefTestResource(p.source, suffix("source-fd")), refs)
				Expect(svcErr).To(BeNil())

				_, svcErr = svc.Delete(t.Context(), p.target, target.ID)
				Expect(svcErr).ToNot(BeNil(), "regular delete should fail — target is referenced")
				Expect(svcErr.HTTPCode).To(Equal(409))

				markFinalizing(t, h, target.ID)

				svcErr = svc.ForceDelete(t.Context(), p.target, target.ID, "force delete target")
				Expect(svcErr).NotTo(BeNil(), "force-delete should reject deleting a required reference")
				Expect(svcErr.HTTPCode).To(Equal(409))

				retainedTarget, getErr := svc.Get(t.Context(), p.target, target.ID)
				Expect(getErr).To(BeNil(), "target should remain after rejected force-delete")
				Expect(retainedTarget.DeletedTime).NotTo(BeNil(), "target should remain finalizing")

				source, getErr = svc.Get(t.Context(), p.source, source.ID)
				Expect(getErr).To(BeNil())
				Expect(source.References).To(HaveLen(1), "required reference should remain intact")
				Expect(source.References[0].TargetID).To(Equal(target.ID))
				Expect(source.References[0].RefType).To(Equal(p.refType))
			})
		})
	}
}

func TestResourceReferences_ForceDeleteOptionalExternalReferenceSucceeds(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupRefTest(t)

	target, svcErr := svc.Create(t.Context(), "RefTarget",
		newRefTestResource("RefTarget", fmt.Sprintf("target-opt-delete-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())
	refs := makeRefs("link", struct{ id, kind string }{target.ID, "RefTarget"})
	source, svcErr := svc.Create(t.Context(), "OptSource",
		newRefTestResource("OptSource", fmt.Sprintf("source-opt-delete-%s", uuid.NewString()[:8])), refs)
	Expect(svcErr).To(BeNil())

	_, svcErr = svc.Delete(t.Context(), "RefTarget", target.ID)
	Expect(svcErr).NotTo(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(409))

	markFinalizing(t, h, target.ID)
	svcErr = svc.ForceDelete(t.Context(), "RefTarget", target.ID, "verify optional reference boundary")
	Expect(svcErr).To(BeNil(), "force-delete should allow clearing an optional reference")
	_, getErr := svc.Get(t.Context(), "RefTarget", target.ID)
	Expect(getErr).NotTo(BeNil())
	Expect(getErr.HTTPCode).To(Equal(404))

	retrieved, getErr := svc.Get(t.Context(), "OptSource", source.ID)
	Expect(getErr).To(BeNil())
	Expect(retrieved.References).To(BeEmpty(), "force-delete should clear the optional reference")
}

func TestResourceReferences_ForceDeleteRetainsEnoughRequiredReferences(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupRefTest(t)

	target1, svcErr := svc.Create(t.Context(), "RefTarget",
		newRefTestResource("RefTarget", fmt.Sprintf("target-multi-one-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())
	target2, svcErr := svc.Create(t.Context(), "RefTarget",
		newRefTestResource("RefTarget", fmt.Sprintf("target-multi-two-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())
	source, svcErr := svc.Create(t.Context(), "MultiSource",
		newRefTestResource("MultiSource", fmt.Sprintf("source-multi-%s", uuid.NewString()[:8])),
		makeRefs("dep",
			struct{ id, kind string }{target1.ID, "RefTarget"},
			struct{ id, kind string }{target2.ID, "RefTarget"}))
	Expect(svcErr).To(BeNil())

	_, svcErr = svc.Delete(t.Context(), "RefTarget", target1.ID)
	Expect(svcErr).NotTo(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(409))
	markFinalizing(t, h, target1.ID)
	Expect(svc.ForceDelete(t.Context(), "RefTarget", target1.ID, "retain another required reference")).To(BeNil())

	_, getErr := svc.Get(t.Context(), "RefTarget", target1.ID)
	Expect(getErr).NotTo(BeNil())
	Expect(getErr.HTTPCode).To(Equal(404))
	retrieved, getErr := svc.Get(t.Context(), "MultiSource", source.ID)
	Expect(getErr).To(BeNil())
	Expect(retrieved.References).To(HaveLen(1))
	Expect(retrieved.References[0].TargetID).To(Equal(target2.ID))
	Expect(retrieved.References[0].RefType).To(Equal("dep"))
}

func TestResourceReferences_InternalSiblingReferencesDeleteWithTree(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupRefTest(t)

	root, svcErr := svc.Create(t.Context(), "RefTree",
		newRefTestResource("RefTree", fmt.Sprintf("tree-internal-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())
	child1, svcErr := svc.Create(t.Context(), "RefChild",
		newOwnedRefTestResource("RefChild", fmt.Sprintf("child-one-%s", uuid.NewString()[:8]), "RefTree", root.ID), nil)
	Expect(svcErr).To(BeNil())
	child2Refs := makeRefs("peer", struct{ id, kind string }{child1.ID, "RefChild"})
	child2, svcErr := svc.Create(t.Context(), "RefChild",
		newOwnedRefTestResource("RefChild", fmt.Sprintf("child-two-%s", uuid.NewString()[:8]), "RefTree", root.ID),
		child2Refs)
	Expect(svcErr).To(BeNil())
	_, svcErr = svc.Patch(t.Context(), "RefChild", child1.ID, &api.ResourcePatch{
		References: makeRefs("peer", struct{ id, kind string }{child2.ID, "RefChild"}),
	})
	Expect(svcErr).To(BeNil())
	grandchildResource := newOwnedRefTestResource("RefGrandchild",
		fmt.Sprintf("grandchild-%s", uuid.NewString()[:8]), "RefChild", child1.ID)
	grandchild, svcErr := svc.Create(t.Context(), "RefGrandchild", grandchildResource, nil)
	Expect(svcErr).To(BeNil())

	markFinalizing(t, h, root.ID)
	Expect(svc.ForceDelete(t.Context(), "RefTree", root.ID, "remove internally referenced tree")).To(BeNil())
	Expect(checkResourceCount(t.Context(), h, []string{root.ID, child1.ID, child2.ID, grandchild.ID}, 0)).To(Succeed())

	var referenceCount int64
	Expect(h.DBFactory.New(t.Context()).Table("resource_references").
		Where("source_id IN ?", []string{child1.ID, child2.ID}).
		Count(&referenceCount).Error).To(Succeed())
	Expect(referenceCount).To(BeZero())
}

func TestResourceReferences_ForceDeleteClearsOptionalDescendantReference(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupRefTest(t)

	root, svcErr := svc.Create(t.Context(), "RefTree",
		newRefTestResource("RefTree", fmt.Sprintf("tree-external-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())
	child, svcErr := svc.Create(t.Context(), "RefChild",
		newOwnedRefTestResource("RefChild", fmt.Sprintf("child-external-%s", uuid.NewString()[:8]), "RefTree", root.ID), nil)
	Expect(svcErr).To(BeNil())
	sourceRefs := makeRefs("child", struct{ id, kind string }{child.ID, "RefChild"})
	source, svcErr := svc.Create(t.Context(), "RefChildSource",
		newRefTestResource("RefChildSource", fmt.Sprintf("source-external-%s", uuid.NewString()[:8])), sourceRefs)
	Expect(svcErr).To(BeNil())

	_, svcErr = svc.Delete(t.Context(), "RefTree", root.ID)
	Expect(svcErr).NotTo(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(409))
	activeRoot, getErr := svc.Get(t.Context(), "RefTree", root.ID)
	Expect(getErr).To(BeNil())
	Expect(activeRoot.DeletedTime).To(BeNil(), "normal deletion must roll back the entire tree")

	markFinalizing(t, h, root.ID)
	svcErr = svc.ForceDelete(t.Context(), "RefTree", root.ID, "clear optional descendant reference")
	Expect(svcErr).To(BeNil())
	Expect(checkResourceCount(t.Context(), h, []string{root.ID, child.ID}, 0)).To(Succeed())
	retrievedSource, getErr := svc.Get(t.Context(), "RefChildSource", source.ID)
	Expect(getErr).To(BeNil())
	Expect(retrievedSource.References).To(BeEmpty())
}

func TestResourceReferences_ForceDeleteClearsOutboundReferenceToExternalTarget(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupRefTest(t)

	extRoot, svcErr := svc.Create(t.Context(), "RefTree",
		newRefTestResource("RefTree", fmt.Sprintf("tree-ext-survivor-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())
	extChild, svcErr := svc.Create(t.Context(), "RefChild",
		newOwnedRefTestResource("RefChild",
			fmt.Sprintf("child-ext-survivor-%s", uuid.NewString()[:8]), "RefTree", extRoot.ID), nil)
	Expect(svcErr).To(BeNil())

	root, svcErr := svc.Create(t.Context(), "RefTree",
		newRefTestResource("RefTree", fmt.Sprintf("tree-outbound-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())
	childRefs := makeRefs("peer", struct{ id, kind string }{extChild.ID, "RefChild"})
	child, svcErr := svc.Create(t.Context(), "RefChild",
		newOwnedRefTestResource("RefChild",
			fmt.Sprintf("child-outbound-%s", uuid.NewString()[:8]), "RefTree", root.ID),
		childRefs)
	Expect(svcErr).To(BeNil())

	markFinalizing(t, h, root.ID)
	svcErr = svc.ForceDelete(t.Context(), "RefTree", root.ID, "clear outbound reference to external target")
	Expect(svcErr).To(BeNil())

	Expect(checkResourceCount(t.Context(), h, []string{root.ID, child.ID}, 0)).To(Succeed())

	var refCount int64
	Expect(h.DBFactory.New(t.Context()).Table("resource_references").
		Where("source_id = ?", child.ID).
		Count(&refCount).Error).To(Succeed())
	Expect(refCount).To(BeZero(), "outbound reference row should be cascade-deleted with its source")

	Expect(checkResourceCount(t.Context(), h, []string{extRoot.ID, extChild.ID}, 2)).To(Succeed())
	survivor, getErr := svc.Get(t.Context(), "RefChild", extChild.ID)
	Expect(getErr).To(BeNil(), "external target must survive the tree deletion")
	Expect(survivor.DeletedTime).To(BeNil())
}

func TestResourceReferences_ConcurrentForceDeletesPreserveMin(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupRefTest(t)
	first, svcErr := svc.Create(t.Context(), "RefTarget",
		newRefTestResource("RefTarget", fmt.Sprintf("race-first-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())
	second, svcErr := svc.Create(t.Context(), "RefTarget",
		newRefTestResource("RefTarget", fmt.Sprintf("race-second-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())
	source, svcErr := svc.Create(t.Context(), "MultiSource",
		newRefTestResource("MultiSource", fmt.Sprintf("race-source-%s", uuid.NewString()[:8])),
		makeRefs("dep", struct{ id, kind string }{first.ID, "RefTarget"},
			struct{ id, kind string }{second.ID, "RefTarget"}))
	Expect(svcErr).To(BeNil())
	markFinalizing(t, h, first.ID)
	markFinalizing(t, h, second.ID)

	results := make(chan *errors.ServiceError, 2)
	ready := make(chan struct{}, 2)
	for _, id := range []string{first.ID, second.ID} {
		go func(id string) {
			ready <- struct{}{}
			results <- svc.ForceDelete(t.Context(), "RefTarget", id, "concurrent delete")
		}(id)
	}
	for range 2 {
		<-ready
	}

	var successes, conflicts int
	for range 2 {
		result := <-results
		switch {
		case result == nil:
			successes++
		case result.HTTPCode == 409:
			conflicts++
		default:
			t.Fatalf("unexpected concurrent deletion error: %v", result)
		}
	}
	Expect(successes).To(Equal(1))
	Expect(conflicts).To(Equal(1))
	retrieved, getErr := svc.Get(t.Context(), "MultiSource", source.ID)
	Expect(getErr).To(BeNil())
	Expect(retrieved.References).To(HaveLen(1))
}

// --- List with ref_type filter ---

func TestResourceReferences_ListByRefTypeAndTarget(t *testing.T) {
	RegisterTestingT(t)
	svc, _ := setupRefTest(t)

	// Create one target and two sources that reference it, plus one that does not.
	target, svcErr := svc.Create(t.Context(), "RefTarget",
		newRefTestResource("RefTarget", fmt.Sprintf("target-list-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())

	otherTarget, svcErr := svc.Create(t.Context(), "RefTarget",
		newRefTestResource("RefTarget", fmt.Sprintf("target-other-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())

	// source1 references target
	refs1 := makeRefs("dep", struct{ id, kind string }{target.ID, "RefTarget"})
	source1, svcErr := svc.Create(t.Context(), "RefSource",
		newRefTestResource("RefSource", fmt.Sprintf("source-list1-%s", uuid.NewString()[:8])), refs1)
	Expect(svcErr).To(BeNil())

	// source2 references target
	refs2 := makeRefs("dep", struct{ id, kind string }{target.ID, "RefTarget"})
	source2, svcErr := svc.Create(t.Context(), "RefSource",
		newRefTestResource("RefSource", fmt.Sprintf("source-list2-%s", uuid.NewString()[:8])), refs2)
	Expect(svcErr).To(BeNil())

	// source3 references otherTarget — should NOT appear in results.
	refs3 := makeRefs("dep", struct{ id, kind string }{otherTarget.ID, "RefTarget"})
	source3, svcErr := svc.Create(t.Context(), "RefSource",
		newRefTestResource("RefSource", fmt.Sprintf("source-list3-%s", uuid.NewString()[:8])), refs3)
	Expect(svcErr).To(BeNil())

	args := &services.ListArguments{
		Page:        1,
		Size:        100,
		RefType:     "dep",
		RefTargetID: target.ID,
	}
	list, _, svcErr := svc.List(t.Context(), "RefSource", args)
	Expect(svcErr).To(BeNil(), "list with ref filter should succeed")

	// Build set of returned IDs.
	foundIDs := make(map[string]bool, len(list))
	for _, item := range list {
		foundIDs[item.ID] = true
	}

	Expect(foundIDs).To(HaveKey(source1.ID), "source1 should be in results")
	Expect(foundIDs).To(HaveKey(source2.ID), "source2 should be in results")
	Expect(foundIDs).NotTo(HaveKey(source3.ID), "source3 should NOT be in results")
}

func TestResourceReferences_ListUnknownRefType(t *testing.T) {
	RegisterTestingT(t)
	svc, _ := setupRefTest(t)

	// RefSource has ref_type "dep" — querying "unknown" should fail.
	args := &services.ListArguments{
		Page:        1,
		Size:        20,
		RefType:     "unknown",
		RefTargetID: "some-id",
	}
	_, _, svcErr := svc.List(t.Context(), "RefSource", args)
	Expect(svcErr).NotTo(BeNil(), "unknown ref_type should fail")
	Expect(svcErr.HTTPCode).To(Equal(400))
}

// --- Optional references ---

func TestResourceReferences_OptionalRefCreate(t *testing.T) {
	RegisterTestingT(t)
	svc, _ := setupRefTest(t)

	// OptSource has Min=0 — creating without refs should succeed.
	sourceName := fmt.Sprintf("optsource-%s", uuid.NewString()[:8])
	source, svcErr := svc.Create(t.Context(), "OptSource", newRefTestResource("OptSource", sourceName), nil)
	Expect(svcErr).To(BeNil(), "optional ref entity should be created without refs")
	Expect(source.ID).NotTo(BeEmpty())

	// Create with a ref should also succeed.
	target, svcErr := svc.Create(t.Context(), "RefTarget",
		newRefTestResource("RefTarget", fmt.Sprintf("target-opt-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())

	source2Name := fmt.Sprintf("optsource2-%s", uuid.NewString()[:8])
	refs := makeRefs("link", struct{ id, kind string }{target.ID, "RefTarget"})
	source2, svcErr := svc.Create(t.Context(), "OptSource", newRefTestResource("OptSource", source2Name), refs)
	Expect(svcErr).To(BeNil())

	retrieved, svcErr := svc.Get(t.Context(), "OptSource", source2.ID)
	Expect(svcErr).To(BeNil())
	Expect(retrieved.References).To(HaveLen(1))
	Expect(retrieved.References[0].RefType).To(Equal("link"))
	Expect(retrieved.References[0].TargetID).To(Equal(target.ID))
}

func TestResourceReferences_PatchClearOptionalRefs(t *testing.T) {
	RegisterTestingT(t)
	svc, _ := setupRefTest(t)

	target, svcErr := svc.Create(t.Context(), "RefTarget",
		newRefTestResource("RefTarget", fmt.Sprintf("target-clropt-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())

	sourceName := fmt.Sprintf("optsource-clr-%s", uuid.NewString()[:8])
	refs := makeRefs("link", struct{ id, kind string }{target.ID, "RefTarget"})
	source, svcErr := svc.Create(t.Context(), "OptSource", newRefTestResource("OptSource", sourceName), refs)
	Expect(svcErr).To(BeNil())

	// Patch with empty references map — Min=0 should allow clearing.
	_, svcErr = svc.Patch(t.Context(), "OptSource", source.ID, &api.ResourcePatch{
		References: api.ReferenceMap{},
	})
	Expect(svcErr).To(BeNil(), "clearing optional refs should succeed")

	retrieved, svcErr := svc.Get(t.Context(), "OptSource", source.ID)
	Expect(svcErr).To(BeNil())
	Expect(retrieved.References).To(BeEmpty(), "references should be cleared")
}

func TestResourceReferences_CreateDuplicateRefTarget(t *testing.T) {
	RegisterTestingT(t)
	svc, _ := setupRefTest(t)

	target, svcErr := svc.Create(t.Context(), "RefTarget",
		newRefTestResource("RefTarget", fmt.Sprintf("target-dup-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())

	// Use OptSource (link ref type, Max=0 i.e. unlimited) so the duplicate
	// check is reached before any max-count validation.
	sourceName := fmt.Sprintf("source-dup-%s", uuid.NewString()[:8])
	refs := api.ReferenceMap{
		"link": {
			{Id: util.ToPtr(target.ID), Kind: "RefTarget"},
			{Id: util.ToPtr(target.ID), Kind: "RefTarget"}, // duplicate
		},
	}

	_, svcErr = svc.Create(t.Context(), "OptSource", newRefTestResource("OptSource", sourceName), refs)
	Expect(svcErr).NotTo(BeNil(), "duplicate ref target should fail")
	Expect(svcErr.HTTPCode).To(Equal(400))
	Expect(svcErr.Reason).To(ContainSubstring("duplicate target id"))
}
func TestResourceReferences_CreateInvalidKind(t *testing.T) {
	testCases := []struct {
		name        string
		kind        string
		description string
	}{
		{"WrongKind", "WrongKind", "wrong kind should fail validation"},
		{"EmptyKind", "", "empty kind should fail validation"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			svc, _ := setupRefTest(t)

			target, svcErr := svc.Create(t.Context(), "RefTarget",
				newRefTestResource("RefTarget",
					fmt.Sprintf("target-%s-%s", strings.ToLower(tc.name[:2]), uuid.NewString()[:8])),
				nil)
			Expect(svcErr).To(BeNil())

			sourceName := fmt.Sprintf("source-%s-%s", strings.ToLower(tc.name[:2]), uuid.NewString()[:8])
			refs := makeRefs("dep", struct{ id, kind string }{target.ID, tc.kind})

			_, svcErr = svc.Create(t.Context(), "RefSource", newRefTestResource("RefSource", sourceName), refs)
			Expect(svcErr).NotTo(BeNil(), tc.description)
			Expect(svcErr.HTTPCode).To(Equal(400))
			Expect(svcErr.Reason).To(ContainSubstring("does not match expected target kind"))
		})
	}
}

func TestResourceReferences_CreateRefToSoftDeletedTarget(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupRefTest(t)

	// Create a target, then soft-delete it via direct DB update
	// (svc.Delete would hard-delete since RefTarget has no required adapters).
	target, svcErr := svc.Create(t.Context(), "RefTarget",
		newRefTestResource("RefTarget", fmt.Sprintf("target-del-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())

	markFinalizing(t, h, target.ID)

	// Attempt to reference the soft-deleted target.
	sourceName := fmt.Sprintf("source-del-%s", uuid.NewString()[:8])
	refs := makeRefs("dep", struct{ id, kind string }{target.ID, "RefTarget"})

	_, svcErr = svc.Create(t.Context(), "RefSource", newRefTestResource("RefSource", sourceName), refs)
	Expect(svcErr).NotTo(BeNil(), "ref to soft-deleted target should fail")
	Expect(svcErr.HTTPCode).To(Equal(400))
	Expect(svcErr.Reason).To(ContainSubstring("marked for deletion"))
}
