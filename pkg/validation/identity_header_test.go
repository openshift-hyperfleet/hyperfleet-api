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
