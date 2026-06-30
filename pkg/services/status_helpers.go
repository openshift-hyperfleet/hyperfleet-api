package services

import (
	"context"
	"encoding/json"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/metrics"
)

func extractPrevReconciledStatus(ctx context.Context, raw []byte) *api.ResourceConditionStatus {
	prevReconciled, _, _ := parsePrevConditions(ctx, raw)
	if prevReconciled == nil {
		return nil
	}
	return &prevReconciled.Status
}

// computeNodePoolConditionsJSON aggregates adapter statuses into marshaled conditions JSON.
// Returns (nil, false, nil) if conditions are unchanged relative to np.StatusConditions.
// The reconciliationStarted flag indicates a Reconciled→False transition occurred;
// callers must emit the metric only after the change is persisted.
func computeNodePoolConditionsJSON(
	ctx context.Context,
	np *api.NodePool,
	adapterStatuses []*api.AdapterStatus,
	requiredAdapters []string,
) (conditionsJSON []byte, reconciliationStarted bool, svcErr *errors.ServiceError) {
	prevReconciledStatus := extractPrevReconciledStatus(ctx, np.StatusConditions)

	reconciled, lastKnownReconciled, adapterConditions := AggregateResourceStatus(ctx, AggregateResourceStatusInput{
		ResourceGeneration: np.Generation,
		RefTime:            nodePoolRefTime(np),
		DeletedTime:        np.DeletedTime,
		PrevConditionsJSON: np.StatusConditions,
		RequiredAdapters:   requiredAdapters,
		AdapterStatuses:    adapterStatuses,
	})

	reconciliationStarted = reconciled.Status == api.ConditionFalse &&
		(prevReconciledStatus == nil || *prevReconciledStatus != api.ConditionFalse)

	allConditions := make([]api.ResourceCondition, 0, fixedConditionCount+len(adapterConditions))
	allConditions = append(allConditions, reconciled, lastKnownReconciled)
	allConditions = append(allConditions, adapterConditions...)

	result, err := json.Marshal(allConditions)
	if err != nil {
		return nil, false, errors.GeneralError("Failed to marshal conditions: %s", err)
	}

	if jsonEqual(np.StatusConditions, result) {
		return nil, false, nil
	}

	return result, reconciliationStarted, nil
}

// updateNodePoolStatusFromAdapters fetches a single nodepool by ID, recomputes its status
// conditions from current adapter reports, and persists the result via Save.
// Returns the updated nodepool unchanged if conditions have not changed.
func updateNodePoolStatusFromAdapters(
	ctx context.Context,
	nodePoolID string,
	nodePoolDao dao.NodePoolDao,
	adapterStatusDao dao.AdapterStatusDao,
	adapterConfig *config.AdapterRequirementsConfig,
) (*api.NodePool, *errors.ServiceError) {
	nodePool, err := nodePoolDao.Get(ctx, nodePoolID)
	if err != nil {
		return nil, handleGetError(api.ResourceTypeNodePool, "id", nodePoolID, err)
	}

	adapterStatuses, err := adapterStatusDao.FindByResource(ctx, api.ResourceTypeNodePool, nodePoolID)
	if err != nil {
		return nil, errors.GeneralError("Failed to get adapter statuses: %s", err)
	}

	conditionsJSON, started, svcErr := computeNodePoolConditionsJSON(
		ctx,
		nodePool,
		adapterStatuses,
		adapterConfig.RequiredNodePoolAdapters())
	if svcErr != nil {
		return nil, svcErr
	}
	if conditionsJSON == nil {
		return nodePool, nil
	}

	nodePool.StatusConditions = conditionsJSON
	if err = nodePoolDao.Save(ctx, nodePool); err != nil {
		return nil, handleUpdateError(api.ResourceTypeNodePool, err)
	}

	if started {
		metrics.RecordReconciliationStarted("nodepool", nodePool.DeletedTime != nil)
	}

	return nodePool, nil
}

// recomputeNodePoolConditions fetches adapter statuses and recomputes status conditions
// for each nodepool, setting StatusConditions directly on the models.
func recomputeNodePoolConditions(
	ctx context.Context,
	nodePools []*api.NodePool,
	adapterStatusDao dao.AdapterStatusDao,
	adapterConfig *config.AdapterRequirementsConfig,
) (reconciliationStartedCount int, svcErr *errors.ServiceError) {
	if len(nodePools) == 0 {
		return 0, nil
	}

	nodePoolIDs := make([]string, len(nodePools))
	for i, np := range nodePools {
		nodePoolIDs[i] = np.ID
	}

	allStatuses, err := adapterStatusDao.FindByResourceIDs(ctx, api.ResourceTypeNodePool, nodePoolIDs)
	if err != nil {
		return 0, errors.GeneralError("Failed to get adapter statuses: %s", err)
	}

	statusesByResource := make(map[string][]*api.AdapterStatus)
	for i := range allStatuses {
		s := allStatuses[i]
		statusesByResource[s.ResourceID] = append(statusesByResource[s.ResourceID], s)
	}

	requiredAdapters := adapterConfig.RequiredNodePoolAdapters()

	var startedCount int
	for _, np := range nodePools {
		conditionsJSON, started, svcErr := computeNodePoolConditionsJSON(ctx, np, statusesByResource[np.ID], requiredAdapters)
		if svcErr != nil {
			return startedCount, svcErr
		}
		if conditionsJSON != nil {
			np.StatusConditions = conditionsJSON
		}
		if started {
			startedCount++
		}
	}

	return startedCount, nil
}
