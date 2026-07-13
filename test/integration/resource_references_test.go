package integration

import (
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/util"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

var registerRefsOnce sync.Once

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

// makeRefs builds a reference map with a single ref type and target(s).
func makeRefs(refType string, targets ...struct{ id, kind string }) api.ReferenceMap {
	refs := make([]openapi.ObjectReference, len(targets))
	for i, t := range targets {
		refs[i] = openapi.ObjectReference{Id: util.ToPtr(t.id), Kind: t.kind}
	}
	return api.ReferenceMap{refType: refs}
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

func TestResourceReferences_DeleteTargetWhileReferenced(t *testing.T) {
	RegisterTestingT(t)
	svc, _ := setupRefTest(t)

	target, svcErr := svc.Create(t.Context(), "RefTarget",
		newRefTestResource("RefTarget", fmt.Sprintf("target-block-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())

	sourceName := fmt.Sprintf("source-block-%s", uuid.NewString()[:8])
	refs := makeRefs("dep", struct{ id, kind string }{target.ID, "RefTarget"})
	_, svcErr = svc.Create(t.Context(), "RefSource", newRefTestResource("RefSource", sourceName), refs)
	Expect(svcErr).To(BeNil())

	// Attempt to delete target while referenced — expect 409.
	_, svcErr = svc.Delete(t.Context(), "RefTarget", target.ID)
	Expect(svcErr).NotTo(BeNil(), "delete of referenced target should fail")
	Expect(svcErr.HTTPCode).To(Equal(409))
}

func TestResourceReferences_DeleteSourceSucceeds(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupRefTest(t)

	target, svcErr := svc.Create(t.Context(), "RefTarget",
		newRefTestResource("RefTarget", fmt.Sprintf("target-delsrc-%s", uuid.NewString()[:8])), nil)
	Expect(svcErr).To(BeNil())

	sourceName := fmt.Sprintf("source-delsrc-%s", uuid.NewString()[:8])
	refs := makeRefs("dep", struct{ id, kind string }{target.ID, "RefTarget"})
	source, svcErr := svc.Create(t.Context(), "RefSource", newRefTestResource("RefSource", sourceName), refs)
	Expect(svcErr).To(BeNil())

	// Deleting the source should succeed — ON DELETE CASCADE cleans ref rows.
	deleted, svcErr := svc.Delete(t.Context(), "RefSource", source.ID)
	Expect(svcErr).To(BeNil(), "delete source should succeed")
	Expect(deleted.DeletedTime).NotTo(BeNil())

	// Source row should be gone (hard delete, no required adapters).
	dbErr := checkResourceCount(t.Context(), h, []string{source.ID}, 0)
	Expect(dbErr).To(BeNil())

	// Target should still exist.
	_, svcErr = svc.Get(t.Context(), "RefTarget", target.ID)
	Expect(svcErr).To(BeNil(), "target should still exist after source delete")
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

func TestResourceReferences_ForceDeleteReferencedTarget(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupRefTest(t)
	prefix := uuid.NewString()[:8]

	target, svcErr := svc.Create(t.Context(), "RefTarget",
		newRefTestResource("RefTarget", fmt.Sprintf("target-fd-%s", prefix)), nil)
	Expect(svcErr).To(BeNil())

	refs := makeRefs("dep", struct{ id, kind string }{target.ID, "RefTarget"})
	_, svcErr = svc.Create(t.Context(), "RefSource",
		newRefTestResource("RefSource", fmt.Sprintf("source-fd-%s", prefix)), refs)
	Expect(svcErr).To(BeNil())

	_, svcErr = svc.Delete(t.Context(), "RefTarget", target.ID)
	Expect(svcErr).ToNot(BeNil(), "regular delete should fail — target is referenced")
	Expect(svcErr.HTTPCode).To(Equal(409))

	markFinalizing(t, h, target.ID)

	svcErr = svc.ForceDelete(t.Context(), "RefTarget", target.ID, "integration test cleanup")
	Expect(svcErr).To(BeNil(), "force-delete should bypass reference restriction")

	_, getErr := svc.Get(t.Context(), "RefTarget", target.ID)
	Expect(getErr).ToNot(BeNil())
	Expect(getErr.HTTPCode).To(Equal(404), "target should be gone after force-delete")
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
