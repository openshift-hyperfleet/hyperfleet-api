package integration

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
)

// TestAPIContract_ResourcePhaseConstants verifies that domain and OpenAPI constants are in sync
// This test ensures that pkg/api.ResourcePhase constants match the generated openapi.ResourcePhase constants
func TestAPIContract_ResourcePhaseConstants(t *testing.T) {
	RegisterTestingT(t)

	// Verify domain constants match OpenAPI generated constants
	// If OpenAPI spec changes, this test will fail and alert us to update domain constants
	Expect(string(api.PhaseNotReady)).To(Equal(string(openapi.NotReady)),
		"api.PhaseNotReady must match openapi.NotReady")
	Expect(string(api.PhaseReady)).To(Equal(string(openapi.Ready)),
		"api.PhaseReady must match openapi.Ready")
	Expect(string(api.PhaseFailed)).To(Equal(string(openapi.Failed)),
		"api.PhaseFailed must match openapi.Failed")
}

// TestAPIContract_ConditionStatusConstants verifies that domain and OpenAPI constants are in sync
// This test ensures that pkg/api.ConditionStatus constants match the generated openapi.ConditionStatus constants
func TestAPIContract_ConditionStatusConstants(t *testing.T) {
	RegisterTestingT(t)

	// Verify domain constants match OpenAPI generated constants
	// If OpenAPI spec changes, this test will fail and alert us to update domain constants
	Expect(string(api.ConditionTrue)).To(Equal(string(openapi.True)),
		"api.ConditionTrue must match openapi.True")
	Expect(string(api.ConditionFalse)).To(Equal(string(openapi.False)),
		"api.ConditionFalse must match openapi.False")
	Expect(string(api.ConditionUnknown)).To(Equal(string(openapi.Unknown)),
		"api.ConditionUnknown must match openapi.Unknown")
}
