package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

const (
	testDeletedBy = "someone"
	testChannelID = "ch-1"
	testParentID  = "p-1"
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
	resources                   map[string]*api.Resource
	createErr                   error
	saveErr                     error
	deleteErr                   error
	existsSoftDeletedByOwnerErr error
	replaceRefsErr              error
	findReferencerResult        *api.ResourceSummary
	lastReplacedRefs            []api.ResourceReference
	replaceRefsCalled           bool
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

func (d *mockResourceDao) ExistsSoftDeletedByOwner(_ context.Context, kinds []string, ownerID string) (bool, error) {
	if d.existsSoftDeletedByOwnerErr != nil {
		return false, d.existsSoftDeletedByOwnerErr
	}
	if len(kinds) == 0 {
		return false, nil
	}
	kindSet := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		kindSet[k] = true
	}
	for _, r := range d.resources {
		if kindSet[r.Kind] && r.OwnerID != nil && *r.OwnerID == ownerID && r.DeletedTime != nil {
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

func (d *mockResourceDao) GetByID(_ context.Context, id string) (*api.Resource, error) {
	for _, r := range d.resources {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
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

func (d *mockResourceDao) ReplaceReferences(_ context.Context, _ string, refs []api.ResourceReference) error {
	d.replaceRefsCalled = true
	d.lastReplacedRefs = refs
	if d.replaceRefsErr != nil {
		return d.replaceRefsErr
	}
	return nil
}

func (d *mockResourceDao) FindReferencer(_ context.Context, _ string) (*api.ResourceSummary, error) {
	return d.findReferencerResult, nil
}

func (d *mockResourceDao) ClearTargetReferences(_ context.Context, _ string) error {
	return nil
}

func (d *mockResourceDao) FindSourceIDsByRef(_ context.Context, _, _ string) ([]string, error) {
	return nil, nil
}
func (d *mockResourceDao) addResource(r *api.Resource) {
	d.resources[resourceKey(r.Kind, r.ID)] = r
}

var _ dao.ResourceDao = &mockResourceDao{}

type mockResourceLabelDao struct {
	labels     map[string][]api.ResourceLabel
	replaceErr error
}

func newMockResourceLabelDao() *mockResourceLabelDao {
	return &mockResourceLabelDao{labels: make(map[string][]api.ResourceLabel)}
}

func (d *mockResourceLabelDao) ReplaceLabels(_ context.Context, resourceID string, labels []api.ResourceLabel) error {
	if d.replaceErr != nil {
		return d.replaceErr
	}
	stored := make([]api.ResourceLabel, len(labels))
	copy(stored, labels)
	for i := range stored {
		stored[i].ResourceID = resourceID
	}
	d.labels[resourceID] = stored
	return nil
}

var _ dao.ResourceLabelDao = &mockResourceLabelDao{}

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

// resourceConditionMock implements dao.ResourceConditionDao for testing.
type resourceConditionMock struct {
	conditions map[string][]api.ResourceCondition
}

func newResourceConditionMock() *resourceConditionMock {
	return &resourceConditionMock{conditions: make(map[string][]api.ResourceCondition)}
}

func (d *resourceConditionMock) UpdateConditions(
	_ context.Context, resourceID string, conditions []api.ResourceCondition,
) error {
	d.conditions[resourceID] = conditions
	return nil
}

func (d *resourceConditionMock) DeleteByResource(_ context.Context, resourceID string) error {
	delete(d.conditions, resourceID)
	return nil
}

var _ dao.ResourceConditionDao = &resourceConditionMock{}

func newTestResourceService(mockDao *mockResourceDao) (ResourceService, *mockResourceDao, *resourceGenericMock) {
	generic := &resourceGenericMock{}
	svc := NewResourceService(
		mockDao, newMockResourceLabelDao(), newMockAdapterStatusDao(), newResourceConditionMock(), generic,
	)
	return svc, mockDao, generic
}

func newTestResourceServiceWithLabelDao(
	mockDao *mockResourceDao,
) (ResourceService, *mockResourceDao, *resourceGenericMock, *mockResourceLabelDao) {
	generic := &resourceGenericMock{}
	labelDao := newMockResourceLabelDao()
	svc := NewResourceService(
		mockDao, labelDao, newMockAdapterStatusDao(), newResourceConditionMock(), generic,
	)
	return svc, mockDao, generic, labelDao
}

func newTestResourceServiceWithAdapterStatus(
	mockDao *mockResourceDao,
) (ResourceService, *mockResourceDao, *mockAdapterStatusDao, *resourceConditionMock) {
	asDao := newMockAdapterStatusDao()
	rcDao := newResourceConditionMock()
	generic := &resourceGenericMock{}
	svc := NewResourceService(mockDao, newMockResourceLabelDao(), asDao, rcDao, generic)
	return svc, mockDao, asDao, rcDao
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

	result, svcErr := svc.Create(context.Background(), "Channel", resource, nil)
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

	result, svcErr := svc.Create(ctx, "Channel", resource, nil)
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

	result, svcErr := svc.Create(context.Background(), "Channel", resource, nil)
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
	result, svcErr := svc.Create(context.Background(), "Channel", resource, nil)
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
	result, svcErr := svc.Create(context.Background(), "Channel", resource, nil)
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
	result, svcErr := svc.Create(context.Background(), "WifConfig", resource, nil)
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
	result, svcErr := svc.Create(context.Background(), "Bogus", resource, nil)
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

	result, svcErr := svc.Create(context.Background(), "Version", child, nil)
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

	result, svcErr := svc.Create(context.Background(), "Version", child, nil)
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

	result, svcErr := svc.Create(context.Background(), "Version", child, nil)
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

func TestResourceService_Create_LabelDaoError(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _, labelDao := newTestResourceServiceWithLabelDao(mockDao)

	labelDao.replaceErr = fmt.Errorf("disk full")

	resource := testResource("Channel", "", "label-err")
	resource.Labels = []api.ResourceLabel{{Key: "env", Value: "prod"}}

	_, svcErr := svc.Create(context.Background(), "Channel", resource, nil)
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(500))
}

func TestResourceService_Patch_SpecOnlyChange_SkipsReplaceLabels(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _, labelDao := newTestResourceServiceWithLabelDao(mockDao)

	existing := testResource("Channel", "ch-1", "stable")
	existing.Labels = []api.ResourceLabel{{Key: "env", Value: "prod"}}
	existing.Generation = 1
	mockDao.addResource(existing)

	newSpec := map[string]interface{}{"key": "new-value"}
	patch := &api.ResourcePatch{Spec: newSpec}

	result, svcErr := svc.Patch(context.Background(), "Channel", "ch-1", patch)
	Expect(svcErr).To(BeNil())
	Expect(result.Generation).To(Equal(int32(2)))
	Expect(labelDao.labels).To(BeEmpty(),
		"ReplaceLabels should not be called when only spec changed")
}

func TestResourceService_Patch_OverlongLabelKey_Returns400(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	existing := testResource("Channel", "ch-1", "stable")
	existing.Generation = 1
	mockDao.addResource(existing)

	overlongKey := strings.Repeat("k", api.MaxLabelKeyLen+1)
	patch := &api.ResourcePatch{Labels: map[string]string{overlongKey: "v"}}

	_, svcErr := svc.Patch(context.Background(), "Channel", "ch-1", patch)
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(400))
	Expect(svcErr.Reason).To(ContainSubstring("exceeds maximum length"))
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
	deletedBy := testDeletedBy
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

func setupDescriptorsWithCascadeAndRequiredAdapters() {
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:   "Workspace",
		Plural: "workspaces",
	})
	registry.Register(registry.EntityDescriptor{
		Kind:             "Task",
		Plural:           "tasks",
		ParentKind:       "Workspace",
		OnParentDelete:   registry.OnParentDeleteCascade,
		RequiredAdapters: []string{"adapter1"},
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

	// Re-delete the already soft-deleted Channel - should now hard-delete
	// This exercises the re-evaluation path: parent was soft-deleted, child is now gone,
	// so calling Delete() again should detect no blockers and hard-delete the parent.
	_, svcErr = svc.Delete(context.Background(), "Channel", "ch-1")
	Expect(svcErr).To(BeNil())

	// Channel should now be hard-deleted (removed from DB)
	channelAfterRedelete := mockDao.resources[resourceKey("Channel", "ch-1")]
	Expect(channelAfterRedelete).To(BeNil(), "Channel should be hard-deleted after re-evaluation with no children")
}

func TestResourceService_Delete_DAOErrorCheckingSoftDeletedChildren(t *testing.T) {
	RegisterTestingT(t)
	setupDescriptorsWithRequiredAdapters()

	svc, mockDao, _ := newTestResourceService(newMockResourceDao())

	// Create parent and child
	parent := testResource("Channel", "ch-1", "beta")
	child := testResourceWithOwner("Version", "v-1", "1.0.0", parent.ID)
	mockDao.addResource(parent)
	mockDao.addResource(child)

	// First delete: child is soft-deleted
	_, err := svc.Delete(context.Background(), "Version", "v-1")
	Expect(err).To(BeNil())
	Expect(mockDao.resources[resourceKey("Version", "v-1")].DeletedTime).NotTo(BeNil())

	// Inject DAO error for ExistsSoftDeletedByOwner
	mockDao.existsSoftDeletedByOwnerErr = gorm.ErrInvalidDB

	// Attempt to delete parent - should fail with GeneralError
	_, svcErr := svc.Delete(context.Background(), "Channel", "ch-1")
	Expect(svcErr).NotTo(BeNil())
	Expect(svcErr.RFC9457Code).To(Equal("HYPERFLEET-INT-001"))
	Expect(svcErr.Reason).To(ContainSubstring("Unable to check soft-deleted children"))
}

// TestResourceService_Delete_CascadeParentSoftDeletedWhileChildSoftDeleted validates AC #4:
// "For generic resources using OnParentDeleteCascade, a parent with a soft-deleted child
// that has RequiredAdapters is not hard-deleted while the child row remains."
func TestResourceService_Delete_CascadeParentSoftDeletedWhileChildSoftDeleted(t *testing.T) {
	RegisterTestingT(t)
	setupDescriptorsWithCascadeAndRequiredAdapters()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	// Create parent and child
	workspace := testResource("Workspace", "ws-1", "dev")
	mockDao.addResource(workspace)

	task := testResourceWithOwner("Task", "t-1", "build", "ws-1")
	mockDao.addResource(task)

	// Delete Workspace -> cascade-deletes Task (soft-delete because Task has RequiredAdapters)
	// -> Workspace should be soft-deleted (soft-deleted child exists)
	_, svcErr := svc.Delete(context.Background(), "Workspace", "ws-1")
	Expect(svcErr).To(BeNil())

	// Verify Workspace is soft-deleted (not hard-deleted)
	ws := mockDao.resources[resourceKey("Workspace", "ws-1")]
	Expect(ws).ToNot(BeNil(), "Workspace should still exist (soft-deleted)")
	Expect(ws.DeletedTime).ToNot(BeNil(), "Workspace should have deleted_time set")

	// Verify Task was cascade-deleted and is also soft-deleted
	tk := mockDao.resources[resourceKey("Task", "t-1")]
	Expect(tk).ToNot(BeNil(), "Task should still exist (soft-deleted)")
	Expect(tk.DeletedTime).ToNot(BeNil(), "Task should have deleted_time set")
}

// --- ForceDelete ---

func TestResourceService_ForceDelete_HappyPath_NoChildren(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	now := time.Now()
	existing := testResource("Channel", testChannelID, "stable")
	existing.DeletedTime = &now
	deletedBy := testDeletedBy
	existing.DeletedBy = &deletedBy
	mockDao.addResource(existing)

	svcErr := svc.ForceDelete(context.Background(), "Channel", testChannelID, "Stuck in finalizing")
	Expect(svcErr).To(BeNil())

	_, exists := mockDao.resources[resourceKey("Channel", testChannelID)]
	Expect(exists).To(BeFalse())
}

func TestResourceService_ForceDelete_CascadesAllChildren(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	now := time.Now()
	channel := testResource("Channel", testChannelID, "stable")
	channel.DeletedTime = &now
	deletedBy := testDeletedBy
	channel.DeletedBy = &deletedBy
	mockDao.addResource(channel)

	chID := testChannelID
	v1 := testResource("Version", "v-1", "v1.0")
	v1.OwnerID = &chID
	mockDao.addResource(v1)

	v2 := testResource("Version", "v-2", "v2.0")
	v2.OwnerID = &chID
	mockDao.addResource(v2)

	svcErr := svc.ForceDelete(context.Background(), "Channel", testChannelID, "stuck")
	Expect(svcErr).To(BeNil())

	_, chExists := mockDao.resources[resourceKey("Channel", testChannelID)]
	Expect(chExists).To(BeFalse())
	_, v1Exists := mockDao.resources[resourceKey("Version", "v-1")]
	Expect(v1Exists).To(BeFalse())
	_, v2Exists := mockDao.resources[resourceKey("Version", "v-2")]
	Expect(v2Exists).To(BeFalse())
}

func TestResourceService_ForceDelete_BypassesRestrict(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	channel := testResource("Channel", testChannelID, "stable")
	mockDao.addResource(channel)

	chID := testChannelID
	version := testResource("Version", "v-1", "v1.0")
	version.OwnerID = &chID
	mockDao.addResource(version)

	// Normal delete blocked by Restrict policy (active children)
	_, normalDeleteErr := svc.Delete(context.Background(), "Channel", testChannelID)
	Expect(normalDeleteErr).ToNot(BeNil())
	Expect(normalDeleteErr.HTTPCode).To(Equal(409))

	// Simulate reaching Finalizing state (e.g., via admin override)
	now := time.Now()
	channel.DeletedTime = &now
	deletedBy := "admin"
	channel.DeletedBy = &deletedBy

	// Force-delete bypasses Restrict and cascades everything
	svcErr := svc.ForceDelete(context.Background(), "Channel", testChannelID, "bypass restrict")
	Expect(svcErr).To(BeNil())

	_, chExists := mockDao.resources[resourceKey("Channel", testChannelID)]
	Expect(chExists).To(BeFalse())
	_, vExists := mockDao.resources[resourceKey("Version", "v-1")]
	Expect(vExists).To(BeFalse())
}

func TestResourceService_ForceDelete_NotInFinalizingState(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	existing := testResource("Channel", testChannelID, "stable")
	mockDao.addResource(existing)

	svcErr := svc.ForceDelete(context.Background(), "Channel", testChannelID, "some reason")
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(409))
}

func TestResourceService_ForceDelete_NotFound(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	svcErr := svc.ForceDelete(context.Background(), "Channel", "nonexistent", "some reason")
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(404))
}

func TestResourceService_ForceDelete_RecursiveGrandchildren(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()
	registry.Register(registry.EntityDescriptor{Kind: "Root", Plural: "roots"})
	registry.Register(registry.EntityDescriptor{
		Kind: "Child", Plural: "children", ParentKind: "Root",
		OnParentDelete: registry.OnParentDeleteCascade,
	})
	registry.Register(registry.EntityDescriptor{
		Kind: "Grandchild", Plural: "grandchildren", ParentKind: "Child",
		OnParentDelete: registry.OnParentDeleteRestrict,
	})

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	now := time.Now()
	root := testResource("Root", "r-1", "root")
	root.DeletedTime = &now
	deletedBy := testDeletedBy
	root.DeletedBy = &deletedBy
	mockDao.addResource(root)

	rootID := "r-1"
	child := testResource("Child", "c-1", "child")
	child.OwnerID = &rootID
	mockDao.addResource(child)

	childID := "c-1"
	grandchild := testResource("Grandchild", "gc-1", "grandchild")
	grandchild.OwnerID = &childID
	mockDao.addResource(grandchild)

	svcErr := svc.ForceDelete(context.Background(), "Root", "r-1", "force all")
	Expect(svcErr).To(BeNil())

	Expect(mockDao.resources).To(HaveLen(0))
}

func TestResourceService_ForceDelete_WithRequiredAdapters_Succeeds(t *testing.T) {
	RegisterTestingT(t)
	setupManagedDescriptor()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	now := time.Now()
	existing := testResource("Managed", "m-1", "managed-1")
	existing.DeletedTime = &now
	deletedBy := testDeletedBy
	existing.DeletedBy = &deletedBy
	mockDao.addResource(existing)

	svcErr := svc.ForceDelete(context.Background(), "Managed", "m-1", "some reason")
	Expect(svcErr).To(BeNil())
	Expect(mockDao.resources).To(HaveLen(0))
}

func TestResourceService_ForceDelete_ChildWithRequiredAdapters_Succeeds(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()
	registry.Register(registry.EntityDescriptor{Kind: "Parent", Plural: "parents"})
	registry.Register(registry.EntityDescriptor{
		Kind: "ManagedChild", Plural: "managedchildren", ParentKind: "Parent",
		OnParentDelete:   registry.OnParentDeleteCascade,
		RequiredAdapters: []string{"provisioner"},
	})

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	now := time.Now()
	parent := testResource("Parent", "p-1", "parent")
	parent.DeletedTime = &now
	deletedBy := testDeletedBy
	parent.DeletedBy = &deletedBy
	mockDao.addResource(parent)

	childID := testParentID
	child := testResource("ManagedChild", "mc-1", "managed-child")
	child.OwnerID = &childID
	mockDao.addResource(child)

	svcErr := svc.ForceDelete(context.Background(), "Parent", "p-1", "force it")
	Expect(svcErr).To(BeNil())
	Expect(mockDao.resources).To(HaveLen(0))
}

func TestResourceService_ForceDelete_InvalidKind(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	svcErr := svc.ForceDelete(context.Background(), "Bogus", testChannelID, "some reason")
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(400))
}

// ─── ProcessAdapterStatus tests ──────────────────────────────

func setupAdapterStatusDescriptors() {
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:             "TestResource",
		Plural:           "testresources",
		RequiredAdapters: []string{"adapter1"},
	})
}

func testAdapterStatusRequest(gen int32) *api.AdapterStatus {
	return &api.AdapterStatus{
		Adapter:            "adapter1",
		ObservedGeneration: gen,
		LastReportTime:     time.Now().UTC(),
		Conditions:         testConditionsJSON(testMandatoryConditions(api.AdapterConditionTrue)...),
	}
}

func TestProcessAdapterStatus_HappyPath(t *testing.T) {
	RegisterTestingT(t)
	setupAdapterStatusDescriptors()

	mockDao := newMockResourceDao()
	svc, _, asDao, rcDao := newTestResourceServiceWithAdapterStatus(mockDao)

	r := testResource("TestResource", "r-1", "test")
	r.Generation = 1
	mockDao.addResource(r)

	result, svcErr := svc.ProcessAdapterStatus(
		context.Background(), "TestResource", "r-1", testAdapterStatusRequest(1),
	)

	Expect(svcErr).To(BeNil())
	Expect(result).ToNot(BeNil())
	Expect(result.ResourceType).To(Equal("TestResource"))
	Expect(result.ResourceID).To(Equal("r-1"))

	// Adapter status should be stored
	Expect(asDao.statuses).To(HaveLen(1))

	// Conditions should be written (aggregation triggered by Available=True)
	Expect(rcDao.conditions).To(HaveKey("r-1"))
	Expect(rcDao.conditions["r-1"]).ToNot(BeEmpty())
}

func TestProcessAdapterStatus_UnknownKind_Returns400(t *testing.T) {
	RegisterTestingT(t)
	setupAdapterStatusDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _, _ := newTestResourceServiceWithAdapterStatus(mockDao)

	_, svcErr := svc.ProcessAdapterStatus(
		context.Background(), "NonExistent", "r-1", testAdapterStatusRequest(1),
	)

	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(400))
}

func TestProcessAdapterStatus_ResourceNotFound_Returns404(t *testing.T) {
	RegisterTestingT(t)
	setupAdapterStatusDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _, _ := newTestResourceServiceWithAdapterStatus(mockDao)

	_, svcErr := svc.ProcessAdapterStatus(
		context.Background(), "TestResource", "nonexistent", testAdapterStatusRequest(1),
	)

	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(404))
}

func TestProcessAdapterStatus_DiscardedStatus_ReturnsNil(t *testing.T) {
	RegisterTestingT(t)
	setupAdapterStatusDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _, rcDao := newTestResourceServiceWithAdapterStatus(mockDao)

	r := testResource("TestResource", "r-1", "test")
	r.Generation = 1
	mockDao.addResource(r)

	// Future generation → discarded
	result, svcErr := svc.ProcessAdapterStatus(
		context.Background(), "TestResource", "r-1", testAdapterStatusRequest(99),
	)

	Expect(svcErr).To(BeNil())
	Expect(result).To(BeNil())
	Expect(rcDao.conditions).ToNot(HaveKey("r-1"))
}

func TestProcessAdapterStatus_UpsertSecondReport(t *testing.T) {
	RegisterTestingT(t)
	setupAdapterStatusDescriptors()

	mockDao := newMockResourceDao()
	svc, _, asDao, _ := newTestResourceServiceWithAdapterStatus(mockDao)

	r := testResource("TestResource", "r-1", "test")
	r.Generation = 1
	mockDao.addResource(r)

	// First report
	_, svcErr := svc.ProcessAdapterStatus(
		context.Background(), "TestResource", "r-1", testAdapterStatusRequest(1),
	)
	Expect(svcErr).To(BeNil())
	Expect(asDao.statuses).To(HaveLen(1))

	// Second report from same adapter
	req2 := testAdapterStatusRequest(1)
	req2.LastReportTime = time.Now().UTC().Add(time.Second)
	_, svcErr = svc.ProcessAdapterStatus(
		context.Background(), "TestResource", "r-1", req2,
	)
	Expect(svcErr).To(BeNil())
	Expect(asDao.statuses).To(HaveLen(1))
}

func TestProcessAdapterStatus_SoftDeleted_AllFinalized_HardDeletes(t *testing.T) {
	RegisterTestingT(t)
	setupAdapterStatusDescriptors()

	mockDao := newMockResourceDao()
	svc, _, asDao, rcDao := newTestResourceServiceWithAdapterStatus(mockDao)

	deletedAt := time.Now().UTC()
	r := testResource("TestResource", "r-1", "test")
	r.Generation = 1
	r.DeletedTime = &deletedAt
	mockDao.addResource(r)

	// Report with Finalized=True
	req := &api.AdapterStatus{
		Adapter:            "adapter1",
		ObservedGeneration: 1,
		LastReportTime:     time.Now().UTC(),
		Conditions: testConditionsJSON(
			api.AdapterCondition{Type: api.AdapterConditionTypeAvailable, Status: api.AdapterConditionTrue},
			api.AdapterCondition{Type: api.AdapterConditionTypeApplied, Status: api.AdapterConditionTrue},
			api.AdapterCondition{Type: api.AdapterConditionTypeHealth, Status: api.AdapterConditionTrue},
			api.AdapterCondition{Type: api.AdapterConditionTypeFinalized, Status: api.AdapterConditionTrue},
		),
	}

	result, svcErr := svc.ProcessAdapterStatus(context.Background(), "TestResource", "r-1", req)

	Expect(svcErr).To(BeNil())
	Expect(result).ToNot(BeNil())

	// Resource should be hard-deleted
	_, exists := mockDao.resources[resourceKey("TestResource", "r-1")]
	Expect(exists).To(BeFalse(), "Resource should be hard-deleted")

	// Adapter statuses and conditions should be cleaned up
	Expect(asDao.statuses).To(BeEmpty())
	Expect(rcDao.conditions).ToNot(HaveKey("r-1"))
}

func TestProcessAdapterStatus_SoftDeleted_NotAllFinalized_NoHardDelete(t *testing.T) {
	RegisterTestingT(t)
	setupAdapterStatusDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _, _ := newTestResourceServiceWithAdapterStatus(mockDao)

	deletedAt := time.Now().UTC()
	r := testResource("TestResource", "r-1", "test")
	r.Generation = 1
	r.DeletedTime = &deletedAt
	mockDao.addResource(r)

	// Report with Available=True but NO Finalized
	result, svcErr := svc.ProcessAdapterStatus(
		context.Background(), "TestResource", "r-1", testAdapterStatusRequest(1),
	)

	Expect(svcErr).To(BeNil())
	Expect(result).ToNot(BeNil())

	// Resource should NOT be hard-deleted
	_, exists := mockDao.resources[resourceKey("TestResource", "r-1")]
	Expect(exists).To(BeTrue(), "Resource should still exist")
}

func TestProcessAdapterStatus_EmptyRequiredAdapters_StillAggregates(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:   "NoAdapter",
		Plural: "noadapters",
	})

	mockDao := newMockResourceDao()
	svc, _, _, rcDao := newTestResourceServiceWithAdapterStatus(mockDao)

	r := testResource("NoAdapter", "r-1", "test")
	r.Generation = 1
	mockDao.addResource(r)

	req := &api.AdapterStatus{
		Adapter:            "voluntary-adapter",
		ObservedGeneration: 1,
		LastReportTime:     time.Now().UTC(),
		Conditions:         testConditionsJSON(testMandatoryConditions(api.AdapterConditionTrue)...),
	}

	result, svcErr := svc.ProcessAdapterStatus(context.Background(), "NoAdapter", "r-1", req)

	Expect(svcErr).To(BeNil())
	Expect(result).ToNot(BeNil())

	// Conditions should still be aggregated
	Expect(rcDao.conditions).To(HaveKey("r-1"))
}

func TestProcessAdapterStatus_SoftDeleted_ChildrenExist_NoHardDelete(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:             "Parent",
		Plural:           "parents",
		RequiredAdapters: []string{"adapter1"},
	})
	registry.Register(registry.EntityDescriptor{
		Kind:             "Child",
		Plural:           "children",
		ParentKind:       "Parent",
		OnParentDelete:   registry.OnParentDeleteCascade,
		RequiredAdapters: []string{"child-adapter"},
	})

	mockDao := newMockResourceDao()
	svc, _, _, _ := newTestResourceServiceWithAdapterStatus(mockDao)

	deletedAt := time.Now().UTC()
	parent := testResource("Parent", "p-1", "parent")
	parent.Generation = 1
	parent.DeletedTime = &deletedAt
	mockDao.addResource(parent)

	// Active child prevents hard-delete
	childOwner := testParentID
	child := testResource("Child", "c-1", "child")
	child.OwnerID = &childOwner
	mockDao.addResource(child)

	req := &api.AdapterStatus{
		Adapter:            "adapter1",
		ObservedGeneration: 1,
		LastReportTime:     time.Now().UTC(),
		Conditions: testConditionsJSON(
			api.AdapterCondition{Type: api.AdapterConditionTypeAvailable, Status: api.AdapterConditionTrue},
			api.AdapterCondition{Type: api.AdapterConditionTypeApplied, Status: api.AdapterConditionTrue},
			api.AdapterCondition{Type: api.AdapterConditionTypeHealth, Status: api.AdapterConditionTrue},
			api.AdapterCondition{Type: api.AdapterConditionTypeFinalized, Status: api.AdapterConditionTrue},
		),
	}

	result, svcErr := svc.ProcessAdapterStatus(context.Background(), "Parent", "p-1", req)
	Expect(svcErr).To(BeNil())
	Expect(result).ToNot(BeNil())

	// Parent should NOT be hard-deleted because child exists
	_, exists := mockDao.resources[resourceKey("Parent", "p-1")]
	Expect(exists).To(BeTrue(), "Parent should still exist — active child blocks hard-delete")
}

// --- Resource References ---

func setupRefDescriptors() {
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:   "Target",
		Plural: "targets",
	})
	registry.Register(registry.EntityDescriptor{
		Kind:   "Parent",
		Plural: "parents",
		References: []registry.ReferenceDescriptor{
			{RefType: "dep", TargetKind: "Target", Min: 1, Max: 1},
		},
	})
}

func setupOptionalRefDescriptors() {
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:   "Target",
		Plural: "targets",
	})
	registry.Register(registry.EntityDescriptor{
		Kind:   "Parent",
		Plural: "parents",
		References: []registry.ReferenceDescriptor{
			{RefType: "dep", TargetKind: "Target", Min: 0, Max: 3},
		},
	})
}

func TestResourceService_Create_RequiredRefMissing_Returns400(t *testing.T) {
	RegisterTestingT(t)
	setupRefDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	resource := testResource("Parent", "p-1", "parent-1")
	result, svcErr := svc.Create(context.Background(), "Parent", resource, nil)
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(400))
	Expect(svcErr.Reason).To(ContainSubstring("required reference type"))
	Expect(svcErr.Reason).To(ContainSubstring("dep"))
}

func TestResourceService_Create_RefExceedsMax_Returns400(t *testing.T) {
	RegisterTestingT(t)
	setupRefDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	// Add two targets
	mockDao.addResource(testResource("Target", "t-1", "target-1"))
	mockDao.addResource(testResource("Target", "t-2", "target-2"))

	resource := testResource("Parent", "p-1", "parent-1")
	refs := api.ReferenceMap{
		"dep": {
			{Id: strPtr("t-1"), Kind: "Target"},
			{Id: strPtr("t-2"), Kind: "Target"},
		},
	}

	result, svcErr := svc.Create(context.Background(), "Parent", resource, refs)
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(400))
	Expect(svcErr.Reason).To(ContainSubstring("exceeds max count"))
}

func TestResourceService_Create_WithValidRefs_Persists(t *testing.T) {
	RegisterTestingT(t)
	setupRefDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	target := testResource("Target", "t-1", "target-1")
	mockDao.addResource(target)

	resource := testResource("Parent", "p-1", "parent-1")
	refs := api.ReferenceMap{
		"dep": {
			{Id: strPtr("t-1"), Kind: "Target"},
		},
	}

	result, svcErr := svc.Create(context.Background(), "Parent", resource, refs)
	Expect(svcErr).To(BeNil())
	Expect(result).ToNot(BeNil())
	Expect(mockDao.replaceRefsCalled).To(BeTrue())
	Expect(mockDao.lastReplacedRefs).To(HaveLen(1))
	Expect(mockDao.lastReplacedRefs[0].RefType).To(Equal("dep"))
	Expect(mockDao.lastReplacedRefs[0].TargetID).To(Equal("t-1"))
}

func TestResourceService_Create_RefTargetNotFound_Returns400(t *testing.T) {
	RegisterTestingT(t)
	setupRefDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	resource := testResource("Parent", "p-1", "parent-1")
	refs := api.ReferenceMap{
		"dep": {
			{Id: strPtr("nonexistent"), Kind: "Target"},
		},
	}

	result, svcErr := svc.Create(context.Background(), "Parent", resource, refs)
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(400))
	Expect(svcErr.Reason).To(ContainSubstring("not found"))
}

func TestResourceService_Patch_ReferencesReplaced(t *testing.T) {
	RegisterTestingT(t)
	setupOptionalRefDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	existing := testResource("Parent", "p-1", "parent-1")
	mockDao.addResource(existing)

	target := testResource("Target", "t-1", "target-1")
	mockDao.addResource(target)

	patch := &api.ResourcePatch{
		References: api.ReferenceMap{
			"dep": {
				{Id: strPtr("t-1"), Kind: "Target"},
			},
		},
	}

	result, svcErr := svc.Patch(context.Background(), "Parent", "p-1", patch)
	Expect(svcErr).To(BeNil())
	Expect(result).ToNot(BeNil())
	Expect(mockDao.replaceRefsCalled).To(BeTrue())
	Expect(mockDao.lastReplacedRefs).To(HaveLen(1))
}

func TestResourceService_Patch_NilReferences_NoOp(t *testing.T) {
	RegisterTestingT(t)
	setupOptionalRefDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	existing := testResource("Parent", "p-1", "parent-1")
	mockDao.addResource(existing)

	patch := &api.ResourcePatch{
		References: nil,
	}

	_, svcErr := svc.Patch(context.Background(), "Parent", "p-1", patch)
	Expect(svcErr).To(BeNil())
	Expect(mockDao.replaceRefsCalled).To(BeFalse())
}

func TestResourceService_Patch_EmptyMapClearsAndValidatesMin(t *testing.T) {
	RegisterTestingT(t)
	setupRefDescriptors() // Min: 1 on "dep"

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	existing := testResource("Parent", "p-1", "parent-1")
	mockDao.addResource(existing)

	patch := &api.ResourcePatch{
		References: api.ReferenceMap{},
	}

	result, svcErr := svc.Patch(context.Background(), "Parent", "p-1", patch)
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(400))
	Expect(svcErr.Reason).To(ContainSubstring("required reference type"))
}

func TestResourceService_Delete_ReferencedResource_Returns409(t *testing.T) {
	RegisterTestingT(t)
	setupRefDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	target := testResource("Target", "t-1", "target-1")
	mockDao.addResource(target)
	mockDao.findReferencerResult = &api.ResourceSummary{Kind: "Parent", Name: "parent-1"}

	result, svcErr := svc.Delete(context.Background(), "Target", "t-1")
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(409))
	Expect(svcErr.Reason).To(ContainSubstring("Parent"))
	Expect(svcErr.Reason).To(ContainSubstring("parent-1"))
}

func TestResourceService_Delete_UnreferencedResource_Succeeds(t *testing.T) {
	RegisterTestingT(t)
	setupRefDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	target := testResource("Target", "t-1", "target-1")
	mockDao.addResource(target)
	mockDao.findReferencerResult = nil

	result, svcErr := svc.Delete(context.Background(), "Target", "t-1")
	Expect(svcErr).To(BeNil())
	Expect(result).ToNot(BeNil())
	Expect(result.DeletedTime).ToNot(BeNil())
}

func TestResourceService_Create_DuplicateRefTargetID_Returns400(t *testing.T) {
	RegisterTestingT(t)
	setupOptionalRefDescriptors() // Max: 3 on "dep"

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	target := testResource("Target", "t-1", "target-1")
	mockDao.addResource(target)

	resource := testResource("Parent", "p-1", "parent-1")
	refs := api.ReferenceMap{
		"dep": {
			{Id: strPtr("t-1"), Kind: "Target"},
			{Id: strPtr("t-1"), Kind: "Target"}, // duplicate
		},
	}

	result, svcErr := svc.Create(context.Background(), "Parent", resource, refs)
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(400))
	Expect(svcErr.Reason).To(ContainSubstring("duplicate target id"))
}

func TestResourceService_Create_SoftDeletedTarget_Returns400(t *testing.T) {
	RegisterTestingT(t)
	setupOptionalRefDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	deletedAt := time.Now()
	target := testResource("Target", "t-1", "target-1")
	target.DeletedTime = &deletedAt
	mockDao.addResource(target)

	resource := testResource("Parent", "p-1", "parent-1")
	refs := api.ReferenceMap{
		"dep": {
			{Id: strPtr("t-1"), Kind: "Target"},
		},
	}

	result, svcErr := svc.Create(context.Background(), "Parent", resource, refs)
	Expect(result).To(BeNil())
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(400))
	Expect(svcErr.Reason).To(ContainSubstring("marked for deletion"))
}

// ─── Missing coverage: state machine, timestamps, children reason ────────────

// findCondition returns the ResourceCondition with the given type from the mock, or nil.
func findCondition(conditions []api.ResourceCondition, condType string) *api.ResourceCondition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}

// TestResourceService_AvailableReconciledTransitions exercises the full state machine
// across multiple adapter reports, generation bumps, and adapter failures.
func TestResourceService_AvailableReconciledTransitions(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:             "StateMachine",
		Plural:           "statemachines",
		RequiredAdapters: []string{"adapter-a", "adapter-b"},
	})

	mockDao := newMockResourceDao()
	svc, _, asDao, rcDao := newTestResourceServiceWithAdapterStatus(mockDao)

	r := testResource("StateMachine", "sm-1", "sm-test")
	r.Generation = 1
	mockDao.addResource(r)

	makeReq := func(adapter string, gen int32, avail api.AdapterConditionStatus) *api.AdapterStatus {
		return &api.AdapterStatus{
			Adapter:            adapter,
			ObservedGeneration: gen,
			LastReportTime:     time.Now().UTC(),
			Conditions: testConditionsJSON(
				api.AdapterCondition{Type: api.AdapterConditionTypeAvailable, Status: avail},
				api.AdapterCondition{Type: api.AdapterConditionTypeApplied, Status: api.AdapterConditionTrue},
				api.AdapterCondition{Type: api.AdapterConditionTypeHealth, Status: api.AdapterConditionTrue},
			),
		}
	}

	// Step 1: adapter-a reports Available=True at gen 1. Only 1 of 2 adapters reported.
	ctx := context.Background()
	result, svcErr := svc.ProcessAdapterStatus(
		ctx, "StateMachine", "sm-1", makeReq("adapter-a", 1, api.AdapterConditionTrue),
	)
	Expect(svcErr).To(BeNil())
	Expect(result).ToNot(BeNil())
	Expect(asDao.statuses).To(HaveLen(1))

	// Conditions written -- Reconciled should be False (not all adapters reported)
	conds := rcDao.conditions["sm-1"]
	Expect(conds).ToNot(BeEmpty())
	recon := findCondition(conds, api.ResourceConditionTypeReconciled)
	Expect(recon).ToNot(BeNil())
	Expect(recon.Status).To(Equal(api.ConditionFalse), "Reconciled should be False with only 1/2 adapters reporting")

	// Step 2: adapter-b reports Available=True at gen 1. Both adapters reported.
	// Feed previous conditions back into the resource for aggregation diff.
	r.Conditions = rcDao.conditions["sm-1"]
	_, svcErr = svc.ProcessAdapterStatus(
		ctx, "StateMachine", "sm-1", makeReq("adapter-b", 1, api.AdapterConditionTrue),
	)
	Expect(svcErr).To(BeNil())
	Expect(asDao.statuses).To(HaveLen(2))

	conds = rcDao.conditions["sm-1"]
	recon = findCondition(conds, api.ResourceConditionTypeReconciled)
	Expect(recon).ToNot(BeNil())
	Expect(recon.Status).To(Equal(api.ConditionTrue),
		"Reconciled should be True with all adapters reporting Available=True")

	// Step 3: Bump generation. Report adapter-a at gen 2 — adapter-b is stale.
	r.Generation = 2
	r.Conditions = rcDao.conditions["sm-1"]
	mockDao.addResource(r)

	_, svcErr = svc.ProcessAdapterStatus(
		ctx, "StateMachine", "sm-1", makeReq("adapter-a", 2, api.AdapterConditionTrue),
	)
	Expect(svcErr).To(BeNil())

	conds = rcDao.conditions["sm-1"]
	recon = findCondition(conds, api.ResourceConditionTypeReconciled)
	Expect(recon).ToNot(BeNil())
	Expect(recon.Status).To(Equal(api.ConditionFalse), "Reconciled should regress to False — adapter-b stale at gen 1")

	// Step 4: adapter-b catches up to gen 2.
	r.Conditions = rcDao.conditions["sm-1"]
	_, svcErr = svc.ProcessAdapterStatus(
		ctx, "StateMachine", "sm-1", makeReq("adapter-b", 2, api.AdapterConditionTrue),
	)
	Expect(svcErr).To(BeNil())

	conds = rcDao.conditions["sm-1"]
	recon = findCondition(conds, api.ResourceConditionTypeReconciled)
	Expect(recon).ToNot(BeNil())
	Expect(recon.Status).To(Equal(api.ConditionTrue), "Reconciled should be True again — both at gen 2")

	// Step 5: adapter-a reports Available=False.
	r.Conditions = rcDao.conditions["sm-1"]
	_, svcErr = svc.ProcessAdapterStatus(
		ctx, "StateMachine", "sm-1", makeReq("adapter-a", 2, api.AdapterConditionFalse),
	)
	Expect(svcErr).To(BeNil())

	conds = rcDao.conditions["sm-1"]
	recon = findCondition(conds, api.ResourceConditionTypeReconciled)
	Expect(recon).ToNot(BeNil())
	Expect(recon.Status).To(Equal(api.ConditionFalse), "Reconciled should be False — adapter-a reports Available=False")
}

// TestResourceService_SyntheticTimestampsStable verifies that re-aggregating without
// new adapter changes doesn't drift condition timestamps.
func TestResourceService_SyntheticTimestampsStable(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:             "Stable",
		Plural:           "stables",
		RequiredAdapters: []string{"adapter1"},
	})

	mockDao := newMockResourceDao()
	svc, _, _, rcDao := newTestResourceServiceWithAdapterStatus(mockDao)

	r := testResource("Stable", "s-1", "stable-test")
	r.Generation = 1
	mockDao.addResource(r)

	req := &api.AdapterStatus{
		Adapter:            "adapter1",
		ObservedGeneration: 1,
		LastReportTime:     time.Now().UTC(),
		Conditions:         testConditionsJSON(testMandatoryConditions(api.AdapterConditionTrue)...),
	}

	// First report
	_, svcErr := svc.ProcessAdapterStatus(context.Background(), "Stable", "s-1", req)
	Expect(svcErr).To(BeNil())

	conds1 := make([]api.ResourceCondition, len(rcDao.conditions["s-1"]))
	copy(conds1, rcDao.conditions["s-1"])
	Expect(conds1).ToNot(BeEmpty())

	recon1 := findCondition(conds1, api.ResourceConditionTypeReconciled)
	Expect(recon1).ToNot(BeNil())
	ts1Created := recon1.CreatedTime
	ts1Transition := recon1.LastTransitionTime

	// Feed conditions back and re-process identical report.
	// Use a slightly later LastReportTime but same content.
	r.Conditions = rcDao.conditions["s-1"]
	req2 := &api.AdapterStatus{
		Adapter:            "adapter1",
		ObservedGeneration: 1,
		LastReportTime:     time.Now().UTC().Add(5 * time.Second),
		Conditions:         testConditionsJSON(testMandatoryConditions(api.AdapterConditionTrue)...),
	}

	_, svcErr = svc.ProcessAdapterStatus(context.Background(), "Stable", "s-1", req2)
	// Discarded as duplicate (same adapter, same gen, same Available status)
	Expect(svcErr).To(BeNil())

	// If conditions were rewritten, check timestamps didn't drift.
	// If conditions were NOT rewritten (jsonEqual optimization), that also proves stability.
	conds2 := rcDao.conditions["s-1"]
	recon2 := findCondition(conds2, api.ResourceConditionTypeReconciled)
	Expect(recon2).ToNot(BeNil())
	Expect(recon2.CreatedTime).To(Equal(ts1Created), "CreatedTime should not drift on re-aggregation")
	Expect(recon2.LastTransitionTime).To(Equal(ts1Transition),
		"LastTransitionTime should not drift without a status change")
}

// TestResourceService_ReconciledDuringDeletion_ChildrenReason verifies that when
// a parent is soft-deleted and all adapters finalize, but children still exist,
// the Reconciled condition carries a reason indicating waiting for children.
func TestResourceService_ReconciledDuringDeletion_ChildrenReason(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:             "Parent",
		Plural:           "parents",
		RequiredAdapters: []string{"adapter1"},
	})
	registry.Register(registry.EntityDescriptor{
		Kind:             "Child",
		Plural:           "children",
		ParentKind:       "Parent",
		OnParentDelete:   registry.OnParentDeleteCascade,
		RequiredAdapters: []string{"child-adapter"},
	})

	mockDao := newMockResourceDao()
	svc, _, _, rcDao := newTestResourceServiceWithAdapterStatus(mockDao)

	deletedAt := time.Now().UTC()
	parent := testResource("Parent", testParentID, "parent")
	parent.Generation = 1
	parent.DeletedTime = &deletedAt
	mockDao.addResource(parent)

	// Active child blocks hard-delete
	childOwner := testParentID
	child := testResource("Child", "c-1", "child")
	child.OwnerID = &childOwner
	mockDao.addResource(child)

	req := &api.AdapterStatus{
		Adapter:            "adapter1",
		ObservedGeneration: 1,
		LastReportTime:     time.Now().UTC(),
		Conditions: testConditionsJSON(
			api.AdapterCondition{Type: api.AdapterConditionTypeAvailable, Status: api.AdapterConditionTrue},
			api.AdapterCondition{Type: api.AdapterConditionTypeApplied, Status: api.AdapterConditionTrue},
			api.AdapterCondition{Type: api.AdapterConditionTypeHealth, Status: api.AdapterConditionTrue},
			api.AdapterCondition{Type: api.AdapterConditionTypeFinalized, Status: api.AdapterConditionTrue},
		),
	}

	result, svcErr := svc.ProcessAdapterStatus(context.Background(), "Parent", testParentID, req)
	Expect(svcErr).To(BeNil())
	Expect(result).ToNot(BeNil())

	// Parent should NOT be hard-deleted
	_, exists := mockDao.resources[resourceKey("Parent", testParentID)]
	Expect(exists).To(BeTrue(), "Parent should still exist — active child blocks hard-delete")

	// Reconciled condition should indicate children are blocking
	conds := rcDao.conditions[testParentID]
	Expect(conds).ToNot(BeEmpty(), "Conditions should be written for soft-deleted resource")
	recon := findCondition(conds, api.ResourceConditionTypeReconciled)
	Expect(recon).ToNot(BeNil())
	Expect(recon.Status).To(Equal(api.ConditionFalse), "Reconciled should be False during deletion with active children")
	Expect(recon.Reason).ToNot(BeNil(), "Reconciled reason should be set when children block deletion")
	Expect(*recon.Reason).To(ContainSubstring("Children"), "Reconciled reason should mention children")
}

// ─── Soft-delete lifecycle tests (migrated from cluster/nodepool tests) ────────

func TestResourceService_Delete_SoftDelete_SetsMetadata(t *testing.T) {
	RegisterTestingT(t)
	setupManagedDescriptor()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	existing := testResource("Managed", "m-1", "managed-1")
	existing.Generation = 1
	mockDao.addResource(existing)

	ctx := auth.SetUsernameContext(context.Background(), "admin@test.com")
	result, svcErr := svc.Delete(ctx, "Managed", "m-1")
	Expect(svcErr).To(BeNil())
	Expect(result.DeletedTime).ToNot(BeNil(), "DeletedTime should be set")
	Expect(result.DeletedBy).ToNot(BeNil(), "DeletedBy should be set")
	Expect(*result.DeletedBy).To(Equal("admin@test.com"), "DeletedBy should match auth context")
	Expect(result.Generation).To(Equal(int32(2)), "Generation should be incremented on soft-delete")

	saved := mockDao.resources[resourceKey("Managed", "m-1")]
	Expect(saved).ToNot(BeNil(), "Resource should still exist (soft-deleted)")
	Expect(saved.DeletedTime).ToNot(BeNil())
	Expect(*saved.DeletedBy).To(Equal("admin@test.com"))
	Expect(saved.Generation).To(Equal(int32(2)))
}

func TestResourceService_Delete_SoftDelete_Idempotent(t *testing.T) {
	RegisterTestingT(t)
	setupManagedDescriptor()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	originalTime := time.Now().Add(-time.Hour)
	originalBy := "original@test.com"
	existing := testResource("Managed", "m-1", "managed-1")
	existing.DeletedTime = &originalTime
	existing.DeletedBy = &originalBy
	existing.Generation = 3
	mockDao.addResource(existing)

	result, svcErr := svc.Delete(context.Background(), "Managed", "m-1")
	Expect(svcErr).To(BeNil())
	Expect(result.DeletedTime.Equal(originalTime)).To(BeTrue(), "DeletedTime should be unchanged on re-delete")
	Expect(*result.DeletedBy).To(Equal("original@test.com"), "DeletedBy should be unchanged on re-delete")
	Expect(result.Generation).To(Equal(int32(3)), "Generation should be unchanged on re-delete")
}

// ─── Cascade sets metadata on children ────────────────────────────────────────

func TestResourceService_Delete_CascadeSetsDeletedByOnChildren(t *testing.T) {
	RegisterTestingT(t)
	setupDeletePolicyDescriptors(
		rootDescriptor("Cluster", "clusters"),
		childDescriptor("NodePool", "nodepools", "Cluster", registry.OnParentDeleteCascade),
	)

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	parent := testResource("Cluster", "c-1", "my-cluster")
	mockDao.addResource(parent)
	mockDao.addResource(testChildResource("NodePool", "np-1", "pool-1", "c-1"))
	mockDao.addResource(testChildResource("NodePool", "np-2", "pool-2", "c-1"))

	ctx := auth.SetUsernameContext(context.Background(), "admin@test.com")
	result, svcErr := svc.Delete(ctx, "Cluster", "c-1")
	Expect(svcErr).To(BeNil())
	Expect(result.DeletedTime).ToNot(BeNil())
	Expect(*result.DeletedBy).To(Equal("admin@test.com"))

	// Both parent and children should be gone (hard-deleted, no RequiredAdapters)
	_, parentExists := mockDao.resources[resourceKey("Cluster", "c-1")]
	_, child1Exists := mockDao.resources[resourceKey("NodePool", "np-1")]
	_, child2Exists := mockDao.resources[resourceKey("NodePool", "np-2")]
	Expect(parentExists).To(BeFalse())
	Expect(child1Exists).To(BeFalse())
	Expect(child2Exists).To(BeFalse())
}

func TestResourceService_Delete_CascadeSoftDeleteSetsMetadataOnChildren(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:             "Cluster",
		Plural:           "clusters",
		RequiredAdapters: []string{"provisioner"},
	})
	registry.Register(registry.EntityDescriptor{
		Kind:             "NodePool",
		Plural:           "nodepools",
		ParentKind:       "Cluster",
		OnParentDelete:   registry.OnParentDeleteCascade,
		RequiredAdapters: []string{"np-provisioner"},
	})

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	parent := testResource("Cluster", "c-1", "my-cluster")
	parent.Generation = 1
	mockDao.addResource(parent)

	child := testChildResource("NodePool", "np-1", "pool-1", "c-1")
	child.Generation = 1
	mockDao.addResource(child)

	ctx := auth.SetUsernameContext(context.Background(), "ops@test.com")
	_, svcErr := svc.Delete(ctx, "Cluster", "c-1")
	Expect(svcErr).To(BeNil())

	// Parent should be soft-deleted with metadata
	savedParent := mockDao.resources[resourceKey("Cluster", "c-1")]
	Expect(savedParent).ToNot(BeNil(), "Parent should be soft-deleted, not hard-deleted")
	Expect(savedParent.DeletedTime).ToNot(BeNil())
	Expect(*savedParent.DeletedBy).To(Equal("ops@test.com"))
	Expect(savedParent.Generation).To(Equal(int32(2)))

	// Child should be cascade-soft-deleted with same deleted_by
	savedChild := mockDao.resources[resourceKey("NodePool", "np-1")]
	Expect(savedChild).ToNot(BeNil(), "Child should be soft-deleted, not hard-deleted")
	Expect(savedChild.DeletedTime).ToNot(BeNil())
	Expect(*savedChild.DeletedBy).To(Equal("ops@test.com"), "Child deleted_by should match caller")
	Expect(savedChild.Generation).To(Equal(int32(2)), "Child generation should be incremented")
}

// ─── Soft-delete + Reconciled condition flip ──────────────────────────────────

func TestResourceService_Delete_SoftDelete_ReconciledFlipsToFalse(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:             "Cluster",
		Plural:           "clusters",
		RequiredAdapters: []string{"adapter1"},
	})

	mockDao := newMockResourceDao()
	svc, _, asDao, rcDao := newTestResourceServiceWithAdapterStatus(mockDao)

	r := testResource("Cluster", "c-1", "my-cluster")
	r.Generation = 1
	mockDao.addResource(r)

	// Process adapter status to get Reconciled=True
	req := &api.AdapterStatus{
		Adapter:            "adapter1",
		ObservedGeneration: 1,
		LastReportTime:     time.Now().UTC(),
		Conditions:         testConditionsJSON(testMandatoryConditions(api.AdapterConditionTrue)...),
	}
	_, svcErr := svc.ProcessAdapterStatus(context.Background(), "Cluster", "c-1", req)
	Expect(svcErr).To(BeNil())
	Expect(asDao.statuses).To(HaveLen(1))

	// Pre-condition: Reconciled=True
	conds := rcDao.conditions["c-1"]
	recon := findCondition(conds, api.ResourceConditionTypeReconciled)
	Expect(recon).ToNot(BeNil())
	Expect(recon.Status).To(Equal(api.ConditionTrue), "Pre-condition: Reconciled should be True")

	// Soft-delete bumps generation → Reconciled should flip to False
	r.Conditions = rcDao.conditions["c-1"]
	_, svcErr = svc.Delete(context.Background(), "Cluster", "c-1")
	Expect(svcErr).To(BeNil())

	saved := mockDao.resources[resourceKey("Cluster", "c-1")]
	Expect(saved).ToNot(BeNil(), "Resource should be soft-deleted, not hard-deleted")
	Expect(saved.Generation).To(Equal(int32(2)), "Generation should be bumped")

	// Re-process adapter status at gen 1 (stale) to trigger re-aggregation
	req2 := &api.AdapterStatus{
		Adapter:            "adapter1",
		ObservedGeneration: 1,
		LastReportTime:     time.Now().UTC().Add(time.Second),
		Conditions:         testConditionsJSON(testMandatoryConditions(api.AdapterConditionTrue)...),
	}
	_, svcErr = svc.ProcessAdapterStatus(context.Background(), "Cluster", "c-1", req2)
	Expect(svcErr).To(BeNil())

	// After gen bump, Reconciled should be False (adapter reports gen 1, resource at gen 2)
	conds = rcDao.conditions["c-1"]
	recon = findCondition(conds, api.ResourceConditionTypeReconciled)
	Expect(recon).ToNot(BeNil())
	Expect(recon.Status).To(Equal(api.ConditionFalse),
		"Reconciled should flip to False after soft-delete bumps generation")
}

// ─── Stale adapter status update policy ───────────────────────────────────────

func TestResourceService_StaleAdapterStatusUpdatePolicy(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:             "Cluster",
		Plural:           "clusters",
		RequiredAdapters: []string{"adapter-a", "adapter-b"},
	})

	mockDao := newMockResourceDao()
	svc, _, asDao, rcDao := newTestResourceServiceWithAdapterStatus(mockDao)

	r := testResource("Cluster", "c-1", "my-cluster")
	r.Generation = 2
	mockDao.addResource(r)

	makeReq := func(adapter string, gen int32, avail api.AdapterConditionStatus) *api.AdapterStatus {
		return &api.AdapterStatus{
			Adapter:            adapter,
			ObservedGeneration: gen,
			LastReportTime:     time.Now().UTC(),
			Conditions:         testConditionsJSON(testMandatoryConditions(avail)...),
		}
	}

	ctx := context.Background()

	// Both adapters report Available=True at gen 2.
	_, svcErr := svc.ProcessAdapterStatus(ctx, "Cluster", "c-1", makeReq("adapter-a", 2, api.AdapterConditionTrue))
	Expect(svcErr).To(BeNil())
	r.Conditions = rcDao.conditions["c-1"]

	_, svcErr = svc.ProcessAdapterStatus(ctx, "Cluster", "c-1", makeReq("adapter-b", 2, api.AdapterConditionTrue))
	Expect(svcErr).To(BeNil())
	Expect(asDao.statuses).To(HaveLen(2))

	conds := rcDao.conditions["c-1"]
	avail := findCondition(conds, api.ResourceConditionTypeLastKnownReconciled)
	Expect(avail).ToNot(BeNil())
	Expect(avail.Status).To(Equal(api.ConditionTrue), "Available should be True with both at gen 2")

	// Stale True at gen 1 should not override newer True at gen 2 (discarded by validation).
	r.Conditions = rcDao.conditions["c-1"]
	result, svcErr := svc.ProcessAdapterStatus(ctx, "Cluster", "c-1", makeReq("adapter-a", 1, api.AdapterConditionTrue))
	Expect(svcErr).To(BeNil())
	Expect(result).To(BeNil(), "Stale gen 1 report should be discarded")

	conds = rcDao.conditions["c-1"]
	avail = findCondition(conds, api.ResourceConditionTypeLastKnownReconciled)
	Expect(avail).ToNot(BeNil())
	Expect(avail.Status).To(Equal(api.ConditionTrue), "Available should remain True after stale report")
}

// ─── ProcessAdapterStatus with custom conditions ──────────────────────────────

func TestProcessAdapterStatus_AllMandatoryWithCustom_Accepted(t *testing.T) {
	RegisterTestingT(t)
	setupAdapterStatusDescriptors()

	mockDao := newMockResourceDao()
	svc, _, asDao, rcDao := newTestResourceServiceWithAdapterStatus(mockDao)

	r := testResource("TestResource", "r-1", "test")
	r.Generation = 1
	mockDao.addResource(r)

	// All mandatory conditions + a custom condition
	req := &api.AdapterStatus{
		Adapter:            "adapter1",
		ObservedGeneration: 1,
		LastReportTime:     time.Now().UTC(),
		Conditions: testConditionsJSON(
			api.AdapterCondition{Type: api.AdapterConditionTypeAvailable, Status: api.AdapterConditionTrue},
			api.AdapterCondition{Type: api.AdapterConditionTypeApplied, Status: api.AdapterConditionTrue},
			api.AdapterCondition{Type: api.AdapterConditionTypeHealth, Status: api.AdapterConditionTrue},
			api.AdapterCondition{Type: "CustomCondition", Status: api.AdapterConditionFalse},
		),
	}

	result, svcErr := svc.ProcessAdapterStatus(context.Background(), "TestResource", "r-1", req)

	Expect(svcErr).To(BeNil())
	Expect(result).ToNot(BeNil(), "Mandatory + custom conditions should be accepted")
	Expect(asDao.statuses).To(HaveLen(1))
	Expect(rcDao.conditions).To(HaveKey("r-1"), "Conditions should be aggregated")
}

func TestProcessAdapterStatus_CustomConditionRemoval(t *testing.T) {
	RegisterTestingT(t)
	setupAdapterStatusDescriptors()

	mockDao := newMockResourceDao()
	svc, _, asDao, rcDao := newTestResourceServiceWithAdapterStatus(mockDao)

	r := testResource("TestResource", "r-1", "test")
	r.Generation = 1
	mockDao.addResource(r)

	// First report with custom condition
	req1 := &api.AdapterStatus{
		Adapter:            "adapter1",
		ObservedGeneration: 1,
		LastReportTime:     time.Now().UTC(),
		Conditions: testConditionsJSON(
			api.AdapterCondition{Type: api.AdapterConditionTypeAvailable, Status: api.AdapterConditionTrue},
			api.AdapterCondition{Type: api.AdapterConditionTypeApplied, Status: api.AdapterConditionTrue},
			api.AdapterCondition{Type: api.AdapterConditionTypeHealth, Status: api.AdapterConditionTrue},
			api.AdapterCondition{Type: "CustomCondition", Status: api.AdapterConditionFalse},
		),
	}
	result, svcErr := svc.ProcessAdapterStatus(context.Background(), "TestResource", "r-1", req1)
	Expect(svcErr).To(BeNil())
	Expect(result).ToNot(BeNil())

	// Verify custom condition is stored in the adapter status
	Expect(asDao.statuses).To(HaveLen(1))
	for _, s := range asDao.statuses {
		var conds []api.AdapterCondition
		Expect(json.Unmarshal(s.Conditions, &conds)).To(Succeed())
		Expect(conds).To(HaveLen(4), "Should have 3 mandatory + 1 custom")
	}

	// Second report without custom condition
	req2 := &api.AdapterStatus{
		Adapter:            "adapter1",
		ObservedGeneration: 1,
		LastReportTime:     time.Now().UTC().Add(time.Second),
		Conditions:         testConditionsJSON(testMandatoryConditions(api.AdapterConditionTrue)...),
	}
	result, svcErr = svc.ProcessAdapterStatus(context.Background(), "TestResource", "r-1", req2)
	Expect(svcErr).To(BeNil())
	Expect(result).ToNot(BeNil())

	// Verify custom condition is removed from adapter status
	Expect(asDao.statuses).To(HaveLen(1))
	for _, s := range asDao.statuses {
		var conds []api.AdapterCondition
		Expect(json.Unmarshal(s.Conditions, &conds)).To(Succeed())
		Expect(conds).To(HaveLen(3), "Should have only 3 mandatory conditions after removal")
	}

	// Verify custom condition is also removed from aggregated resource conditions
	Expect(findCondition(rcDao.conditions["r-1"], "CustomCondition")).To(BeNil(),
		"Removed custom condition must not remain in aggregated resource conditions")
}

// ─── Patch condition recompute tests ─────────────────────────────────────────

func TestResourceService_Patch_ReconciledFlipsToFalse(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:             "TestEntity",
		Plural:           "testentities",
		RequiredAdapters: []string{"adapter-a"},
	})

	mockDao := newMockResourceDao()
	svc, _, _, rcDao := newTestResourceServiceWithAdapterStatus(mockDao)

	r := testResource("TestEntity", "te-1", "patch-recon")
	r.Generation = 1
	mockDao.addResource(r)

	ctx := context.Background()

	// Adapter reports Available=True at gen 1 → Reconciled=True
	_, svcErr := svc.ProcessAdapterStatus(ctx, "TestEntity", "te-1", &api.AdapterStatus{
		Adapter:            "adapter-a",
		ObservedGeneration: 1,
		LastReportTime:     time.Now().UTC(),
		Conditions: testConditionsJSON(
			api.AdapterCondition{Type: api.AdapterConditionTypeAvailable, Status: api.AdapterConditionTrue},
			api.AdapterCondition{Type: api.AdapterConditionTypeApplied, Status: api.AdapterConditionTrue},
			api.AdapterCondition{Type: api.AdapterConditionTypeHealth, Status: api.AdapterConditionTrue},
		),
	})
	Expect(svcErr).To(BeNil())

	recon := findCondition(rcDao.conditions["te-1"], api.ResourceConditionTypeReconciled)
	Expect(recon).ToNot(BeNil())
	Expect(recon.Status).To(Equal(api.ConditionTrue))

	// Feed conditions back so Patch sees them for the diff
	r.Conditions = rcDao.conditions["te-1"]
	mockDao.addResource(r)

	// Patch bumps generation → Reconciled must flip to False
	patch := &api.ResourcePatch{Spec: map[string]any{"key": "changed"}}
	result, svcErr := svc.Patch(ctx, "TestEntity", "te-1", patch)
	Expect(svcErr).To(BeNil())
	Expect(result.Generation).To(Equal(int32(2)))

	recon = findCondition(rcDao.conditions["te-1"], api.ResourceConditionTypeReconciled)
	Expect(recon).ToNot(BeNil())
	Expect(recon.Status).To(Equal(api.ConditionFalse),
		"Reconciled must flip to False after Patch bumps generation")
	Expect(recon.ObservedGeneration).To(Equal(int32(2)),
		"ObservedGeneration should reflect the new resource generation")
}

func TestResourceService_Patch_ConsecutivePatchKeepsReconciledFalse(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:             "TestEntity",
		Plural:           "testentities",
		RequiredAdapters: []string{"adapter-a"},
	})

	mockDao := newMockResourceDao()
	svc, _, _, rcDao := newTestResourceServiceWithAdapterStatus(mockDao)

	r := testResource("TestEntity", "te-2", "patch-double")
	r.Generation = 1
	mockDao.addResource(r)

	ctx := context.Background()

	// Adapter reports → Reconciled=True
	_, svcErr := svc.ProcessAdapterStatus(ctx, "TestEntity", "te-2", &api.AdapterStatus{
		Adapter:            "adapter-a",
		ObservedGeneration: 1,
		LastReportTime:     time.Now().UTC(),
		Conditions: testConditionsJSON(
			api.AdapterCondition{Type: api.AdapterConditionTypeAvailable, Status: api.AdapterConditionTrue},
			api.AdapterCondition{Type: api.AdapterConditionTypeApplied, Status: api.AdapterConditionTrue},
			api.AdapterCondition{Type: api.AdapterConditionTypeHealth, Status: api.AdapterConditionTrue},
		),
	})
	Expect(svcErr).To(BeNil())

	r.Conditions = rcDao.conditions["te-2"]
	mockDao.addResource(r)

	// First patch → Reconciled=False
	patch1 := &api.ResourcePatch{Spec: map[string]any{"v": "1"}}
	_, svcErr = svc.Patch(ctx, "TestEntity", "te-2", patch1)
	Expect(svcErr).To(BeNil())

	recon := findCondition(rcDao.conditions["te-2"], api.ResourceConditionTypeReconciled)
	Expect(recon.Status).To(Equal(api.ConditionFalse))
	firstTransition := recon.LastTransitionTime

	// Feed updated conditions and resource back
	r = mockDao.resources[resourceKey("TestEntity", "te-2")]
	r.Conditions = rcDao.conditions["te-2"]
	mockDao.addResource(r)

	// Second patch → Reconciled stays False, LastTransitionTime preserved
	patch2 := &api.ResourcePatch{Spec: map[string]any{"v": "2"}}
	_, svcErr = svc.Patch(ctx, "TestEntity", "te-2", patch2)
	Expect(svcErr).To(BeNil())

	recon = findCondition(rcDao.conditions["te-2"], api.ResourceConditionTypeReconciled)
	Expect(recon.Status).To(Equal(api.ConditionFalse))
	Expect(recon.LastTransitionTime).To(Equal(firstTransition),
		"LastTransitionTime should be preserved on False→False (no transition)")
}

// ─── Patch key-order insensitive tests ────────────────────────────────────────

func TestResourceService_Patch_SpecKeyOrderInsensitive(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	existing := testResource("Channel", "ch-1", "stable")
	existing.Spec = []byte(`{"z":"last","a":"first","m":"middle"}`)
	existing.Generation = 5
	mockDao.addResource(existing)

	// Same keys/values but constructed from a Go map (may serialize in different order)
	sameSpec := map[string]interface{}{"z": "last", "a": "first", "m": "middle"}
	patch := &api.ResourcePatch{Spec: sameSpec}

	result, svcErr := svc.Patch(context.Background(), "Channel", "ch-1", patch)
	Expect(svcErr).To(BeNil())
	Expect(result.Generation).To(Equal(int32(5)), "Generation should not change for same spec with different key order")
}

func TestResourceService_Patch_LabelsKeyOrderInsensitive(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors()

	mockDao := newMockResourceDao()
	svc, _, _ := newTestResourceService(mockDao)

	existing := testResource("Channel", "ch-1", "stable")
	existing.Labels = []api.ResourceLabel{
		{ResourceID: "ch-1", Key: "z", Value: "zulu"},
		{ResourceID: "ch-1", Key: "a", Value: "alpha"},
	}
	existing.Generation = 4
	mockDao.addResource(existing)

	// Same labels in different order
	sameLabels := map[string]string{"a": "alpha", "z": "zulu"}
	patch := &api.ResourcePatch{Labels: sameLabels}

	result, svcErr := svc.Patch(context.Background(), "Channel", "ch-1", patch)
	Expect(svcErr).To(BeNil())
	Expect(result.Generation).To(Equal(int32(4)), "Generation should not change for same labels in different order")
}

func TestResourceService_Create_SeedsConditionsForRequiredAdapters(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:             "ManagedCluster",
		Plural:           "managedclusters",
		RequiredAdapters: []string{"provisioner"},
	})

	mockDao := newMockResourceDao()
	svc, _, _, rcDao := newTestResourceServiceWithAdapterStatus(mockDao)

	resource := testResource("ManagedCluster", "", "my-cluster")
	result, svcErr := svc.Create(context.Background(), "ManagedCluster", resource, nil)
	Expect(svcErr).To(BeNil())

	// Conditions should be seeded with Reconciled=False.
	conditions := rcDao.conditions[result.ID]
	Expect(conditions).ToNot(BeEmpty(), "Conditions should be initialized on Create for entities with RequiredAdapters")

	var reconciledFound bool
	for _, c := range conditions {
		if c.Type == "Reconciled" {
			reconciledFound = true
			Expect(string(c.Status)).To(Equal("False"))
		}
	}
	Expect(reconciledFound).To(BeTrue(), "Reconciled condition should be seeded")
}

func TestResourceService_Create_SkipsConditionsWithoutRequiredAdapters(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors() // Channel has no RequiredAdapters

	mockDao := newMockResourceDao()
	svc, _, _, rcDao := newTestResourceServiceWithAdapterStatus(mockDao)

	resource := testResource("Channel", "", "stable")
	result, svcErr := svc.Create(context.Background(), "Channel", resource, nil)
	Expect(svcErr).To(BeNil())

	conditions := rcDao.conditions[result.ID]
	Expect(conditions).To(BeEmpty(), "No conditions should be seeded for entities without RequiredAdapters")
}

func TestResourceService_Patch_SkipsConditionsWithoutRequiredAdapters(t *testing.T) {
	RegisterTestingT(t)
	setupTestDescriptors() // Channel has no RequiredAdapters

	mockDao := newMockResourceDao()
	svc, _, _, rcDao := newTestResourceServiceWithAdapterStatus(mockDao)

	resource := testResource("Channel", "ch-1", "stable")
	created, svcErr := svc.Create(context.Background(), "Channel", resource, nil)
	Expect(svcErr).To(BeNil())
	Expect(rcDao.conditions[created.ID]).To(BeEmpty(), "No conditions on Create for zero-adapter entity")

	patch := &api.ResourcePatch{Spec: map[string]any{"updated": "value"}}
	_, svcErr = svc.Patch(context.Background(), "Channel", "ch-1", patch)
	Expect(svcErr).To(BeNil())

	Expect(rcDao.conditions[created.ID]).To(BeEmpty(),
		"Patch should not seed conditions for entities without RequiredAdapters")
}

func TestProcessAdapterStatus_FinalizedTrue_RecomputesConditions_WhenHardDeleteBlocked(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:             "Parent",
		Plural:           "parents",
		RequiredAdapters: []string{"adapter1"},
	})
	registry.Register(registry.EntityDescriptor{
		Kind:           "Child",
		Plural:         "children",
		ParentKind:     "Parent",
		OnParentDelete: registry.OnParentDeleteCascade,
	})

	mockDao := newMockResourceDao()
	svc, _, _, rcDao := newTestResourceServiceWithAdapterStatus(mockDao)

	deletedAt := time.Now().UTC()
	parent := testResource("Parent", "p-1", "parent")
	parent.Generation = 1
	parent.DeletedTime = &deletedAt
	mockDao.addResource(parent)

	// Active child blocks hard-delete.
	childOwner := "p-1"
	child := testResource("Child", "c-1", "child")
	child.OwnerID = &childOwner
	mockDao.addResource(child)

	// Adapter reports Finalized=True with Available=True (unchanged from a prior report).
	req := &api.AdapterStatus{
		Adapter:            "adapter1",
		ObservedGeneration: 1,
		LastReportTime:     time.Now().UTC(),
		Conditions: testConditionsJSON(
			api.AdapterCondition{Type: api.AdapterConditionTypeAvailable, Status: api.AdapterConditionTrue},
			api.AdapterCondition{Type: api.AdapterConditionTypeApplied, Status: api.AdapterConditionTrue},
			api.AdapterCondition{Type: api.AdapterConditionTypeHealth, Status: api.AdapterConditionTrue},
			api.AdapterCondition{Type: api.AdapterConditionTypeFinalized, Status: api.AdapterConditionTrue},
		),
	}

	result, svcErr := svc.ProcessAdapterStatus(context.Background(), "Parent", "p-1", req)
	Expect(svcErr).To(BeNil())
	Expect(result).ToNot(BeNil())

	// Parent should NOT be hard-deleted (child blocks it).
	_, exists := mockDao.resources[resourceKey("Parent", "p-1")]
	Expect(exists).To(BeTrue(), "Parent should still exist - active child blocks hard-delete")

	// Conditions MUST be recomputed despite hard-delete being blocked.
	// Available=True in the report triggers aggregation, which should
	// still run and reflect the blocked-by-children state.
	conditions := rcDao.conditions["p-1"]
	Expect(conditions).ToNot(BeEmpty(),
		"Conditions should be recomputed when Finalized=True is reported, even if hard-delete is blocked")

	recon := findCondition(conditions, api.ResourceConditionTypeReconciled)
	Expect(recon.Status).To(Equal(api.ConditionFalse),
		"Reconciled should be False (waiting for children), not absent")
	Expect(*recon.Reason).To(ContainSubstring("Children"),
		"Reason should indicate waiting for child resources")
}
