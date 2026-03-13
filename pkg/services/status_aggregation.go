package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// Mandatory condition types that must be present in all adapter status updates
var mandatoryConditionTypes = []string{api.ConditionTypeAvailable, api.ConditionTypeApplied, api.ConditionTypeHealth}

// Condition validation error types
const (
	ConditionValidationErrorDuplicate = "duplicate"
	ConditionValidationErrorMissing   = "missing"
)

// adapterConditionStatusTrue is the string value for a True adapter condition status.
const adapterConditionStatusTrue = "True"

// Required adapter lists configured via pkg/config/adapter.go (see AdapterRequirementsConfig)

// adapterConditionSuffixMap allows overriding the default suffix for specific adapters
// Currently empty - all adapters use "Successful" by default
// Future example: To make dns use "Ready" instead, uncomment:
//
//	"dns": "Ready",
var adapterConditionSuffixMap = map[string]string{
	// Add custom mappings here when needed
}

// ValidateMandatoryConditions checks if all mandatory conditions are present and rejects duplicate condition types.
// Returns (errorType, conditionName) where:
// - If duplicate found: (ConditionValidationErrorDuplicate, duplicateConditionType)
// - If missing condition: (ConditionValidationErrorMissing, missingConditionType)
// - If all valid: ("", "")
func ValidateMandatoryConditions(conditions []api.AdapterCondition) (errorType, conditionName string) {
	// Check for duplicate condition types
	seen := make(map[string]bool)
	for _, cond := range conditions {
		// Reject empty condition types
		if cond.Type == "" {
			return ConditionValidationErrorMissing, "<empty type>"
		}
		if seen[cond.Type] {
			return ConditionValidationErrorDuplicate, cond.Type
		}
		seen[cond.Type] = true
	}

	// Check that all mandatory conditions are present
	for _, mandatoryType := range mandatoryConditionTypes {
		if !seen[mandatoryType] {
			return ConditionValidationErrorMissing, mandatoryType
		}
	}

	return "", ""
}

// MapAdapterToConditionType converts an adapter name to a semantic condition type
// by converting the adapter name to PascalCase and appending a suffix.
//
// Current behavior: All adapters → {AdapterName}Successful
// Examples:
//   - "validator" → "ValidatorSuccessful"
//   - "dns" → "DnsSuccessful"
//   - "gcp-provisioner" → "GcpProvisionerSuccessful"
//
// Future customization: Override suffix in adapterConditionSuffixMap
//
//	adapterConditionSuffixMap["dns"] = "Ready" → "DnsReady"
func MapAdapterToConditionType(adapterName string) string {
	// Get the suffix for this adapter, default to "Successful"
	suffix, exists := adapterConditionSuffixMap[adapterName]
	if !exists {
		suffix = "Successful"
	}

	// Convert adapter name to PascalCase
	// Remove hyphens and capitalize each part
	parts := strings.Split(adapterName, "-")
	var result strings.Builder

	for _, part := range parts {
		if len(part) > 0 {
			// Capitalize first letter
			runes := []rune(part)
			runes[0] = unicode.ToUpper(runes[0])
			result.WriteString(string(runes))
		}
	}

	result.WriteString(suffix)
	return result.String()
}

// ComputeAvailableCondition checks if all required adapters have Available=True at ANY generation.
// Returns: (isAvailable bool, minObservedGeneration int32)
// "Available" means the system is running at some known good configuration (last known good config).
// The minObservedGeneration is the lowest generation across all required adapters.
func ComputeAvailableCondition(adapterStatuses api.AdapterStatusList, requiredAdapters []string) (bool, int32) {
	if len(adapterStatuses) == 0 || len(requiredAdapters) == 0 {
		return false, 1
	}

	// Build a map of adapter name -> (available status, observed generation)
	adapterMap := make(map[string]struct {
		available          string
		observedGeneration int32
	})

	for _, adapterStatus := range adapterStatuses {
		// Unmarshal conditions to find "Available"
		var conditions []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		}
		if len(adapterStatus.Conditions) > 0 {
			if err := json.Unmarshal(adapterStatus.Conditions, &conditions); err != nil {
				logger.WithError(context.Background(), err).Warn(
					fmt.Sprintf("failed to parse adapter conditions for adapter %s", adapterStatus.Adapter))
				continue
			}
			for _, cond := range conditions {
				if cond.Type == api.ConditionTypeAvailable {
					adapterMap[adapterStatus.Adapter] = struct {
						available          string
						observedGeneration int32
					}{
						available:          cond.Status,
						observedGeneration: adapterStatus.ObservedGeneration,
					}
					break
				}
			}
		}
	}

	// Count available adapters and track min observed generation
	numAvailable := 0
	minObservedGeneration := int32(math.MaxInt32)

	for _, adapterName := range requiredAdapters {
		adapterInfo, exists := adapterMap[adapterName]

		if !exists {
			// Required adapter not found - not available
			continue
		}

		// For Available condition, we don't check generation matching
		// We just need Available=True at ANY generation
		if adapterInfo.available == adapterConditionStatusTrue {
			numAvailable++
			if adapterInfo.observedGeneration < minObservedGeneration {
				minObservedGeneration = adapterInfo.observedGeneration
			}
		}
	}

	// Available when all required adapters have Available=True (at any generation)
	numRequired := len(requiredAdapters)
	if numAvailable == numRequired {
		return true, minObservedGeneration
	}

	// Return 0 for minObservedGeneration when not available
	return false, 0
}

// ComputeReadyCondition checks if all required adapters have Available=True at the CURRENT generation.
// "Ready" means the system is running at the latest spec generation.
func ComputeReadyCondition(
	adapterStatuses api.AdapterStatusList, requiredAdapters []string, resourceGeneration int32,
) bool {
	if len(adapterStatuses) == 0 || len(requiredAdapters) == 0 {
		return false
	}

	// Build a map of adapter name -> (available status, observed generation)
	adapterMap := make(map[string]struct {
		available          string
		observedGeneration int32
	})

	for _, adapterStatus := range adapterStatuses {
		// Unmarshal conditions to find "Available"
		var conditions []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		}
		if len(adapterStatus.Conditions) > 0 {
			if err := json.Unmarshal(adapterStatus.Conditions, &conditions); err != nil {
				logger.WithError(context.Background(), err).Warn(
					fmt.Sprintf("failed to parse adapter conditions for adapter %s", adapterStatus.Adapter))
				continue
			}
			for _, cond := range conditions {
				if cond.Type == api.ConditionTypeAvailable {
					adapterMap[adapterStatus.Adapter] = struct {
						available          string
						observedGeneration int32
					}{
						available:          cond.Status,
						observedGeneration: adapterStatus.ObservedGeneration,
					}
					break
				}
			}
		}
	}

	// Count ready adapters (Available=True at current generation)
	numReady := 0

	for _, adapterName := range requiredAdapters {
		adapterInfo, exists := adapterMap[adapterName]

		if !exists {
			// Required adapter not found - not ready
			continue
		}

		// For Ready condition, we require generation matching
		if resourceGeneration > 0 && adapterInfo.observedGeneration != resourceGeneration {
			// Adapter is processing old generation (stale) - not ready
			continue
		}

		// Check available status
		if adapterInfo.available == adapterConditionStatusTrue {
			numReady++
		}
	}

	// Ready when all required adapters have Available=True at current generation
	numRequired := len(requiredAdapters)
	return numReady == numRequired
}

// findAdapterStatus returns the first adapter status in the list with the given adapter name, or (nil, false).
func findAdapterStatus(adapterStatuses api.AdapterStatusList, adapterName string) (*api.AdapterStatus, bool) {
	for _, s := range adapterStatuses {
		if s.Adapter == adapterName {
			return s, true
		}
	}
	return nil, false
}

// adapterConditionsHasAvailableTrue returns true if the adapter conditions JSON
// contains a condition with type Available and status True.
func adapterConditionsHasAvailableTrue(conditions []byte) bool {
	if len(conditions) == 0 {
		return false
	}
	var conds []struct {
		Type   string `json:"type"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(conditions, &conds); err != nil {
		return false
	}
	for _, c := range conds {
		if c.Type == api.ConditionTypeAvailable && c.Status == adapterConditionStatusTrue {
			return true
		}
	}
	return false
}

// computeReadyLastUpdated returns the timestamp to use for the Ready condition's LastUpdatedTime.
// When isReady is false, it returns now (Ready=False changes frequently; 10s threshold applies).
// When isReady is true, it returns the minimum LastReportTime across all required adapters
// that have Available=True at the current generation. Falls back to now if no timestamps found.
func computeReadyLastUpdated(
	adapterStatuses api.AdapterStatusList,
	requiredAdapters []string,
	resourceGeneration int32,
	now time.Time,
	isReady bool,
) time.Time {
	if !isReady {
		return now
	}

	var minTime *time.Time
	for _, adapterName := range requiredAdapters {
		status, ok := findAdapterStatus(adapterStatuses, adapterName)
		if !ok {
			return now // safety: required adapter missing
		}
		if status.LastReportTime == nil {
			return now // safety: no timestamp
		}
		if status.ObservedGeneration != resourceGeneration {
			continue // not at current gen, skip
		}
		if !adapterConditionsHasAvailableTrue(status.Conditions) {
			continue
		}
		if minTime == nil || status.LastReportTime.Before(*minTime) {
			t := *status.LastReportTime
			minTime = &t
		}
	}

	if minTime == nil {
		return now // safety fallback
	}
	return *minTime
}

func BuildSyntheticConditions(
	existingConditionsJSON []byte,
	adapterStatuses api.AdapterStatusList,
	requiredAdapters []string,
	resourceGeneration int32,
	now time.Time,
) (api.ResourceCondition, api.ResourceCondition) {
	var existingAvailable *api.ResourceCondition
	var existingReady *api.ResourceCondition

	if len(existingConditionsJSON) > 0 {
		var existingConditions []api.ResourceCondition
		if err := json.Unmarshal(existingConditionsJSON, &existingConditions); err == nil {
			for i := range existingConditions {
				switch existingConditions[i].Type {
				case api.ConditionTypeAvailable:
					existingAvailable = &existingConditions[i]
				case api.ConditionTypeReady:
					existingReady = &existingConditions[i]
				}
			}
		}
	}

	isAvailable, minObservedGeneration := ComputeAvailableCondition(adapterStatuses, requiredAdapters)
	availableStatus := api.ConditionFalse
	if isAvailable {
		availableStatus = api.ConditionTrue
	}
	availableCondition := api.ResourceCondition{
		Type:               api.ConditionTypeAvailable,
		Status:             availableStatus,
		ObservedGeneration: minObservedGeneration,
		LastTransitionTime: now,
		CreatedTime:        now,
		LastUpdatedTime:    now,
	}
	availableLastUpdated := now
	if existingAvailable != nil &&
		existingAvailable.Status == availableStatus &&
		existingAvailable.ObservedGeneration == minObservedGeneration &&
		!existingAvailable.LastUpdatedTime.IsZero() {
		availableLastUpdated = existingAvailable.LastUpdatedTime
	}
	applyConditionHistory(&availableCondition, existingAvailable, now, availableLastUpdated)

	isReady := ComputeReadyCondition(adapterStatuses, requiredAdapters, resourceGeneration)
	readyStatus := api.ConditionFalse
	if isReady {
		readyStatus = api.ConditionTrue
	}
	readyCondition := api.ResourceCondition{
		Type:               api.ConditionTypeReady,
		Status:             readyStatus,
		ObservedGeneration: resourceGeneration,
		LastTransitionTime: now,
		CreatedTime:        now,
		LastUpdatedTime:    now,
	}
	readyLastUpdated := computeReadyLastUpdated(
		adapterStatuses, requiredAdapters, resourceGeneration, now, isReady,
	)
	applyConditionHistory(&readyCondition, existingReady, now, readyLastUpdated)

	return availableCondition, readyCondition
}

// applyConditionHistory copies stable timestamps and metadata from an existing condition.
// lastUpdatedTime is used unconditionally for LastUpdatedTime — the caller is responsible
// for computing the correct value (e.g. now, computeReadyLastUpdated(...)).
func applyConditionHistory(
	target *api.ResourceCondition,
	existing *api.ResourceCondition,
	now time.Time,
	lastUpdatedTime time.Time,
) {
	if existing == nil {
		target.LastUpdatedTime = lastUpdatedTime
		return
	}

	if existing.Status == target.Status && existing.ObservedGeneration == target.ObservedGeneration {
		if !existing.CreatedTime.IsZero() {
			target.CreatedTime = existing.CreatedTime
		}
		if !existing.LastTransitionTime.IsZero() {
			target.LastTransitionTime = existing.LastTransitionTime
		}
		target.LastUpdatedTime = lastUpdatedTime
		if target.Reason == nil && existing.Reason != nil {
			target.Reason = existing.Reason
		}
		if target.Message == nil && existing.Message != nil {
			target.Message = existing.Message
		}
		return
	}

	if !existing.CreatedTime.IsZero() {
		target.CreatedTime = existing.CreatedTime
	}
	target.LastTransitionTime = now
	target.LastUpdatedTime = lastUpdatedTime
}
