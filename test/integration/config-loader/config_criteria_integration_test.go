//go:build integration

package configloaderintegration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/configloader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/criteria"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
)

// TestMain sets up environment variables required by the adapter config template
func TestMain(m *testing.M) {
	// Set required environment variables for tests
	os.Setenv("HYPERFLEET_API_BASE_URL", "http://test-api.example.com")
	os.Setenv("HYPERFLEET_API_TOKEN", "test-token-for-integration-tests")
	os.Exit(m.Run())
}

// getAdapterConfigPath returns the path to the adapter config file.
func getAdapterConfigPath() string {
	if envPath := os.Getenv("ADAPTER_CONFIG_PATH"); envPath != "" {
		return envPath
	}
	return filepath.Join(getProjectRoot(), "test/testdata/adapter-config.yaml")
}

// getTaskConfigPath returns the path to the task config file.
func getTaskConfigPath() string {
	if envPath := os.Getenv("TASK_CONFIG_PATH"); envPath != "" {
		return envPath
	}
	return filepath.Join(getProjectRoot(), "test/testdata/task-config.yaml")
}

// loadTestConfig loads split adapter and task configs and returns the merged Config.
func loadTestConfig(t *testing.T) *configloader.Config {
	t.Helper()
	adapterConfigPath := getAdapterConfigPath()
	taskConfigPath := getTaskConfigPath()

	config, err := configloader.LoadConfig(
		configloader.WithAdapterConfigPath(adapterConfigPath),
		configloader.WithTaskConfigPath(taskConfigPath),
		configloader.WithSkipSemanticValidation(),
	)
	require.NoError(t, err, "should load split config files from %s and %s", adapterConfigPath, taskConfigPath)
	require.NotNil(t, config)
	return config
}

// TestConfigLoadAndCriteriaEvaluation tests loading config and evaluating preconditions
func TestConfigLoadAndCriteriaEvaluation(t *testing.T) {
	// Load split config files (adapter + task) into unified Config
	config := loadTestConfig(t)

	// Create evaluation context with simulated runtime data
	ctx := criteria.NewEvaluationContext()

	// Simulate data extracted from HyperFleet API response
	// NOTE: reconciledConditionStatus must match the condition in the template (True)
	ctx.Set("reconciledConditionStatus", "False")
	ctx.Set("cloudProvider", "aws")
	ctx.Set("vpcId", "vpc-12345")
	ctx.Set("region", "us-east-1")
	ctx.Set("clusterName", "test-cluster")
	ctx.Set("nodeCount", 3)

	// Simulate cluster details response
	ctx.Set("clusterDetails", map[string]interface{}{
		"id":   "test-cluster-id",
		"name": "test-cluster",
		"kind": "Cluster",
		"spec": map[string]interface{}{
			"provider":   "aws",
			"region":     "us-east-1",
			"vpc_id":     "vpc-12345",
			"node_count": 3,
		},
		"status": map[string]interface{}{
			"conditions": []map[string]interface{}{
				{
					"type":   "Reconciled",
					"status": "True",
				},
			},
		},
	})

	evaluator, err := criteria.NewEvaluator(context.Background(), ctx, logger.NewTestLogger())
	require.NoError(t, err)

	t.Run("evaluate precondition conditions from config", func(t *testing.T) {
		// Find the clusterStatus precondition
		precond := config.GetPreconditionByName("clusterStatus")
		require.NotNil(t, precond, "clusterStatus precondition should exist")

		// Evaluate each condition from the config
		for i, cond := range precond.Conditions {
			t.Logf("Evaluating condition %d: %s %s %v", i, cond.Field, cond.Operator, cond.Value)

			result, err := evaluator.EvaluateCondition(
				cond.Field,
				criteria.Operator(cond.Operator),
				cond.Value,
			)
			require.NoError(t, err, "condition %d should evaluate without error", i)
			assert.True(t, result.Matched, "condition %d should match: %s %s %v (got field value: %v)",
				i, cond.Field, cond.Operator, cond.Value, result.FieldValue)
		}
	})

	t.Run("evaluate all preconditions as combined expression", func(t *testing.T) {
		precond := config.GetPreconditionByName("clusterStatus")
		require.NotNil(t, precond)

		// Convert conditions to ConditionDef slice
		conditions := make([]criteria.ConditionDef, len(precond.Conditions))
		for i, cond := range precond.Conditions {
			conditions[i] = criteria.ConditionDef{
				Field:    cond.Field,
				Operator: criteria.Operator(cond.Operator),
				Value:    cond.Value,
			}
		}

		// Evaluate all conditions together
		result, err := evaluator.EvaluateConditions(conditions)
		require.NoError(t, err)
		assert.True(t, result.Matched, "all preconditions should pass")
		assert.Equal(t, -1, result.FailedCondition, "no condition should fail")

		// Verify extracted fields
		assert.NotEmpty(t, result.ExtractedFields)
		t.Logf("Extracted fields: %v", result.ExtractedFields)
	})
}

// TestConfigWithFailingPreconditions tests behavior when preconditions fail
func TestConfigWithFailingPreconditions(t *testing.T) {
	config := loadTestConfig(t)

	precond := config.GetPreconditionByName("clusterStatus")
	require.NotNil(t, precond)

	t.Run("preconditions fail with Reconciled condition True", func(t *testing.T) {
		ctx := criteria.NewEvaluationContext()
		ctx.Set("reconciledConditionStatus", "True")

		evaluator, err := criteria.NewEvaluator(context.Background(), ctx, logger.NewTestLogger())
		require.NoError(t, err)
		conditions := make([]criteria.ConditionDef, len(precond.Conditions))
		for i, cond := range precond.Conditions {
			conditions[i] = criteria.ConditionDef{
				Field:    cond.Field,
				Operator: criteria.Operator(cond.Operator),
				Value:    cond.Value,
			}
		}

		result, err := evaluator.EvaluateConditions(conditions)
		require.NoError(t, err)
		assert.False(t, result.Matched, "preconditions should fail with Reconciled condition True")
		assert.Equal(t, 0, result.FailedCondition, "first condition (reconciledConditionStatus) should fail")
	})

	t.Run("preconditions fail with Reconciled condition Unknown", func(t *testing.T) {
		ctx := criteria.NewEvaluationContext()
		ctx.Set("reconciledConditionStatus", "Unknown") // Not matching expected "True"

		evaluator, err := criteria.NewEvaluator(context.Background(), ctx, logger.NewTestLogger())
		require.NoError(t, err)
		conditions := make([]criteria.ConditionDef, len(precond.Conditions))
		for i, cond := range precond.Conditions {
			conditions[i] = criteria.ConditionDef{
				Field:    cond.Field,
				Operator: criteria.Operator(cond.Operator),
				Value:    cond.Value,
			}
		}

		result, err := evaluator.EvaluateConditions(conditions)
		require.NoError(t, err)
		assert.False(t, result.Matched, "preconditions should fail when Reconciled condition is Unknown")
	})

	t.Run("preconditions fail with missing vpcId", func(t *testing.T) {
		ctx := criteria.NewEvaluationContext()
		// vpcId not set - should fail exists check

		evaluator, err := criteria.NewEvaluator(context.Background(), ctx, logger.NewTestLogger())
		require.NoError(t, err)
		// Just check the vpcId exists condition (this is a general test, not tied to template)
		result, err := evaluator.EvaluateCondition("vpcId", criteria.OperatorExists, true)
		// Either error or not matched means exists check fails
		assert.True(t, err != nil || !result.Matched, "exists check should fail when field is missing")
	})
}

// TestConfigResourceDiscoveryFields tests extracting discovery fields from config
func TestConfigResourceDiscoveryFields(t *testing.T) {
	config := loadTestConfig(t)

	t.Run("verify resource discovery configs", func(t *testing.T) {
		for _, resource := range config.Resources {
			t.Logf("Resource: %s", resource.Name)

			if resource.Discovery != nil {
				t.Logf("  Discovery namespace: %s", resource.Discovery.Namespace)
				t.Logf("  Discovery byName: %s", resource.Discovery.ByName)
				if resource.Discovery.BySelectors != nil {
					t.Logf("  Discovery selectors: %v", resource.Discovery.BySelectors.LabelSelector)
				}

				// Verify discovery config has at least one method
				hasDiscoveryMethod := resource.Discovery.ByName != "" ||
					(resource.Discovery.BySelectors != nil && len(resource.Discovery.BySelectors.LabelSelector) > 0)
				assert.True(t, hasDiscoveryMethod, "resource %s should have discovery method", resource.Name)
			}
		}
	})
}

// TestConfigPostProcessingEvaluation tests evaluating post-processing conditions
func TestConfigPostProcessingEvaluation(t *testing.T) {
	config := loadTestConfig(t)

	require.NotNil(t, config.Post, "config should have post processing")

	t.Run("simulate post-processing with k8s resource data", func(t *testing.T) {
		ctx := criteria.NewEvaluationContext()

		// Simulate K8s resources returned from discovery
		ctx.Set("resources", map[string]interface{}{
			"clusterNamespace": map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]interface{}{
					"name": "cluster-abc123",
				},
				"status": map[string]interface{}{
					"phase": "Active",
				},
			},
			"clusterController": map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name":      "cluster-controller",
					"namespace": "cluster-abc123",
				},
				"status": map[string]interface{}{
					"replicas":          3,
					"readyReplicas":     3,
					"availableReplicas": 3,
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   "Available",
							"status": "True",
							"reason": "MinimumReplicasAvailable",
						},
					},
				},
			},
			"clusterConfigMap": map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "cluster-config",
					"namespace": "cluster-abc123",
				},
			},
		})

		evaluator, err := criteria.NewEvaluator(context.Background(), ctx, logger.NewTestLogger())
		require.NoError(t, err)
		// Test accessing nested K8s resource data
		t.Run("access namespace status", func(t *testing.T) {
			result, err := evaluator.ExtractValue("resources.clusterNamespace.status.phase", "")
			require.NoError(t, err)
			assert.Equal(t, "Active", result.Value)
		})

		t.Run("access deployment replicas", func(t *testing.T) {
			result, err := evaluator.ExtractValue("resources.clusterController.status.readyReplicas", "")
			require.NoError(t, err)
			assert.Equal(t, 3, result.Value)
		})

		t.Run("evaluate deployment ready condition", func(t *testing.T) {
			// Check if ready replicas > 0
			result, err := evaluator.EvaluateCondition(
				"resources.clusterController.status.readyReplicas",
				criteria.OperatorGreaterThan,
				0,
			)
			require.NoError(t, err)
			assert.True(t, result.Matched)
			assert.Equal(t, 3, result.FieldValue)
		})

		t.Run("evaluate replicas match", func(t *testing.T) {
			// Check replicas == readyReplicas
			replicasResult, err := evaluator.ExtractValue("resources.clusterController.status.replicas", "")
			require.NoError(t, err)
			readyReplicasResult, err := evaluator.ExtractValue("resources.clusterController.status.readyReplicas", "")
			require.NoError(t, err)
			assert.Equal(t, replicasResult.Value, readyReplicasResult.Value)
		})

		t.Run("evaluate with CEL expression", func(t *testing.T) {
			result, err := evaluator.EvaluateCEL(`resources.clusterController.status.readyReplicas > 0`)
			require.NoError(t, err)
			assert.True(t, result.Matched)
		})
	})
}

// TestConfigNullSafetyWithMissingResources tests null safety when resources are missing
func TestConfigNullSafetyWithMissingResources(t *testing.T) {
	config := loadTestConfig(t)
	// Verify config has resources defined (the actual resources are tested for null safety below)
	require.NotEmpty(t, config.Resources, "config should have resources defined")

	t.Run("handle missing resource gracefully", func(t *testing.T) {
		ctx := criteria.NewEvaluationContext()

		// Partially populated resources (simulating some not yet created)
		ctx.Set("resources", map[string]interface{}{
			"clusterNamespace": map[string]interface{}{
				"status": map[string]interface{}{
					"phase": "Active",
				},
			},
			"clusterController": nil, // Not created yet
		})

		evaluator, err := criteria.NewEvaluator(context.Background(), ctx, logger.NewTestLogger())
		require.NoError(t, err)

		// ExtractValue returns nil value (not error) for null path - allows default to be used
		result, extractErr := evaluator.ExtractValue("resources.clusterController.status.readyReplicas", "")
		assert.NoError(t, extractErr, "no parse error for valid path")
		assert.Nil(t, result.Value, "should return nil value for null resource path")

		// Evaluation should error or return false for null path
		condResult, err := evaluator.EvaluateCondition(
			"resources.clusterController.status.readyReplicas",
			criteria.OperatorGreaterThan,
			0,
		)
		assert.True(t, err != nil || !condResult.Matched, "should fail for null path")
	})

	t.Run("handle deeply nested null", func(t *testing.T) {
		ctx := criteria.NewEvaluationContext()
		ctx.Set("resources", map[string]interface{}{
			"clusterController": map[string]interface{}{
				"status": nil, // Status is null
			},
		})

		evaluator, err := criteria.NewEvaluator(context.Background(), ctx, logger.NewTestLogger())
		require.NoError(t, err)

		// Should return nil value (not error) for null status path
		result, err := evaluator.ExtractValue("resources.clusterController.status.readyReplicas", "")
		assert.NoError(t, err, "no parse error for valid path")
		assert.Nil(t, result.Value, "should return nil value for null path")
	})
}

// TestConfigParameterExtraction tests parameter definitions from config
func TestConfigParameterExtraction(t *testing.T) {
	config := loadTestConfig(t)

	t.Run("verify required parameters", func(t *testing.T) {
		requiredParams := config.GetRequiredParams()
		assert.NotEmpty(t, requiredParams, "should have required parameters")

		// Check expected required params exist
		requiredNames := make(map[string]bool)
		for _, p := range requiredParams {
			requiredNames[p.Name] = true
			t.Logf("Required param: %s (source: %s)", p.Name, p.Source)
		}

		assert.True(t, requiredNames["clusterId"], "clusterId should be required")
	})

	t.Run("verify parameter sources", func(t *testing.T) {
		for _, param := range config.Params {
			if param.Source != "" {
				// Check source format
				assert.True(t,
					strings.HasPrefix(param.Source, "env.") || strings.HasPrefix(param.Source, "event."),
					"param %s source should start with env. or event., got: %s", param.Name, param.Source)
			}
		}
	})
}
