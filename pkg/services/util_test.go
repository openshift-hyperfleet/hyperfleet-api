package services

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestJSONEqual(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name     string
		a, b     []byte
		expected bool
	}{
		{
			name:     "identical bytes",
			a:        []byte(`{"a":1,"b":2}`),
			b:        []byte(`{"a":1,"b":2}`),
			expected: true,
		},
		{
			name:     "different key order same values",
			a:        []byte(`{"b":2,"a":1}`),
			b:        []byte(`{"a":1,"b":2}`),
			expected: true,
		},
		{
			name:     "nested objects different key order",
			a:        []byte(`{"x":{"b":2,"a":1},"y":3}`),
			b:        []byte(`{"y":3,"x":{"a":1,"b":2}}`),
			expected: true,
		},
		{
			name:     "different values",
			a:        []byte(`{"a":1}`),
			b:        []byte(`{"a":2}`),
			expected: false,
		},
		{
			name:     "extra key",
			a:        []byte(`{"a":1}`),
			b:        []byte(`{"a":1,"b":2}`),
			expected: false,
		},
		{
			name:     "arrays preserve order",
			a:        []byte(`[1,2,3]`),
			b:        []byte(`[1,2,3]`),
			expected: true,
		},
		{
			name:     "arrays different order not equal",
			a:        []byte(`[1,2,3]`),
			b:        []byte(`[3,2,1]`),
			expected: false,
		},
		{
			name:     "invalid json a",
			a:        []byte(`not json`),
			b:        []byte(`{"a":1}`),
			expected: false,
		},
		{
			name:     "invalid json b",
			a:        []byte(`{"a":1}`),
			b:        []byte(`not json`),
			expected: false,
		},
		{
			name:     "whitespace differences",
			a:        []byte(`{ "a" : 1 }`),
			b:        []byte(`{"a":1}`),
			expected: true,
		},
		{
			name:     "both nil",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "both empty",
			a:        []byte{},
			b:        []byte{},
			expected: true,
		},
		{
			name:     "nil and empty",
			a:        nil,
			b:        []byte{},
			expected: true,
		},
		{
			name:     "nil and valid json",
			a:        nil,
			b:        []byte(`{"a":1}`),
			expected: false,
		},
		{
			name:     "valid json and nil",
			a:        []byte(`{"a":1}`),
			b:        nil,
			expected: false,
		},
		{
			name:     "nil and json null",
			a:        nil,
			b:        []byte(`null`),
			expected: false,
		},
		{
			name:     "both json null",
			a:        []byte(`null`),
			b:        []byte(`null`),
			expected: true,
		},
		{
			name:     "both invalid json",
			a:        []byte(`broken`),
			b:        []byte(`broken`),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(jsonEqual(tt.a, tt.b)).To(Equal(tt.expected))
		})
	}
}
