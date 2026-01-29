package integration

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
)

// TestAPIContract_ConditionStatusConstants verifies that domain and OpenAPI constants are in sync
// This test ensures that pkg/api.ConditionStatus constants match the generated openapi condition status constants
func TestAPIContract_ConditionStatusConstants(t *testing.T) {
	RegisterTestingT(t)

	// Verify domain constants match OpenAPI generated constants
	// If OpenAPI spec changes, this test will fail and alert us to update domain constants
	// Both AdapterConditionStatus and ResourceConditionStatus use the same values
	Expect(string(api.ConditionTrue)).To(Equal(string(openapi.AdapterConditionStatusTrue)),
		"api.ConditionTrue must match openapi.AdapterConditionStatusTrue")
	Expect(string(api.ConditionFalse)).To(Equal(string(openapi.AdapterConditionStatusFalse)),
		"api.ConditionFalse must match openapi.AdapterConditionStatusFalse")
	Expect(string(api.ConditionTrue)).To(Equal(string(openapi.ResourceConditionStatusTrue)),
		"api.ConditionTrue must match openapi.ResourceConditionStatusTrue")
	Expect(string(api.ConditionFalse)).To(Equal(string(openapi.ResourceConditionStatusFalse)),
		"api.ConditionFalse must match openapi.ResourceConditionStatusFalse")
}
