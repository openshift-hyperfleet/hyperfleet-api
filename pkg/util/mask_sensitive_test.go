package util

import (
	"testing"

	"github.com/onsi/gomega"
)

// ============================================================================
// Core Masking Tests
// ============================================================================

func TestMaskSensitiveFields(t *testing.T) {
	t.Parallel()
	t.Run("nil map returns nil", func(t *testing.T) {
		g := gomega.NewWithT(t)
		result := MaskSensitiveFields(nil)
		g.Expect(result).To(gomega.BeNil())
	})

	t.Run("empty map returns empty", func(t *testing.T) {
		g := gomega.NewWithT(t)
		result := MaskSensitiveFields(map[string]interface{}{})
		g.Expect(result).To(gomega.BeEmpty())
	})

	t.Run("non-sensitive fields pass through unchanged", func(t *testing.T) {
		g := gomega.NewWithT(t)

		input := map[string]interface{}{
			"clusterName": "test-cluster",
			"region":      "us-west-2",
			"count":       3,
		}

		result := MaskSensitiveFields(input)

		g.Expect(result).To(gomega.HaveLen(3))
		g.Expect(result["clusterName"]).To(gomega.Equal("test-cluster"))
		g.Expect(result["region"]).To(gomega.Equal("us-west-2"))
		g.Expect(result["count"]).To(gomega.Equal(3))
	})

	t.Run("password field is redacted", func(t *testing.T) {
		g := gomega.NewWithT(t)

		input := map[string]interface{}{
			"username": "admin",
			"password": "super-secret-123",
		}

		result := MaskSensitiveFields(input)

		g.Expect(result["username"]).To(gomega.Equal("admin"))
		g.Expect(result["password"]).To(gomega.Equal(RedactedPlaceholder))
	})

	t.Run("all sensitive patterns are redacted case-insensitively", func(t *testing.T) {
		g := gomega.NewWithT(t)

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
			g.Expect(result[k]).To(gomega.Equal(RedactedPlaceholder), "field %s should be redacted", k)
		}
	})

	t.Run("nested maps are recursively masked", func(t *testing.T) {
		g := gomega.NewWithT(t)

		input := map[string]interface{}{
			"clusterName": "test-cluster",
			"auth": map[string]interface{}{
				"username": "admin",
				"password": "secret123",
			},
		}

		result := MaskSensitiveFields(input)

		g.Expect(result["clusterName"]).To(gomega.Equal("test-cluster"))

		// "auth" key itself is redacted (contains "auth" pattern)
		g.Expect(result["auth"]).To(gomega.Equal(RedactedPlaceholder))
	})

	t.Run("nested non-sensitive map is recursively processed", func(t *testing.T) {
		g := gomega.NewWithT(t)

		input := map[string]interface{}{
			"clusterName": "test-cluster",
			"config": map[string]interface{}{
				"region":   "us-west-2",
				"password": "secret123",
			},
		}

		result := MaskSensitiveFields(input)

		g.Expect(result["clusterName"]).To(gomega.Equal("test-cluster"))

		// "config" itself is not sensitive, so we get nested map
		configMap, ok := result["config"].(map[string]interface{})
		g.Expect(ok).To(gomega.BeTrue())
		g.Expect(configMap["region"]).To(gomega.Equal("us-west-2"))
		g.Expect(configMap["password"]).To(gomega.Equal(RedactedPlaceholder))
	})

	t.Run("deeply nested sensitive fields are redacted", func(t *testing.T) {
		g := gomega.NewWithT(t)

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

		g.Expect(level3["password"]).To(gomega.Equal(RedactedPlaceholder))
		g.Expect(level3["region"]).To(gomega.Equal("us-east-1"))
	})

	t.Run("complex real-world adapter data", func(t *testing.T) {
		g := gomega.NewWithT(t)

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
		g.Expect(result["cluster_name"]).To(gomega.Equal("prod-cluster-1"))
		g.Expect(result["namespace"]).To(gomega.Equal("default"))
		g.Expect(result["region"]).To(gomega.Equal("us-west-2"))
		g.Expect(result["node_count"]).To(gomega.Equal(3))

		// Sensitive top-level fields redacted
		g.Expect(result["pull_secret"]).To(gomega.Equal(RedactedPlaceholder))
		g.Expect(result["admin_password"]).To(gomega.Equal(RedactedPlaceholder))

		// Nested sensitive field redacted
		sa := result["service_account"].(map[string]interface{})
		g.Expect(sa["name"]).To(gomega.Equal("hypershift-sa"))
		g.Expect(sa["privateKey"]).To(gomega.Equal(RedactedPlaceholder))
	})

	t.Run("arrays with sensitive fields in elements are masked", func(t *testing.T) {
		g := gomega.NewWithT(t)

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

		g.Expect(result["clusterName"]).To(gomega.Equal("test-cluster"))

		users := result["users"].([]interface{})
		g.Expect(users).To(gomega.HaveLen(2))

		// First user: password should be masked
		user0 := users[0].(map[string]interface{})
		g.Expect(user0["name"]).To(gomega.Equal("admin"))
		g.Expect(user0["password"]).To(gomega.Equal(RedactedPlaceholder))

		// Second user: token should be masked
		user1 := users[1].(map[string]interface{})
		g.Expect(user1["name"]).To(gomega.Equal("viewer"))
		g.Expect(user1["token"]).To(gomega.Equal(RedactedPlaceholder))
	})

	t.Run("nested arrays are recursively masked", func(t *testing.T) {
		g := gomega.NewWithT(t)

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
		g.Expect(env0["name"]).To(gomega.Equal("production"))

		providers := env0["providers"].([]interface{})
		provider0 := providers[0].(map[string]interface{})
		g.Expect(provider0["type"]).To(gomega.Equal("aws"))
		g.Expect(provider0["apiKey"]).To(gomega.Equal(RedactedPlaceholder))
		g.Expect(provider0["region"]).To(gomega.Equal("us-east-1"))
	})

	t.Run("arrays of primitives pass through unchanged", func(t *testing.T) {
		g := gomega.NewWithT(t)

		input := map[string]interface{}{
			"regions": []interface{}{"us-east-1", "us-west-2", "eu-west-1"},
			"ports":   []interface{}{80, 443, 8080},
		}

		result := MaskSensitiveFields(input)

		regions := result["regions"].([]interface{})
		g.Expect(regions).To(gomega.Equal([]interface{}{"us-east-1", "us-west-2", "eu-west-1"}))

		ports := result["ports"].([]interface{})
		g.Expect(ports).To(gomega.Equal([]interface{}{80, 443, 8080}))
	})

	t.Run("deeply nested arrays of arrays are masked", func(t *testing.T) {
		g := gomega.NewWithT(t)

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
		g.Expect(cell["value"]).To(gomega.Equal("data"))
		g.Expect(cell["password"]).To(gomega.Equal(RedactedPlaceholder))
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
			g := gomega.NewWithT(t)
			result := isSensitiveKey(tt.key)
			g.Expect(result).To(gomega.Equal(tt.sensitive))
		})
	}
}

// ============================================================================
// security Pattern Tests
// ============================================================================

func TestMaskSensitiveFields_SEC02Patterns(t *testing.T) {
	t.Parallel()
	t.Run("TLS certificate fields are masked", func(t *testing.T) {
		g := gomega.NewWithT(t)

		input := map[string]interface{}{
			"clusterName":     "prod-cluster",
			"tlsCert":         "-----BEGIN CERTIFICATE-----\nMIIC...",
			"clientCert":      "-----BEGIN CERTIFICATE-----\nMIID...",
			"caCertificate":   "-----BEGIN CERTIFICATE-----\nMIIE...",
			"serverCertChain": "multi-cert-chain",
		}

		result := MaskSensitiveFields(input)

		// Non-sensitive field preserved
		g.Expect(result["clusterName"]).To(gomega.Equal("prod-cluster"))

		// All cert fields should be masked
		g.Expect(result["tlsCert"]).To(gomega.Equal(RedactedPlaceholder))
		g.Expect(result["clientCert"]).To(gomega.Equal(RedactedPlaceholder))
		g.Expect(result["caCertificate"]).To(gomega.Equal(RedactedPlaceholder))
		g.Expect(result["serverCertChain"]).To(gomega.Equal(RedactedPlaceholder))
	})

	t.Run("kubeconfig fields are masked", func(t *testing.T) {
		g := gomega.NewWithT(t)

		input := map[string]interface{}{
			"clusterName":   "prod-cluster",
			"kubeconfig":    "apiVersion: v1\nclusters:\n- cluster:\n    certificate-authority-data: LS0t...",
			"kubeconfigRaw": "base64-encoded-kubeconfig-blob",
			"adminKubeconfig": map[string]interface{}{
				"data": "kubeconfig-content",
			},
		}

		result := MaskSensitiveFields(input)

		g.Expect(result["clusterName"]).To(gomega.Equal("prod-cluster"))
		g.Expect(result["kubeconfig"]).To(gomega.Equal(RedactedPlaceholder))
		g.Expect(result["kubeconfigRaw"]).To(gomega.Equal(RedactedPlaceholder))
		g.Expect(result["adminKubeconfig"]).To(gomega.Equal(RedactedPlaceholder))
	})

	t.Run("bearer token fields are masked", func(t *testing.T) {
		g := gomega.NewWithT(t)

		input := map[string]interface{}{
			"clusterName":  "prod-cluster",
			"bearerToken":  "fake-jwt-token-for-testing-not-real",
			"bearer":       "fake-bearer-token-for-testing",
			"bearerHeader": "Bearer fake-token-for-testing",
		}

		result := MaskSensitiveFields(input)

		g.Expect(result["clusterName"]).To(gomega.Equal("prod-cluster"))
		g.Expect(result["bearerToken"]).To(gomega.Equal(RedactedPlaceholder))
		g.Expect(result["bearer"]).To(gomega.Equal(RedactedPlaceholder))
		g.Expect(result["bearerHeader"]).To(gomega.Equal(RedactedPlaceholder))
	})

	t.Run("connection string fields are masked", func(t *testing.T) {
		g := gomega.NewWithT(t)

		input := map[string]interface{}{
			"dbHost":                "postgres.example.com",
			"connection_string":     "postgresql://user:pass@host:5432/dbname",
			"dbConnectionString":    "mysql://root:pass@localhost:3306/app",
			"redisConnectionString": "redis://:pass@host:6379/0",
		}

		result := MaskSensitiveFields(input)

		// Non-sensitive field preserved
		g.Expect(result["dbHost"]).To(gomega.Equal("postgres.example.com"))

		// All connection string fields should be masked (contain "connection")
		g.Expect(result["connection_string"]).To(gomega.Equal(RedactedPlaceholder))
		g.Expect(result["dbConnectionString"]).To(gomega.Equal(RedactedPlaceholder))
		g.Expect(result["redisConnectionString"]).To(gomega.Equal(RedactedPlaceholder))
	})

	t.Run("nested security patterns in adapter data", func(t *testing.T) {
		g := gomega.NewWithT(t)

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

		g.Expect(result["clusterName"]).To(gomega.Equal("prod-cluster"))

		// hypershift is not sensitive, but nested fields are
		hypershift := result["hypershift"].(map[string]interface{})
		g.Expect(hypershift["kubeconfig"]).To(gomega.Equal(RedactedPlaceholder))
		g.Expect(hypershift["pullSecrets"]).To(gomega.Equal(RedactedPlaceholder), "pullSecrets contains 'secret'")

		// tlsConfig is not sensitive, but nested cert fields are
		tlsConfig := result["tlsConfig"].(map[string]interface{})
		g.Expect(tlsConfig["caCert"]).To(gomega.Equal(RedactedPlaceholder))
		g.Expect(tlsConfig["clientCert"]).To(gomega.Equal(RedactedPlaceholder))
		g.Expect(tlsConfig["serverName"]).To(gomega.Equal("api.cluster.example.com"))
	})

	t.Run("security patterns in arrays", func(t *testing.T) {
		g := gomega.NewWithT(t)

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
		g.Expect(clusters).To(gomega.HaveLen(2))

		cluster1 := clusters[0].(map[string]interface{})
		g.Expect(cluster1["name"]).To(gomega.Equal("cluster-1"))
		g.Expect(cluster1["kubeconfig"]).To(gomega.Equal(RedactedPlaceholder))

		cluster2 := clusters[1].(map[string]interface{})
		g.Expect(cluster2["name"]).To(gomega.Equal("cluster-2"))
		g.Expect(cluster2["connection"]).To(gomega.Equal(RedactedPlaceholder), "connection contains 'connection'")
	})

	t.Run("real-world HyperShift adapter data with security patterns", func(t *testing.T) {
		g := gomega.NewWithT(t)

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
		g.Expect(result["clusterName"]).To(gomega.Equal("prod-hypershift-cluster"))
		g.Expect(result["namespace"]).To(gomega.Equal("clusters"))
		g.Expect(result["baseDomain"]).To(gomega.Equal("example.com"))

		// Sensitive fields masked
		g.Expect(result["pullSecret"]).To(gomega.Equal(RedactedPlaceholder))
		g.Expect(result["sshKey"]).To(gomega.Equal(RedactedPlaceholder), "sshKey contains 'key'")

		// hostedCluster contains kubeconfig (nested sensitive)
		hostedCluster := result["hostedCluster"].(map[string]interface{})
		g.Expect(hostedCluster["name"]).To(gomega.Equal("my-cluster"))
		g.Expect(hostedCluster["kubeconfig"]).To(gomega.Equal(RedactedPlaceholder))
	})
}

// ============================================================================
// SEC-03 Depth Limit Tests
// ============================================================================

func TestMaskSensitiveFields_DepthLimit(t *testing.T) {
	t.Parallel()
	t.Run("deeply nested structure stops at max depth (SEC-03)", func(t *testing.T) {
		g := gomega.NewWithT(t)

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
		g.Expect(result["level"]).To(gomega.Equal(0))
		g.Expect(result["data"]).To(gomega.Equal("value"))

		// Navigate down to depth 19 (last allowed level)
		current = result
		for i := 0; i < 19; i++ {
			nested, ok := current["nested"].(map[string]interface{})
			g.Expect(ok).To(gomega.BeTrue(), "Level %d should be a map", i)
			current = nested
		}

		// At depth 20, we should hit the limit and get an empty map
		nested, ok := current["nested"].(map[string]interface{})
		g.Expect(ok).To(gomega.BeTrue())
		g.Expect(nested).To(gomega.BeEmpty(), "Depth 20 should return empty map (limit reached)")
	})

	t.Run("deeply nested array stops at max depth (SEC-03)", func(t *testing.T) {
		g := gomega.NewWithT(t)

		// Build an array nested 25 levels deep
		deepest := []interface{}{"final"}
		for i := 0; i < 25; i++ {
			deepest = []interface{}{deepest}
		}

		result := maskSensitiveSliceDepth(deepest, 0)

		// Navigate down to depth 19
		currentSlice := result
		for i := 0; i < 19; i++ {
			g.Expect(currentSlice).To(gomega.HaveLen(1))
			next, ok := currentSlice[0].([]interface{})
			g.Expect(ok).To(gomega.BeTrue(), "Level %d should be a slice", i)
			currentSlice = next
		}

		// At depth 20, should get empty slice (limit reached)
		g.Expect(currentSlice).To(gomega.HaveLen(1))
		final, ok := currentSlice[0].([]interface{})
		g.Expect(ok).To(gomega.BeTrue())
		g.Expect(final).To(gomega.BeEmpty(), "Depth 20 should return empty slice (limit reached)")
	})

	t.Run("normal nested structures work fine (SEC-03)", func(t *testing.T) {
		g := gomega.NewWithT(t)

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
		g.Expect(level5["password"]).To(gomega.Equal(RedactedPlaceholder))
		g.Expect(level5["name"]).To(gomega.Equal("final"))
	})
}

// ============================================================================
// False Positive Prevention Tests
// ============================================================================

func TestMaskSensitiveFields_FalsePositivePrevention(t *testing.T) {
	t.Parallel()
	t.Run("database fields with 'key' are NOT redacted", func(t *testing.T) {
		g := gomega.NewWithT(t)

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
		g.Expect(result["partitionKey"]).To(gomega.Equal("user-123"))
		g.Expect(result["sortKey"]).To(gomega.Equal("timestamp"))
		g.Expect(result["primaryKey"]).To(gomega.Equal(42))
		g.Expect(result["foreignKey"]).To(gomega.Equal(99))
		g.Expect(result["uniqueKey"]).To(gomega.Equal("abc"))
		g.Expect(result["indexKey"]).To(gomega.Equal("idx_users_email"))
		g.Expect(result["cacheKey"]).To(gomega.Equal("session:xyz"))
		g.Expect(result["lookupKey"]).To(gomega.Equal("search-term"))
		g.Expect(result["routingKey"]).To(gomega.Equal("orders"))
		g.Expect(result["shardKey"]).To(gomega.Equal("region-us-west"))
	})

	t.Run("legitimate credential 'key' fields ARE redacted", func(t *testing.T) {
		g := gomega.NewWithT(t)

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
			g.Expect(result[k]).To(gomega.Equal(RedactedPlaceholder), "field %s should be redacted", k)
		}
	})

	t.Run("service/product names with 'key' are NOT redacted", func(t *testing.T) {
		g := gomega.NewWithT(t)

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
		g.Expect(result["keystoneEndpoint"]).To(gomega.Equal("https://keystone.example.com"))
		g.Expect(result["monkeyPatch"]).To(gomega.Equal(true))
		g.Expect(result["turkeyMode"]).To(gomega.Equal(false))
		g.Expect(result["keyValueStore"]).To(gomega.Equal("redis"))
		g.Expect(result["hotkey"]).To(gomega.Equal("Ctrl+S"))
		g.Expect(result["keyframe"]).To(gomega.Equal(60))
	})
}
