package db

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/yaacov/tree-search-language/v6/pkg/tsl"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

func walkHelper(t *testing.T, search string) (string, []any) {
	t.Helper()
	RegisterTestingT(t)
	tree, err := tsl.ParseTSL(search)
	Expect(err).ToNot(HaveOccurred())
	sql, values, svcErr := TSLToSQL(tree, WalkConfig{TableName: "resources"})
	Expect(svcErr).To(BeNil())
	return sql, values
}

func walkExpectError(t *testing.T, search string) *errors.ServiceError {
	t.Helper()
	RegisterTestingT(t)
	tree, err := tsl.ParseTSL(search)
	Expect(err).ToNot(HaveOccurred())
	_, _, svcErr := TSLToSQL(tree, WalkConfig{TableName: "resources"})
	Expect(svcErr).ToNot(BeNil())
	return svcErr
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

func TestResolveSpecColumn(t *testing.T) {
	ctx := &walkContext{cfg: WalkConfig{TableName: "resources"}}
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

			field, _, err := resolveSpecColumn(tt.input, ctx)
			if tt.expectError {
				Expect(err).ToNot(BeNil())
			} else {
				Expect(err).To(BeNil())
				Expect(field).To(Equal(tt.expected))
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
			RegisterTestingT(t)

			result := conditionAllowedStatuses[tt.status]
			Expect(result).To(Equal(tt.expectValid))
		})
	}
}

func TestTSLToSQL_BasicFields(t *testing.T) {
	tests := []struct {
		name   string
		search string
		sql    string
		values []any
	}{
		{
			name:   "bare field is table-prefixed",
			search: "id = 'cls-123'",
			sql:    "resources.id = ?",
			values: []any{"cls-123"},
		},
		{
			name:   "IN operator",
			search: "created_by in ['ooo.openshift']",
			sql:    "resources.created_by IN (?)",
			values: []any{"ooo.openshift"},
		},
		{
			name:   "spec 1-level JSONB",
			search: "spec.region = 'us-east-1'",
			sql:    "spec->>'region' = ?",
			values: []any{"us-east-1"},
		},
		{
			name:   "spec 2-level JSONB",
			search: "spec.release.version = '2'",
			sql:    "spec->'release'->>'version' = ?",
			values: []any{"2"},
		},
		{
			name:   "spec 3-level JSONB",
			search: "spec.release.notes.url = 'https://example.com'",
			sql:    "spec->'release'->'notes'->>'url' = ?",
			values: []any{"https://example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			sql, values := walkHelper(t, tt.search)
			Expect(sql).To(Equal(tt.sql))
			Expect(values).To(ConsistOf(tt.values))
		})
	}
}

func TestTSLToSQL_NumericCast(t *testing.T) {
	t.Run("spec field with numeric RHS — CAST applied", func(t *testing.T) {
		RegisterTestingT(t)
		sql, _ := walkHelper(t, "spec.replicas > 9")
		Expect(sql).To(Equal("CAST(spec->>'replicas' AS numeric) > ?"))
	})

	t.Run("nested spec field with numeric RHS — CAST applied", func(t *testing.T) {
		RegisterTestingT(t)
		sql, _ := walkHelper(t, "spec.release.version > 9")
		Expect(sql).To(Equal("CAST(spec->'release'->>'version' AS numeric) > ?"))
	})

	t.Run("deep nested spec field with numeric RHS — CAST applied", func(t *testing.T) {
		RegisterTestingT(t)
		sql, _ := walkHelper(t, "spec.release.config.replicas > 9")
		Expect(sql).To(Equal("CAST(spec->'release'->'config'->>'replicas' AS numeric) > ?"))
	})

	t.Run("spec field with string RHS — no CAST", func(t *testing.T) {
		RegisterTestingT(t)
		sql, _ := walkHelper(t, "spec.channel = 'dev'")
		Expect(sql).To(Equal("spec->>'channel' = ?"))
	})

	t.Run("non-spec field with numeric RHS — no CAST", func(t *testing.T) {
		RegisterTestingT(t)
		sql, _ := walkHelper(t, "generation > 1")
		Expect(sql).To(Equal("resources.generation > ?"))
	})

	t.Run("numeric LHS with spec field RHS — CAST applied", func(t *testing.T) {
		RegisterTestingT(t)
		sql, _ := walkHelper(t, "9 < spec.replicas")
		Expect(sql).To(Equal("? < CAST(spec->>'replicas' AS numeric)"))
	})

	t.Run("AND tree: only spec+numeric nodes get CAST", func(t *testing.T) {
		RegisterTestingT(t)
		sql, _ := walkHelper(t, "spec.replicas > 9 AND generation > 1 AND spec.channel = 'dev'")
		Expect(sql).To(ContainSubstring("CAST(spec->>'replicas' AS numeric) > ?"))
		Expect(sql).To(ContainSubstring("resources.generation > ?"))
		Expect(sql).To(ContainSubstring("spec->>'channel' = ?"))
		Expect(sql).ToNot(ContainSubstring("CAST(resources.generation"))
		Expect(sql).ToNot(ContainSubstring("CAST(spec->>'channel'"))
	})
}

func TestTSLToSQL_TypedFieldValidation(t *testing.T) {
	tests := []struct {
		name          string
		search        string
		errorContains string
		expectError   bool
	}{
		{
			name:   "generation with valid integer",
			search: "generation = 5",
		},
		{
			name:          "generation with string value",
			search:        "generation = 'abc'",
			expectError:   true,
			errorContains: "field 'generation' expects an integer",
		},
		{
			name:   "created_time with valid RFC3339 timestamp",
			search: "created_time > '2026-01-01T00:00:00Z'",
		},
		{
			name:          "created_time with non-timestamp value",
			search:        "created_time = 'not-a-date'",
			expectError:   true,
			errorContains: "field 'created_time' expects an RFC3339 timestamp",
		},
		{
			name:          "generation IN list with string element",
			search:        "generation IN [1, 'abc']",
			expectError:   true,
			errorContains: "field 'generation' expects an integer",
		},
		{
			name:   "generation IN list with all integers",
			search: "generation IN [1, 2, 3]",
		},
		{
			name:   "deleted_time IS NULL accepted",
			search: "deleted_time IS NULL",
		},
		{
			name:   "deleted_time IS NOT NULL accepted",
			search: "deleted_time IS NOT NULL",
		},
		{
			name:   "created_time with date literal",
			search: "created_time > '2026-01-01'",
		},
		{
			name:          "literal-first: string = generation",
			search:        "'abc' = generation",
			expectError:   true,
			errorContains: "field 'generation' expects an integer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			tree, err := tsl.ParseTSL(tt.search)
			Expect(err).ToNot(HaveOccurred())

			_, _, svcErr := TSLToSQL(tree, WalkConfig{TableName: "resources"})
			if tt.expectError {
				Expect(svcErr).ToNot(BeNil())
				Expect(svcErr.Error()).To(ContainSubstring(tt.errorContains))
			} else {
				Expect(svcErr).To(BeNil())
			}
		})
	}
}

func TestTSLToSQL_ConditionStatus(t *testing.T) {
	tests := []struct {
		name          string
		search        string
		sqlContains   string
		errorContains string
		expectedArgs  []any
		expectError   bool
	}{
		{
			name:   "Reconciled = True",
			search: "status.conditions.Reconciled = 'True'",
			sqlContains: "(SELECT rc.status FROM resource_conditions rc" +
				" WHERE rc.resource_id = resources.id AND rc.type = ?) = ?",
			expectedArgs: []any{"Reconciled", "True"},
		},
		{
			name:   "Available = False",
			search: "status.conditions.Available = 'False'",
			sqlContains: "(SELECT rc.status FROM resource_conditions rc" +
				" WHERE rc.resource_id = resources.id AND rc.type = ?) = ?",
			expectedArgs: []any{"Available", "False"},
		},
		{
			name:          "invalid status value",
			search:        "status.conditions.Reconciled = 'Invalid'",
			expectError:   true,
			errorContains: "condition status 'Invalid' is invalid",
		},
		{
			name:          "lowercase condition type",
			search:        "status.conditions.ready = 'True'",
			expectError:   true,
			errorContains: "must be PascalCase",
		},
		{
			name:          "underscore in condition type",
			search:        "status.conditions.Reconciled_Status = 'True'",
			expectError:   true,
			errorContains: "must be PascalCase",
		},
		{
			name:          "only equality for status",
			search:        "status.conditions.Reconciled != 'True'",
			expectError:   true,
			errorContains: "only equality operator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			tree, err := tsl.ParseTSL(tt.search)
			Expect(err).ToNot(HaveOccurred())

			sql, values, svcErr := TSLToSQL(tree, WalkConfig{TableName: "resources"})
			if tt.expectError {
				Expect(svcErr).ToNot(BeNil())
				Expect(svcErr.Error()).To(ContainSubstring(tt.errorContains))
				return
			}

			Expect(svcErr).To(BeNil())
			Expect(sql).To(Equal(tt.sqlContains))
			Expect(values).To(Equal(tt.expectedArgs))
		})
	}
}

func TestTSLToSQL_ConditionSubfields(t *testing.T) {
	tests := []struct {
		name          string
		search        string
		expectedSQL   string
		errorContains string
		expectedArgs  []any
		expectError   bool
	}{
		{
			name:   "last_updated_time less than",
			search: "status.conditions.Reconciled.last_updated_time < '2026-03-06T00:00:00Z'",
			expectedSQL: "(SELECT rc.last_updated_time FROM resource_conditions rc" +
				" WHERE rc.resource_id = resources.id AND rc.type = ?) < ?",
			expectedArgs: []any{"Reconciled", "2026-03-06T00:00:00Z"},
		},
		{
			name:   "last_transition_time greater than",
			search: "status.conditions.Available.last_transition_time > '2026-03-06T00:00:00Z'",
			expectedSQL: "(SELECT rc.last_transition_time FROM resource_conditions rc" +
				" WHERE rc.resource_id = resources.id AND rc.type = ?) > ?",
			expectedArgs: []any{"Available", "2026-03-06T00:00:00Z"},
		},
		{
			name:   "observed_generation less than",
			search: "status.conditions.Reconciled.observed_generation < 5",
			expectedSQL: "(SELECT rc.observed_generation FROM resource_conditions rc" +
				" WHERE rc.resource_id = resources.id AND rc.type = ?) < ?",
			expectedArgs: []any{"Reconciled", float64(5)},
		},
		{
			name:          "invalid subfield",
			search:        "status.conditions.Reconciled.unknown_field < '2026-03-06T00:00:00Z'",
			expectError:   true,
			errorContains: "not supported",
		},
		{
			name:          "invalid timestamp format",
			search:        "status.conditions.Reconciled.last_updated_time < 'not-a-timestamp'",
			expectError:   true,
			errorContains: "expected RFC3339 format",
		},
		{
			name:          "float value for integer subfield",
			search:        "status.conditions.Reconciled.observed_generation < 3.5",
			expectError:   true,
			errorContains: "expected integer value",
		},
		{
			name:          "integer overflow",
			search:        "status.conditions.Reconciled.observed_generation < 3000000000",
			expectError:   true,
			errorContains: "out of 32-bit integer range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			tree, err := tsl.ParseTSL(tt.search)
			Expect(err).ToNot(HaveOccurred())

			sql, values, svcErr := TSLToSQL(tree, WalkConfig{TableName: "resources"})
			if tt.expectError {
				Expect(svcErr).ToNot(BeNil())
				Expect(svcErr.Error()).To(ContainSubstring(tt.errorContains))
				return
			}

			Expect(svcErr).To(BeNil())
			Expect(sql).To(Equal(tt.expectedSQL))
			Expect(values).To(Equal(tt.expectedArgs))
		})
	}
}

func TestTSLToSQL_LabelQueries(t *testing.T) {
	t.Run("label equality", func(t *testing.T) {
		RegisterTestingT(t)
		sql, values := walkHelper(t, "labels.env = 'prod'")
		Expect(sql).To(ContainSubstring("SELECT value FROM resource_labels"))
		Expect(sql).To(ContainSubstring("resource_labels.key = ?"))
		Expect(values).To(ContainElement("env"))
	})

	t.Run("label in AND", func(t *testing.T) {
		RegisterTestingT(t)
		sql, values := walkHelper(t, "labels.env = 'prod' AND name = 'test'")
		Expect(sql).To(ContainSubstring("SELECT value FROM resource_labels"))
		Expect(values).To(ContainElement("env"))
		Expect(values).To(ContainElement("prod"))
		Expect(values).To(ContainElement("test"))
	})

	t.Run("label not equal", func(t *testing.T) {
		RegisterTestingT(t)
		sql, values := walkHelper(t, "labels.env != 'prod'")
		Expect(sql).To(ContainSubstring("SELECT value FROM resource_labels"))
		Expect(sql).To(ContainSubstring("resource_labels.key = ?"))
		Expect(sql).To(ContainSubstring("!= ?"))
		Expect(values).To(ContainElement("env"))
		Expect(values).To(ContainElement("prod"))
	})

	t.Run("label IN", func(t *testing.T) {
		RegisterTestingT(t)
		sql, values := walkHelper(t, "labels.env IN ['prod', 'staging']")
		Expect(sql).To(ContainSubstring("SELECT value FROM resource_labels"))
		Expect(sql).To(ContainSubstring("resource_labels.key = ?"))
		Expect(sql).To(ContainSubstring("IN (?, ?)"))
		Expect(values).To(ContainElement("env"))
		Expect(values).To(ContainElement("prod"))
		Expect(values).To(ContainElement("staging"))
	})

	t.Run("label LIKE", func(t *testing.T) {
		RegisterTestingT(t)
		sql, values := walkHelper(t, "labels.env LIKE 'prod%'")
		Expect(sql).To(ContainSubstring("SELECT value FROM resource_labels"))
		Expect(sql).To(ContainSubstring("resource_labels.key = ?"))
		Expect(sql).To(ContainSubstring("LIKE ?"))
		Expect(values).To(ContainElement("env"))
		Expect(values).To(ContainElement("prod%"))
	})

	t.Run("empty label key rejected", func(t *testing.T) {
		svcErr := walkExpectError(t, "labels. = 'x'")
		Expect(svcErr.Error()).To(ContainSubstring("label key cannot be empty"))
	})
}

func TestTSLToSQL_NotRejection(t *testing.T) {
	t.Run("NOT on condition rejected", func(t *testing.T) {
		svcErr := walkExpectError(t, "NOT (status.conditions.Reconciled = 'True')")
		Expect(svcErr.Error()).To(ContainSubstring("NOT operator is not supported"))
	})

	t.Run("NOT on condition subfield rejected", func(t *testing.T) {
		svcErr := walkExpectError(t, "NOT (status.conditions.Reconciled.last_updated_time < '2026-03-06T00:00:00Z')")
		Expect(svcErr.Error()).To(ContainSubstring("NOT operator is not supported"))
	})

	t.Run("NOT on label rejected", func(t *testing.T) {
		svcErr := walkExpectError(t, "NOT (labels.env = 'prod')")
		Expect(svcErr.Error()).To(ContainSubstring("NOT operator is not supported"))
	})
}

func TestTSLToSQL_OrWithLabelsAndConditions(t *testing.T) {
	t.Run("OR with condition allowed", func(t *testing.T) {
		RegisterTestingT(t)
		sql, values := walkHelper(t, "name = 'test' OR status.conditions.Reconciled = 'True'")
		Expect(sql).To(ContainSubstring("OR"))
		Expect(sql).To(ContainSubstring("SELECT rc.status FROM resource_conditions rc"))
		Expect(values).To(ContainElement("Reconciled"))
		Expect(values).To(ContainElement("True"))
	})

	t.Run("nested OR with condition allowed", func(t *testing.T) {
		RegisterTestingT(t)
		sql, _ := walkHelper(t, "name = 'a' AND (kind = 'us' OR status.conditions.Reconciled = 'True')")
		Expect(sql).To(ContainSubstring("OR"))
		Expect(sql).To(ContainSubstring("SELECT rc.status FROM resource_conditions rc"))
	})

	t.Run("OR with label allowed", func(t *testing.T) {
		RegisterTestingT(t)
		sql, values := walkHelper(t, "labels.env = 'prod' OR name = 'foo'")
		Expect(sql).To(ContainSubstring("OR"))
		Expect(sql).To(ContainSubstring("SELECT value FROM resource_labels"))
		Expect(values).To(ContainElement("env"))
	})

	t.Run("OR between two labels allowed", func(t *testing.T) {
		RegisterTestingT(t)
		sql, values := walkHelper(t, "labels.env = 'prod' OR labels.tier = 'frontend'")
		Expect(sql).To(ContainSubstring("OR"))
		Expect(values).To(ContainElement("env"))
		Expect(values).To(ContainElement("tier"))
	})

	t.Run("OR without conditions or labels still works", func(t *testing.T) {
		RegisterTestingT(t)
		sql, _ := walkHelper(t, "name = 'test' OR kind = 'us'")
		Expect(sql).To(Equal("(resources.name = ?) OR (resources.kind = ?)"))
	})
}

func TestTSLToSQL_FieldValidation(t *testing.T) {
	t.Run("invalid field name rejected", func(t *testing.T) {
		svcErr := walkExpectError(t, "properties.owner = 'team_a'")
		Expect(svcErr.Error()).To(ContainSubstring("is not a valid field name"))
	})
}

func TestTSLToSQL_ResolveRelated(t *testing.T) {
	t.Run("related field resolved via callback", func(t *testing.T) {
		RegisterTestingT(t)
		tree, err := tsl.ParseTSL("creator.username = 'alice'")
		Expect(err).ToNot(HaveOccurred())

		sql, values, svcErr := TSLToSQL(tree, WalkConfig{
			TableName: "resources",
			ResolveRelated: func(name string) (string, error) {
				return "users.username", nil
			},
		})
		Expect(svcErr).To(BeNil())
		Expect(sql).To(Equal("users.username = ?"))
		Expect(values).To(ConsistOf("alice"))
	})

	t.Run("related field without callback rejected", func(t *testing.T) {
		RegisterTestingT(t)
		tree, err := tsl.ParseTSL("creator.username = 'alice'")
		Expect(err).ToNot(HaveOccurred())

		_, _, svcErr := TSLToSQL(tree, WalkConfig{TableName: "resources"})
		Expect(svcErr).ToNot(BeNil())
	})
}

func TestTSLToSQL_LogicalOperators(t *testing.T) {
	t.Run("AND", func(t *testing.T) {
		RegisterTestingT(t)
		sql, _ := walkHelper(t, "name = 'a' AND kind = 'b'")
		Expect(sql).To(Equal("(resources.name = ?) AND (resources.kind = ?)"))
	})

	t.Run("OR", func(t *testing.T) {
		RegisterTestingT(t)
		sql, _ := walkHelper(t, "name = 'a' OR kind = 'b'")
		Expect(sql).To(Equal("(resources.name = ?) OR (resources.kind = ?)"))
	})

	t.Run("NOT", func(t *testing.T) {
		RegisterTestingT(t)
		sql, _ := walkHelper(t, "NOT (name = 'a')")
		Expect(sql).To(Equal("NOT (resources.name = ?)"))
	})
}

func TestTSLToSQL_StringMatch(t *testing.T) {
	t.Run("LIKE", func(t *testing.T) {
		RegisterTestingT(t)
		sql, values := walkHelper(t, "name LIKE 'test%'")
		Expect(sql).To(Equal("resources.name LIKE ?"))
		Expect(values).To(ConsistOf("test%"))
	})

	t.Run("ILIKE", func(t *testing.T) {
		RegisterTestingT(t)
		sql, values := walkHelper(t, "name ILIKE 'test%'")
		Expect(sql).To(Equal("resources.name ILIKE ?"))
		Expect(values).To(ConsistOf("test%"))
	})
}

func TestTSLToSQL_IsNull(t *testing.T) {
	t.Run("IS NULL", func(t *testing.T) {
		RegisterTestingT(t)
		sql, _ := walkHelper(t, "deleted_time IS NULL")
		Expect(sql).To(Equal("resources.deleted_time IS NULL"))
	})
}

func TestTSLToSQL_Between(t *testing.T) {
	t.Run("BETWEEN", func(t *testing.T) {
		RegisterTestingT(t)
		sql, values := walkHelper(t, "generation BETWEEN 1 AND 10")
		Expect(sql).To(Equal("resources.generation BETWEEN ? AND ?"))
		Expect(values).To(ConsistOf(float64(1), float64(10)))
	})
}

func TestTSLToSQL_ConditionReversedOperand(t *testing.T) {
	t.Run("reversed operand: literal = condition", func(t *testing.T) {
		RegisterTestingT(t)
		sql, values := walkHelper(t, "'True' = status.conditions.Reconciled")
		Expect(sql).To(ContainSubstring("SELECT rc.status FROM resource_conditions rc"))
		Expect(values).To(ContainElement("True"))
		Expect(values).To(ContainElement("Reconciled"))
	})

	t.Run("reversed operand: invalid status still rejected", func(t *testing.T) {
		svcErr := walkExpectError(t, "'Invalid' = status.conditions.Reconciled")
		Expect(svcErr.Error()).To(ContainSubstring("condition status 'Invalid' is invalid"))
	})
}

func TestTSLToSQL_ConditionNoStateLeak(t *testing.T) {
	t.Run("IS NULL on condition followed by plain comparison", func(t *testing.T) {
		RegisterTestingT(t)
		sql, values := walkHelper(t, "status.conditions.Reconciled.last_updated_time IS NULL AND name = 'x'")
		Expect(sql).To(ContainSubstring("IS NULL"))
		Expect(sql).To(ContainSubstring("resources.name = ?"))
		Expect(values).To(ContainElement("Reconciled"))
		Expect(values).To(ContainElement("x"))
	})

	t.Run("IS NULL on condition status", func(t *testing.T) {
		RegisterTestingT(t)
		sql, values := walkHelper(t, "status.conditions.Reconciled IS NULL")
		Expect(sql).To(ContainSubstring("IS NULL"))
		Expect(values).To(ContainElement("Reconciled"))
	})
}

func TestTSLToSQL_ConditionRejectedOperators(t *testing.T) {
	t.Run("IN rejected for conditions", func(t *testing.T) {
		svcErr := walkExpectError(t, "status.conditions.Reconciled IN ['True', 'False']")
		Expect(svcErr.Error()).To(ContainSubstring("IN is not supported for condition queries"))
	})

	t.Run("LIKE rejected for conditions", func(t *testing.T) {
		svcErr := walkExpectError(t, "status.conditions.Reconciled LIKE 'Tr%'")
		Expect(svcErr.Error()).To(ContainSubstring("LIKE/ILIKE is not supported for condition queries"))
	})

	t.Run("BETWEEN rejected for conditions", func(t *testing.T) {
		query := "status.conditions.Reconciled.last_updated_time " +
			"BETWEEN '2026-01-01T00:00:00Z' AND '2026-02-01T00:00:00Z'"
		svcErr := walkExpectError(t, query)
		Expect(svcErr.Error()).To(ContainSubstring("BETWEEN is not supported for condition queries"))
	})
}
