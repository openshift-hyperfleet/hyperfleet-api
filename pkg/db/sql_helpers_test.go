package db

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/yaacov/tree-search-language/pkg/tsl"
)

func TestConditionsNodeConverterStatus(t *testing.T) {
	tests := []struct {
		name          string
		field         string
		value         string
		expectedSQL   string
		expectedArgs  []interface{}
		expectError   bool
		errorContains string
	}{
		{
			name:         "Ready condition True",
			field:        "status.conditions.Ready",
			value:        "True",
			expectedSQL:  "jsonb_path_query_first(status_conditions, ?::jsonpath) ->> 'status' = ?",
			expectedArgs: []interface{}{`$[*] ? (@.type == "Ready")`, "True"},
		},
		{
			name:         "Ready condition False",
			field:        "status.conditions.Ready",
			value:        "False",
			expectedSQL:  "jsonb_path_query_first(status_conditions, ?::jsonpath) ->> 'status' = ?",
			expectedArgs: []interface{}{`$[*] ? (@.type == "Ready")`, "False"},
		},
		{
			name:         "Available condition True",
			field:        "status.conditions.Available",
			value:        "True",
			expectedSQL:  "jsonb_path_query_first(status_conditions, ?::jsonpath) ->> 'status' = ?",
			expectedArgs: []interface{}{`$[*] ? (@.type == "Available")`, "True"},
		},
		{
			name:         "Available condition Unknown",
			field:        "status.conditions.Available",
			value:        "Unknown",
			expectedSQL:  "jsonb_path_query_first(status_conditions, ?::jsonpath) ->> 'status' = ?",
			expectedArgs: []interface{}{`$[*] ? (@.type == "Available")`, "Unknown"},
		},
		{
			name:         "Progressing condition",
			field:        "status.conditions.Progressing",
			value:        "True",
			expectedSQL:  "jsonb_path_query_first(status_conditions, ?::jsonpath) ->> 'status' = ?",
			expectedArgs: []interface{}{`$[*] ? (@.type == "Progressing")`, "True"},
		},
		{
			name:          "Invalid condition status",
			field:         "status.conditions.Ready",
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
			field:         "status.conditions.Ready_Status",
			value:         "True",
			expectError:   true,
			errorContains: "must be PascalCase",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			node := tsl.Node{
				Func: tsl.EqOp,
				Left: tsl.Node{
					Func: tsl.IdentOp,
					Left: tt.field,
				},
				Right: tsl.Node{
					Func: tsl.StringOp,
					Left: tt.value,
				},
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
		op            string
		value         interface{} // string for time fields, float64 for integer fields
		expectedSQL   string
		expectedArgs  []interface{}
		expectError   bool
		errorContains string
	}{
		// Time subfield: last_updated_time (encoded with __ after preprocessing)
		{
			name:        "last_updated_time less than",
			field:       "status.conditions.Ready__last_updated_time",
			op:          tsl.LtOp,
			value:       "2026-03-06T00:00:00Z",
			expectedSQL: "CAST(jsonb_path_query_first(status_conditions, ?::jsonpath) ->> ? AS TIMESTAMPTZ) < ?::timestamptz",
			expectedArgs: []interface{}{
				`$[*] ? (@.type == "Ready")`,
				"last_updated_time",
				"2026-03-06T00:00:00Z",
			},
		},
		{
			name:        "last_updated_time greater than",
			field:       "status.conditions.Ready__last_updated_time",
			op:          tsl.GtOp,
			value:       "2026-03-06T00:00:00Z",
			expectedSQL: "CAST(jsonb_path_query_first(status_conditions, ?::jsonpath) ->> ? AS TIMESTAMPTZ) > ?::timestamptz",
			expectedArgs: []interface{}{
				`$[*] ? (@.type == "Ready")`,
				"last_updated_time",
				"2026-03-06T00:00:00Z",
			},
		},
		{
			name:        "last_updated_time less than or equal",
			field:       "status.conditions.Ready__last_updated_time",
			op:          tsl.LteOp,
			value:       "2026-03-06T00:00:00Z",
			expectedSQL: "CAST(jsonb_path_query_first(status_conditions, ?::jsonpath) ->> ? AS TIMESTAMPTZ) <= ?::timestamptz",
			expectedArgs: []interface{}{
				`$[*] ? (@.type == "Ready")`,
				"last_updated_time",
				"2026-03-06T00:00:00Z",
			},
		},
		{
			name:        "last_updated_time greater than or equal",
			field:       "status.conditions.Ready__last_updated_time",
			op:          tsl.GteOp,
			value:       "2026-03-06T00:00:00Z",
			expectedSQL: "CAST(jsonb_path_query_first(status_conditions, ?::jsonpath) ->> ? AS TIMESTAMPTZ) >= ?::timestamptz",
			expectedArgs: []interface{}{
				`$[*] ? (@.type == "Ready")`,
				"last_updated_time",
				"2026-03-06T00:00:00Z",
			},
		},
		{
			name:        "last_updated_time equal",
			field:       "status.conditions.Ready__last_updated_time",
			op:          tsl.EqOp,
			value:       "2026-03-06T00:00:00Z",
			expectedSQL: "CAST(jsonb_path_query_first(status_conditions, ?::jsonpath) ->> ? AS TIMESTAMPTZ) = ?::timestamptz",
			expectedArgs: []interface{}{
				`$[*] ? (@.type == "Ready")`,
				"last_updated_time",
				"2026-03-06T00:00:00Z",
			},
		},
		{
			name:        "last_updated_time not equal",
			field:       "status.conditions.Ready__last_updated_time",
			op:          tsl.NotEqOp,
			value:       "2026-03-06T00:00:00Z",
			expectedSQL: "CAST(jsonb_path_query_first(status_conditions, ?::jsonpath) ->> ? AS TIMESTAMPTZ) != ?::timestamptz",
			expectedArgs: []interface{}{
				`$[*] ? (@.type == "Ready")`,
				"last_updated_time",
				"2026-03-06T00:00:00Z",
			},
		},
		// Time subfield: last_transition_time
		{
			name:        "last_transition_time less than",
			field:       "status.conditions.Available__last_transition_time",
			op:          tsl.LtOp,
			value:       "2026-03-06T00:00:00Z",
			expectedSQL: "CAST(jsonb_path_query_first(status_conditions, ?::jsonpath) ->> ? AS TIMESTAMPTZ) < ?::timestamptz",
			expectedArgs: []interface{}{
				`$[*] ? (@.type == "Available")`,
				"last_transition_time",
				"2026-03-06T00:00:00Z",
			},
		},
		// Integer subfield: observed_generation
		{
			name:        "observed_generation less than",
			field:       "status.conditions.Ready__observed_generation",
			op:          tsl.LtOp,
			value:       float64(5),
			expectedSQL: "CAST(jsonb_path_query_first(status_conditions, ?::jsonpath) ->> ? AS INTEGER) < ?",
			expectedArgs: []interface{}{
				`$[*] ? (@.type == "Ready")`,
				"observed_generation",
				5,
			},
		},
		{
			name:        "observed_generation equal",
			field:       "status.conditions.Ready__observed_generation",
			op:          tsl.EqOp,
			value:       float64(3),
			expectedSQL: "CAST(jsonb_path_query_first(status_conditions, ?::jsonpath) ->> ? AS INTEGER) = ?",
			expectedArgs: []interface{}{
				`$[*] ? (@.type == "Ready")`,
				"observed_generation",
				3,
			},
		},
		// Error cases
		{
			name:          "Invalid subfield name",
			field:         "status.conditions.Ready__unknown_field",
			op:            tsl.LtOp,
			value:         "2026-03-06T00:00:00Z",
			expectError:   true,
			errorContains: "not supported",
		},
		{
			name:          "Invalid operator for subfield",
			field:         "status.conditions.Ready__last_updated_time",
			op:            tsl.LikeOp,
			value:         "2026%",
			expectError:   true,
			errorContains: "not supported for condition subfield",
		},
		{
			name:          "Invalid condition type in subfield query",
			field:         "status.conditions.ready__last_updated_time",
			op:            tsl.LtOp,
			value:         "2026-03-06T00:00:00Z",
			expectError:   true,
			errorContains: "must be PascalCase",
		},
		{
			name:          "Invalid timestamp format",
			field:         "status.conditions.Ready__last_updated_time",
			op:            tsl.LtOp,
			value:         "not-a-timestamp",
			expectError:   true,
			errorContains: "expected RFC3339 format",
		},
		{
			name:          "Float value for integer subfield",
			field:         "status.conditions.Ready__observed_generation",
			op:            tsl.LtOp,
			value:         float64(3.5),
			expectError:   true,
			errorContains: "expected integer value",
		},
		{
			name:          "Integer overflow for integer subfield",
			field:         "status.conditions.Ready__observed_generation",
			op:            tsl.LtOp,
			value:         float64(3000000000),
			expectError:   true,
			errorContains: "out of 32-bit integer range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			var rightNode tsl.Node
			switch v := tt.value.(type) {
			case string:
				rightNode = tsl.Node{Func: tsl.StringOp, Left: v}
			case float64:
				rightNode = tsl.Node{Func: tsl.NumberOp, Left: v}
			}

			node := tsl.Node{
				Func: tt.op,
				Left: tsl.Node{
					Func: tsl.IdentOp,
					Left: tt.field,
				},
				Right: rightNode,
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

func TestPreprocessConditionSubfields(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "4-part path is encoded",
			input:    "status.conditions.Ready.last_updated_time < '2026-03-06T00:00:00Z'",
			expected: "status.conditions.Ready__last_updated_time < '2026-03-06T00:00:00Z'",
		},
		{
			name:     "3-part path is unchanged",
			input:    "status.conditions.Ready='True'",
			expected: "status.conditions.Ready='True'",
		},
		{
			name:     "Mixed 3-part and 4-part",
			input:    "status.conditions.Ready='False' AND status.conditions.Ready.last_updated_time < '2026-03-06T00:00:00Z'",
			expected: "status.conditions.Ready='False' AND status.conditions.Ready__last_updated_time < '2026-03-06T00:00:00Z'",
		},
		{
			name:     "Labels are unchanged",
			input:    "labels.environment='production'",
			expected: "labels.environment='production'",
		},
		{
			name:     "last_transition_time is encoded",
			input:    "status.conditions.Available.last_transition_time > '2026-01-01T00:00:00Z'",
			expected: "status.conditions.Available__last_transition_time > '2026-01-01T00:00:00Z'",
		},
		{
			name:     "observed_generation is encoded",
			input:    "status.conditions.Ready.observed_generation < 5",
			expected: "status.conditions.Ready__observed_generation < 5",
		},
		{
			name:     "Text inside single quotes is not encoded",
			input:    "name='status.conditions.Ready.last_updated_time'",
			expected: "name='status.conditions.Ready.last_updated_time'",
		},
		{
			name:     "Text inside double quotes is not encoded",
			input:    `name="status.conditions.Ready.last_updated_time"`,
			expected: `name="status.conditions.Ready.last_updated_time"`,
		},
		{
			name: "Mixed quoted and unquoted segments",
			input: "status.conditions.Ready.last_updated_time < '2026-03-06T00:00:00Z'" +
				" AND name='status.conditions.Ready.last_updated_time'",
			expected: "status.conditions.Ready__last_updated_time < '2026-03-06T00:00:00Z'" +
				" AND name='status.conditions.Ready.last_updated_time'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			result := PreprocessConditionSubfields(tt.input)
			Expect(result).To(Equal(tt.expected))
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
			field:    "status.conditions.Ready",
			expected: true,
		},
		{
			name:     "Encoded subfield (after preprocessing)",
			field:    "status.conditions.Ready__last_updated_time",
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

			node := tsl.Node{
				Func: tsl.EqOp,
				Left: tsl.Node{
					Func: tsl.IdentOp,
					Left: tt.field,
				},
				Right: tsl.Node{
					Func: tsl.StringOp,
					Left: "value",
				},
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
		expectedConditions   int
		expectedConditionSQL string
		expectError          bool
	}{
		{
			name:               "Subfield query only",
			searchQuery:        "status.conditions.Ready.last_updated_time < '2026-03-06T00:00:00Z'",
			expectedConditions: 1,
			expectedConditionSQL: "CAST(jsonb_path_query_first(status_conditions, " +
				"?::jsonpath) ->> ? AS TIMESTAMPTZ) < ?::timestamptz",
		},
		{
			name: "Mixed status and subfield queries",
			searchQuery: "status.conditions.Ready='False' AND " +
				"status.conditions.Ready.last_updated_time < '2026-03-06T00:00:00Z'",
			expectedConditions: 2,
		},
		{
			name:               "Subfield query combined with label query",
			searchQuery:        "labels.region='us-east' AND status.conditions.Ready.last_updated_time < '2026-03-06T00:00:00Z'",
			expectedConditions: 1,
		},
		{
			name:        "NOT operator on condition query returns error",
			searchQuery: "NOT status.conditions.Ready='True'",
			expectError: true,
		},
		{
			name:        "NOT operator on condition subfield query returns error",
			searchQuery: "NOT status.conditions.Ready.last_updated_time < '2026-03-06T00:00:00Z'",
			expectError: true,
		},
		{
			name:        "NOT operator on nested condition under AND returns error",
			searchQuery: "NOT (status.conditions.Ready='True' AND name='test')",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			preprocessed := PreprocessConditionSubfields(tt.searchQuery)
			tslTree, err := tsl.ParseTSL(preprocessed)
			Expect(err).ToNot(HaveOccurred())

			_, conditions, serviceErr := ExtractConditionQueries(tslTree, "clusters")

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
		expectedConditions   int
		expectedConditionSQL string
		expectError          bool
	}{
		{
			name:                 "Single condition query",
			searchQuery:          "status.conditions.Ready='True'",
			expectedConditions:   1,
			expectedConditionSQL: "jsonb_path_query_first(status_conditions, ?::jsonpath) ->> 'status' = ?",
		},
		{
			name:               "No condition queries",
			searchQuery:        "name='test'",
			expectedConditions: 0,
		},
		{
			name:               "Mixed query with condition",
			searchQuery:        "name='test' AND status.conditions.Ready='True'",
			expectedConditions: 1,
		},
		{
			name:               "Multiple condition queries",
			searchQuery:        "status.conditions.Ready='True' AND status.conditions.Available='True'",
			expectedConditions: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			tslTree, err := tsl.ParseTSL(tt.searchQuery)
			Expect(err).ToNot(HaveOccurred())

			_, conditions, serviceErr := ExtractConditionQueries(tslTree, "clusters")

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

func TestHasCondition(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		expected bool
	}{
		{
			name:     "Valid condition field",
			field:    "status.conditions.Ready",
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

			node := tsl.Node{
				Func: tsl.EqOp,
				Left: tsl.Node{
					Func: tsl.IdentOp,
					Left: tt.field,
				},
				Right: tsl.Node{
					Func: tsl.StringOp,
					Left: "value",
				},
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
		{"Valid - Ready", "Ready", true},
		{"Valid - Available", "Available", true},
		{"Valid - Progressing", "Progressing", true},
		{"Valid - CustomCondition", "CustomCondition", true},
		{"Valid - With numbers", "Ready2", true},
		{"Invalid - lowercase", "ready", false},
		{"Invalid - starts with number", "2Ready", false},
		{"Invalid - contains underscore", "Ready_State", false},
		{"Invalid - contains hyphen", "Ready-State", false},
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
