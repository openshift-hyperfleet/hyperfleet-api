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

func updateNodePoolStatusFromAdapters(
	ctx context.Context,
	nodePoolID string,
	nodePoolDao dao.NodePoolDao,
	adapterStatusDao dao.AdapterStatusDao,
	adapterConfig *config.AdapterRequirementsConfig,
) (*api.NodePool, *errors.ServiceError) {
	// Get the nodepool
	nodePool, err := nodePoolDao.Get(ctx, nodePoolID)
	if err != nil {
		return nil, handleGetError("NodePool", "id", nodePoolID, err)
	}

	// Get adapter statuses
	adapterStatuses, err := adapterStatusDao.FindByResource(ctx, "NodePool", nodePoolID)
	if err != nil {
		return nil, errors.GeneralError("Failed to get adapter statuses: %s", err)
	}

	// Compute reference time
	refTime := nodePoolRefTime(nodePool)

	// Aggregate status
	ready, available, adapterConditions := AggregateResourceStatus(ctx, AggregateResourceStatusInput{
		ResourceGeneration: nodePool.Generation,
		RefTime:            refTime,
		PrevConditionsJSON: nodePool.StatusConditions,
		RequiredAdapters:   adapterConfig.RequiredNodePoolAdapters(),
		AdapterStatuses:    adapterStatuses,
	})

	// Build combined conditions
	allConditions := make([]api.ResourceCondition, 0, 2+len(adapterConditions))
	allConditions = append(allConditions, ready, available)
	allConditions = append(allConditions, adapterConditions...)

	// Marshal conditions
	conditionsJSON, err := json.Marshal(allConditions)
	if err != nil {
		return nil, errors.GeneralError("Failed to marshal conditions: %s", err)
	}

	// Short-circuit if unchanged
	if bytes.Equal(nodePool.StatusConditions, conditionsJSON) {
		return nodePool, nil
	}

	// Update and persist
	nodePool.StatusConditions = conditionsJSON
	nodePool, err = nodePoolDao.Replace(ctx, nodePool)
	if err != nil {
		return nil, handleUpdateError("NodePool", err)
	}

	return nodePool, nil
}
