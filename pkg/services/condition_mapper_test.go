package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/cel-go/common/types"
	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/util"
)

// ============================================================================
// Tests from condition_mapper_check_test.go
// ============================================================================

func TestConditionMapper_CELCheckValidation(t *testing.T) {
	t.Run("undefined function caught at compile time", func(t *testing.T) {
		RegisterTestingT(t)

		rules := []registry.ConditionMappingRule{
			{Type: "Test",
				When: registry.MappingExpression{
					Expression: `undefinedFunction(statuses)`, // No such function
				},
				Output: registry.MappingOutput{
					Status:  registry.MappingExpression{Expression: `"True"`},
					Reason:  registry.MappingExpression{Expression: `"OK"`},
					Message: registry.MappingExpression{Expression: `"OK"`},
				},
			},
		}

		_, err := NewConditionMapper("Cluster", rules)

		// Should fail at compile time with check error
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("check error"))
		Expect(err.Error()).To(ContainSubstring("undefinedFunction"))
	})

	t.Run("wrong function arity caught at compile time", func(t *testing.T) {
		RegisterTestingT(t)

		rules := []registry.ConditionMappingRule{
			{Type: "Test",
				When: registry.MappingExpression{
					// size() expects 1 argument, not 2
					Expression: `size(statuses, 'extra arg')`,
				},
				Output: registry.MappingOutput{
					Status:  registry.MappingExpression{Expression: `"True"`},
					Reason:  registry.MappingExpression{Expression: `"OK"`},
					Message: registry.MappingExpression{Expression: `"OK"`},
				},
			},
		}

		_, err := NewConditionMapper("Cluster", rules)

		// Should fail at compile time with check error
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("check error"))
	})

	t.Run("valid CEL expression compiles successfully", func(t *testing.T) {
		RegisterTestingT(t)

		rules := []registry.ConditionMappingRule{
			{Type: "Test",
				When: registry.MappingExpression{
					Expression: `size(statuses) > 0`, // Valid
				},
				Output: registry.MappingOutput{
					Status:  registry.MappingExpression{Expression: `"True"`},
					Reason:  registry.MappingExpression{Expression: `"OK"`},
					Message: registry.MappingExpression{Expression: `"OK"`},
				},
			},
		}

		mapper, err := NewConditionMapper("Cluster", rules)

		Expect(err).NotTo(HaveOccurred())
		Expect(mapper).NotTo(BeNil())
	})

	t.Run("Check detects error before Program in all expressions", func(t *testing.T) {
		RegisterTestingT(t)

		// Test that Check runs for all 4 expression types (when, status, reason, message)
		testCases := []struct {
			name        string
			badField    string
			badExpr     string
			goodWhen    string
			goodStatus  string
			goodReason  string
			goodMessage string
		}{
			{
				name:        "undefined in when",
				badField:    "when",
				badExpr:     `noSuchFunc()`,
				goodStatus:  `"True"`,
				goodReason:  `"OK"`,
				goodMessage: `"OK"`,
			},
			{
				name:        "undefined in status",
				badField:    "status",
				badExpr:     `noSuchFunc()`,
				goodWhen:    `true`,
				goodReason:  `"OK"`,
				goodMessage: `"OK"`,
			},
			{
				name:        "undefined in reason",
				badField:    "reason",
				badExpr:     `noSuchFunc()`,
				goodWhen:    `true`,
				goodStatus:  `"True"`,
				goodMessage: `"OK"`,
			},
			{
				name:       "undefined in message",
				badField:   "message",
				badExpr:    `noSuchFunc()`,
				goodWhen:   `true`,
				goodStatus: `"True"`,
				goodReason: `"OK"`,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				RegisterTestingT(t)

				// Build rule with bad expression in the specified field
				whenExpr := tc.goodWhen
				if tc.badField == "when" {
					whenExpr = tc.badExpr
				}

				statusExpr := tc.goodStatus
				if tc.badField == "status" {
					statusExpr = tc.badExpr
				}

				reasonExpr := tc.goodReason
				if tc.badField == "reason" {
					reasonExpr = tc.badExpr
				}

				messageExpr := tc.goodMessage
				if tc.badField == "message" {
					messageExpr = tc.badExpr
				}

				rules := []registry.ConditionMappingRule{
					{Type: "Test",
						When: registry.MappingExpression{Expression: whenExpr},
						Output: registry.MappingOutput{
							Status:  registry.MappingExpression{Expression: statusExpr},
							Reason:  registry.MappingExpression{Expression: reasonExpr},
							Message: registry.MappingExpression{Expression: messageExpr},
						},
					},
				}

				_, err := NewConditionMapper("Cluster", rules)

				// Should fail at compile time
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("check error"))
				Expect(err.Error()).To(ContainSubstring(tc.badField)) // Error mentions which field
			})
		}
	})
}

// ============================================================================
// Tests from condition_mapper_masking_test.go
// ============================================================================

func TestConditionMapper_SensitiveDataMasking(t *testing.T) {
	t.Run("sensitive adapter data is masked in CEL context", func(t *testing.T) {
		RegisterTestingT(t)

		// Rule that tries to extract adapter data fields
		rules := []registry.ConditionMappingRule{
			{Type: "DataExtract",
				When: registry.MappingExpression{Expression: `statuses.exists(s, s.adapter == "test")`},
				Output: registry.MappingOutput{
					Status: registry.MappingExpression{Expression: `"True"`},
					Reason: registry.MappingExpression{Expression: `"OK"`},
					// Try to extract sensitive field from data
					Message: registry.MappingExpression{
						Expression: `statuses.filter(s, s.adapter == "test")[0].data.adminPassword`,
					},
				},
			},
		}

		mapper, err := NewConditionMapper("Cluster", rules)
		Expect(err).NotTo(HaveOccurred())

		// Adapter status with sensitive data
		sensitiveData := map[string]interface{}{
			"clusterName":   "prod-cluster",
			"adminPassword": "super-secret-password-123", // Should be masked
		}
		dataJSON, _ := json.Marshal(sensitiveData) // static test data, marshal cannot fail

		statuses := api.AdapterStatusList{
			{
				Adapter:    "test",
				Conditions: []byte(`[]`),
				Data:       dataJSON,
			},
		}

		result, err := mapper.Apply(context.Background(), ApplyInput{
			AdapterStatuses: statuses,
			Resource:        map[string]interface{}{},
			RefTime:         time.Now(),
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(result).To(HaveLen(1))
		cond := result[0]

		// The CEL expression should get the masked value, not the real password
		Expect(*cond.Message).To(Equal("***REDACTED***"))
		Expect(*cond.Message).NotTo(ContainSubstring("super-secret-password-123"))
	})

	t.Run("non-sensitive adapter data passes through", func(t *testing.T) {
		RegisterTestingT(t)

		rules := []registry.ConditionMappingRule{
			{Type: "DataExtract",
				When: registry.MappingExpression{Expression: `statuses.exists(s, s.adapter == "test")`},
				Output: registry.MappingOutput{
					Status: registry.MappingExpression{Expression: `"True"`},
					Reason: registry.MappingExpression{Expression: `"OK"`},
					// Extract non-sensitive field
					Message: registry.MappingExpression{
						Expression: `statuses.filter(s, s.adapter == "test")[0].data.clusterName`,
					},
				},
			},
		}

		mapper, err := NewConditionMapper("Cluster", rules)
		Expect(err).NotTo(HaveOccurred())

		// Adapter status with non-sensitive data
		data := map[string]interface{}{
			"clusterName": "prod-cluster-1",
			"region":      "us-west-2",
		}
		dataJSON, _ := json.Marshal(data) // static test data, marshal cannot fail

		statuses := api.AdapterStatusList{
			{
				Adapter:    "test",
				Conditions: []byte(`[]`),
				Data:       dataJSON,
			},
		}

		result, err := mapper.Apply(context.Background(), ApplyInput{
			AdapterStatuses: statuses,
			Resource:        map[string]interface{}{},
			RefTime:         time.Now(),
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(result).To(HaveLen(1))
		cond := result[0]

		// Non-sensitive data should pass through
		Expect(*cond.Message).To(Equal("prod-cluster-1"))
	})

	t.Run("nested sensitive data is masked", func(t *testing.T) {
		RegisterTestingT(t)

		rules := []registry.ConditionMappingRule{
			{Type: "DataExtract",
				When: registry.MappingExpression{Expression: `statuses.exists(s, s.adapter == "test")`},
				Output: registry.MappingOutput{
					Status: registry.MappingExpression{Expression: `"True"`},
					Reason: registry.MappingExpression{Expression: `"OK"`},
					// Try to extract nested sensitive field
					Message: registry.MappingExpression{
						Expression: `statuses.filter(s, s.adapter == "test")[0].data.serviceAccount.privateKey`,
					},
				},
			},
		}

		mapper, err := NewConditionMapper("Cluster", rules)
		Expect(err).NotTo(HaveOccurred())

		// Adapter status with nested sensitive data
		data := map[string]interface{}{
			"clusterName": "prod-cluster",
			"serviceAccount": map[string]interface{}{
				"name":       "sa-123",
				"privateKey": "-----BEGIN PRIVATE KEY-----\n...", // Should be masked
			},
		}
		dataJSON, _ := json.Marshal(data) // static test data, marshal cannot fail

		statuses := api.AdapterStatusList{
			{
				Adapter:    "test",
				Conditions: []byte(`[]`),
				Data:       dataJSON,
			},
		}

		result, err := mapper.Apply(context.Background(), ApplyInput{
			AdapterStatuses: statuses,
			Resource:        map[string]interface{}{},
			RefTime:         time.Now(),
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(result).To(HaveLen(1))
		cond := result[0]

		// Nested sensitive field should be masked
		Expect(*cond.Message).To(Equal("***REDACTED***"))
		Expect(*cond.Message).NotTo(ContainSubstring("BEGIN PRIVATE KEY"))
	})

	t.Run("multiple adapters with mixed sensitive and non-sensitive data", func(t *testing.T) {
		RegisterTestingT(t)

		rules := []registry.ConditionMappingRule{
			{Type: "Summary",
				When: registry.MappingExpression{Expression: `size(statuses) > 0`},
				Output: registry.MappingOutput{
					Status: registry.MappingExpression{Expression: `"True"`},
					Reason: registry.MappingExpression{Expression: `"OK"`},
					// Build message from multiple adapters
					Message: registry.MappingExpression{
						Expression: `"Cluster: " + statuses[0].data.name + ", Secret: " + statuses[1].data.pullSecret`,
					},
				},
			},
		}

		mapper, err := NewConditionMapper("Cluster", rules)
		Expect(err).NotTo(HaveOccurred())

		// First adapter: non-sensitive
		data1 := map[string]interface{}{
			"name": "my-cluster",
		}
		data1JSON, _ := json.Marshal(data1) // static test data, marshal cannot fail

		// Second adapter: sensitive
		data2 := map[string]interface{}{
			"pullSecret": "test-secret-value-not-real-auth-json",
		}
		data2JSON, _ := json.Marshal(data2) // static test data, marshal cannot fail

		statuses := api.AdapterStatusList{
			{
				Adapter:    "adapter1",
				Conditions: []byte(`[]`),
				Data:       data1JSON,
			},
			{
				Adapter:    "adapter2",
				Conditions: []byte(`[]`),
				Data:       data2JSON,
			},
		}

		result, err := mapper.Apply(context.Background(), ApplyInput{
			AdapterStatuses: statuses,
			Resource:        map[string]interface{}{},
			RefTime:         time.Now(),
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(result).To(HaveLen(1))
		cond := result[0]

		// Message should have non-sensitive data but masked sensitive data
		Expect(*cond.Message).To(Equal("Cluster: my-cluster, Secret: ***REDACTED***"))
		Expect(*cond.Message).NotTo(ContainSubstring("test-secret"))
	})

	t.Run("arrays with sensitive fields are masked in CEL context", func(t *testing.T) {
		RegisterTestingT(t)

		rules := []registry.ConditionMappingRule{
			{Type: "UserPassword",
				When: registry.MappingExpression{Expression: `statuses.exists(s, s.adapter == "test")`},
				Output: registry.MappingOutput{
					Status: registry.MappingExpression{Expression: `"True"`},
					Reason: registry.MappingExpression{Expression: `"OK"`},
					// Try to extract password from users array
					Message: registry.MappingExpression{
						Expression: `statuses.filter(s, s.adapter == "test")[0].data.users[0].password`,
					},
				},
			},
		}

		mapper, err := NewConditionMapper("Cluster", rules)
		Expect(err).NotTo(HaveOccurred())

		// Adapter with array containing sensitive fields
		data := map[string]interface{}{
			"clusterName": "test",
			"users": []interface{}{
				map[string]interface{}{
					"name":     "admin",
					"password": "super-secret-password-123", // Should be masked
				},
			},
		}
		dataJSON, _ := json.Marshal(data) // static test data, marshal cannot fail

		statuses := api.AdapterStatusList{
			{
				Adapter:    "test",
				Conditions: []byte(`[]`),
				Data:       dataJSON,
			},
		}

		result, err := mapper.Apply(context.Background(), ApplyInput{
			AdapterStatuses: statuses,
			Resource:        map[string]interface{}{},
			RefTime:         time.Now(),
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(result).To(HaveLen(1))
		cond := result[0]

		// CEL should receive masked value from array element
		Expect(*cond.Message).To(Equal("***REDACTED***"))
		Expect(*cond.Message).NotTo(ContainSubstring("super-secret-password-123"))
	})

	t.Run("sensitive resource fields are masked in CEL context (defense-in-depth)", func(t *testing.T) {
		RegisterTestingT(t)

		rules := []registry.ConditionMappingRule{
			{Type: "ResourceSecret",
				When: registry.MappingExpression{Expression: `true`},
				Output: registry.MappingOutput{
					Status: registry.MappingExpression{Expression: `"True"`},
					Reason: registry.MappingExpression{Expression: `"OK"`},
					// Try to extract sensitive field from resource
					Message: registry.MappingExpression{
						Expression: `"Admin password: " + resource.spec.adminPassword`,
					},
				},
			},
		}

		mapper, err := NewConditionMapper("Cluster", rules)
		Expect(err).NotTo(HaveOccurred())

		// Resource with sensitive field in spec
		resource := map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "test-cluster",
			},
			"spec": map[string]interface{}{
				"region":        "us-west-2",
				"adminPassword": "cluster-admin-secret-123", // Should be masked
			},
		}

		result, err := mapper.Apply(context.Background(), ApplyInput{
			AdapterStatuses: api.AdapterStatusList{},
			Resource:        resource,
			RefTime:         time.Now(),
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(result).To(HaveLen(1))
		cond := result[0]

		// CEL should receive masked value from resource
		Expect(*cond.Message).To(Equal("Admin password: ***REDACTED***"))
		Expect(*cond.Message).NotTo(ContainSubstring("cluster-admin-secret-123"))
	})

	t.Run("nested sensitive fields in resource arrays are masked", func(t *testing.T) {
		RegisterTestingT(t)

		rules := []registry.ConditionMappingRule{
			{Type: "NodeToken",
				When: registry.MappingExpression{Expression: `size(resource.spec.nodes) > 0`},
				Output: registry.MappingOutput{
					Status: registry.MappingExpression{Expression: `"True"`},
					Reason: registry.MappingExpression{Expression: `"OK"`},
					// Try to extract token from nodes array
					Message: registry.MappingExpression{
						Expression: `"Node token: " + resource.spec.nodes[0].authToken`,
					},
				},
			},
		}

		mapper, err := NewConditionMapper("Cluster", rules)
		Expect(err).NotTo(HaveOccurred())

		// Resource with sensitive fields in nested arrays
		resource := map[string]interface{}{
			"spec": map[string]interface{}{
				"nodes": []interface{}{
					map[string]interface{}{
						"name":      "node-1",
						"authToken": "secret-node-token-abc123", // Should be masked
					},
				},
			},
		}

		result, err := mapper.Apply(context.Background(), ApplyInput{
			AdapterStatuses: api.AdapterStatusList{},
			Resource:        resource,
			RefTime:         time.Now(),
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(result).To(HaveLen(1))
		cond := result[0]

		// CEL should receive masked value from nested array
		Expect(*cond.Message).To(Equal("Node token: ***REDACTED***"))
		Expect(*cond.Message).NotTo(ContainSubstring("secret-node-token-abc123"))
	})
}

// ============================================================================
// Tests from condition_mapper_nil_guard_test.go
// ============================================================================

func TestAdapterStatusToMapWithUnknownCheck_NilGuard(t *testing.T) {
	t.Run("nil status pointer returns safe empty map", func(t *testing.T) {
		RegisterTestingT(t)

		statusMap, hasUnknown := adapterStatusToMapWithUnknownCheck(context.Background(), nil)

		// Should not panic and return safe defaults
		Expect(hasUnknown).To(BeFalse())
		Expect(statusMap).NotTo(BeNil())
		Expect(statusMap[celKeyAdapter]).To(Equal(""))
		Expect(statusMap[celKeyObservedGeneration]).To(Equal(float64(0)))
		Expect(statusMap[celKeyConditions]).To(Equal([]map[string]interface{}{}))
		Expect(statusMap[celKeyData]).To(Equal(map[string]interface{}{}))
	})

	t.Run("buildActivation handles nil element in AdapterStatusList gracefully", func(t *testing.T) {
		RegisterTestingT(t)

		// AdapterStatusList with nil element
		statuses := api.AdapterStatusList{
			nil, // Could happen if DAO has a bug
			{
				Adapter:    "test-adapter",
				Conditions: []byte(`[]`),
			},
		}

		// Should not panic
		activation := testBuildActivation(t, context.Background(), statuses, map[string]interface{}{}, "Cluster")

		statusesList := activation[util.CELVarStatuses].([]interface{})
		// Nil element should be skipped, only valid adapter present
		Expect(statusesList).To(HaveLen(1))

		// Only element is the valid adapter (nil was skipped)
		first := statusesList[0].(map[string]interface{})
		Expect(first[celKeyAdapter]).To(Equal("test-adapter"))
	})
}

// ============================================================================
// Tests for Unknown condition filtering
// ============================================================================

func TestConditionMapper_UnknownConditionFiltering(t *testing.T) {
	t.Run("adapter with Unknown condition excluded from statuses", func(t *testing.T) {
		RegisterTestingT(t)

		rules := []registry.ConditionMappingRule{
			{
				Type: "TestCondition",
				When: registry.MappingExpression{
					// Expression that checks adapter count - should only see healthy-adapter
					Expression: `size(statuses) == 1 && statuses[0].adapter == "healthy-adapter"`,
				},
				Output: registry.MappingOutput{
					Status:  registry.MappingExpression{Expression: `"True"`},
					Reason:  registry.MappingExpression{Expression: `"OK"`},
					Message: registry.MappingExpression{Expression: `"Only healthy adapter visible"`},
				},
			},
		}

		mapper, err := NewConditionMapper("Cluster", rules)
		Expect(err).NotTo(HaveOccurred())

		statuses := api.AdapterStatusList{
			{
				Adapter:            "healthy-adapter",
				ObservedGeneration: 1,
				Conditions: testConditionsJSON(
					api.AdapterCondition{Type: api.AdapterConditionTypeAvailable, Status: api.AdapterConditionTrue},
				),
				Data: []byte(`{}`),
			},
			{
				Adapter:            "unknown-adapter",
				ObservedGeneration: 1,
				// Adapter with Unknown condition should be filtered out
				Conditions: testConditionsJSON(
					api.AdapterCondition{Type: api.AdapterConditionTypeAvailable, Status: api.AdapterConditionUnknown},
				),
				Data: []byte(`{}`),
			},
		}

		result, err := mapper.Apply(context.Background(), ApplyInput{
			AdapterStatuses: statuses,
			Resource:        map[string]interface{}{},
			RefTime:         time.Now(),
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(HaveLen(1), "when expression should match (unknown-adapter excluded)")
		Expect(result[0].Type).To(Equal("TestCondition"))
		Expect(result[0].Status).To(Equal(api.ConditionTrue))
	})

	t.Run("all adapters with Unknown excluded results in empty statuses array", func(t *testing.T) {
		RegisterTestingT(t)

		rules := []registry.ConditionMappingRule{
			{
				Type: "TestCondition",
				When: registry.MappingExpression{
					Expression: `size(statuses) == 0`, // No adapters should be visible
				},
				Output: registry.MappingOutput{
					Status:  registry.MappingExpression{Expression: `"True"`},
					Reason:  registry.MappingExpression{Expression: `"NoAdapters"`},
					Message: registry.MappingExpression{Expression: `"All adapters filtered"`},
				},
			},
		}

		mapper, err := NewConditionMapper("Cluster", rules)
		Expect(err).NotTo(HaveOccurred())

		statuses := api.AdapterStatusList{
			{
				Adapter: "adapter-1",
				Conditions: testConditionsJSON(
					api.AdapterCondition{Type: api.AdapterConditionTypeAvailable, Status: api.AdapterConditionUnknown},
				),
			},
			{
				Adapter: "adapter-2",
				Conditions: testConditionsJSON(
					api.AdapterCondition{Type: api.AdapterConditionTypeHealth, Status: api.AdapterConditionUnknown},
				),
			},
		}

		result, err := mapper.Apply(context.Background(), ApplyInput{
			AdapterStatuses: statuses,
			Resource:        map[string]interface{}{},
			RefTime:         time.Now(),
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(HaveLen(1), "when expression should match (all adapters excluded)")
		Expect(*result[0].Reason).To(Equal("NoAdapters"))
	})
}

// ============================================================================
// Tests from condition_mapper_timestamps_test.go
// ============================================================================

func TestConditionMapper_TimestampPreservation(t *testing.T) {
	t.Run("new condition gets refTime for all timestamps", func(t *testing.T) {
		RegisterTestingT(t)

		rules := []registry.ConditionMappingRule{
			{Type: "TestReady",
				When: registry.MappingExpression{Expression: `true`},
				Output: registry.MappingOutput{
					Status:  registry.MappingExpression{Expression: `"True"`},
					Reason:  registry.MappingExpression{Expression: `"OK"`},
					Message: registry.MappingExpression{Expression: `"All good"`},
				},
			},
		}

		mapper, err := NewConditionMapper("Cluster", rules)
		Expect(err).NotTo(HaveOccurred())

		refTime := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
		result, err := mapper.Apply(context.Background(), ApplyInput{
			AdapterStatuses: api.AdapterStatusList{},
			Resource:        map[string]interface{}{},
			RefTime:         refTime,
			PrevConditions:  nil, // No previous conditions
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(result).To(HaveLen(1))
		cond := result[0]

		// All timestamps should be refTime for new condition
		expectedTime := refTime.UTC().Truncate(time.Microsecond)
		Expect(cond.CreatedTime).To(Equal(expectedTime))
		Expect(cond.LastTransitionTime).To(Equal(expectedTime))
		Expect(cond.LastUpdatedTime).To(Equal(expectedTime))
	})

	t.Run("status unchanged preserves CreatedTime and LastTransitionTime", func(t *testing.T) {
		RegisterTestingT(t)

		rules := []registry.ConditionMappingRule{
			{Type: "TestReady",
				When: registry.MappingExpression{Expression: `true`},
				Output: registry.MappingOutput{
					Status:  registry.MappingExpression{Expression: `"True"`},
					Reason:  registry.MappingExpression{Expression: `"OK"`},
					Message: registry.MappingExpression{Expression: `"All good"`},
				},
			},
		}

		mapper, err := NewConditionMapper("Cluster", rules)
		Expect(err).NotTo(HaveOccurred())

		// Previous condition with old timestamps
		oldCreatedTime := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
		oldTransitionTime := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
		prevConditions := []api.ResourceCondition{
			{
				Type:               "TestReady",
				Status:             api.ConditionTrue, // Same status
				CreatedTime:        oldCreatedTime,
				LastTransitionTime: oldTransitionTime,
			},
		}

		// New evaluation at a later time
		newRefTime := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
		result, err := mapper.Apply(context.Background(), ApplyInput{
			AdapterStatuses: api.AdapterStatusList{},
			Resource:        map[string]interface{}{},
			RefTime:         newRefTime,
			PrevConditions:  prevConditions,
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(result).To(HaveLen(1))
		cond := result[0]

		// Status unchanged → preserve CreatedTime and LastTransitionTime
		Expect(cond.CreatedTime).To(Equal(oldCreatedTime))
		Expect(cond.LastTransitionTime).To(Equal(oldTransitionTime))
		// LastUpdatedTime should be refTime
		Expect(cond.LastUpdatedTime).To(Equal(newRefTime.UTC().Truncate(time.Microsecond)))
	})

	t.Run("status changed updates LastTransitionTime but preserves CreatedTime", func(t *testing.T) {
		RegisterTestingT(t)

		rules := []registry.ConditionMappingRule{
			{Type: "TestReady",
				When: registry.MappingExpression{Expression: `true`},
				Output: registry.MappingOutput{
					Status:  registry.MappingExpression{Expression: `"False"`}, // Changed from True
					Reason:  registry.MappingExpression{Expression: `"NotReady"`},
					Message: registry.MappingExpression{Expression: `"Something broke"`},
				},
			},
		}

		mapper, err := NewConditionMapper("Cluster", rules)
		Expect(err).NotTo(HaveOccurred())

		// Previous condition with True status
		oldCreatedTime := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
		oldTransitionTime := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
		prevConditions := []api.ResourceCondition{
			{
				Type:               "TestReady",
				Status:             api.ConditionTrue, // Will change to False
				CreatedTime:        oldCreatedTime,
				LastTransitionTime: oldTransitionTime,
			},
		}

		// New evaluation at a later time
		newRefTime := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
		result, err := mapper.Apply(context.Background(), ApplyInput{
			AdapterStatuses: api.AdapterStatusList{},
			Resource:        map[string]interface{}{},
			RefTime:         newRefTime,
			PrevConditions:  prevConditions,
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(result).To(HaveLen(1))
		cond := result[0]

		// Status changed → preserve CreatedTime, update LastTransitionTime
		Expect(cond.CreatedTime).To(Equal(oldCreatedTime))
		Expect(cond.LastTransitionTime).To(Equal(newRefTime.UTC().Truncate(time.Microsecond)))
		Expect(cond.LastUpdatedTime).To(Equal(newRefTime.UTC().Truncate(time.Microsecond)))
	})

	t.Run("multiple conditions preserve timestamps independently", func(t *testing.T) {
		RegisterTestingT(t)

		rules := []registry.ConditionMappingRule{
			{Type: "ConditionA",
				When: registry.MappingExpression{Expression: `true`},
				Output: registry.MappingOutput{
					Status:  registry.MappingExpression{Expression: `"True"`},
					Reason:  registry.MappingExpression{Expression: `"OK"`},
					Message: registry.MappingExpression{Expression: `"A is ready"`},
				},
			},
			{Type: "ConditionB",
				When: registry.MappingExpression{Expression: `true`},
				Output: registry.MappingOutput{
					Status:  registry.MappingExpression{Expression: `"False"`}, // Changed
					Reason:  registry.MappingExpression{Expression: `"NotReady"`},
					Message: registry.MappingExpression{Expression: `"B is not ready"`},
				},
			},
		}

		mapper, err := NewConditionMapper("Cluster", rules)
		Expect(err).NotTo(HaveOccurred())

		// Previous conditions: A unchanged (True), B changed (True→False)
		timeA := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
		timeB := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
		prevConditions := []api.ResourceCondition{
			{
				Type:               "ConditionA",
				Status:             api.ConditionTrue,
				CreatedTime:        timeA,
				LastTransitionTime: timeA,
			},
			{
				Type:               "ConditionB",
				Status:             api.ConditionTrue, // Will change to False
				CreatedTime:        timeB,
				LastTransitionTime: timeB,
			},
		}

		newRefTime := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
		result, err := mapper.Apply(context.Background(), ApplyInput{
			AdapterStatuses: api.AdapterStatusList{},
			Resource:        map[string]interface{}{},
			RefTime:         newRefTime,
			PrevConditions:  prevConditions,
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(result).To(HaveLen(2))

		// Find ConditionA and ConditionB
		var condA, condB *api.ResourceCondition
		for i := range result {
			switch result[i].Type {
			case "ConditionA":
				condA = &result[i]
			case "ConditionB":
				condB = &result[i]
			}
		}

		Expect(condA).NotTo(BeNil())
		Expect(condB).NotTo(BeNil())

		// ConditionA: status unchanged → preserve both timestamps
		Expect(condA.CreatedTime).To(Equal(timeA))
		Expect(condA.LastTransitionTime).To(Equal(timeA))

		// ConditionB: status changed → preserve CreatedTime, update LastTransitionTime
		Expect(condB.CreatedTime).To(Equal(timeB))
		Expect(condB.LastTransitionTime).To(Equal(newRefTime.UTC().Truncate(time.Microsecond)))
	})
}

// ============================================================================
// Tests from condition_mapper_logging_test.go
// ============================================================================

func TestAdapterStatusToMapWithUnknownCheck_LogsJSONErrors(t *testing.T) {
	t.Run("invalid conditions JSONB logs warning", func(t *testing.T) {
		RegisterTestingT(t)

		status := &api.AdapterStatus{
			Adapter:            "test-adapter",
			ObservedGeneration: 1,
			Conditions:         []byte(`{invalid json`), // Invalid JSON
			Data:               []byte(`{}`),
		}

		// Should not panic, should return empty conditions and log warning
		statusMap, hasUnknown := adapterStatusToMapWithUnknownCheck(context.Background(), status)

		Expect(hasUnknown).To(BeFalse())
		Expect(statusMap[celKeyConditions]).To(BeEmpty(), "invalid JSON should result in empty conditions")
		Expect(statusMap[celKeyAdapter]).To(Equal("test-adapter"))
	})

	t.Run("invalid data JSONB logs warning", func(t *testing.T) {
		RegisterTestingT(t)

		status := &api.AdapterStatus{
			Adapter:            "test-adapter",
			ObservedGeneration: 1,
			Conditions:         []byte(`[]`),
			Data:               []byte(`{invalid json`), // Invalid JSON
		}

		// Should not panic, should return empty data map and log warning
		statusMap, hasUnknown := adapterStatusToMapWithUnknownCheck(context.Background(), status)

		Expect(hasUnknown).To(BeFalse())
		Expect(statusMap[celKeyData]).To(BeEmpty(), "invalid JSON should result in empty data map")
		Expect(statusMap[celKeyAdapter]).To(Equal("test-adapter"))
	})
}

func TestResourceToMap_LogsJSONErrors(t *testing.T) {
	t.Run("unmarshalable resource logs warning", func(t *testing.T) {
		RegisterTestingT(t)

		// Channels cannot be marshaled to JSON
		resource := make(chan int)

		// Should not panic, should return empty map and log warning
		result := resourceToMap(context.Background(), resource, "Cluster")

		Expect(result).To(BeEmpty(), "unmarshalable resource should result in empty map")
	})

	t.Run("normal resource works without warnings", func(t *testing.T) {
		RegisterTestingT(t)

		resource := map[string]interface{}{
			"name":       "test",
			"generation": 1,
		}

		result := resourceToMap(context.Background(), resource, "Cluster")

		Expect(result).NotTo(BeEmpty())
		Expect(result["name"]).To(Equal("test"))
	})
}

func TestBuildActivation_NumericTypesConsistency(t *testing.T) {
	t.Run("observed_generation is float64 for CEL type consistency", func(t *testing.T) {
		RegisterTestingT(t)

		statuses := api.AdapterStatusList{
			{
				Adapter:            "test",
				ObservedGeneration: 5, // int32
				Conditions:         []byte(`[]`),
				Data:               []byte(`{}`),
			},
		}

		resource := map[string]interface{}{
			"generation": 5, // Will be float64 after JSON round-trip
		}

		activation := testBuildActivation(t, context.Background(), statuses, resource, "Cluster")

		// Verify statuses[0].observed_generation is float64
		statusesList := activation[util.CELVarStatuses].([]interface{})
		Expect(statusesList).To(HaveLen(1))

		firstStatus := statusesList[0].(map[string]interface{})
		observedGen := firstStatus[celKeyObservedGeneration]

		// Should be float64, not int32
		Expect(observedGen).To(BeAssignableToTypeOf(float64(0)),
			"observed_generation should be float64 for consistency with resource.generation after JSON round-trip")

		// Verify resource.generation is also float64 (from resourceToMap JSON round-trip)
		resourceMap := activation[util.CELVarResource].(map[string]interface{})
		resourceGen := resourceMap[resourceKeyGeneration]

		Expect(resourceGen).To(BeAssignableToTypeOf(float64(0)),
			"resource.generation should be float64 after JSON round-trip")

		// Both should be the same type for CEL expression consistency
		Expect(observedGen).To(BeAssignableToTypeOf(resourceGen),
			"observed_generation and generation should have the same type for CEL consistency")
	})
}

// ============================================================================
// Tests for Apply() when-expression control flow
// ============================================================================

func TestConditionMapper_WhenExpressionSkipsRule(t *testing.T) {
	t.Run("when expression returns false skips rule", func(t *testing.T) {
		RegisterTestingT(t)

		rules := []registry.ConditionMappingRule{{
			Type: "SkippedCondition",
			When: registry.MappingExpression{Expression: `false`},
			Output: registry.MappingOutput{
				Status:  registry.MappingExpression{Expression: `"True"`},
				Reason:  registry.MappingExpression{Expression: `"OK"`},
				Message: registry.MappingExpression{Expression: `"Should not appear"`},
			},
		}}

		mapper, err := NewConditionMapper("Cluster", rules)
		Expect(err).NotTo(HaveOccurred())

		conditions, err := mapper.Apply(context.Background(), ApplyInput{
			AdapterStatuses: api.AdapterStatusList{},
			Resource:        map[string]interface{}{},
			RefTime:         time.Now(),
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(conditions).To(BeEmpty(), "when=false should produce no conditions")
	})

	t.Run("mixed when results produce only matching conditions", func(t *testing.T) {
		RegisterTestingT(t)

		rules := []registry.ConditionMappingRule{
			{
				Type: "AlwaysTrue",
				When: registry.MappingExpression{Expression: `true`},
				Output: registry.MappingOutput{
					Status:  registry.MappingExpression{Expression: `"True"`},
					Reason:  registry.MappingExpression{Expression: `"matched"`},
					Message: registry.MappingExpression{Expression: `"should appear"`},
				},
			},
			{
				Type: "AlwaysFalse",
				When: registry.MappingExpression{Expression: `false`},
				Output: registry.MappingOutput{
					Status:  registry.MappingExpression{Expression: `"True"`},
					Reason:  registry.MappingExpression{Expression: `"skipped"`},
					Message: registry.MappingExpression{Expression: `"should not appear"`},
				},
			},
		}

		mapper, err := NewConditionMapper("Cluster", rules)
		Expect(err).NotTo(HaveOccurred())

		conditions, err := mapper.Apply(context.Background(), ApplyInput{
			AdapterStatuses: api.AdapterStatusList{},
			Resource:        map[string]interface{}{},
			RefTime:         time.Now(),
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(conditions).To(HaveLen(1))
		Expect(conditions[0].Type).To(Equal("AlwaysTrue"))
	})
}

// ============================================================================
// Benchmarks
// ============================================================================

// BenchmarkConditionMapper_Apply benchmarks the Apply method with varying numbers of previous conditions
// to demonstrate the O(1) map lookup optimization vs O(N) linear scan
func BenchmarkConditionMapper_Apply(b *testing.B) {
	// Create mapper with multiple rules
	rules := []registry.ConditionMappingRule{
		{Type: "Condition1",
			When: registry.MappingExpression{Expression: `true`},
			Output: registry.MappingOutput{
				Status:  registry.MappingExpression{Expression: `"True"`},
				Reason:  registry.MappingExpression{Expression: `"OK"`},
				Message: registry.MappingExpression{Expression: `"All good"`},
			},
		},
		{Type: "Condition2",
			When: registry.MappingExpression{Expression: `true`},
			Output: registry.MappingOutput{
				Status:  registry.MappingExpression{Expression: `"True"`},
				Reason:  registry.MappingExpression{Expression: `"OK"`},
				Message: registry.MappingExpression{Expression: `"All good"`},
			},
		},
		{Type: "Condition3",
			When: registry.MappingExpression{Expression: `true`},
			Output: registry.MappingOutput{
				Status:  registry.MappingExpression{Expression: `"True"`},
				Reason:  registry.MappingExpression{Expression: `"OK"`},
				Message: registry.MappingExpression{Expression: `"All good"`},
			},
		},
		{Type: "Condition4",
			When: registry.MappingExpression{Expression: `true`},
			Output: registry.MappingOutput{
				Status:  registry.MappingExpression{Expression: `"True"`},
				Reason:  registry.MappingExpression{Expression: `"OK"`},
				Message: registry.MappingExpression{Expression: `"All good"`},
			},
		},
		{Type: "Condition5",
			When: registry.MappingExpression{Expression: `true`},
			Output: registry.MappingOutput{
				Status:  registry.MappingExpression{Expression: `"True"`},
				Reason:  registry.MappingExpression{Expression: `"OK"`},
				Message: registry.MappingExpression{Expression: `"All good"`},
			},
		},
	}

	mapper, err := NewConditionMapper("Cluster", rules)
	if err != nil {
		b.Fatalf("Failed to create mapper: %v", err)
	}

	benchmarks := []struct {
		name          string
		prevCondCount int
	}{
		{"0_previous_conditions", 0},
		{"5_previous_conditions", 5},
		{"10_previous_conditions", 10},
		{"20_previous_conditions", 20},
		{"50_previous_conditions", 50},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Create previous conditions
			prevConditions := make([]api.ResourceCondition, bm.prevCondCount)
			for i := 0; i < bm.prevCondCount; i++ {
				prevConditions[i] = api.ResourceCondition{
					Type:               "OtherCondition" + string(rune('A'+i)),
					Status:             api.ConditionTrue,
					CreatedTime:        time.Now(),
					LastTransitionTime: time.Now(),
				}
			}
			// Add the conditions our mapper actually looks for
			if bm.prevCondCount > 0 {
				prevConditions[0].Type = "Condition1"
			}

			input := ApplyInput{
				AdapterStatuses: api.AdapterStatusList{},
				Resource:        map[string]interface{}{},
				RefTime:         time.Now(),
				PrevConditions:  prevConditions,
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = mapper.Apply(context.Background(), input)
			}
		})
	}
}

// BenchmarkConditionMapper_PreAllocation benchmarks the effect of pre-allocating the result slice
func BenchmarkConditionMapper_PreAllocation(b *testing.B) {
	rules := make([]registry.ConditionMappingRule, 0, 10)
	for i := 0; i < 10; i++ {
		name := "Condition" + string(rune('A'+i))
		rules = append(rules, registry.ConditionMappingRule{
			Type: name,
			When: registry.MappingExpression{Expression: `true`},
			Output: registry.MappingOutput{
				Status:  registry.MappingExpression{Expression: `"True"`},
				Reason:  registry.MappingExpression{Expression: `"OK"`},
				Message: registry.MappingExpression{Expression: `"All good"`},
			},
		})
	}

	mapper, err := NewConditionMapper("Cluster", rules)
	if err != nil {
		b.Fatalf("Failed to create mapper: %v", err)
	}

	input := ApplyInput{
		AdapterStatuses: api.AdapterStatusList{},
		Resource:        map[string]interface{}{},
		RefTime:         time.Now(),
		PrevConditions:  nil,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = mapper.Apply(context.Background(), input)
	}
}

// ============================================================================
// Tests from extract_string_test.go
// ============================================================================

func TestExtractString_NullHandling(t *testing.T) {
	t.Run("CEL null value returns empty string", func(t *testing.T) {
		RegisterTestingT(t)
		// CEL expressions can return null (e.g., optional field access, map lookup miss)
		// We must handle this to prevent "<nil>" appearing in API responses
		result := extractString(types.NullValue)
		Expect(result).To(Equal(""), "null should produce empty string, not '<nil>'")
	})

	t.Run("string value returns as-is", func(t *testing.T) {
		RegisterTestingT(t)
		result := extractString(types.String("test"))
		Expect(result).To(Equal("test"))
	})

	t.Run("bool true returns True", func(t *testing.T) {
		RegisterTestingT(t)
		result := extractString(types.Bool(true))
		Expect(result).To(Equal("True"))
	})

	t.Run("bool false returns False", func(t *testing.T) {
		RegisterTestingT(t)
		result := extractString(types.Bool(false))
		Expect(result).To(Equal("False"))
	})

	t.Run("int value returns stringified", func(t *testing.T) {
		RegisterTestingT(t)
		result := extractString(types.Int(42))
		Expect(result).To(Equal("42"))
	})
}

// ============================================================================
// Tests from truncate_utf8_test.go
// ============================================================================

func TestTruncateUTF8(t *testing.T) {
	t.Run("empty string returns empty", func(t *testing.T) {
		RegisterTestingT(t)
		result := truncateUTF8("", 10)
		Expect(result).To(Equal(""))
	})

	t.Run("maxBytes is 0 returns empty", func(t *testing.T) {
		RegisterTestingT(t)
		result := truncateUTF8("hello", 0)
		Expect(result).To(Equal(""))
	})

	t.Run("string shorter than maxBytes returns unchanged", func(t *testing.T) {
		RegisterTestingT(t)
		input := "hello"
		result := truncateUTF8(input, 10)
		Expect(result).To(Equal(input))
	})

	t.Run("string equal to maxBytes returns unchanged", func(t *testing.T) {
		RegisterTestingT(t)
		input := "hello"
		result := truncateUTF8(input, 5)
		Expect(result).To(Equal(input))
	})

	t.Run("ASCII truncation at exact boundary", func(t *testing.T) {
		RegisterTestingT(t)
		input := "hello world"
		result := truncateUTF8(input, 5)
		Expect(result).To(Equal("hello"))
		Expect(utf8.ValidString(result)).To(BeTrue())
	})

	t.Run("2-byte UTF-8 character (é) at truncation boundary", func(t *testing.T) {
		RegisterTestingT(t)
		// "café" = 5 bytes: c(1) a(1) f(1) é(2)
		input := "café"

		// Truncate at 4 bytes - should cut before é
		result := truncateUTF8(input, 4)
		Expect(result).To(Equal("caf"))
		Expect(utf8.ValidString(result)).To(BeTrue())

		// Truncate at 5 bytes - should include é
		result = truncateUTF8(input, 5)
		Expect(result).To(Equal("café"))
		Expect(utf8.ValidString(result)).To(BeTrue())
	})

	t.Run("3-byte UTF-8 character (€) at truncation boundary", func(t *testing.T) {
		RegisterTestingT(t)
		// "10€" = 5 bytes: 1(1) 0(1) €(3)
		input := "10€"

		// Truncate at 2 bytes - should be "10"
		result := truncateUTF8(input, 2)
		Expect(result).To(Equal("10"))
		Expect(utf8.ValidString(result)).To(BeTrue())

		// Truncate at 3 or 4 bytes - still "10" (can't fit complete €)
		result = truncateUTF8(input, 3)
		Expect(result).To(Equal("10"))
		Expect(utf8.ValidString(result)).To(BeTrue())

		result = truncateUTF8(input, 4)
		Expect(result).To(Equal("10"))
		Expect(utf8.ValidString(result)).To(BeTrue())

		// Truncate at 5 bytes - includes €
		result = truncateUTF8(input, 5)
		Expect(result).To(Equal("10€"))
		Expect(utf8.ValidString(result)).To(BeTrue())
	})

	t.Run("4-byte UTF-8 character (emoji 😀) at truncation boundary", func(t *testing.T) {
		RegisterTestingT(t)
		// "hi😀" = 6 bytes: h(1) i(1) 😀(4)
		input := "hi😀"

		// Truncate at 2 bytes - should be "hi"
		result := truncateUTF8(input, 2)
		Expect(result).To(Equal("hi"))
		Expect(utf8.ValidString(result)).To(BeTrue())

		// Truncate at 3/4/5 bytes - still "hi" (can't fit 😀)
		result = truncateUTF8(input, 5)
		Expect(result).To(Equal("hi"))
		Expect(utf8.ValidString(result)).To(BeTrue())

		// Truncate at 6 bytes - includes emoji
		result = truncateUTF8(input, 6)
		Expect(result).To(Equal("hi😀"))
		Expect(utf8.ValidString(result)).To(BeTrue())
	})

	t.Run("all multibyte string with maxBytes smaller than first rune", func(t *testing.T) {
		RegisterTestingT(t)
		// "€€€" = 9 bytes (3 bytes each)
		input := "€€€"

		// maxBytes=1 or 2 - can't fit even one €
		result := truncateUTF8(input, 1)
		Expect(result).To(Equal(""))

		result = truncateUTF8(input, 2)
		Expect(result).To(Equal(""))

		// maxBytes=3 - fits one €
		result = truncateUTF8(input, 3)
		Expect(result).To(Equal("€"))
		Expect(utf8.ValidString(result)).To(BeTrue())
	})

	t.Run("mixed ASCII and multibyte characters", func(t *testing.T) {
		RegisterTestingT(t)
		// "Hello世界" = 11 bytes: H(1) e(1) l(1) l(1) o(1) 世(3) 界(3)
		input := "Hello世界"

		// Truncate at 5 - "Hello"
		result := truncateUTF8(input, 5)
		Expect(result).To(Equal("Hello"))
		Expect(utf8.ValidString(result)).To(BeTrue())

		// Truncate at 7 - "Hello" (can't fit 世)
		result = truncateUTF8(input, 7)
		Expect(result).To(Equal("Hello"))
		Expect(utf8.ValidString(result)).To(BeTrue())

		// Truncate at 8 - "Hello世"
		result = truncateUTF8(input, 8)
		Expect(result).To(Equal("Hello世"))
		Expect(utf8.ValidString(result)).To(BeTrue())
	})

	t.Run("result is always valid UTF-8", func(t *testing.T) {
		RegisterTestingT(t)
		testCases := []string{
			"café",
			"10€20",
			"hi😀bye",
			"世界你好",
			"mixed-ASCII-and-日本語",
			"emoji🎉test🎊string",
		}

		for _, input := range testCases {
			// Try various truncation points
			for maxBytes := 0; maxBytes <= len(input); maxBytes++ {
				result := truncateUTF8(input, maxBytes)
				Expect(utf8.ValidString(result)).To(BeTrue(),
					"truncateUTF8(%q, %d) = %q should be valid UTF-8",
					input, maxBytes, result)
			}
		}
	})

	t.Run("preserves complete characters only", func(t *testing.T) {
		RegisterTestingT(t)
		// "test😀end" = 11 bytes: t(1) e(1) s(1) t(1) 😀(4) e(1) n(1) d(1)
		input := "test😀end"

		// Truncate at 7 - should be "test" (can't fit 😀)
		result := truncateUTF8(input, 7)
		Expect(result).To(Equal("test"))
		Expect(utf8.RuneCountInString(result)).To(Equal(4))

		// Truncate at 8 - should include emoji
		result = truncateUTF8(input, 8)
		Expect(result).To(Equal("test😀"))
		Expect(utf8.RuneCountInString(result)).To(Equal(5))
	})
}

func TestConditionMapper_InvalidStatus(t *testing.T) {
	RegisterTestingT(t)

	// CEL rule that outputs "Maybe" instead of "True"/"False"
	rules := []registry.ConditionMappingRule{
		{Type: "TestCondition",
			When: registry.MappingExpression{
				Expression: "true",
			},
			Output: registry.MappingOutput{
				Status: registry.MappingExpression{
					Expression: `"Maybe"`, // Invalid status
				},
				Reason: registry.MappingExpression{
					Expression: `"test"`,
				},
				Message: registry.MappingExpression{
					Expression: `"test message"`,
				},
			},
		},
	}

	mapper, err := NewConditionMapper("Cluster", rules)
	Expect(err).NotTo(HaveOccurred())

	input := ApplyInput{
		AdapterStatuses: api.AdapterStatusList{},
		Resource:        map[string]interface{}{},
		RefTime:         time.Now(),
	}

	// Should return error (not panic, not skip) to trigger transaction rollback
	result, err := mapper.Apply(context.Background(), input)
	Expect(err).To(HaveOccurred(), "invalid status should return error")
	Expect(err.Error()).To(ContainSubstring("invalid status value"))
	Expect(result).To(BeNil(), "result should be nil when error occurs")
}

func TestConditionMapper_NonBooleanWhen(t *testing.T) {
	RegisterTestingT(t)

	// CEL rule with when expression that returns string instead of boolean
	rules := []registry.ConditionMappingRule{
		{Type: "TestCondition",
			When: registry.MappingExpression{
				Expression: `"not a boolean"`, // Should return bool
			},
			Output: registry.MappingOutput{
				Status: registry.MappingExpression{
					Expression: `"True"`,
				},
				Reason: registry.MappingExpression{
					Expression: `"test"`,
				},
				Message: registry.MappingExpression{
					Expression: `"test message"`,
				},
			},
		},
	}

	mapper, err := NewConditionMapper("Cluster", rules)
	Expect(err).NotTo(HaveOccurred())

	input := ApplyInput{
		AdapterStatuses: api.AdapterStatusList{},
		Resource:        map[string]interface{}{},
		RefTime:         time.Now(),
	}

	// Should return error (not panic, not skip) to trigger transaction rollback
	result, err := mapper.Apply(context.Background(), input)
	Expect(err).To(HaveOccurred(), "non-boolean when expression should return error")
	Expect(err.Error()).To(ContainSubstring("did not return boolean"))
	Expect(result).To(BeNil(), "result should be nil when error occurs")
}

func TestConditionMapper_MessageTruncationThroughPipeline(t *testing.T) {
	RegisterTestingT(t)

	// Build a CEL expression that produces a message exceeding MaxConditionMessageLength (2048)
	longMsg := strings.Repeat("x", registry.MaxConditionMessageLength+100)
	msgExpr := fmt.Sprintf(`"%s"`, longMsg)

	rules := []registry.ConditionMappingRule{
		{Type: "TestCondition",
			When: registry.MappingExpression{
				Expression: "true",
			},
			Output: registry.MappingOutput{
				Status: registry.MappingExpression{
					Expression: `"True"`,
				},
				Reason: registry.MappingExpression{
					Expression: `"TestReason"`,
				},
				Message: registry.MappingExpression{
					Expression: msgExpr,
				},
			},
		},
	}

	mapper, err := NewConditionMapper("Cluster", rules)
	Expect(err).NotTo(HaveOccurred())

	input := ApplyInput{
		AdapterStatuses: api.AdapterStatusList{},
		Resource:        map[string]interface{}{},
		RefTime:         time.Now(),
	}

	result, err := mapper.Apply(context.Background(), input)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(HaveLen(1), "should produce a condition even with long message")
	Expect(len(*result[0].Message)).To(BeNumerically("<=", registry.MaxConditionMessageLength),
		"message should be truncated to MaxConditionMessageLength")
	Expect(*result[0].Reason).To(Equal("TestReason"), "reason should be unchanged")
}

func TestConditionMapper_OutputExpressionRuntimeError(t *testing.T) {
	RegisterTestingT(t)

	// CEL rule where the status expression causes a runtime error (out-of-bounds index)
	rules := []registry.ConditionMappingRule{
		{Type: "TestCondition",
			When: registry.MappingExpression{
				Expression: "true",
			},
			Output: registry.MappingOutput{
				Status: registry.MappingExpression{
					Expression: `statuses[99].adapter`, // out of bounds → runtime error
				},
				Reason: registry.MappingExpression{
					Expression: `"reason"`,
				},
				Message: registry.MappingExpression{
					Expression: `"message"`,
				},
			},
		},
	}

	mapper, err := NewConditionMapper("Cluster", rules)
	Expect(err).NotTo(HaveOccurred())

	input := ApplyInput{
		AdapterStatuses: api.AdapterStatusList{},
		Resource:        map[string]interface{}{},
		RefTime:         time.Now(),
	}

	// Should return error (not panic, not skip) to trigger transaction rollback
	result, err := mapper.Apply(context.Background(), input)
	Expect(err).To(HaveOccurred(), "output expression runtime error should return error")
	Expect(err.Error()).To(ContainSubstring("status expression evaluation failed"))
	Expect(result).To(BeNil(), "result should be nil when error occurs")
}

func TestConditionMapper_ConcurrentApply(t *testing.T) {
	RegisterTestingT(t)

	rules := []registry.ConditionMappingRule{
		{Type: "TestCondition",
			When: registry.MappingExpression{
				Expression: "true",
			},
			Output: registry.MappingOutput{
				Status: registry.MappingExpression{
					Expression: `"True"`,
				},
				Reason: registry.MappingExpression{
					Expression: `"test"`,
				},
				Message: registry.MappingExpression{
					Expression: `"test message"`,
				},
			},
		},
	}

	mapper, err := NewConditionMapper("Cluster", rules)
	Expect(err).NotTo(HaveOccurred())

	// Run under -race flag to detect data races
	const numGoroutines = 10
	type result struct {
		err        error
		conditions []api.ResourceCondition
	}
	results := make(chan result, numGoroutines)

	// Create different *api.Resource instances to exercise cache path
	// (using map[string]interface{} bypasses cache via type assertion fallback)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			// Different resources to trigger cache eviction/thrashing and race detection
			resource := &api.Resource{
				Meta: api.Meta{
					ID: fmt.Sprintf("resource-%d", id),
				},
				Kind:       "Cluster",
				Name:       fmt.Sprintf("cluster-%d", id),
				Generation: int32(id),
			}

			input := ApplyInput{
				AdapterStatuses: api.AdapterStatusList{},
				Resource:        resource,
				RefTime:         time.Now(),
			}

			// Collect result, don't assert in goroutine
			conditions, err := mapper.Apply(context.Background(), input)
			results <- result{conditions: conditions, err: err}
		}(i)
	}

	// Wait for all goroutines and assert on main test goroutine
	for i := 0; i < numGoroutines; i++ {
		res := <-results
		Expect(res.err).ToNot(HaveOccurred())
		Expect(res.conditions).To(HaveLen(1))
		Expect(res.conditions[0].Type).To(Equal("TestCondition"))
	}
}

// testBuildActivation is the test-only variant of buildActivationWithCache that doesn't use caching.
// Used by unit tests to validate CEL context construction without needing a full ConditionMapper instance.
// Production code exclusively uses buildActivationWithCache through the mapper.
func testBuildActivation(
	t *testing.T,
	ctx context.Context,
	statuses api.AdapterStatusList,
	resource interface{},
	resourceKind string,
) map[string]interface{} {
	t.Helper()

	// Convert adapter statuses using shared logic (PERF-03: avoid duplication)
	statusesList := buildStatusesList(ctx, statuses)

	// Convert resource to map and mask sensitive fields (defense-in-depth)
	// Matches the protection applied to adapter data to prevent credential leakage
	resourceMap := util.MaskSensitiveFields(resourceToMap(ctx, resource, resourceKind))

	return map[string]interface{}{
		util.CELVarStatuses: statusesList,
		util.CELVarResource: resourceMap,
		util.CELVarEnv:      emptyEnvMap, // Shared package-level var (PERF-03)
	}
}
