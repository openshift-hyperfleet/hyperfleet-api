package handlers

import (
	"context"
	"embed"
	"io/fs"
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/crd"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/openapi"
)

//go:embed openapi-ui.html
var openapiui embed.FS

type openAPIHandler struct {
	openAPIDefinitions []byte
	uiContent          []byte
}

func NewOpenAPIHandler() (*openAPIHandler, error) {
	ctx := context.Background()

	// Generate the OpenAPI spec dynamically from CRD registry
	spec := openapi.GenerateSpec(crd.Default())

	// Marshal the spec to JSON
	data, err := spec.MarshalJSON()
	if err != nil {
		return nil, errors.GeneralError(
			"can't marshal OpenAPI specification to JSON: %v",
			err,
		)
	}
	logger.Info(ctx, "Generated OpenAPI specification from CRD registry")

	// Load the OpenAPI UI HTML content
	uiContent, err := fs.ReadFile(openapiui, "openapi-ui.html")
	if err != nil {
		return nil, errors.GeneralError(
			"can't load OpenAPI UI HTML from embedded file: %v",
			err,
		)
	}
	logger.Info(ctx, "Loaded OpenAPI UI HTML from embedded file")

	return &openAPIHandler{
		openAPIDefinitions: data,
		uiContent:          uiContent,
	}, nil
}

func (h *openAPIHandler) GetOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(h.openAPIDefinitions); err != nil {
		// Response already committed, can't report error
		logger.With(r.Context(),
			logger.HTTPPath(r.URL.Path),
			logger.HTTPMethod(r.Method),
			logger.HTTPStatusCode(http.StatusOK),
		).WithError(err).Error("Failed to write OpenAPI specification response")
		return
	}
}

func (h *openAPIHandler) GetOpenAPIUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(h.uiContent); err != nil {
		// Response already committed, can't report error
		logger.With(r.Context(),
			logger.HTTPPath(r.URL.Path),
			logger.HTTPMethod(r.Method),
			logger.HTTPStatusCode(http.StatusOK),
		).WithError(err).Error("Failed to write OpenAPI UI response")
		return
	}
}
