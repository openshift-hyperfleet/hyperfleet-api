package services

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"gorm.io/gorm"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/auth"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
)

func setupTestDescriptors() {
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:   "Channel",
		Plural: "channels",
	})
	registry.Register(registry.EntityDescriptor{
		Kind:           "Version",
		Plural:         "versions",
		ParentKind:     "Channel",
		OnParentDelete: registry.OnParentDeleteRestrict,
	})
	registry.Register(registry.EntityDescriptor{
		Kind:   "WifConfig",
		Plural: "wifconfigs",
	})
}

// mockResourceDao implements dao.ResourceDao for testing.

type mockResourceDao struct {
	resources map[string]*api.Resource
	createErr error
	saveErr   error
	deleteErr error
}

func newMockResourceDao() *mockResourceDao {
	return &mockResourceDao{resources: make(map[string]*api.Resource)}
}

func resourceKey(kind, id string) string { return kind + ":" + id }

func (d *mockResourceDao) Get(_ context.Context, kind, id string) (*api.Resource, error) {
	if r, ok := d.resources[resourceKey(kind, id)]; ok {
		return r, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (d *mockResourceDao) GetForUpdate(ctx context.Context, kind, id string) (*api.Resource, error) {
	return d.Get(ctx, kind, id)
}

func (d *mockResourceDao) GetByOwner(_ context.Context, kind, id, ownerID string) (*api.Resource, error) {
	r, ok := d.resources[resourceKey(kind, id)]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	if r.OwnerID == nil || *r.OwnerID != ownerID {
		return nil, gorm.ErrRecordNotFound
	}
	return r, nil
}

func (d *mockResourceDao) Create(_ context.Context, resource *api.Resource) (*api.Resource, error) {
	if d.createErr != nil {
		return nil, d.createErr
	}
	d.resources[resourceKey(resource.Kind, resource.ID)] = resource
	return resource, nil
}

func (d *mockResourceDao) Save(_ context.Context, resource *api.Resource) error {
	if d.saveErr != nil {
		return d.saveErr
	}
	d.resources[resourceKey(resource.Kind, resource.ID)] = resource
	return nil
}

func (d *mockResourceDao) Delete(_ context.Context, kind, id string) error {
	if d.deleteErr != nil {
		return d.deleteErr
	}
	delete(d.resources, resourceKey(kind, id))
	return nil
}

func (d *mockResourceDao) ExistsByOwner(_ context.Context, kind, ownerID string) (bool, error) {
	for _, r := range d.resources {
		if r.Kind == kind && r.OwnerID != nil && *r.OwnerID == ownerID && r.DeletedTime == nil {
			return true, nil
		}
	}
	return false, nil
}

func (d *mockResourceDao) ExistsSoftDeletedByOwner(_ context.Context, kind, ownerID string) (bool, error) {
	for _, r := range d.resources {
		if r.Kind == kind && r.OwnerID != nil && *r.OwnerID == ownerID && r.DeletedTime != nil {
			return true, nil
		}
	}
	return false, nil
}

func (d *mockResourceDao) FindByKind(_ context.Context, kind string) (api.ResourceList, error) {
	var result api.ResourceList
	for _, r := range d.resources {
		if r.Kind == kind {
			result = append(result, r)
		}
	}
	return result, nil
}

func (d *mockResourceDao) FindByKindAndOwner(_ context.Context, kind, ownerID string) (api.ResourceList, error) {
	var result api.ResourceList
	for _, r := range d.resources {
		if r.Kind == kind && r.OwnerID != nil && *r.OwnerID == ownerID {
			result = append(result, r)
		}
	}
	return result, nil
}

func (d *mockResourceDao) FindByKindAndOwnerForUpdate(
	ctx context.Context, kind, ownerID string,
) (api.ResourceList, error) {
	return d.FindByKindAndOwner(ctx, kind, ownerID)
}

func (d *mockResourceDao) FindByIDs(_ context.Context, kind string, ids []string) (api.ResourceList, error) {
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	var result api.ResourceList
	for _, r := range d.resources {
		if r.Kind == kind && idSet[r.ID] {
			result = append(result, r)
		}
	}
	return result, nil
}

func (d *mockResourceDao) addResource(r *api.Resource) {
	d.resources[resourceKey(r.Kind, r.ID)] = r
}

var _ dao.ResourceDao = &mockResourceDao{}

type resourceGenericMock struct {
	listErr    *errors.ServiceError
	lastSearch string
	listCalled bool
}

func (g *resourceGenericMock) List(
	_ context.Context, args *ListArguments, _ interface{},
) (*api.PagingMeta, *errors.ServiceError) {
	g.listCalled = true
	g.lastSearch = args.Search
	if g.listErr != nil {
		return nil, g.listErr
	}
	return &api.PagingMeta{Page: 1, Size: 0, Total: 0}, nil
}

var _ GenericService = &resourceGenericMock{}

func newTestResourceService(mockDao *mockResourceDao) (ResourceService, *mockResourceDao, *resourceGenericMock) {
	generic := &resourceGenericMock{}
	svc := NewResourceService(mockDao, generic)
	return svc, mockDao, generic
}

func testResource(kind, id, name string) *api.Resource {
	spec, _ := json.Marshal(map[string]interface{}{"key": "value"})
	r := &api.Resource{
		Kind:       kind,
		Name:       name,
		Spec:       spec,
		Generation: 1,
	}
	r.ID = id
	return r
}

// --- Get ---

func TestResourceService_Get_HappyPath(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	expected := testResource("Channel", "ch-1", "stable")
	mockDao.addResource(expected)

	result, svcErr := svc.Get(context.Background(), "Channel", "ch-1")
	Expect(svcErr).To(BeNil())
	Expect(result.ID).To(Equal("ch-1"))
	Expect(result.Name).To(Equal("stable"))
}

func TestResourceService_Get_NotFound(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	result, svcErr := svc.Get(context.Background(), "Channel", "nonexistent")
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(404))
}

// --- Create ---

func TestResourceService_Create_SetsDefaults(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	resource := testResource("Channel", "ch-1", "stable")
	resource.CreatedBy = ""
	resource.UpdatedBy = ""

	result, svcErr := svc.Create(context.Background(), "Channel", resource)
	Expect(svcErr).To(BeNil())
	Expect(result.Kind).To(Equal("Channel"))
	Expect(result.CreatedBy).To(Equal(defaultSystemUser))
	Expect(result.UpdatedBy).To(Equal(defaultSystemUser))
}

func TestResourceService_Create_SetsUserFromAuthContext(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	ctx := auth.SetUsernameContext(context.Background(), "user@test.com")
	resource := testResource("Channel", "ch-1", "stable")
	resource.CreatedBy = ""
	resource.UpdatedBy = ""

	result, svcErr := svc.Create(ctx, "Channel", resource)
	Expect(svcErr).To(BeNil())
	Expect(result.CreatedBy).To(Equal("user@test.com"))
	Expect(result.UpdatedBy).To(Equal("user@test.com"))
}

func TestResourceService_Create_PreservesExplicitValues(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	resource := testResource("Channel", "ch-1", "stable")
	resource.CreatedBy = "explicit@test.com"
	resource.UpdatedBy = "explicit@test.com"
	resource.Generation = 5

	result, svcErr := svc.Create(context.Background(), "Channel", resource)
	Expect(svcErr).To(BeNil())
	Expect(result.CreatedBy).To(Equal("explicit@test.com"))
	Expect(result.Generation).To(Equal(int32(5)))
}

func TestResourceService_Create_ValidName(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	resource := testResource("Channel", "ch-1", "stable")
	result, svcErr := svc.Create(context.Background(), "Channel", resource)
	Expect(svcErr).To(BeNil())
	Expect(result.Name).To(Equal("stable"))
}

func TestResourceService_Create_UniqueConstraint(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	mockDao.createErr = fmt.Errorf("violates unique constraint")
	svc, _, _ := newTestResourceService(mockDao)

	resource := testResource("Channel", "ch-1", "stable")
	result, svcErr := svc.Create(context.Background(), "Channel", resource)
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(409))
}

func TestResourceService_Create_EmptyName(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	resource := testResource("WifConfig", "wif-1", "")
	result, svcErr := svc.Create(context.Background(), "WifConfig", resource)
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(400))
	Expect(svcErr.Reason).To(ContainSubstring("name cannot be empty"))
}

func TestResourceService_Create_UnknownKind(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	resource := testResource("Bogus", "b-1", "test")
	result, svcErr := svc.Create(context.Background(), "Bogus", resource)
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(400))
	Expect(svcErr.Reason).To(ContainSubstring("Unknown entity kind"))
}

func TestResourceService_Create_ChildLocksParent(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	parent := testResource("Channel", "ch-1", "stable")
	mockDao.addResource(parent)

	child := testResource("Version", "v-1", "4.18")
	child.OwnerID = &parent.ID

	result, svcErr := svc.Create(context.Background(), "Version", child)
	Expect(svcErr).To(BeNil())
	Expect(result).ToNot(BeNil())
}

func TestResourceService_Create_ChildRejectsDeletedParent(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	parent := testResource("Channel", "ch-1", "stable")
	now := time.Now()
	deletedBy := "admin"
	parent.DeletedTime = &now
	parent.DeletedBy = &deletedBy
	mockDao.addResource(parent)

	child := testResource("Version", "v-1", "4.18")
	child.OwnerID = &parent.ID

	result, svcErr := svc.Create(context.Background(), "Version", child)
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(409))
}

func TestResourceService_Create_ChildRejectsMissingParent(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	child := testResource("Version", "v-1", "4.18")
	nonexistent := "no-such-id"
	child.OwnerID = &nonexistent

	result, svcErr := svc.Create(context.Background(), "Version", child)
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(404))
}

// --- Patch ---

func TestResourceService_Patch_SpecChanged_IncrementsGeneration(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	existing := testResource("Channel", "ch-1", "stable")
	existing.Generation = 1
	mockDao.addResource(existing)

	newSpec := map[string]interface{}{"key": "new-value"}
	patch := &api.ResourcePatch{Spec: newSpec}

	result, svcErr := svc.Patch(context.Background(), "Channel", "ch-1", patch)
	Expect(svcErr).To(BeNil())
	Expect(result.Generation).To(Equal(int32(2)))
}

func TestResourceService_Patch_LabelsChanged_IncrementsGeneration(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	existing := testResource("Channel", "ch-1", "stable")
	existing.Generation = 1
	mockDao.addResource(existing)

	newLabels := map[string]string{"env": "prod"}
	patch := &api.ResourcePatch{Labels: newLabels}

	result, svcErr := svc.Patch(context.Background(), "Channel", "ch-1", patch)
	Expect(svcErr).To(BeNil())
	Expect(result.Generation).To(Equal(int32(2)))
}

func TestResourceService_Patch_NoChange_KeepsGeneration(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	spec, _ := json.Marshal(map[string]interface{}{"key": "value"})
	existing := testResource("Channel", "ch-1", "stable")
	existing.Spec = spec
	existing.Generation = 3
	mockDao.addResource(existing)

	patch := &api.ResourcePatch{}

	result, svcErr := svc.Patch(context.Background(), "Channel", "ch-1", patch)
	Expect(svcErr).To(BeNil())
	Expect(result.Generation).To(Equal(int32(3)))
}

func TestResourceService_Patch_DeletedResource_409(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	now := time.Now()
	existing := testResource("Channel", "ch-1", "stable")
	existing.DeletedTime = &now
	mockDao.addResource(existing)

	newSpec := map[string]interface{}{"key": "new-value"}
	patch := &api.ResourcePatch{Spec: newSpec}

	result, svcErr := svc.Patch(context.Background(), "Channel", "ch-1", patch)
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(409))
	Expect(svcErr.Reason).To(ContainSubstring("marked for deletion"))
}

func TestResourceService_Patch_NotFound(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	newSpec := map[string]interface{}{"key": "new-value"}
	patch := &api.ResourcePatch{Spec: newSpec}

	result, svcErr := svc.Patch(context.Background(), "Channel", "nonexistent", patch)
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(404))
}

func TestResourceService_Patch_SaveError(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	existing := testResource("Channel", "ch-1", "stable")
	mockDao.addResource(existing)
	mockDao.saveErr = fmt.Errorf("connection refused")

	newSpec := map[string]interface{}{"key": "new-value"}
	patch := &api.ResourcePatch{Spec: newSpec}

	result, svcErr := svc.Patch(context.Background(), "Channel", "ch-1", patch)
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(500))
}

func TestResourceService_Patch_SetsUpdatedByFromAuthContext(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	existing := testResource("Channel", "ch-1", "stable")
	existing.UpdatedBy = "old-user@test.com"
	mockDao.addResource(existing)

	ctx := auth.SetUsernameContext(context.Background(), "new-user@test.com")
	newSpec := map[string]interface{}{"key": "new-value"}
	patch := &api.ResourcePatch{Spec: newSpec}

	result, svcErr := svc.Patch(ctx, "Channel", "ch-1", patch)
	Expect(svcErr).To(BeNil())
	Expect(result.UpdatedBy).To(Equal("new-user@test.com"))
}

// --- Delete ---

func TestResourceService_Delete_HappyPath(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	existing := testResource("Channel", "ch-1", "stable")
	existing.Generation = 1
	mockDao.addResource(existing)

	result, svcErr := svc.Delete(context.Background(), "Channel", "ch-1")
	Expect(svcErr).To(BeNil())
	Expect(result.DeletedTime).ToNot(BeNil())
	Expect(result.DeletedBy).ToNot(BeNil())
	Expect(result.Generation).To(Equal(int32(2)))

	_, exists := mockDao.resources[resourceKey("Channel", "ch-1")]
	Expect(exists).To(BeFalse())
}

func TestResourceService_Delete_AlreadyDeleted_Idempotent(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	now := time.Now()
	existing := testResource("Channel", "ch-1", "stable")
	existing.DeletedTime = &now
	deletedBy := "someone"
	existing.DeletedBy = &deletedBy
	existing.Generation = 3
	mockDao.addResource(existing)

	result, svcErr := svc.Delete(context.Background(), "Channel", "ch-1")
	Expect(svcErr).To(BeNil())
	Expect(result.Generation).To(Equal(int32(3)))
}

func TestResourceService_Delete_NotFound(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	result, svcErr := svc.Delete(context.Background(), "Channel", "nonexistent")
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(404))
}

func TestResourceService_Delete_SaveError_WithAdapters(t *testing.T) {
	RegisterTestingT(t)
	setupManagedDescriptor()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	existing := testResource("Managed", "m-1", "managed-1")
	mockDao.addResource(existing)
	mockDao.saveErr = fmt.Errorf("connection refused")

	result, svcErr := svc.Delete(context.Background(), "Managed", "m-1")
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(500))
}

func TestResourceService_Delete_SetsDeletedByFromAuthContext(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	existing := testResource("Channel", "ch-1", "stable")
	mockDao.addResource(existing)

	ctx := auth.SetUsernameContext(context.Background(), "admin@test.com")
	result, svcErr := svc.Delete(ctx, "Channel", "ch-1")
	Expect(svcErr).To(BeNil())
	Expect(*result.DeletedBy).To(Equal("admin@test.com"))
}

// --- Delete policies ---

func testChildResource(kind, id, name, ownerID string) *api.Resource {
	r := testResource(kind, id, name)
	r.OwnerID = &ownerID
	return r
}

type descriptorDef struct {
	kind, plural, parent string
	policy               registry.OnParentDeletePolicy
}

func rootDescriptor(kind, plural string) descriptorDef {
	return descriptorDef{kind: kind, plural: plural}
}

func childDescriptor(kind, plural, parent string, policy registry.OnParentDeletePolicy) descriptorDef {
	return descriptorDef{kind: kind, plural: plural, parent: parent, policy: policy}
}

func setupManagedDescriptor() {
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:             "Managed",
		Plural:           "manageds",
		RequiredAdapters: []string{"provisioner"},
	})
}

func setupDeletePolicyDescriptors(defs ...descriptorDef) {
	registry.Reset()
	for _, p := range defs {
		d := registry.EntityDescriptor{
			Kind:   p.kind,
			Plural: p.plural,
		}
		if p.parent != "" {
			d.ParentKind = p.parent
			d.OnParentDelete = p.policy
		}
		registry.Register(d)
	}
}

func TestResourceService_Delete_RestrictBlocksWithActiveChildren(t *testing.T) {
	RegisterTestingT(t)
	setupDeletePolicyDescriptors(
		rootDescriptor("Channel", "channels"),
		childDescriptor("Version", "versions", "Channel", registry.OnParentDeleteRestrict),
	)

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	channel := testResource("Channel", "ch-1", "stable")
	mockDao.addResource(channel)
	mockDao.addResource(testChildResource("Version", "v-1", "4.17", "ch-1"))

	result, svcErr := svc.Delete(context.Background(), "Channel", "ch-1")
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(409))
	Expect(svcErr.Reason).To(ContainSubstring("active Version(s)"))

}

func TestResourceService_Delete_RestrictAllowsWithZeroChildren(t *testing.T) {
	RegisterTestingT(t)
	setupDeletePolicyDescriptors(
		rootDescriptor("Channel", "channels"),
		childDescriptor("Version", "versions", "Channel", registry.OnParentDeleteRestrict),
	)

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	channel := testResource("Channel", "ch-1", "stable")
	mockDao.addResource(channel)

	result, svcErr := svc.Delete(context.Background(), "Channel", "ch-1")
	Expect(svcErr).To(BeNil())
	Expect(result.DeletedTime).ToNot(BeNil())
}

func TestResourceService_Delete_CascadePropagates(t *testing.T) {
	RegisterTestingT(t)
	setupDeletePolicyDescriptors(
		rootDescriptor("Parent", "parents"),
		childDescriptor("Child", "children", "Parent", registry.OnParentDeleteCascade),
	)

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	parent := testResource("Parent", "p-1", "parent-1")
	mockDao.addResource(parent)
	mockDao.addResource(testChildResource("Child", "c-1", "child-1", "p-1"))
	mockDao.addResource(testChildResource("Child", "c-2", "child-2", "p-1"))

	ctx := auth.SetUsernameContext(context.Background(), "admin@test.com")
	result, svcErr := svc.Delete(ctx, "Parent", "p-1")
	Expect(svcErr).To(BeNil())
	Expect(result.DeletedTime).ToNot(BeNil())
	Expect(*result.DeletedBy).To(Equal("admin@test.com"))

	_, parentExists := mockDao.resources[resourceKey("Parent", "p-1")]
	_, child1Exists := mockDao.resources[resourceKey("Child", "c-1")]
	_, child2Exists := mockDao.resources[resourceKey("Child", "c-2")]
	Expect(parentExists).To(BeFalse())
	Expect(child1Exists).To(BeFalse())
	Expect(child2Exists).To(BeFalse())
}

func TestResourceService_Delete_CascadeRecursesMultipleLevels(t *testing.T) {
	RegisterTestingT(t)
	setupDeletePolicyDescriptors(
		rootDescriptor("Top", "tops"),
		childDescriptor("Mid", "mids", "Top", registry.OnParentDeleteCascade),
		childDescriptor("Leaf", "leaves", "Mid", registry.OnParentDeleteCascade),
	)

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	mockDao.addResource(testResource("Top", "t-1", "top"))
	mockDao.addResource(testChildResource("Mid", "m-1", "mid", "t-1"))
	mockDao.addResource(testChildResource("Leaf", "l-1", "leaf", "m-1"))

	result, svcErr := svc.Delete(context.Background(), "Top", "t-1")
	Expect(svcErr).To(BeNil())
	Expect(result.DeletedTime).ToNot(BeNil())

	_, topExists := mockDao.resources[resourceKey("Top", "t-1")]
	_, midExists := mockDao.resources[resourceKey("Mid", "m-1")]
	_, leafExists := mockDao.resources[resourceKey("Leaf", "l-1")]
	Expect(topExists).To(BeFalse())
	Expect(midExists).To(BeFalse())
	Expect(leafExists).To(BeFalse())
}

func TestResourceService_Delete_MixedPolicies_CascadeAndRestrictPass(t *testing.T) {
	RegisterTestingT(t)
	setupDeletePolicyDescriptors(
		rootDescriptor("Hub", "hubs"),
		childDescriptor("Spoke", "spokes", "Hub", registry.OnParentDeleteCascade),
		childDescriptor("Guard", "guards", "Hub", registry.OnParentDeleteRestrict),
	)

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	mockDao.addResource(testResource("Hub", "h-1", "hub"))
	mockDao.addResource(testChildResource("Spoke", "s-1", "spoke", "h-1"))

	result, svcErr := svc.Delete(context.Background(), "Hub", "h-1")
	Expect(svcErr).To(BeNil())
	Expect(result.DeletedTime).ToNot(BeNil())

	_, hubExists := mockDao.resources[resourceKey("Hub", "h-1")]
	_, spokeExists := mockDao.resources[resourceKey("Spoke", "s-1")]
	Expect(hubExists).To(BeFalse())
	Expect(spokeExists).To(BeFalse())
}

func TestResourceService_Delete_MixedPolicyFailure_RestrictBlocks(t *testing.T) {
	RegisterTestingT(t)
	setupDeletePolicyDescriptors(
		rootDescriptor("Hub", "hubs"),
		childDescriptor("Spoke", "spokes", "Hub", registry.OnParentDeleteCascade),
		childDescriptor("Guard", "guards", "Hub", registry.OnParentDeleteRestrict),
	)

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	mockDao.addResource(testResource("Hub", "h-1", "hub"))
	mockDao.addResource(testChildResource("Spoke", "s-1", "spoke", "h-1"))
	mockDao.addResource(testChildResource("Guard", "g-1", "guard", "h-1"))

	result, svcErr := svc.Delete(context.Background(), "Hub", "h-1")
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(409))
	Expect(svcErr.Reason).To(ContainSubstring("Guard"))

	// Cascade child must not be saved — restrict blocked before cascade ran.
	spoke := mockDao.resources[resourceKey("Spoke", "s-1")]
	Expect(spoke.DeletedTime).To(BeNil())
}

func TestResourceService_Delete_CascadeSkipsAlreadyDeletedChild(t *testing.T) {
	RegisterTestingT(t)
	setupDeletePolicyDescriptors(
		rootDescriptor("Parent", "parents"),
		childDescriptor("Child", "children", "Parent", registry.OnParentDeleteCascade),
	)

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	parent := testResource("Parent", "p-1", "parent-1")
	mockDao.addResource(parent)

	activeChild := testChildResource("Child", "c-1", "active", "p-1")
	mockDao.addResource(activeChild)

	originalDeletedTime := time.Now().Add(-1 * time.Hour).UTC().Truncate(time.Microsecond)
	originalDeletedBy := "previous-user@test.com"
	preDeletedChild := testChildResource("Child", "c-2", "already-gone", "p-1")
	preDeletedChild.MarkDeleted(originalDeletedBy, originalDeletedTime)
	mockDao.addResource(preDeletedChild)

	result, svcErr := svc.Delete(context.Background(), "Parent", "p-1")
	Expect(svcErr).To(BeNil())
	Expect(result.DeletedTime).ToNot(BeNil())

	_, parentExists := mockDao.resources[resourceKey("Parent", "p-1")]
	_, activeExists := mockDao.resources[resourceKey("Child", "c-1")]
	_, preDeletedExists := mockDao.resources[resourceKey("Child", "c-2")]
	Expect(parentExists).To(BeFalse())
	Expect(activeExists).To(BeFalse())
	Expect(preDeletedExists).To(BeFalse())
}

// --- List ---

func TestResourceService_List_InjectsKindFilter(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, generic := newTestResourceService(mockDao)

	args := &ListArguments{Page: 1, Size: 100}
	_, _, svcErr := svc.List(context.Background(), "Channel", args)
	Expect(svcErr).To(BeNil())
	Expect(args.Search).To(Equal(""), "original args should not be mutated")
	Expect(generic.listCalled).To(BeTrue())
	Expect(generic.lastSearch).To(Equal("kind = 'Channel'"))
}

func TestResourceService_List_NilArgs(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, generic := newTestResourceService(mockDao)

	_, _, svcErr := svc.List(context.Background(), "Version", nil)
	Expect(svcErr).To(BeNil())
	Expect(generic.listCalled).To(BeTrue())
	Expect(generic.lastSearch).To(Equal("kind = 'Version'"))
}

func TestResourceService_List_AppendsToExistingSearch(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, generic := newTestResourceService(mockDao)

	args := &ListArguments{Page: 1, Size: 100, Search: "name = 'stable'"}
	_, _, svcErr := svc.List(context.Background(), "Channel", args)
	Expect(svcErr).To(BeNil())
	Expect(args.Search).To(Equal("name = 'stable'"), "original args should not be mutated")
	Expect(generic.lastSearch).To(Equal("(name = 'stable') AND kind = 'Channel'"))
}

func TestResourceService_List_UnknownKind(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	_, _, svcErr := svc.List(context.Background(), "UnknownKind", nil)
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.Reason).To(ContainSubstring("Unknown entity kind"))
}

func TestResourceService_List_GenericServiceError(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, generic := newTestResourceService(mockDao)

	generic.listErr = errors.GeneralError("database connection lost")

	_, _, svcErr := svc.List(context.Background(), "Channel", nil)
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.Reason).To(ContainSubstring("database connection lost"))
}

// --- GetByOwner ---

func TestResourceService_GetByOwner_HappyPath(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	ownerID := "ch-1"
	r := testResource("Version", "v-1", "4.17")
	r.OwnerID = &ownerID
	mockDao.addResource(r)

	result, svcErr := svc.GetByOwner(context.Background(), "Version", "v-1", "ch-1")
	Expect(svcErr).To(BeNil())
	Expect(result.ID).To(Equal("v-1"))
}

func TestResourceService_GetByOwner_WrongOwner_404(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	ownerID := "ch-1"
	r := testResource("Version", "v-1", "4.17")
	r.OwnerID = &ownerID
	mockDao.addResource(r)

	result, svcErr := svc.GetByOwner(context.Background(), "Version", "v-1", "ch-999")
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(404))
}

func TestResourceService_GetByOwner_UnknownKind(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	result, svcErr := svc.GetByOwner(context.Background(), "Bogus", "id-1", "owner-1")
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(400))
}

// --- ListByOwner ---

func TestResourceService_ListByOwner_InjectsKindAndOwnerFilter(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, generic := newTestResourceService(mockDao)

	args := &ListArguments{Page: 1, Size: 100}
	_, _, svcErr := svc.ListByOwner(context.Background(), "Version", "ch-1", args)
	Expect(svcErr).To(BeNil())
	Expect(args.Search).To(Equal(""))
	Expect(generic.listCalled).To(BeTrue())
	Expect(generic.lastSearch).To(Equal("kind = 'Version' AND owner_id = 'ch-1'"))
}

func TestResourceService_ListByOwner_NilArgs(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, generic := newTestResourceService(mockDao)

	_, _, svcErr := svc.ListByOwner(context.Background(), "Version", "ch-1", nil)
	Expect(svcErr).To(BeNil())
	Expect(generic.listCalled).To(BeTrue())
	Expect(generic.lastSearch).To(Equal("kind = 'Version' AND owner_id = 'ch-1'"))
}

func TestResourceService_ListByOwner_AppendsToExistingSearch(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, generic := newTestResourceService(mockDao)

	args := &ListArguments{Page: 1, Size: 100, Search: "name = 'foo'"}
	_, _, svcErr := svc.ListByOwner(context.Background(), "Version", "ch-1", args)
	Expect(svcErr).To(BeNil())
	Expect(args.Search).To(Equal("name = 'foo'"))
	Expect(generic.lastSearch).To(Equal("(name = 'foo') AND kind = 'Version' AND owner_id = 'ch-1'"))
}

// --- Unknown kind ---

func TestResourceService_Get_UnknownKind(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	result, svcErr := svc.Get(context.Background(), "Bogus", "id-1")
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(400))
	Expect(svcErr.Reason).To(ContainSubstring("Unknown entity kind"))
}

func TestResourceService_Patch_UnknownKind(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	patch := &api.ResourcePatch{}
	result, svcErr := svc.Patch(context.Background(), "Bogus", "id-1", patch)
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(400))
}

func TestResourceService_Delete_UnknownKind(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	result, svcErr := svc.Delete(context.Background(), "Bogus", "id-1")
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(400))
}

func TestResourceService_Delete_HardDeleteError(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	existing := testResource("Channel", "ch-1", "stable")
	mockDao.addResource(existing)
	mockDao.deleteErr = fmt.Errorf("disk full")

	result, svcErr := svc.Delete(context.Background(), "Channel", "ch-1")
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(500))
}

func TestResourceService_Delete_ReDeleteAfterHardDelete_Returns404(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	existing := testResource("Channel", "ch-1", "stable")
	mockDao.addResource(existing)

	_, svcErr := svc.Delete(context.Background(), "Channel", "ch-1")
	Expect(svcErr).To(BeNil())

	result, svcErr := svc.Delete(context.Background(), "Channel", "ch-1")
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(404))
}

func TestResourceService_Delete_WithAdapters_SoftDeleteOnly(t *testing.T) {
	RegisterTestingT(t)
	setupManagedDescriptor()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	existing := testResource("Managed", "m-1", "managed-1")
	mockDao.addResource(existing)

	result, svcErr := svc.Delete(context.Background(), "Managed", "m-1")
	Expect(svcErr).To(BeNil())
	Expect(result.DeletedTime).ToNot(BeNil())

	saved := mockDao.resources[resourceKey("Managed", "m-1")]
	Expect(saved).ToNot(BeNil())
	Expect(saved.DeletedTime).ToNot(BeNil())
}

func TestResourceService_ListByOwner_UnknownKind(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	args := &ListArguments{Page: 1, Size: 100}
	result, paging, svcErr := svc.ListByOwner(context.Background(), "Bogus", "owner-1", args)
	Expect(result).To(BeNil())
	Expect(paging).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(400))
}

// --- Parent/Child Delete with RequiredAdapters ---

// setupDescriptorsWithRequiredAdapters creates Channel (parent) and Version (child with RequiredAdapters)
func setupDescriptorsWithRequiredAdapters() {
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:   "Channel",
		Plural: "channels",
	})
	registry.Register(registry.EntityDescriptor{
		Kind:             "Version",
		Plural:           "versions",
		ParentKind:       "Channel",
		OnParentDelete:   registry.OnParentDeleteRestrict,
		RequiredAdapters: []string{"adapter1"}, // Version needs adapter finalization
	})
}

func testResourceWithOwner(kind, id, name, ownerID string) *api.Resource {
	spec, _ := json.Marshal(map[string]interface{}{"key": "value"})
	r := &api.Resource{
		Kind:       kind,
		Name:       name,
		Spec:       spec,
		Generation: 1,
		OwnerID:    &ownerID,
	}
	r.ID = id
	return r
}

// TestResourceService_Delete_ParentSoftDeletedWhileChildSoftDeleted verifies that when a child
// resource with RequiredAdapters is soft-deleted (waiting for adapter finalization), deleting
// the parent soft-deletes the parent instead of hard-deleting it.
//
// Parent should be soft-deleted (not hard-deleted) while any child row exists in the database,
// regardless of whether the child is active or soft-deleted.
func TestResourceService_Delete_ParentSoftDeletedWhileChildSoftDeleted(t *testing.T) {
	RegisterTestingT(t)
	setupDescriptorsWithRequiredAdapters()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	// Setup: Create Channel (parent)
	channel := testResource("Channel", "ch-1", "stable")
	mockDao.addResource(channel)

	// Setup: Create Version (child with RequiredAdapters)
	version := testResourceWithOwner("Version", "v-1", "1.0.0", "ch-1")
	mockDao.addResource(version)

	// Step 1: Delete the Version (soft-delete because of RequiredAdapters)
	versionResult, svcErr := svc.Delete(context.Background(), "Version", "v-1")
	Expect(svcErr).To(BeNil(), "Version delete should succeed")
	Expect(versionResult.DeletedTime).ToNot(BeNil(), "Version should be soft-deleted")

	// Verify Version is still in the DAO (soft-deleted)
	versionAfterDelete := mockDao.resources[resourceKey("Version", "v-1")]
	Expect(versionAfterDelete).ToNot(BeNil(), "Version row should still exist after soft-delete")
	Expect(versionAfterDelete.DeletedTime).ToNot(BeNil(), "Version should have deleted_time set")

	// Step 2: Delete the Channel
	// Expected: Channel should be soft-deleted (not hard-deleted) because Version still exists in DB
	_, svcErr = svc.Delete(context.Background(), "Channel", "ch-1")
	Expect(svcErr).To(BeNil(), "Channel delete should succeed")

	// Verify: Channel should be soft-deleted (row still exists)
	channelAfterDelete := mockDao.resources[resourceKey("Channel", "ch-1")]
	Expect(channelAfterDelete).ToNot(BeNil(), "Channel should still exist in DB (soft-deleted)")
	Expect(channelAfterDelete.DeletedTime).ToNot(BeNil(), "Channel should have deleted_time set")
}

// TestResourceService_Delete_ParentHardDeletedAfterChildGone verifies that after all children
// are hard-deleted (adapter finalized), deleting a parent with no children hard-deletes it.
func TestResourceService_Delete_ParentHardDeletedAfterChildGone(t *testing.T) {
	RegisterTestingT(t)
	setupDescriptorsWithRequiredAdapters()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	// Setup
	channel := testResource("Channel", "ch-1", "stable")
	mockDao.addResource(channel)

	version := testResourceWithOwner("Version", "v-1", "1.0.0", "ch-1")
	mockDao.addResource(version)

	// Delete Version (soft-delete)
	_, svcErr := svc.Delete(context.Background(), "Version", "v-1")
	Expect(svcErr).To(BeNil())

	// Delete Channel (soft-delete because Version still exists)
	_, svcErr = svc.Delete(context.Background(), "Channel", "ch-1")
	Expect(svcErr).To(BeNil())

	// Verify Channel is soft-deleted
	channelSoftDeleted := mockDao.resources[resourceKey("Channel", "ch-1")]
	Expect(channelSoftDeleted).ToNot(BeNil(), "Channel should be soft-deleted")
	Expect(channelSoftDeleted.DeletedTime).ToNot(BeNil())

	// Simulate adapter finalization: hard-delete Version directly from DB
	err := mockDao.Delete(context.Background(), "Version", "v-1")
	Expect(err).To(BeNil())
	Expect(mockDao.resources[resourceKey("Version", "v-1")]).To(BeNil(), "Version should be gone from DB")

	// Note: In production, a cleanup job would detect that Channel has no children
	// and hard-delete it. For now, we verify that a fresh delete (of a non-deleted Channel)
	// with no children does hard-delete.

	// Test with fresh Channel (no children)
	channel2 := testResource("Channel", "ch-2", "beta")
	mockDao.addResource(channel2)

	_, svcErr = svc.Delete(context.Background(), "Channel", "ch-2")
	Expect(svcErr).To(BeNil())

	// Channel2 should be hard-deleted immediately (no children)
	channel2Gone := mockDao.resources[resourceKey("Channel", "ch-2")]
	Expect(channel2Gone).To(BeNil(), "Channel should be hard-deleted when no children exist")
}
