package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/auth"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/util"
)

//go:generate go tool mockgen -source=resource.go -package=services -destination=resource_mock.go

type ResourceService interface {
	Get(ctx context.Context, kind, id string) (*api.Resource, *errors.ServiceError)
	Create(ctx context.Context, kind string, resource *api.Resource) (*api.Resource, *errors.ServiceError)
	Patch(ctx context.Context, kind, id string, patch *api.ResourcePatchRequest) (*api.Resource, *errors.ServiceError)
	Delete(ctx context.Context, kind, id string) (*api.Resource, *errors.ServiceError)
	FindByIDs(ctx context.Context, kind string, ids []string) (api.ResourceList, *errors.ServiceError)
	List(ctx context.Context, kind string, args *ListArguments) (api.ResourceList, *api.PagingMeta, *errors.ServiceError)
	GetByOwner(ctx context.Context, kind, id, ownerID string) (*api.Resource, *errors.ServiceError)
	ListByOwner(ctx context.Context, kind, ownerID string, args *ListArguments) (api.ResourceList, *api.PagingMeta, *errors.ServiceError) // nolint:lll
}

func NewResourceService(resourceDao dao.ResourceDao, generic GenericService) ResourceService {
	return &sqlResourceService{resourceDao: resourceDao, generic: generic}
}

var _ ResourceService = &sqlResourceService{}

type sqlResourceService struct {
	resourceDao dao.ResourceDao
	generic     GenericService
}

// Get returns a single resource by kind and ID. Returns 404 if not found.
func (s *sqlResourceService) Get(ctx context.Context, kind, id string) (*api.Resource, *errors.ServiceError) {
	if svcErr := validateKind(kind); svcErr != nil {
		return nil, svcErr
	}
	resource, err := s.resourceDao.Get(ctx, kind, id)
	if err != nil {
		return nil, handleGetError(kind, "id", id, err)
	}
	return resource, nil
}

// Create validates name constraints from the EntityDescriptor, sets CreatedBy/UpdatedBy
// from the auth context, and persists a new resource. ID generation, timestamps, href
// computation, and generation initialisation are handled by the GORM BeforeCreate hook.
func (s *sqlResourceService) Create(
	ctx context.Context, kind string, resource *api.Resource,
) (*api.Resource, *errors.ServiceError) {
	resource.Kind = kind

	if svcErr := validateResourceName(kind, resource.Name); svcErr != nil {
		return nil, svcErr
	}

	// Lock parent row to serialize with concurrent deletes.
	if ownerID := util.FromPtr(resource.OwnerID); ownerID != "" {
		desc := registry.MustGet(kind)
		parent, err := s.resourceDao.GetForUpdate(ctx, desc.ParentKind, ownerID)
		if err != nil {
			return nil, handleGetError(desc.ParentKind, "id", ownerID, err)
		}
		if parent.DeletedTime != nil {
			return nil, errors.ConflictState("%s '%s' is marked for deletion", desc.ParentKind, ownerID)
		}
	}

	username := auth.GetUsernameFromContext(ctx)
	if username == "" {
		username = defaultSystemUser
	}
	if resource.CreatedBy == "" {
		resource.CreatedBy = username
	}
	if resource.UpdatedBy == "" {
		resource.UpdatedBy = username
	}

	resource, err := s.resourceDao.Create(ctx, resource)
	if err != nil {
		return nil, handleCreateError(kind, err)
	}
	return resource, nil
}

// Patch applies spec/label changes to a resource. Acquires a row-level lock via GetForUpdate
// to prevent concurrent modifications. Increments generation only when spec or labels actually
// change (compared via deep JSON equality). Rejects patches on soft-deleted resources with 409.
func (s *sqlResourceService) Patch(
	ctx context.Context, kind, id string, patch *api.ResourcePatchRequest,
) (*api.Resource, *errors.ServiceError) {
	if svcErr := validateKind(kind); svcErr != nil {
		return nil, svcErr
	}
	resource, err := s.resourceDao.GetForUpdate(ctx, kind, id)
	if err != nil {
		return nil, handleGetError(kind, "id", id, err)
	}

	// Check if resource is marked for deletion
	if resource.DeletedTime != nil {
		return nil, errors.ConflictState("%s '%s' is marked for deletion", kind, id)
	}

	// Snapshot current values before applying the patch. Defensive copy required because
	// applyResourcePatch replaces the slice reference, and we need the originals for comparison.
	oldSpec := append([]byte(nil), resource.Spec...)
	oldLabels := append([]byte(nil), resource.Labels...)

	if applyErr := applyResourcePatch(resource, patch); applyErr != nil {
		return nil, errors.Validation("Invalid patch data: %v", applyErr)
	}

	if jsonBytesEqual(oldSpec, resource.Spec) && jsonBytesEqual(oldLabels, resource.Labels) {
		return resource, nil
	}

	resource.IncrementGeneration()

	username := auth.GetUsernameFromContext(ctx)
	if username == "" {
		username = defaultSystemUser
	}
	resource.UpdatedBy = username

	if saveErr := s.resourceDao.Save(ctx, resource); saveErr != nil {
		return nil, handleUpdateError(kind, saveErr)
	}

	return resource, nil
}

// Delete removes a resource and its cascade subtree. Resources with required
// adapters are soft-deleted; all others are hard-deleted.
func (s *sqlResourceService) Delete(ctx context.Context, kind, id string) (*api.Resource, *errors.ServiceError) {
	if svcErr := validateKind(kind); svcErr != nil {
		return nil, svcErr
	}
	resource, err := s.resourceDao.GetForUpdate(ctx, kind, id)
	if err != nil {
		return nil, handleSoftDeleteError(kind, err)
	}

	if resource.DeletedTime != nil {
		return resource, nil
	}

	deletedBy := actorFromContext(ctx)
	deletedAt := time.Now().UTC().Truncate(time.Microsecond)

	resource.MarkDeleted(deletedBy, deletedAt)
	resource.IncrementGeneration()

	if svcErr := s.deleteResourceTree(ctx, resource, deletedBy, deletedAt); svcErr != nil {
		db.MarkForRollback(ctx, svcErr)
		return nil, svcErr
	}

	return resource, nil
}

// deleteResourceTree enforces child delete policies then persists bottom-up.
func (s *sqlResourceService) deleteResourceTree(
	ctx context.Context, resource *api.Resource,
	deletedBy string, deletedAt time.Time,
) *errors.ServiceError {
	children := registry.ChildrenOf(resource.Kind)

	for _, child := range children {
		if child.OnParentDelete == registry.OnParentDeleteRestrict {
			if svcErr := s.checkCanDelete(ctx, resource, child); svcErr != nil {
				return svcErr
			}
		}
	}

	for _, child := range children {
		if child.OnParentDelete == registry.OnParentDeleteCascade {
			items, err := s.resourceDao.FindByKindAndOwnerForUpdate(ctx, child.Kind, resource.ID)
			if err != nil {
				return errors.GeneralError(
					"Unable to find %s children for cascade delete: %s", child.Kind, err,
				)
			}
			for _, item := range items {
				if item.DeletedTime == nil {
					item.MarkDeleted(deletedBy, deletedAt)
					item.IncrementGeneration()
				}
				if svcErr := s.deleteResourceTree(ctx, item, deletedBy, deletedAt); svcErr != nil {
					return svcErr
				}
			}
		}
	}

	desc := registry.MustGet(resource.Kind)
	if len(desc.RequiredAdapters) > 0 {
		if saveErr := s.resourceDao.Save(ctx, resource); saveErr != nil {
			return handleSoftDeleteError(resource.Kind, saveErr)
		}
		return nil
	}

	if err := s.resourceDao.Delete(ctx, resource.Kind, resource.ID); err != nil {
		return handleDeleteError(resource.Kind, err)
	}

	return nil
}

func (s *sqlResourceService) checkCanDelete(
	ctx context.Context, resource *api.Resource, child registry.EntityDescriptor,
) *errors.ServiceError {
	exists, err := s.resourceDao.ExistsByOwner(ctx, child.Kind, resource.ID)
	if err != nil {
		return errors.GeneralError("Unable to check %s children: %s", child.Kind, err)
	}
	if exists {
		return errors.ConflictState(
			"Cannot delete %s '%s': active %s(s) exist",
			resource.Kind, resource.ID, child.Kind,
		)
	}
	return nil
}

// FindByIDs returns resources matching the given IDs, scoped to the specified kind.
func (s *sqlResourceService) FindByIDs(
	ctx context.Context, kind string, ids []string,
) (api.ResourceList, *errors.ServiceError) {
	if svcErr := validateKind(kind); svcErr != nil {
		return nil, svcErr
	}
	resources, err := s.resourceDao.FindByIDs(ctx, kind, ids)
	if err != nil {
		return nil, errors.GeneralError("Unable to find %s resources by IDs: %s", kind, err)
	}
	return resources, nil
}

// GetByOwner returns a single child resource, validated as belonging to the specified owner.
// Returns 404 if the resource doesn't exist or belongs to a different owner.
func (s *sqlResourceService) GetByOwner(
	ctx context.Context, kind, id, ownerID string,
) (*api.Resource, *errors.ServiceError) {
	if svcErr := validateKind(kind); svcErr != nil {
		return nil, svcErr
	}
	resource, err := s.resourceDao.GetByOwner(ctx, kind, id, ownerID)
	if err != nil {
		return nil, handleGetError(kind, "id", id, err)
	}
	return resource, nil
}

// List returns resources of the given kind with pagination, search, and ordering.
func (s *sqlResourceService) List(
	ctx context.Context, kind string, args *ListArguments,
) (api.ResourceList, *api.PagingMeta, *errors.ServiceError) {
	if svcErr := validateKind(kind); svcErr != nil {
		return nil, nil, svcErr
	}
	if args == nil {
		args = &ListArguments{Page: 1, Size: 100}
	}
	scopedArgs := *args
	kindFilter := fmt.Sprintf("kind = '%s'", kind)
	if scopedArgs.Search == "" {
		scopedArgs.Search = kindFilter
	} else {
		scopedArgs.Search = "(" + scopedArgs.Search + ") AND " + kindFilter
	}

	var resources api.ResourceList
	paging, svcErr := s.generic.List(ctx, &scopedArgs, &resources)
	if svcErr != nil {
		return nil, nil, svcErr
	}
	return resources, paging, nil
}

// ListByOwner returns child resources of the given owner with pagination, search, and ordering.
// Injects kind and owner_id filters into the TSL search string before delegating to GenericService.List.
// A shallow copy of args is made to avoid mutating the caller's ListArguments.
func (s *sqlResourceService) ListByOwner(
	ctx context.Context, kind, ownerID string, args *ListArguments,
) (api.ResourceList, *api.PagingMeta, *errors.ServiceError) {
	if svcErr := validateKind(kind); svcErr != nil {
		return nil, nil, svcErr
	}
	if args == nil {
		args = &ListArguments{Page: 1, Size: 100}
	}
	scopedArgs := *args
	kindFilter := fmt.Sprintf("kind = '%s' AND owner_id = '%s'", kind, ownerID)
	if scopedArgs.Search == "" {
		scopedArgs.Search = kindFilter
	} else {
		scopedArgs.Search = "(" + scopedArgs.Search + ") AND " + kindFilter
	}

	var resources []api.Resource
	paging, svcErr := s.generic.List(ctx, &scopedArgs, &resources)
	if svcErr != nil {
		return nil, nil, svcErr
	}

	result := make(api.ResourceList, len(resources))
	for i := range resources {
		result[i] = &resources[i]
	}
	return result, paging, nil
}

// validateKind checks that the kind is a registered entity type.
// Returns 400 if the kind is unknown, preventing invalid kinds from reaching the DAO.
func validateKind(kind string) *errors.ServiceError {
	if _, ok := registry.Get(kind); !ok {
		return errors.Validation("Unknown entity kind: %s", kind)
	}
	return nil
}

// validateResourceName checks that the kind is registered and the name is non-empty.
// Name format and length validation is handled by OpenAPI spec validation middleware.
func validateResourceName(kind, name string) *errors.ServiceError {
	if svcErr := validateKind(kind); svcErr != nil {
		return svcErr
	}
	if name == "" {
		return errors.Validation("%s name cannot be empty", kind)
	}
	return nil
}

// jsonBytesEqual is a nil-safe wrapper around jsonEqual. Returns true if both slices are
// nil/empty, false if only one is, and delegates to jsonEqual for semantic JSON comparison.
// Needed because Resource.Labels is nullable (JSONB NULL), and jsonEqual(nil, nil) returns
// false due to json.Unmarshal(nil) error.
func jsonBytesEqual(a, b []byte) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	return jsonEqual(a, b)
}

// applyResourcePatch merges non-nil patch fields into the resource by marshaling them to JSON.
func applyResourcePatch(resource *api.Resource, patch *api.ResourcePatchRequest) error {
	if patch.Spec != nil {
		specJSON, err := json.Marshal(*patch.Spec)
		if err != nil {
			return fmt.Errorf("failed to marshal resource spec: %w", err)
		}
		resource.Spec = specJSON
	}
	if patch.Labels != nil {
		labelsJSON, err := json.Marshal(*patch.Labels)
		if err != nil {
			return fmt.Errorf("failed to marshal resource labels: %w", err)
		}
		resource.Labels = labelsJSON
	}
	return nil
}
