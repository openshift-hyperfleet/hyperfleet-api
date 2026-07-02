package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"gopkg.in/resty.v1"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

const wifConfigsPath = "/wifconfigs"

// TestWifConfigSchemaValidation tests schema validation middleware for WifConfig resources.
// Default integration tests use test/validation-schema.yaml (permissive WifConfigSpec).
func TestWifConfigSchemaValidation(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := test.GetAccessTokenFromContext(ctx)

	// Test 1: Valid spec (permissive schema accepts any object)
	validInput := openapi.ResourceCreateRequest{
		Kind: "WifConfig",
		Name: "schema-valid-WifConfig",
		Spec: map[string]interface{}{
			"projectId": "my-project",
			"version":   "4.17",
		},
	}
	validBody, err := json.Marshal(validInput)
	Expect(err).NotTo(HaveOccurred())

	resp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(validBody).
		Post(h.RestURL(wifConfigsPath))
	Expect(err).NotTo(HaveOccurred(), "Valid WifConfig spec should be accepted")
	Expect(resp.StatusCode()).To(Equal(http.StatusCreated))

	var created map[string]interface{}
	err = json.Unmarshal(resp.Body(), &created)
	Expect(err).NotTo(HaveOccurred())
	Expect(created["id"]).NotTo(BeEmpty())

	// Test 2: Invalid spec type (spec must be object, not string)
	invalidTypeJSON := `{
		"kind": "WifConfig",
		"name": "schema-invalid-type",
		"spec": "invalid-string-spec"
	}`

	resp2, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(invalidTypeJSON).
		Post(h.RestURL(wifConfigsPath))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp2.StatusCode()).To(Equal(http.StatusBadRequest),
		"Non-object spec must be rejected")

	var errorResponse openapi.ProblemDetails
	Expect(json.Unmarshal(resp2.Body(), &errorResponse)).To(Succeed())
	Expect(errorResponse.Type).ToNot(BeEmpty())
	Expect(errorResponse.Code).ToNot(BeNil())
	Expect(errorResponse.Detail).ToNot(BeNil())

	// Test 3: Empty spec (valid when WifConfigSpec has no required fields)
	emptySpecInput := openapi.ResourceCreateRequest{
		Kind: "WifConfig",
		Name: "schema-empty-spec",
		Spec: map[string]interface{}{},
	}
	emptyBody, err := json.Marshal(emptySpecInput)
	Expect(err).NotTo(HaveOccurred())

	resp3, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(emptyBody).
		Post(h.RestURL(wifConfigsPath))
	Expect(err).NotTo(HaveOccurred(), "Empty spec should be accepted by permissive schema")
	Expect(resp3.StatusCode()).To(Equal(http.StatusCreated))
}

// TestWifConfigSchemaValidationWithProviderSchema runs only when a strict provider-specific schema is configured.
func TestWifConfigSchemaValidationWithProviderSchema(t *testing.T) {
	RegisterTestingT(t)

	schemaPath := os.Getenv("HYPERFLEET_SERVER_OPENAPI_SCHEMA_PATH")
	if schemaPath == "" ||
		strings.HasSuffix(schemaPath, "openapi/openapi.yaml") ||
		strings.HasSuffix(schemaPath, "validation-schema.yaml") {
		t.Skip("Skipping provider schema validation test - using permissive or base schema")
		return
	}

	h, _ := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := test.GetAccessTokenFromContext(ctx)

	// Missing required project_id (typical in strict WifConfigSpec overlays)
	invalidInput := openapi.ResourceCreateRequest{
		Kind: "WifConfig",
		Name: "provider-schema-invalid",
		Spec: map[string]interface{}{
			"version": "4.17",
		},
	}
	body, err := json.Marshal(invalidInput)
	Expect(err).NotTo(HaveOccurred())

	resp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(body).
		Post(h.RestURL(wifConfigsPath))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusBadRequest),
		"Should reject spec with missing required field")

	var errorResponse openapi.ProblemDetails
	if err := json.Unmarshal(resp.Body(), &errorResponse); err != nil {
		t.Fatalf("failed to unmarshal error response body: %v", err)
	}

	Expect(errorResponse.Type).ToNot(BeEmpty())
	Expect(errorResponse.Code).ToNot(BeNil())
	Expect(*errorResponse.Code).To(Equal("HYPERFLEET-VAL-000"))
	Expect(errorResponse.Errors).ToNot(BeEmpty(), "Should include field-level error details")

	foundProjectIDError := false
	if errorResponse.Errors != nil {
		for _, detail := range *errorResponse.Errors {
			if strings.Contains(detail.Field, "project_id") {
				foundProjectIDError = true
				break
			}
		}
	}
	Expect(foundProjectIDError).To(BeTrue(), "Error details should mention missing 'project_id' field")
}

// TestWifConfigSchemaValidationErrorDetails tests RFC 9457 error structure for invalid spec types.
func TestWifConfigSchemaValidationErrorDetails(t *testing.T) {
	RegisterTestingT(t)
	h, _ := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := test.GetAccessTokenFromContext(ctx)

	invalidTypeRequest := map[string]interface{}{
		"kind": "WifConfig",
		"name": "error-details-test",
		"spec": "not-an-object",
	}

	body, err := json.Marshal(invalidTypeRequest)
	Expect(err).NotTo(HaveOccurred())

	resp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(body).
		Post(h.RestURL(wifConfigsPath))
	Expect(err).To(BeNil())

	t.Logf("Response status: %d, body: %s", resp.StatusCode(), string(resp.Body()))
	Expect(resp.StatusCode()).To(Equal(http.StatusBadRequest), "Should return 400 for invalid spec type")

	var errorResponse openapi.ProblemDetails
	if err := json.Unmarshal(resp.Body(), &errorResponse); err != nil {
		t.Fatalf("failed to unmarshal error response: %v, response body: %s", err, string(resp.Body()))
	}

	Expect(errorResponse.Type).ToNot(BeEmpty())
	Expect(errorResponse.Title).ToNot(BeEmpty())
	Expect(errorResponse.Code).ToNot(BeNil())

	validCodes := []string{"HYPERFLEET-VAL-000", "HYPERFLEET-VAL-006"}
	Expect(validCodes).To(ContainElement(*errorResponse.Code))

	Expect(errorResponse.Detail).ToNot(BeNil())
	Expect(*errorResponse.Detail).To(ContainSubstring("spec"))
	Expect(errorResponse.Instance).ToNot(BeNil())
	Expect(errorResponse.TraceId).ToNot(BeNil())
}

// TestWifConfigPost_EmptyKind tests that empty kind field returns 400.
func TestWifConfigPost_EmptyKind(t *testing.T) {
	RegisterTestingT(t)
	h, _ := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := test.GetAccessTokenFromContext(ctx)

	invalidInput := `{
		"kind": "",
		"name": "test-WifConfig",
		"spec": {}
	}`

	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(invalidInput).
		Post(h.RestURL(wifConfigsPath))

	Expect(err).ToNot(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))

	var errorResponse map[string]interface{}
	err = json.Unmarshal(restyResp.Body(), &errorResponse)
	Expect(err).ToNot(HaveOccurred())

	detail, ok := errorResponse["detail"].(string)
	Expect(ok).To(BeTrue())
	Expect(detail).To(ContainSubstring("kind is required"))
}

// TestWifConfigPost_WrongKind tests that wrong kind field returns 400.
func TestWifConfigPost_WrongKind(t *testing.T) {
	RegisterTestingT(t)
	h, _ := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := test.GetAccessTokenFromContext(ctx)

	invalidInput := `{
		"kind": "Cluster",
		"name": "test-WifConfig",
		"spec": {}
	}`

	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(invalidInput).
		Post(h.RestURL(wifConfigsPath))

	Expect(err).ToNot(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))

	var errorResponse map[string]interface{}
	err = json.Unmarshal(restyResp.Body(), &errorResponse)
	Expect(err).ToNot(HaveOccurred())

	detail, ok := errorResponse["detail"].(string)
	Expect(ok).To(BeTrue())
	Expect(detail).To(ContainSubstring("kind must be 'WifConfig'"))
}

// TestWifConfigPost_NullSpec tests that null spec field returns 400.
func TestWifConfigPost_NullSpec(t *testing.T) {
	RegisterTestingT(t)
	h, _ := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := test.GetAccessTokenFromContext(ctx)

	invalidInput := `{
		"kind": "WifConfig",
		"name": "test-WifConfig",
		"spec": null
	}`

	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(invalidInput).
		Post(h.RestURL(wifConfigsPath))

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
	Expect(detail).To(ContainSubstring("spec field must be an object"))
}

// TestWifConfigPost_MissingSpec tests that missing spec field returns 400.
func TestWifConfigPost_MissingSpec(t *testing.T) {
	RegisterTestingT(t)
	h, _ := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := test.GetAccessTokenFromContext(ctx)

	invalidInput := `{
		"kind": "WifConfig",
		"name": "test-WifConfig"
	}`

	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(invalidInput).
		Post(h.RestURL(wifConfigsPath))

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
	Expect(detail).To(ContainSubstring("spec is required"))
}
