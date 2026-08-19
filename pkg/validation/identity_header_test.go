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
		name  string
		valid bool
	}{
		{name: "X-HyperFleet-Org", valid: true},
		{name: "X-Tenant", valid: true},
		{name: "X Tenant", valid: false},   // contains whitespace
		{name: "   ", valid: false},        // whitespace-only
		{name: "", valid: false},           // empty
		{name: "X-Tenant\t", valid: false}, // trailing tab
	}

	for _, tc := range cases {
		Expect(IsValidHeaderName(tc.name)).To(Equal(tc.valid), "name %q", tc.name)
	}
}
