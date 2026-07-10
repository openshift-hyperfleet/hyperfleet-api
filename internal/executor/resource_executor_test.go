package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/configloader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/k8sclient"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/manifest"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/transportclient"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// testDeletedTime is a non-null deleted_time value used in lifecycle delete tests to trigger when-expressions.
const testDeletedTime = "2026-01-01T00:00:00Z"

// TestResourceExecutor_ExecuteAll_DiscoveryFailure verifies that when discovery fails after a successful apply,
// the error is logged and notified: ExecuteAll returns an error, result is failed,
// and execCtx.Adapter.ExecutionError is set.
func TestResourceExecutor_ExecuteAll_DiscoveryFailure(t *testing.T) {
	discoveryErr := errors.New("discovery failed: resource not found")
	// This resource has no lifecycle.delete, so preDiscoverAll is skipped.
	// The only GetResource call is post-apply discovery, which returns the transient error.
	mock := k8sclient.NewMockK8sClient()
	mock.GetResourceError = discoveryErr
	// Apply succeeds so we reach post-apply discovery.
	mock.ApplyResourceResult = &transportclient.ApplyResult{
		Operation: manifest.OperationCreate,
		Reason:    "mock",
	}

	config := &ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	}
	re := newResourceExecutor(config)

	resource := configloader.Resource{
		Name:      "test-resource",
		Transport: &configloader.TransportConfig{Client: "kubernetes"},
		Manifest: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm",
				"namespace": "default",
			},
		},
		Discovery: &configloader.DiscoveryConfig{
			Namespace: "default",
			ByName:    "test-cm",
		},
	}
	resources := []configloader.Resource{resource}
	execCtx := NewExecutionContext(context.Background(), map[string]interface{}{}, nil)

	results, err := re.ExecuteAll(context.Background(), resources, execCtx)

	require.Error(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, StatusFailed, results[0].Status, "result status should be failed")
	require.NotNil(t, results[0].Error)
	assert.Contains(t, results[0].Error.Error(), "discovery failed", "result error should describe discovery failure")
	require.NotNil(t, execCtx.Adapter.ExecutionError, "ExecutionError should be set for notification")
	assert.Equal(t, string(PhaseResources), execCtx.Adapter.ExecutionError.Phase)
	assert.Equal(t, resource.Name, execCtx.Adapter.ExecutionError.Step)
	assert.Contains(t, execCtx.Adapter.ExecutionError.Message, "discovery failed")
}

func TestResourceExecutor_ExecuteAll_StoresNestedDiscoveriesByName(t *testing.T) {
	mock := k8sclient.NewMockK8sClient()
	mock.ApplyResourceResult = &transportclient.ApplyResult{
		Operation: manifest.OperationCreate,
		Reason:    "mock",
	}
	mock.GetResourceResult = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      "cluster-1-adapter2",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": []interface{}{
						map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "ConfigMap",
							"metadata": map[string]interface{}{
								"name":      "cluster-1-adapter2-configmap",
								"namespace": "default",
							},
							"data": map[string]interface{}{
								"cluster_id": "cluster-1",
							},
						},
					},
				},
			},
			"status": map[string]interface{}{
				"resourceStatus": map[string]interface{}{
					"manifests": []interface{}{
						map[string]interface{}{
							"resourceMeta": map[string]interface{}{
								"name":      "cluster-1-adapter2-configmap",
								"namespace": "default",
								"resource":  "configmaps",
								"group":     "",
							},
							"statusFeedback": map[string]interface{}{
								"values": []interface{}{
									map[string]interface{}{
										"name": "data",
										"fieldValue": map[string]interface{}{
											"type":    "JsonRaw",
											"jsonRaw": "{\"cluster_id\":\"cluster-1\"}",
										},
									},
								},
							},
							"conditions": []interface{}{
								map[string]interface{}{
									"type":   "Applied",
									"status": "True",
								},
							},
						},
					},
				},
			},
		},
	}

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	resource := configloader.Resource{
		Name: "resource0",
		Transport: &configloader.TransportConfig{
			Client: "kubernetes",
		},
		Manifest: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      "cluster-1-adapter2",
				"namespace": "default",
			},
		},
		Discovery: &configloader.DiscoveryConfig{
			Namespace: "default",
			ByName:    "cluster-1-adapter2",
		},
		NestedDiscoveries: []configloader.NestedDiscovery{
			{
				Name: "configmap0",
				Discovery: &configloader.DiscoveryConfig{
					Namespace: "default",
					ByName:    "cluster-1-adapter2-configmap",
				},
			},
		},
	}

	execCtx := NewExecutionContext(context.Background(), map[string]interface{}{}, nil)
	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resource}, execCtx)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, StatusSuccess, results[0].Status)

	parent, ok := execCtx.Resources["resource0"].(*unstructured.Unstructured)
	require.True(t, ok, "resource0 should store the discovered parent resource")
	assert.Equal(t, "ManifestWork", parent.GetKind())
	assert.Equal(t, "cluster-1-adapter2", parent.GetName())

	nested, ok := execCtx.Resources["configmap0"].(*unstructured.Unstructured)
	require.True(t, ok, "configmap0 should be stored as top-level nested discovery")
	assert.Equal(t, "ConfigMap", nested.GetKind())
	assert.Equal(t, "cluster-1-adapter2-configmap", nested.GetName())

	// Verify statusFeedback and conditions were enriched from parent's status.resourceStatus
	_, hasSF := nested.Object["statusFeedback"]
	assert.True(t, hasSF, "configmap0 should have statusFeedback merged from parent")
	_, hasConds := nested.Object["conditions"]
	assert.True(t, hasConds, "configmap0 should have conditions merged from parent")

	sf := nested.Object["statusFeedback"].(map[string]interface{})
	values := sf["values"].([]interface{})
	assert.Len(t, values, 1)
	v0 := values[0].(map[string]interface{})
	assert.Equal(t, "data", v0["name"])
}

func TestRenderToBytes_StringManifest(t *testing.T) {
	re := newResourceExecutor(&ExecutorConfig{
		Logger: logger.NewTestLogger(),
	})

	tests := []struct {
		name         string
		manifest     string
		params       map[string]interface{}
		wantContains []string
		wantErr      bool
	}{
		{
			name: "simple string manifest with template values",
			manifest: `apiVersion: v1
kind: ConfigMap
metadata:
  name: "{{ .name }}"
  namespace: "{{ .namespace }}"
data:
  key: value`,
			params: map[string]interface{}{
				"name":      "my-config",
				"namespace": "default",
			},
			wantContains: []string{`"name":"my-config"`, `"namespace":"default"`},
		},
		{
			name: "structural if template",
			manifest: `apiVersion: v1
kind: ConfigMap
metadata:
  name: "test"
{{ if .addLabels }}
  labels:
    app: "myapp"
{{ end }}
data:
  key: value`,
			params: map[string]interface{}{
				"addLabels": true,
			},
			wantContains: []string{`"labels"`, `"app":"myapp"`},
		},
		{
			name: "structural if template - false branch",
			manifest: `apiVersion: v1
kind: ConfigMap
metadata:
  name: "test"
{{ if .addLabels }}
  labels:
    app: "myapp"
{{ end }}
data:
  key: value`,
			params: map[string]interface{}{
				"addLabels": false,
			},
			wantContains: []string{`"name":"test"`, `"key":"value"`},
		},
		{
			name: "range template for list generation",
			manifest: `apiVersion: v1
kind: ConfigMap
metadata:
  name: "test"
data:
{{ range $k, $v := .items }}
  {{ $k }}: "{{ $v }}"
{{ end }}`,
			params: map[string]interface{}{
				"items": map[string]interface{}{
					"key1": "val1",
					"key2": "val2",
				},
			},
			wantContains: []string{`"key1":"val1"`, `"key2":"val2"`},
		},
		{
			name: "if-else template for conditional properties",
			manifest: `apiVersion: v1
kind: ConfigMap
metadata:
  name: "test"
  labels:
{{ if .isGood }}
    status: "good"
{{ else }}
    status: "bad"
{{ end }}`,
			params: map[string]interface{}{
				"isGood": true,
			},
			wantContains: []string{`"status":"good"`},
		},
		{
			name:     "invalid template syntax",
			manifest: `apiVersion: v1{{ if }}`,
			params:   map[string]interface{}{},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := configloader.Resource{
				Name:     "test",
				Manifest: tt.manifest,
			}
			execCtx := NewExecutionContext(context.Background(), nil, nil)
			execCtx.Params = tt.params

			data, err := re.renderToBytes(resource, execCtx)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			for _, want := range tt.wantContains {
				assert.Contains(t, string(data), want)
			}
		})
	}
}

func TestRenderToBytes_StringManifestWithSubnetList(t *testing.T) {
	// Test the customer's original use case: generating a list of subnets
	re := newResourceExecutor(&ExecutorConfig{
		Logger: logger.NewTestLogger(),
	})

	manifest := `apiVersion: v1
kind: ConfigMap
metadata:
  name: "subnet-config"
data:
  subnets: |
{{ range .subnetIds }}
    - id: {{ . }}
{{ end }}`

	params := map[string]interface{}{
		"subnetIds": []interface{}{"sub1", "sub2", "sub3"},
	}

	resource := configloader.Resource{
		Name:     "subnets",
		Manifest: manifest,
	}
	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Params = params

	data, err := re.renderToBytes(resource, execCtx)
	require.NoError(t, err)
	assert.Contains(t, string(data), "sub1")
	assert.Contains(t, string(data), "sub2")
	assert.Contains(t, string(data), "sub3")
}

func TestRenderToBytes_StringManifestEdgeCases(t *testing.T) {
	re := newResourceExecutor(&ExecutorConfig{
		Logger: logger.NewTestLogger(),
	})

	t.Run("plain YAML string without templates", func(t *testing.T) {
		// Backward compatibility: plain YAML ref files (no templates) still work
		manifest := `apiVersion: v1
kind: ConfigMap
metadata:
  name: "static-config"
data:
  key: value`
		resource := configloader.Resource{
			Name:     "test",
			Manifest: manifest,
		}
		execCtx := NewExecutionContext(context.Background(), nil, nil)
		execCtx.Params = map[string]interface{}{}

		data, err := re.renderToBytes(resource, execCtx)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"name":"static-config"`)
		assert.Contains(t, string(data), `"key":"value"`)
	})

	t.Run("empty string manifest", func(t *testing.T) {
		resource := configloader.Resource{
			Name:     "test",
			Manifest: "",
		}
		execCtx := NewExecutionContext(context.Background(), nil, nil)
		execCtx.Params = map[string]interface{}{}

		_, err := re.renderToBytes(resource, execCtx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty manifest")
	})

	t.Run("template rendering produces invalid YAML", func(t *testing.T) {
		manifest := `{{ .content }}`
		resource := configloader.Resource{
			Name:     "test",
			Manifest: manifest,
		}
		execCtx := NewExecutionContext(context.Background(), nil, nil)
		execCtx.Params = map[string]interface{}{
			"content": "not: valid: yaml: [broken",
		}

		_, err := re.renderToBytes(resource, execCtx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse rendered manifest as YAML")
	})

	t.Run("missing template variable errors", func(t *testing.T) {
		manifest := `apiVersion: v1
kind: ConfigMap
metadata:
  name: "{{ .missingVar }}"`
		resource := configloader.Resource{
			Name:     "test",
			Manifest: manifest,
		}
		execCtx := NewExecutionContext(context.Background(), nil, nil)
		execCtx.Params = map[string]interface{}{} // missingVar not provided

		_, err := re.renderToBytes(resource, execCtx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missingVar")
	})

	t.Run("nil manifest", func(t *testing.T) {
		resource := configloader.Resource{
			Name:     "test",
			Manifest: nil,
		}
		execCtx := NewExecutionContext(context.Background(), nil, nil)

		_, err := re.renderToBytes(resource, execCtx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no manifest specified")
	})

	t.Run("map manifest still works (backward compatibility)", func(t *testing.T) {
		resource := configloader.Resource{
			Name: "test",
			Manifest: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "{{ .name }}",
					"namespace": "default",
				},
			},
		}
		execCtx := NewExecutionContext(context.Background(), nil, nil)
		execCtx.Params = map[string]interface{}{
			"name": "rendered-name",
		}

		data, err := re.renderToBytes(resource, execCtx)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"name":"rendered-name"`)
		assert.Contains(t, string(data), `"namespace":"default"`)
	})
}

func TestResourceExecutor_ExecuteAll_StringManifest(t *testing.T) {
	// End-to-end test: string manifest through the full executor flow
	mock := k8sclient.NewMockK8sClient()
	// Don't set ApplyResourceResult — use default behavior which parses and stores the resource
	mock.GetResourceResult = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-config",
				"namespace": "default",
			},
		},
	}

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	// Use a string manifest with structural Go templates
	manifestStr := `apiVersion: v1
kind: ConfigMap
metadata:
  name: "{{ .configName }}"
  namespace: "{{ .namespace }}"
{{ if .addLabels }}
  labels:
    managed-by: "adapter"
{{ end }}
data:
  cluster: "{{ .clusterId }}"`

	resource := configloader.Resource{
		Name:     "testConfig",
		Manifest: manifestStr,
		Discovery: &configloader.DiscoveryConfig{
			Namespace: "default",
			ByName:    "test-config",
		},
	}

	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Params = map[string]interface{}{
		"configName": "test-config",
		"namespace":  "default",
		"addLabels":  true,
		"clusterId":  "cluster-1",
	}

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resource}, execCtx)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, StatusSuccess, results[0].Status)
	assert.Equal(t, "ConfigMap", results[0].Kind)
	assert.Equal(t, "test-config", results[0].ResourceName)

	// Verify the mock stored the rendered resource correctly
	stored, ok := mock.Resources["default/test-config"]
	require.True(t, ok, "Resource should be stored in mock")
	assert.Equal(t, "ConfigMap", stored.GetKind())
	assert.Equal(t, "test-config", stored.GetName())

	// Verify labels were rendered (addLabels=true)
	labels := stored.GetLabels()
	assert.Equal(t, "adapter", labels["managed-by"])

	// Verify data was rendered
	data, found, _ := unstructured.NestedString(stored.Object, "data", "cluster")
	assert.True(t, found)
	assert.Equal(t, "cluster-1", data)
}

func TestResolveGVK_StringManifest(t *testing.T) {
	re := &ResourceExecutor{}

	tests := []struct {
		name        string
		manifest    interface{}
		wantGroup   string
		wantVersion string
		wantKind    string
		wantEmpty   bool
	}{
		{
			name: "map manifest",
			manifest: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
			},
			wantVersion: "v1",
			wantKind:    "ConfigMap",
		},
		{
			name:        "string manifest with Go templates",
			manifest:    "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: \"{{ .clusterId }}\"\n",
			wantVersion: "v1",
			wantKind:    "ConfigMap",
		},
		{
			name: "string manifest with structural Go template directives",
			manifest: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n" +
				"  name: \"test-{{ .clusterId }}\"\n  labels:\n    app: test\n" +
				"{{ if .testRunId }}\n    run-id: \"{{ .testRunId }}\"\n{{ end }}\n" +
				"data:\n  key: value\n",
			wantVersion: "v1",
			wantKind:    "ConfigMap",
		},
		{
			name:        "string manifest with apps/v1",
			manifest:    "apiVersion: apps/v1\nkind: Deployment\n",
			wantGroup:   "apps",
			wantVersion: "v1",
			wantKind:    "Deployment",
		},
		{
			name:      "nil manifest",
			manifest:  nil,
			wantEmpty: true,
		},
		{
			name:      "invalid string YAML",
			manifest:  "not: valid: yaml: {{{}",
			wantEmpty: true,
		},
		{
			name:      "string manifest missing kind",
			manifest:  "apiVersion: v1\nmetadata:\n  name: test\n",
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := configloader.Resource{
				Manifest: tt.manifest,
			}
			gvk := re.resolveGVK(resource)

			if tt.wantEmpty {
				assert.True(t, gvk.Empty(), "expected empty GVK")
			} else {
				assert.Equal(t, tt.wantGroup, gvk.Group)
				assert.Equal(t, tt.wantVersion, gvk.Version)
				assert.Equal(t, tt.wantKind, gvk.Kind)
			}
		})
	}
}

// newResourceWithLifecycle is a helper that builds a Resource with lifecycle.delete config.
func newResourceWithLifecycle(expression, propagationPolicy string) configloader.Resource {
	r := configloader.Resource{
		Name:      "test-resource",
		Transport: &configloader.TransportConfig{Client: "kubernetes"},
		Manifest: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm",
				"namespace": "default",
			},
		},
		Discovery: &configloader.DiscoveryConfig{
			Namespace: "default",
			ByName:    "test-cm",
		},
		Lifecycle: &configloader.ResourceLifecycle{
			Delete: &configloader.LifecycleDelete{
				PropagationPolicy: propagationPolicy,
			},
		},
	}
	if expression != "" {
		r.Lifecycle.Delete.When = &configloader.LifecycleWhen{Expression: expression}
	}
	return r
}

// newResourceWithLifecycleCreate is a helper that builds a Resource with lifecycle.create config.
func newResourceWithLifecycleCreate(expression string) configloader.Resource {
	r := configloader.Resource{
		Name:      "test-resource",
		Transport: &configloader.TransportConfig{Client: "kubernetes"},
		Manifest: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm",
				"namespace": "default",
			},
		},
		Discovery: &configloader.DiscoveryConfig{
			Namespace: "default",
			ByName:    "test-cm",
		},
		Lifecycle: &configloader.ResourceLifecycle{
			Create: &configloader.LifecycleCreate{
				When: &configloader.LifecycleWhen{Expression: expression},
			},
		},
	}
	return r
}

// sequencedGetResourceMock returns GetResource results/errors in order (one entry per call),
// repeating the last entry once exhausted. Used when pre-discovery and post-apply discovery
// need different GetResource outcomes (e.g. NotFound, then the newly-applied resource).
type sequencedGetResourceMock struct {
	*k8sclient.MockK8sClient
	results     []*unstructured.Unstructured
	errs        []error
	callCount   int
	ApplyCalled bool
}

func (m *sequencedGetResourceMock) GetResource(
	ctx context.Context,
	gvk schema.GroupVersionKind,
	namespace, name string,
	target transportclient.TransportContext,
) (*unstructured.Unstructured, error) {
	idx := m.callCount
	if idx >= len(m.results) {
		idx = len(m.results) - 1
	}
	m.callCount++
	return m.results[idx], m.errs[idx]
}

func (m *sequencedGetResourceMock) ApplyResource(
	ctx context.Context,
	data []byte,
	opts *transportclient.ApplyOptions,
	target transportclient.TransportContext,
) (*transportclient.ApplyResult, error) {
	m.ApplyCalled = true
	return m.MockK8sClient.ApplyResource(ctx, data, opts, target)
}

func TestResourceExecutor_LifecycleCreate_WhenTrue_ResourceNotFound_Applied(t *testing.T) {
	// Resource doesn't exist yet, create.when evaluates true → applied normally.
	notFoundErr := apierrors.NewNotFound(schema.GroupResource{Resource: "configmaps"}, "test-cm")
	discovered := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]interface{}{"name": "test-cm", "namespace": "default"},
	}}
	mock := &sequencedGetResourceMock{
		MockK8sClient: k8sclient.NewMockK8sClient(),
		results:       []*unstructured.Unstructured{nil, discovered}, // pre-discovery: absent; post-apply: found
		errs:          []error{notFoundErr, nil},
	}
	mock.ApplyResourceResult = &transportclient.ApplyResult{
		Operation: manifest.OperationCreate,
		Reason:    "mock create",
	}

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	resource := newResourceWithLifecycleCreate("shouldCreate")
	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Params["shouldCreate"] = true

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resource}, execCtx)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, StatusSuccess, results[0].Status)
	assert.Equal(t, manifest.OperationCreate, results[0].Operation)
	assert.False(t, execCtx.Adapter.ResourcesSkipped, "resource was applied, not skipped")
}

func TestResourceExecutor_LifecycleCreate_WhenFalse_ResourceNotFound_Skipped(t *testing.T) {
	// Resource doesn't exist yet, create.when evaluates false → skipped, apply never called.
	mock := &trackingApplyMockClient{MockK8sClient: k8sclient.NewMockK8sClient()}
	mock.GetResourceError = apierrors.NewNotFound(schema.GroupResource{Resource: "configmaps"}, "test-cm")

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	resource := newResourceWithLifecycleCreate("shouldCreate")
	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Params["shouldCreate"] = false

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resource}, execCtx)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, StatusSkipped, results[0].Status)
	assert.Equal(t, manifest.OperationSkip, results[0].Operation)
	assert.False(t, mock.ApplyCalled, "ApplyResource should not be called when create.when is false")

	// Verifies the fix: skipping a resource must be reflected in adapter metadata so
	// post-action `when` gates can observe it.
	assert.True(t, execCtx.Adapter.ResourcesSkipped, "adapter.resourcesSkipped must be set on skip")
	assert.NotEmpty(t, execCtx.Adapter.SkipReason)
}

func TestResourceExecutor_LifecycleCreate_WhenCELError_ExecutionFails(t *testing.T) {
	// Resource doesn't exist yet, create.when fails to evaluate → execution fails,
	// resource is neither applied nor silently skipped.
	mock := &trackingApplyMockClient{MockK8sClient: k8sclient.NewMockK8sClient()}
	mock.GetResourceError = apierrors.NewNotFound(schema.GroupResource{Resource: "configmaps"}, "test-cm")

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	// Invalid CEL syntax — evaluateLifecycleWhen will error.
	resource := newResourceWithLifecycleCreate("shouldCreate &&")
	execCtx := NewExecutionContext(context.Background(), nil, nil)

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resource}, execCtx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to evaluate")
	require.Len(t, results, 1)
	assert.Equal(t, StatusFailed, results[0].Status)
	assert.False(t, mock.ApplyCalled, "ApplyResource should not be called when create.when errors")
	assert.NotNil(t, execCtx.Adapter.ExecutionError)
}

func TestResourceExecutor_LifecycleCreate_ResourceAlreadyExists_IgnoresWhen(t *testing.T) {
	// Resource already exists (pre-discovered) → create.when is ignored, normal apply (update flow).
	discovered := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm",
				"namespace": "default",
			},
		},
	}
	mock := k8sclient.NewMockK8sClient()
	mock.GetResourceResult = discovered
	mock.ApplyResourceResult = &transportclient.ApplyResult{
		Operation: manifest.OperationUpdate,
		Reason:    "mock update",
	}

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	resource := newResourceWithLifecycleCreate("shouldCreate")
	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Params["shouldCreate"] = false

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resource}, execCtx)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, StatusSuccess, results[0].Status)
	assert.Equal(t, manifest.OperationUpdate, results[0].Operation,
		"existing resource must be applied, ignoring create.when=false")
	assert.False(t, execCtx.Adapter.ResourcesSkipped)
}

func TestResourceExecutor_LifecycleCreate_Absent_NormalApply(t *testing.T) {
	// No lifecycle.create configured at all → resource applied normally (regression guard).
	// No lifecycle configured means preDiscoverAll is skipped, so the only GetResource call
	// is post-apply discovery — it must succeed for the apply flow to complete.
	mock := k8sclient.NewMockK8sClient()
	mock.GetResourceResult = &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]interface{}{"name": "test-cm", "namespace": "default"},
	}}
	mock.ApplyResourceResult = &transportclient.ApplyResult{
		Operation: manifest.OperationCreate,
		Reason:    "mock create",
	}

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	resource := configloader.Resource{
		Name:      "test-resource",
		Transport: &configloader.TransportConfig{Client: "kubernetes"},
		Manifest: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm",
				"namespace": "default",
			},
		},
		Discovery: &configloader.DiscoveryConfig{
			Namespace: "default",
			ByName:    "test-cm",
		},
	}
	execCtx := NewExecutionContext(context.Background(), nil, nil)

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resource}, execCtx)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, StatusSuccess, results[0].Status)
	assert.Equal(t, manifest.OperationCreate, results[0].Operation)
	assert.False(t, execCtx.Adapter.ResourcesSkipped)
}

// trackingApplyMockClient is a thin wrapper around MockK8sClient to capture whether
// ApplyResource was called.
type trackingApplyMockClient struct {
	*k8sclient.MockK8sClient
	ApplyCalled bool
}

func (m *trackingApplyMockClient) ApplyResource(
	ctx context.Context,
	data []byte,
	opts *transportclient.ApplyOptions,
	target transportclient.TransportContext,
) (*transportclient.ApplyResult, error) {
	m.ApplyCalled = true
	return m.MockK8sClient.ApplyResource(ctx, data, opts, target)
}

// trackingMockClient is a thin wrapper around MockK8sClient to also capture DeleteResource calls.
type trackingMockClient struct {
	*k8sclient.MockK8sClient
	DeleteCalledWithPolicy string
	DeleteCalled           bool
}

func (m *trackingMockClient) DeleteResource(
	ctx context.Context,
	gvk schema.GroupVersionKind,
	namespace, name string,
	opts *transportclient.DeleteOptions,
	target transportclient.TransportContext,
) error {
	m.DeleteCalled = true
	if opts != nil {
		m.DeleteCalledWithPolicy = opts.PropagationPolicy
	}
	return m.MockK8sClient.DeleteResource(ctx, gvk, namespace, name, opts, target)
}

// firstCallResultMock returns (firstResult, firstErr) on the first GetResource call and
// (nil, laterErr) on all subsequent calls. It also tracks whether DeleteResource was invoked.
// Use this when preDiscoverAll and a later discovery step need different GetResource outcomes.
type firstCallResultMock struct {
	*k8sclient.MockK8sClient
	firstResult  *unstructured.Unstructured
	firstErr     error
	laterErr     error
	callCount    int
	DeleteCalled bool
}

func (m *firstCallResultMock) GetResource(
	ctx context.Context,
	gvk schema.GroupVersionKind,
	namespace, name string,
	target transportclient.TransportContext,
) (*unstructured.Unstructured, error) {
	m.callCount++
	if m.callCount == 1 {
		return m.firstResult, m.firstErr
	}
	return nil, m.laterErr
}

func (m *firstCallResultMock) DeleteResource(
	ctx context.Context,
	gvk schema.GroupVersionKind,
	namespace, name string,
	opts *transportclient.DeleteOptions,
	target transportclient.TransportContext,
) error {
	m.DeleteCalled = true
	return m.MockK8sClient.DeleteResource(ctx, gvk, namespace, name, opts, target)
}

func TestResourceExecutor_LifecycleDelete_WhenTrue_ResourceFound_InstantDelete(t *testing.T) {
	// when.expression is true + resource exists, post-delete rediscovery returns NotFound
	// (no finalizers — resource is instantly gone after delete API call).
	// Expected: nil stored → dependent resources can cascade in the same reconciliation.
	discovered := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm",
				"namespace": "default",
			},
		},
	}

	mock := &trackingMockClient{MockK8sClient: k8sclient.NewMockK8sClient()}
	// Store via Resources map so DeleteResource clears it and post-delete GetResource returns NotFound.
	mock.Resources["default/test-cm"] = discovered

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	resource := newResourceWithLifecycle("deleted_time != null", "Background")
	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Params["deleted_time"] = testDeletedTime

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resource}, execCtx)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, StatusSuccess, results[0].Status)
	assert.Equal(t, manifest.OperationDelete, results[0].Operation)
	assert.True(t, mock.DeleteCalled, "DeleteResource should have been called")
	assert.Equal(t, "Background", mock.DeleteCalledWithPolicy)

	// nil stored: resource is confirmed gone → dependent resources can cascade in this reconciliation.
	storedVal, exists := execCtx.Resources[resource.Name]
	assert.True(t, exists, "nil sentinel should be in execCtx.Resources")
	assert.Nil(t, storedVal, "nil stored when post-delete rediscovery returns NotFound")
}

func TestResourceExecutor_LifecycleDelete_WhenTrue_ResourceFound_WithFinalizers(t *testing.T) {
	// when.expression is true + resource exists, post-delete rediscovery still returns the resource
	// (finalizers are running, or Maestro deletion is async).
	// Expected: discovered object stored (non-nil) → dependent resources wait for next reconciliation.
	discovered := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm",
				"namespace": "default",
			},
		},
	}

	mock := &trackingMockClient{MockK8sClient: k8sclient.NewMockK8sClient()}
	// GetResourceResult persists through DeleteResource → simulates finalizers / async deletion.
	mock.GetResourceResult = discovered

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	resource := newResourceWithLifecycle("deleted_time != null", "Background")
	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Params["deleted_time"] = testDeletedTime

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resource}, execCtx)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, StatusSuccess, results[0].Status)
	assert.Equal(t, manifest.OperationDelete, results[0].Operation)
	assert.True(t, mock.DeleteCalled, "DeleteResource should have been called")

	// non-nil stored: resource still present (finalizers) → dependents wait for next reconciliation.
	storedVal, exists := execCtx.Resources[resource.Name]
	assert.True(t, exists, "resource should be in execCtx.Resources")
	assert.NotNil(t, storedVal, "non-nil stored when post-delete rediscovery still finds the resource")
}

func TestResourceExecutor_LifecycleDelete_WhenTrue_ResourceNotFound(t *testing.T) {
	// when.expression is true + resource not found → no-op, nil stored in execCtx
	gr := schema.GroupResource{Group: "", Resource: "configmaps"}
	notFoundErr := apierrors.NewNotFound(gr, "test-cm")

	mock := k8sclient.NewMockK8sClient()
	mock.GetResourceError = notFoundErr

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	resource := newResourceWithLifecycle("deleted_time != null", "Background")
	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Params["deleted_time"] = testDeletedTime

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resource}, execCtx)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, StatusSuccess, results[0].Status)
	assert.Equal(t, manifest.OperationDelete, results[0].Operation)
	assert.Contains(t, results[0].OperationReason, "already deleted")

	// nil stored so dependent resources' ordering conditions evaluate to true
	storedVal, exists := execCtx.Resources[resource.Name]
	assert.True(t, exists, "nil sentinel should be in execCtx.Resources")
	assert.Nil(t, storedVal, "stored value should be nil when resource not found")
}

func TestResourceExecutor_LifecycleDelete_WhenFalse_NormalApply(t *testing.T) {
	// when.expression is false → normal apply path, no delete called
	mock := k8sclient.NewMockK8sClient()
	mock.ApplyResourceResult = &transportclient.ApplyResult{
		Operation: manifest.OperationCreate,
		Reason:    "mock create",
	}

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	// deleted_time is null → expression "deleted_time != null" is false
	resource := newResourceWithLifecycle("deleted_time != null", "Background")

	// Set up GetResource result for post-apply discovery (resource uses ByName discovery → GetResource)
	mock.GetResourceResult = &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]interface{}{"name": "test-cm", "namespace": "default"},
	}}

	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Params["deleted_time"] = nil // deleted_time is null/missing

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resource}, execCtx)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, StatusSuccess, results[0].Status)
	assert.Equal(t, manifest.OperationCreate, results[0].Operation, "should use normal apply path")
}

func TestResourceExecutor_LifecycleDelete_NoLifecycle_NormalApply(t *testing.T) {
	// No lifecycle config → normal apply path (backward compatible)
	mock := k8sclient.NewMockK8sClient()
	mock.ApplyResourceResult = &transportclient.ApplyResult{
		Operation: manifest.OperationCreate,
		Reason:    "mock create",
	}

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	resource := configloader.Resource{
		Name:      "test-resource",
		Transport: &configloader.TransportConfig{Client: "kubernetes"},
		Manifest: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm",
				"namespace": "default",
			},
		},
		// No Lifecycle
	}
	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Params["deleted_time"] = testDeletedTime

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resource}, execCtx)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, manifest.OperationCreate, results[0].Operation, "no lifecycle → normal apply")
}

func TestResourceExecutor_LifecycleDelete_NoExpression_DefaultsFalse(t *testing.T) {
	// lifecycle.delete with no when.expression → defaults to false, resource is applied normally
	mock := &trackingMockClient{MockK8sClient: k8sclient.NewMockK8sClient()}

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	resource := newResourceWithLifecycle("", "Foreground")
	execCtx := NewExecutionContext(context.Background(), nil, nil)

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resource}, execCtx)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, StatusSuccess, results[0].Status)
	assert.False(t, mock.DeleteCalled, "DeleteResource should not be called when expression is absent")
}

func TestResourceExecutor_LifecycleDelete_OrderingViaResources_InstantDelete(t *testing.T) {
	// Two resources with ordered deletion (no finalizers).
	// clusterJob is deleted; post-delete rediscovery returns NotFound (instant delete).
	// clusterConfigMap sees resources.clusterJob == null in the SAME reconciliation → also deletes.
	jobResource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata":   map[string]interface{}{"name": "my-job", "namespace": "default"},
		},
	}
	configMapResource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]interface{}{"name": "my-cm", "namespace": "default"},
		},
	}

	mock := k8sclient.NewMockK8sClient()
	// Resources map: DeleteResource clears entries → post-delete GetResource returns NotFound.
	mock.Resources["default/my-job"] = jobResource
	mock.Resources["default/my-cm"] = configMapResource

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	clusterJob := configloader.Resource{
		Name:      "clusterJob",
		Transport: &configloader.TransportConfig{Client: "kubernetes"},
		Manifest: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata":   map[string]interface{}{"name": "my-job", "namespace": "default"},
		},
		Discovery: &configloader.DiscoveryConfig{Namespace: "default", ByName: "my-job"},
		Lifecycle: &configloader.ResourceLifecycle{
			Delete: &configloader.LifecycleDelete{
				PropagationPolicy: "Background",
				When:              &configloader.LifecycleWhen{Expression: "deleted_time != null"},
			},
		},
	}

	clusterConfigMap := configloader.Resource{
		Name:      "clusterConfigMap",
		Transport: &configloader.TransportConfig{Client: "kubernetes"},
		Manifest: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]interface{}{"name": "my-cm", "namespace": "default"},
		},
		Discovery: &configloader.DiscoveryConfig{Namespace: "default", ByName: "my-cm"},
		Lifecycle: &configloader.ResourceLifecycle{
			Delete: &configloader.LifecycleDelete{
				PropagationPolicy: "Background",
				When: &configloader.LifecycleWhen{
					Expression: "deleted_time != null && !resources.?clusterJob.hasValue()",
				},
			},
		},
	}

	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Params["deleted_time"] = testDeletedTime

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{clusterJob, clusterConfigMap}, execCtx)

	require.NoError(t, err)
	require.Len(t, results, 2)

	// clusterJob: deleted; post-delete NotFound → nil stored.
	assert.Equal(t, manifest.OperationDelete, results[0].Operation)
	assert.Equal(t, "clusterJob", results[0].Name)
	assert.Nil(t, execCtx.Resources["clusterJob"], "clusterJob nil after instant delete")

	// clusterConfigMap: cascades in the SAME reconciliation because clusterJob is already nil.
	assert.Equal(t, "clusterConfigMap", results[1].Name)
	assert.Equal(t, manifest.OperationDelete, results[1].Operation,
		"clusterConfigMap should cascade in same reconciliation when clusterJob instantly deleted")
}

func TestResourceExecutor_LifecycleDelete_OrderingViaResources_WithFinalizers(t *testing.T) {
	// Two resources with ordered deletion (clusterJob has finalizers / async deletion).
	// clusterJob is deleted; post-delete rediscovery still returns the resource (finalizers running).
	// clusterConfigMap sees resources.clusterJob != null → waits for next reconciliation.
	jobResource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata":   map[string]interface{}{"name": "my-job", "namespace": "default"},
		},
	}

	mock := k8sclient.NewMockK8sClient()
	// GetResourceResult persists through DeleteResource → simulates finalizers / async deletion.
	mock.GetResourceResult = jobResource
	mock.ApplyResourceResult = &transportclient.ApplyResult{Operation: manifest.OperationCreate, Reason: "mock"}

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	clusterJob := configloader.Resource{
		Name:      "clusterJob",
		Transport: &configloader.TransportConfig{Client: "kubernetes"},
		Manifest: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata":   map[string]interface{}{"name": "my-job", "namespace": "default"},
		},
		Discovery: &configloader.DiscoveryConfig{Namespace: "default", ByName: "my-job"},
		Lifecycle: &configloader.ResourceLifecycle{
			Delete: &configloader.LifecycleDelete{
				PropagationPolicy: "Background",
				When:              &configloader.LifecycleWhen{Expression: "deleted_time != null"},
			},
		},
	}

	clusterConfigMap := configloader.Resource{
		Name:      "clusterConfigMap",
		Transport: &configloader.TransportConfig{Client: "kubernetes"},
		Manifest: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]interface{}{"name": "my-cm", "namespace": "default"},
		},
		Discovery: &configloader.DiscoveryConfig{Namespace: "default", ByName: "my-cm"},
		Lifecycle: &configloader.ResourceLifecycle{
			Delete: &configloader.LifecycleDelete{
				PropagationPolicy: "Background",
				When: &configloader.LifecycleWhen{
					Expression: "deleted_time != null && !resources.?clusterJob.hasValue()",
				},
			},
		},
	}

	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Params["deleted_time"] = testDeletedTime

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{clusterJob, clusterConfigMap}, execCtx)

	require.NoError(t, err)
	require.Len(t, results, 2)

	// clusterJob: delete sent; post-delete still present (finalizers) → non-nil stored.
	assert.Equal(t, manifest.OperationDelete, results[0].Operation)
	assert.NotNil(t, execCtx.Resources["clusterJob"], "clusterJob non-nil while finalizers running")

	// clusterConfigMap: condition false (clusterJob still present) → waits for next reconciliation.
	assert.Equal(t, "clusterConfigMap", results[1].Name)
	assert.NotEqual(t, manifest.OperationDelete, results[1].Operation,
		"clusterConfigMap must wait while clusterJob finalizers are running")
}

func TestResourceExecutor_LifecycleDelete_OrderingSecondReconciliation(t *testing.T) {
	// Second reconciliation: clusterJob is now gone (NotFound)
	// clusterConfigMap sees resources.clusterJob == null → deletes
	// Job returns NotFound; ConfigMap returns a found resource
	configMapResource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]interface{}{"name": "my-cm", "namespace": "default"},
		},
	}

	// Use mock with configmap stored but job not present (job is gone → GetResource returns NotFound)
	mock2 := k8sclient.NewMockK8sClient()
	mock2.Resources["default/my-cm"] = configMapResource

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock2,
		Logger:          logger.NewTestLogger(),
	})

	clusterJob := configloader.Resource{
		Name:      "clusterJob",
		Transport: &configloader.TransportConfig{Client: "kubernetes"},
		Manifest: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata":   map[string]interface{}{"name": "my-job", "namespace": "default"},
		},
		Discovery: &configloader.DiscoveryConfig{Namespace: "default", ByName: "my-job"},
		Lifecycle: &configloader.ResourceLifecycle{
			Delete: &configloader.LifecycleDelete{
				When: &configloader.LifecycleWhen{Expression: "deleted_time != null"},
			},
		},
	}
	clusterConfigMap := configloader.Resource{
		Name:      "clusterConfigMap",
		Transport: &configloader.TransportConfig{Client: "kubernetes"},
		Manifest: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]interface{}{"name": "my-cm", "namespace": "default"},
		},
		Discovery: &configloader.DiscoveryConfig{Namespace: "default", ByName: "my-cm"},
		Lifecycle: &configloader.ResourceLifecycle{
			Delete: &configloader.LifecycleDelete{
				When: &configloader.LifecycleWhen{Expression: "deleted_time != null && !resources.?clusterJob.hasValue()"},
			},
		},
	}

	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Params["deleted_time"] = testDeletedTime

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{clusterJob, clusterConfigMap}, execCtx)

	require.NoError(t, err)
	require.Len(t, results, 2)

	// clusterJob: not found → nil stored → already deleted
	assert.Equal(t, manifest.OperationDelete, results[0].Operation)
	assert.Contains(t, results[0].OperationReason, "already deleted")

	// nil stored in context for clusterJob
	assert.Nil(t, execCtx.Resources["clusterJob"])

	// clusterConfigMap: clusterJob is nil in context → when-expression is true → delete
	assert.Equal(t, manifest.OperationDelete, results[1].Operation)
	assert.Equal(t, StatusSuccess, results[1].Status)
}

func TestResourceExecutor_LifecycleDelete_DeleteError(t *testing.T) {
	// Delete call fails → result is failed, ExecutionError is set
	discovered := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]interface{}{"name": "test-cm", "namespace": "default"},
		},
	}
	deleteErr := errors.New("RBAC denied")

	mock := k8sclient.NewMockK8sClient()
	mock.GetResourceResult = discovered
	mock.DeleteResourceError = deleteErr

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	resource := newResourceWithLifecycle("deleted_time != null", "Background")
	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Params["deleted_time"] = testDeletedTime

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resource}, execCtx)

	require.Error(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, StatusFailed, results[0].Status)
	assert.Equal(t, manifest.OperationDelete, results[0].Operation)
	require.NotNil(t, execCtx.Adapter.ExecutionError)
	assert.Equal(t, string(PhaseResources), execCtx.Adapter.ExecutionError.Phase)
	require.NotNil(t, execCtx.Adapter.ResourceErrors, "ResourceErrors map should be populated")
	assert.Contains(t, execCtx.Adapter.ResourceErrors, resource.Name, "resource error should be keyed by resource name")
}

// keepOnDeleteMockClient wraps MockK8sClient but intentionally does NOT remove resources on
// DeleteResource — it only records the TransportContext that was passed. This simulates the
// async Maestro deletion model: the resource stays discoverable after the delete call until
// Maestro finishes cleaning up sub-resources.
type keepOnDeleteMockClient struct {
	*k8sclient.MockK8sClient
	DeleteCalledWithTarget transportclient.TransportContext
}

func (m *keepOnDeleteMockClient) DeleteResource(
	ctx context.Context,
	gvk schema.GroupVersionKind,
	namespace, name string,
	opts *transportclient.DeleteOptions,
	target transportclient.TransportContext,
) error {
	m.DeleteCalledWithTarget = target
	// Intentionally skip removal: resource remains discoverable (Maestro async behavior).
	return nil
}

// ---- HIGH: CEL expression error handling ----

func TestResourceExecutor_LifecycleDelete_InvalidCELExpression(t *testing.T) {
	// when.expression contains a syntax error that makes the CEL compiler reject it.
	// Expected: evaluator returns error → executor fails with StatusFailed and ExecutionError set.
	// DeleteResource must not be called.
	mock := &trackingMockClient{MockK8sClient: k8sclient.NewMockK8sClient()}

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	// "deleted_time != null &&" is a dangling logical-AND — invalid CEL syntax.
	resource := newResourceWithLifecycle("deleted_time != null &&", "Background")
	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Params["deleted_time"] = testDeletedTime

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resource}, execCtx)

	require.Error(t, err, "CEL syntax error must surface as an executor error")
	assert.Contains(t, err.Error(), "failed to evaluate")
	require.Len(t, results, 1)
	assert.Equal(t, StatusFailed, results[0].Status)
	assert.False(t, mock.DeleteCalled, "DeleteResource must not be called when CEL has a syntax error")
}

func TestResourceExecutor_LifecycleDelete_CELUndeclaredVariable(t *testing.T) {
	// when.expression references a variable that was never captured in the precondition phase.
	// CEL treats undeclared variables as null (DynType), so "not_captured_var != null" evaluates
	// to false without error — the executor falls through to the normal apply path (OperationCreate).
	// This is distinct from a syntax error: the expression is syntactically valid but semantically
	// evaluates to false because the variable resolves to null.
	mock := &trackingMockClient{MockK8sClient: k8sclient.NewMockK8sClient()}

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	// "not_captured_var" is intentionally absent from execCtx.Params.
	resource := newResourceWithLifecycle("not_captured_var != null", "Background")
	execCtx := NewExecutionContext(context.Background(), nil, nil)
	// Note: "not_captured_var" is deliberately not set — simulates a missing precondition capture.

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resource}, execCtx)

	require.NoError(t, err, "CEL with undeclared variable evaluates to false (null != null = false), not an error")
	require.Len(t, results, 1)
	assert.Equal(t, StatusSuccess, results[0].Status)
	assert.Equal(t, manifest.OperationCreate, results[0].Operation)
	assert.False(t, mock.DeleteCalled, "DeleteResource must not be called when lifecycle.delete.when evaluates to false")
}

// ---- MEDIUM: PropagationPolicy passthrough ----

func TestResourceExecutor_LifecycleDelete_PropagationPolicy(t *testing.T) {
	// Verifies that the configured PropagationPolicy is passed through to DeleteResource,
	// and that an empty policy defaults to "Background".
	tests := []struct {
		name       string
		policy     string
		wantPolicy string
	}{
		{"Foreground passed through", "Foreground", "Foreground"},
		{"Orphan passed through", "Orphan", "Orphan"},
		{"empty policy defaults to Background", "", "Background"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			discovered := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{"name": "test-cm", "namespace": "default"},
				},
			}
			mock := &trackingMockClient{MockK8sClient: k8sclient.NewMockK8sClient()}
			// Store the resource so pre-delete discovery finds it and delete is attempted.
			mock.Resources["default/test-cm"] = discovered

			re := newResourceExecutor(&ExecutorConfig{
				TransportClient: mock,
				Logger:          logger.NewTestLogger(),
			})

			resource := newResourceWithLifecycle("deleted_time != null", tt.policy)
			execCtx := NewExecutionContext(context.Background(), nil, nil)
			execCtx.Params["deleted_time"] = testDeletedTime

			results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resource}, execCtx)

			require.NoError(t, err)
			require.Len(t, results, 1)
			assert.Equal(t, manifest.OperationDelete, results[0].Operation)
			assert.True(t, mock.DeleteCalled)
			assert.Equal(t, tt.wantPolicy, mock.DeleteCalledWithPolicy,
				"propagation policy %q should be passed to DeleteResource as %q", tt.policy, tt.wantPolicy)
		})
	}
}

// ---- MEDIUM: Pre-delete discovery transient error ----

func TestResourceExecutor_LifecycleDelete_PreDeleteDiscoveryError(t *testing.T) {
	// Pre-delete discovery (inside executeResourceDelete) returns a non-NotFound error.
	// preDiscoverAll (call 1) sees the resource as existing so the when-expression evaluates
	// to true and the delete path is entered. The discovery inside executeResourceDelete
	// (call 2) then fails with a transient error (e.g. network timeout, RBAC denial).
	// Expected: result is failed, ExecutionError is populated, DeleteResource is not called.
	transientErr := errors.New("connection timeout: context deadline exceeded")
	existingCM := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]interface{}{"name": "test-cm", "namespace": "default"},
		},
	}
	mock := &firstCallResultMock{
		MockK8sClient: k8sclient.NewMockK8sClient(),
		firstResult:   existingCM,
		firstErr:      nil,
		laterErr:      transientErr,
	}

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	resource := newResourceWithLifecycle("deleted_time != null", "Background")
	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Params["deleted_time"] = testDeletedTime

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resource}, execCtx)

	require.Error(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, StatusFailed, results[0].Status)
	assert.Equal(t, manifest.OperationDelete, results[0].Operation)
	require.NotNil(t, results[0].Error)
	assert.Contains(t, results[0].Error.Error(), "connection timeout")
	require.NotNil(t, execCtx.Adapter.ExecutionError, "ExecutionError must be set for upstream notification")
	assert.Equal(t, string(PhaseResources), execCtx.Adapter.ExecutionError.Phase)
	require.NotNil(t, execCtx.Adapter.ResourceErrors, "ResourceErrors map should be populated")
	assert.Contains(t, execCtx.Adapter.ResourceErrors, resource.Name, "resource error should be keyed by resource name")
	assert.False(t, mock.DeleteCalled, "DeleteResource must not be called when pre-delete discovery fails")
}

// ---- MEDIUM: lifecycle block present, but Delete is nil ----

func TestResourceExecutor_LifecycleDelete_DeleteConfigNil(t *testing.T) {
	// lifecycle block is present but has no delete sub-key (Lifecycle.Delete == nil).
	// The condition `lifecycle != nil && lifecycle.Delete != nil` is false — falls through to apply.
	// Expected: normal apply, no delete attempted.
	mock := k8sclient.NewMockK8sClient()

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	resource := configloader.Resource{
		Name:      "test-resource",
		Transport: &configloader.TransportConfig{Client: "kubernetes"},
		Manifest: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]interface{}{"name": "test-cm", "namespace": "default"},
		},
		Discovery: &configloader.DiscoveryConfig{Namespace: "default", ByName: "test-cm"},
		Lifecycle: &configloader.ResourceLifecycle{
			// Delete is nil — lifecycle block exists but no delete config
		},
	}
	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Params["deleted_time"] = testDeletedTime

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resource}, execCtx)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, StatusSuccess, results[0].Status)
	assert.Equal(t, manifest.OperationCreate, results[0].Operation,
		"lifecycle block with no delete config must fall through to normal apply")
}

// ---- MEDIUM: Maestro async deletion through executor ----

func TestResourceExecutor_LifecycleDelete_Maestro_AsyncDeletion(t *testing.T) {
	// Maestro transport: when.expression is true → delete is sent with a non-nil TransportContext.
	// Maestro deletion is async: the resource stays discoverable (deletionTimestamp set, not removed).
	// Post-delete rediscovery still finds the resource → stored as non-nil → dependents wait.
	discovered := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata":   map[string]interface{}{"name": "cluster-1-work", "namespace": "cluster-1"},
		},
	}

	// keepOnDeleteMockClient does NOT remove from Resources on delete — simulates Maestro async cleanup.
	mock := &keepOnDeleteMockClient{MockK8sClient: k8sclient.NewMockK8sClient()}
	mock.Resources["cluster-1/cluster-1-work"] = discovered

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	resource := configloader.Resource{
		Name: "clusterWork",
		Transport: &configloader.TransportConfig{
			Client:  "maestro",
			Maestro: &configloader.MaestroTransportConfig{TargetCluster: "cluster-1"},
		},
		Manifest: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata":   map[string]interface{}{"name": "cluster-1-work", "namespace": "cluster-1"},
		},
		Discovery: &configloader.DiscoveryConfig{Namespace: "cluster-1", ByName: "cluster-1-work"},
		Lifecycle: &configloader.ResourceLifecycle{
			Delete: &configloader.LifecycleDelete{
				When: &configloader.LifecycleWhen{Expression: "deleted_time != null"},
			},
		},
	}

	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Params["deleted_time"] = testDeletedTime

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resource}, execCtx)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, StatusSuccess, results[0].Status)
	assert.Equal(t, manifest.OperationDelete, results[0].Operation)

	// Maestro transport must pass a non-nil TransportContext to DeleteResource.
	assert.NotNil(t, mock.DeleteCalledWithTarget,
		"Maestro transport must supply a non-nil TransportContext to DeleteResource")

	// Resource stays non-nil in context: async cleanup → dependents wait for next reconciliation.
	storedVal, exists := execCtx.Resources["clusterWork"]
	assert.True(t, exists, "resource key must be present in execCtx after Maestro delete")
	assert.NotNil(t, storedVal,
		"non-nil stored: Maestro deletion is async — dependents must wait for next reconciliation")
}

// multiDeleteMock tracks which resource names were passed to DeleteResource and always
// returns a configured error, while GetResource returns a distinct object per name.
type multiDeleteMock struct {
	*k8sclient.MockK8sClient
	objects     map[string]*unstructured.Unstructured
	deleteErr   error
	deleteCalls []string
}

func (m *multiDeleteMock) GetResource(
	ctx context.Context,
	gvk schema.GroupVersionKind,
	namespace, name string,
	target transportclient.TransportContext,
) (*unstructured.Unstructured, error) {
	if obj, ok := m.objects[name]; ok {
		return obj, nil
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{}, name)
}

func (m *multiDeleteMock) DeleteResource(
	ctx context.Context,
	gvk schema.GroupVersionKind,
	namespace, name string,
	opts *transportclient.DeleteOptions,
	target transportclient.TransportContext,
) error {
	m.deleteCalls = append(m.deleteCalls, name)
	return m.deleteErr
}

func TestResourceExecutor_ExecuteAll_ContinuesAfterDeleteFailure(t *testing.T) {
	// JIRA HYPERFLEET-849: "continue with the rest of resources deletion" even when one fails.
	// If resource A's delete fails, resource B must still be attempted.
	makeObj := func(name string) *unstructured.Unstructured {
		return &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]interface{}{"name": name, "namespace": "default"},
			},
		}
	}

	mock := &multiDeleteMock{
		MockK8sClient: k8sclient.NewMockK8sClient(),
		objects: map[string]*unstructured.Unstructured{
			"resource-a": makeObj("resource-a"),
			"resource-b": makeObj("resource-b"),
		},
		deleteErr: errors.New("RBAC denied"),
	}
	// ApplyResource is never reached in the delete path, but set a result to be safe.
	mock.ApplyResourceResult = &transportclient.ApplyResult{Operation: manifest.OperationCreate}

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	resourceA := newResourceWithLifecycle("deleted_time != null", "Background")
	resourceA.Name = "resource-a"
	resourceA.Discovery = &configloader.DiscoveryConfig{ByName: "resource-a", Namespace: "default"}

	resourceB := newResourceWithLifecycle("deleted_time != null", "Background")
	resourceB.Name = "resource-b"
	resourceB.Discovery = &configloader.DiscoveryConfig{ByName: "resource-b", Namespace: "default"}

	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Params["deleted_time"] = testDeletedTime

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resourceA, resourceB}, execCtx)

	// Both resources must have been attempted despite the first failure.
	require.Len(t, mock.deleteCalls, 2, "both resources must be attempted even after first delete fails")
	assert.Equal(t, "resource-a", mock.deleteCalls[0])
	assert.Equal(t, "resource-b", mock.deleteCalls[1])

	// ExecuteAll returns the first delete error.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RBAC denied")

	// Both results are present and marked failed.
	require.Len(t, results, 2)
	assert.Equal(t, StatusFailed, results[0].Status)
	assert.Equal(t, StatusFailed, results[1].Status)
}

func TestResourceExecutor_ExecuteAll_ContinuesAfterCELEvalError(t *testing.T) {
	// JIRA HYPERFLEET-849: a CEL evaluation error on resourceA's lifecycle.delete.when must not
	// skip resourceB. Before the fix, result.Operation was "" (zero value), so the delete-continue
	// branch was never taken and ExecuteAll returned early.
	// Default mock behavior stores applied resources, so post-apply discovery succeeds for resourceB.
	mock := k8sclient.NewMockK8sClient()

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	// resourceA has an invalid CEL expression — evaluateLifecycleDeleteWhen will error.
	resourceA := newResourceWithLifecycle("deleted_time != null &&", "Background")
	resourceA.Name = "resource-a"

	// resourceB has a valid expression that evaluates to false → normal apply.
	resourceB := newResourceWithLifecycle("false", "Background")
	resourceB.Name = "resource-b"

	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Params["deleted_time"] = testDeletedTime

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resourceA, resourceB}, execCtx)

	// Both resources must appear in results.
	require.Len(t, results, 2, "resourceB must be processed even after resourceA's CEL error")
	assert.Equal(t, StatusFailed, results[0].Status)
	assert.Equal(t, manifest.OperationDelete, results[0].Operation, "CEL error on delete path must set OperationDelete")
	// resourceB: lifecycle.delete.when=false → falls through to apply → success.
	assert.Equal(t, StatusSuccess, results[1].Status)

	// ExecuteAll returns the accumulated CEL error.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to evaluate")
}

func TestResourceExecutor_GetCELVariables_DeletedResourceAbsent(t *testing.T) {
	// Nil (deleted) resources must be absent from the CEL resources map so that
	// "!resources.?clusterJob.hasValue()" correctly evaluates to true when a
	// resource is confirmed deleted. Optional.none().hasValue() == false, which
	// is the intended semantics for lifecycle ordering expressions.
	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Resources["clusterJob"] = nil // explicitly stored nil (deleted resource)
	execCtx.Resources["clusterConfigMap"] = &unstructured.Unstructured{
		Object: map[string]interface{}{"kind": "ConfigMap"},
	}

	vars := execCtx.GetCELVariables()
	resources, ok := vars["resources"].(map[string]interface{})
	require.True(t, ok)

	// nil-sentinel resource must NOT be in the CEL resources map;
	// accessing via resources.?clusterJob returns Optional.none() → hasValue() = false
	_, exists := resources["clusterJob"]
	assert.False(t, exists, "nil-sentinel (deleted) resource must be absent from CEL resources map")

	// non-nil resource is present normally
	_, exists = resources["clusterConfigMap"]
	assert.True(t, exists, "non-nil resource should be in CEL resources map")
}

// selectorTrackingMockClient overrides DiscoverResources to return the pre-delete list on the
// first call and an empty list on subsequent calls, simulating instant K8s deletion via
// label-selector discovery.
type selectorTrackingMockClient struct {
	*k8sclient.MockK8sClient
	DeleteCalledWithPolicy string
	discoverCalls          int
	DeleteCalled           bool
}

func (m *selectorTrackingMockClient) DiscoverResources(
	ctx context.Context,
	gvk schema.GroupVersionKind,
	discovery manifest.Discovery,
	target transportclient.TransportContext,
) (*unstructured.UnstructuredList, error) {
	m.discoverCalls++
	// Calls 1 (preDiscoverAll) and 2 (executeResourceDelete pre-delete): resource exists.
	// Call 3+ (post-delete rediscovery): resource gone (instant K8s delete, no finalizers).
	if m.discoverCalls <= 2 {
		return m.MockK8sClient.DiscoverResources(ctx, gvk, discovery, target)
	}
	return &unstructured.UnstructuredList{}, nil
}

func (m *selectorTrackingMockClient) DeleteResource(
	ctx context.Context,
	gvk schema.GroupVersionKind,
	namespace, name string,
	opts *transportclient.DeleteOptions,
	target transportclient.TransportContext,
) error {
	m.DeleteCalled = true
	if opts != nil {
		m.DeleteCalledWithPolicy = opts.PropagationPolicy
	}
	return m.MockK8sClient.DeleteResource(ctx, gvk, namespace, name, opts, target)
}

// TestResourceExecutor_LifecycleDelete_BySelectors verifies that lifecycle delete works with
// label-selector discovery (JIRA HYPERFLEET-849 AC: "Label selector discovery works for
// multi-generation resources"). DiscoverResources is used instead of GetResource, and the
// resource returned by GetLatestGenerationFromList is the one that gets deleted.
func TestResourceExecutor_LifecycleDelete_BySelectors(t *testing.T) {
	discovered := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "my-cm",
				"namespace": "default",
			},
		},
	}

	inner := k8sclient.NewMockK8sClient()
	inner.DiscoverResult = &unstructured.UnstructuredList{Items: []unstructured.Unstructured{*discovered}}

	mock := &selectorTrackingMockClient{MockK8sClient: inner}

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	resource := configloader.Resource{
		Name:      "test-resource",
		Transport: &configloader.TransportConfig{Client: "kubernetes"},
		Manifest: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]interface{}{"name": "my-cm", "namespace": "default"},
		},
		Discovery: &configloader.DiscoveryConfig{
			Namespace: "default",
			BySelectors: &configloader.SelectorConfig{
				LabelSelector: map[string]string{"app": "test"},
			},
		},
		Lifecycle: &configloader.ResourceLifecycle{
			Delete: &configloader.LifecycleDelete{
				When: &configloader.LifecycleWhen{Expression: "deleted_time != null"},
			},
		},
	}
	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Params["deleted_time"] = testDeletedTime

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resource}, execCtx)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, StatusSuccess, results[0].Status)
	assert.Equal(t, manifest.OperationDelete, results[0].Operation)
	assert.True(t, mock.DeleteCalled, "DeleteResource should have been called")
	assert.GreaterOrEqual(t, mock.discoverCalls, 2, "DiscoverResources called for pre- and post-delete")

	// Resource confirmed gone (post-delete discovery returned empty) → nil stored.
	storedVal, exists := execCtx.Resources[resource.Name]
	assert.True(t, exists, "nil sentinel should be in execCtx.Resources")
	assert.Nil(t, storedVal, "nil stored when post-delete discovery finds no resources")
}
