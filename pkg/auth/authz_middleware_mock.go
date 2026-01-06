package auth

import (
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

type authzMiddlewareMock struct{}

var _ AuthorizationMiddleware = &authzMiddlewareMock{}

func NewAuthzMiddlewareMock() AuthorizationMiddleware {
	return &authzMiddlewareMock{}
}

func (a authzMiddlewareMock) AuthorizeApi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.NewOCMLogger(r.Context()).Extra("method", r.Method).Extra("url", r.URL.String()).Info("Mock authz allows <any>/<any>")
		next.ServeHTTP(w, r)
	})
}
