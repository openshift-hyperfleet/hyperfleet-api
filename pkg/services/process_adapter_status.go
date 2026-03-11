package services

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// processAdapterStatus contains the shared pre-processing pipeline for adapter status updates.
// resourceType is the resource kind (e.g. "Cluster", "NodePool"); resourceID is the resource's ID.
// getGeneration fetches the current resource generation (used for P1 check).
// triggerAggregation is called fire-and-forget after a successful upsert; callers handle errors.
func processAdapterStatus(
	ctx context.Context,
	resourceType, resourceID string,
	adapterStatus *api.AdapterStatus,
	adapterStatusDao dao.AdapterStatusDao,
	getGeneration func(ctx context.Context) (int32, *errors.ServiceError),
	triggerAggregation func(ctx context.Context, observedTime time.Time),
) (*api.AdapterStatus, *errors.ServiceError) {
	existingStatus, findErr := adapterStatusDao.FindByResourceAndAdapter(
		ctx, resourceType, resourceID, adapterStatus.Adapter,
	)
	if findErr != nil && !stderrors.Is(findErr, gorm.ErrRecordNotFound) {
		if !strings.Contains(findErr.Error(), errors.CodeNotFoundGeneric) {
			return nil, errors.GeneralError("Failed to get adapter status: %s", findErr)
		}
	}
	// Stale check: discard if older than the adapter's last recorded generation.
	if existingStatus != nil && adapterStatus.ObservedGeneration < existingStatus.ObservedGeneration {
		return nil, nil
	}

	// Parse conditions from the adapter status (needed for P2 and P3 before resource fetch).
	var conditions []api.AdapterCondition
	if len(adapterStatus.Conditions) > 0 {
		if err := json.Unmarshal(adapterStatus.Conditions, &conditions); err != nil {
			return nil, errors.GeneralError("Failed to unmarshal adapter status conditions: %s", err)
		}
	}

	// P2: validate mandatory conditions (presence and valid status values).
	if errorType, conditionName := ValidateMandatoryConditions(conditions); errorType != "" {
		logger.With(ctx, "resource_type", resourceType, "resource_id", resourceID).
			Info(fmt.Sprintf("Discarding adapter status update from %s: %s condition %s",
				adapterStatus.Adapter, errorType, conditionName))
		return nil, nil
	}

	// P3: discard if Available == Unknown (spec §2, all reports).
	for _, cond := range conditions {
		if cond.Type == api.ConditionTypeAvailable && cond.Status == api.AdapterConditionUnknown {
			logger.With(ctx, "resource_type", resourceType, "resource_id", resourceID).
				Info(fmt.Sprintf("Discarding adapter status update from %s: Available=Unknown reports are not processed",
					adapterStatus.Adapter))
			return nil, nil
		}
	}

	// P1: discard if observed_generation is ahead of the current resource generation.
	// Checked after P2/P3 to avoid unnecessary resource fetches for invalid/Unknown reports.
	generation, svcErr := getGeneration(ctx)
	if svcErr != nil {
		return nil, svcErr
	}
	if adapterStatus.ObservedGeneration > generation {
		logger.With(ctx, "resource_type", resourceType, "resource_id", resourceID).
			Info(fmt.Sprintf(
				"Discarding adapter status update from %s: observed_generation %d > resource generation %d",
				adapterStatus.Adapter, adapterStatus.ObservedGeneration, generation))
		return nil, nil
	}

	// Upsert the adapter status (complete replacement).
	upsertedStatus, applied, upsertErr := adapterStatusDao.Upsert(ctx, adapterStatus)
	if upsertErr != nil {
		return nil, handleCreateError("AdapterStatus", upsertErr)
	}
	if !applied {
		// A concurrent write with a higher generation was already stored; discard this update.
		return nil, nil
	}

	// Trigger aggregation using the adapter's observed_time for transition timestamps.
	observedTime := time.Now()
	if upsertedStatus.LastReportTime != nil {
		observedTime = *upsertedStatus.LastReportTime
	}
	triggerAggregation(ctx, observedTime)

	return upsertedStatus, nil
}

// buildAdapterResourceConditions builds a []api.ResourceCondition from adapter statuses,
// using each adapter's Available condition as the reported status.
func buildAdapterResourceConditions(adapterStatuses api.AdapterStatusList) []api.ResourceCondition {
	adapterConditions := []api.ResourceCondition{}

	for _, adapterStatus := range adapterStatuses {
		// Unmarshal Conditions from JSONB
		var conditions []api.AdapterCondition
		if err := json.Unmarshal(adapterStatus.Conditions, &conditions); err != nil {
			continue // Skip if can't unmarshal
		}

		// Find the "Available" condition
		var availableCondition *api.AdapterCondition
		for i := range conditions {
			if conditions[i].Type == api.ConditionTypeAvailable {
				availableCondition = &conditions[i]
				break
			}
		}

		if availableCondition == nil {
			// No Available condition, skip this adapter
			continue
		}

		// Convert to ResourceCondition
		condResource := api.ResourceCondition{
			Type:               MapAdapterToConditionType(adapterStatus.Adapter),
			Status:             api.ResourceConditionStatus(availableCondition.Status),
			Reason:             availableCondition.Reason,
			Message:            availableCondition.Message,
			ObservedGeneration: adapterStatus.ObservedGeneration,
			LastTransitionTime: availableCondition.LastTransitionTime,
		}

		// Set CreatedTime with nil check
		if adapterStatus.CreatedTime != nil {
			condResource.CreatedTime = *adapterStatus.CreatedTime
		}

		// Set LastUpdatedTime with nil check
		if adapterStatus.LastReportTime != nil {
			condResource.LastUpdatedTime = *adapterStatus.LastReportTime
		}

		adapterConditions = append(adapterConditions, condResource)
	}

	return adapterConditions
}
