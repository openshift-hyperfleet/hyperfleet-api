package configloader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/criteria"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// baseTaskConfig returns a minimal valid AdapterTaskConfig for testing.
// Tests can modify the returned config to set up specific scenarios.
func baseTaskConfig() *AdapterTaskConfig {
	return &AdapterTaskConfig{}
}

// newTaskValidator is a helper that creates a TaskConfigValidator with semantic validation
func newTaskValidator(cfg *AdapterTaskConfig) *TaskConfigValidator {
	return NewTaskConfigValidator(cfg, "")
}

func TestValidateActionBaseName(t *testing.T) {
	t.Run("valid name", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Preconditions = []Precondition{{
			ActionBase: ActionBase{Name: "fetchCluster"},
			Expression: "true",
		}}
		err := newTaskValidator(cfg).ValidateStructure()
		require.NoError(t, err)
	})

	t.Run("invalid name with hyphen", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Preconditions = []Precondition{{
			ActionBase: ActionBase{Name: "fetch-cluster"},
			Expression: "true",
		}}
		err := newTaskValidator(cfg).ValidateStructure()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must start with lowercase letter and contain only letters, numbers, underscores")
	})

	t.Run("invalid name starting with uppercase", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Preconditions = []Precondition{{
			ActionBase: ActionBase{Name: "FetchCluster"},
			Expression: "true",
		}}
		err := newTaskValidator(cfg).ValidateStructure()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must start with lowercase letter and contain only letters, numbers, underscores")
	})

	t.Run("post action invalid name with hyphen", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Post = &PostConfig{
			PostActions: []PostAction{{
				ActionBase: ActionBase{
					Name:    "update-status",
					APICall: &APICall{Method: "PUT", URL: "/status"},
				},
			}},
		}
		err := newTaskValidator(cfg).ValidateStructure()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must start with lowercase letter and contain only letters, numbers, underscores")
	})
}

func TestValidateConditionOperators(t *testing.T) {
	// Helper to create task config with a single condition
	withCondition := func(cond Condition) *AdapterTaskConfig {
		cfg := baseTaskConfig()
		cfg.Preconditions = []Precondition{{
			ActionBase: ActionBase{Name: "checkStatus"},
			Conditions: []Condition{cond},
		}}
		return cfg
	}

	t.Run("valid operators", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Preconditions = []Precondition{{
			ActionBase: ActionBase{Name: "checkStatus"},
			Conditions: []Condition{
				{Field: "status", Operator: "equals", Value: "Ready"},
				{Field: "provider", Operator: "in", Value: []interface{}{"aws", "gcp"}},
				{Field: "vpcId", Operator: "exists"},
			},
		}}
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})

	t.Run("invalid operator", func(t *testing.T) {
		cfg := withCondition(Condition{Field: "status", Operator: "invalidOp", Value: "Ready"})
		err := newTaskValidator(cfg).ValidateStructure()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid operator")
	})

	t.Run("missing operator", func(t *testing.T) {
		cfg := withCondition(Condition{Field: "status", Value: "Ready"})
		err := newTaskValidator(cfg).ValidateStructure()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "operator")
	})

	t.Run("missing value for equals operator", func(t *testing.T) {
		cfg := withCondition(Condition{Field: "status", Operator: "equals"})
		v := newTaskValidator(cfg)
		_ = v.ValidateStructure()
		err := v.ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "value is required for operator \"equals\"")
	})

	t.Run("missing value for in operator", func(t *testing.T) {
		cfg := withCondition(Condition{Field: "provider", Operator: "in"})
		v := newTaskValidator(cfg)
		_ = v.ValidateStructure()
		err := v.ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "value is required for operator \"in\"")
	})

	t.Run("non-list value for in operator", func(t *testing.T) {
		cfg := withCondition(Condition{Field: "provider", Operator: "in", Value: "aws"})
		v := newTaskValidator(cfg)
		_ = v.ValidateStructure()
		err := v.ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "value must be a list for operator \"in\"")
	})

	t.Run("non-list value for notIn operator", func(t *testing.T) {
		cfg := withCondition(Condition{Field: "provider", Operator: "notIn", Value: "aws"})
		v := newTaskValidator(cfg)
		_ = v.ValidateStructure()
		err := v.ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "value must be a list for operator \"notIn\"")
	})

	t.Run("exists operator without value is valid", func(t *testing.T) {
		cfg := withCondition(Condition{Field: "vpcId", Operator: "exists"})
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})

	t.Run("exists operator with value should fail", func(t *testing.T) {
		cfg := withCondition(Condition{Field: "vpcId", Operator: "exists", Value: "any-value"})
		v := newTaskValidator(cfg)
		_ = v.ValidateStructure()
		err := v.ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "value/values should not be set for operator \"exists\"")
	})

	t.Run("exists operator with list value should fail", func(t *testing.T) {
		cfg := withCondition(Condition{Field: "vpcId", Operator: "exists", Value: []interface{}{"a", "b"}})
		v := newTaskValidator(cfg)
		_ = v.ValidateStructure()
		err := v.ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "value/values should not be set for operator \"exists\"")
	})

	t.Run("missing value for greaterThan operator", func(t *testing.T) {
		cfg := withCondition(Condition{Field: "count", Operator: "greaterThan"})
		v := newTaskValidator(cfg)
		_ = v.ValidateStructure()
		err := v.ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "value is required for operator \"greaterThan\"")
	})
}

func TestValidateTemplateVariables(t *testing.T) {
	t.Run("defined variables", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Params = []Parameter{
			{Name: "clusterId", Source: StringSource("event.id")},
			{Name: "apiUrl", Source: StringSource("env.API_URL")},
			{Name: "clusterData", Source: APICallSource(&APICall{
				Method: "GET",
				URL:    "{{ .apiUrl }}/clusters/{{ .clusterId }}",
			})},
		}
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})

	t.Run("undefined variable in URL", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Params = []Parameter{
			{Name: "clusterId", Source: StringSource("event.id")},
			{Name: "clusterData", Source: APICallSource(&APICall{
				Method: "GET",
				URL:    "{{ .undefinedVar }}/clusters/{{ .clusterId }}",
			})},
		}
		v := newTaskValidator(cfg)
		_ = v.ValidateStructure()
		err := v.ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "undefined template variable \"undefinedVar\"")
	})

	t.Run("undefined variable in resource manifest", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Params = []Parameter{{Name: "clusterId", Source: StringSource("event.id")}}
		cfg.Resources = []Resource{{
			Name: "testNs",
			Manifest: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata":   map[string]interface{}{"name": "ns-{{ .undefinedVar }}"},
			},
			Discovery: &DiscoveryConfig{Namespace: "*", ByName: "ns-{{ .clusterId }}"},
		}}
		v := newTaskValidator(cfg)
		_ = v.ValidateStructure()
		err := v.ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "undefined template variable \"undefinedVar\"")
	})

	t.Run("derived param variable is available for resources", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Params = []Parameter{
			{Name: "apiUrl", Source: StringSource("env.API_URL")},
			{Name: "clusterData", Source: APICallSource(&APICall{Method: "GET", URL: "{{ .apiUrl }}/clusters"})},
			{Name: "clusterName", Source: StringSource("clusterData.name")},
		}
		cfg.Resources = []Resource{{
			Name: "testNs",
			Manifest: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata":   map[string]interface{}{"name": "ns-{{ .clusterName }}"},
			},
			Discovery: &DiscoveryConfig{Namespace: "*", ByName: "ns-{{ .clusterName }}"},
		}}
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})
}

func TestValidateCELExpressions(t *testing.T) {
	// Helper to create config with a CEL expression precondition
	withExpression := func(expr string) *AdapterTaskConfig {
		cfg := baseTaskConfig()
		cfg.Preconditions = []Precondition{{ActionBase: ActionBase{Name: "check"}, Expression: expr}}
		return cfg
	}

	t.Run("valid CEL expression", func(t *testing.T) {
		cfg := withExpression(`clusterPhase == "Ready" || clusterPhase == "Provisioning"`)
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})

	t.Run("invalid CEL expression - syntax error", func(t *testing.T) {
		cfg := withExpression(`clusterPhase ==== "Ready"`)
		v := newTaskValidator(cfg)
		_ = v.ValidateStructure()
		err := v.ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "CEL parse error")
	})

	t.Run("valid CEL with has() function", func(t *testing.T) {
		cfg := withExpression(
			`has(cluster.status) && ` +
				`cluster.status.conditions.exists(c, c.type == "Reconciled" && c.status == "True")`,
		)
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})
}

func TestValidatePayloadWhenCELExpression(t *testing.T) {
	t.Run("valid payload when expression", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Post = &PostConfig{
			Payloads: []Payload{{
				Name:  "testPayload",
				Build: map[string]interface{}{"status": "ok"},
				When:  &PostActionWhen{Expression: `!adapter.resourcesSkipped`},
			}},
		}
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})

	t.Run("invalid payload when expression - syntax error", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Post = &PostConfig{
			Payloads: []Payload{{
				Name:  "testPayload",
				Build: map[string]interface{}{"status": "ok"},
				When:  &PostActionWhen{Expression: `=== invalid ===`},
			}},
		}
		v := newTaskValidator(cfg)
		_ = v.ValidateStructure()
		err := v.ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "CEL parse error")
	})

	t.Run("payload without when - no validation error", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Post = &PostConfig{
			Payloads: []Payload{{
				Name:  "testPayload",
				Build: map[string]interface{}{"status": "ok"},
			}},
		}
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})
}

func TestValidatePostActionWhenCELExpression(t *testing.T) {
	t.Run("valid post-action when expression", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Post = &PostConfig{
			PostActions: []PostAction{{
				ActionBase: ActionBase{Name: "reportStatus"},
				When:       &PostActionWhen{Expression: `adapter.?executionStatus.orValue("") == "success"`},
			}},
		}
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})

	t.Run("invalid post-action when expression - syntax error", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Post = &PostConfig{
			PostActions: []PostAction{{
				ActionBase: ActionBase{Name: "reportStatus"},
				When:       &PostActionWhen{Expression: `=== invalid ===`},
			}},
		}
		v := newTaskValidator(cfg)
		_ = v.ValidateStructure()
		err := v.ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "CEL parse error")
	})
}

func TestValidateK8sManifests(t *testing.T) {
	// Helper to create config with a resource manifest
	withResource := func(manifest map[string]interface{}) *AdapterTaskConfig {
		cfg := baseTaskConfig()
		cfg.Resources = []Resource{{
			Name:      "testResource",
			Manifest:  manifest,
			Discovery: &DiscoveryConfig{Namespace: "*", ByName: "test"},
		}}
		return cfg
	}

	// Complete valid manifest
	validManifest := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]interface{}{
			"name":   "test-namespace",
			"labels": map[string]interface{}{"app": "test"},
		},
	}

	t.Run("valid K8s manifest", func(t *testing.T) {
		cfg := withResource(validManifest)
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})

	t.Run("map manifest without apiVersion defers validation to execution", func(t *testing.T) {
		// K8s structural validation is deferred to execution time since
		// all manifests are rendered as Go templates
		cfg := withResource(map[string]interface{}{
			"kind":     "Namespace",
			"metadata": map[string]interface{}{"name": "test"},
		})
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})

	t.Run("valid manifest ref", func(t *testing.T) {
		cfg := withResource(map[string]interface{}{"ref": "templates/deployment.yaml"})
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})

	t.Run("empty manifest ref", func(t *testing.T) {
		cfg := withResource(map[string]interface{}{"ref": ""})
		v := newTaskValidator(cfg)
		_ = v.ValidateStructure()
		err := v.ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "manifest ref cannot be empty")
	})

	t.Run("string manifest skips K8s validation", func(t *testing.T) {
		// String manifests contain raw Go templates that can't be validated
		// until template rendering at execution time
		cfg := baseTaskConfig()
		cfg.Params = []Parameter{
			{Name: "name", Source: StringSource("event.name"), Type: "string"},
			{Name: "addLabels", Source: StringSource("event.addLabels"), Type: "bool"},
			{Name: "appName", Source: StringSource("event.appName"), Type: "string"},
		}
		cfg.Resources = []Resource{{
			Name: "testResource",
			Manifest: `apiVersion: v1
kind: ConfigMap
metadata:
  name: "{{ .name }}"
{{ if .addLabels }}
  labels:
    app: "{{ .appName }}"
{{ end }}`,
			Discovery: &DiscoveryConfig{Namespace: "*", ByName: "test"},
		}}
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})

	t.Run("string manifest without K8s fields passes validation", func(t *testing.T) {
		// Even a string manifest missing apiVersion/kind should pass validation
		// because validation is deferred to execution time
		cfg := baseTaskConfig()
		cfg.Resources = []Resource{{
			Name:      "testResource",
			Manifest:  `{{ if .condition }}full: template{{ end }}`,
			Discovery: &DiscoveryConfig{Namespace: "*", ByName: "test"},
		}}
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})

	t.Run("map manifest defers K8s validation to execution", func(t *testing.T) {
		// All manifests are rendered as Go templates — K8s structural
		// validation is deferred to execution time
		cfg := withResource(map[string]interface{}{
			"kind": "Namespace",
			// missing apiVersion and metadata — caught at execution time, not load time
		})
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})
}

func TestValidOperators(t *testing.T) {
	// Verify all expected operators are defined in criteria package
	expectedOperators := []string{
		"equals", "notEquals", "in", "notIn",
		"contains", "greaterThan", "lessThan", "exists",
	}

	for _, op := range expectedOperators {
		assert.True(t, criteria.IsValidOperator(op), "operator %s should be valid", op)
	}
}

func TestValidationErrorsFormat(t *testing.T) {
	errors := &ValidationErrors{}
	errors.Add("path.to.field", "some error message")
	errors.Add("another.path", "another error")

	assert.True(t, errors.HasErrors())
	assert.Len(t, errors.Errors, 2)
	assert.Contains(t, errors.Error(), "validation failed with 2 error(s)")
	assert.Contains(t, errors.Error(), "path.to.field: some error message")
	assert.Contains(t, errors.Error(), "another.path: another error")
}

func TestValidateSemantic(t *testing.T) {
	// Test that ValidateSemantic catches multiple errors
	cfg := baseTaskConfig()
	cfg.Preconditions = []Precondition{
		{
			ActionBase: ActionBase{Name: "check1"},
			Conditions: []Condition{{Field: "status", Operator: "badOperator", Value: "Ready"}},
		},
		{ActionBase: ActionBase{Name: "check2"}, Expression: "invalid ))) syntax"},
	}
	cfg.Resources = []Resource{{
		Name: "testNs",
		Manifest: map[string]interface{}{
			"kind":     "Namespace", // missing apiVersion
			"metadata": map[string]interface{}{"name": "test"},
		},
		Discovery: &DiscoveryConfig{Namespace: "*", ByName: "test"},
	}}

	v := newTaskValidator(cfg)
	_ = v.ValidateStructure()
	err := v.ValidateSemantic()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestBuiltinVariables(t *testing.T) {
	// Test that builtin variables (like adapter.name) are recognized
	cfg := baseTaskConfig()
	cfg.Resources = []Resource{{
		Name: "testNs",
		Manifest: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name":   "ns-{{ .adapter.name }}",
				"labels": map[string]interface{}{"adapter": "{{ .adapter.name }}"},
			},
		},
		Discovery: &DiscoveryConfig{Namespace: "*", ByName: "ns-{{ .adapter.name }}"},
	}}
	v := newTaskValidator(cfg)
	require.NoError(t, v.ValidateStructure())
	require.NoError(t, v.ValidateSemantic())
}

func TestPayloadValidate(t *testing.T) {
	tests := []struct {
		payload   Payload
		name      string
		errorMsg  string
		wantError bool
	}{
		{
			name: "valid payload with Build only",
			payload: Payload{
				Name:  "test",
				Build: map[string]interface{}{"status": "ready"},
			},
			wantError: false,
		},
		{
			name: "valid payload with BuildRef only",
			payload: Payload{
				Name:     "test",
				BuildRef: "templates/payload.yaml",
			},
			wantError: false,
		},
		{
			name: "invalid - both Build and BuildRef set",
			payload: Payload{
				Name:     "test",
				Build:    map[string]interface{}{"status": "ready"},
				BuildRef: "templates/payload.yaml",
			},
			wantError: true,
			errorMsg:  "mutually exclusive",
		},
		{
			name: "invalid - neither Build nor BuildRef set",
			payload: Payload{
				Name: "test",
			},
			wantError: true,
			errorMsg:  "must have either 'build' or 'build_ref' set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateStruct(&tt.payload)
			if tt.wantError {
				require.NotNil(t, errs)
				require.True(t, errs.HasErrors())
				assert.Contains(t, errs.Error(), tt.errorMsg)
			} else if errs != nil {
				assert.False(t, errs.HasErrors(), "unexpected error: %v", errs)
			}
		})
	}
}

func TestValidateCaptureFields(t *testing.T) {
	// Helper to create config with capture fields on a precondition (legacy path)
	withCapture := func(captures []CaptureField) *AdapterTaskConfig {
		cfg := baseTaskConfig()
		cfg.Preconditions = []Precondition{{
			ActionBase: ActionBase{Name: "getStatus"},
			Expression: "true",
			Capture:    captures,
		}}
		return cfg
	}

	t.Run("valid capture with field only", func(t *testing.T) {
		cfg := withCapture([]CaptureField{
			{Name: "clusterName", FieldExpressionDef: FieldExpressionDef{Field: "name"}},
			{
				Name:               "clusterPhase",
				FieldExpressionDef: FieldExpressionDef{Field: "status.phase"},
			},
		})
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})

	t.Run("valid capture with expression only", func(t *testing.T) {
		cfg := withCapture([]CaptureField{{
			Name:               "activeCount",
			FieldExpressionDef: FieldExpressionDef{Expression: "1 + 1"},
		}})
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})

	t.Run("invalid - both field and expression set", func(t *testing.T) {
		cfg := withCapture([]CaptureField{{
			Name:               "conflicting",
			FieldExpressionDef: FieldExpressionDef{Field: "name", Expression: "1 + 1"},
		}})
		err := newTaskValidator(cfg).ValidateStructure()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mutually exclusive")
	})

	t.Run("invalid - neither field nor expression set", func(t *testing.T) {
		cfg := withCapture([]CaptureField{{Name: "empty"}})
		err := newTaskValidator(cfg).ValidateStructure()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have either")
	})

	t.Run("invalid - capture name missing", func(t *testing.T) {
		cfg := withCapture([]CaptureField{{FieldExpressionDef: FieldExpressionDef{Field: "name"}}})
		err := newTaskValidator(cfg).ValidateStructure()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})
}

func TestYamlFieldName(t *testing.T) {
	// Ensure validator is initialized (populates fieldNameCache)
	getStructValidator()

	tests := []struct {
		goFieldName  string
		expectedYaml string
	}{
		{"ByName", "by_name"},
		{"BySelectors", "by_selectors"},
		{"Field", "field"},
		{"Expression", "expression"},
		{"APIVersion", "api_version"},
		{"Name", "name"},
		{"Namespace", "namespace"},
		{"LabelSelector", "label_selector"},
	}

	for _, tt := range tests {
		t.Run(tt.goFieldName, func(t *testing.T) {
			result := yamlFieldName(tt.goFieldName)
			assert.Equal(t, tt.expectedYaml, result)
		})
	}
}

func TestFieldNameCachePopulated(t *testing.T) {
	// Ensure validator is initialized
	getStructValidator()

	// Verify key fields are in the cache
	expectedFields := []string{
		"ByName", "BySelectors", "Field", "Expression",
		"Name", "Namespace", "APIVersion",
	}

	for _, field := range expectedFields {
		t.Run(field, func(t *testing.T) {
			_, ok := fieldNameCache[field]
			assert.True(t, ok, "field %s should be in cache", field)
		})
	}
}

// =============================================================================
// Transport Config Validation Tests
// =============================================================================

func TestValidateTransportConfig(t *testing.T) {
	t.Run("valid kubernetes transport", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Resources = []Resource{{
			Name: "testNs",
			Transport: &TransportConfig{
				Client: TransportClientKubernetes,
			},
			Manifest: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata":   map[string]interface{}{"name": "test"},
			},
			Discovery: &DiscoveryConfig{Namespace: "*", ByName: "test"},
		}}
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})

	t.Run("valid maestro transport with inline manifest (ManifestWork)", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Resources = []Resource{{
			Name: "testMW",
			Transport: &TransportConfig{
				Client: TransportClientMaestro,
				Maestro: &MaestroTransportConfig{
					TargetCluster: "cluster1",
				},
			},
			Manifest: map[string]interface{}{
				"apiVersion": "work.open-cluster-management.io/v1",
				"kind":       "ManifestWork",
				"metadata":   map[string]interface{}{"name": "test-mw"},
			},
			Discovery: &DiscoveryConfig{
				BySelectors: &SelectorConfig{
					LabelSelector: map[string]string{"app": "test"},
				},
			},
		}}
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})

	t.Run("unsupported transport client", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Resources = []Resource{{
			Name: "testNs",
			Transport: &TransportConfig{
				Client: "unsupported",
			},
			Manifest: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata":   map[string]interface{}{"name": "test"},
			},
			Discovery: &DiscoveryConfig{ByName: "test"},
		}}
		v := newTaskValidator(cfg)
		// Structure validation catches invalid oneof
		err := v.ValidateStructure()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid")
	})

	t.Run("maestro transport missing maestro config", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Resources = []Resource{{
			Name: "testMW",
			Transport: &TransportConfig{
				Client: TransportClientMaestro,
				// Missing Maestro config
			},
			Discovery: &DiscoveryConfig{
				BySelectors: &SelectorConfig{
					LabelSelector: map[string]string{"app": "test"},
				},
			},
		}}
		v := newTaskValidator(cfg)
		_ = v.ValidateStructure()
		err := v.ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "maestro transport config is required")
	})

	t.Run("maestro transport missing targetCluster", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Resources = []Resource{{
			Name: "testMW",
			Transport: &TransportConfig{
				Client:  TransportClientMaestro,
				Maestro: &MaestroTransportConfig{
					// Missing TargetCluster
				},
			},
			Manifest: map[string]interface{}{
				"apiVersion": "work.open-cluster-management.io/v1",
				"kind":       "ManifestWork",
			},
			Discovery: &DiscoveryConfig{
				BySelectors: &SelectorConfig{
					LabelSelector: map[string]string{"app": "test"},
				},
			},
		}}
		v := newTaskValidator(cfg)
		// targetCluster is structurally required
		err := v.ValidateStructure()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "target_cluster")
	})

	t.Run("maestro transport missing manifest", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Resources = []Resource{{
			Name: "testMW",
			Transport: &TransportConfig{
				Client: TransportClientMaestro,
				Maestro: &MaestroTransportConfig{
					TargetCluster: "cluster1",
				},
			},
			// No Manifest
			Discovery: &DiscoveryConfig{
				BySelectors: &SelectorConfig{
					LabelSelector: map[string]string{"app": "test"},
				},
			},
		}}
		v := newTaskValidator(cfg)
		_ = v.ValidateStructure()
		err := v.ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "manifest is required for maestro transport")
	})

	t.Run("kubernetes transport missing manifest", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Resources = []Resource{{
			Name: "testNs",
			Transport: &TransportConfig{
				Client: TransportClientKubernetes,
			},
			// Missing Manifest
			Discovery: &DiscoveryConfig{ByName: "test"},
		}}
		v := newTaskValidator(cfg)
		_ = v.ValidateStructure()
		err := v.ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "manifest is required for kubernetes transport")
	})

	t.Run("no transport defaults to kubernetes - manifest required", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Resources = []Resource{{
			Name: "testNs",
			// No Transport (defaults to kubernetes)
			// No Manifest
			Discovery: &DiscoveryConfig{ByName: "test"},
		}}
		v := newTaskValidator(cfg)
		_ = v.ValidateStructure()
		err := v.ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "manifest is required for kubernetes transport")
	})

	t.Run("maestro transport with template variable in targetCluster", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Params = []Parameter{{Name: "clusterName", Source: StringSource("event.name")}}
		cfg.Resources = []Resource{{
			Name: "testMW",
			Transport: &TransportConfig{
				Client: TransportClientMaestro,
				Maestro: &MaestroTransportConfig{
					TargetCluster: "{{ .clusterName }}",
				},
			},
			Manifest: map[string]interface{}{
				"apiVersion": "work.open-cluster-management.io/v1",
				"kind":       "ManifestWork",
				"metadata":   map[string]interface{}{"name": "test"},
			},
			Discovery: &DiscoveryConfig{
				BySelectors: &SelectorConfig{
					LabelSelector: map[string]string{"app": "test"},
				},
			},
		}}
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})

	t.Run("maestro transport with undefined template variable in targetCluster", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Resources = []Resource{{
			Name: "testMW",
			Transport: &TransportConfig{
				Client: TransportClientMaestro,
				Maestro: &MaestroTransportConfig{
					TargetCluster: "{{ .undefinedVar }}",
				},
			},
			Manifest: map[string]interface{}{
				"apiVersion": "work.open-cluster-management.io/v1",
				"kind":       "ManifestWork",
				"metadata":   map[string]interface{}{"name": "test"},
			},
			Discovery: &DiscoveryConfig{
				BySelectors: &SelectorConfig{
					LabelSelector: map[string]string{"app": "test"},
				},
			},
		}}
		v := newTaskValidator(cfg)
		_ = v.ValidateStructure()
		err := v.ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "undefined template variable \"undefinedVar\"")
	})

	t.Run("maestro transport skips K8s manifest validation", func(t *testing.T) {
		// Maestro resources use manifest for ManifestWork content
		// should skip K8s apiVersion/kind validation
		cfg := baseTaskConfig()
		cfg.Resources = []Resource{{
			Name: "testMW",
			Transport: &TransportConfig{
				Client: TransportClientMaestro,
				Maestro: &MaestroTransportConfig{
					TargetCluster: "cluster1",
				},
			},
			Manifest: map[string]interface{}{
				"apiVersion": "work.open-cluster-management.io/v1",
				"kind":       "ManifestWork",
			},
			Discovery: &DiscoveryConfig{
				BySelectors: &SelectorConfig{
					LabelSelector: map[string]string{"app": "test"},
				},
			},
		}}
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})
}

func TestValidateFileReferencesManifestRef(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test manifest file (ManifestWork content)
	manifestDir := filepath.Join(tmpDir, "templates")
	require.NoError(t, os.MkdirAll(manifestDir, 0o755))
	manifestFile := filepath.Join(manifestDir, "manifestwork.yaml")
	manifestContent := []byte("apiVersion: work.open-cluster-management.io/v1\nkind: ManifestWork")
	require.NoError(t, os.WriteFile(manifestFile, manifestContent, 0o644))

	tests := []struct {
		name    string
		config  *AdapterTaskConfig
		errMsg  string
		wantErr bool
	}{
		{
			name: "valid manifest ref for maestro transport",
			config: &AdapterTaskConfig{
				Resources: []Resource{{
					Name: "test",
					Transport: &TransportConfig{
						Client: TransportClientMaestro,
						Maestro: &MaestroTransportConfig{
							TargetCluster: "cluster1",
						},
					},
					Manifest: map[string]interface{}{
						"ref": "templates/manifestwork.yaml",
					},
					Discovery: &DiscoveryConfig{
						BySelectors: &SelectorConfig{
							LabelSelector: map[string]string{"app": "test"},
						},
					},
				}},
			},
			wantErr: false,
		},
		{
			name: "invalid manifest ref - file not found",
			config: &AdapterTaskConfig{
				Resources: []Resource{{
					Name: "test",
					Transport: &TransportConfig{
						Client: TransportClientMaestro,
						Maestro: &MaestroTransportConfig{
							TargetCluster: "cluster1",
						},
					},
					Manifest: map[string]interface{}{
						"ref": "templates/nonexistent.yaml",
					},
					Discovery: &DiscoveryConfig{
						BySelectors: &SelectorConfig{
							LabelSelector: map[string]string{"app": "test"},
						},
					},
				}},
			},
			wantErr: true,
			errMsg:  "does not exist",
		},
		{
			name: "inline manifest - no file reference validation needed",
			config: &AdapterTaskConfig{
				Resources: []Resource{{
					Name: "test",
					Transport: &TransportConfig{
						Client: TransportClientMaestro,
						Maestro: &MaestroTransportConfig{
							TargetCluster: "cluster1",
						},
					},
					Manifest: map[string]interface{}{
						"apiVersion": "work.open-cluster-management.io/v1",
						"kind":       "ManifestWork",
					},
					Discovery: &DiscoveryConfig{
						BySelectors: &SelectorConfig{
							LabelSelector: map[string]string{"app": "test"},
						},
					},
				}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewTaskConfigValidator(tt.config, tmpDir)
			err := validator.ValidateFileReferences()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateLifecycleConfig(t *testing.T) {
	// minResource satisfies required fields (manifest, discovery) so we can focus on lifecycle validation.
	minDiscovery := &DiscoveryConfig{ByName: "my-resource"}
	minManifest := map[string]interface{}{"apiVersion": "v1", "kind": "ConfigMap"}

	withLifecycle := func(del *LifecycleDelete) *AdapterTaskConfig {
		cfg := baseTaskConfig()
		cfg.Resources = []Resource{{
			Name:      "myResource",
			Discovery: minDiscovery,
			Manifest:  minManifest,
			Lifecycle: &ResourceLifecycle{Delete: del},
		}}
		return cfg
	}

	t.Run("valid lifecycle delete with when expression", func(t *testing.T) {
		cfg := withLifecycle(&LifecycleDelete{
			When: &LifecycleWhen{Expression: "deletedTime != null"},
		})
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})

	t.Run("lifecycle delete missing when block", func(t *testing.T) {
		cfg := withLifecycle(&LifecycleDelete{
			PropagationPolicy: "Background",
		})
		err := newTaskValidator(cfg).ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "lifecycle.delete.when.expression is required")
	})

	t.Run("lifecycle delete with empty when expression", func(t *testing.T) {
		cfg := withLifecycle(&LifecycleDelete{
			When: &LifecycleWhen{Expression: ""},
		})
		err := newTaskValidator(cfg).ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "lifecycle.delete.when.expression is required")
	})

	t.Run("lifecycle delete with invalid propagationPolicy", func(t *testing.T) {
		cfg := withLifecycle(&LifecycleDelete{
			PropagationPolicy: "Invalid",
			When:              &LifecycleWhen{Expression: "deletedTime != null"},
		})
		err := newTaskValidator(cfg).ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid propagationPolicy")
	})

	t.Run("lifecycle delete with invalid CEL expression", func(t *testing.T) {
		cfg := withLifecycle(&LifecycleDelete{
			When: &LifecycleWhen{Expression: "deletedTime != null &&"},
		})
		err := newTaskValidator(cfg).ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "CEL parse error")
	})

	t.Run("lifecycle delete missing discovery", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Resources = []Resource{{
			Name:     "myResource",
			Manifest: minManifest,
			// Discovery intentionally absent
			Lifecycle: &ResourceLifecycle{Delete: &LifecycleDelete{
				When: &LifecycleWhen{Expression: "is_deleting"},
			}},
		}}
		err := newTaskValidator(cfg).ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "lifecycle.delete requires a discovery config")
	})

	t.Run("no lifecycle config is valid", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Resources = []Resource{{Name: "myResource", Discovery: minDiscovery, Manifest: minManifest}}
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})
}

func TestValidateParamsAPICallSource(t *testing.T) {
	t.Run("api_call source passes validation", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Params = []Parameter{
			{Name: "clusterId", Source: StringSource("event.id")},
			{Name: "clusterData", Source: APICallSource(&APICall{Method: "GET", URL: "/clusters/{{ .clusterId }}"})},
		}
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})

	t.Run("api_call source URL can reference param defined before it", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Params = []Parameter{
			{Name: "clusterId", Source: StringSource("event.id")},
			{Name: "clusterData", Source: APICallSource(&APICall{Method: "GET", URL: "/clusters/{{ .clusterId }}"})},
			{Name: "nodePoolData", Source: APICallSource(&APICall{Method: "GET", URL: "/clusters/{{ .clusterId }}/nodepools"})},
		}
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})

	t.Run("api_call source URL cannot reference param defined after it", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Params = []Parameter{
			// nodePoolName is defined AFTER nodePoolData uses it (order violation)
			{Name: "nodePoolData", Source: APICallSource(&APICall{Method: "GET", URL: "/nodepools/{{ .nodePoolName }}"})},
			{Name: "nodePoolName", Source: StringSource("event.nodePool")},
		}
		v := newTaskValidator(cfg)
		_ = v.ValidateStructure()
		err := v.ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "undefined template variable \"nodePoolName\"")
	})

	t.Run("expression source passes CEL validation", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Params = []Parameter{
			{Name: "clusterId", Source: StringSource("event.id")},
			{Name: "clusterData", Source: APICallSource(&APICall{Method: "GET", URL: "/clusters/{{ .clusterId }}"})},
			{Name: "isActive", Source: ExpressionSource(`clusterData.status == "Active"`)},
		}
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
	})

	t.Run("expression source with invalid CEL fails validation", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Params = []Parameter{
			{Name: "bad", Source: ExpressionSource(`====invalid`)},
		}
		v := newTaskValidator(cfg)
		_ = v.ValidateStructure()
		err := v.ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "CEL parse error")
	})

	t.Run("empty string source fails validation", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Params = []Parameter{
			{Name: "bad", Source: StringSource("")},
		}
		v := newTaskValidator(cfg)
		_ = v.ValidateStructure()
		err := v.ValidateSemantic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "source is required")
	})
}

func TestValidatePreconditionAPICallForbidden(t *testing.T) {
	t.Run("precondition with api_call produces deprecation warning not error", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Preconditions = []Precondition{{
			ActionBase: ActionBase{
				Name:    "fetchCluster",
				APICall: &APICall{Method: "GET", URL: "/clusters/x"},
			},
		}}
		v := newTaskValidator(cfg)
		_ = v.ValidateStructure()
		err := v.ValidateSemantic()
		require.NoError(t, err)
		warnings := v.Warnings()
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "DEPRECATED")
		assert.Contains(t, warnings[0], "fetchCluster")
		assert.Contains(t, warnings[0], "source.api_call")
	})

	t.Run("precondition without api_call passes with no warnings", func(t *testing.T) {
		cfg := baseTaskConfig()
		cfg.Preconditions = []Precondition{{
			ActionBase: ActionBase{Name: "check"},
			Expression: "true",
		}}
		v := newTaskValidator(cfg)
		require.NoError(t, v.ValidateStructure())
		require.NoError(t, v.ValidateSemantic())
		assert.Empty(t, v.Warnings())
	})
}

func TestAdapterConfigValidator_HyperfleetAuth(t *testing.T) {
	baseAdapterConfig := func() *AdapterConfig {
		return &AdapterConfig{
			Adapter: AdapterInfo{Name: "test-adapter"},
		}
	}
	newValidator := func(cfg *AdapterConfig) *AdapterConfigValidator {
		return NewAdapterConfigValidator(cfg, "")
	}

	t.Run("nil auth is valid", func(t *testing.T) {
		cfg := baseAdapterConfig()
		require.NoError(t, newValidator(cfg).ValidateStructure())
	})

	t.Run("valid absolute token path", func(t *testing.T) {
		cfg := baseAdapterConfig()
		cfg.Clients.HyperfleetAPI.Auth = &HyperfleetAPIAuthConfig{
			TokenPath:     "/var/run/secrets/token",
			TokenCacheTTL: 30,
		}
		require.NoError(t, newValidator(cfg).ValidateStructure())
	})

	t.Run("empty token_path is an error", func(t *testing.T) {
		cfg := baseAdapterConfig()
		cfg.Clients.HyperfleetAPI.Auth = &HyperfleetAPIAuthConfig{TokenPath: ""}
		err := newValidator(cfg).ValidateStructure()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "token_path must be set")
	})

	t.Run("relative token_path is an error", func(t *testing.T) {
		cfg := baseAdapterConfig()
		cfg.Clients.HyperfleetAPI.Auth = &HyperfleetAPIAuthConfig{TokenPath: "relative/path/token"}
		err := newValidator(cfg).ValidateStructure()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be an absolute path")
	})

	t.Run("negative token_cache_ttl is an error", func(t *testing.T) {
		cfg := baseAdapterConfig()
		cfg.Clients.HyperfleetAPI.Auth = &HyperfleetAPIAuthConfig{
			TokenPath:     "/var/run/secrets/token",
			TokenCacheTTL: -1,
		}
		err := newValidator(cfg).ValidateStructure()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "token_cache_ttl must not be negative")
	})
}
