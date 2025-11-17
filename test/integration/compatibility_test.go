package integration

import (
	"context"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

// TestCompatibilityGet tests the compatibility endpoint
func TestCompatibilityGet(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// GET /api/hyperfleet/v1/compatibility
	result, resp, err := client.DefaultAPI.GetCompatibility(ctx).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting compatibility: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(result).NotTo(BeEmpty())
}

// TestCompatibilityNoAuth tests that compatibility endpoint requires authentication
func TestCompatibilityNoAuth(t *testing.T) {
	_, client := test.RegisterIntegration(t)

	// Try to get compatibility without authentication
	_, _, err := client.DefaultAPI.GetCompatibility(context.Background()).Execute()
	Expect(err).To(HaveOccurred(), "Expected authentication error")
}
