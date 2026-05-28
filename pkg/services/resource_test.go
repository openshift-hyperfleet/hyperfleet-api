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
		Kind:       "Version",
		Plural:     "versions",
		ParentKind: "Channel",
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
	delete(d.resources, resourceKey(kind, id))
	return nil
}

func (d *mockResourceDao) CountByOwner(_ context.Context, kind, ownerID string) (int64, error) {
	var count int64
	for _, r := range d.resources {
		if r.Kind == kind && r.OwnerID != nil && *r.OwnerID == ownerID {
			count++
		}
	}
	return count, nil
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
	patch := &api.ResourcePatchRequest{Spec: &newSpec}

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
	patch := &api.ResourcePatchRequest{Labels: &newLabels}

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

	patch := &api.ResourcePatchRequest{}

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
	patch := &api.ResourcePatchRequest{Spec: &newSpec}

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
	patch := &api.ResourcePatchRequest{Spec: &newSpec}

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
	patch := &api.ResourcePatchRequest{Spec: &newSpec}

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
	patch := &api.ResourcePatchRequest{Spec: &newSpec}

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

func TestResourceService_Delete_SaveError(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	existing := testResource("Channel", "ch-1", "stable")
	mockDao.addResource(existing)
	mockDao.saveErr = fmt.Errorf("connection refused")

	result, svcErr := svc.Delete(context.Background(), "Channel", "ch-1")
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

// --- FindByIDs ---

func TestResourceService_FindByIDs_ReturnsMatching(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	mockDao.addResource(testResource("Channel", "ch-1", "stable"))
	mockDao.addResource(testResource("Channel", "ch-2", "candidate"))
	mockDao.addResource(testResource("Channel", "ch-3", "nightly"))

	result, svcErr := svc.FindByIDs(context.Background(), "Channel", []string{"ch-1", "ch-3"})
	Expect(svcErr).To(BeNil())
	Expect(result).To(HaveLen(2))
}

func TestResourceService_FindByIDs_UnknownKind(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	result, svcErr := svc.FindByIDs(context.Background(), "Bogus", []string{"id-1"})
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(400))
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

	patch := &api.ResourcePatchRequest{}
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
