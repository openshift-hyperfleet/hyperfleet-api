package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/response"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// handlerConfig defines the common things each REST controller must do.
// The corresponding handle() func runs the basic handlerConfig.
// This is not meant to be an HTTP framework or anything larger than simple CRUD in handlers.
//
//	MarshalInto is a pointer to the object to hold the unmarshaled JSON.
//	Validate is a list of validation function that run in order, returning fast on the first error.
//	Action is the specific logic a handler must take (e.g, find an object, save an object)
//	ErrorHandler is the way errors are returned to the client
type handlerConfig struct {
	MarshalInto  interface{}
	Validate     []validate
	Action       httpAction
	ErrorHandler errorHandlerFunc
}

type validate func() *errors.ServiceError
type errorHandlerFunc func(r *http.Request, w http.ResponseWriter, err *errors.ServiceError)
type httpAction func() (interface{}, *errors.ServiceError)

func handleError(r *http.Request, w http.ResponseWriter, err *errors.ServiceError) {
	traceID, _ := logger.GetRequestID(r.Context())
	instance := r.URL.Path

	// Log with RFC 9457 code format
	if err.HttpCode >= 400 && err.HttpCode <= 499 {
		logger.With(r.Context(),
			"code", err.RFC9457Code,
			"http_code", err.HttpCode,
			"reason", err.Reason).Info("Client error response")
	} else {
		logger.With(r.Context(),
			"code", err.RFC9457Code,
			"http_code", err.HttpCode,
			"reason", err.Reason).Error("Server error response")
	}

	response.WriteProblemDetailsResponse(w, r, err.HttpCode, err.AsProblemDetails(instance, traceID))
}

func handle(w http.ResponseWriter, r *http.Request, cfg *handlerConfig, httpStatus int) {
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = handleError
	}

	bytes, err := io.ReadAll(r.Body)
	if err != nil {
		handleError(r, w, errors.MalformedRequest("Unable to read request body: %s", err))
		return
	}

	err = json.Unmarshal(bytes, &cfg.MarshalInto)
	if err != nil {
		handleError(r, w, errors.MalformedRequest("Invalid request format: %s", err))
		return
	}

	for _, v := range cfg.Validate {
		err := v()
		if err != nil {
			cfg.ErrorHandler(r, w, err)
			return
		}
	}

	result, serviceErr := cfg.Action()

	switch {
	case serviceErr != nil:
		cfg.ErrorHandler(r, w, serviceErr)
	default:
		writeJSONResponse(w, r, httpStatus, result)
	}

}

func handleDelete(w http.ResponseWriter, r *http.Request, cfg *handlerConfig, httpStatus int) {
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = handleError
	}
	for _, v := range cfg.Validate {
		err := v()
		if err != nil {
			cfg.ErrorHandler(r, w, err)
			return
		}
	}

	result, serviceErr := cfg.Action()

	switch {
	case serviceErr != nil:
		cfg.ErrorHandler(r, w, serviceErr)
	default:
		writeJSONResponse(w, r, httpStatus, result)
	}

}

func handleGet(w http.ResponseWriter, r *http.Request, cfg *handlerConfig) {
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = handleError
	}

	result, serviceErr := cfg.Action()
	switch serviceErr {
	case nil:
		writeJSONResponse(w, r, http.StatusOK, result)
	default:
		cfg.ErrorHandler(r, w, serviceErr)
	}
}

func handleList(w http.ResponseWriter, r *http.Request, cfg *handlerConfig) {
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = handleError
	}

	results, serviceError := cfg.Action()
	if serviceError != nil {
		cfg.ErrorHandler(r, w, serviceError)
		return
	}
	writeJSONResponse(w, r, http.StatusOK, results)
}

// handleCreateWithNoContent handles create requests that may return 204 No Content
// If action returns (nil, nil), it means a no-op and returns 204 No Content
// Otherwise, it returns 201 Created with the result
func handleCreateWithNoContent(w http.ResponseWriter, r *http.Request, cfg *handlerConfig) {
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = handleError
	}

	bytes, err := io.ReadAll(r.Body)
	if err != nil {
		handleError(r, w, errors.MalformedRequest("Unable to read request body: %s", err))
		return
	}

	err = json.Unmarshal(bytes, &cfg.MarshalInto)
	if err != nil {
		handleError(r, w, errors.MalformedRequest("Invalid request format: %s", err))
		return
	}

	for _, v := range cfg.Validate {
		err := v()
		if err != nil {
			cfg.ErrorHandler(r, w, err)
			return
		}
	}

	result, serviceErr := cfg.Action()

	switch {
	case serviceErr != nil:
		cfg.ErrorHandler(r, w, serviceErr)
	case result == nil:
		// No-op case: return 204 No Content
		w.WriteHeader(http.StatusNoContent)
	default:
		writeJSONResponse(w, r, http.StatusCreated, result)
	}
}
