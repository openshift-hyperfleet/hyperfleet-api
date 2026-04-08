package api

import (
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewID(t *testing.T) {
	// Generate multiple IDs to test uniqueness, format, and length
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, err := NewID()
		if err != nil {
			t.Fatalf("NewID() returned error: %v", err)
		}

		// Verify length is 36 characters (UUID format with hyphens)
		if len(id) != 36 {
			t.Errorf("Expected ID length 36, got %d: %s", len(id), id)
		}

		// Verify UUID v7 format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
		if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`).MatchString(id) {
			t.Errorf("ID does not match UUID format: %s", id)
		}

		// Verify uniqueness
		if ids[id] {
			t.Errorf("Duplicate ID generated: %s", id)
		}
		ids[id] = true
	}
}

func TestNewID_TimeOrdering(t *testing.T) {
	// UUID v7 uses millisecond-level timestamps, providing better time-ordering
	// than KSUID (which used second-level timestamps). IDs generated sequentially
	// should have monotonically increasing timestamps embedded in them.

	id1, err := NewID()
	if err != nil {
		t.Fatalf("NewID() returned error: %v", err)
	}

	// Sleep briefly to ensure we cross a millisecond boundary
	time.Sleep(2 * time.Millisecond)

	id2, err := NewID()
	if err != nil {
		t.Fatalf("NewID() returned error: %v", err)
	}

	if len(id1) != 36 || len(id2) != 36 {
		t.Errorf("IDs should have consistent length of 36")
	}

	// Verify ID uniqueness
	if id1 == id2 {
		t.Errorf("IDs should be unique: %s == %s", id1, id2)
	}

	// Verify time ordering: parse UUIDs and compare timestamps
	uuid1, err1 := uuid.Parse(id1)
	uuid2, err2 := uuid.Parse(id2)

	if err1 != nil || err2 != nil {
		t.Errorf("Failed to parse UUIDs: %v, %v", err1, err2)
	}

	// UUID v7 stores timestamp in first 48 bits
	// We can compare the UUIDs as strings; due to the timestamp prefix,
	// lexicographic ordering matches time ordering for UUID v7
	if id1 >= id2 {
		t.Errorf("UUID v7 time-ordering failed: id1=%s should be < id2=%s", id1, id2)
	}

	// Verify they are valid UUID v7 (version field should be 7)
	if uuid1.Version() != 7 || uuid2.Version() != 7 {
		t.Errorf("Expected UUID version 7, got %d and %d", uuid1.Version(), uuid2.Version())
	}
}
