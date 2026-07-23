package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	. "github.com/onsi/gomega"
)

// TestNewRouter_PublicVsProtectedMiddleware guards the auth boundary set up in NewRouter:
// public routes (metadata, openapi, openapi.html) must never see apiMiddleware or
// protectedMiddleware; protected routes must see both, with protectedMiddleware
// (auth) gating the request before apiMiddleware (schema validation, DB
// transaction) ever runs.
func TestNewRouter_PublicVsProtectedMiddleware(t *testing.T) {
	RegisterTestingT(t)

	var apiMiddlewareCalls, protectedMiddlewareCalls int
	var authorized bool
	countingAPIMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiMiddlewareCalls++
			next.ServeHTTP(w, r)
		})
	}
	gatingProtectedMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			protectedMiddlewareCalls++
			if !authorized {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	registrar := RouteRegistrar{
		Name: "widgets",
		Register: func(r *mux.Router) error {
			r.HandleFunc("/widgets", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}).Methods(http.MethodGet)
			return nil
		},
	}

	router, err := NewRouter(
		nil,
		[]mux.MiddlewareFunc{countingAPIMiddleware},
		[]mux.MiddlewareFunc{gatingProtectedMiddleware},
		[]RouteRegistrar{registrar},
	)
	Expect(err).NotTo(HaveOccurred())

	publicPaths := []string{
		"/api/hyperfleet",
		"/api/hyperfleet/v1/openapi",
		"/api/hyperfleet/v1/openapi.html",
	}
	for _, path := range publicPaths {
		apiMiddlewareCalls, protectedMiddlewareCalls = 0, 0

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))

		Expect(rr.Code).To(Equal(http.StatusOK), "public path %s should be reachable without auth", path)
		Expect(apiMiddlewareCalls).To(Equal(0), "apiMiddleware must not run for public path %s", path)
		Expect(protectedMiddlewareCalls).To(Equal(0), "protectedMiddleware must not run for public path %s", path)
	}

	authorized = false
	apiMiddlewareCalls, protectedMiddlewareCalls = 0, 0
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/hyperfleet/v1/widgets", nil))

	Expect(rr.Code).To(Equal(http.StatusUnauthorized), "protected route must be gated by protectedMiddleware")
	Expect(protectedMiddlewareCalls).To(Equal(1), "protectedMiddleware must run for protected routes")
	Expect(apiMiddlewareCalls).To(Equal(0),
		"apiMiddleware (schema validation, DB transaction) must not run when auth rejects the request")

	authorized = true
	apiMiddlewareCalls, protectedMiddlewareCalls = 0, 0
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/hyperfleet/v1/widgets", nil))

	Expect(rr.Code).To(Equal(http.StatusOK), "protected route must be reachable once auth passes")
	Expect(protectedMiddlewareCalls).To(Equal(1), "protectedMiddleware must run for protected routes")
	Expect(apiMiddlewareCalls).To(Equal(1), "apiMiddleware must run once auth allows the request through")
}
