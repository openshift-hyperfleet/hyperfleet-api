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

const (
	RedactedValue = "***REDACTED***"
	maxDepth      = 100
	maxBodySize   = 1 * 1024 * 1024 // 1MB
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
	apiKeyPattern = regexp.MustCompile(
		`(?i)(bearer\s+|api[_\-]?key[\s:=]+|token[\s:=]+|secret[\s:=]+|password[\s:=]+)[a-zA-Z0-9\-_\.+/=]+`,
	)
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

	text = emailPattern.ReplaceAllString(text, RedactedValue)
	text = creditCardPattern.ReplaceAllString(text, RedactedValue)

	text = apiKeyPattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := apiKeyPattern.FindStringSubmatch(match)
		if len(parts) > 1 {
			return parts[1] + RedactedValue
		}
		return RedactedValue
	})

	text = formEncodedPattern.ReplaceAllStringFunc(text, func(match string) string {
		idx := strings.Index(match, "=")
		if idx > 0 {
			return match[:idx+1] + RedactedValue
		}
		return RedactedValue
	})

	return []byte(text)
}

func (m *MaskingMiddleware) MaskHeaders(headers http.Header) http.Header {
	if !m.enabled {
		return headers
	}

	masked := make(http.Header)
	for key, values := range headers {
		if m.isSensitiveHeader(key) {
			redacted := make([]string, len(values))
			for i := range redacted {
				redacted[i] = RedactedValue
			}
			masked[key] = redacted
		} else {
			copied := make([]string, len(values))
			copy(copied, values)
			masked[key] = copied
		}
	}
	return masked
}

// MaskBody masks sensitive fields in JSON body
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

	if len(body) > maxBodySize {
		logger.With(context.Background()).Warn("Body too large for JSON masking, using text fallback")
		return m.maskTextFallback(body)
	}

	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		// JSON parsing failed - use text-based fallback masking to prevent leakage
		logger.WithError(context.Background(), err).
			Warn("JSON parsing failed in MaskBody, applying text-based fallback masking")
		return m.maskTextFallback(body)
	}

	m.maskRecursiveDepth(data, 0)

	masked, err := json.Marshal(data)
	if err != nil {
		// JSON marshaling failed - use text-based fallback masking to prevent leakage
		logger.WithError(context.Background(), err).
			Warn("JSON marshaling failed in MaskBody, applying text-based fallback masking")
		return m.maskTextFallback(body)
	}
	return masked
}

func (m *MaskingMiddleware) maskRecursiveDepth(data interface{}, depth int) {
	if depth > maxDepth {
		return
	}
	switch v := data.(type) {
	case map[string]interface{}:
		m.maskMapRecursiveDepth(v, depth)
	case []interface{}:
		for _, item := range v {
			m.maskRecursiveDepth(item, depth+1)
		}
	}
}

func (m *MaskingMiddleware) maskMapRecursiveDepth(data map[string]interface{}, depth int) {
	for key, value := range data {
		if m.isSensitiveField(key) {
			data[key] = RedactedValue
			continue
		}

		switch v := value.(type) {
		case map[string]interface{}:
			m.maskMapRecursiveDepth(v, depth+1)
		case []interface{}:
			for _, item := range v {
				m.maskRecursiveDepth(item, depth+1)
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
