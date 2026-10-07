package util

import "testing"

func TestValidateResourceName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{
			name:      "valid lowercase with hyphens",
			input:     "valid-name-123",
			wantError: false,
		},
		{
			name:      "valid single character",
			input:     "a",
			wantError: false,
		},
		{
			name:      "too long name",
			input:     "a" + string(make([]byte, 253)),
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateResourceName(tt.input)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateResourceName() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}
