package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/util"
)

//go:generate go tool -modfile=../../tools/go.mod mockgen -source=resource.go -package=services -destination=resource_mock.go

type ResourceService interface {
	Get(ctx context.Context, kind, id string) (*api.Resource, *errors.ServiceError)
	Create(ctx context.Context, kind string, resource *api.Resource, refs api.ReferenceMap) (*api.Resource, *errors.ServiceError) //nolint:lll
	Patch(ctx context.Context, kind, id string, patch *api.ResourcePatch) (*api.Resource, *errors.ServiceError)
	Delete(ctx context.Context, kind, id string) (*api.Resource, *errors.ServiceError)
	List(ctx context.Context, kind string, args *ListArguments) (api.ResourceList, *api.PagingMeta, *errors.ServiceError)
	GetByOwner(ctx context.Context, kind, id, ownerID string) (*api.Resource, *errors.ServiceError)
	ListByOwner(ctx context.Context, kind, ownerID string, args *ListArguments) (api.ResourceList, *api.PagingMeta, *errors.ServiceError) // nolint:lll
	ForceDelete(ctx context.Context, kind, id, reason string) *errors.ServiceError
	GetByID(ctx context.Context, id string) (*api.Resource, *errors.ServiceError)
	ListAll(ctx context.Context, args *ListArguments) (api.ResourceList, *api.PagingMeta, *errors.ServiceError)
	ProcessAdapterStatus(ctx context.Context, kind, resourceID string, adapterStatus *api.AdapterStatus) (*api.AdapterStatus, *errors.ServiceError) // nolint:lll
	ListWithStatuses(ctx context.Context, kind string, args *ListArguments) ([]ResourceWithStatuses, *api.PagingMeta, *errors.ServiceError)            // nolint:lll
}

type ResourceWithStatuses struct {
	Resource       *api.Resource
	AdapterStatuses api.AdapterStatusList
}

func NewResourceService(
	resourceDao dao.ResourceDao,
	resourceLabelDao dao.ResourceLabelDao,
	adapterStatusDao dao.AdapterStatusDao,
	generic GenericService,
) ResourceService {
	return &sqlResourceService{
		resourceDao:      resourceDao,
		resourceLabelDao: resourceLabelDao,
		adapterStatusDao: adapterStatusDao,
		generic:          generic,
	}
}

var _ ResourceService = &sqlResourceService{}

type sqlResourceService struct {
	resourceDao      dao.ResourceDao
	resourceLabelDao dao.ResourceLabelDao
	adapterStatusDao dao.AdapterStatusDao
	generic          GenericService
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
// refs carries the non-ownership references from the API request. nil means "no references
// supplied" — required ref types (Min > 0) will still be validated and rejected if missing.
// An empty map {} means "clear all references" — Min>0 descriptors will reject this with 400.
func (s *sqlResourceService) Create(
	ctx context.Context, kind string, resource *api.Resource,
	refs api.ReferenceMap,
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

	if svcErr := s.validateReferences(ctx, kind, refs); svcErr != nil {
		return nil, svcErr
	}

	username := actorFromContext(ctx)
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

	if len(resource.Labels) > 0 {
		if labelErr := s.resourceLabelDao.ReplaceLabels(ctx, resource.ID, resource.Labels); labelErr != nil {
			return nil, handleCreateError(kind, labelErr)
		}
	}

	// Persist references after the resource row exists (FK requires source_id).
	if len(refs) > 0 {
		refRows := convertRefs(kind, resource.ID, refs)
		if refErr := s.resourceDao.ReplaceReferences(ctx, resource.ID, refRows); refErr != nil {
			return nil, errors.GeneralError("failed to save references: %s", refErr)
		}
		resource.References = refRows
	}

	return resource, nil
}

func (s *sqlResourceService) Patch(
	ctx context.Context, kind, id string, patch *api.ResourcePatch,
) (*api.Resource, *errors.ServiceError) {
	if svcErr := validateKind(kind); svcErr != nil {
		return nil, svcErr
	}
	resource, err := s.resourceDao.GetForUpdate(ctx, kind, id)
	if err != nil {
		return nil, handleGetError(kind, "id", id, err)
	}

	if resource.DeletedTime != nil {
		return nil, errors.ConflictState("%s '%s' is marked for deletion", kind, id)
	}

	oldSpec := append([]byte(nil), resource.Spec...)
	oldLabels := resource.Labels

	if applyErr := applyResourcePatch(resource, patch); applyErr != nil {
		return nil, errors.Validation("Invalid patch data: %v", applyErr)
	}

	specChanged := !jsonBytesEqual(oldSpec, resource.Spec)
	labelsChanged := !labelsEqual(oldLabels, resource.Labels)
	refsChanged := patch.References != nil

	// Validate and persist references when the patch includes them (nil = skip, {} = clear).
	if refsChanged {
		if svcErr := s.validateReferences(ctx, kind, patch.References); svcErr != nil {
			return nil, svcErr
		}
		refRows := convertRefs(kind, resource.ID, patch.References)
		if refErr := s.resourceDao.ReplaceReferences(
			ctx, resource.ID, refRows,
		); refErr != nil {
			return nil, errors.GeneralError("failed to save references: %s", refErr)
		}
		resource.References = refRows
	}

	if !specChanged && !labelsChanged && !refsChanged {
		return resource, nil
	}

	resource.IncrementGeneration()
	resource.UpdatedBy = actorFromContext(ctx)

	if saveErr := s.resourceDao.Save(ctx, resource); saveErr != nil {
		return nil, handleUpdateError(kind, saveErr)
	}

	if labelsChanged {
		if labelErr := s.resourceLabelDao.ReplaceLabels(ctx, resource.ID, resource.Labels); labelErr != nil {
			return nil, handleUpdateError(kind, labelErr)
		}
	}

	return resource, nil
}

// Resources with required adapters are soft-deleted; all others are hard-deleted.
func (s *sqlResourceService) Delete(ctx context.Context, kind, id string) (*api.Resource, *errors.ServiceError) {
	if svcErr := validateKind(kind); svcErr != nil {
		return nil, svcErr
	}
	resource, err := s.resourceDao.GetForUpdate(ctx, kind, id)
	if err != nil {
		return nil, handleSoftDeleteError(kind, err)
	}

	deletedBy := actorFromContext(ctx)
	deletedAt := time.Now().UTC().Truncate(time.Microsecond)

	// Mark for deletion if not already soft-deleted
	if resource.DeletedTime == nil {
		resource.MarkDeleted(deletedBy, deletedAt)
		resource.IncrementGeneration()
	}

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

	// Check if other resources reference this one before any deletion.
	referencers, refErr := s.resourceDao.FindReferencers(ctx, resource.ID)
	if refErr != nil {
		return errors.GeneralError("failed to check references: %s", refErr)
	}
	// List out all the references for a specific resource
	if len(referencers) > 0 {
		names := make([]string, len(referencers))
		for i, r := range referencers {
			names[i] = fmt.Sprintf("%s %q", r.Kind, r.Name)
		}
		return errors.ConflictState(
			"cannot delete %s %q: referenced by %s — remove the reference(s) before deleting",
			resource.Kind, resource.Name, strings.Join(names, ", "))
	}

	shouldSoftDelete, svcErr := s.shouldSoftDelete(ctx, resource, children)
	if svcErr != nil {
		return svcErr
	}

	if shouldSoftDelete {
		if saveErr := s.resourceDao.Save(ctx, resource); saveErr != nil {
			return handleSoftDeleteError(resource.Kind, saveErr)
		}
		if err := s.resourceDao.ReplaceReferences(ctx, resource.ID, nil); err != nil {
			return errors.GeneralError("failed to clear outbound references on soft-delete: %s", err)
		}
		return nil
	}

	if err := s.resourceDao.Delete(ctx, resource.Kind, resource.ID); err != nil {
		return handleDeleteError(resource.Kind, err)
	}

	return nil
}

// shouldSoftDelete determines whether a resource requires soft-deletion.
// Soft-delete is required when:
// 1. Resource has RequiredAdapters (must wait for adapter finalization)
// 2. Resource has soft-deleted children (parent must remain until children are gone)
func (s *sqlResourceService) shouldSoftDelete(
	ctx context.Context, resource *api.Resource, children []registry.EntityDescriptor,
) (bool, *errors.ServiceError) {
	// Resource has soft-deleted children
	// Parent must remain in DB until all children (active or soft-deleted) are gone
	if len(children) > 0 {
		childKinds := make([]string, len(children))
		for i, child := range children {
			childKinds[i] = child.Kind
		}
		exists, err := s.resourceDao.ExistsSoftDeletedByOwner(ctx, childKinds, resource.ID)
		if err != nil {
			return false, errors.GeneralError(
				"Unable to check soft-deleted children: %s", err,
			)
		}
		if exists {
			return true, nil
		}
	}

	return false, nil
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

// GetByOwner returns a single child resource scoped to the specified owner. Returns 404 if not found.
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
		args = NewListArguments()
	}
	scopedArgs := *args
	scopedArgs.Preloads = append(append([]string(nil), scopedArgs.Preloads...), "Labels", "References")
	kindFilter := fmt.Sprintf("kind = '%s'", kind)
	if scopedArgs.Search == "" {
		scopedArgs.Search = kindFilter
	} else {
		scopedArgs.Search = "(" + scopedArgs.Search + ") AND " + kindFilter
	}

	if svcErr := s.applyRefFilter(ctx, kind, &scopedArgs); svcErr != nil {
		return nil, nil, svcErr
	}

	var resources api.ResourceList
	paging, svcErr := s.generic.List(ctx, &scopedArgs, &resources)
	if svcErr != nil {
		return nil, nil, svcErr
	}
	return resources, paging, nil
}

// ListByOwner returns child resources of the given owner with pagination, search, and ordering.
func (s *sqlResourceService) ListByOwner(
	ctx context.Context, kind, ownerID string, args *ListArguments,
) (api.ResourceList, *api.PagingMeta, *errors.ServiceError) {
	if svcErr := validateKind(kind); svcErr != nil {
		return nil, nil, svcErr
	}
	if args == nil {
		args = NewListArguments()
	}
	scopedArgs := *args
	scopedArgs.Preloads = append(append([]string(nil), scopedArgs.Preloads...), "Labels", "References")
	kindFilter := fmt.Sprintf("kind = '%s' AND owner_id = '%s'", kind, ownerID)
	if scopedArgs.Search == "" {
		scopedArgs.Search = kindFilter
	} else {
		scopedArgs.Search = "(" + scopedArgs.Search + ") AND " + kindFilter
	}

	if svcErr := s.applyRefFilter(ctx, kind, &scopedArgs); svcErr != nil {
		return nil, nil, svcErr
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

func (s *sqlResourceService) GetByID(ctx context.Context, id string) (*api.Resource, *errors.ServiceError) {
	resource, err := s.resourceDao.GetByID(ctx, id)
	if err != nil {
		return nil, handleGetError("Resource", "id", id, err)
	}
	return resource, nil
}

func (s *sqlResourceService) ListAll(
	ctx context.Context, args *ListArguments,
) (api.ResourceList, *api.PagingMeta, *errors.ServiceError) {
	if args == nil {
		args = NewListArguments()
	}
	scopedArgs := *args
	scopedArgs.Preloads = append(append([]string(nil), scopedArgs.Preloads...), "Labels", "References")
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

// ProcessAdapterStatus validates and upserts an adapter status report.
// No aggregation is performed — the API is a simple store.
func (s *sqlResourceService) ProcessAdapterStatus(
	ctx context.Context, kind, resourceID string, adapterStatus *api.AdapterStatus,
) (*api.AdapterStatus, *errors.ServiceError) {
	if svcErr := validateKind(kind); svcErr != nil {
		return nil, svcErr
	}

	resource, err := s.resourceDao.GetForUpdate(ctx, kind, resourceID)
	if err != nil {
		return nil, handleGetError(kind, "id", resourceID, err)
	}

	allStatuses, err := s.adapterStatusDao.FindByResource(ctx, kind, resourceID)
	if err != nil {
		return nil, errors.GeneralError("Failed to get adapter statuses: %s", err)
	}

	existingStatus := findAdapterStatusInList(allStatuses, adapterStatus.Adapter)

	log := logger.With(ctx, "resource_type", kind, "resource_id", resourceID,
		logger.FieldAdapter, adapterStatus.Adapter)
	_, _, svcErr := validateAndClassifyAdapterStatus(
		resource.Generation, adapterStatus, existingStatus, log,
	)
	if svcErr != nil {
		return nil, svcErr
	}

	adapterStatus.ResourceType = kind
	adapterStatus.ResourceID = resourceID
	setConditionTransitionTimes(adapterStatus, existingStatus)

	upsertedStatus, err := s.adapterStatusDao.Upsert(ctx, adapterStatus, existingStatus)
	if err != nil {
		return nil, handleCreateError("AdapterStatus", err)
	}

	return upsertedStatus, nil
}

// hasActiveChildren returns true if any registered child kind has at least one
// active (non-deleted) resource owned by the given resource.
func (s *sqlResourceService) hasActiveChildren(
	ctx context.Context, resource *api.Resource,
) (bool, error) {
	for _, child := range registry.ChildrenOf(resource.Kind) {
		exists, err := s.resourceDao.ExistsByOwner(ctx, child.Kind, resource.ID)
		if err != nil {
			return false, err
		}
		if exists {
			return true, nil
		}
	}
	return false, nil
}

func (s *sqlResourceService) applyRefFilter(
	ctx context.Context, kind string, args *ListArguments,
) *errors.ServiceError {
	if args.RefType == "" {
		return nil
	}
	desc := registry.MustGet(kind)
	found := false
	for _, ref := range desc.References {
		if ref.RefType == args.RefType {
			found = true
			break
		}
	}
	if !found {
		return errors.Validation("Unknown ref_type %q for entity %s", args.RefType, kind)
	}
	sourceIDs, err := s.resourceDao.FindSourceIDsByRef(ctx, args.RefType, args.RefTargetID)
	if err != nil {
		return errors.GeneralError("failed to query references: %s", err)
	}
	if len(sourceIDs) == 0 {
		args.Search += ` AND id = ""`
		return nil
	}
	// sourceIDs are server-generated UUIDs from the database, so manual quoting is safe.
	quoted := make([]string, len(sourceIDs))
	for i, sid := range sourceIDs {
		quoted[i] = `"` + sid + `"`
	}
	args.Search += " AND id in [" + strings.Join(quoted, ", ") + "]"
	return nil
}

// validateKind checks that the kind is a registered entity type.
// Returns 400 if the kind is unknown, preventing invalid kinds from reaching the DAO.
func validateKind(kind string) *errors.ServiceError {
	if _, ok := registry.Get(kind); !ok {
		return errors.Validation("Unknown entity kind: %s", kind)
	}
	return nil
}

// Name format/length validation is handled by OpenAPI spec validation middleware.
func validateResourceName(kind, name string) *errors.ServiceError {
	if svcErr := validateKind(kind); svcErr != nil {
		return svcErr
	}
	if name == "" {
		return errors.Validation("%s name cannot be empty", kind)
	}
	return nil
}

// jsonBytesEqual is a nil-safe wrapper around jsonEqual for comparing JSONB spec fields.
func jsonBytesEqual(a, b []byte) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	return jsonEqual(a, b)
}

// labelsEqual compares two label slices by key-value content (order-independent).
func labelsEqual(a, b []api.ResourceLabel) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]string, len(a))
	for _, l := range a {
		m[l.Key] = l.Value
	}
	for _, l := range b {
		if v, ok := m[l.Key]; !ok || v != l.Value {
			return false
		}
	}
	return true
}

func applyResourcePatch(resource *api.Resource, patch *api.ResourcePatch) error {
	if patch.Spec != nil {
		specJSON, err := json.Marshal(patch.Spec)
		if err != nil {
			return fmt.Errorf("failed to marshal resource spec: %w", err)
		}
		resource.Spec = specJSON
	}
	if patch.Labels != nil {
		labels := make([]api.ResourceLabel, 0, len(patch.Labels))
		for k, v := range patch.Labels {
			if err := api.ValidateLabel(k, v); err != nil {
				return err
			}
			labels = append(labels, api.ResourceLabel{Key: k, Value: v})
		}
		resource.Labels = labels
	}

	return nil
}

func (s *sqlResourceService) ForceDelete(ctx context.Context, kind, id, reason string) *errors.ServiceError {
	if svcErr := validateKind(kind); svcErr != nil {
		return svcErr
	}

	resource, err := s.resourceDao.GetForUpdate(ctx, kind, id)
	if err != nil {
		return handleGetError(kind, "id", id, err)
	}

	if resource.DeletedTime == nil {
		return errors.ConflictState("%s '%s' is not in Finalizing state", kind, id)
	}

	caller := actorFromContext(ctx)
	if svcErr := s.forceDeleteResourceTree(ctx, resource, caller, reason); svcErr != nil {
		db.MarkForRollback(ctx, svcErr)
		return svcErr
	}
	return nil
}

func (s *sqlResourceService) forceDeleteResourceTree(
	ctx context.Context, resource *api.Resource, caller, reason string,
) *errors.ServiceError {
	children := registry.ChildrenOf(resource.Kind)

	childIDs := make([]string, 0)
	for _, child := range children {
		items, err := s.resourceDao.FindByKindAndOwnerForUpdate(ctx, child.Kind, resource.ID)
		if err != nil {
			logger.With(ctx, "resource_id", resource.ID, "child_kind", child.Kind).
				WithError(err).Error("Failed to find children for force-delete")
			return errors.GeneralError("Unable to find %s children for force-delete", child.Kind)
		}
		for _, item := range items {
			childIDs = append(childIDs, item.ID)
			if svcErr := s.forceDeleteResourceTree(ctx, item, caller, reason); svcErr != nil {
				return svcErr
			}
		}
	}

	logger.With(ctx,
		"resource_kind", resource.Kind,
		"resource_id", resource.ID,
		"caller", caller,
		"reason", reason,
		"child_resource_ids", childIDs,
	).Info("Force-deleting resource")

	if err := s.adapterStatusDao.DeleteByResource(ctx, resource.Kind, resource.ID); err != nil {
		return errors.GeneralError("Failed to delete adapter statuses during force-delete: %s", err)
	}
	// Clear inbound references before hard-deleting (FK uses ON DELETE RESTRICT).
	// Note: referencing resources with Min>0 on this ref type will silently
	// violate their required-reference invariant after this operation.
	if err := s.resourceDao.ClearTargetReferences(ctx, resource.ID); err != nil {
		return errors.GeneralError("failed to clear references: %s", err)
	}
	logger.With(ctx,
		"resource_kind", resource.Kind,
		"resource_id", resource.ID,
	).Info("Cleared inbound references for force-delete")
	if err := s.resourceDao.Delete(ctx, resource.Kind, resource.ID); err != nil {
		return handleDeleteError(resource.Kind, err)
	}

	return nil
}

// validateReferences checks that refs satisfies the ReferenceDescriptors on the entity:
//   - required ref types (Min > 0) must be present
//   - unknown ref types are rejected
//   - per-type count must not exceed Max (when Max > 0)
//   - every referenced target must exist in the database
func (s *sqlResourceService) validateReferences(
	ctx context.Context, kind string, refs api.ReferenceMap,
) *errors.ServiceError {
	desc := registry.MustGet(kind)

	descByType := make(map[string]registry.ReferenceDescriptor, len(desc.References))
	for _, rd := range desc.References {
		descByType[rd.RefType] = rd
		if rd.Min > 0 {
			if len(refs[rd.RefType]) < rd.Min {
				return errors.Validation(
					"required reference type %q missing for %s (min %d)",
					rd.RefType, kind, rd.Min,
				)
			}
		}
	}

	// Validate each supplied ref type.
	for refType, objRefs := range refs {
		rd, ok := descByType[refType]
		if !ok {
			return errors.Validation("unknown reference type %q for %s", refType, kind)
		}
		if rd.Max > 0 && len(objRefs) > rd.Max {
			return errors.Validation(
				"reference type %q for %s exceeds max count %d (got %d)",
				refType, kind, rd.Max, len(objRefs),
			)
		}
		seen := make(map[string]bool, len(objRefs))
		for _, ref := range objRefs {
			if ref.Id == nil || *ref.Id == "" {
				return errors.Validation("reference type %q: id is required", refType)
			}
			if seen[*ref.Id] {
				return errors.Validation(
					"reference type %q: duplicate target id %q",
					refType, *ref.Id,
				)
			}
			seen[*ref.Id] = true
			if ref.Kind != rd.TargetKind {
				return errors.Validation(
					"reference type %q: kind %q does not match expected target kind %q",
					refType, ref.Kind, rd.TargetKind,
				)
			}
			target, err := s.resourceDao.Get(ctx, rd.TargetKind, *ref.Id)
			if err != nil {
				return errors.Validation(
					"reference type %q: target %s %q not found",
					refType, rd.TargetKind, *ref.Id,
				)
			}
			if target.DeletedTime != nil {
				return errors.Validation(
					"reference type %q: target %s %q is marked for deletion",
					refType, rd.TargetKind, *ref.Id,
				)
			}
		}
	}

	return nil
}

// convertRefs flattens the API reference map into a slice of ResourceReference rows for the DAO.
// Uses the registry's TargetKind (not the client-supplied Kind) so the stored value is always authoritative.
func convertRefs(kind, sourceID string, refs api.ReferenceMap) []api.ResourceReference {
	desc := registry.MustGet(kind)
	targetKindByRef := make(map[string]string, len(desc.References))
	for _, rd := range desc.References {
		targetKindByRef[rd.RefType] = rd.TargetKind
	}
	var result []api.ResourceReference
	for refType, objRefs := range refs {
		for _, ref := range objRefs {
			result = append(result, api.ResourceReference{
				SourceID:   sourceID,
				RefType:    refType,
				TargetID:   *ref.Id,
				TargetKind: targetKindByRef[refType],
			})
		}
	}
	return result
}

func (s *sqlResourceService) ListWithStatuses(
	ctx context.Context, kind string, args *ListArguments,
) ([]ResourceWithStatuses, *api.PagingMeta, *errors.ServiceError) {
	resources, paging, svcErr := s.List(ctx, kind, args)
	if svcErr != nil {
		return nil, nil, svcErr
	}

	if len(resources) == 0 {
		return []ResourceWithStatuses{}, paging, nil
	}

	ids := make([]string, len(resources))
	for i, r := range resources {
		ids[i] = r.ID
	}

	allStatuses, err := s.adapterStatusDao.FindByResourceIDs(ctx, kind, ids)
	if err != nil {
		return nil, nil, errors.GeneralError("failed to fetch adapter statuses: %s", err)
	}

	statusesByResource := make(map[string]api.AdapterStatusList)
	for _, as := range allStatuses {
		statusesByResource[as.ResourceID] = append(statusesByResource[as.ResourceID], as)
	}

	result := make([]ResourceWithStatuses, len(resources))
	for i, r := range resources {
		result[i] = ResourceWithStatuses{
			Resource:        r,
			AdapterStatuses: statusesByResource[r.ID],
		}
	}
	return result, paging, nil
}
