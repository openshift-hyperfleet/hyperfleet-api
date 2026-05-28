package integration

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/util"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

func shortClusterName(prefix, id string) string {
	suffix := strings.ReplaceAll(id, "-", "")
	if len(suffix) > 12 {
		suffix = suffix[:12]
	}
	return fmt.Sprintf("%s-%s", prefix, suffix)
}

func TestCallerIdentityCreate(t *testing.T) {
	cases := []struct {
		name        string
		namePrefix  string
		email       string
		setHeader   bool
		headerActor string
	}{
		{
			name:        "header present overrides JWT",
			namePrefix:  "ci-hdr",
			email:       "jwt-user@example.com",
			setHeader:   true,
			headerActor: "gateway-user@example.com",
		},
		{
			name:       "header absent uses JWT claim",
			namePrefix: "ci-jwt",
			email:      "jwt-only-user@example.com",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, client := test.RegisterIntegration(t)

			account := h.NewAccount(h.NewID(), "Test User", tc.email)
			ctx := h.NewAuthenticatedContext(account)

			wantCreatedBy := account.Email
			if tc.setHeader {
				wantCreatedBy = tc.headerActor
			}

			clusterInput := openapi.ClusterCreateRequest{
				Kind: util.PtrString("Cluster"),
				Name: shortClusterName(tc.namePrefix, h.NewID()),
				Spec: map[string]interface{}{"test": "spec"},
			}

			opts := []openapi.RequestEditorFn{test.WithAuthToken(ctx)}
			if tc.setHeader {
				opts = append(opts, test.WithIdentityHeader(test.IdentityHeaderName(), tc.headerActor))
			}

			resp, err := client.PostClusterWithResponse(
				ctx,
				openapi.PostClusterJSONRequestBody(clusterInput),
				opts...,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusCreated))
			Expect(resp.JSON201).NotTo(BeNil())
			Expect(string(resp.JSON201.CreatedBy)).To(Equal(wantCreatedBy))
		})
	}
}
