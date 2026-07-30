package services

import (
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

func mandatoryConditions() []string {
	return []string{api.AdapterConditionTypeAvailable, api.AdapterConditionTypeApplied, api.AdapterConditionTypeHealth}
}

const (
	ConditionValidationErrorMissing = "missing"
)

func ValidateMandatoryConditions(conditions []api.AdapterCondition) (errorType, conditionName string) {
	seen := make(map[string]bool)
	for _, cond := range conditions {
		seen[cond.Type] = true
	}

	for _, mandatoryType := range mandatoryConditions() {
		if !seen[mandatoryType] {
			return ConditionValidationErrorMissing, mandatoryType
		}
	}

	return "", ""
}

func AdapterObservedTime(as *api.AdapterStatus) time.Time {
	if as == nil {
		return time.Time{}
	}
	return as.LastReportTime
}
