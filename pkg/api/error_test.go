package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"

	. "github.com/onsi/gomega"
)

func TestSendNotFound_ReturnsEndpointCode(t *testing.T) {
	RegisterTestingT(t)

	req := httptest.NewRequest(http.MethodGet, "/api/hyperfleet/v1/invalid-path", nil)
	rec := httptest.NewRecorder()

	SendNotFound(rec, req)

	Expect(rec.Code).To(Equal(http.StatusNotFound))
	Expect(rec.Header().Get("Content-Type")).To(Equal("application/problem+json"))

	var body openapi.ProblemDetails
	Expect(json.Unmarshal(rec.Body.Bytes(), &body)).To(Succeed())

	Expect(body.Status).To(Equal(http.StatusNotFound))
	Expect(body.Title).To(Equal("Endpoint Not Found"))
	Expect(body.Code).NotTo(BeNil())
	Expect(*body.Code).To(Equal(errors.CodeNotFoundEndpoint))
	Expect(body.Detail).NotTo(BeNil())
	Expect(*body.Detail).To(Equal("The requested endpoint '/api/hyperfleet/v1/invalid-path' does not exist"))
}
