package services

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// Required adapter lists configured via pkg/config/adapter.go (see AdapterRequirementsConfig)

// adapterConditionSuffixMap allows overriding the default suffix for specific adapters
// Currently empty - all adapters use "Successful" by default
// Future example: To make dns use "Ready" instead, uncomment:
//
//	"dns": "Ready",
var adapterConditionSuffixMap = map[string]string{
	// Add custom mappings here when needed
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

// ComputePhase calculates the overall phase for a resource based on adapter statuses
//
// MVP Phase Logic (per architecture/hyperfleet/docs/status-guide.md):
// - "Ready": All required adapters have Available=True for current generation
// - "NotReady": Otherwise (any adapter has Available=False or hasn't reported yet)
//
// An adapter is considered "available" when:
// 1. Available condition status == "True"
// 2. observed_generation == resource.generation (only checked if resourceGeneration > 0)
//
// Note: Post-MVP will add more phases (Pending, Provisioning, Failed, Degraded)
// based on Health condition and Applied condition states.
func ComputePhase(ctx context.Context, adapterStatuses api.AdapterStatusList, requiredAdapters []string, resourceGeneration int32) string {
	if len(adapterStatuses) == 0 {
		return "NotReady"
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
					if cond.Type == "Available" {
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

	// Count available adapters
	numAvailable := 0

	// Iterate over required adapters
	for _, adapterName := range requiredAdapters {
		adapterInfo, exists := adapterMap[adapterName]

		if !exists {
			// Required adapter not found
			logger.With(ctx, logger.FieldAdapter, adapterName).Warn("Required adapter not found in adapter statuses")
			continue
		}

		// Check generation matching only if resourceGeneration > 0
		if resourceGeneration > 0 && adapterInfo.observedGeneration != resourceGeneration {
			// Adapter is processing old generation (stale)
			logger.With(ctx,
				logger.FieldAdapter, adapterName,
				"observed_generation", adapterInfo.observedGeneration,
				"expected_generation", resourceGeneration).Warn("Required adapter has stale generation")
			continue
		}

		// Check available status
		if adapterInfo.available == "True" {
			numAvailable++
		}
	}

	// MVP: Only Ready or NotReady
	// Ready when all required adapters have Available=True for current generation
	numRequired := len(requiredAdapters)
	if numAvailable == numRequired && numRequired > 0 {
		return "Ready"
	}
	return "NotReady"
}
