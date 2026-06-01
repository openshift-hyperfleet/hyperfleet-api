package services

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(jsonEqual(tt.a, tt.b)).To(Equal(tt.expected))
		})
	}
}

func TestBuildAdapterSummaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		statuses api.AdapterStatusList
		expected []adapterSummary
	}{
		{
			name:     "empty input",
			statuses: api.AdapterStatusList{},
			expected: []adapterSummary{},
		},
		{
			name: "valid conditions",
			statuses: api.AdapterStatusList{
				{
					Adapter: "validation",
					Conditions: mustMarshal([]api.AdapterCondition{
						{Type: "Applied", Status: "True"},
						{Type: "Available", Status: "False"},
					}),
				},
			},
			expected: []adapterSummary{
				{Adapter: "validation", Conditions: map[string]string{"Applied": "True", "Available": "False"}},
			},
		},
		{
			name: "empty conditions field",
			statuses: api.AdapterStatusList{
				{Adapter: "provisioning", Conditions: nil},
			},
			expected: []adapterSummary{
				{Adapter: "provisioning", Conditions: map[string]string{}},
			},
		},
		{
			name: "malformed JSON falls back to empty map",
			statuses: api.AdapterStatusList{
				{Adapter: "broken", Conditions: []byte(`not valid json`)},
			},
			expected: []adapterSummary{
				{Adapter: "broken", Conditions: map[string]string{}},
			},
		},
		{
			name: "multiple adapters",
			statuses: api.AdapterStatusList{
				{
					Adapter:    "validation",
					Conditions: mustMarshal([]api.AdapterCondition{{Type: "Applied", Status: "True"}}),
				},
				{
					Adapter:    "provisioning",
					Conditions: mustMarshal([]api.AdapterCondition{{Type: "Reconciled", Status: "False"}}),
				},
			},
			expected: []adapterSummary{
				{Adapter: "validation", Conditions: map[string]string{"Applied": "True"}},
				{Adapter: "provisioning", Conditions: map[string]string{"Reconciled": "False"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			result := buildAdapterSummaries(context.Background(), tt.statuses)
			Expect(result).To(Equal(tt.expected))
		})
	}
}

func mustMarshal(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
