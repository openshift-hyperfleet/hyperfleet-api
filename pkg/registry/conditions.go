package registry

import (
	"fmt"

	"cel.dev/cel-go/cel"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/util"
)

// Reserved condition types that cannot be overridden by mapping rules
// Using string literals to avoid import cycle with pkg/api
var reservedConditionTypes = map[string]bool{
	"Reconciled":          true, // api.ResourceConditionTypeReconciled
	"LastKnownReconciled": true, // api.ResourceConditionTypeLastKnownReconciled
}

// Field length constraints
const (
	MaxConditionTypeLength    = 100
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
	Type   string            `mapstructure:"type" json:"type" validate:"required"`
	When   MappingExpression `mapstructure:"when" json:"when" validate:"required"`
	Output MappingOutput     `mapstructure:"output" json:"output" validate:"required"`
}

// ValidateEntityConditions validates condition mappings for a single entity descriptor
// Used by registry.Validate() to check conditions inline in entity descriptors
//
// entities: all registered entity descriptors (needed to compute per-adapter synthesized types)
// descriptor: the specific entity descriptor being validated
func ValidateEntityConditions(entities []EntityDescriptor, descriptor EntityDescriptor) error {
	if len(descriptor.Conditions) == 0 {
		return nil
	}

	// Build reserved types from all entities (adapter-synthesized types are global)
	reserved := buildReservedConditionTypes(entities)

	// Create CEL environment once
	env, err := util.NewConditionMappingEnvironment()
	if err != nil {
		return fmt.Errorf("failed to create CEL environment for validation: %w", err)
	}

	// Validate each condition mapping rule and detect duplicates
	seen := make(map[string]bool, len(descriptor.Conditions))
	for _, rule := range descriptor.Conditions {
		// Check for duplicate types (fail-fast, consistent with CEL validation)
		if seen[rule.Type] {
			return fmt.Errorf(
				"%s condition type '%s' is defined multiple times (each type must be unique)",
				descriptor.Kind, rule.Type,
			)
		}
		seen[rule.Type] = true

		if err := validateConditionMapping(descriptor.Kind, rule.Type, rule, reserved, env); err != nil {
			return err
		}
	}

	return nil
}

// buildReservedConditionTypes computes the full set of reserved condition types:
//   - Static types: Reconciled, LastKnownReconciled
//   - Per-adapter synthesized types: computed from required_adapters in all entity descriptors
//     (e.g., "validation" → "ValidationSuccessful")
func buildReservedConditionTypes(entities []EntityDescriptor) map[string]bool {
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
	// Check empty type - YAML can have empty string keys
	if condType == "" {
		return fmt.Errorf(
			"%s condition type cannot be empty",
			resourceType,
		)
	}

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
