package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/auth"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
)

//go:generate mockgen-v0.6.0 -source=resource.go -package=services -destination=resource_mock.go

type ResourceService interface {
	Get(ctx context.Context, kind, id string) (*api.Resource, *errors.ServiceError)
	Create(ctx context.Context, kind string, resource *api.Resource) (*api.Resource, *errors.ServiceError)
	Patch(ctx context.Context, kind, id string, patch *api.ResourcePatchRequest) (*api.Resource, *errors.ServiceError)
	Delete(ctx context.Context, kind, id string) (*api.Resource, *errors.ServiceError)
	FindByIDs(ctx context.Context, kind string, ids []string) (api.ResourceList, *errors.ServiceError)
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

// Delete performs a soft-delete by setting DeletedTime and DeletedBy, then incrementing generation.
// Idempotent — returns the resource unchanged if already marked for deletion.
// Child cascade policies are not enforced here (deferred to HYPERFLEET-1093).
func (s *sqlResourceService) Delete(ctx context.Context, kind, id string) (*api.Resource, *errors.ServiceError) {
	if svcErr := validateKind(kind); svcErr != nil {
		return nil, svcErr
	}
	resource, err := s.resourceDao.GetForUpdate(ctx, kind, id)
	if err != nil {
		return nil, handleSoftDeleteError(kind, err)
	}

	// Already marked for deletion — return as-is to keep the operation idempotent.
	if resource.DeletedTime != nil {
		return resource, nil
	}

	deletedTime := time.Now().UTC().Truncate(time.Microsecond)
	username := auth.GetUsernameFromContext(ctx)
	if username == "" {
		username = defaultSystemUser
	}
	resource.MarkDeleted(username, deletedTime)
	resource.IncrementGeneration()

	if saveErr := s.resourceDao.Save(ctx, resource); saveErr != nil {
		return nil, handleSoftDeleteError(kind, saveErr)
	}

	return resource, nil
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
