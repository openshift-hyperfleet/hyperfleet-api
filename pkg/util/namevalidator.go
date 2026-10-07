package util

import (
	"fmt"
	"regexp"
)

// ValidateResourceName checks if a resource name meets Kubernetes naming requirements.
// Names must be lowercase alphanumeric with hyphens, start and end with alphanumeric.
func ValidateResourceName(name string) error {
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}

	if len(name) > 253 {
		return fmt.Errorf("name too long: %d characters", len(name))
	}

	pattern := `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	matched, _ := regexp.MatchString(pattern, name)
	if !matched {
		return fmt.Errorf("invalid name format: %s", name)
	}

	return nil
}
