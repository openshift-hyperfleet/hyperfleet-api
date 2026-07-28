package config

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
)

// ============================================================================
// Tests from conditions_test.go
// ============================================================================

func TestConditionsConfig_Validate(t *testing.T) {
	t.Parallel()
	// Typical entity config with required adapters
	entities := []registry.EntityDescriptor{
		{
			Kind:             "Cluster",
			RequiredAdapters: []string{"validation", "dns", "pullsecret", "hypershift"},
		},
		{
			Kind:             "NodePool",
			RequiredAdapters: []string{"validation"},
		},
	}

	tests := []struct {
		config    *ConditionsConfig
		name      string
		errorMsg  string
		entities  []registry.EntityDescriptor
		wantError bool
	}{
		{
			name:     "valid config with single cluster rule",
			entities: entities,
			config: &ConditionsConfig{
				Clusters: map[string]ConditionMappingRule{
					"QuotaValid": {
						When: MappingExpression{
							Expression: `statuses.exists(s, s.adapter == "validation")`,
						},
						Output: MappingOutput{
							Status: MappingExpression{
								Expression: `"True"`,
							},
							Reason: MappingExpression{
								Expression: `"QuotaOK"`,
							},
							Message: MappingExpression{
								Expression: `"Quota is sufficient"`,
							},
						},
					},
				},
				NodePools: map[string]ConditionMappingRule{},
			},
			wantError: false,
		},
		{
			name:     "empty config is valid",
			entities: entities,
			config: &ConditionsConfig{
				Clusters:  map[string]ConditionMappingRule{},
				NodePools: map[string]ConditionMappingRule{},
			},
			wantError: false,
		},
		{
			name:     "reserved type Reconciled is rejected",
			entities: entities,
			config: &ConditionsConfig{
				Clusters: map[string]ConditionMappingRule{
					"Reconciled": {
						When: MappingExpression{Expression: `true`},
						Output: MappingOutput{
							Status:  MappingExpression{Expression: `"True"`},
							Reason:  MappingExpression{Expression: `"OK"`},
							Message: MappingExpression{Expression: `"OK"`},
						},
					},
				},
				NodePools: map[string]ConditionMappingRule{},
			},
			wantError: true,
			errorMsg:  "reserved",
		},
		{
			name:     "reserved type LastKnownReconciled is rejected",
			entities: entities,
			config: &ConditionsConfig{
				Clusters: map[string]ConditionMappingRule{},
				NodePools: map[string]ConditionMappingRule{
					"LastKnownReconciled": {
						When: MappingExpression{Expression: `true`},
						Output: MappingOutput{
							Status:  MappingExpression{Expression: `"True"`},
							Reason:  MappingExpression{Expression: `"OK"`},
							Message: MappingExpression{Expression: `"OK"`},
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "reserved",
		},
		{
			name:     "per-adapter synthesized type ValidationSuccessful is rejected",
			entities: entities, // Contains "validation" adapter
			config: &ConditionsConfig{
				Clusters: map[string]ConditionMappingRule{
					"ValidationSuccessful": { // Synthesized from "validation" adapter
						When: MappingExpression{Expression: `true`},
						Output: MappingOutput{
							Status:  MappingExpression{Expression: `"True"`},
							Reason:  MappingExpression{Expression: `"OK"`},
							Message: MappingExpression{Expression: `"OK"`},
						},
					},
				},
				NodePools: map[string]ConditionMappingRule{},
			},
			wantError: true,
			errorMsg:  "reserved",
		},
		{
			name:     "per-adapter synthesized type DnsSuccessful is rejected",
			entities: entities, // Contains "dns" adapter
			config: &ConditionsConfig{
				Clusters: map[string]ConditionMappingRule{
					"DnsSuccessful": { // Synthesized from "dns" adapter
						When: MappingExpression{Expression: `true`},
						Output: MappingOutput{
							Status:  MappingExpression{Expression: `"True"`},
							Reason:  MappingExpression{Expression: `"OK"`},
							Message: MappingExpression{Expression: `"OK"`},
						},
					},
				},
				NodePools: map[string]ConditionMappingRule{},
			},
			wantError: true,
			errorMsg:  "reserved",
		},
		{
			name:     "condition type exceeding max length is rejected",
			entities: entities,
			config: &ConditionsConfig{
				Clusters: map[string]ConditionMappingRule{
					strings.Repeat("A", 129): { // 129 chars > 128 max
						When: MappingExpression{Expression: `true`},
						Output: MappingOutput{
							Status:  MappingExpression{Expression: `"True"`},
							Reason:  MappingExpression{Expression: `"OK"`},
							Message: MappingExpression{Expression: `"OK"`},
						},
					},
				},
				NodePools: map[string]ConditionMappingRule{},
			},
			wantError: true,
			errorMsg:  "max length",
		},
		{
			name:     "invalid CEL syntax in when expression",
			entities: entities,
			config: &ConditionsConfig{
				Clusters: map[string]ConditionMappingRule{
					"Valid": {
						When: MappingExpression{
							Expression: `statuses.exists(s, s.adapter == `, // incomplete
						},
						Output: MappingOutput{
							Status:  MappingExpression{Expression: `"True"`},
							Reason:  MappingExpression{Expression: `"OK"`},
							Message: MappingExpression{Expression: `"OK"`},
						},
					},
				},
				NodePools: map[string]ConditionMappingRule{},
			},
			wantError: true,
			errorMsg:  "invalid CEL expression",
		},
		{
			name:     "invalid CEL syntax in output status",
			entities: entities,
			config: &ConditionsConfig{
				Clusters: map[string]ConditionMappingRule{
					"Valid": {
						When: MappingExpression{Expression: `true`},
						Output: MappingOutput{
							Status: MappingExpression{
								Expression: `statuses[`, // incomplete
							},
							Reason:  MappingExpression{Expression: `"OK"`},
							Message: MappingExpression{Expression: `"OK"`},
						},
					},
				},
				NodePools: map[string]ConditionMappingRule{},
			},
			wantError: true,
			errorMsg:  "invalid CEL expression",
		},
		{
			name:     "complex valid CEL expression",
			entities: entities,
			config: &ConditionsConfig{
				Clusters: map[string]ConditionMappingRule{
					"ComplexCondition": {
						When: MappingExpression{
							Expression: `statuses.exists(s, s.adapter == "validation" && ` +
								`s.conditions.exists(c, c.type == "QuotaSufficient" && c.status == "True"))`,
						},
						Output: MappingOutput{
							Status: MappingExpression{
								Expression: `statuses.filter(s, s.adapter == "validation")[0].` +
									`conditions.filter(c, c.type == "QuotaSufficient")[0].status`,
							},
							Reason: MappingExpression{
								Expression: `statuses.filter(s, s.adapter == "validation")[0].` +
									`conditions.filter(c, c.type == "QuotaSufficient")[0].reason`,
							},
							Message: MappingExpression{
								Expression: `"Quota: " + statuses.filter(s, s.adapter == "validation")[0].` +
									`conditions.filter(c, c.type == "QuotaSufficient")[0].message`,
							},
						},
					},
				},
				NodePools: map[string]ConditionMappingRule{},
			},
			wantError: false,
		},
		{
			name:     "non-boolean when expression passes validation but fails at runtime",
			entities: entities,
			config: &ConditionsConfig{
				Clusters: map[string]ConditionMappingRule{
					"StringWhen": {
						When: MappingExpression{
							Expression: `"true"`, // String, not boolean - CEL validates types at runtime
						},
						Output: MappingOutput{
							Status:  MappingExpression{Expression: `"True"`},
							Reason:  MappingExpression{Expression: `"OK"`},
							Message: MappingExpression{Expression: `"OK"`},
						},
					},
				},
				NodePools: map[string]ConditionMappingRule{},
			},
			wantError: false, // Validation passes - CEL type checking happens at runtime, not compile time
		},
		{
			name:     "nested comprehensions pass validation (cost limit enforced at runtime)",
			entities: entities,
			config: &ConditionsConfig{
				Clusters: map[string]ConditionMappingRule{
					"Expensive": {
						When: MappingExpression{
							// Nested comprehensions - valid syntax, but may be expensive at runtime
							// Cost limit of 10000 is enforced during Program creation, not Check
							Expression: `statuses.map(s, s.conditions.map(c, c.type + c.status)).size() > 0`,
						},
						Output: MappingOutput{
							Status:  MappingExpression{Expression: `"True"`},
							Reason:  MappingExpression{Expression: `"OK"`},
							Message: MappingExpression{Expression: `"OK"`},
						},
					},
				},
				NodePools: map[string]ConditionMappingRule{},
			},
			wantError: false, // Validation passes - cost limit is compile-time static analysis estimate
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := tt.config.Validate(tt.entities)

			if tt.wantError {
				Expect(err).To(HaveOccurred(), "expected validation to fail")
				Expect(err.Error()).To(ContainSubstring(tt.errorMsg), "error message should contain expected text")
			} else {
				Expect(err).NotTo(HaveOccurred(), "expected validation to succeed")
			}
		})
	}
}
func TestConditionsConfig_IsEmpty(t *testing.T) {
	t.Parallel()
	tests := []struct {
		config   *ConditionsConfig
		name     string
		expected bool
	}{
		{
			name: "empty config",
			config: &ConditionsConfig{
				Clusters:  map[string]ConditionMappingRule{},
				NodePools: map[string]ConditionMappingRule{},
			},
			expected: true,
		},
		{
			name: "nil maps are empty",
			config: &ConditionsConfig{
				Clusters:  nil,
				NodePools: nil,
			},
			expected: true,
		},
		{
			name: "clusters has rules",
			config: &ConditionsConfig{
				Clusters: map[string]ConditionMappingRule{
					"Test": {
						When:   MappingExpression{Expression: `true`},
						Output: MappingOutput{},
					},
				},
				NodePools: map[string]ConditionMappingRule{},
			},
			expected: false,
		},
		{
			name: "nodepools has rules",
			config: &ConditionsConfig{
				Clusters: map[string]ConditionMappingRule{},
				NodePools: map[string]ConditionMappingRule{
					"Test": {
						When:   MappingExpression{Expression: `true`},
						Output: MappingOutput{},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			result := tt.config.IsEmpty()
			Expect(result).To(Equal(tt.expected))
		})
	}
}
func TestNewConditionsConfig(t *testing.T) {
	t.Parallel()
	RegisterTestingT(t)
	config := NewConditionsConfig()

	Expect(config).NotTo(BeNil())
	Expect(config.Clusters).NotTo(BeNil())
	Expect(config.NodePools).NotTo(BeNil())
	Expect(config.Clusters).To(BeEmpty())
	Expect(config.NodePools).To(BeEmpty())
	Expect(config.IsEmpty()).To(BeTrue())
}
func TestConditionsConfig_NilReceiverGuards(t *testing.T) {
	t.Parallel()
	t.Run("Validate on nil receiver returns nil", func(t *testing.T) {
		RegisterTestingT(t)
		var c *ConditionsConfig
		err := c.Validate(nil)
		Expect(err).NotTo(HaveOccurred(), "nil receiver should return nil, not panic")
	})

	t.Run("IsEmpty on nil receiver returns true", func(t *testing.T) {
		RegisterTestingT(t)
		var c *ConditionsConfig
		result := c.IsEmpty()
		Expect(result).To(BeTrue(), "nil receiver should be considered empty")
	})
}
func TestConditionsConfig_YAMLNullHandling(t *testing.T) {
	t.Parallel()
	t.Run("YAML with conditions: null is handled gracefully", func(t *testing.T) {
		RegisterTestingT(t)
		yamlContent := `
server:
  port: 8000
conditions: null
`
		// Simulate viper unmarshaling
		type testConfig struct {
			Conditions *ConditionsConfig
			Server     struct{ Port int }
		}

		var cfg testConfig
		err := yaml.Unmarshal([]byte(yamlContent), &cfg)
		Expect(err).NotTo(HaveOccurred())

		// Verify Conditions is nil
		Expect(cfg.Conditions).To(BeNil())

		// Verify Validate doesn't panic
		Expect(func() {
			err := cfg.Conditions.Validate(nil)
			Expect(err).NotTo(HaveOccurred())
		}).NotTo(Panic())

		// Verify IsEmpty doesn't panic
		Expect(func() {
			empty := cfg.Conditions.IsEmpty()
			Expect(empty).To(BeTrue())
		}).NotTo(Panic())
	})
}

// ============================================================================
// Tests from conditions_determinism_test.go
// ============================================================================

func TestConditionsConfig_Validate_DeterministicErrors(t *testing.T) {
	t.Parallel()
	t.Run("validation errors are deterministic with multiple invalid rules", func(t *testing.T) {
		RegisterTestingT(t)

		// Config with multiple invalid rules
		// If sorted: "AInvalid" is checked before "ZInvalid"
		// If unsorted: order is non-deterministic
		config := &ConditionsConfig{
			Clusters: map[string]ConditionMappingRule{
				"ZInvalid": {
					When: MappingExpression{Expression: `invalid syntax`},
					Output: MappingOutput{
						Status:  MappingExpression{Expression: `"True"`},
						Reason:  MappingExpression{Expression: `"OK"`},
						Message: MappingExpression{Expression: `"OK"`},
					},
				},
				"AInvalid": {
					When: MappingExpression{Expression: `another bad`},
					Output: MappingOutput{
						Status:  MappingExpression{Expression: `"True"`},
						Reason:  MappingExpression{Expression: `"OK"`},
						Message: MappingExpression{Expression: `"OK"`},
					},
				},
			},
		}

		// Run validation multiple times - should always fail on same rule first
		for i := 0; i < 10; i++ {
			err := config.Validate(nil)
			Expect(err).To(HaveOccurred())
			// Should always fail on "AInvalid" first (alphabetically first)
			Expect(err.Error()).To(ContainSubstring("clusters.AInvalid"))
		}
	})
}

// ============================================================================
// Tests from conditions_cel_check_test.go
// ============================================================================

func TestValidate_CELCheckCatchesErrors(t *testing.T) {
	t.Parallel()
	t.Run("undefined function caught by Check", func(t *testing.T) {
		RegisterTestingT(t)

		config := &ConditionsConfig{
			Clusters: map[string]ConditionMappingRule{
				"TestCondition": {
					When: MappingExpression{
						Expression: "undefinedFunction(statuses)", // No such function
					},
					Output: MappingOutput{
						Status:  MappingExpression{Expression: "'True'"},
						Reason:  MappingExpression{Expression: "'TestReason'"},
						Message: MappingExpression{Expression: "'TestMessage'"},
					},
				},
			},
		}

		err := config.Validate([]registry.EntityDescriptor{})

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("CEL check failed"))
		Expect(err.Error()).To(ContainSubstring("undefinedFunction"))
	})

	t.Run("wrong function arity caught by Check", func(t *testing.T) {
		RegisterTestingT(t)

		config := &ConditionsConfig{
			Clusters: map[string]ConditionMappingRule{
				"TestCondition": {
					When: MappingExpression{
						// size() expects 1 argument, not 2
						Expression: "size(statuses, 'extra arg')",
					},
					Output: MappingOutput{
						Status:  MappingExpression{Expression: "'True'"},
						Reason:  MappingExpression{Expression: "'TestReason'"},
						Message: MappingExpression{Expression: "'TestMessage'"},
					},
				},
			},
		}

		err := config.Validate([]registry.EntityDescriptor{})

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("CEL check failed"))
	})

	t.Run("valid CEL expression passes Check", func(t *testing.T) {
		RegisterTestingT(t)

		config := &ConditionsConfig{
			Clusters: map[string]ConditionMappingRule{
				"TestCondition": {
					When: MappingExpression{
						Expression: "size(statuses) > 0", // Valid
					},
					Output: MappingOutput{
						Status:  MappingExpression{Expression: "'True'"},
						Reason:  MappingExpression{Expression: "'TestReason'"},
						Message: MappingExpression{Expression: "'TestMessage'"},
					},
				},
			},
		}

		err := config.Validate([]registry.EntityDescriptor{})

		Expect(err).NotTo(HaveOccurred())
	})
}
