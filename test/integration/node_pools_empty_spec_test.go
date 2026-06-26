package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"
	"gopkg.in/resty.v1"

	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

// TestNodePoolPost_EmptySpec tests that creating a nodepool with an empty spec {}
// returns 400 with HYPERFLEET-VAL-000 error code.
func TestNodePoolPost_EmptySpec(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	RegisterTestingT(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := test.GetAccessTokenFromContext(ctx)

	// Create a parent cluster first
	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Send request with empty spec
	invalidInput := `{
		"kind": "NodePool",
		"name": "test-nodepool-empty-spec",
		"spec": {}
	}`

	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(invalidInput).
		Post(h.RestURL(fmt.Sprintf("/clusters/%s/nodepools", cluster.ID)))

	Expect(err).ToNot(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))

	var errorResponse map[string]interface{}
	err = json.Unmarshal(restyResp.Body(), &errorResponse)
	Expect(err).ToNot(HaveOccurred())

	code, ok := errorResponse["code"].(string)
	Expect(ok).To(BeTrue())
	Expect(code).To(Equal("HYPERFLEET-VAL-000"))

	detail, ok := errorResponse["detail"].(string)
	Expect(ok).To(BeTrue())
	Expect(detail).To(ContainSubstring("spec must not be empty"))
}
