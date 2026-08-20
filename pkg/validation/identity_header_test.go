package validation

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestIsForbiddenIdentityHeaderName(t *testing.T) {
	RegisterTestingT(t)
	Expect(IsForbiddenIdentityHeaderName("Authorization")).To(BeTrue())
	Expect(IsForbiddenIdentityHeaderName("X-HyperFleet-Identity")).To(BeFalse())
}

func TestIsValidHeaderName(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name   string
		header string
		valid  bool
	}{
		{name: "accepts hyphenated header", header: "X-HyperFleet-Org", valid: true},
		{name: "accepts short hyphenated header", header: "X-Tenant", valid: true},
		{name: "rejects header with whitespace", header: "X Tenant", valid: false},
		{name: "rejects whitespace-only header", header: "   ", valid: false},
		{name: "rejects empty header", header: "", valid: false},
		{name: "rejects header with trailing tab", header: "X-Tenant\t", valid: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(IsValidHeaderName(tc.header)).To(Equal(tc.valid), "header %q", tc.header)
		})
	}
}
