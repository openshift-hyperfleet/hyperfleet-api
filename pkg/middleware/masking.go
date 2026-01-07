package middleware

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
)

// MaskingMiddleware handles sensitive data masking for logging
// Masks sensitive HTTP headers and JSON body fields to prevent PII/secrets leakage
type MaskingMiddleware struct {
	enabled          bool
	sensitiveHeaders []string
	sensitiveFields  []string
}

// NewMaskingMiddleware creates a new masking middleware from logging config
func NewMaskingMiddleware(cfg *config.LoggingConfig) *MaskingMiddleware {
	return &MaskingMiddleware{
		enabled:          cfg.Masking.Enabled,
		sensitiveHeaders: cfg.GetSensitiveHeadersList(),
		sensitiveFields:  cfg.GetSensitiveFieldsList(),
	}
}

// MaskHeaders masks sensitive HTTP headers
// Returns a new header map with sensitive values replaced by "***REDACTED***"
func (m *MaskingMiddleware) MaskHeaders(headers http.Header) http.Header {
	if !m.enabled {
		return headers
	}

	masked := make(http.Header)
	for key, values := range headers {
		if m.isSensitiveHeader(key) {
			masked[key] = []string{"***REDACTED***"}
		} else {
			masked[key] = values
		}
	}
	return masked
}

// MaskBody masks sensitive fields in JSON body
// Returns a new JSON byte array with sensitive fields replaced by "***REDACTED***"
// If body is not valid JSON, returns original body unchanged
func (m *MaskingMiddleware) MaskBody(body []byte) []byte {
	if !m.enabled || len(body) == 0 {
		return body
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		// Not valid JSON, return unchanged
		return body
	}

	m.maskMapRecursive(data)

	masked, err := json.Marshal(data)
	if err != nil {
		// Failed to marshal, return original
		return body
	}
	return masked
}

// maskMapRecursive recursively masks sensitive fields in nested maps
func (m *MaskingMiddleware) maskMapRecursive(data map[string]interface{}) {
	for key, value := range data {
		if m.isSensitiveField(key) {
			data[key] = "***REDACTED***"
			continue
		}

		// Recursively handle nested objects
		switch v := value.(type) {
		case map[string]interface{}:
			m.maskMapRecursive(v)
		case []interface{}:
			// Handle arrays of objects
			for _, item := range v {
				if nestedMap, ok := item.(map[string]interface{}); ok {
					m.maskMapRecursive(nestedMap)
				}
			}
		}
	}
}

// isSensitiveHeader checks if a header name is sensitive (case-insensitive)
func (m *MaskingMiddleware) isSensitiveHeader(header string) bool {
	for _, sensitive := range m.sensitiveHeaders {
		if strings.EqualFold(header, sensitive) {
			return true
		}
	}
	return false
}

// isSensitiveField checks if a field name contains sensitive keywords (case-insensitive)
func (m *MaskingMiddleware) isSensitiveField(field string) bool {
	lower := strings.ToLower(field)
	for _, sensitive := range m.sensitiveFields {
		if strings.Contains(lower, strings.ToLower(sensitive)) {
			return true
		}
	}
	return false
}
