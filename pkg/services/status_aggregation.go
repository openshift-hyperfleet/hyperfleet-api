package services

import (
	"encoding/json"
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

// Mandatory condition types that must be present in all adapter status updates
var mandatoryConditionTypes = []string{api.ConditionTypeAvailable, api.ConditionTypeApplied, api.ConditionTypeHealth}

// Required adapter lists configured via pkg/config/adapter.go (see AdapterRequirementsConfig)

// adapterConditionSuffixMap allows overriding the default suffix for specific adapters
// Currently empty - all adapters use "Successful" by default
// Future example: To make dns use "Ready" instead, uncomment:
//
//	"dns": "Ready",
var adapterConditionSuffixMap = map[string]string{
	// Add custom mappings here when needed
}

// ValidateMandatoryConditions checks if all mandatory conditions are present and not Unknown.
// Returns (missingCondition, unknownCondition) where:
// - missingCondition: the name of the first missing mandatory condition, or empty string if all present
// - unknownCondition: the name of the first mandatory condition with Unknown status, or empty string if none
//
// Usage:
//
//	if missing, unknown := ValidateMandatoryConditions(conditions); missing != "" {
//	    // Handle missing condition
//	} else if unknown != "" {
//	    // Handle unknown condition
//	}
func ValidateMandatoryConditions(conditions []api.AdapterCondition) (missingCondition, unknownCondition string) {
	// Build a map of condition types for validation
	// If duplicate condition types exist, preserve Unknown status (highest severity)
	conditionMap := make(map[string]api.AdapterConditionStatus)
	for _, cond := range conditions {
		existing, exists := conditionMap[cond.Type]
		// If already Unknown, keep it (don't let True/False overwrite Unknown)
		if exists && existing == api.AdapterConditionUnknown {
			continue
		}
		conditionMap[cond.Type] = cond.Status
	}

	// Check that all mandatory conditions are present and not Unknown
	for _, mandatoryType := range mandatoryConditionTypes {
		status, exists := conditionMap[mandatoryType]
		if !exists {
			return mandatoryType, ""
		}
		if status == api.AdapterConditionUnknown {
			return "", mandatoryType
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
			if err := json.Unmarshal(adapterStatus.Conditions, &conditions); err == nil {
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
		if adapterInfo.available == "True" {
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
			if err := json.Unmarshal(adapterStatus.Conditions, &conditions); err == nil {
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
		if adapterInfo.available == "True" {
			numReady++
		}
	}

	// Ready when all required adapters have Available=True at current generation
	numRequired := len(requiredAdapters)
	return numReady == numRequired
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
	preserveSyntheticCondition(&availableCondition, existingAvailable, now)

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
	preserveSyntheticCondition(&readyCondition, existingReady, now)

	return availableCondition, readyCondition
}

func preserveSyntheticCondition(target *api.ResourceCondition, existing *api.ResourceCondition, now time.Time) {
	if existing == nil {
		return
	}

	if existing.Status == target.Status && existing.ObservedGeneration == target.ObservedGeneration {
		if !existing.CreatedTime.IsZero() {
			target.CreatedTime = existing.CreatedTime
		}
		if !existing.LastTransitionTime.IsZero() {
			target.LastTransitionTime = existing.LastTransitionTime
		}
		if !existing.LastUpdatedTime.IsZero() {
			target.LastUpdatedTime = existing.LastUpdatedTime
		}
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
	target.LastUpdatedTime = now
}
