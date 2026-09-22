package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestTimeHandler_Get(t *testing.T) {
	RegisterTestingT(t)

	fixed := time.Date(2026, 9, 22, 13, 45, 5, 0, time.UTC)
	h := TimeHandler{now: func() time.Time { return fixed }}

	rr := httptest.NewRecorder()
	h.Get(rr, httptest.NewRequest(http.MethodGet, "/api/hyperfleet/v1/time", nil))

	Expect(rr.Code).To(Equal(http.StatusOK))
	Expect(rr.Header().Get("Content-Type")).To(Equal("application/json"))

	var body timeResponse
	Expect(json.Unmarshal(rr.Body.Bytes(), &body)).To(Succeed())
	Expect(body.Time).To(Equal(fixed.Format(time.RFC3339Nano)))
}

func TestTimeHandler_Get_ReturnsUTC(t *testing.T) {
	RegisterTestingT(t)

	loc := time.FixedZone("UTC+5", 5*60*60)
	local := time.Date(2026, 9, 22, 18, 45, 5, 0, loc)
	h := TimeHandler{now: func() time.Time { return local }}

	rr := httptest.NewRecorder()
	h.Get(rr, httptest.NewRequest(http.MethodGet, "/api/hyperfleet/v1/time", nil))

	var body timeResponse
	Expect(json.Unmarshal(rr.Body.Bytes(), &body)).To(Succeed())

	parsed, err := time.Parse(time.RFC3339Nano, body.Time)
	Expect(err).NotTo(HaveOccurred())
	Expect(parsed.Equal(local)).To(BeTrue(), "response must represent the same instant")
	Expect(body.Time).To(Equal(local.UTC().Format(time.RFC3339Nano)), "response must be normalized to UTC")
}
