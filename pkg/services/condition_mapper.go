package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/util"
)

// emptyEnvMap is a shared empty map for the env CEL variable (PERF-03)
// Hoisted to package level to avoid allocation on every Apply() call
// Safe to share because it is never mutated (env variables not yet implemented)
var emptyEnvMap = map[string]string{}

// CEL adapter status map keys (QUAL-01)
// Used in both nil-guard and normal paths to prevent key mismatches
const (
	celKeyAdapter            = "adapter"
	celKeyObservedGeneration = "observed_generation"
	celKeyConditions         = "conditions"
	celKeyData               = "data"
)

// CEL boolean string representations (QUAL-01)
// Used in status parsing (evaluateRule) and extractString to ensure consistency
const (
	celBoolTrue  = "True"  // Title-case matches extractString output
	celBoolFalse = "False" // Title-case matches extractString output
)

// Condition sub-field keys (QUAL-01)
// Used when building adapter condition maps for CEL context
const (
	condKeyType    = "type"
	condKeyStatus  = "status"
	condKeyReason  = "reason"
	condKeyMessage = "message"
)

// Resource field keys (QUAL-01)
// Used when extracting fields from resource map in CEL context
const (
	resourceKeyGeneration = "generation"
)

// ConditionMapper evaluates CEL-based condition mapping rules
type ConditionMapper struct {
	rules          map[string]*compiledRule
	cachedResource *cachedResourceContext // Cache for masked resource map (PERF-03)
	resourceKind   string
	sortedNames    []string     // Pre-sorted rule names for deterministic ordering
	mu             sync.RWMutex // Protects cachedResource from concurrent access
}

// cachedResourceContext caches the masked resource map to avoid redundant
// marshal + MaskSensitiveFields operations across adapter status reports.
//
// Cache Correctness Rationale:
//   - Resource.spec/metadata only change on PATCH operations, which bump generation
//   - Resource.Conditions has json:"-" tag, so it's excluded from json.Marshal()
//   - ProcessAdapterStatus() never modifies the Resource object itself
//   - UpdatedTime is included in the map BUT doesn't change during status updates
//     (only resource_conditions table is modified, not the resource row)
//   - Therefore, cache key (resourceID + generation) is sufficient for correctness
//
// Performance Note: Single-entry cache may thrash under high concurrent load
// processing different resources. Consider LRU if profiling shows contention.
type cachedResourceContext struct {
	maskedMap  map[string]interface{}
	resourceID string
	generation int32
}

// compiledRule holds pre-compiled CEL programs for a mapping rule
type compiledRule struct {
	whenProgram    cel.Program
	statusProgram  cel.Program
	reasonProgram  cel.Program
	messageProgram cel.Program
	conditionType  string
}

// ApplyInput holds the input data for condition mapping
type ApplyInput struct {
	AdapterStatuses api.AdapterStatusList
	Resource        interface{}
	RefTime         time.Time
	// PrevConditions is the previous mapped conditions from resource.Conditions
	// Used to preserve CreatedTime and LastTransitionTime (matching aggregation.go pattern)
	PrevConditions []api.ResourceCondition
}

// NewConditionMapper creates a new condition mapper with pre-compiled rules
func NewConditionMapper(resourceKind string, rules []registry.ConditionMappingRule) (*ConditionMapper, error) {
	// Create CEL environment with context variables and custom functions
	// Uses the same environment as validation to ensure consistency
	env, err := util.NewConditionMappingEnvironment()
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	// Compile all rules at initialization (fail-fast)
	compiled := make(map[string]*compiledRule, len(rules))
	for _, rule := range rules {
		compiledRule, err := compileRule(env, rule.Type, rule)
		if err != nil {
			return nil, fmt.Errorf("failed to compile rule for condition %s: %w", rule.Type, err)
		}
		compiled[rule.Type] = compiledRule
	}

	// Pre-sort rule names for deterministic ordering (avoids repeated sort in Apply)
	sortedNames := make([]string, 0, len(compiled))
	for name := range compiled {
		sortedNames = append(sortedNames, name)
	}
	sort.Strings(sortedNames)

	return &ConditionMapper{
		rules:        compiled,
		sortedNames:  sortedNames,
		resourceKind: resourceKind,
	}, nil
}

// Apply evaluates mapping rules and returns mapped conditions.
// Returns error on CEL evaluation failure to trigger transaction rollback per design doc.
// This ensures timely retry (10s) instead of delayed retry (30min) that would occur
// if partial results were committed without mapped conditions.
func (m *ConditionMapper) Apply(ctx context.Context, input ApplyInput) ([]api.ResourceCondition, error) {
	if len(m.rules) == 0 {
		return nil, nil
	}

	// Build CEL activation context (filtering Unknown conditions happens inside buildActivationWithCache)
	// Use cached resource map when possible to avoid redundant marshal + mask operations (PERF-03)
	activation := m.buildActivationWithCache(ctx, input.AdapterStatuses, input.Resource)

	// Build lookup map for previous conditions to avoid O(N×M) linear scans
	prevConditionsByType := make(map[string]*api.ResourceCondition, len(input.PrevConditions))
	for i := range input.PrevConditions {
		prevConditionsByType[input.PrevConditions[i].Type] = &input.PrevConditions[i]
	}

	// Evaluate each rule in sorted order to produce deterministic condition ordering
	// This prevents spurious DB writes from jsonEqual comparison in resource.go
	// Pre-allocate with capacity to avoid incremental growth
	mappedConditions := make([]api.ResourceCondition, 0, len(m.sortedNames))
	for _, name := range m.sortedNames {
		rule := m.rules[name]

		// Lookup previous condition for this type (O(1) instead of O(N))
		prevCondition := prevConditionsByType[rule.conditionType]

		condition, err := m.evaluateRule(ctx, rule, activation, input.RefTime, prevCondition)
		if err != nil {
			// Return error to trigger transaction rollback (per design doc)
			// This ensures adapter status update is retried in 10s instead of 30min
			return nil, fmt.Errorf(
				"condition mapping failed for %s.%s: %w",
				m.resourceKind, rule.conditionType, err,
			)
		}
		if condition != nil {
			mappedConditions = append(mappedConditions, *condition)
		}
	}

	return mappedConditions, nil
}

// evaluateRule evaluates a single mapping rule
func (m *ConditionMapper) evaluateRule(
	ctx context.Context,
	rule *compiledRule,
	activation map[string]interface{},
	refTime time.Time,
	prevCondition *api.ResourceCondition,
) (*api.ResourceCondition, error) {
	// Evaluate when expression
	whenResult, _, err := rule.whenProgram.Eval(activation)
	if err != nil {
		return nil, fmt.Errorf("when expression evaluation failed: %w", err)
	}

	// Check if when condition is met
	whenBool, ok := whenResult.Value().(bool)
	if !ok {
		return nil, fmt.Errorf("when expression did not return boolean (got %T)", whenResult.Value())
	}
	if !whenBool {
		// Condition not met, skip this rule
		return nil, nil
	}

	// Evaluate output expressions
	statusResult, _, err := rule.statusProgram.Eval(activation)
	if err != nil {
		return nil, fmt.Errorf("status expression evaluation failed: %w", err)
	}

	reasonResult, _, err := rule.reasonProgram.Eval(activation)
	if err != nil {
		return nil, fmt.Errorf("reason expression evaluation failed: %w", err)
	}

	messageResult, _, err := rule.messageProgram.Eval(activation)
	if err != nil {
		return nil, fmt.Errorf("message expression evaluation failed: %w", err)
	}

	// Extract and validate values
	statusStr := extractString(statusResult)
	reasonStr := extractString(reasonResult)
	messageStr := extractString(messageResult)

	// Validate field lengths and truncate message if needed (QUAL-03)
	validatedReason, validatedMessage, err := m.validateFieldLengths(ctx, rule, reasonStr, messageStr)
	if err != nil {
		// Propagate error to trigger rollback (consistent with CEL evaluation error handling)
		return nil, fmt.Errorf("field validation failed for %s: %w", rule.conditionType, err)
	}

	// Build the mapped condition with all required fields (QUAL-03)
	return m.buildMappedCondition(
		ctx, rule, statusStr, validatedReason, validatedMessage, activation, refTime, prevCondition,
	)
}

// validateFieldLengths validates and enforces field length constraints (QUAL-03)
// Returns (validatedReason, validatedMessage, error)
// If error is non-nil, the condition should be skipped
func (m *ConditionMapper) validateFieldLengths(
	ctx context.Context,
	rule *compiledRule,
	reasonStr string,
	messageStr string,
) (string, string, error) {
	// Validate condition type length
	if len(rule.conditionType) > registry.MaxConditionTypeLength {
		logger.With(
			ctx,
			"resource_kind", m.resourceKind,
			"condition_type", rule.conditionType,
			"length", len(rule.conditionType),
		).Warn("Condition type exceeds max length, skipping")
		return "", "", fmt.Errorf("condition type exceeds max length")
	}

	// Truncate reason if too long (rune-aware to preserve valid UTF-8)
	validatedReason := reasonStr
	if len(reasonStr) > registry.MaxConditionReasonLength {
		validatedReason = truncateUTF8(reasonStr, registry.MaxConditionReasonLength)
		logger.With(ctx, "resource_kind", m.resourceKind, "condition_type", rule.conditionType).
			Info("Condition reason truncated to max length")
	}

	// Truncate message if too long (rune-aware to preserve valid UTF-8)
	validatedMessage := messageStr
	if len(messageStr) > registry.MaxConditionMessageLength {
		validatedMessage = truncateUTF8(messageStr, registry.MaxConditionMessageLength)
		logger.With(ctx, "resource_kind", m.resourceKind, "condition_type", rule.conditionType).
			Info("Condition message truncated to max length")
	}

	return validatedReason, validatedMessage, nil
}

// buildMappedCondition builds the final ResourceCondition from validated inputs (QUAL-03)
func (m *ConditionMapper) buildMappedCondition(
	ctx context.Context,
	rule *compiledRule,
	statusStr string,
	reasonStr string,
	messageStr string,
	activation map[string]interface{},
	refTime time.Time,
	prevCondition *api.ResourceCondition,
) (*api.ResourceCondition, error) {
	// Parse status string to ResourceConditionStatus (case-insensitive)
	var status api.ResourceConditionStatus
	switch {
	case strings.EqualFold(statusStr, celBoolTrue):
		status = api.ConditionTrue
	case strings.EqualFold(statusStr, celBoolFalse):
		status = api.ConditionFalse
	default:
		return nil, fmt.Errorf(
			"invalid status value: %s (must be %s or %s)",
			statusStr, celBoolTrue, celBoolFalse,
		)
	}

	resourceGen := extractResourceGeneration(ctx, activation, m.resourceKind, rule.conditionType)

	// Preserve CreatedTime from previous condition (matching aggregation.go:332-333)
	createdTime := refTime.UTC().Truncate(time.Microsecond)
	if prevCondition != nil && !prevCondition.CreatedTime.IsZero() {
		createdTime = prevCondition.CreatedTime
	}

	// Preserve LastTransitionTime if status unchanged (matching aggregation.go:640-652)
	lastTransitionTime := refTime.UTC().Truncate(time.Microsecond)
	if prevCondition != nil && prevCondition.Status == status && !prevCondition.LastTransitionTime.IsZero() {
		lastTransitionTime = prevCondition.LastTransitionTime
	}

	// Build the mapped condition
	// Use refTime (not time.Now) to ensure reproducible results - same pattern as aggregation.go:63-64
	condition := api.ResourceCondition{
		Type:               rule.conditionType,
		Status:             status,
		Reason:             strPtr(reasonStr),
		Message:            strPtr(messageStr),
		ObservedGeneration: resourceGen,
		LastTransitionTime: lastTransitionTime,
		CreatedTime:        createdTime,
		LastUpdatedTime:    refTime.UTC().Truncate(time.Microsecond),
	}

	return &condition, nil
}

func extractResourceGeneration(
	ctx context.Context,
	activation map[string]interface{},
	resourceKind, conditionType string,
) int32 {
	resourceMap, ok := activation[util.CELVarResource].(map[string]interface{})
	if !ok {
		return 0
	}
	gen, ok := resourceMap[resourceKeyGeneration].(float64)
	if !ok {
		return 0
	}
	if gen < math.MinInt32 || gen > math.MaxInt32 {
		logger.With(ctx, "resource_kind", resourceKind, "condition_type", conditionType, "generation", gen).
			Warn("Resource generation out of int32 range, using 0")
		return 0
	}
	return int32(gen)
}

// compileRule compiles all CEL expressions in a mapping rule
func compileRule(env *cel.Env, condType string, rule registry.ConditionMappingRule) (*compiledRule, error) {
	// Compile when expression
	whenPrg, err := compileExpression(env, rule.When.Expression)
	if err != nil {
		return nil, fmt.Errorf("when: %w", err)
	}

	// Compile output expressions
	statusPrg, err := compileExpression(env, rule.Output.Status.Expression)
	if err != nil {
		return nil, fmt.Errorf("output.status: %w", err)
	}

	reasonPrg, err := compileExpression(env, rule.Output.Reason.Expression)
	if err != nil {
		return nil, fmt.Errorf("output.reason: %w", err)
	}

	messagePrg, err := compileExpression(env, rule.Output.Message.Expression)
	if err != nil {
		return nil, fmt.Errorf("output.message: %w", err)
	}

	return &compiledRule{
		conditionType:  condType,
		whenProgram:    whenPrg,
		statusProgram:  statusPrg,
		reasonProgram:  reasonPrg,
		messageProgram: messagePrg,
	}, nil
}

// compileExpression compiles a single CEL expression
func compileExpression(env *cel.Env, expression string) (cel.Program, error) {
	ast, issues := env.Parse(expression)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("parse error: %w", issues.Err())
	}

	// Check for function arity errors and undefined functions
	// Matches the validation pipeline in conditions.go for consistent fail-fast behavior
	_, issues = env.Check(ast)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("check error: %w", issues.Err())
	}

	// Bound CEL evaluation cost to prevent CPU spikes from misconfigured expressions.
	// Status aggregation is on the hot path (every adapter status update triggers it).
	// Cost limit allows moderately complex expressions while preventing
	// pathological cases (e.g., nested loops, unbounded string operations).
	prg, err := env.Program(ast, cel.CostLimit(util.CELCostLimit))
	if err != nil {
		return nil, fmt.Errorf("program error: %w", err)
	}

	return prg, nil
}

// buildActivationWithCache builds the CEL activation context using cached resource map when possible.
// Caches the masked resource map to avoid redundant marshal + MaskSensitiveFields on every Apply().
// The resource (spec, metadata) only changes on PATCH operations, not adapter status reports.
func (m *ConditionMapper) buildActivationWithCache(
	ctx context.Context,
	statuses api.AdapterStatusList,
	resource interface{},
) map[string]interface{} {
	// Build statuses list using shared logic (PERF-03: avoid duplication)
	statusesList := buildStatusesList(ctx, statuses)

	// Get cached or build resource map (cache invalidates on ID/generation change)
	resourceMap := m.getCachedOrBuildResource(ctx, resource)

	return map[string]interface{}{
		util.CELVarStatuses: statusesList,
		util.CELVarResource: resourceMap,
		util.CELVarEnv:      emptyEnvMap,
	}
}

// getCachedOrBuildResource returns the masked resource map, using cache when valid.
// Cache key: resourceID + generation. Invalidates automatically on PATCH (generation bump).
// Thread-safe: protected by RWMutex to prevent data races when different resources of the
// same kind are processed concurrently (GetForUpdate only serializes same resourceID).
func (m *ConditionMapper) getCachedOrBuildResource(
	ctx context.Context,
	resource interface{},
) map[string]interface{} {
	r, ok := resource.(*api.Resource)
	if !ok {
		// Fallback for non-Resource types (shouldn't happen in production)
		return util.MaskSensitiveFields(resourceToMap(ctx, resource, m.resourceKind))
	}

	// Try cache read (RLock allows concurrent reads).
	// IMPORTANT: callers MUST NOT mutate the returned map — it may be a cached reference.
	// CEL evaluation is read-only, so this is safe for current usage.
	m.mu.RLock()
	if m.cachedResource != nil &&
		m.cachedResource.resourceID == r.ID &&
		m.cachedResource.generation == r.Generation {
		cached := m.cachedResource.maskedMap
		m.mu.RUnlock()
		return cached
	}
	m.mu.RUnlock()

	// Cache miss: rebuild (expensive operation outside of lock)
	maskedMap := util.MaskSensitiveFields(resourceToMap(ctx, resource, m.resourceKind))

	// Update cache (Lock for exclusive write access)
	m.mu.Lock()
	m.cachedResource = &cachedResourceContext{
		resourceID: r.ID,
		generation: r.Generation,
		maskedMap:  maskedMap,
	}
	m.mu.Unlock()

	return maskedMap
}

// buildStatusesList converts adapter statuses to CEL-compatible format, filtering Unknown conditions.
// Extracted to shared function to avoid duplication (PERF-03).
func buildStatusesList(ctx context.Context, statuses api.AdapterStatusList) []interface{} {
	statusesList := make([]interface{}, 0, len(statuses))
	for _, status := range statuses {
		// Skip nil entries (can occur in AdapterStatusList []*AdapterStatus)
		if status == nil {
			continue
		}
		// Parse conditions once and check for Unknown in same operation
		statusMap, hasUnknown := adapterStatusToMapWithUnknownCheck(ctx, status)
		if !hasUnknown {
			statusesList = append(statusesList, statusMap)
		}
	}
	return statusesList
}

// parseConditionsWithUnknownCheck unmarshals adapter conditions from JSONB and converts to CEL maps.
// Returns (conditions, hasUnknown) where hasUnknown indicates if any condition has Unknown status.
// Returns empty slice (not nil) on unmarshal failure so CEL receives [] instead of null.
func parseConditionsWithUnknownCheck(
	ctx context.Context,
	conditionsJSON []byte,
	adapterName string,
) ([]map[string]interface{}, bool) {
	// Initialize to empty slice (not nil) so CEL receives [] instead of null
	conditions := make([]map[string]interface{}, 0)
	hasUnknown := false

	if conditionsJSON == nil {
		return conditions, hasUnknown
	}

	var parsedConds []api.AdapterCondition
	if err := json.Unmarshal(conditionsJSON, &parsedConds); err != nil {
		// Unmarshal failure: return empty conditions array (ERR-02).
		// Degraded mode: statuses array contains entry with empty conditions, allowing
		// resource-level CEL expressions to still run (e.g., counting adapters).
		logger.With(ctx, "adapter", adapterName).
			WithError(err).
			Warn("Failed to unmarshal adapter conditions JSONB, using empty conditions array")
		return conditions, hasUnknown
	}

	// Preallocate with exact capacity to avoid reallocation (PERF-01)
	conditions = make([]map[string]interface{}, 0, len(parsedConds))
	for _, cond := range parsedConds {
		// Check for Unknown status while building the map (single pass)
		if cond.Status == api.AdapterConditionUnknown {
			hasUnknown = true
			// Break early - buildStatusesList will discard this entire statusMap
			break
		}

		condMap := map[string]interface{}{
			condKeyType:    cond.Type,
			condKeyStatus:  string(cond.Status),
			condKeyReason:  "",
			condKeyMessage: "",
		}
		// SEC-02: Adapter contract requires condition reason/message to be user-safe
		// (no secrets, credentials, or sensitive data). Currently unmasked to preserve
		// human-readable diagnostic messages. Defense-in-depth improvement (HYPERFLEET-1128):
		// Consider applying pattern-based string scanning to reason/message before placing
		// in CEL context, matching the approach already applied to adapter data blobs via
		// MaskSensitiveFields (e.g., regex for AWS keys, JWT tokens, base64 secrets).
		if cond.Reason != nil {
			condMap[condKeyReason] = *cond.Reason
		}
		if cond.Message != nil {
			condMap[condKeyMessage] = *cond.Message
		}
		conditions = append(conditions, condMap)
	}

	return conditions, hasUnknown
}

// parseAdapterData unmarshals adapter data from JSONB to a map.
// Returns empty map on unmarshal failure to maintain CEL context consistency.
func parseAdapterData(ctx context.Context, dataJSON []byte, adapterName string) map[string]interface{} {
	data := make(map[string]interface{})

	if dataJSON == nil {
		return data
	}

	if err := json.Unmarshal(dataJSON, &data); err != nil {
		// Reset to empty map on parse failure to maintain consistency
		logger.With(ctx, "adapter", adapterName).
			WithError(err).
			Warn("Failed to unmarshal adapter data JSONB, using empty map")
		return make(map[string]interface{})
	}

	return data
}

// adapterStatusToMapWithUnknownCheck converts an AdapterStatus to a CEL-compatible map
// and reports whether any condition has Unknown status.
// Returns (statusMap, hasUnknown) to allow filtering in a single pass (PERF-03).
func adapterStatusToMapWithUnknownCheck(ctx context.Context, status *api.AdapterStatus) (map[string]interface{}, bool) {
	// Guard against nil pointer (AdapterStatusList is []*AdapterStatus, so elements can be nil)
	if status == nil {
		return map[string]interface{}{
			celKeyAdapter:            "",
			celKeyObservedGeneration: float64(0),
			celKeyConditions:         []map[string]interface{}{},
			celKeyData:               map[string]interface{}{},
		}, false
	}

	// Parse conditions and check for Unknown status (QUAL-03)
	conditions, hasUnknown := parseConditionsWithUnknownCheck(ctx, status.Conditions, status.Adapter)

	// Early return if Unknown found - buildStatusesList will discard this statusMap anyway (PERF-03)
	// Skips data parsing and MaskSensitiveFields call to avoid wasted work
	if hasUnknown {
		return map[string]interface{}{
			celKeyAdapter:            status.Adapter,
			celKeyObservedGeneration: float64(status.ObservedGeneration),
			celKeyConditions:         conditions,
			celKeyData:               map[string]interface{}{},
		}, true
	}

	// Parse data field from JSONB (QUAL-03)
	data := parseAdapterData(ctx, status.Data, status.Adapter)

	statusMap := map[string]interface{}{
		celKeyAdapter: status.Adapter,
		// Normalize observed_generation to float64 for consistency with resource.generation
		// (which becomes float64 after JSON marshal/unmarshal round-trip)
		celKeyObservedGeneration: float64(status.ObservedGeneration),
		celKeyConditions:         conditions,
		// Mask sensitive fields before exposing to CEL evaluation context
		// This prevents accidental leakage of credentials in public API condition messages/reasons
		celKeyData: util.MaskSensitiveFields(data),
	}

	return statusMap, hasUnknown
}

// resourceToMap converts a resource to a CEL-compatible map
func resourceToMap(ctx context.Context, resource interface{}, resourceKind string) map[string]interface{} {
	// Use JSON marshaling for generic conversion
	data, err := json.Marshal(resource)
	if err != nil {
		// Marshal failure: return empty map (ERR-02).
		// Degraded mode: CEL expressions using resource.* will receive empty object.
		logger.With(ctx, "resource_kind", resourceKind).WithError(err).
			Warn("Failed to marshal resource to JSON, using empty map")
		return make(map[string]interface{})
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		// Unmarshal failure: return empty map (ERR-02).
		// Degraded mode: CEL expressions using resource.* will receive empty object.
		logger.With(ctx, "resource_kind", resourceKind).WithError(err).
			Warn("Failed to unmarshal resource JSON to map, using empty map")
		return make(map[string]interface{})
	}

	return result
}

// extractString extracts a string value from a CEL result
func extractString(result ref.Val) string {
	if types.IsUnknownOrError(result) {
		return ""
	}
	// Handle CEL null values explicitly to prevent "<nil>" in API responses
	if result.Equal(types.NullValue) == types.True {
		return ""
	}
	switch v := result.Value().(type) {
	case string:
		return v
	case bool:
		if v {
			return celBoolTrue
		}
		return celBoolFalse
	default:
		return fmt.Sprintf("%v", v)
	}
}

// truncateUTF8 truncates a string to maxBytes preserving valid UTF-8 encoding.
// It ensures we don't cut in the middle of a multi-byte character.
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}

	// Start from maxBytes and walk backward to find a valid rune boundary
	for i := maxBytes; i > 0; i-- {
		// Check if this position is a valid rune start
		if (s[i] & 0xC0) != 0x80 {
			// This is either ASCII (0xxxxxxx) or a multibyte start (11xxxxxx)
			// Verify the rune is complete
			r, size := utf8.DecodeRuneInString(s[i:])
			if r != utf8.RuneError && i+size <= len(s) {
				// Valid rune boundary, truncate here
				return s[:i]
			}
		}
	}

	// Fallback: return empty string if we can't find a valid boundary
	return ""
}
