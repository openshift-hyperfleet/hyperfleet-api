package configloaderintegration

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/configloader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleetapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getProjectRoot traverses upwards from the directory of the current file,
// checking each parent for a .git directory, returning the first match.
// This approach reliably finds the project root, even if the project name is repeated in the path.
func getProjectRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("runtime.Caller failed")
	}

	dir := filepath.Dir(filename)
	for {
		gitDir := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitDir); err == nil && (info.IsDir() || info.Mode().IsRegular()) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // reached root
		}
		dir = parent
	}
	panic("could not find project root: no .git directory found upwards from path: " + filename)
}

// TestLoadSplitConfig tests loading split adapter and task config files
func TestLoadSplitConfig(t *testing.T) {
	// Set required environment variables for the test config
	t.Setenv("HYPERFLEET_API_BASE_URL", "http://test-api.example.com")

	projectRoot := getProjectRoot()
	adapterConfigPath := filepath.Join(projectRoot, "test/testdata/adapter-config.yaml")
	taskConfigPath := filepath.Join(projectRoot, "test/testdata/task-config.yaml")

	config, err := configloader.LoadConfig(
		configloader.WithAdapterConfigPath(adapterConfigPath),
		configloader.WithTaskConfigPath(taskConfigPath),
		configloader.WithSkipSemanticValidation(),
	)
	require.NoError(t, err, "should be able to load split config files")
	require.NotNil(t, config)

	// Verify merged structure
	// Adapter info comes from adapter config
	assert.Equal(t, "test-adapter", config.Adapter.Name)
	assert.Equal(t, "0.1.0", config.Adapter.Version)

	// Clients config comes from adapter config
	assert.Equal(t, 2*time.Second, config.Clients.HyperfleetAPI.Timeout)
	assert.Equal(t, 3, config.Clients.HyperfleetAPI.RetryAttempts)
	assert.Equal(t, hyperfleetapi.BackoffExponential, config.Clients.HyperfleetAPI.RetryBackoff)

	// Verify params exist (from task config)
	assert.NotEmpty(t, config.Params)
	assert.GreaterOrEqual(t, len(config.Params), 1, "should have at least 1 parameter")

	// Check specific params (using accessor method)
	clusterIDParam := config.GetParamByName("clusterId")
	require.NotNil(t, clusterIDParam, "clusterId parameter should exist")
	assert.Equal(t, "event.id", clusterIDParam.Source)
	assert.True(t, clusterIDParam.Required)

	// Verify preconditions (from task config)
	assert.NotEmpty(t, config.Preconditions)
	assert.GreaterOrEqual(t, len(config.Preconditions), 1, "should have at least 1 precondition")

	// Check first precondition
	firstPrecond := config.Preconditions[0]
	assert.Equal(t, "clusterStatus", firstPrecond.Name)
	assert.NotNil(t, firstPrecond.APICall)
	assert.Equal(t, "GET", firstPrecond.APICall.Method)
	assert.NotEmpty(t, firstPrecond.Capture)
	assert.NotEmpty(t, firstPrecond.Conditions)

	// Verify captured fields
	clusterNameCapture := findCaptureByName(firstPrecond.Capture, "clusterName")
	require.NotNil(t, clusterNameCapture)
	assert.Equal(t, "name", clusterNameCapture.Field)

	// Verify conditions in precondition
	assert.GreaterOrEqual(t, len(firstPrecond.Conditions), 1)
	firstCondition := firstPrecond.Conditions[0]
	assert.Equal(t, "reconciledConditionStatus", firstCondition.Field)
	assert.Equal(t, "equals", firstCondition.Operator)

	// Verify resources (from task config)
	assert.NotEmpty(t, config.Resources)
	assert.GreaterOrEqual(t, len(config.Resources), 1, "should have at least 1 resource")

	// Check first resource
	firstResource := config.Resources[0]
	assert.Equal(t, "clusterNamespace", firstResource.Name)
	assert.NotNil(t, firstResource.Manifest)
	assert.NotNil(t, firstResource.Discovery)

	// Verify post configuration (from task config)
	if config.Post != nil {
		assert.NotEmpty(t, config.Post.Payloads)
		assert.NotEmpty(t, config.Post.PostActions)

		// Check post action
		if len(config.Post.PostActions) > 0 {
			firstAction := config.Post.PostActions[0]
			assert.NotEmpty(t, firstAction.Name)
			if firstAction.APICall != nil {
				assert.NotEmpty(t, firstAction.APICall.Method)
				assert.NotEmpty(t, firstAction.APICall.URL)
			}
		}
	}
}

// TestLoadSplitConfigWithResourceByName tests the GetResourceByName accessor on merged config
func TestLoadSplitConfigWithResourceByName(t *testing.T) {
	// Set required environment variables for the test config
	t.Setenv("HYPERFLEET_API_BASE_URL", "http://test-api.example.com")

	projectRoot := getProjectRoot()
	adapterConfigPath := filepath.Join(projectRoot, "test/testdata/adapter-config.yaml")
	taskConfigPath := filepath.Join(projectRoot, "test/testdata/task-config.yaml")

	config, err := configloader.LoadConfig(
		configloader.WithAdapterConfigPath(adapterConfigPath),
		configloader.WithTaskConfigPath(taskConfigPath),
		configloader.WithSkipSemanticValidation(),
	)
	require.NoError(t, err)
	require.NotNil(t, config)

	// Verify resource exists using accessor
	configMapResource := config.GetResourceByName("clusterConfigMap")
	require.NotNil(t, configMapResource, "clusterConfigMap resource should exist")
	assert.Equal(t, "clusterConfigMap", configMapResource.Name)
}

// Helper function to find a capture field by name
func findCaptureByName(captures []configloader.CaptureField, name string) *configloader.CaptureField {
	for i := range captures {
		if captures[i].Name == name {
			return &captures[i]
		}
	}
	return nil
}
