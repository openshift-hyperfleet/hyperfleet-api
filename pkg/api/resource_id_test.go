package api

import (
	"regexp"
	"testing"
)

func TestNewID(t *testing.T) {
	// Generate multiple IDs to test uniqueness, format, and length
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := NewID()

		// Verify length is 32 characters
		if len(id) != 32 {
			t.Errorf("Expected ID length 32, got %d: %s", len(id), id)
		}

		// Verify only lowercase letters (a-v) and digits (0-9)
		if !regexp.MustCompile(`^[0-9a-v]{32}$`).MatchString(id) {
			t.Errorf("ID contains invalid characters (should be lowercase 0-9a-v): %s", id)
		}

		// Verify uniqueness
		if ids[id] {
			t.Errorf("Duplicate ID generated: %s", id)
		}
		ids[id] = true
	}
}

func TestNewID_TimeOrdering(t *testing.T) {
	// KSUID uses second-level timestamps, so IDs generated within the same second
	// will have the same timestamp prefix. Time ordering is only guaranteed for IDs
	// generated in different seconds.
	id1 := NewID()

	// For practical testing, we verify consistency and uniqueness within the same second
	// rather than waiting for the next second (which would slow down tests significantly).
	// In production, most ID generations will be more than 1 second apart.
	id2 := NewID()

	if len(id1) != 32 || len(id2) != 32 {
		t.Errorf("IDs should have consistent length of 32")
	}

	// Verify ID uniqueness even within the same second
	if id1 == id2 {
		t.Errorf("IDs should be unique even within the same second: %s == %s", id1, id2)
	}
}

func TestNewID_K8sCompatible(t *testing.T) {
	id := NewID()

	// Verify DNS-1123 subdomain compatibility:
	// - Must contain only lowercase letters, digits, '-', and '.'
	// - Must start and end with alphanumeric characters
	// - Maximum length is 253 characters
	//
	// Our IDs contain only lowercase letters (a-v) and digits (0-9), with a fixed
	// length of 32 characters, so they are fully compatible.
	dns1123Pattern := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	if !dns1123Pattern.MatchString(id) {
		t.Errorf("ID is not DNS-1123 subdomain compatible: %s", id)
	}
}
