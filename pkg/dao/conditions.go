package dao

import (
	"encoding/json"
	"time"

	"gorm.io/datatypes"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

// resetReadyConditionOnSpecChange flips Ready=False when the spec changes (generation incremented).
// This ensures Sentinel's selective querying immediately picks up the resource
// via the "not-ready resources" query on the next poll cycle.
// Available is intentionally left unchanged — it reflects last known good state at any generation.
func resetReadyConditionOnSpecChange(existingConditions datatypes.JSON, now time.Time) (datatypes.JSON, error) {
	if len(existingConditions) == 0 {
		return existingConditions, nil
	}

	var conditions []api.ResourceCondition
	if err := json.Unmarshal(existingConditions, &conditions); err != nil {
		return existingConditions, nil
	}

	changed := false
	for i := range conditions {
		if conditions[i].Type == api.ConditionTypeReady && conditions[i].Status == api.ConditionTrue {
			conditions[i].Status = api.ConditionFalse
			conditions[i].LastTransitionTime = now
			conditions[i].LastUpdatedTime = now
			reason := "SpecChanged"
			message := "Spec updated, awaiting adapters to reconcile at new generation"
			conditions[i].Reason = &reason
			conditions[i].Message = &message
			changed = true
			break
		}
	}

	if !changed {
		return existingConditions, nil
	}

	return json.Marshal(conditions)
}
