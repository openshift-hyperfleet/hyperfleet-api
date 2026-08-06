package util

import (
	"testing"

	. "github.com/onsi/gomega"
)

// ============================================================================
// Core Masking Tests
// ============================================================================

func TestMaskSensitiveFields(t *testing.T) {
	t.Parallel()
	t.Run("nil map returns nil", func(t *testing.T) {
		RegisterTestingT(t)
		result := MaskSensitiveFields(nil)
		Expect(result).To(BeNil())
	})

	t.Run("empty map returns empty", func(t *testing.T) {
		RegisterTestingT(t)
		result := MaskSensitiveFields(map[string]interface{}{})
		Expect(result).To(BeEmpty())
	})

	t.Run("non-sensitive fields pass through unchanged", func(t *testing.T) {
		RegisterTestingT(t)

		input := map[string]interface{}{
			"clusterName": "test-cluster",
			"region":      "us-west-2",
			"count":       3,
		}

		result := MaskSensitiveFields(input)

		Expect(result).To(HaveLen(3))
		Expect(result["clusterName"]).To(Equal("test-cluster"))
		Expect(result["region"]).To(Equal("us-west-2"))
		Expect(result["count"]).To(Equal(3))
	})

	t.Run("password field is redacted", func(t *testing.T) {
		RegisterTestingT(t)

		input := map[string]interface{}{
			"username": "admin",
			"password": "super-secret-123",
		}

		result := MaskSensitiveFields(input)

		Expect(result["username"]).To(Equal("admin"))
		Expect(result["password"]).To(Equal(RedactedPlaceholder))
	})

	t.Run("all sensitive patterns are redacted case-insensitively", func(t *testing.T) {
		RegisterTestingT(t)

		input := map[string]interface{}{
			"adminPassword":        "secret1",
			"pullSecret":           "secret2",
			"apiToken":             "secret3",
			"gcpServiceAccountKey": "secret4",
			"awsCredential":        "secret5",
			"ApiKey":               "secret6", // Mixed case
			"PRIVATE_KEY":          "secret7", // Upper case
			"authToken":            "secret8",
		}

		result := MaskSensitiveFields(input)

		// All should be redacted
		for k := range input {
			Expect(result[k]).To(Equal(RedactedPlaceholder), "field %s should be redacted", k)
		}
	})

	t.Run("nested maps are recursively masked", func(t *testing.T) {
		RegisterTestingT(t)

		input := map[string]interface{}{
			"clusterName": "test-cluster",
			"auth": map[string]interface{}{
				"username": "admin",
				"password": "secret123",
			},
		}

		result := MaskSensitiveFields(input)

		Expect(result["clusterName"]).To(Equal("test-cluster"))

		// "auth" key itself is redacted (contains "auth" pattern)
		Expect(result["auth"]).To(Equal(RedactedPlaceholder))
	})

	t.Run("nested non-sensitive map is recursively processed", func(t *testing.T) {
		RegisterTestingT(t)

		input := map[string]interface{}{
			"clusterName": "test-cluster",
			"config": map[string]interface{}{
				"region":   "us-west-2",
				"password": "secret123",
			},
		}

		result := MaskSensitiveFields(input)

		Expect(result["clusterName"]).To(Equal("test-cluster"))

		// "config" itself is not sensitive, so we get nested map
		configMap, ok := result["config"].(map[string]interface{})
		Expect(ok).To(BeTrue())
		Expect(configMap["region"]).To(Equal("us-west-2"))
		Expect(configMap["password"]).To(Equal(RedactedPlaceholder))
	})

	t.Run("deeply nested sensitive fields are redacted", func(t *testing.T) {
		RegisterTestingT(t)

		input := map[string]interface{}{
			"level1": map[string]interface{}{
				"level2": map[string]interface{}{
					"level3": map[string]interface{}{
						"password": "deep-secret",
						"region":   "us-east-1",
					},
				},
			},
		}

		result := MaskSensitiveFields(input)

		level1 := result["level1"].(map[string]interface{})
		level2 := level1["level2"].(map[string]interface{})
		level3 := level2["level3"].(map[string]interface{})

		Expect(level3["password"]).To(Equal(RedactedPlaceholder))
		Expect(level3["region"]).To(Equal("us-east-1"))
	})

	t.Run("complex real-world adapter data", func(t *testing.T) {
		RegisterTestingT(t)

		input := map[string]interface{}{
			"cluster_name": "prod-cluster-1",
			"namespace":    "default",
			"service_account": map[string]interface{}{
				"name":       "hypershift-sa",
				"privateKey": "fake-private-key-for-testing-not-real",
			},
			"pull_secret":    "fake-pull-secret-for-testing",
			"admin_password": "admin123",
			"region":         "us-west-2",
			"node_count":     3,
		}

		result := MaskSensitiveFields(input)

		// Non-sensitive fields preserved
		Expect(result["cluster_name"]).To(Equal("prod-cluster-1"))
		Expect(result["namespace"]).To(Equal("default"))
		Expect(result["region"]).To(Equal("us-west-2"))
		Expect(result["node_count"]).To(Equal(3))

		// Sensitive top-level fields redacted
		Expect(result["pull_secret"]).To(Equal(RedactedPlaceholder))
		Expect(result["admin_password"]).To(Equal(RedactedPlaceholder))

		// Nested sensitive field redacted
		sa := result["service_account"].(map[string]interface{})
		Expect(sa["name"]).To(Equal("hypershift-sa"))
		Expect(sa["privateKey"]).To(Equal(RedactedPlaceholder))
	})

	t.Run("arrays with sensitive fields in elements are masked", func(t *testing.T) {
		RegisterTestingT(t)

		input := map[string]interface{}{
			"clusterName": "test-cluster",
			"users": []interface{}{
				map[string]interface{}{
					"name":     "admin",
					"password": "secret123", // Should be masked
				},
				map[string]interface{}{
					"name":  "viewer",
					"token": "abc-def-ghi", // Should be masked
				},
			},
		}

		result := MaskSensitiveFields(input)

		Expect(result["clusterName"]).To(Equal("test-cluster"))

		users := result["users"].([]interface{})
		Expect(users).To(HaveLen(2))

		// First user: password should be masked
		user0 := users[0].(map[string]interface{})
		Expect(user0["name"]).To(Equal("admin"))
		Expect(user0["password"]).To(Equal(RedactedPlaceholder))

		// Second user: token should be masked
		user1 := users[1].(map[string]interface{})
		Expect(user1["name"]).To(Equal("viewer"))
		Expect(user1["token"]).To(Equal(RedactedPlaceholder))
	})

	t.Run("nested arrays are recursively masked", func(t *testing.T) {
		RegisterTestingT(t)

		input := map[string]interface{}{
			"environments": []interface{}{
				map[string]interface{}{
					"name": "production",
					"providers": []interface{}{ // "providers" is not a sensitive key
						map[string]interface{}{
							"type":   "aws",
							"apiKey": "fake-aws-key-for-testing", // Should be masked
							"region": "us-east-1",
						},
					},
				},
			},
		}

		result := MaskSensitiveFields(input)

		envs := result["environments"].([]interface{})
		env0 := envs[0].(map[string]interface{})
		Expect(env0["name"]).To(Equal("production"))

		providers := env0["providers"].([]interface{})
		provider0 := providers[0].(map[string]interface{})
		Expect(provider0["type"]).To(Equal("aws"))
		Expect(provider0["apiKey"]).To(Equal(RedactedPlaceholder))
		Expect(provider0["region"]).To(Equal("us-east-1"))
	})

	t.Run("arrays of primitives pass through unchanged", func(t *testing.T) {
		RegisterTestingT(t)

		input := map[string]interface{}{
			"regions": []interface{}{"us-east-1", "us-west-2", "eu-west-1"},
			"ports":   []interface{}{80, 443, 8080},
		}

		result := MaskSensitiveFields(input)

		regions := result["regions"].([]interface{})
		Expect(regions).To(Equal([]interface{}{"us-east-1", "us-west-2", "eu-west-1"}))

		ports := result["ports"].([]interface{})
		Expect(ports).To(Equal([]interface{}{80, 443, 8080}))
	})

	t.Run("deeply nested arrays of arrays are masked", func(t *testing.T) {
		RegisterTestingT(t)

		input := map[string]interface{}{
			"matrix": []interface{}{
				[]interface{}{
					map[string]interface{}{
						"value":    "data",
						"password": "secret", // Should be masked
					},
				},
			},
		}

		result := MaskSensitiveFields(input)

		matrix := result["matrix"].([]interface{})
		row0 := matrix[0].([]interface{})
		cell := row0[0].(map[string]interface{})
		Expect(cell["value"]).To(Equal("data"))
		Expect(cell["password"]).To(Equal(RedactedPlaceholder))
	})
}

func TestIsSensitiveKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		key       string
		sensitive bool
	}{
		{"password lowercase", "password", true},
		{"password mixed case", "adminPassword", true},
		{"password uppercase", "ADMIN_PASSWORD", true},
		{"secret lowercase", "secret", true},
		{"secret in compound", "pullSecret", true},
		{"token", "apiToken", true},
		{"key", "privateKey", true},
		{"credential", "awsCredential", true},
		{"apikey compound", "gcpApiKey", true},
		{"api_key underscore", "service_api_key", true},
		{"auth", "authToken", true},
		{"private", "privateData", true},
		// security patterns
		{"cert", "tlsCert", true},
		{"kubeconfig", "adminKubeconfig", true},
		{"bearer", "bearerToken", true},
		{"connection", "dbConnectionString", true},
		// Non-sensitive
		{"non-sensitive", "clusterName", false},
		{"non-sensitive with key substring", "keyboard", false}, // "key" alone is too broad
		{"region", "region", false},
		{"count", "nodeCount", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			result := isSensitiveKey(tt.key)
			Expect(result).To(Equal(tt.sensitive))
		})
	}
}

// ============================================================================
// security Pattern Tests
// ============================================================================

func TestMaskSensitiveFields_SEC02Patterns(t *testing.T) {
	t.Parallel()
	t.Run("TLS certificate fields are masked", func(t *testing.T) {
		RegisterTestingT(t)

		input := map[string]interface{}{
			"clusterName":     "prod-cluster",
			"tlsCert":         "-----BEGIN CERTIFICATE-----\nMIIC...",
			"clientCert":      "-----BEGIN CERTIFICATE-----\nMIID...",
			"caCertificate":   "-----BEGIN CERTIFICATE-----\nMIIE...",
			"serverCertChain": "multi-cert-chain",
		}

		result := MaskSensitiveFields(input)

		// Non-sensitive field preserved
		Expect(result["clusterName"]).To(Equal("prod-cluster"))

		// All cert fields should be masked
		Expect(result["tlsCert"]).To(Equal(RedactedPlaceholder))
		Expect(result["clientCert"]).To(Equal(RedactedPlaceholder))
		Expect(result["caCertificate"]).To(Equal(RedactedPlaceholder))
		Expect(result["serverCertChain"]).To(Equal(RedactedPlaceholder))
	})

	t.Run("kubeconfig fields are masked", func(t *testing.T) {
		RegisterTestingT(t)

		input := map[string]interface{}{
			"clusterName":   "prod-cluster",
			"kubeconfig":    "apiVersion: v1\nclusters:\n- cluster:\n    certificate-authority-data: LS0t...",
			"kubeconfigRaw": "base64-encoded-kubeconfig-blob",
			"adminKubeconfig": map[string]interface{}{
				"data": "kubeconfig-content",
			},
		}

		result := MaskSensitiveFields(input)

		Expect(result["clusterName"]).To(Equal("prod-cluster"))
		Expect(result["kubeconfig"]).To(Equal(RedactedPlaceholder))
		Expect(result["kubeconfigRaw"]).To(Equal(RedactedPlaceholder))
		Expect(result["adminKubeconfig"]).To(Equal(RedactedPlaceholder))
	})

	t.Run("bearer token fields are masked", func(t *testing.T) {
		RegisterTestingT(t)

		input := map[string]interface{}{
			"clusterName":  "prod-cluster",
			"bearerToken":  "fake-jwt-token-for-testing-not-real",
			"bearer":       "fake-bearer-token-for-testing",
			"bearerHeader": "Bearer fake-token-for-testing",
		}

		result := MaskSensitiveFields(input)

		Expect(result["clusterName"]).To(Equal("prod-cluster"))
		Expect(result["bearerToken"]).To(Equal(RedactedPlaceholder))
		Expect(result["bearer"]).To(Equal(RedactedPlaceholder))
		Expect(result["bearerHeader"]).To(Equal(RedactedPlaceholder))
	})

	t.Run("connection string fields are masked", func(t *testing.T) {
		RegisterTestingT(t)

		input := map[string]interface{}{
			"dbHost":                "postgres.example.com",
			"connection_string":     "postgresql://user:pass@host:5432/dbname",
			"dbConnectionString":    "mysql://root:pass@localhost:3306/app",
			"redisConnectionString": "redis://:pass@host:6379/0",
		}

		result := MaskSensitiveFields(input)

		// Non-sensitive field preserved
		Expect(result["dbHost"]).To(Equal("postgres.example.com"))

		// All connection string fields should be masked (contain "connection")
		Expect(result["connection_string"]).To(Equal(RedactedPlaceholder))
		Expect(result["dbConnectionString"]).To(Equal(RedactedPlaceholder))
		Expect(result["redisConnectionString"]).To(Equal(RedactedPlaceholder))
	})

	t.Run("nested security patterns in adapter data", func(t *testing.T) {
		RegisterTestingT(t)

		input := map[string]interface{}{
			"clusterName": "prod-cluster",
			"hypershift": map[string]interface{}{
				"kubeconfig":  "admin-kubeconfig-blob",
				"pullSecrets": "registry-credentials",
			},
			"tlsConfig": map[string]interface{}{
				"caCert":     "-----BEGIN CERTIFICATE-----...",
				"clientCert": "-----BEGIN CERTIFICATE-----...",
				"serverName": "api.cluster.example.com", // Not sensitive
			},
		}

		result := MaskSensitiveFields(input)

		Expect(result["clusterName"]).To(Equal("prod-cluster"))

		// hypershift is not sensitive, but nested fields are
		hypershift := result["hypershift"].(map[string]interface{})
		Expect(hypershift["kubeconfig"]).To(Equal(RedactedPlaceholder))
		Expect(hypershift["pullSecrets"]).To(Equal(RedactedPlaceholder), "pullSecrets contains 'secret'")

		// tlsConfig is not sensitive, but nested cert fields are
		tlsConfig := result["tlsConfig"].(map[string]interface{})
		Expect(tlsConfig["caCert"]).To(Equal(RedactedPlaceholder))
		Expect(tlsConfig["clientCert"]).To(Equal(RedactedPlaceholder))
		Expect(tlsConfig["serverName"]).To(Equal("api.cluster.example.com"))
	})

	t.Run("security patterns in arrays", func(t *testing.T) {
		RegisterTestingT(t)

		input := map[string]interface{}{
			"clusters": []interface{}{
				map[string]interface{}{
					"name":       "cluster-1",
					"kubeconfig": "kubeconfig-1",
				},
				map[string]interface{}{
					"name":       "cluster-2",
					"connection": "postgresql://...",
				},
			},
		}

		result := MaskSensitiveFields(input)

		clusters := result["clusters"].([]interface{})
		Expect(clusters).To(HaveLen(2))

		cluster1 := clusters[0].(map[string]interface{})
		Expect(cluster1["name"]).To(Equal("cluster-1"))
		Expect(cluster1["kubeconfig"]).To(Equal(RedactedPlaceholder))

		cluster2 := clusters[1].(map[string]interface{})
		Expect(cluster2["name"]).To(Equal("cluster-2"))
		Expect(cluster2["connection"]).To(Equal(RedactedPlaceholder), "connection contains 'connection'")
	})

	t.Run("real-world HyperShift adapter data with security patterns", func(t *testing.T) {
		RegisterTestingT(t)

		input := map[string]interface{}{
			"clusterName": "prod-hypershift-cluster",
			"namespace":   "clusters",
			"hostedCluster": map[string]interface{}{
				"name": "my-cluster",
				"kubeconfig": map[string]interface{}{
					"name": "admin-kubeconfig",
					"data": "fake-base64-kubeconfig-data-for-testing-not-real",
				},
			},
			"pullSecret": "fake-pull-secret-for-testing-not-real",
			"sshKey":     "fake-ssh-key-for-testing-not-real",
			"baseDomain": "example.com", // Not sensitive
		}

		result := MaskSensitiveFields(input)

		// Non-sensitive fields preserved
		Expect(result["clusterName"]).To(Equal("prod-hypershift-cluster"))
		Expect(result["namespace"]).To(Equal("clusters"))
		Expect(result["baseDomain"]).To(Equal("example.com"))

		// Sensitive fields masked
		Expect(result["pullSecret"]).To(Equal(RedactedPlaceholder))
		Expect(result["sshKey"]).To(Equal(RedactedPlaceholder), "sshKey contains 'key'")

		// hostedCluster contains kubeconfig (nested sensitive)
		hostedCluster := result["hostedCluster"].(map[string]interface{})
		Expect(hostedCluster["name"]).To(Equal("my-cluster"))
		Expect(hostedCluster["kubeconfig"]).To(Equal(RedactedPlaceholder))
	})
}

// ============================================================================
// SEC-03 Depth Limit Tests
// ============================================================================

func TestMaskSensitiveFields_DepthLimit(t *testing.T) {
	t.Parallel()
	t.Run("deeply nested structure stops at max depth (SEC-03)", func(t *testing.T) {
		RegisterTestingT(t)

		// Build a structure nested 25 levels deep (exceeds maxMaskingDepth=20)
		deeplyNested := make(map[string]interface{})
		current := deeplyNested
		for i := 0; i < 25; i++ {
			current["level"] = i
			current["data"] = "value"
			next := make(map[string]interface{})
			current["nested"] = next
			current = next
		}
		current["final"] = "deepest"

		// Should not panic (stack overflow protection)
		result := MaskSensitiveFields(deeplyNested)

		// Verify first few levels are intact
		Expect(result["level"]).To(Equal(0))
		Expect(result["data"]).To(Equal("value"))

		// Navigate down to depth 19 (last allowed level)
		current = result
		for i := 0; i < 19; i++ {
			nested, ok := current["nested"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "Level %d should be a map", i)
			current = nested
		}

		// At depth 20, we should hit the limit and get an empty map
		nested, ok := current["nested"].(map[string]interface{})
		Expect(ok).To(BeTrue())
		Expect(nested).To(BeEmpty(), "Depth 20 should return empty map (limit reached)")
	})

	t.Run("deeply nested array stops at max depth (SEC-03)", func(t *testing.T) {
		RegisterTestingT(t)

		// Build an array nested 25 levels deep
		deepest := []interface{}{"final"}
		for i := 0; i < 25; i++ {
			deepest = []interface{}{deepest}
		}

		result := maskSensitiveSliceDepth(deepest, 0)

		// Navigate down to depth 19
		currentSlice := result
		for i := 0; i < 19; i++ {
			Expect(currentSlice).To(HaveLen(1))
			next, ok := currentSlice[0].([]interface{})
			Expect(ok).To(BeTrue(), "Level %d should be a slice", i)
			currentSlice = next
		}

		// At depth 20, should get empty slice (limit reached)
		Expect(currentSlice).To(HaveLen(1))
		final, ok := currentSlice[0].([]interface{})
		Expect(ok).To(BeTrue())
		Expect(final).To(BeEmpty(), "Depth 20 should return empty slice (limit reached)")
	})

	t.Run("normal nested structures work fine (SEC-03)", func(t *testing.T) {
		RegisterTestingT(t)

		// Build a reasonably nested structure (5 levels, well under limit)
		input := map[string]interface{}{
			"level1": map[string]interface{}{
				"level2": map[string]interface{}{
					"level3": map[string]interface{}{
						"level4": map[string]interface{}{
							"level5": map[string]interface{}{
								"password": "secret123",
								"name":     "final",
							},
						},
					},
				},
			},
		}

		result := MaskSensitiveFields(input)

		// Navigate to level5
		level1 := result["level1"].(map[string]interface{})
		level2 := level1["level2"].(map[string]interface{})
		level3 := level2["level3"].(map[string]interface{})
		level4 := level3["level4"].(map[string]interface{})
		level5 := level4["level5"].(map[string]interface{})

		// Verify masking works at all levels
		Expect(level5["password"]).To(Equal(RedactedPlaceholder))
		Expect(level5["name"]).To(Equal("final"))
	})
}

// ============================================================================
// False Positive Prevention Tests
// ============================================================================

func TestMaskSensitiveFields_FalsePositivePrevention(t *testing.T) {
	t.Parallel()
	t.Run("database fields with 'key' are NOT redacted", func(t *testing.T) {
		RegisterTestingT(t)

		// Common database field names that should NOT be redacted
		input := map[string]interface{}{
			"partitionKey": "user-123",        // DynamoDB partition key
			"sortKey":      "timestamp",       // DynamoDB sort key
			"primaryKey":   42,                // Database primary key
			"foreignKey":   99,                // Database foreign key
			"uniqueKey":    "abc",             // Database unique constraint
			"indexKey":     "idx_users_email", // Database index
			"cacheKey":     "session:xyz",     // Redis/cache key
			"lookupKey":    "search-term",     // Search index key
			"routingKey":   "orders",          // Message queue routing key
			"shardKey":     "region-us-west",  // Sharding key
		}

		result := MaskSensitiveFields(input)

		// None should be redacted - these are metadata, not credentials
		Expect(result["partitionKey"]).To(Equal("user-123"))
		Expect(result["sortKey"]).To(Equal("timestamp"))
		Expect(result["primaryKey"]).To(Equal(42))
		Expect(result["foreignKey"]).To(Equal(99))
		Expect(result["uniqueKey"]).To(Equal("abc"))
		Expect(result["indexKey"]).To(Equal("idx_users_email"))
		Expect(result["cacheKey"]).To(Equal("session:xyz"))
		Expect(result["lookupKey"]).To(Equal("search-term"))
		Expect(result["routingKey"]).To(Equal("orders"))
		Expect(result["shardKey"]).To(Equal("region-us-west"))
	})

	t.Run("legitimate credential 'key' fields ARE redacted", func(t *testing.T) {
		RegisterTestingT(t)

		// Real credential fields that SHOULD be redacted
		input := map[string]interface{}{
			"apiKey":               "fake-api-key-for-testing",
			"privateKey":           "fake-private-key-for-testing",
			"secretKey":            "fake-secret-key-for-testing",
			"accessKey":            "fake-access-key-for-testing",
			"sshKey":               "fake-ssh-key-for-testing",
			"encryptionKey":        "fake-encryption-key-for-testing",
			"gcpServiceAccountKey": "fake-gcp-key-for-testing",
			"azureAccountKey":      "fake-azure-key-for-testing",
			"registryKey":          "docker-registry-token",
			"signingKey":           "code-signing-key",
		}

		result := MaskSensitiveFields(input)

		// All should be redacted - these are credentials
		for k := range input {
			Expect(result[k]).To(Equal(RedactedPlaceholder), "field %s should be redacted", k)
		}
	})

	t.Run("service/product names with 'key' are NOT redacted", func(t *testing.T) {
		RegisterTestingT(t)

		// Service/product names that contain 'key' but aren't credentials
		input := map[string]interface{}{
			"keystoneEndpoint": "https://keystone.example.com", // OpenStack Keystone
			"monkeyPatch":      true,                           // Code patching
			"turkeyMode":       false,                          // Silly example
			"keyValueStore":    "redis",                        // Generic KV store
			"hotkey":           "Ctrl+S",                       // Keyboard shortcut
			"keyframe":         60,                             // Video encoding
		}

		result := MaskSensitiveFields(input)

		// None should be redacted - these are not credentials
		Expect(result["keystoneEndpoint"]).To(Equal("https://keystone.example.com"))
		Expect(result["monkeyPatch"]).To(Equal(true))
		Expect(result["turkeyMode"]).To(Equal(false))
		Expect(result["keyValueStore"]).To(Equal("redis"))
		Expect(result["hotkey"]).To(Equal("Ctrl+S"))
		Expect(result["keyframe"]).To(Equal(60))
	})
}
