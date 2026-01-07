package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// MaskingMiddleware handles sensitive data masking for logging
// Masks sensitive HTTP headers and JSON body fields to prevent PII/secrets leakage
type MaskingMiddleware struct {
	enabled          bool
	sensitiveHeaders []string
	sensitiveFields  []string
}

var (
	// Regex patterns for text-based fallback masking
	emailPattern      = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	creditCardPattern = regexp.MustCompile(`\b\d{4}[\s\-]?\d{4}[\s\-]?\d{4}[\s\-]?\d{4}\b`)
	// Matches common API key/token formats: Bearer tokens, API keys, etc.
	apiKeyPattern = regexp.MustCompile(`(?i)(bearer\s+|api[_\-]?key[\s:=]+|token[\s:=]+|secret[\s:=]+|password[\s:=]+)[a-zA-Z0-9\-_\.]+`)
	// Matches form-encoded sensitive key-values
	formEncodedPattern = regexp.MustCompile(`(?i)(password|token|secret|api[_\-]?key)=[^&\s]+`)
)

// NewMaskingMiddleware creates a new masking middleware from logging config
func NewMaskingMiddleware(cfg *config.LoggingConfig) *MaskingMiddleware {
	return &MaskingMiddleware{
		enabled:          cfg.Masking.Enabled,
		sensitiveHeaders: cfg.GetSensitiveHeadersList(),
		sensitiveFields:  cfg.GetSensitiveFieldsList(),
	}
}

// maskTextFallback applies text-based regex heuristics to redact sensitive data
// Used as fallback when JSON parsing fails
func (m *MaskingMiddleware) maskTextFallback(body []byte) []byte {
	text := string(body)

	// Redact email addresses
	text = emailPattern.ReplaceAllString(text, "***REDACTED_EMAIL***")

	// Redact credit card numbers
	text = creditCardPattern.ReplaceAllString(text, "***REDACTED_CC***")

	// Redact API keys and tokens (keep the prefix for context)
	text = apiKeyPattern.ReplaceAllStringFunc(text, func(match string) string {
		// Extract and preserve the prefix (e.g., "bearer ", "api_key=")
		parts := apiKeyPattern.FindStringSubmatch(match)
		if len(parts) > 1 {
			return parts[1] + "***REDACTED***"
		}
		return "***REDACTED***"
	})

	// Redact form-encoded sensitive values
	text = formEncodedPattern.ReplaceAllStringFunc(text, func(match string) string {
		// Keep the key name but redact the value
		idx := strings.Index(match, "=")
		if idx > 0 {
			return match[:idx+1] + "***REDACTED***"
		}
		return "***REDACTED***"
	})

	return []byte(text)
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
// Handles both top-level objects and top-level arrays
// If masking is disabled, returns original body unchanged
// If masking is enabled but JSON parsing fails, applies text-based fallback masking
func (m *MaskingMiddleware) MaskBody(body []byte) []byte {
	if len(body) == 0 {
		return body
	}

	// If masking is disabled, return original body
	if !m.enabled {
		return body
	}

	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		// JSON parsing failed - use text-based fallback masking to prevent leakage
		logger.Warn(context.Background(), "JSON parsing failed in MaskBody, applying text-based fallback masking",
			"error", err.Error())
		return m.maskTextFallback(body)
	}

	m.maskRecursive(data)

	masked, err := json.Marshal(data)
	if err != nil {
		// JSON marshaling failed - use text-based fallback masking to prevent leakage
		logger.Warn(context.Background(), "JSON marshaling failed in MaskBody, applying text-based fallback masking",
			"error", err.Error())
		return m.maskTextFallback(body)
	}
	return masked
}

// maskRecursive recursively masks sensitive data in any JSON structure
func (m *MaskingMiddleware) maskRecursive(data interface{}) {
	switch v := data.(type) {
	case map[string]interface{}:
		m.maskMapRecursive(v)
	case []interface{}:
		for _, item := range v {
			m.maskRecursive(item)
		}
	}
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
