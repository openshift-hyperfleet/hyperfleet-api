package configloader

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
)

const testAdapterConfigYAML = `
adapter:
  name: test-adapter
  version: "0.1.0"
clients:
  hyperfleet_api:
    base_url: "https://test.example.com"
    timeout: 2s
  kubernetes:
    api_version: "v1"
`

// createTestConfigFiles creates temporary adapter and task config files for testing
func createTestConfigFiles(t *testing.T, tmpDir string, adapterYAML, taskYAML string) (adapterPath, taskPath string) {
	t.Helper()

	adapterPath = filepath.Join(tmpDir, "adapter-config.yaml")
	taskPath = filepath.Join(tmpDir, "task-config.yaml")

	err := os.WriteFile(adapterPath, []byte(adapterYAML), 0644)
	require.NoError(t, err)

	err = os.WriteFile(taskPath, []byte(taskYAML), 0644)
	require.NoError(t, err)

	return adapterPath, taskPath
}

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()

	adapterYAML := `
adapter:
  name: deployment-config
  version: "0.1.0"
clients:
  hyperfleet_api:
    base_url: "https://test.example.com"
    timeout: 2s
    retry_attempts: 3
    retry_backoff: exponential
  kubernetes:
    api_version: "v1"
`

	taskYAML := `
params:
  - name: "clusterId"
    source: "event.id"
    type: "string"
    required: true
preconditions:
  - name: "clusterStatus"
    api_call:
      method: "GET"
      url: "https://api.example.com/clusters/{{ .clusterId }}"
resources:
  - name: "testNamespace"
    manifest:
      apiVersion: v1
      kind: Namespace
      metadata:
        name: "test-ns"
    discovery:
      namespace: "*"
      by_name: "test-ns"
`

	adapterPath, taskPath := createTestConfigFiles(t, tmpDir, adapterYAML, taskYAML)

	// Test loading
	config, err := LoadConfig(
		WithAdapterConfigPath(adapterPath),
		WithTaskConfigPath(taskPath),
		WithSkipSemanticValidation(),
	)
	require.NoError(t, err)
	require.NotNil(t, config)

	// Verify merged config fields
	// Adapter info comes from adapter config
	assert.Equal(t, "deployment-config", config.Adapter.Name)
	assert.Equal(t, "0.1.0", config.Adapter.Version)
	// Clients config comes from adapter config
	assert.Equal(t, "https://test.example.com", config.Clients.HyperfleetAPI.BaseURL)
	assert.Equal(t, 2*time.Second, config.Clients.HyperfleetAPI.Timeout)
	// Task fields come from task config
	require.Len(t, config.Params, 1)
	assert.Equal(t, "clusterId", config.Params[0].Name)
	require.Len(t, config.Preconditions, 1)
	assert.Equal(t, "clusterStatus", config.Preconditions[0].Name)
	require.Len(t, config.Resources, 1)
	assert.Equal(t, "testNamespace", config.Resources[0].Name)
}

func TestLoadConfigMissingAdapterConfig(t *testing.T) {
	tmpDir := t.TempDir()
	taskPath := filepath.Join(tmpDir, "task-config.yaml")
	err := os.WriteFile(taskPath, []byte(`{}`), 0644)
	require.NoError(t, err)

	config, err := LoadConfig(
		WithAdapterConfigPath("/nonexistent/adapter-config.yaml"),
		WithTaskConfigPath(taskPath),
	)
	assert.Error(t, err)
	assert.Nil(t, config)
	assert.Contains(t, err.Error(), "failed to load adapter config")
}

func TestLoadConfigMissingTaskConfig(t *testing.T) {
	tmpDir := t.TempDir()
	adapterPath := filepath.Join(tmpDir, "adapter-config.yaml")
	err := os.WriteFile(adapterPath, []byte(`
adapter:
  name: test-adapter
  version: "1.0.0"
clients:
  hyperfleet_api:
    timeout: 5s
  kubernetes:
    api_version: v1
`), 0644)
	require.NoError(t, err)

	config, err := LoadConfig(
		WithAdapterConfigPath(adapterPath),
		WithTaskConfigPath("/nonexistent/task-config.yaml"),
	)
	assert.Error(t, err)
	assert.Nil(t, config)
	assert.Contains(t, err.Error(), "failed to load task config")
}

func TestAdapterConfigValidation(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		errorMsg  string
		wantError bool
	}{
		{
			name: "valid minimal adapter config",
			yaml: `
adapter:
  name: test-adapter
  version: "1.0.0"
clients:
  hyperfleet_api:
    timeout: 5s
  kubernetes:
    api_version: "v1"
`,
			wantError: false,
		},
		{
			name: "missing adapter.name",
			yaml: `
adapter:
  version: "1.0.0"
clients:
  hyperfleet_api:
    timeout: 5s
`,
			wantError: true,
			errorMsg:  "name is required",
		},
		{
			name: "missing adapter.version",
			yaml: `
adapter:
  name: test-adapter
clients:
  hyperfleet_api:
    timeout: 5s
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config AdapterConfig
			err := yaml.Unmarshal([]byte(tt.yaml), &config)
			require.NoError(t, err)

			validator := NewAdapterConfigValidator(&config, "")
			err = validator.ValidateStructure()

			if tt.wantError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTaskConfigValidation(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		errorMsg  string
		wantError bool
	}{
		{
			name:      "valid minimal task config",
			yaml:      `{}`,
			wantError: false,
		},
		{
			name: "valid task config with params",
			yaml: `
params:
  - name: "clusterId"
    source: "event.id"
    required: true
`,
			wantError: false,
		},
		{
			name: "parameter without name",
			yaml: `
params:
  - source: "event.id"
`,
			wantError: true,
			errorMsg:  "params[0].name is required",
		},
		{
			name: "parameter without source",
			yaml: `
params:
  - name: "clusterId"
    required: true
`,
			wantError: true,
			errorMsg:  "source is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config AdapterTaskConfig
			err := yaml.Unmarshal([]byte(tt.yaml), &config)
			require.NoError(t, err)

			validator := NewTaskConfigValidator(&config, "")
			err = validator.ValidateStructure()

			if tt.wantError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePreconditionsInTaskConfig(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		errorMsg  string
		wantError bool
	}{
		{
			name: "valid precondition with API call",
			yaml: `
preconditions:
  - name: "checkCluster"
    api_call:
      method: "GET"
      url: "https://api.example.com/clusters"
`,
			wantError: false,
		},
		{
			name: "precondition without name",
			yaml: `
preconditions:
  - api_call:
      method: "GET"
      url: "https://api.example.com/clusters"
`,
			wantError: true,
			errorMsg:  "preconditions[0].name is required",
		},
		{
			name: "precondition without apiCall or expression",
			yaml: `
preconditions:
  - name: "checkCluster"
`,
			wantError: true,
			errorMsg:  "preconditions[0]: must specify api_call, conditions",
		},
		{
			name: "API call without method",
			yaml: `
preconditions:
  - name: "checkCluster"
    api_call:
      url: "https://api.example.com/clusters"
`,
			wantError: true,
			errorMsg:  "method is required",
		},
		{
			name: "API call with invalid method",
			yaml: `
preconditions:
  - name: "checkCluster"
    api_call:
      method: "INVALID"
      url: "https://api.example.com/clusters"
`,
			wantError: true,
			errorMsg:  "is invalid (allowed:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config AdapterTaskConfig
			err := yaml.Unmarshal([]byte(tt.yaml), &config)
			require.NoError(t, err)

			validator := NewTaskConfigValidator(&config, "")
			err = validator.ValidateStructure()

			if tt.wantError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateResourcesInTaskConfig(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		errorMsg  string
		wantError bool
	}{
		{
			name: "valid resource with manifest",
			yaml: `
resources:
  - name: "testNamespace"
    manifest:
      apiVersion: v1
      kind: Namespace
      metadata:
        name: "test-ns"
    discovery:
      namespace: "*"
      by_name: "test-ns"
`,
			wantError: false,
		},
		{
			name: "resource without name",
			yaml: `
resources:
  - manifest:
      apiVersion: v1
      kind: Namespace
`,
			wantError: true,
			errorMsg:  "resources[0].name is required",
		},
		{
			name: "resource without manifest - kubernetes transport requires manifest in semantic validation",
			yaml: `
resources:
  - name: "testNamespace"
    discovery:
      by_name: "test-ns"
`,
			wantError: false, // Manifest is no longer structurally required (validated semantically based on transport type)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config AdapterTaskConfig
			err := yaml.Unmarshal([]byte(tt.yaml), &config)
			require.NoError(t, err)

			validator := NewTaskConfigValidator(&config, "")
			err = validator.ValidateStructure()

			if tt.wantError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMergeConfigs(t *testing.T) {
	adapterCfg := &AdapterConfig{
		Adapter: AdapterInfo{
			Name:    "adapter-deployment",
			Version: "1.0.0",
		},
		Clients: ClientsConfig{
			HyperfleetAPI: HyperfleetAPIConfig{
				BaseURL:       "https://api.example.com",
				Timeout:       5 * time.Second,
				RetryAttempts: 3,
			},
			Kubernetes: KubernetesConfig{
				APIVersion: "v1",
			},
		},
	}

	taskCfg := &AdapterTaskConfig{
		Params: []Parameter{
			{Name: "clusterId", Source: StringSource("event.id"), Required: true},
		},
		Preconditions: []Precondition{
			{ActionBase: ActionBase{Name: "checkStatus"}},
		},
		Resources: []Resource{
			{Name: "namespace"},
		},
	}

	merged := Merge(adapterCfg, taskCfg)

	// Verify merged config
	// Adapter info from adapter config
	assert.Equal(t, "adapter-deployment", merged.Adapter.Name)
	assert.Equal(t, "1.0.0", merged.Adapter.Version)
	// Clients from adapter config
	assert.Equal(t, "https://api.example.com", merged.Clients.HyperfleetAPI.BaseURL)
	assert.Equal(t, 5*time.Second, merged.Clients.HyperfleetAPI.Timeout)
	// Task fields from task config
	require.Len(t, merged.Params, 1)
	assert.Equal(t, "clusterId", merged.Params[0].Name)
	require.Len(t, merged.Preconditions, 1)
	assert.Equal(t, "checkStatus", merged.Preconditions[0].Name)
	require.Len(t, merged.Resources, 1)
	assert.Equal(t, "namespace", merged.Resources[0].Name)
}

func TestGetRequiredParams(t *testing.T) {
	config := &Config{
		Params: []Parameter{
			{Name: "clusterId", Source: StringSource("event.id"), Required: true},
			{Name: "optional", Source: StringSource("event.optional"), Required: false},
		},
	}

	requiredParams := config.GetRequiredParams()
	assert.Len(t, requiredParams, 1)
	assert.Equal(t, "clusterId", requiredParams[0].Name)
}

func TestGetResourceByName(t *testing.T) {
	config := &Config{
		Resources: []Resource{
			{Name: "namespace1"},
			{Name: "namespace2"},
		},
	}

	resource := config.GetResourceByName("namespace1")
	assert.NotNil(t, resource)
	assert.Equal(t, "namespace1", resource.Name)

	resource = config.GetResourceByName("nonexistent")
	assert.Nil(t, resource)
}

func TestGetPreconditionByName(t *testing.T) {
	config := &Config{
		Preconditions: []Precondition{
			{ActionBase: ActionBase{Name: "precond1"}},
			{ActionBase: ActionBase{Name: "precond2"}},
		},
	}

	precond := config.GetPreconditionByName("precond1")
	assert.NotNil(t, precond)
	assert.Equal(t, "precond1", precond.Name)

	precond = config.GetPreconditionByName("nonexistent")
	assert.Nil(t, precond)
}

func TestValidateAdapterVersion(t *testing.T) {
	ctx := context.Background()
	log := newTestLogger(nil)

	config := &AdapterConfig{
		Adapter: AdapterInfo{
			Name:    "test-adapter",
			Version: "1.0.0",
		},
	}

	// Exact match
	err := ValidateAdapterVersion(ctx, log, config, "1.0.0")
	assert.NoError(t, err)

	// Patch version differs - should pass (bug fix release)
	err = ValidateAdapterVersion(ctx, log, config, "1.0.5")
	assert.NoError(t, err)

	// Minor version differs - should fail
	err = ValidateAdapterVersion(ctx, log, config, "1.1.0")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "adapter version mismatch")

	// Major version differs - should fail
	err = ValidateAdapterVersion(ctx, log, config, "2.0.0")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "adapter version mismatch")

	// Empty expected version (skip validation)
	err = ValidateAdapterVersion(ctx, log, config, "")
	assert.NoError(t, err)

	// Dev build versions (0.0.0-* skip validation)
	err = ValidateAdapterVersion(ctx, log, config, "0.0.0-dev")
	assert.NoError(t, err)

	err = ValidateAdapterVersion(ctx, log, config, "0.0.0-master")
	assert.NoError(t, err)

	err = ValidateAdapterVersion(ctx, log, config, "v0.0.0-dev")
	assert.NoError(t, err)

	// Empty config version (not provided in adapter config - skip validation)
	noVersionConfig := &AdapterConfig{
		Adapter: AdapterInfo{
			Name: "test-adapter",
		},
	}
	err = ValidateAdapterVersion(ctx, log, noVersionConfig, "1.0.0")
	assert.NoError(t, err)

	// Pre-release version with same major.minor - should pass
	err = ValidateAdapterVersion(ctx, log, config, "1.0.1-rc.1")
	assert.NoError(t, err)

	// Non-semver config version - should warn and skip validation
	invalidConfig := &AdapterConfig{
		Adapter: AdapterInfo{
			Name:    "test-adapter",
			Version: "not-a-version",
		},
	}
	var buf bytes.Buffer
	logWithCapture := newTestLogger(&buf)

	err = ValidateAdapterVersion(ctx, logWithCapture, invalidConfig, "1.0.0")
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Skipping adapter version validation")
	assert.Contains(t, buf.String(), "config version is not valid semver")

	// Non-semver binary version - should warn and skip validation
	buf.Reset()
	err = ValidateAdapterVersion(ctx, logWithCapture, config, "not-a-version")
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Skipping adapter version validation")
	assert.Contains(t, buf.String(), "binary version is not valid semver")

	// Non-semver binary version "dev" (Konflux pipeline case)
	buf.Reset()
	err = ValidateAdapterVersion(ctx, logWithCapture, config, "dev")
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Skipping adapter version validation")
	assert.Contains(t, buf.String(), "binary version is not valid semver")

	// Non-semver config version with valid binary version
	devConfig := &AdapterConfig{
		Adapter: AdapterInfo{
			Name:    "test-adapter",
			Version: "dev",
		},
	}
	buf.Reset()
	err = ValidateAdapterVersion(ctx, logWithCapture, devConfig, "1.0.0")
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Skipping adapter version validation")
	assert.Contains(t, buf.String(), "config version is not valid semver")

	// Both versions non-semver (config parse fails first, so only config warning emitted)
	buf.Reset()
	err = ValidateAdapterVersion(ctx, logWithCapture, devConfig, "latest")
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Skipping adapter version validation")
	assert.Contains(t, buf.String(), "config version is not valid semver")
}

func newTestLogger(buf *bytes.Buffer) logger.Logger {
	cfg := logger.DefaultConfig()
	cfg.Format = "text"
	if buf != nil {
		cfg.Writer = buf
	}
	l, _ := logger.NewLogger(cfg)
	return l
}

func TestValidateFileReferencesInTaskConfig(t *testing.T) {
	// Create temporary directory with test files
	tmpDir := t.TempDir()

	// Create a test template file
	templatePath := filepath.Join(tmpDir, "templates")
	require.NoError(t, os.MkdirAll(templatePath, 0755))
	templateFile := filepath.Join(templatePath, "test-template.yaml")
	require.NoError(t, os.WriteFile(templateFile, []byte("test: value"), 0644))

	tests := []struct {
		name    string
		config  *AdapterTaskConfig
		errMsg  string
		baseDir string
		wantErr bool
	}{
		{
			name: "valid payload buildRef",
			config: &AdapterTaskConfig{
				Post: &PostConfig{
					Payloads: []Payload{
						{Name: "test", BuildRef: "templates/test-template.yaml"},
					},
				},
			},
			baseDir: tmpDir,
			wantErr: false,
		},
		{
			name: "invalid payload buildRef - file not found",
			config: &AdapterTaskConfig{
				Post: &PostConfig{
					Payloads: []Payload{
						{Name: "test", BuildRef: "templates/nonexistent.yaml"},
					},
				},
			},
			baseDir: tmpDir,
			wantErr: true,
			errMsg:  "does not exist",
		},
		{
			name: "invalid payload buildRef - is a directory",
			config: &AdapterTaskConfig{
				Post: &PostConfig{
					Payloads: []Payload{
						{Name: "test", BuildRef: "templates"},
					},
				},
			},
			baseDir: tmpDir,
			wantErr: true,
			errMsg:  "is a directory",
		},
		{
			name: "valid manifest.ref",
			config: &AdapterTaskConfig{
				Resources: []Resource{
					{
						Name: "test",
						Manifest: map[string]interface{}{
							"ref": "templates/test-template.yaml",
						},
					},
				},
			},
			baseDir: tmpDir,
			wantErr: false,
		},
		{
			name: "invalid manifest.ref - file not found",
			config: &AdapterTaskConfig{
				Resources: []Resource{
					{
						Name: "test",
						Manifest: map[string]interface{}{
							"ref": "templates/nonexistent.yaml",
						},
					},
				},
			},
			baseDir: tmpDir,
			wantErr: true,
			errMsg:  "does not exist",
		},
		{
			name: "valid multiple payloads with buildRef",
			config: &AdapterTaskConfig{
				Post: &PostConfig{
					Payloads: []Payload{
						{Name: "payload1", BuildRef: "templates/test-template.yaml"},
						{Name: "payload2", BuildRef: "templates/test-template.yaml"},
					},
				},
			},
			baseDir: tmpDir,
			wantErr: false,
		},
		{
			name: "no file references - should pass",
			config: &AdapterTaskConfig{
				Params: []Parameter{
					{Name: "test", Source: StringSource("event.test")},
				},
			},
			baseDir: tmpDir,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewTaskConfigValidator(tt.config, tt.baseDir)
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

func TestLoadConfigWithFileReferences(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Create a template file
	templateDir := filepath.Join(tmpDir, "templates")
	require.NoError(t, os.MkdirAll(templateDir, 0755))
	templateFile := filepath.Join(templateDir, "status-payload.yaml")
	require.NoError(t, os.WriteFile(templateFile, []byte(`
status: "{{ .status }}"
`), 0644))

	// Create adapter config file
	adapterYAML := testAdapterConfigYAML
	adapterPath := filepath.Join(tmpDir, "adapter-config.yaml")
	require.NoError(t, os.WriteFile(adapterPath, []byte(adapterYAML), 0644))

	// Create task config file with build_ref
	taskYAML := `
params:
  - name: "clusterId"
    source: "event.id"
resources:
  - name: "testNamespace"
    manifest:
      apiVersion: v1
      kind: Namespace
      metadata:
        name: test
    discovery:
      namespace: "*"
      by_name: "test"
post:
  payloads:
    - name: "statusPayload"
      build_ref: "templates/status-payload.yaml"
`
	taskPath := filepath.Join(tmpDir, "task-config.yaml")
	require.NoError(t, os.WriteFile(taskPath, []byte(taskYAML), 0644))

	// Load should succeed because template file exists
	config, err := LoadConfig(
		WithAdapterConfigPath(adapterPath),
		WithTaskConfigPath(taskPath),
		WithSkipSemanticValidation(),
	)
	require.NoError(t, err)
	require.NotNil(t, config)
	assert.Equal(t, "test-adapter", config.Adapter.Name)

	// Verify buildRef content was loaded
	require.NotNil(t, config.Post)
	require.Len(t, config.Post.Payloads, 1)
	assert.NotNil(t, config.Post.Payloads[0].BuildRefContent)

	// Now test with non-existent buildRef
	taskYAMLBad := `
params:
  - name: "clusterId"
    source: "event.id"
resources:
  - name: "testNamespace"
    manifest:
      apiVersion: v1
      kind: Namespace
      metadata:
        name: test
    discovery:
      namespace: "*"
      by_name: "test"
post:
  payloads:
    - name: "statusPayload"
      build_ref: "templates/nonexistent.yaml"
`
	taskPathBad := filepath.Join(tmpDir, "task-config-bad.yaml")
	require.NoError(t, os.WriteFile(taskPathBad, []byte(taskYAMLBad), 0644))

	// Load should fail because template file doesn't exist
	config, err = LoadConfig(
		WithAdapterConfigPath(adapterPath),
		WithTaskConfigPath(taskPathBad),
		WithSkipSemanticValidation(),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
	assert.Nil(t, config)
}

func TestLoadFileReferencesContent(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	templateDir := filepath.Join(tmpDir, "templates")
	require.NoError(t, os.MkdirAll(templateDir, 0755))

	// Create a buildRef template file
	buildRefFile := filepath.Join(templateDir, "status-payload.yaml")
	require.NoError(t, os.WriteFile(buildRefFile, []byte(`
status: "{{ .status }}"
message: "Operation completed"
`), 0644))

	// Create a manifest.ref template file
	manifestRefFile := filepath.Join(templateDir, "deployment.yaml")
	require.NoError(t, os.WriteFile(manifestRefFile, []byte(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: "{{ .name }}"
  namespace: "{{ .namespace }}"
spec:
  replicas: 1
`), 0644))

	// Create adapter config
	adapterYAML := testAdapterConfigYAML
	adapterPath := filepath.Join(tmpDir, "adapter-config.yaml")
	require.NoError(t, os.WriteFile(adapterPath, []byte(adapterYAML), 0644))

	// Create task config file with both build_ref and manifest.ref
	taskYAML := `
params:
  - name: "clusterId"
    source: "event.id"
resources:
  - name: "deployment"
    manifest:
      ref: "templates/deployment.yaml"
    discovery:
      namespace: "*"
      by_selectors:
        label_selector:
          app: "test"
post:
  payloads:
    - name: "statusPayload"
      build_ref: "templates/status-payload.yaml"
`
	taskPath := filepath.Join(tmpDir, "task-config.yaml")
	require.NoError(t, os.WriteFile(taskPath, []byte(taskYAML), 0644))

	// Load config
	config, err := LoadConfig(
		WithAdapterConfigPath(adapterPath),
		WithTaskConfigPath(taskPath),
		WithSkipSemanticValidation(),
	)
	require.NoError(t, err)
	require.NotNil(t, config)

	// Verify manifest.ref was loaded as raw string (for Go template support)
	require.Len(t, config.Resources, 1)
	manifestStr, ok := config.Resources[0].Manifest.(string)
	require.True(t, ok, "Manifest should be a raw string after loading ref")
	assert.Contains(t, manifestStr, "apiVersion: apps/v1")
	assert.Contains(t, manifestStr, "kind: Deployment")
	assert.Contains(t, manifestStr, "{{ .name }}")
	assert.Contains(t, manifestStr, "{{ .namespace }}")

	// Verify buildRef content was loaded into BuildRefContent
	require.NotNil(t, config.Post)
	require.Len(t, config.Post.Payloads, 1)
	assert.NotNil(t, config.Post.Payloads[0].BuildRefContent)
	assert.Equal(t, "{{ .status }}", config.Post.Payloads[0].BuildRefContent["status"])
	assert.Equal(t, "Operation completed", config.Post.Payloads[0].BuildRefContent["message"])
	// Original BuildRef path should still be preserved
	assert.Equal(t, "templates/status-payload.yaml", config.Post.Payloads[0].BuildRef)
}

func TestValidateResourceDiscoveryInTaskConfig(t *testing.T) {
	// Helper to create a valid task config with given resources
	configWithResources := func(resources []Resource) *AdapterTaskConfig {
		return &AdapterTaskConfig{
			Resources: resources,
		}
	}

	tests := []struct {
		name      string
		errMsg    string
		resources []Resource
		wantErr   bool
	}{
		{
			name: "valid - manifest.ref with discovery bySelectors",
			resources: []Resource{
				{
					Name:     "test",
					Manifest: map[string]interface{}{"ref": "templates/test.yaml"},
					Discovery: &DiscoveryConfig{
						Namespace: "test-ns",
						BySelectors: &SelectorConfig{
							LabelSelector: map[string]string{"app": "test"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid - manifest.ref with discovery byName",
			resources: []Resource{
				{
					Name:     "test",
					Manifest: map[string]interface{}{"ref": "templates/test.yaml"},
					Discovery: &DiscoveryConfig{
						Namespace: "*",
						ByName:    "my-resource",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid - inline manifest with discovery",
			resources: []Resource{
				{
					Name: "test",
					Manifest: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
					},
					Discovery: &DiscoveryConfig{
						Namespace: "test-ns",
						ByName:    "my-configmap",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid - inline manifest missing discovery",
			resources: []Resource{
				{
					Name: "test",
					Manifest: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
					},
					// Missing discovery - required for all resources
				},
			},
			wantErr: true,
			errMsg:  "discovery is required",
		},
		{
			name: "invalid - manifest.ref missing discovery",
			resources: []Resource{
				{
					Name:     "test",
					Manifest: map[string]interface{}{"ref": "templates/test.yaml"},
					// Missing discovery
				},
			},
			wantErr: true,
			errMsg:  "discovery is required",
		},
		{
			name: "valid - manifest.ref with discovery missing namespace (all namespaces)",
			resources: []Resource{
				{
					Name:     "test",
					Manifest: map[string]interface{}{"ref": "templates/test.yaml"},
					Discovery: &DiscoveryConfig{
						// Empty namespace means all namespaces
						ByName: "my-resource",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid - manifest.ref missing byName or bySelectors",
			resources: []Resource{
				{
					Name:     "test",
					Manifest: map[string]interface{}{"ref": "templates/test.yaml"},
					Discovery: &DiscoveryConfig{
						Namespace: "test-ns",
						// Missing byName and bySelectors
					},
				},
			},
			wantErr: true,
			errMsg:  "resources[0].discovery: must have either 'by_name' or 'by_selectors' set",
		},
		{
			name: "invalid - bySelectors without labelSelector defined",
			resources: []Resource{
				{
					Name:     "test",
					Manifest: map[string]interface{}{"ref": "templates/test.yaml"},
					Discovery: &DiscoveryConfig{
						Namespace:   "test-ns",
						BySelectors: &SelectorConfig{
							// Empty selectors
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "resources[0].discovery.by_selectors.label_selector is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := configWithResources(tt.resources)
			errs := ValidateStruct(config)
			if tt.wantErr {
				require.True(t, errs != nil && errs.HasErrors(), "expected error but got none")
				assert.Contains(t, errs.Error(), tt.errMsg)
			} else {
				assert.True(t, errs == nil || !errs.HasErrors(), "unexpected error: %v", errs)
			}
		})
	}
}

func TestConditionValuesAlias(t *testing.T) {
	// Test that both "value" and "values" YAML keys are supported
	tests := []struct {
		name     string
		expected interface{}
		yaml     string
	}{
		{
			name: "value with single item",
			yaml: `
field: status
operator: equals
value: "Ready"
`,
			expected: "Ready",
		},
		{
			name: "value with list",
			yaml: `
field: status
operator: in
value:
  - "Ready"
  - "Running"
`,
			expected: []interface{}{"Ready", "Running"},
		},
		{
			name: "values with list (alias)",
			yaml: `
field: status
operator: in
values:
  - "Ready"
  - "Running"
`,
			expected: []interface{}{"Ready", "Running"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cond Condition
			err := yaml.Unmarshal([]byte(tt.yaml), &cond)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, cond.Value)
		})
	}
}

// TestConditionValueAndValuesError verifies that specifying both value and values is an error
func TestConditionValueAndValuesError(t *testing.T) {
	yamlContent := `
field: status
operator: in
value: "ignored"
values:
  - "Used"
`
	var cond Condition
	err := yaml.Unmarshal([]byte(yamlContent), &cond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "condition has both 'value' and 'values' keys")
}

// =============================================================================
// Transport Config Tests
// =============================================================================

func TestTransportConfigYAMLParsing(t *testing.T) {
	tests := []struct {
		name           string
		yaml           string
		wantClient     string
		wantTarget     string
		wantMaestroNil bool
		wantError      bool
	}{
		{
			name: "resource with kubernetes transport",
			yaml: `
resources:
  - name: "testResource"
    transport:
      client: "kubernetes"
    manifest:
      apiVersion: v1
      kind: Namespace
      metadata:
        name: "test-ns"
    discovery:
      by_name: "test-ns"
`,
			wantError:      false,
			wantClient:     "kubernetes",
			wantMaestroNil: true,
		},
		{
			name: "resource with maestro transport",
			yaml: `
resources:
  - name: "testResource"
    transport:
      client: "maestro"
      maestro:
        target_cluster: "cluster1"
        manifestWork:
          apiVersion: work.open-cluster-management.io/v1
          kind: ManifestWork
          metadata:
            name: "test-mw"
    discovery:
      by_name: "test-mw"
`,
			wantError:      false,
			wantClient:     "maestro",
			wantTarget:     "cluster1",
			wantMaestroNil: false,
		},
		{
			name: "resource with maestro transport and manifestWork ref",
			yaml: `
resources:
  - name: "testResource"
    transport:
      client: "maestro"
      maestro:
        target_cluster: "{{ .clusterName }}"
        manifestWork:
          ref: "/path/to/manifestwork.yaml"
    discovery:
      by_name: "test-mw"
`,
			wantError:      false,
			wantClient:     "maestro",
			wantTarget:     "{{ .clusterName }}",
			wantMaestroNil: false,
		},
		{
			name: "resource without transport (defaults to kubernetes)",
			yaml: `
resources:
  - name: "testResource"
    manifest:
      apiVersion: v1
      kind: Namespace
      metadata:
        name: "test-ns"
    discovery:
      by_name: "test-ns"
`,
			wantError:      false,
			wantClient:     "kubernetes",
			wantMaestroNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config AdapterTaskConfig
			err := yaml.Unmarshal([]byte(tt.yaml), &config)

			if tt.wantError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, config.Resources, 1)

			resource := config.Resources[0]
			assert.Equal(t, tt.wantClient, resource.GetTransportClient())

			if tt.wantMaestroNil {
				if resource.Transport != nil {
					assert.Nil(t, resource.Transport.Maestro)
				}
			} else {
				require.NotNil(t, resource.Transport)
				require.NotNil(t, resource.Transport.Maestro)
				assert.Equal(t, tt.wantTarget, resource.Transport.Maestro.TargetCluster)
			}
		})
	}
}

func TestGetTransportClient(t *testing.T) {
	tests := []struct {
		name     string
		want     string
		resource Resource
	}{
		{
			name:     "nil transport defaults to kubernetes",
			resource: Resource{Name: "test"},
			want:     TransportClientKubernetes,
		},
		{
			name:     "empty client defaults to kubernetes",
			resource: Resource{Name: "test", Transport: &TransportConfig{Client: ""}},
			want:     TransportClientKubernetes,
		},
		{
			name:     "explicit kubernetes",
			resource: Resource{Name: "test", Transport: &TransportConfig{Client: "kubernetes"}},
			want:     TransportClientKubernetes,
		},
		{
			name:     "explicit maestro",
			resource: Resource{Name: "test", Transport: &TransportConfig{Client: "maestro"}},
			want:     TransportClientMaestro,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.resource.GetTransportClient())
		})
	}
}

func TestIsMaestroTransport(t *testing.T) {
	tests := []struct {
		name     string
		resource Resource
		want     bool
	}{
		{
			name:     "nil transport is not maestro",
			resource: Resource{Name: "test"},
			want:     false,
		},
		{
			name:     "kubernetes transport is not maestro",
			resource: Resource{Name: "test", Transport: &TransportConfig{Client: "kubernetes"}},
			want:     false,
		},
		{
			name:     "maestro transport",
			resource: Resource{Name: "test", Transport: &TransportConfig{Client: "maestro"}},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.resource.IsMaestroTransport())
		})
	}
}

func TestLoadConfigWithManifestWorkRef(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a manifestWork template file
	manifestWorkFile := filepath.Join(tmpDir, "manifestwork.yaml")
	require.NoError(t, os.WriteFile(manifestWorkFile, []byte(`
apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  name: "test-manifestwork"
spec:
  workload:
    manifests: []
`), 0644))

	adapterYAML := testAdapterConfigYAML

	taskYAML := `
params:
  - name: "clusterName"
    source: "event.name"
resources:
  - name: "testManifestWork"
    transport:
      client: "maestro"
      maestro:
        target_cluster: "{{ .clusterName }}"
    manifest:
      ref: "manifestwork.yaml"
    discovery:
      by_selectors:
        label_selector:
          app: "test"
`

	adapterPath, taskPath := createTestConfigFiles(t, tmpDir, adapterYAML, taskYAML)

	config, err := LoadConfig(
		WithAdapterConfigPath(adapterPath),
		WithTaskConfigPath(taskPath),
		WithSkipSemanticValidation(),
	)
	require.NoError(t, err)
	require.NotNil(t, config)

	// Verify manifest ref was loaded as raw string (for Go template support)
	require.Len(t, config.Resources, 1)
	resource := config.Resources[0]

	mw, ok := resource.Manifest.(string)
	require.True(t, ok, "Manifest should be a raw string after loading ref")
	assert.Contains(t, mw, "apiVersion: work.open-cluster-management.io/v1")
	assert.Contains(t, mw, "kind: ManifestWork")
}

func TestLoadConfigWithManifestWorkRefNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	adapterYAML := testAdapterConfigYAML

	taskYAML := `
resources:
  - name: "testManifestWork"
    transport:
      client: "maestro"
      maestro:
        target_cluster: "cluster1"
    manifest:
      ref: "nonexistent-manifestwork.yaml"
    discovery:
      by_selectors:
        label_selector:
          app: "test"
`

	adapterPath, taskPath := createTestConfigFiles(t, tmpDir, adapterYAML, taskYAML)

	config, err := LoadConfig(
		WithAdapterConfigPath(adapterPath),
		WithTaskConfigPath(taskPath),
		WithSkipSemanticValidation(),
	)
	require.Error(t, err)
	assert.Nil(t, config)
	assert.Contains(t, err.Error(), "does not exist")
}

func TestLoadConfigWithInlineManifestWork(t *testing.T) {
	tmpDir := t.TempDir()

	adapterYAML := testAdapterConfigYAML

	taskYAML := `
params:
  - name: "clusterName"
    source: "event.name"
resources:
  - name: "testManifestWork"
    transport:
      client: "maestro"
      maestro:
        target_cluster: "{{ .clusterName }}"
    manifest:
      apiVersion: work.open-cluster-management.io/v1
      kind: ManifestWork
      metadata:
        name: "inline-mw"
      spec:
        workload:
          manifests: []
    discovery:
      by_selectors:
        label_selector:
          app: "test"
`

	adapterPath, taskPath := createTestConfigFiles(t, tmpDir, adapterYAML, taskYAML)

	config, err := LoadConfig(
		WithAdapterConfigPath(adapterPath),
		WithTaskConfigPath(taskPath),
		WithSkipSemanticValidation(),
	)
	require.NoError(t, err)
	require.NotNil(t, config)

	// Verify inline manifest (ManifestWork) is preserved as-is
	require.Len(t, config.Resources, 1)
	resource := config.Resources[0]

	mw, ok := resource.Manifest.(map[string]interface{})
	require.True(t, ok, "Manifest should be a map")
	assert.Equal(t, "work.open-cluster-management.io/v1", mw["apiVersion"])
	assert.Equal(t, "ManifestWork", mw["kind"])
}

func TestLoadConfigWithManifestRefGoTemplates(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a manifest file with structural Go templates ({{ if }}, {{ range }})
	manifestFile := filepath.Join(tmpDir, "manifest-with-templates.yaml")
	require.NoError(t, os.WriteFile(manifestFile, []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: "{{ .name }}"
  namespace: "{{ .namespace }}"
{{ if .addLabels }}
  labels:
    app: "{{ .appName }}"
{{ end }}
data:
  key: value
`), 0644))

	adapterYAML := testAdapterConfigYAML
	adapterPath := filepath.Join(tmpDir, "adapter-config.yaml")
	require.NoError(t, os.WriteFile(adapterPath, []byte(adapterYAML), 0644))

	taskYAML := `
params:
  - name: "name"
    source: "event.name"
resources:
  - name: "configMap"
    manifest:
      ref: "manifest-with-templates.yaml"
    discovery:
      namespace: "*"
      by_name: "test"
`
	taskPath := filepath.Join(tmpDir, "task-config.yaml")
	require.NoError(t, os.WriteFile(taskPath, []byte(taskYAML), 0644))

	config, err := LoadConfig(
		WithAdapterConfigPath(adapterPath),
		WithTaskConfigPath(taskPath),
		WithSkipSemanticValidation(),
	)
	require.NoError(t, err)
	require.NotNil(t, config)

	// Manifest ref with Go templates should be stored as raw string
	require.Len(t, config.Resources, 1)
	manifestStr, ok := config.Resources[0].Manifest.(string)
	require.True(t, ok, "Manifest should be a raw string")
	assert.Contains(t, manifestStr, "{{ if .addLabels }}")
	assert.Contains(t, manifestStr, "{{ .name }}")
	assert.Contains(t, manifestStr, "{{ end }}")
}

func TestLoadConfigWithInlineStringManifest(t *testing.T) {
	tmpDir := t.TempDir()

	adapterYAML := testAdapterConfigYAML
	adapterPath := filepath.Join(tmpDir, "adapter-config.yaml")
	require.NoError(t, os.WriteFile(adapterPath, []byte(adapterYAML), 0644))

	// Inline manifest using YAML multiline string syntax for Go templates
	taskYAML := `
params:
  - name: "name"
    source: "event.name"
resources:
  - name: "configMap"
    manifest: |
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: "{{ .name }}"
      {{ if .addLabels }}
        labels:
          app: "{{ .appName }}"
      {{ end }}
    discovery:
      namespace: "*"
      by_name: "test"
`
	taskPath := filepath.Join(tmpDir, "task-config.yaml")
	require.NoError(t, os.WriteFile(taskPath, []byte(taskYAML), 0644))

	config, err := LoadConfig(
		WithAdapterConfigPath(adapterPath),
		WithTaskConfigPath(taskPath),
		WithSkipSemanticValidation(),
	)
	require.NoError(t, err)
	require.NotNil(t, config)

	// Inline manifest with YAML multiline string should be stored as string
	require.Len(t, config.Resources, 1)
	manifestStr, ok := config.Resources[0].Manifest.(string)
	require.True(t, ok, "Inline manifest with multiline string should be stored as string")
	assert.Contains(t, manifestStr, "{{ if .addLabels }}")
	assert.Contains(t, manifestStr, "{{ .name }}")
}

func TestLoadRawFile(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("loads file as raw string preserving template syntax", func(t *testing.T) {
		content := `apiVersion: v1
kind: ConfigMap
{{ if .condition }}
metadata:
  name: "{{ .name }}"
{{ end }}`
		filePath := filepath.Join(tmpDir, "template.yaml")
		require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))

		result, err := loadRawFile(tmpDir, "template.yaml")
		require.NoError(t, err)
		assert.Equal(t, content, result)
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := loadRawFile(tmpDir, "nonexistent.yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read file")
	})

	t.Run("path traversal blocked", func(t *testing.T) {
		_, err := loadRawFile(tmpDir, "../../../etc/passwd")
		require.Error(t, err)
	})
}

func TestAccessorsWithStringManifest(t *testing.T) {
	t.Run("HasManifestRef returns false for string manifest", func(t *testing.T) {
		r := &Resource{
			Name:     "test",
			Manifest: "apiVersion: v1\nkind: ConfigMap",
		}
		assert.False(t, r.HasManifestRef())
	})

	t.Run("GetManifestRef returns empty for string manifest", func(t *testing.T) {
		r := &Resource{
			Name:     "test",
			Manifest: "apiVersion: v1\nkind: ConfigMap",
		}
		assert.Equal(t, "", r.GetManifestRef())
	})

	t.Run("UnmarshalManifest returns error for string manifest", func(t *testing.T) {
		r := &Resource{
			Name:     "test",
			Manifest: "apiVersion: v1\nkind: ConfigMap",
		}
		result, err := r.UnmarshalManifest()
		assert.Nil(t, result)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "manifest is not a map")
	})

	t.Run("HasManifestRef still works for map manifest", func(t *testing.T) {
		r := &Resource{
			Name:     "test",
			Manifest: map[string]interface{}{"ref": "templates/test.yaml"},
		}
		assert.True(t, r.HasManifestRef())
		assert.Equal(t, "templates/test.yaml", r.GetManifestRef())
	})
}

func TestLoadConfigManifestRefPlainYAML(t *testing.T) {
	// Verify backward compatibility: ref files with plain YAML (no templates)
	// still work correctly when loaded as raw strings
	tmpDir := t.TempDir()

	manifestFile := filepath.Join(tmpDir, "plain-manifest.yaml")
	require.NoError(t, os.WriteFile(manifestFile, []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: "my-config"
  namespace: "default"
data:
  key: value
`), 0644))

	adapterYAML := testAdapterConfigYAML
	adapterPath := filepath.Join(tmpDir, "adapter-config.yaml")
	require.NoError(t, os.WriteFile(adapterPath, []byte(adapterYAML), 0644))

	taskYAML := `
params:
  - name: "clusterId"
    source: "event.id"
resources:
  - name: "configMap"
    manifest:
      ref: "plain-manifest.yaml"
    discovery:
      namespace: "*"
      by_name: "my-config"
`
	taskPath := filepath.Join(tmpDir, "task-config.yaml")
	require.NoError(t, os.WriteFile(taskPath, []byte(taskYAML), 0644))

	config, err := LoadConfig(
		WithAdapterConfigPath(adapterPath),
		WithTaskConfigPath(taskPath),
		WithSkipSemanticValidation(),
	)
	require.NoError(t, err)
	require.NotNil(t, config)

	// Plain YAML ref is now stored as string (not map)
	require.Len(t, config.Resources, 1)
	manifestStr, ok := config.Resources[0].Manifest.(string)
	require.True(t, ok, "Manifest ref should be stored as raw string")
	assert.Contains(t, manifestStr, "apiVersion: v1")
	assert.Contains(t, manifestStr, "name: \"my-config\"")
}

func TestParameterSourceUnmarshal(t *testing.T) {
	t.Run("string scalar source", func(t *testing.T) {
		var cfg AdapterTaskConfig
		err := yaml.Unmarshal([]byte(`
params:
  - name: clusterId
    source: "event.id"
`), &cfg)
		require.NoError(t, err)
		require.Len(t, cfg.Params, 1)
		p := cfg.Params[0]
		assert.True(t, p.Source.IsString())
		assert.Equal(t, "event.id", p.Source.StringVal)
		assert.False(t, p.Source.IsAPICall())
		assert.False(t, p.Source.IsExpression())
	})

	t.Run("api_call mapping source", func(t *testing.T) {
		var cfg AdapterTaskConfig
		err := yaml.Unmarshal([]byte(`
params:
  - name: clusterData
    source:
      api_call:
        method: GET
        url: /clusters/abc
        timeout: 10s
`), &cfg)
		require.NoError(t, err)
		require.Len(t, cfg.Params, 1)
		p := cfg.Params[0]
		assert.True(t, p.Source.IsAPICall())
		require.NotNil(t, p.Source.APICall)
		assert.Equal(t, "GET", p.Source.APICall.Method)
		assert.Equal(t, "/clusters/abc", p.Source.APICall.URL)
		assert.False(t, p.Source.IsString())
		assert.False(t, p.Source.IsExpression())
	})

	t.Run("expression mapping source", func(t *testing.T) {
		var cfg AdapterTaskConfig
		err := yaml.Unmarshal([]byte(`
params:
  - name: isActive
    source:
      expression: "clusterData.status == \"Active\""
`), &cfg)
		require.NoError(t, err)
		require.Len(t, cfg.Params, 1)
		p := cfg.Params[0]
		assert.True(t, p.Source.IsExpression())
		assert.Contains(t, p.Source.Expression, "clusterData.status")
		assert.False(t, p.Source.IsString())
		assert.False(t, p.Source.IsAPICall())
	})

	t.Run("both api_call and expression in mapping is an error", func(t *testing.T) {
		var cfg AdapterTaskConfig
		err := yaml.Unmarshal([]byte(`
params:
  - name: bad
    source:
      api_call:
        method: GET
        url: /clusters
      expression: "true"
`), &cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mutually exclusive")
	})

	t.Run("mapping with neither api_call nor expression is an error", func(t *testing.T) {
		var cfg AdapterTaskConfig
		err := yaml.Unmarshal([]byte(`
params:
  - name: bad
    source:
      unknown_key: value
`), &cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mapping must contain either")
	})
}
