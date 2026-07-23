package db

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/yaacov/tree-search-language/v6/pkg/tsl"
)

func TestConditionsNodeConverterStatus(t *testing.T) {
	tests := []struct {
		name          string
		field         string
		value         string
		expectedSQL   string
		errorContains string
		expectedArgs  []interface{}
		expectError   bool
	}{
		{
			name:  "Reconciled condition True",
			field: "status.conditions.Reconciled",
			value: "True",
			expectedSQL: "EXISTS (SELECT 1 FROM resource_conditions " +
				"WHERE resource_conditions.resource_id = resources.id " +
				"AND resource_conditions.type = ? AND resource_conditions.status = ?)",
			expectedArgs: []interface{}{"Reconciled", "True"},
		},
		{
			name:  "Reconciled condition False",
			field: "status.conditions.Reconciled",
			value: "False",
			expectedSQL: "EXISTS (SELECT 1 FROM resource_conditions " +
				"WHERE resource_conditions.resource_id = resources.id " +
				"AND resource_conditions.type = ? AND resource_conditions.status = ?)",
			expectedArgs: []interface{}{"Reconciled", "False"},
		},
		{
			name:  "Available condition True",
			field: "status.conditions.Available",
			value: "True",
			expectedSQL: "EXISTS (SELECT 1 FROM resource_conditions " +
				"WHERE resource_conditions.resource_id = resources.id " +
				"AND resource_conditions.type = ? AND resource_conditions.status = ?)",
			expectedArgs: []interface{}{"Available", "True"},
		},
		{
			name:  "Available condition Unknown",
			field: "status.conditions.Available",
			value: "Unknown",
			expectedSQL: "EXISTS (SELECT 1 FROM resource_conditions " +
				"WHERE resource_conditions.resource_id = resources.id " +
				"AND resource_conditions.type = ? AND resource_conditions.status = ?)",
			expectedArgs: []interface{}{"Available", "Unknown"},
		},
		{
			name:          "Invalid condition status",
			field:         "status.conditions.Reconciled",
			value:         "Invalid",
			expectError:   true,
			errorContains: "condition status 'Invalid' is invalid",
		},
		{
			name:          "Invalid condition type - lowercase",
			field:         "status.conditions.ready",
			value:         "True",
			expectError:   true,
			errorContains: "must be PascalCase",
		},
		{
			name:          "Invalid condition type - with underscore",
			field:         "status.conditions.Reconciled_Status",
			value:         "True",
			expectError:   true,
			errorContains: "must be PascalCase",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			node := &tsl.Node{
				Kind:     tsl.KindBinaryExpr,
				Operator: tsl.OpEQ,
				Left:     &tsl.Node{Kind: tsl.KindIdentifier, Value: tt.field},
				Right:    &tsl.Node{Kind: tsl.KindStringLiteral, Value: tt.value},
			}

			result, err := conditionsNodeConverter(node)

			if tt.expectError {
				Expect(err).ToNot(BeNil())
				if tt.errorContains != "" {
					Expect(err.Error()).To(ContainSubstring(tt.errorContains))
				}
				return
			}

			Expect(err).To(BeNil())

			sqlizer := result.(interface {
				ToSql() (string, []interface{}, error)
			})
			sql, args, sqlErr := sqlizer.ToSql()
			Expect(sqlErr).ToNot(HaveOccurred())
			Expect(sql).To(Equal(tt.expectedSQL))
			Expect(args).To(HaveLen(len(tt.expectedArgs)))
			for i, expectedArg := range tt.expectedArgs {
				Expect(args[i]).To(Equal(expectedArg))
			}
		})
	}
}

func TestConditionsNodeConverterSubfields(t *testing.T) {
	tests := []struct {
		name          string
		field         string
		expectedSQL   string
		errorContains string
		value         interface{}
		expectedArgs  []interface{}
		op            tsl.Operator
		expectError   bool
	}{
		{
			name:  "last_updated_time less than",
			field: "status.conditions.Reconciled.last_updated_time",
			op:    tsl.OpLT,
			value: "2026-03-06T00:00:00Z",
			expectedSQL: "(SELECT rc.last_updated_time FROM resource_conditions rc " +
				"WHERE rc.resource_id = resources.id AND rc.type = ?) < ?::timestamptz",
			expectedArgs: []interface{}{
				"Reconciled",
				"2026-03-06T00:00:00Z",
			},
		},
		{
			name:  "last_updated_time greater than",
			field: "status.conditions.Reconciled.last_updated_time",
			op:    tsl.OpGT,
			value: "2026-03-06T00:00:00Z",
			expectedSQL: "(SELECT rc.last_updated_time FROM resource_conditions rc " +
				"WHERE rc.resource_id = resources.id AND rc.type = ?) > ?::timestamptz",
			expectedArgs: []interface{}{
				"Reconciled",
				"2026-03-06T00:00:00Z",
			},
		},
		{
			name:  "last_updated_time less than or equal",
			field: "status.conditions.Reconciled.last_updated_time",
			op:    tsl.OpLE,
			value: "2026-03-06T00:00:00Z",
			expectedSQL: "(SELECT rc.last_updated_time FROM resource_conditions rc " +
				"WHERE rc.resource_id = resources.id AND rc.type = ?) <= ?::timestamptz",
			expectedArgs: []interface{}{
				"Reconciled",
				"2026-03-06T00:00:00Z",
			},
		},
		{
			name:  "last_updated_time greater than or equal",
			field: "status.conditions.Reconciled.last_updated_time",
			op:    tsl.OpGE,
			value: "2026-03-06T00:00:00Z",
			expectedSQL: "(SELECT rc.last_updated_time FROM resource_conditions rc " +
				"WHERE rc.resource_id = resources.id AND rc.type = ?) >= ?::timestamptz",
			expectedArgs: []interface{}{
				"Reconciled",
				"2026-03-06T00:00:00Z",
			},
		},
		{
			name:  "last_updated_time equal",
			field: "status.conditions.Reconciled.last_updated_time",
			op:    tsl.OpEQ,
			value: "2026-03-06T00:00:00Z",
			expectedSQL: "(SELECT rc.last_updated_time FROM resource_conditions rc " +
				"WHERE rc.resource_id = resources.id AND rc.type = ?) = ?::timestamptz",
			expectedArgs: []interface{}{
				"Reconciled",
				"2026-03-06T00:00:00Z",
			},
		},
		{
			name:  "last_updated_time not equal",
			field: "status.conditions.Reconciled.last_updated_time",
			op:    tsl.OpNE,
			value: "2026-03-06T00:00:00Z",
			expectedSQL: "(SELECT rc.last_updated_time FROM resource_conditions rc " +
				"WHERE rc.resource_id = resources.id AND rc.type = ?) != ?::timestamptz",
			expectedArgs: []interface{}{
				"Reconciled",
				"2026-03-06T00:00:00Z",
			},
		},
		{
			name:  "last_transition_time less than",
			field: "status.conditions.Available.last_transition_time",
			op:    tsl.OpLT,
			value: "2026-03-06T00:00:00Z",
			expectedSQL: "(SELECT rc.last_transition_time FROM resource_conditions rc " +
				"WHERE rc.resource_id = resources.id AND rc.type = ?) < ?::timestamptz",
			expectedArgs: []interface{}{
				"Available",
				"2026-03-06T00:00:00Z",
			},
		},
		{
			name:  "observed_generation less than",
			field: "status.conditions.Reconciled.observed_generation",
			op:    tsl.OpLT,
			value: float64(5),
			expectedSQL: "(SELECT rc.observed_generation FROM resource_conditions rc " +
				"WHERE rc.resource_id = resources.id AND rc.type = ?) < ?",
			expectedArgs: []interface{}{
				"Reconciled",
				5,
			},
		},
		{
			name:  "observed_generation equal",
			field: "status.conditions.Reconciled.observed_generation",
			op:    tsl.OpEQ,
			value: float64(3),
			expectedSQL: "(SELECT rc.observed_generation FROM resource_conditions rc " +
				"WHERE rc.resource_id = resources.id AND rc.type = ?) = ?",
			expectedArgs: []interface{}{
				"Reconciled",
				3,
			},
		},
		{
			name:  "KindTimestampLiteral preserves fractional seconds",
			field: "status.conditions.Reconciled.last_updated_time",
			op:    tsl.OpLT,
			value: time.Date(2026, 3, 6, 12, 30, 45, 123456789, time.UTC),
			expectedSQL: "(SELECT rc.last_updated_time FROM resource_conditions rc " +
				"WHERE rc.resource_id = resources.id AND rc.type = ?) < ?::timestamptz",
			expectedArgs: []interface{}{
				"Reconciled",
				"2026-03-06T12:30:45.123456789Z",
			},
		},
		{
			name:  "KindTimestampLiteral without fractional seconds",
			field: "status.conditions.Reconciled.last_updated_time",
			op:    tsl.OpGE,
			value: time.Date(2026, 3, 6, 0, 0, 0, 0, time.UTC),
			expectedSQL: "(SELECT rc.last_updated_time FROM resource_conditions rc " +
				"WHERE rc.resource_id = resources.id AND rc.type = ?) >= ?::timestamptz",
			expectedArgs: []interface{}{
				"Reconciled",
				"2026-03-06T00:00:00Z",
			},
		},
		{
			name:          "Invalid subfield name",
			field:         "status.conditions.Reconciled.unknown_field",
			op:            tsl.OpLT,
			value:         "2026-03-06T00:00:00Z",
			expectError:   true,
			errorContains: "not supported",
		},
		{
			name:          "Invalid operator for subfield",
			field:         "status.conditions.Reconciled.last_updated_time",
			op:            tsl.OpLike,
			value:         "2026%",
			expectError:   true,
			errorContains: "not supported for condition subfield",
		},
		{
			name:          "Invalid condition type in subfield query",
			field:         "status.conditions.ready.last_updated_time",
			op:            tsl.OpLT,
			value:         "2026-03-06T00:00:00Z",
			expectError:   true,
			errorContains: "must be PascalCase",
		},
		{
			name:          "Invalid timestamp format",
			field:         "status.conditions.Reconciled.last_updated_time",
			op:            tsl.OpLT,
			value:         "not-a-timestamp",
			expectError:   true,
			errorContains: "expected RFC3339 format",
		},
		{
			name:          "Float value for integer subfield",
			field:         "status.conditions.Reconciled.observed_generation",
			op:            tsl.OpLT,
			value:         float64(3.5),
			expectError:   true,
			errorContains: "expected integer value",
		},
		{
			name:          "Integer overflow for integer subfield",
			field:         "status.conditions.Reconciled.observed_generation",
			op:            tsl.OpLT,
			value:         float64(3000000000),
			expectError:   true,
			errorContains: "out of 32-bit integer range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			var rightNode *tsl.Node
			switch v := tt.value.(type) {
			case string:
				rightNode = &tsl.Node{Kind: tsl.KindStringLiteral, Value: v}
			case float64:
				rightNode = &tsl.Node{Kind: tsl.KindNumericLiteral, Value: v}
			case time.Time:
				rightNode = &tsl.Node{Kind: tsl.KindTimestampLiteral, Value: v}
			}

			node := &tsl.Node{
				Kind:     tsl.KindBinaryExpr,
				Operator: tt.op,
				Left:     &tsl.Node{Kind: tsl.KindIdentifier, Value: tt.field},
				Right:    rightNode,
			}

			result, err := conditionsNodeConverter(node)

			if tt.expectError {
				Expect(err).ToNot(BeNil())
				if tt.errorContains != "" {
					Expect(err.Error()).To(ContainSubstring(tt.errorContains))
				}
				return
			}

			Expect(err).To(BeNil())

			sqlizer := result.(interface {
				ToSql() (string, []interface{}, error)
			})
			sql, args, sqlErr := sqlizer.ToSql()
			Expect(sqlErr).ToNot(HaveOccurred())
			Expect(sql).To(Equal(tt.expectedSQL))
			Expect(args).To(HaveLen(len(tt.expectedArgs)))
			for i, expectedArg := range tt.expectedArgs {
				Expect(args[i]).To(Equal(expectedArg))
			}
		})
	}
}

func TestHasConditionWithSubfields(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		expected bool
	}{
		{
			name:     "3-part condition field",
			field:    "status.conditions.Reconciled",
			expected: true,
		},
		{
			name:     "4-part subfield (v6 native)",
			field:    "status.conditions.Reconciled.last_updated_time",
			expected: true,
		},
		{
			name:     "Non-condition field",
			field:    "labels.environment",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			node := &tsl.Node{
				Kind:     tsl.KindBinaryExpr,
				Operator: tsl.OpEQ,
				Left:     &tsl.Node{Kind: tsl.KindIdentifier, Value: tt.field},
				Right:    &tsl.Node{Kind: tsl.KindStringLiteral, Value: "value"},
			}

			result := hasCondition(node)
			Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestExtractConditionQueriesWithSubfields(t *testing.T) {
	tests := []struct {
		name                 string
		searchQuery          string
		expectedConditionSQL string
		expectedConditions   int
		expectError          bool
	}{
		{
			name:               "Subfield query only",
			searchQuery:        "status.conditions.Reconciled.last_updated_time < '2026-03-06T00:00:00Z'",
			expectedConditions: 1,
			expectedConditionSQL: "(SELECT rc.last_updated_time FROM resource_conditions rc " +
				"WHERE rc.resource_id = resources.id AND rc.type = ?) < ?::timestamptz",
		},
		{
			name: "Mixed status and subfield queries",
			searchQuery: "status.conditions.Reconciled='False' AND " +
				"status.conditions.Reconciled.last_updated_time < '2026-03-06T00:00:00Z'",
			expectedConditions: 2,
		},
		{
			name: "Subfield query combined with label query",
			searchQuery: "labels.region='us-east' AND " +
				"status.conditions.Reconciled.last_updated_time < '2026-03-06T00:00:00Z'",
			expectedConditions: 1,
		},
		{
			name:        "NOT operator on condition query returns error",
			searchQuery: "NOT (status.conditions.Reconciled='True')",
			expectError: true,
		},
		{
			name:        "NOT operator on condition subfield query returns error",
			searchQuery: "NOT (status.conditions.Reconciled.last_updated_time < '2026-03-06T00:00:00Z')",
			expectError: true,
		},
		{
			name:        "NOT operator on nested condition under AND returns error",
			searchQuery: "NOT (status.conditions.Reconciled='True' AND name='test')",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			// v6 handles deep identifiers natively — no preprocessing needed
			tslTreeWrapper, err := tsl.ParseTSL(tt.searchQuery)
			Expect(err).ToNot(HaveOccurred())

			_, conditions, serviceErr := ExtractConditionQueries(tslTreeWrapper.Node)

			if tt.expectError {
				Expect(serviceErr).ToNot(BeNil())
				return
			}

			Expect(serviceErr).To(BeNil())
			Expect(conditions).To(HaveLen(tt.expectedConditions))

			if tt.expectedConditions > 0 && tt.expectedConditionSQL != "" {
				sql, _, sqlErr := conditions[0].ToSql()
				Expect(sqlErr).ToNot(HaveOccurred())
				Expect(sql).To(Equal(tt.expectedConditionSQL))
			}
		})
	}
}

func TestExtractConditionQueries(t *testing.T) {
	tests := []struct {
		name                 string
		searchQuery          string
		expectedConditionSQL string
		expectedConditions   int
		expectError          bool
	}{
		{
			name:               "Single condition query",
			searchQuery:        "status.conditions.Reconciled='True'",
			expectedConditions: 1,
			expectedConditionSQL: "EXISTS (SELECT 1 FROM resource_conditions " +
				"WHERE resource_conditions.resource_id = resources.id " +
				"AND resource_conditions.type = ? AND resource_conditions.status = ?)",
		},
		{
			name:               "No condition queries",
			searchQuery:        "name='test'",
			expectedConditions: 0,
		},
		{
			name:               "Mixed query with condition",
			searchQuery:        "name='test' AND status.conditions.Reconciled='True'",
			expectedConditions: 1,
		},
		{
			name:               "Multiple condition queries",
			searchQuery:        "status.conditions.Reconciled='True' AND status.conditions.Available='True'",
			expectedConditions: 2,
		},
		{
			name:        "OR with condition on right side is rejected",
			searchQuery: "name='test' OR status.conditions.Reconciled='True'",
			expectError: true,
		},
		{
			name:        "OR with condition on left side is rejected",
			searchQuery: "status.conditions.Available='True' OR name='test'",
			expectError: true,
		},
		{
			name:        "OR with conditions on both sides is rejected",
			searchQuery: "status.conditions.Reconciled='True' OR status.conditions.Available='True'",
			expectError: true,
		},
		{
			name:        "Nested OR with condition is rejected",
			searchQuery: "name='a' AND (region='us' OR status.conditions.Reconciled='True')",
			expectError: true,
		},
		{
			name:               "OR without conditions is allowed",
			searchQuery:        "name='test' OR region='us'",
			expectedConditions: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			tslTreeWrapper, err := tsl.ParseTSL(tt.searchQuery)
			Expect(err).ToNot(HaveOccurred())

			_, conditions, serviceErr := ExtractConditionQueries(tslTreeWrapper.Node)

			if tt.expectError {
				Expect(serviceErr).ToNot(BeNil())
				Expect(serviceErr.Error()).To(ContainSubstring("OR operator is not supported with condition queries"))
				return
			}

			Expect(serviceErr).To(BeNil())
			Expect(conditions).To(HaveLen(tt.expectedConditions))

			if tt.expectedConditions > 0 && tt.expectedConditionSQL != "" {
				sql, _, sqlErr := conditions[0].ToSql()
				Expect(sqlErr).ToNot(HaveOccurred())
				Expect(sql).To(Equal(tt.expectedConditionSQL))
			}
		})
	}
}

func TestHasCondition(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		expected bool
	}{
		{
			name:     "Valid condition field",
			field:    "status.conditions.Reconciled",
			expected: true,
		},
		{
			name:     "Status field without conditions prefix",
			field:    "status.other_field",
			expected: false,
		},
		{
			name:     "Labels field",
			field:    "labels.environment",
			expected: false,
		},
		{
			name:     "Simple field",
			field:    "name",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			node := &tsl.Node{
				Kind:     tsl.KindBinaryExpr,
				Operator: tsl.OpEQ,
				Left:     &tsl.Node{Kind: tsl.KindIdentifier, Value: tt.field},
				Right:    &tsl.Node{Kind: tsl.KindStringLiteral, Value: "value"},
			}

			result := hasCondition(node)
			Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestConditionTypeValidation(t *testing.T) {
	tests := []struct {
		name        string
		condType    string
		expectMatch bool
	}{
		{"Valid - Reconciled", "Reconciled", true},
		{"Valid - Available", "Available", true},
		{"Valid - Progressing", "Progressing", true},
		{"Valid - CustomCondition", "CustomCondition", true},
		{"Valid - With numbers", "Reconciled2", true},
		{"Invalid - lowercase", "ready", false},
		{"Invalid - starts with number", "2Reconciled", false},
		{"Invalid - contains underscore", "Reconciled_State", false},
		{"Invalid - contains hyphen", "Reconciled-State", false},
		{"Invalid - empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			result := conditionTypePattern.MatchString(tt.condType)
			Expect(result).To(Equal(tt.expectMatch))
		})
	}
}

func TestGetField_SpecMapping(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:     "valid snake_case key",
			input:    "spec.is_default",
			expected: "spec->>'is_default'",
		},
		{
			name:     "valid single word key",
			input:    "spec.region",
			expected: "spec->>'region'",
		},
		{
			name:     "valid key with digits",
			input:    "spec.release_image_v2",
			expected: "spec->>'release_image_v2'",
		},
		{
			name:        "invalid key with uppercase",
			input:       "spec.ReleaseImage",
			expectError: true,
		},
		{
			name:        "invalid key with hyphens",
			input:       "spec.release-image",
			expectError: true,
		},
		{
			name:        "empty key",
			input:       "spec.",
			expectError: true,
		},
		{
			name:        "injection attempt",
			input:       "spec.'; DROP TABLE resources;--",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			field, err := getField(tt.input)
			if tt.expectError {
				Expect(err).ToNot(BeNil())
			} else {
				Expect(err).To(BeNil())
				Expect(field).To(Equal(tt.expected))
			}
		})
	}
}

func TestGetField_SpecNested(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "1-level: spec.region",
			input:    "spec.region",
			expected: "spec->>'region'",
		},
		{
			name:     "2-level: spec.release.channel",
			input:    "spec.release.channel",
			expected: "spec->'release'->>'channel'",
		},
		{
			name:     "3-level: spec.release.config.zone",
			input:    "spec.release.config.zone",
			expected: "spec->'release'->'config'->>'zone'",
		},
		{
			name:     "2-level with underscore in key: spec.release.image_v2",
			input:    "spec.release.image_v2",
			expected: "spec->'release'->>'image_v2'",
		},
		{
			name:     "leading/trailing spaces are trimmed",
			input:    "  spec.region  ",
			expected: "spec->>'region'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			field, err := getField(tt.input)
			Expect(err).To(BeNil())
			Expect(field).To(Equal(tt.expected))
		})
	}
}

// TestFieldNameWalk_NumericCast verifies that FieldNameWalk applies CAST(... AS numeric)
// to spec JSONB fields when compared against a number. This logic was previously in a
// separate WrapSpecNumericCasts tree walk and is now integrated into FieldNameWalk.
func TestFieldNameWalk_NumericCast(t *testing.T) {
	parseAndWalk := func(t *testing.T, search string) *tsl.Node {
		t.Helper()
		tree, err := tsl.ParseTSL(search)
		Expect(err).ToNot(HaveOccurred())
		result, serviceErr := FieldNameWalk(tree.Node)
		Expect(serviceErr).To(BeNil())
		return result
	}

	t.Run("spec field with numeric RHS — CAST applied", func(t *testing.T) {
		RegisterTestingT(t)
		result := parseAndWalk(t, "spec.replicas > 9")
		Expect(result.Left.Value).To(Equal("CAST(spec->>'replicas' AS numeric)"))
	})

	t.Run("nested spec field with numeric RHS — CAST applied", func(t *testing.T) {
		RegisterTestingT(t)
		result := parseAndWalk(t, "spec.release.version > 9")
		Expect(result.Left.Value).To(Equal("CAST(spec->'release'->>'version' AS numeric)"))
	})

	t.Run("spec field with string RHS — no CAST", func(t *testing.T) {
		RegisterTestingT(t)
		result := parseAndWalk(t, "spec.channel = 'dev'")
		Expect(result.Left.Value).To(Equal("spec->>'channel'"))
	})

	t.Run("non-spec field with numeric RHS — no CAST", func(t *testing.T) {
		RegisterTestingT(t)
		result := parseAndWalk(t, "generation > 1")
		Expect(result.Left.Value).To(Equal("generation"))
	})

	t.Run("numeric LHS with spec field RHS — CAST applied", func(t *testing.T) {
		RegisterTestingT(t)
		result := parseAndWalk(t, "9 < spec.replicas")
		Expect(result.Right.Value).To(Equal("CAST(spec->>'replicas' AS numeric)"))
	})

	t.Run("numeric LHS with nested spec field RHS — CAST applied", func(t *testing.T) {
		RegisterTestingT(t)
		result := parseAndWalk(t, "9 < spec.release.version")
		Expect(result.Right.Value).To(Equal("CAST(spec->'release'->>'version' AS numeric)"))
	})

	t.Run("numeric LHS with non-spec field RHS — no CAST", func(t *testing.T) {
		RegisterTestingT(t)
		result := parseAndWalk(t, "1 < generation")
		Expect(result.Right.Value).To(Equal("generation"))
	})

	t.Run("AND tree: only spec+numeric nodes get CAST", func(t *testing.T) {
		RegisterTestingT(t)
		result := parseAndWalk(t, "spec.replicas > 9 AND generation > 1 AND spec.channel = 'dev'")

		andLeft := result.Left
		specIdent := andLeft.Left.Left.Value.(string)
		Expect(specIdent).To(Equal("CAST(spec->>'replicas' AS numeric)"))

		genIdent := andLeft.Right.Left.Value.(string)
		Expect(genIdent).To(Equal("generation"))

		chanIdent := result.Right.Left.Value.(string)
		Expect(chanIdent).To(Equal("spec->>'channel'"))
	})
}

// TestFieldNameWalk_TypedFieldValidation verifies that typed top-level columns
// (generation: INTEGER; created_time/updated_time/deleted_time: TIMESTAMPTZ)
// reject mismatched values with a clean 400 before ever reaching Postgres.
func TestFieldNameWalk_TypedFieldValidation(t *testing.T) {
	tests := []struct {
		name          string
		searchQuery   string
		errorContains string
		expectError   bool
	}{
		{
			name:        "generation with valid integer",
			searchQuery: "generation = 5",
		},
		{
			name:          "generation with non-integer string value",
			searchQuery:   "generation = 'abc'",
			expectError:   true,
			errorContains: "field 'generation' expects an integer value",
		},
		{
			name:        "created_time with valid RFC3339 timestamp",
			searchQuery: "created_time > '2026-01-01T00:00:00Z'",
		},
		{
			name:          "created_time with non-timestamp string value",
			searchQuery:   "created_time = 'not-a-date'",
			expectError:   true,
			errorContains: "field 'created_time' expects an RFC3339 timestamp value",
		},
		{
			name:          "updated_time with non-timestamp string value",
			searchQuery:   "updated_time = 'not-a-date'",
			expectError:   true,
			errorContains: "field 'updated_time' expects an RFC3339 timestamp value",
		},
		{
			name:          "deleted_time with non-timestamp string value",
			searchQuery:   "deleted_time = 'not-a-date'",
			expectError:   true,
			errorContains: "field 'deleted_time' expects an RFC3339 timestamp value",
		},
		{
			name:          "generation IN list with a non-integer element",
			searchQuery:   "generation IN [1, 'abc']",
			expectError:   true,
			errorContains: "field 'generation' expects an integer value",
		},
		{
			name:        "generation IN list with all integers",
			searchQuery: "generation IN [1, 2, 3]",
		},
		{
			name:        "combined query: typed field valid, text field untouched",
			searchQuery: "generation = 1 AND name = 'anything-goes'",
		},
		{
			name:          "combined query: error surfaces even when other predicate is valid",
			searchQuery:   "name = 'ok' AND generation = 'abc'",
			expectError:   true,
			errorContains: "field 'generation' expects an integer value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			tree, err := tsl.ParseTSL(tt.searchQuery)
			Expect(err).ToNot(HaveOccurred())

			_, serviceErr := FieldNameWalk(tree.Node)

			if tt.expectError {
				Expect(serviceErr).ToNot(BeNil())
				Expect(serviceErr.Error()).To(ContainSubstring(tt.errorContains))
				// No raw driver details should ever leak into the message.
				Expect(serviceErr.Error()).ToNot(ContainSubstring("pq:"))
				Expect(serviceErr.Error()).ToNot(ContainSubstring("SQLSTATE"))
				return
			}
			Expect(serviceErr).To(BeNil())
		})
	}
}

func TestConditionStatusValidation(t *testing.T) {
	tests := []struct {
		status      string
		expectValid bool
	}{
		{"True", true},
		{"False", true},
		{"Unknown", true},
		{"true", false},
		{"false", false},
		{"unknown", false},
		{"Yes", false},
		{"No", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			RegisterTestingT(t)

			result := validConditionStatuses[tt.status]
			Expect(result).To(Equal(tt.expectValid))
		})
	}
}

func TestArgsToOrder(t *testing.T) {
	tests := []struct {
		name          string
		errorContains string
		input         []string
		expected      []string
		expectError   bool
	}{
		{
			name:     "single field with asc",
			input:    []string{"name asc"},
			expected: []string{"name asc"},
		},
		{
			name:     "single field with desc",
			input:    []string{"created_time desc"},
			expected: []string{"created_time desc"},
		},
		{
			name:     "single field without direction defaults to asc",
			input:    []string{"created_time"},
			expected: []string{"created_time asc"},
		},
		{
			name:     "multiple fields",
			input:    []string{"name asc", "created_time desc"},
			expected: []string{"name asc", "created_time desc"},
		},
		{
			name:     "field with extra spaces",
			input:    []string{"  name   asc  "},
			expected: []string{"name asc"},
		},
		{
			name:     "field with tabs and spaces",
			input:    []string{"name  \t  desc"},
			expected: []string{"name desc"},
		},
		{
			name:     "all allowed fields",
			input:    []string{"id", "name", "created_time", "updated_time", "kind"},
			expected: []string{"id asc", "name asc", "created_time asc", "updated_time asc", "kind asc"},
		},
		{
			name:          "invalid direction",
			input:         []string{"name ascending"},
			expectError:   true,
			errorContains: "invalid order format",
		},
		{
			name:          "SQL injection attempt - semicolon",
			input:         []string{"name; DROP TABLE resources"},
			expectError:   true,
			errorContains: "invalid order format",
		},
		{
			name:          "SQL injection attempt - comment",
			input:         []string{"name-- asc"},
			expectError:   true,
			errorContains: "invalid order format",
		},
		{
			name:          "uppercase field name",
			input:         []string{"NAME asc"},
			expectError:   true,
			errorContains: "invalid order format",
		},
		{
			name:          "uppercase direction",
			input:         []string{"name ASC"},
			expectError:   true,
			errorContains: "invalid order format",
		},
		{
			name:     "empty string in array is skipped",
			input:    []string{""},
			expected: nil,
		},
		{
			name:     "empty string in array with field at end",
			input:    []string{"", "", "", "", "", "kind asc", "href desc"},
			expected: []string{"kind asc", "href desc"},
		},
		{
			name:     "whitespace only string is skipped",
			input:    []string{"   "},
			expected: nil,
		},
		{
			name:     "mixed valid and empty strings and tabs",
			input:    []string{"name asc", "", "created_time desc", "   ", "\t"},
			expected: []string{"name asc", "created_time desc"},
		},
		{
			name:          "mixed valid and invalid field",
			input:         []string{"created_time desc", "name", "wrong_field"},
			expectError:   true,
			errorContains: "not allowed for ordering",
		},
		{
			name:          "field not in whitelist",
			input:         []string{"custom_field asc"},
			expectError:   true,
			errorContains: "not allowed for ordering",
		},
		{
			name:     "deleted_time field",
			input:    []string{"deleted_time desc"},
			expected: []string{"deleted_time desc"},
		},
		{
			name:     "generation field",
			input:    []string{"generation asc"},
			expected: []string{"generation asc"},
		},
		{
			name:          "too many parts",
			input:         []string{"name asc extra"},
			expectError:   true,
			errorContains: "invalid order format",
		},
		{
			name:     "empty array",
			input:    []string{},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			result, err := ArgsToOrder(tt.input)

			if tt.expectError {
				Expect(err).ToNot(BeNil(), "expected error but got nil")
				if tt.errorContains != "" {
					Expect(err.Reason).To(ContainSubstring(tt.errorContains))
				}
			} else {
				Expect(err).To(BeNil(), "unexpected error: %v", err)
				Expect(result).To(Equal(tt.expected))
			}
		})
	}
}

func TestArgsToOrder_SecurityValidation(t *testing.T) {
	RegisterTestingT(t)

	// SQL injection attempts that should all fail
	injectionAttempts := []struct {
		name  string
		input string
	}{
		{"union injection", "name UNION SELECT password FROM users"},
		{"comment injection", "name--"},
		{"semicolon terminator", "name; DROP TABLE resources;"},
		{"quote escape", "name' OR '1'='1"},
		{"parentheses", "name) OR (1=1"},
		{"wildcard", "name*"},
		{"backtick", "name`"},
	}

	for _, tt := range injectionAttempts {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			_, err := ArgsToOrder([]string{tt.input})
			Expect(err).ToNot(BeNil(), "injection attempt '%s' should be rejected", tt.input)
		})
	}
}
func TestExtractLabelQueries(t *testing.T) {
	tests := []struct {
		name             string
		searchQuery      string
		expectedLabelSQL string
		expectedArgs     []interface{}
		expectedLabels   int
		expectRemaining  bool
		expectError      bool
	}{
		{
			name:           "Single label EQ query",
			searchQuery:    "labels.env='prod'",
			expectedLabels: 1,
			expectedLabelSQL: "EXISTS (SELECT 1 FROM resource_labels " +
				"WHERE resource_labels.resource_id = resources.id " +
				"AND resource_labels.key = ? AND resource_labels.value = ?)",
			expectedArgs:    []interface{}{"env", "prod"},
			expectRemaining: false,
		},
		{
			name:           "Single label NE query",
			searchQuery:    "labels.env!='staging'",
			expectedLabels: 1,
			expectedLabelSQL: "EXISTS (SELECT 1 FROM resource_labels " +
				"WHERE resource_labels.resource_id = resources.id " +
				"AND resource_labels.key = ? AND resource_labels.value != ?)",
			expectedArgs:    []interface{}{"env", "staging"},
			expectRemaining: false,
		},
		{
			name:            "Label query combined with non-label field",
			searchQuery:     "labels.env='prod' AND name='foo'",
			expectedLabels:  1,
			expectRemaining: true,
		},
		{
			name:           "Label IN query",
			searchQuery:    "labels.env IN ['prod','staging']",
			expectedLabels: 1,
			expectedLabelSQL: "EXISTS (SELECT 1 FROM resource_labels " +
				"WHERE resource_labels.resource_id = resources.id " +
				"AND resource_labels.key = ? AND resource_labels.value IN (?,?))",
			expectedArgs:    []interface{}{"env", "prod", "staging"},
			expectRemaining: false,
		},
		{
			name:            "Label IN combined with non-label field",
			searchQuery:     "labels.env IN ['prod','staging'] AND name='foo'",
			expectedLabels:  1,
			expectRemaining: true,
		},
		{
			name:        "Unsupported operator GT returns error",
			searchQuery: "labels.env>'prod'",
			expectError: true,
		},
		{
			name:        "NOT wrapping label query returns error",
			searchQuery: "NOT (labels.env='prod')",
			expectError: true,
		},
		{
			name:        "Unparenthesized NOT before label is rejected",
			searchQuery: "not labels.env='prod'",
			expectError: true,
		},
		{
			name:        "OR with label query returns error",
			searchQuery: "labels.env='prod' OR name='foo'",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			tslTreeWrapper, err := tsl.ParseTSL(tt.searchQuery)
			Expect(err).ToNot(HaveOccurred())

			remaining, labels, serviceErr := ExtractLabelQueries(tslTreeWrapper.Node)

			if tt.expectError {
				Expect(serviceErr).ToNot(BeNil())
				return
			}

			Expect(serviceErr).To(BeNil())
			Expect(labels).To(HaveLen(tt.expectedLabels))

			if tt.expectedLabels > 0 && tt.expectedLabelSQL != "" {
				sql, args, sqlErr := labels[0].ToSql()
				Expect(sqlErr).ToNot(HaveOccurred())
				Expect(sql).To(Equal(tt.expectedLabelSQL))
				Expect(sql).To(ContainSubstring("EXISTS (SELECT 1 FROM resource_labels"))
				if tt.expectedArgs != nil {
					Expect(args).To(Equal(tt.expectedArgs),
						"bound arguments should match expected key/value bindings")
				}
			}

			if tt.expectRemaining {
				Expect(remaining).ToNot(BeNil())
				// The remaining tree must not contain any leaked label identifiers
				Expect(subtreeHasMatch(remaining, isLabelIdentifier)).To(BeFalse(),
					"remaining tree should not contain label identifiers")
				// Verify the non-label predicate survived by checking for a "name" identifier
				hasName := subtreeHasMatch(remaining, func(n *tsl.Node) bool {
					s, ok := n.Value.(string)
					return ok && s == "name"
				})
				Expect(hasName).To(BeTrue(), "remaining tree should contain the non-label 'name' predicate")
			} else if tt.expectedLabels > 0 {
				// When all nodes were label queries, remaining tree is a 1=1 placeholder
				Expect(remaining).ToNot(BeNil())
			}
		})
	}
}
