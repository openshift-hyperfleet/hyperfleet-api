package services

import (
	"encoding/json"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// validateAndClassifyAdapterStatus performs all stateless validation and discard-rule
// checks on an incoming adapter status. Returns the parsed conditions and whether
// aggregation should be triggered. Returns (nil, false, nil) when the update should
// be silently discarded.
//
// This is the shared implementation used by ClusterService, NodePoolService, and
// ResourceService. Callers provide the resource generation and a pre-built logger
// with entity-specific context fields.
func validateAndClassifyAdapterStatus(
	resourceGeneration int32,
	adapterStatus *api.AdapterStatus,
	existingStatus *api.AdapterStatus,
	log *logger.ContextLogger,
) ([]api.AdapterCondition, bool, *errors.ServiceError) {
	if adapterStatus.ObservedGeneration > resourceGeneration {
		log.Debug("Discarding adapter status update: future generation")
		return nil, false, nil
	}

	if existingStatus != nil && adapterStatus.ObservedGeneration < existingStatus.ObservedGeneration {
		log.Debug("Discarding adapter status update: stale generation")
		return nil, false, nil
	}

	incomingObs := AdapterObservedTime(adapterStatus)
	if incomingObs.IsZero() {
		log.Debug("Discarding adapter status update: zero observed time")
		return nil, false, nil
	}

	if existingStatus != nil && adapterStatus.ObservedGeneration == existingStatus.ObservedGeneration {
		prevObs := AdapterObservedTime(existingStatus)
		if !prevObs.IsZero() && incomingObs.Before(prevObs) {
			log.Debug("Discarding adapter status update: stale observed time")
			return nil, false, nil
		}
	}

	var conditions []api.AdapterCondition
	if len(adapterStatus.Conditions) > 0 {
		if errUnmarshal := json.Unmarshal(adapterStatus.Conditions, &conditions); errUnmarshal != nil {
			return nil, false, errors.GeneralError(
				"Failed to unmarshal adapter status conditions: %s", errUnmarshal,
			)
		}
	}

	if errorType, conditionName := ValidateMandatoryConditions(conditions); errorType != "" {
		return nil, false, errors.Validation(
			"missing mandatory condition '%s': all adapters must report Available, Applied, and Health",
			conditionName,
		)
	}

	triggerAggregation := false
	for _, cond := range conditions {
		if cond.Type != api.AdapterConditionTypeAvailable {
			continue
		}

		isValidStatus := cond.Status == api.AdapterConditionTrue ||
			cond.Status == api.AdapterConditionFalse ||
			cond.Status == api.AdapterConditionUnknown
		if !isValidStatus {
			return nil, false, errors.Validation(
				"condition '%s' has invalid status '%s': must be True, False, or Unknown",
				cond.Type, cond.Status,
			)
		}

		if cond.Status != api.AdapterConditionTrue && cond.Status != api.AdapterConditionFalse {
			if existingStatus != nil {
				log.Debug("Discarding adapter status update: subsequent Unknown Available")
				return nil, false, nil
			}
			triggerAggregation = false
			break
		}

		triggerAggregation = true
		break
	}

	return conditions, triggerAggregation, nil
}
