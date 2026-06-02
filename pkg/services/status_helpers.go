package services

import (
	"context"
	"encoding/json"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

// computeNodePoolConditionsJSON aggregates adapter statuses into marshaled conditions JSON.
// Returns nil if conditions are unchanged relative to np.StatusConditions.
func computeNodePoolConditionsJSON(
	ctx context.Context,
	np *api.NodePool,
	adapterStatuses []*api.AdapterStatus,
	requiredAdapters []string,
) ([]byte, *errors.ServiceError) {
	reconciled, lastKnownReconciled, adapterConditions := AggregateResourceStatus(ctx, AggregateResourceStatusInput{
		ResourceGeneration: np.Generation,
		RefTime:            nodePoolRefTime(np),
		DeletedTime:        np.DeletedTime,
		PrevConditionsJSON: np.StatusConditions,
		RequiredAdapters:   requiredAdapters,
		AdapterStatuses:    adapterStatuses,
	})

	allConditions := make([]api.ResourceCondition, 0, fixedConditionCount+len(adapterConditions))
	allConditions = append(allConditions, reconciled, lastKnownReconciled)
	allConditions = append(allConditions, adapterConditions...)

	conditionsJSON, err := json.Marshal(allConditions)
	if err != nil {
		return nil, errors.GeneralError("Failed to marshal conditions: %s", err)
	}

	if jsonEqual(np.StatusConditions, conditionsJSON) {
		return nil, nil
	}

	return conditionsJSON, nil
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

	conditionsJSON, svcErr := computeNodePoolConditionsJSON(
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

	return nodePool, nil
}

// recomputeNodePoolConditions fetches adapter statuses and recomputes status conditions
// for each nodepool, setting StatusConditions directly on the models.
func recomputeNodePoolConditions(
	ctx context.Context,
	nodePools []*api.NodePool,
	adapterStatusDao dao.AdapterStatusDao,
	adapterConfig *config.AdapterRequirementsConfig,
) *errors.ServiceError {
	if len(nodePools) == 0 {
		return nil
	}

	nodePoolIDs := make([]string, len(nodePools))
	for i, np := range nodePools {
		nodePoolIDs[i] = np.ID
	}

	allStatuses, err := adapterStatusDao.FindByResourceIDs(ctx, api.ResourceTypeNodePool, nodePoolIDs)
	if err != nil {
		return errors.GeneralError("Failed to get adapter statuses: %s", err)
	}

	statusesByResource := make(map[string][]*api.AdapterStatus)
	for i := range allStatuses {
		s := allStatuses[i]
		statusesByResource[s.ResourceID] = append(statusesByResource[s.ResourceID], s)
	}

	requiredAdapters := adapterConfig.RequiredNodePoolAdapters()

	for _, np := range nodePools {
		conditionsJSON, svcErr := computeNodePoolConditionsJSON(ctx, np, statusesByResource[np.ID], requiredAdapters)
		if svcErr != nil {
			return svcErr
		}
		if conditionsJSON != nil {
			np.StatusConditions = conditionsJSON
		}
	}

	return nil
}
