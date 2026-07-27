package config

import (
	"fmt"
	"sort"

	"github.com/google/cel-go/cel"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/util"
)

// Reserved condition types that cannot be overridden by mapping rules
var reservedConditionTypes = map[string]bool{
	api.ResourceConditionTypeReconciled:          true,
	api.ResourceConditionTypeLastKnownReconciled: true,
}

// Field length constraints
const (
	MaxConditionTypeLength    = 128
	MaxConditionReasonLength  = 256
	MaxConditionMessageLength = 2048
)

// MappingExpression wraps a CEL expression string
type MappingExpression struct {
	Expression string `mapstructure:"expression" json:"expression" validate:"required"`
}

// MappingOutput defines the output expressions for a mapped condition
type MappingOutput struct {
	Status  MappingExpression `mapstructure:"status" json:"status" validate:"required"`
	Reason  MappingExpression `mapstructure:"reason" json:"reason" validate:"required"`
	Message MappingExpression `mapstructure:"message" json:"message" validate:"required"`
}

// ConditionMappingRule defines a single condition mapping rule
type ConditionMappingRule struct {
	When   MappingExpression `mapstructure:"when" json:"when" validate:"required"`
	Output MappingOutput     `mapstructure:"output" json:"output" validate:"required"`
}

// ConditionsConfig holds condition mapping configuration per resource type
// Map key is the output condition type (e.g., "LandingZoneReady")
type ConditionsConfig struct {
	Clusters  map[string]ConditionMappingRule `mapstructure:"clusters" json:"clusters"`
	NodePools map[string]ConditionMappingRule `mapstructure:"nodepools" json:"nodepools"`
}

// NewConditionsConfig returns a default ConditionsConfig with empty maps
func NewConditionsConfig() *ConditionsConfig {
	return &ConditionsConfig{
		Clusters:  make(map[string]ConditionMappingRule),
		NodePools: make(map[string]ConditionMappingRule),
	}
}

// Validate validates the conditions configuration
// Returns error on:
// - Reserved condition types (Reconciled, LastKnownReconciled, per-adapter synthesized types)
// - Invalid CEL expressions (fail-fast at startup)
// - Field length constraint violations
//
// entities: entity descriptors from ApplicationConfig.Entities - used to compute
// per-adapter synthesized condition types (e.g., ValidationSuccessful from "validation" adapter)
func (c *ConditionsConfig) Validate(entities []registry.EntityDescriptor) error {
	// Nil receiver guard - YAML can set `conditions: null`
	if c == nil {
		return nil
	}

	// Build the complete set of reserved types: static + per-adapter synthesized
	reserved := buildReservedConditionTypes(entities)

	// Create CEL environment once and reuse for all validations
	// This matches the pattern in NewConditionMapper and avoids recreating the environment per-expression
	env, err := util.NewConditionMappingEnvironment()
	if err != nil {
		return fmt.Errorf("failed to create CEL environment for validation: %w", err)
	}

	// Validate cluster mappings (sort keys for deterministic error messages)
	clusterKeys := make([]string, 0, len(c.Clusters))
	for condType := range c.Clusters {
		clusterKeys = append(clusterKeys, condType)
	}
	sort.Strings(clusterKeys)
	for _, condType := range clusterKeys {
		if err := validateConditionMapping("clusters", condType, c.Clusters[condType], reserved, env); err != nil {
			return err
		}
	}

	// Validate nodepool mappings (sort keys for deterministic error messages)
	nodepoolKeys := make([]string, 0, len(c.NodePools))
	for condType := range c.NodePools {
		nodepoolKeys = append(nodepoolKeys, condType)
	}
	sort.Strings(nodepoolKeys)
	for _, condType := range nodepoolKeys {
		if err := validateConditionMapping("nodepools", condType, c.NodePools[condType], reserved, env); err != nil {
			return err
		}
	}

	return nil
}

// buildReservedConditionTypes computes the full set of reserved condition types:
//   - Static types: Reconciled, LastKnownReconciled
//   - Per-adapter synthesized types: computed from required_adapters in all entity descriptors
//     (e.g., "validation" → "ValidationSuccessful")
func buildReservedConditionTypes(entities []registry.EntityDescriptor) map[string]bool {
	reserved := make(map[string]bool)

	// Add static reserved types
	for k, v := range reservedConditionTypes {
		reserved[k] = v
	}

	// Add per-adapter synthesized types from all entities
	seen := make(map[string]bool)
	for _, entity := range entities {
		for _, adapter := range entity.RequiredAdapters {
			// Skip duplicates across entities (e.g., "validation" appears in both Cluster and NodePool)
			if seen[adapter] {
				continue
			}
			seen[adapter] = true

			// Compute the synthesized condition type name using shared helper
			condType := util.MapAdapterToConditionType(adapter)
			reserved[condType] = true
		}
	}

	return reserved
}

// validateConditionMapping validates a single mapping rule
func validateConditionMapping(
	resourceType, condType string,
	rule ConditionMappingRule,
	reserved map[string]bool,
	env *cel.Env,
) error {
	// Check reserved types
	if reserved[condType] {
		return fmt.Errorf(
			"%s condition type '%s' is reserved and cannot be overridden by mapping rules",
			resourceType, condType,
		)
	}

	// Check condition type length
	if len(condType) > MaxConditionTypeLength {
		return fmt.Errorf(
			"%s condition type '%s' exceeds max length %d (got %d)",
			resourceType, condType, MaxConditionTypeLength, len(condType),
		)
	}

	// Validate CEL expressions
	if err := validateCELExpression(resourceType, condType, "when", rule.When.Expression, env); err != nil {
		return err
	}
	if err := validateCELExpression(
		resourceType, condType, "output.status", rule.Output.Status.Expression, env,
	); err != nil {
		return err
	}
	if err := validateCELExpression(
		resourceType, condType, "output.reason", rule.Output.Reason.Expression, env,
	); err != nil {
		return err
	}
	if err := validateCELExpression(
		resourceType, condType, "output.message", rule.Output.Message.Expression, env,
	); err != nil {
		return err
	}

	return nil
}

// validateCELExpression validates a CEL expression by attempting to compile it
// This provides fail-fast validation at startup
func validateCELExpression(resourceType, condType, field, expression string, env *cel.Env) error {
	// Parse expression
	ast, issues := env.Parse(expression)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf(
			"%s.%s.%s: invalid CEL expression: %w\nExpression: %s",
			resourceType, condType, field, issues.Err(), expression,
		)
	}

	// Check for function arity errors and undefined functions
	// While DynType limits compile-time type checking, Check still catches
	// undefined functions and incorrect argument counts
	_, issues = env.Check(ast)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf(
			"%s.%s.%s: CEL check failed: %w\nExpression: %s",
			resourceType, condType, field, issues.Err(), expression,
		)
	}

	// Create program with same cost limit as runtime (see util.CELCostLimit and compileExpression)
	// This ensures overly expensive expressions are rejected at startup, not runtime
	_, err := env.Program(ast, cel.CostLimit(util.CELCostLimit))
	if err != nil {
		return fmt.Errorf(
			"%s.%s.%s: failed to compile CEL expression: %w\nExpression: %s",
			resourceType, condType, field, err, expression,
		)
	}

	return nil
}

// IsEmpty returns true if no condition mappings are configured
func (c *ConditionsConfig) IsEmpty() bool {
	// Nil receiver guard - treat nil config as empty
	if c == nil {
		return true
	}
	return len(c.Clusters) == 0 && len(c.NodePools) == 0
}
