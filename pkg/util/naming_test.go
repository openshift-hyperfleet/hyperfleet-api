package util

import (
	"testing"

	"github.com/onsi/gomega"
)

func TestMapAdapterToConditionType(t *testing.T) {
	t.Parallel()
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
			g := gomega.NewWithT(t)
			result := MapAdapterToConditionType(tt.adapter)
			g.Expect(result).To(gomega.Equal(tt.expected),
				"MapAdapterToConditionType(%q) should return %q", tt.adapter, tt.expected)
		})
	}
}
