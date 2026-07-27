package util

import "strings"

// RedactedPlaceholder is the string used to replace sensitive field values (QUAL-01)
const RedactedPlaceholder = "***REDACTED***"

// maxMaskingDepth limits recursion depth to prevent stack overflow from
// deeply-nested adapter data payloads (SEC-03)
const maxMaskingDepth = 20

// Sensitive field patterns per HyperFleet security best practices (SEC-02)
// Matches common secret/credential field names case-insensitively
var sensitivePatterns = []string{
	"password",
	"secret",
	"token",
	// Specific key patterns (SEC-02: avoid false positives like "partitionKey", "sortKey")
	"apikey",        // API keys
	"privatekey",    // Private keys (SSH, TLS, etc.)
	"secretkey",     // Secret keys
	"accesskey",     // AWS access keys
	"sshkey",        // SSH keys
	"encryptionkey", // Encryption keys
	"accountkey",    // Service account keys (GCP, Azure)
	"servicekey",    // Service keys
	"registrykey",   // Container registry keys
	"signingkey",    // Code signing keys
	"credential",
	"api_key", // Snake_case variant
	"passphrase",
	// Broad patterns (SEC-02): intentional over-masking for defense-in-depth
	// Trade-off: May mask non-sensitive fields like "privateEndpoint", "authProvider", "connectionTimeout"
	// Decision: Prefer over-masking to under-masking for security (prevents credential leakage)
	"private",    // Private keys, private data - broad but catches "privateKey", "privateData"
	"auth",       // Auth tokens, auth keys - broad but catches "authToken", "authKey", "authorization"
	"connection", // Connection strings - broad but catches "connectionString", "dbConnection"
	"cert",       // TLS certificates (SEC-02)
	"kubeconfig", // Kubernetes config blobs (SEC-02)
	"bearer",     // Bearer tokens (SEC-02)
}

// MaskSensitiveFields redacts adapter data keys matching sensitive patterns
// before exposing to CEL evaluation context. This prevents accidental leakage
// of credentials in public API condition messages/reasons.
//
// Keys are checked case-insensitively against sensitivePatterns.
// Redacted values are replaced with RedactedPlaceholder.
//
// Recursion is limited to maxMaskingDepth to prevent stack overflow (SEC-03).
//
// Examples:
//   - "adminPassword" → redacted (contains "password")
//   - "pullSecret" → redacted (contains "secret")
//   - "gcpServiceAccountKey" → redacted (contains "accountkey")
//   - "clusterName" → NOT redacted (no sensitive pattern match)
func MaskSensitiveFields(data map[string]interface{}) map[string]interface{} {
	return maskSensitiveFieldsDepth(data, 0)
}

// maskSensitiveFieldsDepth is the internal implementation with depth tracking (SEC-03)
func maskSensitiveFieldsDepth(data map[string]interface{}, depth int) map[string]interface{} {
	if data == nil {
		return nil
	}

	// SEC-03: Stop recursion at maxMaskingDepth to prevent stack overflow
	// from malicious/malformed deeply-nested adapter payloads
	if depth >= maxMaskingDepth {
		// Return empty map at depth limit (safe degradation)
		return make(map[string]interface{})
	}

	masked := make(map[string]interface{}, len(data))
	for k, v := range data {
		if isSensitiveKey(k) {
			masked[k] = RedactedPlaceholder
		} else {
			// Recursively mask nested maps and slices
			if nestedMap, ok := v.(map[string]interface{}); ok {
				masked[k] = maskSensitiveFieldsDepth(nestedMap, depth+1)
			} else if nestedSlice, ok := v.([]interface{}); ok {
				masked[k] = maskSensitiveSliceDepth(nestedSlice, depth+1)
			} else {
				masked[k] = v
			}
		}
	}
	return masked
}

// maskSensitiveSliceDepth recursively masks sensitive fields in slice elements
// with depth tracking to prevent stack overflow (SEC-03)
func maskSensitiveSliceDepth(slice []interface{}, depth int) []interface{} {
	if slice == nil {
		return nil
	}

	// SEC-03: Stop recursion at maxMaskingDepth
	if depth >= maxMaskingDepth {
		// Return empty slice at depth limit (safe degradation)
		return []interface{}{}
	}

	masked := make([]interface{}, len(slice))
	for i, elem := range slice {
		// Recursively mask maps inside the slice
		if elemMap, ok := elem.(map[string]interface{}); ok {
			masked[i] = maskSensitiveFieldsDepth(elemMap, depth+1)
		} else if elemSlice, ok := elem.([]interface{}); ok {
			// Handle nested slices (arrays of arrays)
			masked[i] = maskSensitiveSliceDepth(elemSlice, depth+1)
		} else {
			masked[i] = elem
		}
	}
	return masked
}

// isSensitiveKey returns true if the key matches any sensitive pattern
func isSensitiveKey(key string) bool {
	lowerKey := strings.ToLower(key)
	for _, pattern := range sensitivePatterns {
		if strings.Contains(lowerKey, pattern) {
			return true
		}
	}
	return false
}
