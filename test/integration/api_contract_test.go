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
	Expect(string(api.PhaseNotReady)).To(Equal(string(openapi.NOT_READY)),
		"api.PhaseNotReady must match openapi.NOT_READY")
	Expect(string(api.PhaseReady)).To(Equal(string(openapi.READY)),
		"api.PhaseReady must match openapi.READY")
	Expect(string(api.PhaseFailed)).To(Equal(string(openapi.FAILED)),
		"api.PhaseFailed must match openapi.FAILED")
}

// TestAPIContract_ConditionStatusConstants verifies that domain and OpenAPI constants are in sync
// This test ensures that pkg/api.ConditionStatus constants match the generated openapi.ConditionStatus constants
func TestAPIContract_ConditionStatusConstants(t *testing.T) {
	RegisterTestingT(t)

	// Verify domain constants match OpenAPI generated constants
	// If OpenAPI spec changes, this test will fail and alert us to update domain constants
	Expect(string(api.ConditionTrue)).To(Equal(string(openapi.TRUE)),
		"api.ConditionTrue must match openapi.TRUE")
	Expect(string(api.ConditionFalse)).To(Equal(string(openapi.FALSE)),
		"api.ConditionFalse must match openapi.FALSE")
	Expect(string(api.ConditionUnknown)).To(Equal(string(openapi.UNKNOWN)),
		"api.ConditionUnknown must match openapi.UNKNOWN")
}
