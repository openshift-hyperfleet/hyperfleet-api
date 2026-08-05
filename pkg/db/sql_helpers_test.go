package db

import (
	"testing"

	. "github.com/onsi/gomega"
)

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
			name:     "standard fields",
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
			name:     "field with digit",
			input:    []string{"release_v2 asc"},
			expected: []string{"release_v2 asc"},
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
