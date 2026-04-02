package api

import (
	"fmt"

	"github.com/google/uuid"
)

// NewID generates a new RFC4122 UUID v7 identifier in lowercase format.
// UUID v7 embeds a Unix timestamp (millisecond precision) in the first 48 bits,
// providing time-ordering and improved database index performance.
// The resulting 36-character string (xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx)
// is suitable for use in REST APIs, database storage, and Kubernetes resource names.
//
// The lowercase UUID format is DNS-1123 compliant (contains only a-f, 0-9, and hyphens;
// starts and ends with alphanumeric; 36 chars < 253 char subdomain limit).
//
// Returns an error if UUID generation fails (extremely unlikely in practice).
func NewID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("failed to generate UUID v7: %w", err)
	}
	return id.String(), nil
}
