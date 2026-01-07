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
				"Authorization": []string{"***REDACTED***"},
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
				"Authorization": []string{"***REDACTED***"},
				"X-Api-Key":     []string{"***REDACTED***"},
				"Cookie":        []string{"***REDACTED***"},
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
				"authorization": []string{"***REDACTED***"},
				"COOKIE":        []string{"***REDACTED***"},
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
				} else if len(resultValues) != len(expectedValues) || resultValues[0] != expectedValues[0] {
					t.Errorf("header %s = %v, want %v", key, resultValues, expectedValues)
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
			name:    "non-JSON body unchanged",
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

			// For JSON, compare as maps to handle key ordering
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
		} else if a[i] != b[i] {
			return false
		}
	}

	return true
}
