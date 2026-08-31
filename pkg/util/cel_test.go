package util

import (
	"strings"
	"testing"

	"cel.dev/cel-go/cel"
	. "github.com/onsi/gomega"
)

// ============================================================================
// Tests from cel_test.go
// ============================================================================

func TestNewConditionMappingEnvironment(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	env, err := NewConditionMappingEnvironment()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(env).NotTo(BeNil())
}

func TestDigFunc_MapNavigation(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	env, err := NewConditionMappingEnvironment()
	g.Expect(err).NotTo(HaveOccurred())

	// Test data: nested map
	resourceData := map[string]interface{}{
		"adapter": "validation",
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type":   "Available",
					"status": "True",
				},
			},
		},
	}

	tests := []struct {
		want       interface{}
		name       string
		expression string
		wantErr    bool
	}{
		{
			name:       "simple map key",
			expression: `dig(resource, "adapter")`,
			want:       "validation",
		},
		{
			name:       "nested map key",
			expression: `dig(resource, "status.conditions")`,
			want: []interface{}{
				map[string]interface{}{
					"type":   "Available",
					"status": "True",
				},
			},
		},
		{
			name:       "array index",
			expression: `dig(resource, "status.conditions.0.type")`,
			want:       "Available",
		},
		{
			name:       "missing key returns null",
			expression: `dig(resource, "missing") == null`,
			want:       true,
		},
		{
			name:       "out of bounds index returns null",
			expression: `dig(resource, "status.conditions.99") == null`,
			want:       true,
		},
		{
			name:       "negative index returns null",
			expression: `dig(resource, "status.conditions.-1") == null`,
			want:       true,
		},
		{
			name:       "empty path returns original",
			expression: `dig(resource, "").adapter`,
			want:       "validation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ast, issues := env.Parse(tt.expression)
			g.Expect(issues).To(BeNil())

			// Check step: validates variable names and function signatures match environment declaration
			// Matches the 3-step compilation pipeline in compileExpression (condition_mapper.go:389-412)
			_, issues = env.Check(ast)
			g.Expect(issues).To(BeNil())

			prg, err := env.Program(ast, cel.CostLimit(CELCostLimit))
			g.Expect(err).NotTo(HaveOccurred())

			out, _, err := prg.Eval(map[string]interface{}{
				CELVarResource: resourceData,
			})

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out.Value()).To(Equal(tt.want))
			}
		})
	}
}

func TestDigFunc_ArrayNavigation(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	env, err := NewConditionMappingEnvironment()
	g.Expect(err).NotTo(HaveOccurred())

	// Test data: array at root
	statusesData := []interface{}{
		map[string]interface{}{
			"adapter": "validation",
			"conditions": []interface{}{
				map[string]interface{}{
					"type":   "Available",
					"status": "True",
				},
				map[string]interface{}{
					"type":   "Health",
					"status": "False",
				},
			},
		},
		map[string]interface{}{
			"adapter": "dns",
			"conditions": []interface{}{
				map[string]interface{}{
					"type":   "Ready",
					"status": "True",
				},
			},
		},
	}

	tests := []struct {
		want       interface{}
		name       string
		expression string
	}{
		{
			name:       "first adapter name",
			expression: `dig(statuses, "0.adapter")`,
			want:       "validation",
		},
		{
			name:       "second adapter name",
			expression: `dig(statuses, "1.adapter")`,
			want:       "dns",
		},
		{
			name:       "nested array access",
			expression: `dig(statuses, "0.conditions.1.type")`,
			want:       "Health",
		},
		{
			name:       "deep nested navigation",
			expression: `dig(statuses, "1.conditions.0.status")`,
			want:       "True",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ast, issues := env.Parse(tt.expression)
			g.Expect(issues).To(BeNil())

			// Check step: validates variable names and function signatures match environment declaration
			// Matches the 3-step compilation pipeline in compileExpression (condition_mapper.go:389-412)
			_, issues = env.Check(ast)
			g.Expect(issues).To(BeNil())

			prg, err := env.Program(ast, cel.CostLimit(CELCostLimit))
			g.Expect(err).NotTo(HaveOccurred())

			out, _, err := prg.Eval(map[string]interface{}{
				CELVarStatuses: statusesData,
			})

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(out.Value()).To(Equal(tt.want))
		})
	}
}

func TestToJsonFunc(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	env, err := NewConditionMappingEnvironment()
	g.Expect(err).NotTo(HaveOccurred())

	resourceData := map[string]interface{}{
		"name":  "test",
		"count": int64(42),
	}

	ast, issues := env.Parse(`toJson(resource)`)
	g.Expect(issues).To(BeNil())

	// Check step: validates variable names and function signatures match environment declaration
	// Matches the 3-step compilation pipeline in compileExpression (condition_mapper.go:389-412)
	_, issues = env.Check(ast)
	g.Expect(issues).To(BeNil())

	prg, err := env.Program(ast, cel.CostLimit(CELCostLimit))
	g.Expect(err).NotTo(HaveOccurred())

	out, _, err := prg.Eval(map[string]interface{}{
		CELVarResource: resourceData,
	})

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(out.Value()).To(Equal(`{"count":42,"name":"test"}`))
}

func TestToJsonFunc_SizeLimit(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	env, err := NewConditionMappingEnvironment()
	g.Expect(err).NotTo(HaveOccurred())

	// Create data that would exceed the 1MB limit when JSON-encoded.
	// Implementation uses json.Encoder with limitedWriter to stop encoding
	// DURING the operation (not after), preventing unbounded allocation.
	largeData := map[string]interface{}{
		"big": strings.Repeat("x", 1024*1024+1),
	}

	ast, issues := env.Parse(`toJson(resource)`)
	g.Expect(issues).To(BeNil())

	_, issues = env.Check(ast)
	g.Expect(issues).To(BeNil())

	prg, err := env.Program(ast, cel.CostLimit(CELCostLimit))
	g.Expect(err).NotTo(HaveOccurred())

	_, _, err = prg.Eval(map[string]interface{}{
		CELVarResource: largeData,
	})
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("exceeds 1MB"))
}

// ============================================================================
// Tests from cel_constants_test.go
// ============================================================================

func TestCELVariableConstants(t *testing.T) {
	t.Parallel()
	t.Run("CEL variable constants prevent name mismatches", func(t *testing.T) {
		g := NewWithT(t)

		// Create environment using constants
		env, err := NewConditionMappingEnvironment()
		g.Expect(err).NotTo(HaveOccurred())

		// Verify that expressions using the documented variable names compile successfully
		// This test ensures that if we change a constant, the environment declaration changes too
		validExpressions := []string{
			"size(statuses) > 0",                   // Uses CELVarStatuses
			"resource.metadata.name",               // Uses CELVarResource
			"statuses.exists(s, s.adapter == 'x')", // Complex usage
			"resource.generation > 0",              // Resource field access
		}

		for _, expr := range validExpressions {
			ast, issues := env.Parse(expr)
			g.Expect(issues).To(BeNil(), "Expression should parse: %s", expr)

			_, issues = env.Check(ast)
			g.Expect(issues).To(BeNil(), "Expression should check: %s", expr)
		}
	})

	t.Run("typo in variable name causes compile error", func(t *testing.T) {
		g := NewWithT(t)

		env, err := NewConditionMappingEnvironment()
		g.Expect(err).NotTo(HaveOccurred())

		// If someone hardcodes "status" instead of "statuses", Check catches it
		invalidExpr := "size(status) > 0" // Typo: "status" instead of "statuses"

		ast, issues := env.Parse(invalidExpr)
		g.Expect(issues).To(BeNil(), "Parse should succeed even with undefined variable")

		// Check should fail because "status" is not declared
		_, issues = env.Check(ast)
		g.Expect(issues).NotTo(BeNil(), "Check should fail for undefined variable 'status'")
		g.Expect(issues.Err().Error()).To(ContainSubstring("status"), "Error should mention the undefined variable")
	})

	t.Run("constant values match expected strings", func(t *testing.T) {
		g := NewWithT(t)

		// Verify constant values are what we expect (prevents accidental changes)
		g.Expect(CELVarStatuses).To(Equal("statuses"))
		g.Expect(CELVarResource).To(Equal("resource"))
	})
}
