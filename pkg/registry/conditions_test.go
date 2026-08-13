package registry

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/util"
)

func TestValidateConditionMapping_EmptyType(t *testing.T) {
	g := NewWithT(t)

	env, err := util.NewConditionMappingEnvironment()
	g.Expect(err).ToNot(HaveOccurred())

	rule := ConditionMappingRule{
		Type: "", // Empty type
		When: MappingExpression{Expression: "true"},
		Output: MappingOutput{
			Status:  MappingExpression{Expression: `"True"`},
			Reason:  MappingExpression{Expression: `"Ready"`},
			Message: MappingExpression{Expression: `"All good"`},
		},
	}

	err = validateConditionMapping("Cluster", "", rule, map[string]bool{}, env)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("condition type cannot be empty"))
}

func TestValidateConditionMapping_ReservedTypes(t *testing.T) {
	g := NewWithT(t)

	env, err := util.NewConditionMappingEnvironment()
	g.Expect(err).ToNot(HaveOccurred())

	reserved := map[string]bool{
		"Reconciled":           true,
		"LastKnownReconciled":  true,
		"ValidationSuccessful": true, // Per-adapter synthesized
	}

	tests := []struct {
		name          string
		condType      string
		errorContains string
		expectError   bool
	}{
		{
			name:          "Reconciled is reserved",
			condType:      "Reconciled",
			expectError:   true,
			errorContains: "reserved and cannot be overridden",
		},
		{
			name:          "LastKnownReconciled is reserved",
			condType:      "LastKnownReconciled",
			expectError:   true,
			errorContains: "reserved and cannot be overridden",
		},
		{
			name:          "Per-adapter synthesized type is reserved",
			condType:      "ValidationSuccessful",
			expectError:   true,
			errorContains: "reserved and cannot be overridden",
		},
		{
			name:        "Custom type is allowed",
			condType:    "CustomReady",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			rule := ConditionMappingRule{
				Type: tt.condType,
				When: MappingExpression{Expression: "true"},
				Output: MappingOutput{
					Status:  MappingExpression{Expression: `"True"`},
					Reason:  MappingExpression{Expression: `"Ready"`},
					Message: MappingExpression{Expression: `"msg"`},
				},
			}

			err := validateConditionMapping("Cluster", tt.condType, rule, reserved, env)
			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.errorContains))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestValidateConditionMapping_OversizedType(t *testing.T) {
	g := NewWithT(t)

	env, err := util.NewConditionMappingEnvironment()
	g.Expect(err).ToNot(HaveOccurred())

	oversizedType := strings.Repeat("A", MaxConditionTypeLength+1)

	rule := ConditionMappingRule{
		Type: oversizedType,
		When: MappingExpression{Expression: "true"},
		Output: MappingOutput{
			Status:  MappingExpression{Expression: `"True"`},
			Reason:  MappingExpression{Expression: `"Ready"`},
			Message: MappingExpression{Expression: `"msg"`},
		},
	}

	err = validateConditionMapping("Cluster", oversizedType, rule, map[string]bool{}, env)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("exceeds max length"))
	g.Expect(err.Error()).To(ContainSubstring("100"))
}

func TestValidateCELExpression_ValidExpressions(t *testing.T) {
	g := NewWithT(t)

	env, err := util.NewConditionMappingEnvironment()
	g.Expect(err).ToNot(HaveOccurred())

	tests := []struct {
		name       string
		expression string
	}{
		{
			name:       "Boolean literal",
			expression: "true",
		},
		{
			name:       "String literal",
			expression: `"Ready"`,
		},
		{
			name: "Statuses filter with exists",
			expression: `statuses.exists(s, s.adapter == "test" && ` +
				`s.conditions.exists(c, c.type == "Available" && c.status == "True"))`,
		},
		{
			name:       "Statuses filter with array access",
			expression: `statuses.filter(s, s.adapter == "test")[0].conditions.filter(c, c.type == "Available")[0].status`,
		},
		{
			name:       "String concatenation",
			expression: `"Prefix: " + statuses[0].conditions[0].message`,
		},
		{
			name:       "Conditional expression",
			expression: `statuses.size() > 0 ? "True" : "False"`,
		},
		{
			name:       "Resource field access",
			expression: `resource.name + " is ready"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := validateCELExpression("Cluster", "TestCondition", "when", tt.expression, env)
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func TestValidateCELExpression_MalformedExpressions(t *testing.T) {
	g := NewWithT(t)

	env, err := util.NewConditionMappingEnvironment()
	g.Expect(err).ToNot(HaveOccurred())

	tests := []struct {
		name          string
		expression    string
		errorContains string
	}{
		{
			name:          "Syntax error - unclosed brace",
			expression:    `statuses.filter(s, s.adapter == "test"`,
			errorContains: "invalid CEL expression",
		},
		{
			name:          "Syntax error - invalid operator",
			expression:    `statuses === "test"`,
			errorContains: "invalid CEL expression",
		},
		{
			name:          "Undefined function",
			expression:    `undefinedFunction()`,
			errorContains: "CEL check failed",
		},
		{
			name:          "Wrong function arity",
			expression:    `statuses.exists()`,
			errorContains: "CEL check failed",
		},
		{
			name:          "Invalid field access",
			expression:    `statuses..adapter`,
			errorContains: "invalid CEL expression",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := validateCELExpression("Cluster", "TestCondition", "when", tt.expression, env)
			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).To(ContainSubstring(tt.errorContains))
		})
	}
}

func TestValidateEntityConditions_Integration(t *testing.T) {
	//nolint:govet // fieldalignment: gofmt ordering conflicts with memory alignment
	tests := []struct {
		name        string
		errorMatch  string
		descriptor  EntityDescriptor
		entities    []EntityDescriptor
		expectError bool
	}{
		{
			name: "Valid single condition",
			entities: []EntityDescriptor{
				{
					Kind:             "Cluster",
					Plural:           "clusters",
					RequiredAdapters: []string{"test-adapter"},
					Conditions: []ConditionMappingRule{
						{
							Type: "CustomReady",
							When: MappingExpression{Expression: "true"},
							Output: MappingOutput{
								Status:  MappingExpression{Expression: `"True"`},
								Reason:  MappingExpression{Expression: `"Ready"`},
								Message: MappingExpression{Expression: `"All systems operational"`},
							},
						},
					},
				},
			},
			descriptor: EntityDescriptor{
				Kind:             "Cluster",
				Plural:           "clusters",
				RequiredAdapters: []string{"test-adapter"},
				Conditions: []ConditionMappingRule{
					{
						Type: "CustomReady",
						When: MappingExpression{Expression: "true"},
						Output: MappingOutput{
							Status:  MappingExpression{Expression: `"True"`},
							Reason:  MappingExpression{Expression: `"Ready"`},
							Message: MappingExpression{Expression: `"All systems operational"`},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Empty type should fail",
			entities: []EntityDescriptor{
				{
					Kind:   "Cluster",
					Plural: "clusters",
					Conditions: []ConditionMappingRule{
						{
							Type: "",
							When: MappingExpression{Expression: "true"},
							Output: MappingOutput{
								Status:  MappingExpression{Expression: `"True"`},
								Reason:  MappingExpression{Expression: `"Ready"`},
								Message: MappingExpression{Expression: `"msg"`},
							},
						},
					},
				},
			},
			descriptor: EntityDescriptor{
				Kind:   "Cluster",
				Plural: "clusters",
				Conditions: []ConditionMappingRule{
					{
						Type: "",
						When: MappingExpression{Expression: "true"},
						Output: MappingOutput{
							Status:  MappingExpression{Expression: `"True"`},
							Reason:  MappingExpression{Expression: `"Ready"`},
							Message: MappingExpression{Expression: `"msg"`},
						},
					},
				},
			},
			expectError: true,
			errorMatch:  "condition type cannot be empty",
		},
		{
			name: "Reserved type Reconciled should fail",
			entities: []EntityDescriptor{
				{
					Kind:   "Cluster",
					Plural: "clusters",
					Conditions: []ConditionMappingRule{
						{
							Type: "Reconciled",
							When: MappingExpression{Expression: "true"},
							Output: MappingOutput{
								Status:  MappingExpression{Expression: `"True"`},
								Reason:  MappingExpression{Expression: `"Ready"`},
								Message: MappingExpression{Expression: `"msg"`},
							},
						},
					},
				},
			},
			descriptor: EntityDescriptor{
				Kind:   "Cluster",
				Plural: "clusters",
				Conditions: []ConditionMappingRule{
					{
						Type: "Reconciled",
						When: MappingExpression{Expression: "true"},
						Output: MappingOutput{
							Status:  MappingExpression{Expression: `"True"`},
							Reason:  MappingExpression{Expression: `"Ready"`},
							Message: MappingExpression{Expression: `"msg"`},
						},
					},
				},
			},
			expectError: true,
			errorMatch:  "reserved and cannot be overridden",
		},
		{
			name: "Per-adapter synthesized type should fail",
			entities: []EntityDescriptor{
				{
					Kind:             "Cluster",
					Plural:           "clusters",
					RequiredAdapters: []string{"validation"},
					Conditions: []ConditionMappingRule{
						{
							Type: "ValidationSuccessful", // Synthesized from "validation" adapter
							When: MappingExpression{Expression: "true"},
							Output: MappingOutput{
								Status:  MappingExpression{Expression: `"True"`},
								Reason:  MappingExpression{Expression: `"OK"`},
								Message: MappingExpression{Expression: `"msg"`},
							},
						},
					},
				},
			},
			descriptor: EntityDescriptor{
				Kind:             "Cluster",
				Plural:           "clusters",
				RequiredAdapters: []string{"validation"},
				Conditions: []ConditionMappingRule{
					{
						Type: "ValidationSuccessful",
						When: MappingExpression{Expression: "true"},
						Output: MappingOutput{
							Status:  MappingExpression{Expression: `"True"`},
							Reason:  MappingExpression{Expression: `"OK"`},
							Message: MappingExpression{Expression: `"msg"`},
						},
					},
				},
			},
			expectError: true,
			errorMatch:  "reserved and cannot be overridden",
		},
		{
			name: "Invalid CEL expression should fail",
			entities: []EntityDescriptor{
				{
					Kind:   "Cluster",
					Plural: "clusters",
					Conditions: []ConditionMappingRule{
						{
							Type: "BadCondition",
							When: MappingExpression{Expression: "invalid CEL {{"},
							Output: MappingOutput{
								Status:  MappingExpression{Expression: `"True"`},
								Reason:  MappingExpression{Expression: `"OK"`},
								Message: MappingExpression{Expression: `"msg"`},
							},
						},
					},
				},
			},
			descriptor: EntityDescriptor{
				Kind:   "Cluster",
				Plural: "clusters",
				Conditions: []ConditionMappingRule{
					{
						Type: "BadCondition",
						When: MappingExpression{Expression: "invalid CEL {{"},
						Output: MappingOutput{
							Status:  MappingExpression{Expression: `"True"`},
							Reason:  MappingExpression{Expression: `"OK"`},
							Message: MappingExpression{Expression: `"msg"`},
						},
					},
				},
			},
			expectError: true,
			errorMatch:  "invalid CEL expression",
		},
		{
			name: "Duplicate condition type should fail",
			entities: []EntityDescriptor{
				{
					Kind:   "Cluster",
					Plural: "clusters",
					Conditions: []ConditionMappingRule{
						{
							Type: "CustomReady",
							When: MappingExpression{Expression: "true"},
							Output: MappingOutput{
								Status:  MappingExpression{Expression: `"True"`},
								Reason:  MappingExpression{Expression: `"OK"`},
								Message: MappingExpression{Expression: `"msg"`},
							},
						},
						{
							Type: "CustomReady", // DUPLICATE!
							When: MappingExpression{Expression: "false"},
							Output: MappingOutput{
								Status:  MappingExpression{Expression: `"False"`},
								Reason:  MappingExpression{Expression: `"NotOK"`},
								Message: MappingExpression{Expression: `"msg2"`},
							},
						},
					},
				},
			},
			descriptor: EntityDescriptor{
				Kind:   "Cluster",
				Plural: "clusters",
				Conditions: []ConditionMappingRule{
					{
						Type: "CustomReady",
						When: MappingExpression{Expression: "true"},
						Output: MappingOutput{
							Status:  MappingExpression{Expression: `"True"`},
							Reason:  MappingExpression{Expression: `"OK"`},
							Message: MappingExpression{Expression: `"msg"`},
						},
					},
					{
						Type: "CustomReady", // DUPLICATE!
						When: MappingExpression{Expression: "false"},
						Output: MappingOutput{
							Status:  MappingExpression{Expression: `"False"`},
							Reason:  MappingExpression{Expression: `"NotOK"`},
							Message: MappingExpression{Expression: `"msg2"`},
						},
					},
				},
			},
			expectError: true,
			errorMatch:  "defined multiple times",
		},
		{
			name: "No conditions should pass",
			entities: []EntityDescriptor{
				{
					Kind:   "Cluster",
					Plural: "clusters",
				},
			},
			descriptor: EntityDescriptor{
				Kind:   "Cluster",
				Plural: "clusters",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := ValidateEntityConditions(tt.entities, tt.descriptor)
			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.errorMatch))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestBuildReservedConditionTypes(t *testing.T) {
	entities := []EntityDescriptor{
		{
			Kind:             "Cluster",
			RequiredAdapters: []string{"validation", "landing-zone"},
		},
		{
			Kind:             "NodePool",
			RequiredAdapters: []string{"validation", "compute"}, // "validation" duplicated
		},
	}

	reserved := buildReservedConditionTypes(entities)

	g := NewWithT(t)

	// Static reserved types
	g.Expect(reserved["Reconciled"]).To(BeTrue())
	g.Expect(reserved["LastKnownReconciled"]).To(BeTrue())

	// Per-adapter synthesized types
	g.Expect(reserved["ValidationSuccessful"]).To(BeTrue())
	g.Expect(reserved["LandingZoneSuccessful"]).To(BeTrue())
	g.Expect(reserved["ComputeSuccessful"]).To(BeTrue())

	// Should not have duplicates
	expectedCount := 2 + 3 // 2 static + 3 unique adapters
	g.Expect(len(reserved)).To(Equal(expectedCount))
}
