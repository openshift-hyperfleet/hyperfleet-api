package util

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestMapAdapterToConditionType(t *testing.T) {
	testCases := []struct {
		adapter  string
		expected string
	}{
		{"validation", "ValidationSuccessful"},
		{"dns", "DnsSuccessful"},
		{"pullsecret", "PullsecretSuccessful"},
		{"hypershift", "HypershiftSuccessful"},
		{"my-adapter", "MyAdapterSuccessful"},
		{"multi-word-adapter", "MultiWordAdapterSuccessful"},
		{"a", "ASuccessful"},
		{"adapter1", "Adapter1Successful"},
	}

	for _, tt := range testCases {
		t.Run(tt.adapter, func(t *testing.T) {
			RegisterTestingT(t)
			result := MapAdapterToConditionType(tt.adapter)
			Expect(result).To(Equal(tt.expected),
				"MapAdapterToConditionType(%q) should return %q", tt.adapter, tt.expected)
		})
	}
}
