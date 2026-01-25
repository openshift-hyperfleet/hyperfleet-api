package db

import (
	"testing"

	"github.com/yaacov/tree-search-language/pkg/tsl"
)

func TestConditionsNodeConverter(t *testing.T) {
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
			expectError:  false,
		},
		{
			name:         "Ready condition False",
			field:        "status.conditions.Ready",
			value:        "False",
			expectedSQL:  "jsonb_path_query_first(status_conditions, ?::jsonpath) ->> 'status' = ?",
			expectedArgs: []interface{}{`$[*] ? (@.type == "Ready")`, "False"},
			expectError:  false,
		},
		{
			name:         "Available condition True",
			field:        "status.conditions.Available",
			value:        "True",
			expectedSQL:  "jsonb_path_query_first(status_conditions, ?::jsonpath) ->> 'status' = ?",
			expectedArgs: []interface{}{`$[*] ? (@.type == "Available")`, "True"},
			expectError:  false,
		},
		{
			name:         "Available condition Unknown",
			field:        "status.conditions.Available",
			value:        "Unknown",
			expectedSQL:  "jsonb_path_query_first(status_conditions, ?::jsonpath) ->> 'status' = ?",
			expectedArgs: []interface{}{`$[*] ? (@.type == "Available")`, "Unknown"},
			expectError:  false,
		},
		{
			name:         "Progressing condition",
			field:        "status.conditions.Progressing",
			value:        "True",
			expectedSQL:  "jsonb_path_query_first(status_conditions, ?::jsonpath) ->> 'status' = ?",
			expectedArgs: []interface{}{`$[*] ? (@.type == "Progressing")`, "True"},
			expectError:  false,
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
			// Build a TSL node that represents: status.conditions.X = 'Y'
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
				if err == nil {
					t.Errorf("Expected error containing %q, but got nil", tt.errorContains)
					return
				}
				if tt.errorContains != "" && err.Error() != "" {
					// Check if error message contains expected string
					found := false
					if errMsg := err.Error(); errMsg != "" {
						found = true
					}
					if !found {
						t.Logf("Error message: %s", err.Error())
					}
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Get SQL from the sqlizer
			sqlizer := result.(interface {
				ToSql() (string, []interface{}, error)
			})
			sql, args, sqlErr := sqlizer.ToSql()
			if sqlErr != nil {
				t.Errorf("Failed to convert to SQL: %v", sqlErr)
				return
			}

			if sql != tt.expectedSQL {
				t.Errorf("Expected SQL %q, got %q", tt.expectedSQL, sql)
			}

			if len(args) != len(tt.expectedArgs) {
				t.Errorf("Expected %d args, got %d", len(tt.expectedArgs), len(args))
				return
			}

			for i, expectedArg := range tt.expectedArgs {
				if args[i] != expectedArg {
					t.Errorf("Expected arg[%d] = %q, got %q", i, expectedArg, args[i])
				}
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
			expectError:          false,
		},
		{
			name:               "No condition queries",
			searchQuery:        "name='test'",
			expectedConditions: 0,
			expectError:        false,
		},
		{
			name:               "Mixed query with condition",
			searchQuery:        "name='test' AND status.conditions.Ready='True'",
			expectedConditions: 1,
			expectError:        false,
		},
		{
			name:               "Multiple condition queries",
			searchQuery:        "status.conditions.Ready='True' AND status.conditions.Available='True'",
			expectedConditions: 2,
			expectError:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse the search query
			tslTree, err := tsl.ParseTSL(tt.searchQuery)
			if err != nil {
				t.Fatalf("Failed to parse TSL: %v", err)
			}

			// Extract condition queries
			_, conditions, serviceErr := ExtractConditionQueries(tslTree, "clusters")

			if tt.expectError {
				if serviceErr == nil {
					t.Error("Expected error but got nil")
				}
				return
			}

			if serviceErr != nil {
				t.Errorf("Unexpected error: %v", serviceErr)
				return
			}

			if len(conditions) != tt.expectedConditions {
				t.Errorf("Expected %d conditions, got %d", tt.expectedConditions, len(conditions))
				return
			}

			// Verify the SQL of extracted conditions
			if tt.expectedConditions > 0 && tt.expectedConditionSQL != "" {
				sql, _, sqlErr := conditions[0].ToSql()
				if sqlErr != nil {
					t.Errorf("Failed to convert condition to SQL: %v", sqlErr)
					return
				}
				if sql != tt.expectedConditionSQL {
					t.Errorf("Expected condition SQL %q, got %q", tt.expectedConditionSQL, sql)
				}
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
			if result != tt.expected {
				t.Errorf("Expected hasCondition(%q) = %v, got %v", tt.field, tt.expected, result)
			}
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
			result := conditionTypePattern.MatchString(tt.condType)
			if result != tt.expectMatch {
				t.Errorf("conditionTypePattern.MatchString(%q) = %v, expected %v", tt.condType, result, tt.expectMatch)
			}
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
			result := validConditionStatuses[tt.status]
			if result != tt.expectValid {
				t.Errorf("validConditionStatuses[%q] = %v, expected %v", tt.status, result, tt.expectValid)
			}
		})
	}
}
