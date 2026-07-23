package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/gomega"
)

// TestNewRouterFromConfig_PublicVsProtectedMiddleware guards the middleware layering:
//
//   - apiMiddleware (metrics, compress) runs for ALL API routes, public and protected.
//   - protectedAPIMiddleware (schema validation, DB transaction) runs only for
//     protected routes, after auth passes.
//   - authMiddleware gates protected routes; unauthenticated requests never reach
//     protectedAPIMiddleware.
//   - The /api/hyperfleet metadata route sits outside apiV1 and gets neither.
func TestNewRouterFromConfig_PublicVsProtectedMiddleware(t *testing.T) {
	RegisterTestingT(t)

	var apiMiddlewareCalls, protectedAPIMiddlewareCalls, authMiddlewareCalls int
	var authorized bool
	countingAPIMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiMiddlewareCalls++
			next.ServeHTTP(w, r)
		})
	}
	countingProtectedAPIMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			protectedAPIMiddlewareCalls++
			next.ServeHTTP(w, r)
		})
	}
	gatingAuthMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authMiddlewareCalls++
			if !authorized {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	registrar := RouteRegistrar{
		Name: "widgets",
		Register: func(r *Router) error {
			r.HandleFunc("GET /widgets", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			return nil
		},
	}

	router, err := NewRouterFromConfig(
		nil,
		[]Middleware{countingAPIMiddleware},
		[]Middleware{countingProtectedAPIMiddleware},
		[]Middleware{gatingAuthMiddleware},
		[]RouteRegistrar{registrar},
	)
	Expect(err).NotTo(HaveOccurred())

	resetCounters := func() {
		apiMiddlewareCalls, protectedAPIMiddlewareCalls, authMiddlewareCalls = 0, 0, 0
	}

	// Metadata route (/api/hyperfleet) sits outside apiV1 - no apiMiddleware.
	resetCounters()
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/hyperfleet", nil))
	Expect(rr.Code).To(Equal(http.StatusOK), "metadata route should be reachable")
	Expect(apiMiddlewareCalls).To(Equal(0), "apiMiddleware must not run for metadata route")
	Expect(authMiddlewareCalls).To(Equal(0), "authMiddleware must not run for metadata route")

	// Public v1 paths get apiMiddleware (metrics, compress) but not auth or protectedAPI.
	for _, path := range []string{
		"/api/hyperfleet/v1/openapi",
		"/api/hyperfleet/v1/openapi.html",
	} {
		resetCounters()

		rr = httptest.NewRecorder()
		router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))

		Expect(rr.Code).To(Equal(http.StatusOK), "public path %s should be reachable without auth", path)
		Expect(apiMiddlewareCalls).To(Equal(1), "apiMiddleware must run for public v1 path %s", path)
		Expect(authMiddlewareCalls).To(Equal(0), "authMiddleware must not run for public path %s", path)
		Expect(protectedAPIMiddlewareCalls).To(Equal(0), "protectedAPIMiddleware must not run for public path %s", path)
	}

	// Protected route, unauthorized - auth rejects before protectedAPIMiddleware runs.
	authorized = false
	resetCounters()
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/hyperfleet/v1/widgets", nil))

	Expect(rr.Code).To(Equal(http.StatusUnauthorized), "protected route must be gated by authMiddleware")
	Expect(apiMiddlewareCalls).To(Equal(1), "apiMiddleware must run for protected routes")
	Expect(authMiddlewareCalls).To(Equal(1), "authMiddleware must run for protected routes")
	Expect(protectedAPIMiddlewareCalls).To(Equal(0),
		"protectedAPIMiddleware (schema validation, DB transaction) must not run when auth rejects the request")

	// Protected route, authorized - all middleware layers run.
	authorized = true
	resetCounters()
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/hyperfleet/v1/widgets", nil))

	Expect(rr.Code).To(Equal(http.StatusOK), "protected route must be reachable once auth passes")
	Expect(apiMiddlewareCalls).To(Equal(1), "apiMiddleware must run for protected routes")
	Expect(authMiddlewareCalls).To(Equal(1), "authMiddleware must run for protected routes")
	Expect(protectedAPIMiddlewareCalls).To(Equal(1),
		"protectedAPIMiddleware must run once auth allows the request through")
}
