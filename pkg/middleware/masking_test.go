package middleware

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
)

func TestMaskHeaders(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		headers  http.Header
		expected http.Header
	}{
		{
			name:    "mask authorization header",
			enabled: true,
			headers: http.Header{
				"Authorization": []string{"Bearer token123"},
				"Content-Type":  []string{"application/json"},
			},
			expected: http.Header{
				"Authorization": []string{RedactedValue},
				"Content-Type":  []string{"application/json"},
			},
		},
		{
			name:    "mask multiple sensitive headers",
			enabled: true,
			headers: http.Header{
				"Authorization": []string{"Bearer token123"},
				"X-Api-Key":     []string{"secret-key"},
				"Cookie":        []string{"session=abc123"},
				"User-Agent":    []string{"curl/7.0"},
			},
			expected: http.Header{
				"Authorization": []string{RedactedValue},
				"X-Api-Key":     []string{RedactedValue},
				"Cookie":        []string{RedactedValue},
				"User-Agent":    []string{"curl/7.0"},
			},
		},
		{
			name:    "case insensitive header matching",
			enabled: true,
			headers: http.Header{
				"authorization": []string{"Bearer token123"},
				"COOKIE":        []string{"session=abc123"},
			},
			expected: http.Header{
				"authorization": []string{RedactedValue},
				"COOKIE":        []string{RedactedValue},
			},
		},
		{
			name:    "masking disabled",
			enabled: false,
			headers: http.Header{
				"Authorization": []string{"Bearer token123"},
			},
			expected: http.Header{
				"Authorization": []string{"Bearer token123"},
			},
		},
		{
			name:    "mask multi-value headers",
			enabled: true,
			headers: http.Header{
				"Cookie":      []string{"session=abc123", "tracking=xyz789", "preferences=dark"},
				"X-Api-Key":   []string{"key1", "key2"},
				"User-Agent":  []string{"browser/1.0"},
			},
			expected: http.Header{
				"Cookie":      []string{RedactedValue, RedactedValue, RedactedValue},
				"X-Api-Key":   []string{RedactedValue, RedactedValue},
				"User-Agent":  []string{"browser/1.0"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.LoggingConfig{
				Masking: config.MaskingConfig{
					Enabled:          tt.enabled,
					SensitiveHeaders: "Authorization,X-API-Key,Cookie",
				},
			}
			m := NewMaskingMiddleware(cfg)
			result := m.MaskHeaders(tt.headers)

			for key, expectedValues := range tt.expected {
				if resultValues, ok := result[key]; !ok {
					t.Errorf("expected header %s not found in result", key)
				} else if len(resultValues) != len(expectedValues) {
					t.Errorf("header %s length = %d, want %d (values: %v vs %v)", key, len(resultValues), len(expectedValues), resultValues, expectedValues)
				} else {
					// Compare all values in the slice
					for i := range expectedValues {
						if resultValues[i] != expectedValues[i] {
							t.Errorf("header %s = %v, want %v", key, resultValues, expectedValues)
							break
						}
					}
				}
			}
		})
	}
}

func TestMaskBody(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		body     string
		expected string
	}{
		{
			name:    "mask password field",
			enabled: true,
			body:    `{"username":"alice","password":"secret123"}`,
			expected: `{"password":"***REDACTED***","username":"alice"}`,
		},
		{
			name:    "mask nested sensitive fields",
			enabled: true,
			body:    `{"user":{"name":"alice","password":"secret"},"api_key":"key123"}`,
			expected: `{"api_key":"***REDACTED***","user":{"name":"alice","password":"***REDACTED***"}}`,
		},
		{
			name:    "mask array of objects",
			enabled: true,
			body:    `{"users":[{"name":"alice","password":"pass1"},{"name":"bob","secret":"pass2"}]}`,
			expected: `{"users":[{"name":"alice","password":"***REDACTED***"},{"name":"bob","secret":"***REDACTED***"}]}`,
		},
		{
			name:    "mask top-level array with sensitive fields",
			enabled: true,
			body:    `[{"name":"alice","password":"secret1"},{"name":"bob","token":"secret2"}]`,
			expected: `[{"name":"alice","password":"***REDACTED***"},{"name":"bob","token":"***REDACTED***"}]`,
		},
		{
			name:    "mask nested arrays with sensitive fields",
			enabled: true,
			body:    `[[{"password":"secret1"}],[{"api_key":"secret2"}]]`,
			expected: `[[{"password":"***REDACTED***"}],[{"api_key":"***REDACTED***"}]]`,
		},
		{
			name:    "mask nested arrays inside map values",
			enabled: true,
			body:    `{"users":[[{"password":"secret1"}]],"data":[[{"token":"secret2"}]]}`,
			expected: `{"data":[[{"token":"***REDACTED***"}]],"users":[[{"password":"***REDACTED***"}]]}`,
		},
		{
			name:    "mask multiple sensitive fields",
			enabled: true,
			body:    `{"password":"pass","secret":"sec","token":"tok","api_key":"key","normal":"value"}`,
			expected: `{"api_key":"***REDACTED***","normal":"value","password":"***REDACTED***","secret":"***REDACTED***","token":"***REDACTED***"}`,
		},
		{
			name:    "case insensitive field matching",
			enabled: true,
			body:    `{"Password":"pass","SECRET":"sec","AccessToken":"tok"}`,
			expected: `{"AccessToken":"***REDACTED***","Password":"***REDACTED***","SECRET":"***REDACTED***"}`,
		},
		{
			name:    "non-JSON body without sensitive data unchanged",
			enabled: true,
			body:    `not json content`,
			expected: `not json content`,
		},
		{
			name:    "empty body",
			enabled: true,
			body:    ``,
			expected: ``,
		},
		{
			name:    "masking disabled",
			enabled: false,
			body:    `{"password":"secret"}`,
			expected: `{"password":"secret"}`,
		},
		// Fallback masking tests (non-JSON content with sensitive data)
		{
			name:    "fallback: redact email addresses",
			enabled: true,
			body:    `User email: alice@example.com, contact bob.smith@company.co.uk`,
			expected: `User email: ***REDACTED***, contact ***REDACTED***`,
		},
		{
			name:    "fallback: redact credit card numbers",
			enabled: true,
			body:    `Card: 4532-1234-5678-9010 and 5425233430109903`,
			expected: `Card: ***REDACTED*** and ***REDACTED***`,
		},
		{
			name:    "fallback: redact Bearer tokens",
			enabled: true,
			body:    `Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9`,
			expected: `Authorization: Bearer ***REDACTED***`,
		},
		{
			name:    "fallback: redact API keys",
			enabled: true,
			body:    `API_KEY=sk_test_123456789abcdef and api-key: prod_key_xyz`,
			expected: `API_KEY=***REDACTED*** and api-key: ***REDACTED***`,
		},
		{
			name:    "fallback: redact form-encoded passwords",
			enabled: true,
			body:    `username=alice&password=secret123&email=test@example.com`,
			expected: `username=alice&password=***REDACTED***&email=***REDACTED***`,
		},
		{
			name:    "fallback: redact multiple sensitive patterns",
			enabled: true,
			body:    `User: alice@example.com, Token: secret_abc123, CC: 4532123456789010`,
			expected: `User: ***REDACTED***, Token: ***REDACTED***, CC: ***REDACTED***`,
		},
		{
			name:    "fallback: disabled masking returns original",
			enabled: false,
			body:    `password=secret123&api_key=test_key&user@example.com`,
			expected: `password=secret123&api_key=test_key&user@example.com`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.LoggingConfig{
				Masking: config.MaskingConfig{
					Enabled:         tt.enabled,
					SensitiveFields: "password,secret,token,api_key,access_token",
				},
			}
			m := NewMaskingMiddleware(cfg)
			result := m.MaskBody([]byte(tt.body))

			// For JSON objects, compare as maps to handle key ordering
			if tt.body != "" && tt.body[0] == '{' {
				var resultMap, expectedMap map[string]interface{}
				if err := json.Unmarshal(result, &resultMap); err != nil {
					t.Fatalf("failed to unmarshal result: %v", err)
				}
				if err := json.Unmarshal([]byte(tt.expected), &expectedMap); err != nil {
					t.Fatalf("failed to unmarshal expected: %v", err)
				}

				// Deep comparison
				if !deepEqual(resultMap, expectedMap) {
					t.Errorf("MaskBody() = %s, want %s", result, tt.expected)
				}
			} else if tt.body != "" && tt.body[0] == '[' {
				// For JSON arrays, compare as slices
				var resultArray, expectedArray []interface{}
				if err := json.Unmarshal(result, &resultArray); err != nil {
					t.Fatalf("failed to unmarshal result array: %v", err)
				}
				if err := json.Unmarshal([]byte(tt.expected), &expectedArray); err != nil {
					t.Fatalf("failed to unmarshal expected array: %v", err)
				}

				// Deep comparison
				if !deepEqualSlice(resultArray, expectedArray) {
					t.Errorf("MaskBody() = %s, want %s", result, tt.expected)
				}
			} else {
				// For non-JSON, compare as strings
				if string(result) != tt.expected {
					t.Errorf("MaskBody() = %s, want %s", result, tt.expected)
				}
			}
		})
	}
}

func TestIsSensitiveHeader(t *testing.T) {
	cfg := &config.LoggingConfig{
		Masking: config.MaskingConfig{
			Enabled:          true,
			SensitiveHeaders: "Authorization,X-API-Key,Cookie",
		},
	}
	m := NewMaskingMiddleware(cfg)

	tests := []struct {
		header   string
		expected bool
	}{
		{"Authorization", true},
		{"authorization", true},
		{"AUTHORIZATION", true},
		{"X-API-Key", true},
		{"x-api-key", true},
		{"Cookie", true},
		{"Content-Type", false},
		{"User-Agent", false},
	}

	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			result := m.isSensitiveHeader(tt.header)
			if result != tt.expected {
				t.Errorf("isSensitiveHeader(%s) = %v, want %v", tt.header, result, tt.expected)
			}
		})
	}
}

func TestIsSensitiveField(t *testing.T) {
	cfg := &config.LoggingConfig{
		Masking: config.MaskingConfig{
			Enabled:         true,
			SensitiveFields: "password,secret,token,api_key",
		},
	}
	m := NewMaskingMiddleware(cfg)

	tests := []struct {
		field    string
		expected bool
	}{
		{"password", true},
		{"Password", true},
		{"user_password", true},
		{"secret", true},
		{"client_secret", true},
		{"token", true},
		{"access_token", true},
		{"api_key", true},
		{"username", false},
		{"email", false},
		{"normal_field", false},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			result := m.isSensitiveField(tt.field)
			if result != tt.expected {
				t.Errorf("isSensitiveField(%s) = %v, want %v", tt.field, result, tt.expected)
			}
		})
	}
}

// deepEqual compares two maps recursively
func deepEqual(a, b map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}

	for key, aVal := range a {
		bVal, ok := b[key]
		if !ok {
			return false
		}

		switch aTyped := aVal.(type) {
		case map[string]interface{}:
			bTyped, ok := bVal.(map[string]interface{})
			if !ok || !deepEqual(aTyped, bTyped) {
				return false
			}
		case []interface{}:
			bTyped, ok := bVal.([]interface{})
			if !ok || !deepEqualSlice(aTyped, bTyped) {
				return false
			}
		default:
			if aVal != bVal {
				return false
			}
		}
	}

	return true
}

// deepEqualSlice compares two slices recursively
func deepEqualSlice(a, b []interface{}) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		aMap, aIsMap := a[i].(map[string]interface{})
		bMap, bIsMap := b[i].(map[string]interface{})

		if aIsMap && bIsMap {
			if !deepEqual(aMap, bMap) {
				return false
			}
		} else {
			aSlice, aIsSlice := a[i].([]interface{})
			bSlice, bIsSlice := b[i].([]interface{})

			if aIsSlice && bIsSlice {
				if !deepEqualSlice(aSlice, bSlice) {
					return false
				}
			} else if a[i] != b[i] {
				return false
			}
		}
	}

	return true
}
