package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// mandatoryConditions returns the condition types that must be present in all adapter status updates.
// Returned as a new slice each call to prevent accidental mutation of a shared package-level value.
func mandatoryConditions() []string {
	return []string{api.ConditionTypeAvailable, api.ConditionTypeApplied, api.ConditionTypeHealth}
}

// Condition validation error type
const (
	ConditionValidationErrorMissing = "missing"
)

// reasonMissingRequiredAdapters is the reason code for the Ready condition when one or more
// required adapters have not yet reported Available=True at the current resource generation.
const reasonMissingRequiredAdapters = "MissingRequiredAdapters"
const reasonAllAdaptersAvailable = "All required adapters reported Available=True at the current generation"
const reasonAllAdaptersReconciled = "All required adapters reconciled at the current generation"

// ValidateMandatoryConditions checks if all mandatory conditions are present.
// Format validation (empty type, duplicates, invalid status) is done in the Handler layer.
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

// --- Aggregated Ready / Available -------------------------------------------------

// adapterConditionSuffixMap allows overriding the default suffix for specific adapters (reserved).
var adapterConditionSuffixMap = map[string]string{}

// MapAdapterToConditionType converts an adapter name to a semantic condition type (PascalCase + suffix).
// Used to derive the type name for per-adapter conditions mirrored into resource status
// (e.g. "adapter1" → "Adapter1Successful", "my-adapter" → "MyAdapterSuccessful").
func MapAdapterToConditionType(adapterName string) string {
	return mapAdapterToConditionType(adapterName, adapterConditionSuffixMap)
}

func mapAdapterToConditionType(adapterName string, suffixMap map[string]string) string {
	suffix, exists := suffixMap[adapterName]
	if !exists {
		suffix = "Successful"
	}

	parts := strings.Split(adapterName, "-")
	var result strings.Builder

	for _, part := range parts {
		if len(part) > 0 {
			runes := []rune(part)
			runes[0] = unicode.ToUpper(runes[0])
			result.WriteString(string(runes))
		}
	}

	result.WriteString(suffix)
	return result.String()
}

// AggregateResourceStatusInput carries everything needed to compute deterministic conditions.
// RefTime must be resource.updated_time (never time.Now) so results are reproducible.
//
//nolint:govet // fieldalignment: field order matches logical grouping for readers
type AggregateResourceStatusInput struct {
	ResourceGeneration int32
	RefTime            time.Time
	DeletedTime        *time.Time
	PrevConditionsJSON []byte
	RequiredAdapters   []string
	AdapterStatuses    api.AdapterStatusList
}

// AdapterObservedTime returns the adapter-reported observation instant used for ordering and aggregation.
func AdapterObservedTime(as *api.AdapterStatus) time.Time {
	if as == nil {
		return time.Time{}
	}
	return as.LastReportTime
}

// AggregateResourceStatus computes Ready, Available, and per-adapter conditions from stored adapter
// rows and previous conditions. It does not use wall clock.
//
// The returned adapterConditions slice contains one entry per required adapter that has reported,
// with a type derived from the adapter name (e.g. "adapter1" → "Adapter1Successful").
func AggregateResourceStatus(ctx context.Context, in AggregateResourceStatusInput) (
	ready, available, reconciled api.ResourceCondition, adapterConditions []api.ResourceCondition,
) {
	prevReady, prevAvail, prevReconciled, prevAdapterByType := parsePrevConditions(ctx, in.PrevConditionsJSON)

	reports := normalizeAdapterReportsForAggregation(ctx, in.AdapterStatuses, in.RequiredAdapters, in.ResourceGeneration)

	ready = computeReady(
		in.ResourceGeneration,
		in.RefTime,
		prevReady,
		in.RequiredAdapters,
		reports,
	)
	reconciled = computeReconciled(
		in.ResourceGeneration,
		in.RefTime,
		in.DeletedTime,
		prevReconciled,
		in.RequiredAdapters,
		reports,
	)
	available = computeAvailable(
		in.RefTime,
		prevAvail,
		in.RequiredAdapters,
		reports,
	)
	adapterConditions = computeAdapterConditions(in.RequiredAdapters, reports, prevAdapterByType, in.RefTime)
	return ready, available, reconciled, adapterConditions
}

func parsePrevConditions(ctx context.Context, raw []byte) (
	prevReady, prevAvail, prevReconciled *api.ResourceCondition, prevAdapterByType map[string]*api.ResourceCondition,
) {
	prevAdapterByType = make(map[string]*api.ResourceCondition)
	if len(raw) == 0 {
		return nil, nil, nil, prevAdapterByType
	}
	var conditions []api.ResourceCondition
	if err := json.Unmarshal(raw, &conditions); err != nil {
		logger.WithError(ctx, err).Error("Failed to unmarshal previous conditions JSON; proceeding with empty state")
		return nil, nil, nil, prevAdapterByType
	}
	for i := range conditions {
		c := conditions[i]
		switch c.Type {
		case api.ConditionTypeReady:
			prevReady = &c
		case api.ConditionTypeReconciled:
			prevReconciled = &c
		case api.ConditionTypeAvailable:
			prevAvail = &c
		default:
			prevAdapterByType[c.Type] = &c
		}
	}
	return prevReady, prevAvail, prevReconciled, prevAdapterByType
}

// adapterAvailableSnapshot is one required adapter's Available signal after normalization.
type adapterAvailableSnapshot struct {
	observedTime       time.Time
	reason             *string
	message            *string
	availableTrue      bool
	finalizedTrue      bool
	observedGeneration int32
}

func normalizeAdapterReportsForAggregation(
	ctx context.Context,
	list api.AdapterStatusList,
	required []string,
	resourceGen int32,
) map[string]adapterAvailableSnapshot {
	requiredSet := make(map[string]struct{}, len(required))
	for _, a := range required {
		requiredSet[a] = struct{}{}
	}

	out := make(map[string]adapterAvailableSnapshot, len(required))

	for _, as := range list {
		if _, ok := requiredSet[as.Adapter]; !ok {
			continue
		}

		obsTime := AdapterObservedTime(as)

		if as.ObservedGeneration > resourceGen {
			continue
		}

		var conditions []api.AdapterCondition
		if len(as.Conditions) > 0 {
			if err := json.Unmarshal(as.Conditions, &conditions); err != nil {
				logger.With(ctx, "adapter", as.Adapter).WithError(err).
					Error("Failed to unmarshal adapter status conditions; skipping adapter")
				continue
			}
		}

		var avail *api.AdapterCondition
		for i := range conditions {
			if conditions[i].Type == api.ConditionTypeAvailable {
				avail = &conditions[i]
				break
			}
		}
		if avail == nil {
			continue
		}

		if avail.Status != api.AdapterConditionTrue && avail.Status != api.AdapterConditionFalse {
			continue
		}

		var finalized *api.AdapterCondition
		for i := range conditions {
			if conditions[i].Type == api.ConditionTypeFinalized {
				finalized = &conditions[i]
				break
			}
		}

		out[as.Adapter] = adapterAvailableSnapshot{
			observedTime:       obsTime,
			availableTrue:      avail.Status == api.AdapterConditionTrue,
			finalizedTrue:      finalized != nil && finalized.Status == api.AdapterConditionTrue,
			observedGeneration: as.ObservedGeneration,
			reason:             avail.Reason,
			message:            avail.Message,
		}
	}

	return out
}

// buildReadyFalseMessage returns the diagnostic message for a False Ready condition,
// listing which required adapters are not yet reporting Available=True at the current
// generation and which adapters have sent any report at all.
func buildReadyFalseMessage(
	required []string, byAdapter map[string]adapterAvailableSnapshot, resourceGen int32,
) string {
	var notReady, reporting []string
	for _, name := range required {
		s, ok := byAdapter[name]
		if !ok || !s.availableTrue || s.observedGeneration != resourceGen {
			notReady = append(notReady, name)
		}
		if ok {
			reporting = append(reporting, name)
		}
	}
	sort.Strings(notReady)
	sort.Strings(reporting)
	return fmt.Sprintf(
		"Required adapters not reporting Available=True: [%s]. Currently reporting: [%s]",
		strings.Join(notReady, ", "),
		strings.Join(reporting, ", "),
	)
}

// buildFinalizedFalseMessage returns the diagnostic message for a False Reconciled condition during deletion,
// listing which required adapters are not yet reporting Finalized=True at the current generation.
func buildFinalizedFalseMessage(
	required []string, byAdapter map[string]adapterAvailableSnapshot, resourceGen int32,
) string {
	var notFinalized, reporting []string
	for _, name := range required {
		s, ok := byAdapter[name]
		if !ok || !s.finalizedTrue || s.observedGeneration != resourceGen {
			notFinalized = append(notFinalized, name)
		}
		if ok {
			reporting = append(reporting, name)
		}
	}
	sort.Strings(notFinalized)
	sort.Strings(reporting)
	return fmt.Sprintf(
		"Required adapters not reporting Finalized=True: [%s]. Currently reporting: [%s]",
		strings.Join(notFinalized, ", "),
		strings.Join(reporting, ", "),
	)
}

func computeReady(
	resourceGen int32,
	refTime time.Time,
	prev *api.ResourceCondition,
	required []string,
	byAdapter map[string]adapterAvailableSnapshot,
) api.ResourceCondition {
	allAtCurrent := true
	for _, name := range required {
		s, ok := byAdapter[name]
		if !ok || !s.availableTrue || s.observedGeneration != resourceGen {
			allAtCurrent = false
			break
		}
	}

	status := api.ConditionFalse
	if len(required) > 0 && allAtCurrent {
		status = api.ConditionTrue
	}

	lastUpdated := computeReadyLastUpdatedTime(
		resourceGen, refTime, required, byAdapter,
	)
	lastTransition := computeReadyLastTransitionTime(
		resourceGen, refTime, prev, status, lastUpdated,
	)

	created := refTime
	if prev != nil && !prev.CreatedTime.IsZero() {
		created = prev.CreatedTime
	}

	reason := reasonMissingRequiredAdapters
	message := buildReadyFalseMessage(required, byAdapter, resourceGen)
	if status == api.ConditionTrue {
		reason = reasonAllAdaptersAvailable
		message = reason
	}

	return api.ResourceCondition{
		Type:               api.ConditionTypeReady,
		Status:             status,
		ObservedGeneration: resourceGen,
		Reason:             strPtr(reason),
		Message:            strPtr(message),
		CreatedTime:        created,
		LastUpdatedTime:    lastUpdated,
		LastTransitionTime: lastTransition,
	}
}

// computeReconciled synthesizes the Reconciled condition from adapter reports.
// Its meaning adapts based on resource lifecycle:
//   - Normal (deletedTime == nil): True when all required adapters report Available=True at current generation.
//   - Deletion (deletedTime != nil): True when all required adapters report Finalized=True at current generation.
func computeReconciled(
	resourceGen int32,
	refTime time.Time,
	deletedTime *time.Time,
	prev *api.ResourceCondition,
	required []string,
	byAdapter map[string]adapterAvailableSnapshot,
) api.ResourceCondition {
	allAtCurrent := true
	for _, name := range required {
		s, ok := byAdapter[name]
		if !ok || s.observedGeneration != resourceGen {
			allAtCurrent = false
			break
		}
		// Normal lifecycle: reconciled when all adapters report Available=True
		// Deletion lifecycle: reconciled when all adapters report Finalized=True
		conditionMet := s.availableTrue
		if deletedTime != nil {
			conditionMet = s.finalizedTrue
		}
		if !conditionMet {
			allAtCurrent = false
			break
		}
	}

	status := api.ConditionFalse
	if len(required) > 0 && allAtCurrent {
		status = api.ConditionTrue
	}

	lastUpdated := computeReadyLastUpdatedTime(
		resourceGen, refTime, required, byAdapter,
	)
	lastTransition := computeReadyLastTransitionTime(
		resourceGen, refTime, prev, status, lastUpdated,
	)

	created := refTime
	if prev != nil && !prev.CreatedTime.IsZero() {
		created = prev.CreatedTime
	}

	var reason, message string
	if deletedTime != nil {
		reason = reasonMissingRequiredAdapters
		message = buildFinalizedFalseMessage(required, byAdapter, resourceGen)
		if status == api.ConditionTrue {
			reason = reasonAllAdaptersReconciled
			message = reason
		}
	} else {
		reason = reasonMissingRequiredAdapters
		message = buildReadyFalseMessage(required, byAdapter, resourceGen)
		if status == api.ConditionTrue {
			reason = reasonAllAdaptersReconciled
			message = reason
		}
	}

	return api.ResourceCondition{
		Type:               api.ConditionTypeReconciled,
		Status:             status,
		ObservedGeneration: resourceGen,
		Reason:             strPtr(reason),
		Message:            strPtr(message),
		CreatedTime:        created,
		LastUpdatedTime:    lastUpdated,
		LastTransitionTime: lastTransition,
	}
}

func computeReadyLastUpdatedTime(
	resourceGen int32,
	refTime time.Time,
	required []string,
	byAdapter map[string]adapterAvailableSnapshot,
) time.Time {
	// Collect observed times for all required adapters that have reported at the current generation,
	// regardless of their Available status. When none have reported yet, fall back to refTime.
	atGen := make([]time.Time, 0, len(required))
	for _, name := range required {
		s, ok := byAdapter[name]
		if !ok || s.observedGeneration != resourceGen {
			continue
		}
		atGen = append(atGen, s.observedTime)
	}

	if len(atGen) == 0 {
		return refTime
	}

	return minTime(atGen)
}

func computeReadyLastTransitionTime(
	resourceGen int32,
	refTime time.Time,
	prev *api.ResourceCondition,
	newStatus api.ResourceConditionStatus,
	newLastUpdated time.Time,
) time.Time {
	if prev == nil {
		return newLastUpdated
	}
	if prev.Status == newStatus {
		return prev.LastTransitionTime
	}
	// Status changed.
	if prev.Status == api.ConditionTrue && newStatus == api.ConditionFalse &&
		resourceGen > prev.ObservedGeneration {
		// Generation bump: spec — last_transition_time becomes resource.last_update_time if status was True.
		return refTime
	}
	return newLastUpdated
}

// computeAvailableStatus decides the Available condition status from normalized adapter snapshots.
//
// Rules (in order):
//  1. No required adapters, or any required adapter has not yet reported → False.
//  2. All adapters True at a uniform generation, or mixed-gen but aggregate was already True → True.
//  3. Some adapter is False, but aggregate was True and no False is at the tracked generation → True (sticky).
//  4. Otherwise → False.
func computeAvailableStatus(
	prev *api.ResourceCondition,
	required []string,
	byAdapter map[string]adapterAvailableSnapshot,
	allTrue bool,
	mixed bool,
) api.ResourceConditionStatus {
	if len(required) == 0 {
		return api.ConditionFalse
	}
	for _, name := range required {
		if _, ok := byAdapter[name]; !ok {
			return api.ConditionFalse
		}
	}

	if allTrue {
		// Uniform generation (not mixed) is unambiguously True.
		// Mixed generation keeps True only when the aggregate was already True.
		if !mixed || (prev != nil && prev.Status == api.ConditionTrue) {
			return api.ConditionTrue
		}
		return api.ConditionFalse
	}

	// Some adapter reports False: preserve True only when no failure lands on the tracked generation.
	if prev == nil || prev.Status != api.ConditionTrue {
		return api.ConditionFalse
	}
	tracked := prev.ObservedGeneration
	for _, name := range required {
		s := byAdapter[name]
		if !s.availableTrue && s.observedGeneration == tracked {
			return api.ConditionFalse
		}
	}
	return api.ConditionTrue
}

func computeAvailable(
	refTime time.Time,
	prev *api.ResourceCondition,
	required []string,
	byAdapter map[string]adapterAvailableSnapshot,
) api.ResourceCondition {
	allTrue, commonGen, mixed := sameGenerationAllTrue(required, byAdapter)
	status := computeAvailableStatus(prev, required, byAdapter, allTrue, mixed)

	obsGen := computeAvailableObservedGeneration(status, prev, required, byAdapter, allTrue, commonGen, mixed)

	lastUpdated := computeAvailableLastUpdatedTime(
		status, prev, refTime, required, byAdapter, obsGen, allTrue, mixed,
	)
	lastTransition := computeGenericLastTransitionTime(prev, status, lastUpdated)

	created := refTime
	if prev != nil && !prev.CreatedTime.IsZero() {
		created = prev.CreatedTime
	}

	reason := "AdaptersNotAtSameGeneration"
	message := "Required adapters do not report a consistent Available state"
	if status == api.ConditionTrue {
		reason = "All required adapters report Available=True for the tracked generation"
		message = reason
	}

	return api.ResourceCondition{
		Type:               api.ConditionTypeAvailable,
		Status:             status,
		ObservedGeneration: obsGen,
		Reason:             strPtr(reason),
		Message:            strPtr(message),
		CreatedTime:        created,
		LastUpdatedTime:    lastUpdated,
		LastTransitionTime: lastTransition,
	}
}

func sameGenerationAllTrue(
	required []string,
	byAdapter map[string]adapterAvailableSnapshot,
) (allTrue bool, gen int32, mixed bool) {
	if len(required) == 0 {
		return true, 1, false
	}

	var g *int32
	for _, name := range required {
		s, ok := byAdapter[name]
		if !ok || !s.availableTrue {
			return false, 0, false
		}
		if g == nil {
			v := s.observedGeneration
			g = &v
		} else if *g != s.observedGeneration {
			mixed = true
		}
	}
	return true, *g, mixed
}

func computeAvailableObservedGeneration(
	status api.ResourceConditionStatus,
	prev *api.ResourceCondition,
	required []string,
	byAdapter map[string]adapterAvailableSnapshot,
	allTrue bool,
	commonGen int32,
	mixed bool,
) int32 {
	if len(required) == 0 {
		return 1
	}

	if status == api.ConditionTrue {
		if allTrue && !mixed {
			return commonGen
		}
		if prev != nil {
			return prev.ObservedGeneration
		}
		return 1
	}

	// False
	maxG := int32(0)
	for _, name := range required {
		s, ok := byAdapter[name]
		if !ok {
			continue
		}
		if s.observedGeneration > maxG {
			maxG = s.observedGeneration
		}
	}
	if maxG == 0 {
		if prev != nil {
			return prev.ObservedGeneration
		}
		return 1
	}
	return maxG
}

func computeAvailableLastUpdatedTime(
	status api.ResourceConditionStatus,
	prev *api.ResourceCondition,
	refTime time.Time,
	required []string,
	byAdapter map[string]adapterAvailableSnapshot,
	observedGen int32,
	allTrue bool,
	mixed bool,
) time.Time {
	if len(required) == 0 {
		return refTime
	}

	if allTrue && !mixed {
		times := make([]time.Time, 0, len(required))
		for _, name := range required {
			s := byAdapter[name]
			times = append(times, s.observedTime)
		}
		return minTime(times)
	}

	if allTrue && mixed && status == api.ConditionTrue {
		if prev != nil {
			return prev.LastUpdatedTime
		}
		return refTime
	}

	if status == api.ConditionFalse {
		x := observedGen
		hasFalseAtX := false
		for _, name := range required {
			s, ok := byAdapter[name]
			if !ok {
				continue
			}
			if s.observedGeneration == x && !s.availableTrue {
				hasFalseAtX = true
				break
			}
		}
		if hasFalseAtX {
			var atX []time.Time
			for _, name := range required {
				s, ok := byAdapter[name]
				if !ok {
					continue
				}
				if s.observedGeneration == x {
					atX = append(atX, s.observedTime)
				}
			}
			if len(atX) > 0 {
				return minTime(atX)
			}
		}
	}

	if prev != nil {
		return prev.LastUpdatedTime
	}
	return refTime
}

func computeGenericLastTransitionTime(
	prev *api.ResourceCondition,
	newStatus api.ResourceConditionStatus,
	newLastUpdated time.Time,
) time.Time {
	if prev == nil {
		return newLastUpdated
	}
	if prev.Status == newStatus {
		return prev.LastTransitionTime
	}
	return newLastUpdated
}

// computeAdapterConditions produces one ResourceCondition per required adapter that has reported.
// The condition type is derived from the adapter name via MapAdapterToConditionType.
// Status, reason, and message are taken from the adapter's Available condition snapshot.
// last_transition_time is updated only when the status (True/False) changes from the previous value.
func computeAdapterConditions(
	required []string,
	byAdapter map[string]adapterAvailableSnapshot,
	prevByType map[string]*api.ResourceCondition,
	refTime time.Time,
) []api.ResourceCondition {
	result := make([]api.ResourceCondition, 0, len(required))
	for _, adapterName := range required {
		snap, ok := byAdapter[adapterName]
		if !ok {
			continue
		}
		condType := MapAdapterToConditionType(adapterName)
		prev := prevByType[condType]

		status := api.ConditionFalse
		if snap.availableTrue {
			status = api.ConditionTrue
		}

		created := refTime
		if prev != nil && !prev.CreatedTime.IsZero() {
			created = prev.CreatedTime
		}

		lastUpdated := snap.observedTime

		lastTransition := lastUpdated
		if prev != nil && prev.Status == status {
			lastTransition = prev.LastTransitionTime
		}

		result = append(result, api.ResourceCondition{
			Type:               condType,
			Status:             status,
			ObservedGeneration: snap.observedGeneration,
			Reason:             snap.reason,
			Message:            snap.message,
			CreatedTime:        created,
			LastUpdatedTime:    lastUpdated,
			LastTransitionTime: lastTransition,
		})
	}
	return result
}

func minTime(times []time.Time) time.Time {
	if len(times) == 0 {
		return time.Time{}
	}
	if len(times) == 1 {
		return times[0]
	}
	t0 := times[0]
	for _, t := range times[1:] {
		if t.Before(t0) {
			t0 = t
		}
	}
	return t0
}

func strPtr(s string) *string {
	return &s
}
