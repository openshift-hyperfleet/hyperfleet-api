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

// TestClusterPost_EmptySpec tests that creating a cluster with an empty spec {}
// returns 400 with HYPERFLEET-VAL-000 error code.
func TestClusterPost_EmptySpec(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	RegisterTestingT(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := test.GetAccessTokenFromContext(ctx)

	// Send request with empty spec
	invalidInput := `{
		"kind": "Cluster",
		"name": "test-cluster-empty-spec",
		"spec": {}
	}`

	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(invalidInput).
		Post(h.RestURL("/clusters"))

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

// TestClusterPatch_EmptySpec tests that patching a cluster with an empty spec {}
// returns 400 with HYPERFLEET-VAL-000 error code.
func TestClusterPatch_EmptySpec(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	RegisterTestingT(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := test.GetAccessTokenFromContext(ctx)

	// Create a cluster to patch
	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Send PATCH with empty spec
	invalidInput := `{
		"spec": {}
	}`

	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(invalidInput).
		Patch(h.RestURL(fmt.Sprintf("/clusters/%s", cluster.ID)))

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
