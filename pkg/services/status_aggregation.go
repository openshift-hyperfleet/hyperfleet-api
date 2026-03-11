package services

import (
	"context"
	"encoding/json"
	"fmt"
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
	ConditionValidationErrorDuplicate     = "duplicate"
	ConditionValidationErrorMissing       = "missing"
	ConditionValidationErrorInvalidStatus = "invalid_status"
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

// validAdapterConditionStatuses holds the set of allowed status values for adapter conditions.
var validAdapterConditionStatuses = map[api.AdapterConditionStatus]bool{
	api.AdapterConditionTrue:    true,
	api.AdapterConditionFalse:   true,
	api.AdapterConditionUnknown: true,
}

// ValidateMandatoryConditions checks that all mandatory conditions are present, have no
// duplicate types, and carry a valid status value (True, False, or Unknown).
// Returns (errorType, conditionName) where:
// - If duplicate found: (ConditionValidationErrorDuplicate, duplicateConditionType)
// - If missing condition: (ConditionValidationErrorMissing, missingConditionType)
// - If invalid status: (ConditionValidationErrorInvalidStatus, conditionType)
// - If all valid: ("", "")
func ValidateMandatoryConditions(conditions []api.AdapterCondition) (errorType, conditionName string) {
	// Check for duplicate condition types and track status values.
	seen := make(map[string]api.AdapterConditionStatus)
	for _, cond := range conditions {
		// Reject empty condition types
		if cond.Type == "" {
			return ConditionValidationErrorMissing, "<empty type>"
		}
		if _, exists := seen[cond.Type]; exists {
			return ConditionValidationErrorDuplicate, cond.Type
		}
		seen[cond.Type] = cond.Status
	}

	// Check that all mandatory conditions are present and have valid status values.
	for _, mandatoryType := range mandatoryConditionTypes {
		status, exists := seen[mandatoryType]
		if !exists {
			return ConditionValidationErrorMissing, mandatoryType
		}
		if !validAdapterConditionStatuses[status] {
			return ConditionValidationErrorInvalidStatus, mandatoryType
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

// adapterAvailabilitySnapshot is the result of scanning all required adapters to determine
// whether they are at a consistent generation and whether they all report Available=True.
type adapterAvailabilitySnapshot struct {
	// consistent is true when every required adapter has reported at the same observed generation.
	// When false, the existing Available condition should be left unchanged.
	consistent bool
	// available is true when consistent=true and every required adapter has Available=True.
	available bool
	// generation is the common observed generation across required adapters (only valid when consistent=true).
	generation int32
	// minLastReportTime is the earliest LastReportTime across all required adapters (nil when none available).
	minLastReportTime *time.Time
}

// scanAdapterAvailability inspects the stored adapter statuses for the required adapters and
// returns a snapshot describing whether they are consistent and available.
//
// Available=True requires all required adapters to have reported at the same observed generation
// and all to have Available=True. If any required adapter is missing or at a different generation,
// consistent=false is returned and the caller should leave the current Available condition unchanged.
func scanAdapterAvailability(
	ctx context.Context,
	adapterStatuses api.AdapterStatusList,
	requiredAdapters []string,
) adapterAvailabilitySnapshot {
	if len(requiredAdapters) == 0 {
		return adapterAvailabilitySnapshot{}
	}

	type adapterInfo struct {
		available          string
		observedGeneration int32
		lastReportTime     *time.Time
	}
	adapterMap := make(map[string]adapterInfo, len(adapterStatuses))

	for _, s := range adapterStatuses {
		var conditions []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		}
		if len(s.Conditions) == 0 {
			continue
		}
		if err := json.Unmarshal(s.Conditions, &conditions); err != nil {
			logger.WithError(ctx, err).Warn(
				fmt.Sprintf("failed to parse adapter conditions for adapter %s", s.Adapter))
			continue
		}
		for _, cond := range conditions {
			if cond.Type == api.ConditionTypeAvailable {
				adapterMap[s.Adapter] = adapterInfo{
					available:          cond.Status,
					observedGeneration: s.ObservedGeneration,
					lastReportTime:     s.LastReportTime,
				}
				break
			}
		}
	}

	// All required adapters must have reported at the same observed generation.
	var commonGen *int32
	for _, name := range requiredAdapters {
		info, exists := adapterMap[name]
		if !exists {
			return adapterAvailabilitySnapshot{}
		}
		if commonGen == nil {
			g := info.observedGeneration
			commonGen = &g
		} else if info.observedGeneration != *commonGen {
			return adapterAvailabilitySnapshot{}
		}
	}

	if commonGen == nil {
		return adapterAvailabilitySnapshot{}
	}

	// Consistent generation: determine availability and earliest report time.
	allAvailable := true
	var minLRT *time.Time
	for _, name := range requiredAdapters {
		info := adapterMap[name]
		if info.available != string(api.AdapterConditionTrue) {
			allAvailable = false
		}
		if info.lastReportTime != nil {
			if minLRT == nil || info.lastReportTime.Before(*minLRT) {
				t := *info.lastReportTime
				minLRT = &t
			}
		}
	}

	return adapterAvailabilitySnapshot{
		consistent:        true,
		available:         allAvailable,
		generation:        *commonGen,
		minLastReportTime: minLRT,
	}
}

// buildAvailableCondition computes the Available ResourceCondition from the current adapter
// availability snapshot, the previous condition (may be nil), and the evaluation time.
//
// observedTime is the triggering adapter's observed_time (its LastReportTime). It is used for
// LastTransitionTime on status changes and LastUpdatedTime on True→False transitions. For all
// other cases LastUpdatedTime is the earliest report time across required adapters.
//
// When adapters are not at a consistent generation, the existing condition is preserved unchanged.
func buildAvailableCondition(
	snapshot adapterAvailabilitySnapshot,
	existing *api.ResourceCondition,
	resourceGeneration int32,
	now time.Time,
	observedTime time.Time,
) api.ResourceCondition {
	if !snapshot.consistent {
		if existing != nil {
			return *existing
		}
		return api.ResourceCondition{
			Type:               api.ConditionTypeAvailable,
			Status:             api.ConditionFalse,
			ObservedGeneration: resourceGeneration,
			LastTransitionTime: now,
			CreatedTime:        now,
			LastUpdatedTime:    now,
		}
	}

	newStatus := api.ConditionFalse
	if snapshot.available {
		newStatus = api.ConditionTrue
	}

	prevStatus := api.ConditionFalse
	if existing != nil {
		prevStatus = existing.Status
	}

	// True→False: use the triggering adapter's observed time (when the failure was first seen).
	// All other cases (stays True, stays False, False→True): use earliest adapter report time.
	var lut time.Time
	switch {
	case prevStatus == api.ConditionTrue && newStatus == api.ConditionFalse:
		lut = observedTime
	case snapshot.minLastReportTime != nil:
		lut = *snapshot.minLastReportTime
	default:
		lut = now
	}

	// LastTransitionTime advances on status change using the triggering adapter's observed time.
	ltt := observedTime
	if existing != nil && existing.Status == newStatus && !existing.LastTransitionTime.IsZero() {
		ltt = existing.LastTransitionTime
	}

	createdTime := now
	if existing != nil && !existing.CreatedTime.IsZero() {
		createdTime = existing.CreatedTime
	}

	cond := api.ResourceCondition{
		Type:               api.ConditionTypeAvailable,
		Status:             newStatus,
		ObservedGeneration: snapshot.generation,
		LastTransitionTime: ltt,
		CreatedTime:        createdTime,
		LastUpdatedTime:    lut,
	}

	// Carry over Reason/Message when status is unchanged.
	if existing != nil && prevStatus == newStatus {
		if cond.Reason == nil {
			cond.Reason = existing.Reason
		}
		if cond.Message == nil {
			cond.Message = existing.Message
		}
	}

	return cond
}

// ComputeReadyCondition reports whether all required adapters have Available=True at the current
// resource generation. Unlike Available, Ready requires adapters to be caught up to the latest
// generation — it does not accept reports from older generations.
func ComputeReadyCondition(
	ctx context.Context, adapterStatuses api.AdapterStatusList, requiredAdapters []string, resourceGeneration int32,
) bool {
	if len(adapterStatuses) == 0 || len(requiredAdapters) == 0 {
		return false
	}

	type adapterInfo struct {
		available          string
		observedGeneration int32
	}
	adapterMap := make(map[string]adapterInfo, len(adapterStatuses))

	for _, s := range adapterStatuses {
		var conditions []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		}
		if len(s.Conditions) == 0 {
			continue
		}
		if err := json.Unmarshal(s.Conditions, &conditions); err != nil {
			logger.WithError(ctx, err).Warn(
				fmt.Sprintf("failed to parse adapter conditions for adapter %s", s.Adapter))
			continue
		}
		for _, cond := range conditions {
			if cond.Type == api.ConditionTypeAvailable {
				adapterMap[s.Adapter] = adapterInfo{
					available:          cond.Status,
					observedGeneration: s.ObservedGeneration,
				}
				break
			}
		}
	}

	numReady := 0
	for _, name := range requiredAdapters {
		info, exists := adapterMap[name]
		if !exists || (resourceGeneration > 0 && info.observedGeneration != resourceGeneration) {
			continue
		}
		if info.available == string(api.AdapterConditionTrue) {
			numReady++
		}
	}

	return numReady == len(requiredAdapters)
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
func adapterConditionsHasAvailableTrue(ctx context.Context, adapterName string, conditions []byte) bool {
	if len(conditions) == 0 {
		return false
	}
	var conds []struct {
		Type   string `json:"type"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(conditions, &conds); err != nil {
		logger.WithError(ctx, err).Warn(
			fmt.Sprintf("failed to parse adapter conditions for adapter %s", adapterName))
		return false
	}
	for _, c := range conds {
		if c.Type == api.ConditionTypeAvailable && c.Status == string(api.AdapterConditionTrue) {
			return true
		}
	}
	return false
}

// computeReadyLastUpdated returns the timestamp to use for the Ready condition's LastUpdatedTime.
//
// When isReady is false, it returns the minimum LastReportTime across all required adapters
// (spec: last_update_time=min(resource.statuses[].last_update_time) when Ready stays False).
// Falls back to now if any required adapter has no stored status or no LRT yet.
//
// When isReady is true, it returns the minimum LastReportTime across all required adapters
// that have Available=True at the current generation. Falls back to now if none qualify.
//
// Note: True→False transitions override this value with observedTime in buildReadyCondition.
func computeReadyLastUpdated(
	ctx context.Context,
	adapterStatuses api.AdapterStatusList,
	requiredAdapters []string,
	resourceGeneration int32,
	now time.Time,
	isReady bool,
) time.Time {
	if !isReady {
		// Use min(LRTs) across all required adapters.
		// Fall back to now if a required adapter has no stored status or no LRT.
		var minTime *time.Time
		for _, adapterName := range requiredAdapters {
			status, ok := findAdapterStatus(adapterStatuses, adapterName)
			if !ok || status.LastReportTime == nil {
				return now // safety: required adapter missing or has no timestamp
			}
			if minTime == nil || status.LastReportTime.Before(*minTime) {
				t := *status.LastReportTime
				minTime = &t
			}
		}
		if minTime == nil {
			return now
		}
		return *minTime
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
		if !adapterConditionsHasAvailableTrue(ctx, adapterName, status.Conditions) {
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

// buildReadyCondition computes the Ready ResourceCondition from the current adapter statuses,
// the previous condition (may be nil), and the evaluation time.
//
// observedTime is the triggering adapter's observed_time. It is used for LastTransitionTime
// on status changes and LastUpdatedTime on True→False transitions.
//
// Ready=True requires all required adapters to have Available=True at the current resource
// generation. LastUpdatedTime is the evaluation time when False (so callers can apply a
// freshness threshold), or the earliest adapter report time when True.
func buildReadyCondition(
	ctx context.Context,
	adapterStatuses api.AdapterStatusList,
	requiredAdapters []string,
	resourceGeneration int32,
	existing *api.ResourceCondition,
	now time.Time,
	observedTime time.Time,
) api.ResourceCondition {
	isReady := ComputeReadyCondition(ctx, adapterStatuses, requiredAdapters, resourceGeneration)
	status := api.ConditionFalse
	if isReady {
		status = api.ConditionTrue
	}

	cond := api.ResourceCondition{
		Type:               api.ConditionTypeReady,
		Status:             status,
		ObservedGeneration: resourceGeneration,
		LastTransitionTime: now,
		CreatedTime:        now,
		LastUpdatedTime:    now,
	}

	lut := computeReadyLastUpdated(ctx, adapterStatuses, requiredAdapters, resourceGeneration, now, isReady)

	// True→False: use the triggering adapter's observed time (when the failure was first seen).
	prevStatus := api.ConditionFalse
	if existing != nil {
		prevStatus = existing.Status
	}
	if prevStatus == api.ConditionTrue && status == api.ConditionFalse {
		lut = observedTime
	}

	applyConditionHistory(&cond, existing, observedTime, lut)

	return cond
}

func BuildSyntheticConditions(
	ctx context.Context,
	existingConditionsJSON []byte,
	adapterStatuses api.AdapterStatusList,
	requiredAdapters []string,
	resourceGeneration int32,
	now time.Time,
	observedTime time.Time,
	isLifecycleChange bool,
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

	// Zero adapters: trivially satisfied — both conditions are True.
	if len(requiredAdapters) == 0 {
		available := buildTrueCondition(api.ConditionTypeAvailable, existingAvailable, resourceGeneration, now)
		ready := buildTrueCondition(api.ConditionTypeReady, existingReady, resourceGeneration, now)
		return available, ready
	}

	// Lifecycle change (Create / Replace): Available is frozen; Ready resets with lut=now.
	if isLifecycleChange {
		var available api.ResourceCondition
		if existingAvailable != nil {
			available = *existingAvailable
		} else {
			available = api.ResourceCondition{
				Type:               api.ConditionTypeAvailable,
				Status:             api.ConditionFalse,
				ObservedGeneration: resourceGeneration,
				LastTransitionTime: now,
				CreatedTime:        now,
				LastUpdatedTime:    now,
			}
		}
		ready := buildReadyConditionLifecycle(existingReady, resourceGeneration, now, observedTime)
		return available, ready
	}

	// Normal adapter-report path.
	snapshot := scanAdapterAvailability(ctx, adapterStatuses, requiredAdapters)
	available := buildAvailableCondition(snapshot, existingAvailable, resourceGeneration, now, observedTime)
	ready := buildReadyCondition(
		ctx, adapterStatuses, requiredAdapters, resourceGeneration, existingReady, now, observedTime)

	return available, ready
}

// buildTrueCondition produces a True ResourceCondition, preserving history from existing.
// CreatedTime and LastTransitionTime are preserved when the existing condition was also True.
func buildTrueCondition(
	condType string,
	existing *api.ResourceCondition,
	resourceGeneration int32,
	now time.Time,
) api.ResourceCondition {
	cond := api.ResourceCondition{
		Type:               condType,
		Status:             api.ConditionTrue,
		ObservedGeneration: resourceGeneration,
		LastTransitionTime: now,
		CreatedTime:        now,
		LastUpdatedTime:    now,
	}
	if existing != nil {
		if !existing.CreatedTime.IsZero() {
			cond.CreatedTime = existing.CreatedTime
		}
		if existing.Status == api.ConditionTrue && !existing.LastTransitionTime.IsZero() {
			cond.LastTransitionTime = existing.LastTransitionTime
		}
	}
	return cond
}

// buildReadyConditionLifecycle produces Ready=False at the new generation with lut=now.
// History (CreatedTime, LastTransitionTime) is preserved via applyConditionHistory.
func buildReadyConditionLifecycle(
	existing *api.ResourceCondition,
	resourceGeneration int32,
	now time.Time,
	observedTime time.Time,
) api.ResourceCondition {
	cond := api.ResourceCondition{
		Type:               api.ConditionTypeReady,
		Status:             api.ConditionFalse,
		ObservedGeneration: resourceGeneration,
		LastTransitionTime: now,
		CreatedTime:        now,
		LastUpdatedTime:    now,
	}
	applyConditionHistory(&cond, existing, observedTime, now)
	return cond
}

// applyConditionHistory copies stable timestamps and metadata from an existing condition.
// transitionTime is used for LastTransitionTime on status changes — callers pass the
// triggering adapter's observed_time so the timestamp reflects when the change was observed.
// lastUpdatedTime is used unconditionally for LastUpdatedTime — the caller is responsible
// for computing the correct value (e.g. now, computeReadyLastUpdated(...)).
func applyConditionHistory(
	target *api.ResourceCondition,
	existing *api.ResourceCondition,
	transitionTime time.Time,
	lastUpdatedTime time.Time,
) {
	if existing == nil {
		target.LastUpdatedTime = lastUpdatedTime
		return
	}

	if !existing.CreatedTime.IsZero() {
		target.CreatedTime = existing.CreatedTime
	}

	// LastTransitionTime only advances when the status value (True/False) changes.
	// A change in ObservedGeneration alone does not constitute a transition.
	if existing.Status == target.Status && !existing.LastTransitionTime.IsZero() {
		target.LastTransitionTime = existing.LastTransitionTime
	} else {
		target.LastTransitionTime = transitionTime
	}

	target.LastUpdatedTime = lastUpdatedTime

	if existing.Status == target.Status && existing.ObservedGeneration == target.ObservedGeneration {
		if target.Reason == nil && existing.Reason != nil {
			target.Reason = existing.Reason
		}
		if target.Message == nil && existing.Message != nil {
			target.Message = existing.Message
		}
	}
}
