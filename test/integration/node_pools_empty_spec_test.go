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

func TestNodePoolPatch_EmptySpec(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	RegisterTestingT(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := test.GetAccessTokenFromContext(ctx)

	nodePool, err := h.Factories.NewNodePools(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	invalidInput := `{
		"spec": {}
	}`

	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(invalidInput).
		Patch(h.RestURL(fmt.Sprintf("/clusters/%s/nodepools/%s", nodePool.OwnerID, nodePool.ID)))

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

func TestNodePoolPost_EmptySpec(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	RegisterTestingT(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := test.GetAccessTokenFromContext(ctx)

	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

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
