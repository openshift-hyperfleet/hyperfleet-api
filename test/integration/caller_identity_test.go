package integration

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"

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
				Kind: "Cluster",
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
				Kind: "Cluster",
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

func TestCallerIdentityMultiplePatches(t *testing.T) {
	RegisterTestingT(t)

	h, client := test.RegisterIntegration(t)

	// Create cluster as user-a.
	accountA := h.NewAccount(h.NewID(), "User A", "user-a@example.com")
	ctxA := h.NewAuthenticatedContext(accountA)

	clusterInput := openapi.ClusterCreateRequest{
		Kind: "Cluster",
		Name: shortClusterName("ci-multi", h.NewID()),
		Spec: map[string]interface{}{"version": "1"},
	}
	createResp, err := client.PostClusterWithResponse(
		ctxA, openapi.PostClusterJSONRequestBody(clusterInput), test.WithAuthToken(ctxA),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(createResp.StatusCode()).To(Equal(http.StatusCreated))
	clusterID := *createResp.JSON201.Id

	Expect(string(createResp.JSON201.CreatedBy)).To(Equal("user-a@example.com"))
	Expect(string(createResp.JSON201.UpdatedBy)).To(Equal("user-a@example.com"))
	Expect(createResp.JSON201.Generation).To(Equal(int32(1)))

	// Patch 1: user-b via JWT.
	accountB := h.NewAccount(h.NewID(), "User B", "user-b@example.com")
	ctxB := h.NewAuthenticatedContext(accountB)

	spec2 := openapi.ClusterSpec{"version": "2"}
	patch1, err := client.PatchClusterByIdWithResponse(
		ctxB, clusterID,
		openapi.PatchClusterByIdJSONRequestBody{Spec: &spec2},
		test.WithAuthToken(ctxB),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(patch1.StatusCode()).To(Equal(http.StatusOK))

	Expect(string(patch1.JSON200.CreatedBy)).To(Equal("user-a@example.com"), "created_by must never change")
	Expect(string(patch1.JSON200.UpdatedBy)).To(Equal("user-b@example.com"))
	Expect(patch1.JSON200.Generation).To(Equal(int32(2)))

	// Patch 2: user-c via identity header.
	spec3 := openapi.ClusterSpec{"version": "3"}
	patch2, err := client.PatchClusterByIdWithResponse(
		ctxA, clusterID,
		openapi.PatchClusterByIdJSONRequestBody{Spec: &spec3},
		test.WithAuthToken(ctxA),
		test.WithIdentityHeader(test.IdentityHeaderName(), "user-c@gateway.com"),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(patch2.StatusCode()).To(Equal(http.StatusOK))

	Expect(string(patch2.JSON200.CreatedBy)).To(Equal("user-a@example.com"), "created_by must never change")
	Expect(string(patch2.JSON200.UpdatedBy)).To(Equal("user-c@gateway.com"))
	Expect(patch2.JSON200.Generation).To(Equal(int32(3)))

	// GET confirms persisted state.
	getResp, err := client.GetClusterByIdWithResponse(ctxA, clusterID, nil, test.WithAuthToken(ctxA))
	Expect(err).NotTo(HaveOccurred())
	Expect(getResp.StatusCode()).To(Equal(http.StatusOK))
	Expect(string(getResp.JSON200.CreatedBy)).To(Equal("user-a@example.com"))
	Expect(string(getResp.JSON200.UpdatedBy)).To(Equal("user-c@gateway.com"))
	Expect(getResp.JSON200.Generation).To(Equal(int32(3)))
}

func TestCallerIdentityDelete(t *testing.T) {
	RegisterTestingT(t)

	h, client := test.RegisterIntegration(t)

	// Create cluster with identity header.
	account := h.NewAccount(h.NewID(), "Creator", "creator@example.com")
	ctx := h.NewAuthenticatedContext(account)

	clusterInput := openapi.ClusterCreateRequest{
		Kind: "Cluster",
		Name: shortClusterName("ci-del", h.NewID()),
		Spec: map[string]interface{}{"test": "spec"},
	}
	createResp, err := client.PostClusterWithResponse(
		ctx, openapi.PostClusterJSONRequestBody(clusterInput),
		test.WithAuthToken(ctx),
		test.WithIdentityHeader(test.IdentityHeaderName(), "header-creator@corp.com"),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(createResp.StatusCode()).To(Equal(http.StatusCreated))
	clusterID := *createResp.JSON201.Id
	Expect(string(createResp.JSON201.CreatedBy)).To(Equal("header-creator@corp.com"))

	// Soft-delete the cluster.
	delResp, err := client.DeleteClusterByIdWithResponse(ctx, clusterID, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(delResp.StatusCode()).To(Equal(http.StatusAccepted))
	Expect(delResp.JSON202).NotTo(BeNil())
	Expect(delResp.JSON202.DeletedTime).NotTo(BeNil())
	Expect(delResp.JSON202.DeletedBy).NotTo(BeNil())
	// deleted_by reflects the caller identity.
	Expect(string(*delResp.JSON202.DeletedBy)).To(Equal("creator@example.com"))
	// created_by is preserved.
	Expect(string(delResp.JSON202.CreatedBy)).To(Equal("header-creator@corp.com"))
}

func TestCallerIdentityEmptyHeaderFallback(t *testing.T) {
	RegisterTestingT(t)

	h, client := test.RegisterIntegration(t)

	account := h.NewAccount(h.NewID(), "JWT User", "jwt-fallback@example.com")
	ctx := h.NewAuthenticatedContext(account)

	// Create with an empty identity header — should fall back to JWT claim.
	clusterInput := openapi.ClusterCreateRequest{
		Kind: "Cluster",
		Name: shortClusterName("ci-empty", h.NewID()),
		Spec: map[string]interface{}{"test": "spec"},
	}
	resp, err := client.PostClusterWithResponse(
		ctx, openapi.PostClusterJSONRequestBody(clusterInput),
		test.WithAuthToken(ctx),
		test.WithIdentityHeader(test.IdentityHeaderName(), ""),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusCreated))
	Expect(string(resp.JSON201.CreatedBy)).To(Equal("jwt-fallback@example.com"))
	Expect(string(resp.JSON201.UpdatedBy)).To(Equal("jwt-fallback@example.com"))
}

func TestCallerIdentityOversizedHeader(t *testing.T) {
	RegisterTestingT(t)

	h, client := test.RegisterIntegration(t)

	account := h.NewAccount(h.NewID(), "Test User", "user@example.com")
	ctx := h.NewAuthenticatedContext(account)

	// Create with an oversized identity header (>256 chars) — should be rejected.
	clusterInput := openapi.ClusterCreateRequest{
		Kind: "Cluster",
		Name: shortClusterName("ci-long", h.NewID()),
		Spec: map[string]interface{}{"test": "spec"},
	}
	oversized := strings.Repeat("a", 257)
	resp, err := client.PostClusterWithResponse(
		ctx, openapi.PostClusterJSONRequestBody(clusterInput),
		test.WithAuthToken(ctx),
		test.WithIdentityHeader(test.IdentityHeaderName(), oversized),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusUnauthorized))
}
