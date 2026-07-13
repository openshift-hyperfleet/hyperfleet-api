package api

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

func TestValidateLabel(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		value       string
		expectError bool
	}{
		{
			name:        "empty key returns error",
			key:         "",
			value:       "v",
			expectError: true,
		},
		{
			name:        "valid key and value",
			key:         "env",
			value:       "prod",
			expectError: false,
		},
		{
			name:        "key at exactly 255 chars",
			key:         strings.Repeat("k", MaxLabelKeyLen),
			value:       "v",
			expectError: false,
		},
		{
			name:        "key at 256 chars returns error",
			key:         strings.Repeat("k", MaxLabelKeyLen+1),
			value:       "v",
			expectError: true,
		},
		{
			name:        "value at exactly 255 chars",
			key:         "env",
			value:       strings.Repeat("v", MaxLabelValueLen),
			expectError: false,
		},
		{
			name:        "value at 256 chars returns error",
			key:         "env",
			value:       strings.Repeat("v", MaxLabelValueLen+1),
			expectError: true,
		},
		{
			name:        "empty value is allowed",
			key:         "env",
			value:       "",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			err := ValidateLabel(tt.key, tt.value)

			if tt.expectError {
				Expect(err).ToNot(BeNil())
				return
			}

			Expect(err).To(BeNil())
		})
	}
}
