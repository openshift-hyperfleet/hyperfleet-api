package db

import (
	"net/http"
	"testing"
)

func TestIsWriteMethod_StandardHTTPMethods(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		expected bool
	}{
		// Write methods
		{"POST is write", http.MethodPost, true},
		{"PUT is write", http.MethodPut, true},
		{"PATCH is write", http.MethodPatch, true},
		{"DELETE is write", http.MethodDelete, true},

		// Read methods
		{"GET is read-only", http.MethodGet, false},
		{"HEAD is read-only", http.MethodHead, false},
		{"OPTIONS is read-only", http.MethodOptions, false},

		// Edge cases
		{"CONNECT is read-only (conservative)", http.MethodConnect, false},
		{"TRACE is read-only (conservative)", http.MethodTrace, false},
		{"Empty string is read-only", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isWriteMethod(tt.method)
			if result != tt.expected {
				t.Errorf("isWriteMethod(%q) = %v, want %v", tt.method, result, tt.expected)
			}
		})
	}
}

func TestIsWriteMethod_NonStandardMethods(t *testing.T) {
	// WebDAV and other non-standard methods
	// These are treated as read-only (false) which is safe because:
	// 1. hyperfleet-api router doesn't accept these methods (405 at routing layer)
	// 2. If they somehow reach here, no transaction is conservative but acceptable
	nonStandardMethods := []string{
		"PROPFIND",
		"PROPPATCH",
		"MKCOL",
		"COPY",
		"MOVE",
		"LOCK",
		"UNLOCK",
		"CUSTOM",
	}

	for _, method := range nonStandardMethods {
		t.Run(method, func(t *testing.T) {
			result := isWriteMethod(method)
			if result != false {
				t.Errorf("isWriteMethod(%q) = %v, want false (non-standard methods default to read-only)", method, result)
			}
		})
	}
}
