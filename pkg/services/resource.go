package services

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	hfl "github.com/openshift-hyperfleet/hyperfleet-logger"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/metrics"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/tenant"
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
}

func NewResourceService(
	resourceDao dao.ResourceDao,
	resourceLabelDao dao.ResourceLabelDao,
	adapterStatusDao dao.AdapterStatusDao,
	resourceConditionDao dao.ResourceConditionDao,
	generic GenericService,
	transactionRunner db.TxRunner,
) (ResourceService, error) {
	mappers, err := buildConditionMappers(registry.All())
	if err != nil {
		return nil, fmt.Errorf("initialize resource service: %w", err)
	}
	return &sqlResourceService{
		resourceDao:          resourceDao,
		resourceLabelDao:     resourceLabelDao,
		adapterStatusDao:     adapterStatusDao,
		resourceConditionDao: resourceConditionDao,
		generic:              generic,
		txRunner:             transactionRunner,
		conditionMappers:     mappers,
	}, nil
}

func buildConditionMappers(entities []registry.EntityDescriptor) (map[string]*ConditionMapper, error) {
	conditionMappers := make(map[string]*ConditionMapper)
	for _, descriptor := range entities {
		if len(descriptor.Conditions) > 0 {
			mapper, err := NewConditionMapper(descriptor.Kind, descriptor.Conditions)
			if err != nil {
				return nil, fmt.Errorf("failed to create condition mapper for %s: %w", descriptor.Kind, err)
			}
			conditionMappers[descriptor.Kind] = mapper
		}
	}
	return conditionMappers, nil
}

var _ ResourceService = &sqlResourceService{}

type sqlResourceService struct {
	resourceDao          dao.ResourceDao
	resourceLabelDao     dao.ResourceLabelDao
	adapterStatusDao     dao.AdapterStatusDao
	resourceConditionDao dao.ResourceConditionDao
	generic              GenericService
	txRunner             db.TxRunner
	conditionMappers     map[string]*ConditionMapper // Indexed by Kind (e.g., "Cluster", "NodePool")
}

// Get returns a single resource by kind and ID. Returns 404 if not found.
func (s *sqlResourceService) Get(ctx context.Context, kind, id string) (*api.Resource, *errors.ServiceError) {
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("hyperfleet.resource_id", id))
	if svcErr := validateKind(kind); svcErr != nil {
		return nil, svcErr
	}
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("hyperfleet.resource_type", registry.MustGet(kind).Plural))
	resource, err := s.resourceDao.Get(ctx, kind, id)
	if err != nil {
		return nil, handleGetError(kind, id, err)
	}
	return resource, nil
}

// rejectSystemIdentityWrite returns a Forbidden error if the caller is a
// system identity (Sentinel, adapters). System identities may only write
// status and conditions via ProcessAdapterStatus; every other resource
// mutation is rejected here.
func rejectSystemIdentityWrite(ctx context.Context) *errors.ServiceError {
	if t := tenant.FromContext(ctx); t != nil && t.System {
		return errors.Forbidden("system identities may only write status and conditions")
	}
	return nil
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
	if svcErr := rejectSystemIdentityWrite(ctx); svcErr != nil {
		return nil, svcErr
	}
	resource.Kind = kind
	if svcErr := validateKind(kind); svcErr != nil {
		return nil, svcErr
	}
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("hyperfleet.resource_type", registry.MustGet(kind).Plural))
	if svcErr := validateName(kind, resource.Name); svcErr != nil {
		return nil, svcErr
	}

	var result *api.Resource
	var outcome *reconciliationOutcome
	err := s.txRunner.Do(ctx, func(ctx context.Context) error {
		// Lock parent row to serialize with concurrent deletes.
		if ownerID := util.FromPtr(resource.OwnerID); ownerID != "" {
			desc := registry.MustGet(kind)
			parent, err := s.resourceDao.GetForUpdate(ctx, desc.ParentKind, ownerID)
			if err != nil {
				return handleGetError(desc.ParentKind, ownerID, err)
			}
			if parent.DeletedTime != nil {
				return errors.ConflictState("%s '%s' is marked for deletion", desc.ParentKind, ownerID)
			}
		}

		if svcErr := s.validateReferences(ctx, kind, refs); svcErr != nil {
			return svcErr
		}

		username := actorFromContext(ctx)
		if resource.CreatedBy == "" {
			resource.CreatedBy = username
		}
		if resource.UpdatedBy == "" {
			resource.UpdatedBy = username
		}
		resource.Tenancy = tenant.TenancyJSON(ctx)

		created, err := s.resourceDao.Create(ctx, resource)
		if err != nil {
			return handleCreateError(kind, err)
		}
		result = created
		trace.SpanFromContext(ctx).SetAttributes(attribute.String("hyperfleet.resource_id", created.ID))

		if len(created.Labels) > 0 {
			if labelErr := s.resourceLabelDao.ReplaceLabels(ctx, created.ID, created.Labels); labelErr != nil {
				return handleCreateError(kind, labelErr)
			}
		}

		// Persist references after the resource row exists (FK requires source_id).
		if len(refs) > 0 {
			refRows := convertRefs(kind, created.ID, refs)
			if refErr := s.resourceDao.ReplaceReferences(ctx, created.ID, refRows); refErr != nil {
				return errors.GeneralError("failed to save references: %s", refErr)
			}
			created.References = refRows
		}

		// Initialize conditions for entities with required adapters, matching the
		// old ClusterService/NodePoolService behavior.
		desc := registry.MustGet(kind)
		if len(desc.RequiredAdapters) > 0 {
			recomputed, svcErr := s.recomputeAndSaveResourceConditions(ctx, created, nil)
			if svcErr != nil {
				return svcErr
			}
			outcome = recomputed
		}

		return nil
	})
	if svcErr := serviceErrorFromTransaction(err); svcErr != nil {
		return nil, svcErr
	}
	if outcome != nil {
		metrics.RecordReconciliationStarted(outcome.kind, outcome.isDelete)
	}
	return result, nil
}

func (s *sqlResourceService) Patch(
	ctx context.Context, kind, id string, patch *api.ResourcePatch,
) (*api.Resource, *errors.ServiceError) {
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("hyperfleet.resource_id", id))
	if svcErr := rejectSystemIdentityWrite(ctx); svcErr != nil {
		return nil, svcErr
	}
	if svcErr := validateKind(kind); svcErr != nil {
		return nil, svcErr
	}
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("hyperfleet.resource_type", registry.MustGet(kind).Plural))

	var result *api.Resource
	var outcome *reconciliationOutcome
	err := s.txRunner.Do(ctx, func(ctx context.Context) error {
		resource, err := s.resourceDao.GetForUpdate(ctx, kind, id)
		if err != nil {
			return handleGetError(kind, id, err)
		}

		if resource.DeletedTime != nil {
			return errors.ConflictState("%s '%s' is marked for deletion", kind, id)
		}

		oldSpec := append([]byte(nil), resource.Spec...)
		oldLabels := resource.Labels

		if applyErr := applyResourcePatch(resource, patch); applyErr != nil {
			return errors.Validation("Invalid patch data: %v", applyErr)
		}

		specChanged := !jsonBytesEqual(oldSpec, resource.Spec)
		labelsChanged := !labelsEqual(oldLabels, resource.Labels)
		refsChanged := patch.References != nil

		// Validate and persist references when the patch includes them (nil = skip, {} = clear).
		if refsChanged {
			if svcErr := s.validateReferences(ctx, kind, patch.References); svcErr != nil {
				return svcErr
			}
			refRows := convertRefs(kind, resource.ID, patch.References)
			if refErr := s.resourceDao.ReplaceReferences(ctx, resource.ID, refRows); refErr != nil {
				return errors.GeneralError("failed to save references: %s", refErr)
			}
			resource.References = refRows
		}

		if !specChanged && !labelsChanged && !refsChanged {
			result = resource
			return nil
		}

		resource.IncrementGeneration()
		resource.UpdatedBy = actorFromContext(ctx)

		if saveErr := s.resourceDao.Save(ctx, resource); saveErr != nil {
			return handleUpdateError(kind, saveErr)
		}

		if labelsChanged {
			if labelErr := s.resourceLabelDao.ReplaceLabels(ctx, resource.ID, resource.Labels); labelErr != nil {
				return handleUpdateError(kind, labelErr)
			}
		}

		// Recompute conditions after generation change.
		desc := registry.MustGet(kind)
		if len(desc.RequiredAdapters) > 0 {
			adapterStatuses, statusErr := s.adapterStatusDao.FindByResource(ctx, kind, resource.ID)
			if statusErr != nil {
				return errors.GeneralError("failed to get adapter statuses for condition recompute: %s", statusErr)
			}
			recomputed, svcErr := s.recomputeAndSaveResourceConditions(ctx, resource, adapterStatuses)
			if svcErr != nil {
				return svcErr
			}
			outcome = recomputed
		}
		result = resource
		return nil
	})
	if svcErr := serviceErrorFromTransaction(err); svcErr != nil {
		return nil, svcErr
	}
	if outcome != nil {
		metrics.RecordReconciliationStarted(outcome.kind, outcome.isDelete)
	}
	return result, nil
}

// Resources with required adapters are soft-deleted; all others are hard-deleted.
func (s *sqlResourceService) Delete(ctx context.Context, kind, id string) (*api.Resource, *errors.ServiceError) {
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("hyperfleet.resource_id", id))
	if svcErr := rejectSystemIdentityWrite(ctx); svcErr != nil {
		return nil, svcErr
	}
	if svcErr := validateKind(kind); svcErr != nil {
		return nil, svcErr
	}
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("hyperfleet.resource_type", registry.MustGet(kind).Plural))

	var result *api.Resource
	var outcomes []reconciliationOutcome
	err := s.txRunner.Do(ctx, func(ctx context.Context) error {
		resource, err := s.resourceDao.GetForUpdate(ctx, kind, id)
		if err != nil {
			return handleSoftDeleteError(kind, err)
		}

		deletedBy := actorFromContext(ctx)
		deletedAt := time.Now().UTC().Truncate(time.Microsecond)

		// Mark for deletion if not already soft-deleted.
		if resource.DeletedTime == nil {
			resource.MarkDeleted(deletedBy, deletedAt)
			resource.IncrementGeneration()
		}

		deleteOutcomes, svcErr := s.deleteResourceTree(ctx, resource, deletedBy, deletedAt)
		if svcErr != nil {
			return svcErr
		}
		outcomes = append(outcomes, deleteOutcomes...)
		result = resource
		return nil
	})
	if svcErr := serviceErrorFromTransaction(err); svcErr != nil {
		return nil, svcErr
	}
	for _, outcome := range outcomes {
		metrics.RecordReconciliationStarted(outcome.kind, outcome.isDelete)
	}
	return result, nil
}

// deleteResourceTree enforces child delete policies then persists bottom-up.
func (s *sqlResourceService) deleteResourceTree(
	ctx context.Context, resource *api.Resource,
	deletedBy string, deletedAt time.Time,
) ([]reconciliationOutcome, *errors.ServiceError) {
	var outcomes []reconciliationOutcome
	children := registry.ChildrenOf(resource.Kind)

	for _, child := range children {
		if child.OnParentDelete == registry.OnParentDeleteRestrict {
			if svcErr := s.checkCanDelete(ctx, resource, child); svcErr != nil {
				return nil, svcErr
			}
		}
	}

	for _, child := range children {
		if child.OnParentDelete == registry.OnParentDeleteCascade {
			items, err := s.resourceDao.FindByKindAndOwnerForUpdate(ctx, child.Kind, resource.ID)
			if err != nil {
				return nil, errors.GeneralError(
					"Unable to find %s children for cascade delete: %s", child.Kind, err,
				)
			}
			for _, item := range items {
				if item.DeletedTime == nil {
					item.MarkDeleted(deletedBy, deletedAt)
					item.IncrementGeneration()
				}
				childOutcomes, svcErr := s.deleteResourceTree(ctx, item, deletedBy, deletedAt)
				if svcErr != nil {
					return nil, svcErr
				}
				outcomes = append(outcomes, childOutcomes...)
			}
		}
	}

	// Check if other resources reference this one before any deletion.
	referencers, refErr := s.resourceDao.FindReferencers(ctx, resource.ID)
	if refErr != nil {
		return nil, errors.GeneralError("failed to check references: %s", refErr)
	}
	// List out all the references for a specific resource
	if len(referencers) > 0 {
		names := make([]string, len(referencers))
		for i, r := range referencers {
			names[i] = fmt.Sprintf("%s %q", r.Kind, r.Name)
		}
		return nil, errors.ConflictState(
			"cannot delete %s %q: referenced by %s — remove the reference(s) before deleting",
			resource.Kind, resource.Name, strings.Join(names, ", "))
	}

	shouldSoftDelete, svcErr := s.shouldSoftDelete(ctx, resource, children)
	if svcErr != nil {
		return nil, svcErr
	}

	if shouldSoftDelete {
		if saveErr := s.resourceDao.Save(ctx, resource); saveErr != nil {
			return nil, handleSoftDeleteError(resource.Kind, saveErr)
		}
		// Soft-delete should clear all resource references to satisfy the ON DELETE RESTRICT FK constraint.
		if err := s.resourceDao.ReplaceReferences(ctx, resource.ID, nil); err != nil {
			return nil, errors.GeneralError("failed to clear outbound references on soft-delete: %s", err)
		}
		// Recompute conditions — generation incremented, Reconciled must flip to False.
		adapterStatuses, statusErr := s.adapterStatusDao.FindByResource(ctx, resource.Kind, resource.ID)
		if statusErr != nil {
			return nil, errors.GeneralError("failed to get adapter statuses for condition recompute: %s", statusErr)
		}
		outcome, svcErr := s.recomputeAndSaveResourceConditions(ctx, resource, adapterStatuses)
		if svcErr != nil {
			return nil, svcErr
		}
		if outcome != nil {
			outcomes = append(outcomes, *outcome)
		}
		return outcomes, nil
	}

	if err := s.resourceDao.Delete(ctx, resource.Kind, resource.ID); err != nil {
		return nil, handleDeleteError(resource.Kind, err)
	}

	return outcomes, nil
}

// shouldSoftDelete determines whether a resource requires soft-deletion.
// Soft-delete is required when:
// 1. Resource has RequiredAdapters (must wait for adapter finalization)
// 2. Resource has soft-deleted children (parent must remain until children are gone)
func (s *sqlResourceService) shouldSoftDelete(
	ctx context.Context, resource *api.Resource, children []registry.EntityDescriptor,
) (bool, *errors.ServiceError) {
	desc := registry.MustGet(resource.Kind)

	// Reason 1: Resource has RequiredAdapters
	if len(desc.RequiredAdapters) > 0 {
		return true, nil
	}

	// Reason 2: Resource has soft-deleted children
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
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("hyperfleet.resource_id", id))
	if svcErr := validateKind(kind); svcErr != nil {
		return nil, svcErr
	}
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("hyperfleet.resource_type", registry.MustGet(kind).Plural))
	resource, err := s.resourceDao.GetByOwner(ctx, kind, id, ownerID)
	if err != nil {
		return nil, handleGetError(kind, id, err)
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
	scopedArgs.Preloads = append(append([]string(nil), scopedArgs.Preloads...), "Labels", "Conditions", "References")
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
	scopedArgs.Preloads = append(append([]string(nil), scopedArgs.Preloads...), "Labels", "Conditions", "References")
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
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("hyperfleet.resource_id", id))
	resource, err := s.resourceDao.GetByID(ctx, id)
	if err != nil {
		return nil, handleGetError("Resource", id, err)
	}
	desc, ok := registry.Get(resource.Kind)
	if ok {
		trace.SpanFromContext(ctx).SetAttributes(
			attribute.String("hyperfleet.resource_type", desc.Plural),
		)
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
	scopedArgs.Preloads = append(append([]string(nil), scopedArgs.Preloads...), "Labels", "Conditions", "References")
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

// ProcessAdapterStatus validates, upserts an adapter status report, and triggers
// status aggregation for a generic resource. Follows a 4-DB-call pattern:
//  1. GetForUpdate        — lock + fetch resource with conditions
//  2. FindByResource      — all adapter statuses (existing found in-memory)
//  3. Upsert              — write adapter status
//  4. UpdateConditions    — write aggregated conditions (if changed)
func (s *sqlResourceService) ProcessAdapterStatus(
	ctx context.Context, kind, resourceID string, adapterStatus *api.AdapterStatus,
) (*api.AdapterStatus, *errors.ServiceError) {
	ctx = hfl.WithResourceType(ctx, kind)
	ctx = hfl.WithResourceID(ctx, resourceID)
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("hyperfleet.resource_id", resourceID))
	if svcErr := validateKind(kind); svcErr != nil {
		return nil, svcErr
	}
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("hyperfleet.resource_type", registry.MustGet(kind).Plural))
	adapterStatus.LastReportTime = adapterStatus.LastReportTime.Truncate(time.Microsecond)

	var result *api.AdapterStatus
	var outcome *reconciliationOutcome
	err := s.txRunner.Do(ctx, func(ctx context.Context) error {
		resource, err := s.resourceDao.GetForUpdate(ctx, kind, resourceID)
		if err != nil {
			return handleGetError(kind, resourceID, err)
		}
		allStatuses, err := s.adapterStatusDao.FindByResource(ctx, kind, resourceID)
		if err != nil {
			return errors.GeneralError("Failed to get adapter statuses: %s", err)
		}

		existingStatus := findAdapterStatusInList(allStatuses, adapterStatus.Adapter)
		conditions, triggerAggregation, svcErr := validateAndClassifyAdapterStatus(
			resource.Generation, adapterStatus, existingStatus, ctx,
		)
		if svcErr != nil {
			return svcErr
		}
		if conditions == nil && !triggerAggregation {
			return nil
		}

		adapterStatus.ResourceType = kind
		adapterStatus.ResourceID = resourceID
		setConditionTransitionTimes(adapterStatus, existingStatus)
		upsertedStatus, err := s.adapterStatusDao.Upsert(ctx, adapterStatus, existingStatus)
		if err != nil {
			return handleCreateError("AdapterStatus", err)
		}
		updatedStatuses := replaceAdapterStatusInList(allStatuses, upsertedStatus)

		if resource.DeletedTime != nil {
			hardDeleted, hdErr := s.tryHardDeleteResource(ctx, resource, conditions, updatedStatuses)
			if hdErr != nil {
				return hdErr
			}
			if hardDeleted {
				result = upsertedStatus
				return nil
			}
		}

		hasMapper := s.conditionMappers[resource.Kind] != nil
		if triggerAggregation || (hasMapper && (existingStatus == nil ||
			!jsonEqual(existingStatus.Conditions, adapterStatus.Conditions) ||
			!jsonEqual(existingStatus.Data, adapterStatus.Data))) {
			recomputed, aggregateErr := s.recomputeAndSaveResourceConditions(
				ctx, resource, updatedStatuses,
			)
			if aggregateErr != nil {
				return aggregateErr
			}
			outcome = recomputed
		}

		result = upsertedStatus
		return nil
	})
	if svcErr := serviceErrorFromTransaction(err); svcErr != nil {
		return nil, svcErr
	}
	if outcome != nil {
		metrics.RecordReconciliationStarted(outcome.kind, outcome.isDelete)
	}
	return result, nil
}

// recomputeAndSaveResourceConditions runs AggregateResourceStatus and persists
// the result to the resource_conditions table. Skips the write when conditions
// are unchanged.
func (s *sqlResourceService) recomputeAndSaveResourceConditions(
	ctx context.Context,
	resource *api.Resource,
	adapterStatuses api.AdapterStatusList,
) (*reconciliationOutcome, *errors.ServiceError) {
	desc := registry.MustGet(resource.Kind)

	// Convert the GORM association ([]ResourceCondition) to JSON so it can be
	// passed to AggregateResourceStatus via PrevConditionsJSON. This is needed
	// because the aggregation function uses the previous conditions to preserve
	// LastTransitionTime and the sticky LastKnownReconciled condition.
	var prevConditionsJSON []byte
	if len(resource.Conditions) > 0 {
		var marshalErr error
		prevConditionsJSON, marshalErr = json.Marshal(resource.Conditions)
		if marshalErr != nil {
			return nil, errors.GeneralError("Failed to marshal previous conditions: %s", marshalErr)
		}
	}
	prevReconciledStatus := extractPrevReconciledStatus(ctx, prevConditionsJSON)

	// During deletion, check if child resources still exist. The aggregation
	// function uses this to prevent premature Reconciled=True on a parent
	// whose children haven't finished their own reconciliation.
	hasChildResources := false
	if resource.DeletedTime != nil {
		var err error
		hasChildResources, err = s.hasActiveChildren(ctx, resource)
		if err != nil {
			return nil, errors.GeneralError("Failed to check children for status aggregation: %s", err)
		}
	}

	// Use UpdatedTime as the reference time for aggregation. Falls back to
	// CreatedTime for resources that haven't been patched yet.
	refTime := resource.UpdatedTime
	if refTime.IsZero() {
		refTime = resource.CreatedTime
	}

	reconciled, lastKnownReconciled, adapterConditions := AggregateResourceStatus(
		ctx, AggregateResourceStatusInput{
			ResourceGeneration: resource.Generation,
			RefTime:            refTime,
			DeletedTime:        resource.DeletedTime,
			PrevConditionsJSON: prevConditionsJSON,
			RequiredAdapters:   desc.RequiredAdapters,
			AdapterStatuses:    adapterStatuses,
			HasChildResources:  hasChildResources,
		},
	)

	// Build the full conditions slice: Reconciled + LastKnownReconciled + per-adapter + mapped conditions.
	mapper := s.conditionMappers[resource.Kind]
	var mappedCapacity int
	if mapper != nil {
		mappedCapacity = len(mapper.sortedNames)
	}
	newConditions := make([]api.ResourceCondition, 0, fixedConditionCount+len(adapterConditions)+mappedCapacity)
	newConditions = append(newConditions, reconciled, lastKnownReconciled)
	newConditions = append(newConditions, adapterConditions...)

	// Apply CEL condition mapping if configured
	if mapper != nil {
		mappedConditions, err := mapper.Apply(ctx, ApplyInput{
			AdapterStatuses: adapterStatuses,
			Resource:        resource,
			RefTime:         refTime,
			PrevConditions:  resource.Conditions, // Preserve timestamps from previous conditions
		})
		if err != nil {
			return nil, errors.GeneralError("Condition mapping failed: %s", err)
		}
		newConditions = append(newConditions, mappedConditions...)
	}

	// Compare via JSON to detect actual changes.
	newJSON, marshalErr := json.Marshal(newConditions)
	if marshalErr != nil {
		return nil, errors.GeneralError("Failed to marshal conditions: %s", marshalErr)
	}
	if jsonEqual(prevConditionsJSON, newJSON) {
		return nil, nil
	}

	// Write to resource_conditions table (not JSONB on the resource row).
	if err := s.resourceConditionDao.UpdateConditions(ctx, resource.ID, newConditions); err != nil {
		return nil, errors.GeneralError("Failed to update resource conditions: %s", err)
	}

	// Update the in-memory resource so callers see the new conditions.
	resource.Conditions = newConditions

	// Return reconciliation work for the public mutation to emit after commit.
	if reconciled.Status == api.ConditionFalse &&
		(prevReconciledStatus == nil || *prevReconciledStatus != api.ConditionFalse) {
		return &reconciliationOutcome{kind: resource.Kind, isDelete: resource.DeletedTime != nil}, nil
	}

	return nil, nil
}

// tryHardDeleteResource checks whether all required adapters have reported
// Finalized=True for a soft-deleted resource and no children remain, then
// permanently removes the resource and its adapter statuses/conditions.
func (s *sqlResourceService) tryHardDeleteResource(
	ctx context.Context,
	resource *api.Resource,
	conditions []api.AdapterCondition,
	allStatuses api.AdapterStatusList,
) (bool, *errors.ServiceError) {
	resourceCtx := hfl.WithResourceType(ctx, resource.Kind)
	resourceCtx = hfl.WithResourceID(resourceCtx, resource.ID)
	// Quick check: does the incoming report contain Finalized=True?
	// If not, hard-delete is not possible regardless of other adapters.
	if !incomingReportedFinalized(conditions) {
		return false, nil
	}

	// Check that ALL required adapters (not just this one) have reported
	// Finalized=True at the current generation.
	desc := registry.MustGet(resource.Kind)
	if !allAdaptersFinalized(desc.RequiredAdapters, allStatuses, resource.Generation) {
		return false, nil
	}

	// Ensure no children exist (active or soft-deleted) — hard-deleting a
	// parent with remaining children would leave orphaned resources.
	hasActive, err := s.hasActiveChildren(ctx, resource)
	if err != nil {
		return false, errors.GeneralError("Failed to check children during hard-delete: %s", err)
	}
	if hasActive {
		return false, nil
	}

	children := registry.ChildrenOf(resource.Kind)
	if len(children) > 0 {
		childKinds := make([]string, len(children))
		for i, c := range children {
			childKinds[i] = c.Kind
		}
		hasSoftDeleted, sdErr := s.resourceDao.ExistsSoftDeletedByOwner(ctx, childKinds, resource.ID)
		if sdErr != nil {
			return false, errors.GeneralError("Failed to check soft-deleted children during hard-delete: %s", sdErr)
		}
		if hasSoftDeleted {
			return false, nil
		}
	}

	// All checks passed — clean up associated data and hard-delete the resource.
	// Order matters: adapter statuses and conditions must be removed before the
	// resource row, since they reference it.
	if err := s.adapterStatusDao.DeleteByResource(resourceCtx, resource.Kind, resource.ID); err != nil {
		return false, errors.GeneralError("Failed to delete adapter statuses during hard-delete: %s", err)
	}
	if err := s.resourceConditionDao.DeleteByResource(resourceCtx, resource.ID); err != nil {
		return false, errors.GeneralError("Failed to delete resource conditions during hard-delete: %s", err)
	}
	if err := s.resourceDao.Delete(resourceCtx, resource.Kind, resource.ID); err != nil {
		return false, errors.GeneralError("Failed to hard-delete %s: %s", resource.Kind, err)
	}
	slog.InfoContext(resourceCtx, "Hard-deleted resource after all required adapters reported Finalized=True")

	return true, nil
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
func validateName(kind, name string) *errors.ServiceError {
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

// ForceDelete hard-deletes a resource tree stuck in the Finalizing state,
// bypassing adapter finalization but still refusing to drop references required
// by surviving resources. Deadlocks return 409 so callers can retry the request.
func (s *sqlResourceService) ForceDelete(ctx context.Context, kind, id, reason string) *errors.ServiceError {
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("hyperfleet.resource_id", id))
	if svcErr := rejectSystemIdentityWrite(ctx); svcErr != nil {
		return svcErr
	}
	if svcErr := validateKind(kind); svcErr != nil {
		return svcErr
	}
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("hyperfleet.resource_type", registry.MustGet(kind).Plural))

	var deleted api.ResourceList
	err := s.txRunner.Do(ctx, func(txCtx context.Context) error {
		resources, svcErr := s.forceDeleteTx(txCtx, kind, id, reason)
		if svcErr != nil {
			return svcErr
		}
		deleted = resources
		return nil
	})
	if err != nil {
		resourceCtx := hfl.WithResourceID(hfl.WithResourceType(ctx, kind), id)
		slog.ErrorContext(resourceCtx, "Force-delete failed",
			"caller", actorFromContext(ctx), "reason", reason, "error", err)
		if svcErr := forceDeleteTransactionConflict(err); svcErr != nil {
			return svcErr
		}
		return serviceErrorFromTransaction(err)
	}
	for _, item := range deleted {
		resourceCtx := hfl.WithResourceID(hfl.WithResourceType(ctx, item.Kind), item.ID)
		slog.InfoContext(resourceCtx, "Force-deleted resource", "caller", actorFromContext(ctx), "reason", reason)
	}
	return nil
}

// forceDeleteTx locks the root, requires the Finalizing state, collects and locks
// the scoped tree, enforces remaining reference minimums, then hard-deletes the
// tree. It runs inside the caller's transaction and maps DAO failures to terminal
// ServiceErrors. The FOR UPDATE lock on surviving sources can rarely deadlock when
// two cyclically-referencing resources are force-deleted at once; the victim
// transaction rolls back and the deadlock surfaces as a 409 conflict.
func (s *sqlResourceService) forceDeleteTx(
	ctx context.Context, kind, id, reason string,
) (api.ResourceList, *errors.ServiceError) {
	resource, err := s.resourceDao.GetRowForUpdate(ctx, kind, id)
	if err != nil {
		if svcErr := forceDeleteTransactionConflict(err); svcErr != nil {
			return nil, svcErr
		}
		return nil, handleGetError(kind, id, err)
	}
	if resource.DeletedTime == nil {
		return nil, errors.ConflictState("%s '%s' is not in Finalizing state", kind, id)
	}

	resources, err := s.collectForceDeletionResources(ctx, resource)
	if err != nil {
		if svcErr := forceDeleteTransactionConflict(err); svcErr != nil {
			return nil, svcErr
		}
		return nil, errors.GeneralError("force-delete failed: %s", err)
	}
	ids := make([]string, len(resources))
	for i, item := range resources {
		ids[i] = item.ID
	}

	counts, err := s.resourceDao.FindExternalReferenceCounts(ctx, ids, ids)
	if err != nil {
		if svcErr := forceDeleteTransactionConflict(err); svcErr != nil {
			return nil, svcErr
		}
		return nil, errors.GeneralError("force-delete failed: %s", err)
	}
	if svcErr := validateRemainingReferenceMins(kind, resource.Name, counts); svcErr != nil {
		return nil, svcErr
	}

	// Record intent outside the database transaction's rollback semantics, before
	// the first destructive statement. This is not a claim of successful deletion.
	type auditResource struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	}
	subresources := make([]auditResource, 0, len(resources)-1)
	for _, item := range resources[1:] {
		subresources = append(subresources, auditResource{Kind: item.Kind, ID: item.ID})
	}
	resourceCtx := hfl.WithResourceID(hfl.WithResourceType(ctx, kind), id)
	slog.InfoContext(resourceCtx, "Force-deleting resource",
		"caller", actorFromContext(ctx), "reason", reason, "subresources", subresources)

	if err := s.deleteResourceRows(ctx, ids); err != nil {
		if svcErr := forceDeleteTransactionConflict(err); svcErr != nil {
			return nil, svcErr
		}
		return nil, handleDeleteError(kind, err)
	}
	return resources, nil
}

func forceDeleteTransactionConflict(err error) *errors.ServiceError {
	if sqlErr, ok := stderrors.AsType[interface {
		error
		SQLState() string
	}](err); ok {
		if sqlErr.SQLState() == "40P01" {
			return errors.ConflictState("force-delete conflicted with a concurrent transaction; retry the request")
		}
	}
	return nil
}

// deleteResourceRows removes a collected force-delete tree in FK-safe order:
// inbound references first, then adapter statuses, then the resource rows. It
// runs in the caller's transaction. Resource rows stay tenant-scoped via
// DeleteIDs; the reference and adapter-status deletes are keyed by the already
// tenant-scoped, locked IDs. The calls live in the service rather than in a
// single resource-DAO method because adapter statuses are owned by
// AdapterStatusDao, which the resource DAO does not delete from.
func (s *sqlResourceService) deleteResourceRows(ctx context.Context, ids []string) error {
	if err := s.resourceDao.DeleteReferencesByTargets(ctx, ids); err != nil {
		return err
	}
	if err := s.adapterStatusDao.DeleteByResourceIDs(ctx, ids); err != nil {
		return err
	}
	return s.resourceDao.DeleteIDs(ctx, ids)
}

// collectForceDeletionResources locks the complete scoped tree parent-first.
func (s *sqlResourceService) collectForceDeletionResources(
	ctx context.Context, root *api.Resource,
) (api.ResourceList, error) {
	resources := api.ResourceList{root}
	var collect func(*api.Resource) error
	collect = func(parent *api.Resource) error {
		children, err := s.resourceDao.FindChildrenForUpdate(ctx, parent.ID)
		if err != nil {
			return err
		}
		for _, child := range children {
			resources = append(resources, child)
			if err := collect(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := collect(root); err != nil {
		return nil, err
	}
	return resources, nil
}

func validateRemainingReferenceMins(
	kind, name string, counts []dao.ExternalReferenceCount,
) *errors.ServiceError {
	for _, count := range counts {
		desc, registered := registry.Get(count.SourceKind)
		if !registered {
			return errors.GeneralError("unregistered reference source kind %s", count.SourceKind)
		}
		ref, ok := findReferenceDescriptor(desc.References, count.RefType)
		if !ok {
			return errors.GeneralError(
				"unregistered reference type %s for %s", count.RefType, count.SourceKind,
			)
		}
		if count.Remaining < int64(ref.Min) {
			return errors.ConflictState(
				"cannot delete %s %q: required references would be removed", kind, name,
			)
		}
	}
	return nil
}

// findReferenceDescriptor returns the descriptor for refType, or false if the
// entity does not declare that reference type.
func findReferenceDescriptor(
	refs []registry.ReferenceDescriptor, refType string,
) (registry.ReferenceDescriptor, bool) {
	for _, ref := range refs {
		if ref.RefType == refType {
			return ref, true
		}
	}
	return registry.ReferenceDescriptor{}, false
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
