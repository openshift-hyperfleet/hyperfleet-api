package services

import (
	"bytes"
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
	ready, available, reconciled, adapterConditions := AggregateResourceStatus(ctx, AggregateResourceStatusInput{
		ResourceGeneration: np.Generation,
		RefTime:            nodePoolRefTime(np),
		DeletedTime:        np.DeletedTime,
		PrevConditionsJSON: np.StatusConditions,
		RequiredAdapters:   requiredAdapters,
		AdapterStatuses:    adapterStatuses,
	})

	allConditions := make([]api.ResourceCondition, 0, 3+len(adapterConditions))
	allConditions = append(allConditions, ready, available, reconciled)
	allConditions = append(allConditions, adapterConditions...)

	conditionsJSON, err := json.Marshal(allConditions)
	if err != nil {
		return nil, errors.GeneralError("Failed to marshal conditions: %s", err)
	}

	if bytes.Equal(np.StatusConditions, conditionsJSON) {
		return nil, nil
	}

	return conditionsJSON, nil
}

// updateNodePoolStatusFromAdapters fetches a single nodepool by ID, recomputes its status
// conditions from current adapter reports, and persists the result via Replace.
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
		return nil, handleGetError("NodePool", "id", nodePoolID, err)
	}

	adapterStatuses, err := adapterStatusDao.FindByResource(ctx, "NodePool", nodePoolID)
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
	nodePool, err = nodePoolDao.Replace(ctx, nodePool)
	if err != nil {
		return nil, handleUpdateError("NodePool", err)
	}

	return nodePool, nil
}

// batchUpdateNodePoolStatusesFromAdapters updates status conditions for multiple nodepools.
// It's fetching all adapter statuses in one query and persisting
// all changed nodepools via UpdateStatusConditionsByIDs.
func batchUpdateNodePoolStatusesFromAdapters(
	ctx context.Context,
	nodePools []*api.NodePool,
	nodePoolDao dao.NodePoolDao,
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

	allStatuses, err := adapterStatusDao.FindByResourceIDs(ctx, "NodePool", nodePoolIDs)
	if err != nil {
		return errors.GeneralError("Failed to get adapter statuses: %s", err)
	}

	statusesByResource := make(map[string][]*api.AdapterStatus)
	for i := range allStatuses {
		s := allStatuses[i]
		statusesByResource[s.ResourceID] = append(statusesByResource[s.ResourceID], s)
	}

	updates := make(map[string][]byte)
	requiredAdapters := adapterConfig.RequiredNodePoolAdapters()

	for _, np := range nodePools {
		conditionsJSON, svcErr := computeNodePoolConditionsJSON(ctx, np, statusesByResource[np.ID], requiredAdapters)
		if svcErr != nil {
			return svcErr
		}
		if conditionsJSON != nil {
			updates[np.ID] = conditionsJSON
		}
	}

	if len(updates) > 0 {
		if err := nodePoolDao.UpdateStatusConditionsByIDs(ctx, updates); err != nil {
			return handleUpdateError("NodePool", err)
		}
	}

	return nil
}
