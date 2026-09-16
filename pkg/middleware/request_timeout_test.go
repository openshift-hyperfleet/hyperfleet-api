package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequestTimeoutMiddlewareUsesDerivedRequestContext(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(t.Context())
	called := false
	handler := RequestTimeoutMiddleware(time.Second)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		called = true
		if _, ok := r.Context().Deadline(); !ok {
			t.Fatal("forwarded request has no deadline")
		}
	}))
	handler.ServeHTTP(httptest.NewRecorder(), request)
	if !called {
		t.Fatal("handler was not called")
	}
	if _, ok := request.Context().Deadline(); ok {
		t.Fatal("original request context was modified")
	}
}

func TestRequestTimeoutMiddlewareWithoutTimeoutPreservesContext(t *testing.T) {
	type contextKey string
	ctx := context.WithValue(t.Context(), contextKey("key"), "value")
	request := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	handler := RequestTimeoutMiddleware(0)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if r.Context() != ctx {
			t.Fatal("request context changed without a configured timeout")
		}
	}))
	handler.ServeHTTP(httptest.NewRecorder(), request)
}

func TestRequestTimeoutMiddlewareNegativeTimeoutPreservesContext(t *testing.T) {
	ctx := t.Context()
	request := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	handler := RequestTimeoutMiddleware(-time.Second)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if r.Context() != ctx {
			t.Fatal("request context changed with a negative timeout")
		}
	}))
	handler.ServeHTTP(httptest.NewRecorder(), request)
}

func TestRequestTimeoutMiddlewareCancelsDerivedContextAfterHandlerReturns(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(t.Context())
	var forwarded context.Context
	handler := RequestTimeoutMiddleware(time.Second)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		forwarded = r.Context()
	}))

	handler.ServeHTTP(httptest.NewRecorder(), request)
	if forwarded == nil {
		t.Fatal("handler did not receive a request context")
	}
	if err := forwarded.Err(); err != context.Canceled {
		t.Fatalf("derived context error = %v, want %v", err, context.Canceled)
	}
}
