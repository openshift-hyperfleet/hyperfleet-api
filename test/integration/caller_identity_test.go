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
		headerActor string
		setHeader   bool
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

func TestCallerIdentityPatch(t *testing.T) {
	cases := []struct {
		name          string
		namePrefix    string
		createEmail   string
		patchEmail    string
		headerActor   string
		wantUpdatedBy string
		setHeader     bool
	}{
		{
			name:          "patch with header updates updated_by",
			namePrefix:    "ci-patch-hdr",
			createEmail:   "creator@example.com",
			patchEmail:    "creator@example.com",
			setHeader:     true,
			headerActor:   "updater@example.com",
			wantUpdatedBy: "updater@example.com",
		},
		{
			name:          "patch without header uses JWT identity",
			namePrefix:    "ci-patch-jwt",
			createEmail:   "creator@example.com",
			patchEmail:    "patcher@example.com",
			wantUpdatedBy: "patcher@example.com",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, client := test.RegisterIntegration(t)

			// Create cluster with one identity.
			createAccount := h.NewAccount(h.NewID(), "Creator", tc.createEmail)
			createCtx := h.NewAuthenticatedContext(createAccount)

			clusterInput := openapi.ClusterCreateRequest{
				Kind: util.PtrString("Cluster"),
				Name: shortClusterName(tc.namePrefix, h.NewID()),
				Spec: map[string]interface{}{"test": "spec"},
			}

			createResp, err := client.PostClusterWithResponse(
				createCtx,
				openapi.PostClusterJSONRequestBody(clusterInput),
				test.WithAuthToken(createCtx),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(createResp.StatusCode()).To(Equal(http.StatusCreated))
			clusterID := createResp.JSON201.Id

			// Patch cluster with a different identity.
			patchAccount := h.NewAccount(h.NewID(), "Patcher", tc.patchEmail)
			patchCtx := h.NewAuthenticatedContext(patchAccount)

			newSpec := openapi.ClusterSpec{"test": "patched"}
			patchBody := openapi.PatchClusterByIdJSONRequestBody{Spec: &newSpec}

			opts := []openapi.RequestEditorFn{test.WithAuthToken(patchCtx)}
			if tc.setHeader {
				opts = append(opts, test.WithIdentityHeader(test.IdentityHeaderName(), tc.headerActor))
			}

			patchResp, err := client.PatchClusterByIdWithResponse(
				patchCtx,
				*clusterID,
				patchBody,
				opts...,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(patchResp.StatusCode()).To(Equal(http.StatusOK))
			Expect(patchResp.JSON200).NotTo(BeNil())
			Expect(string(patchResp.JSON200.UpdatedBy)).To(Equal(tc.wantUpdatedBy))
			// created_by must remain unchanged.
			Expect(string(patchResp.JSON200.CreatedBy)).To(Equal(tc.createEmail))
		})
	}
}
