/*
Copyright (c) 2018 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package handlers

import (
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

type configHandler struct {
	config *config.ApplicationConfig
}

func NewConfigHandler(cfg *config.ApplicationConfig) *configHandler {
	return &configHandler{
		config: cfg,
	}
}

// Get sends the merged configuration response with sensitive values redacted.
func (h configHandler) Get(w http.ResponseWriter, r *http.Request) {
	// Set the content type:
	w.Header().Set("Content-Type", "application/json")

	// Get redacted configuration JSON
	jsonConfig, err := h.config.GetJSONConfig()
	if err != nil {
		log := logger.NewOCMLogger(r.Context())
		log.Extra("endpoint", r.URL.Path).Extra("method", r.Method).Extra("error", err.Error()).
			Error("Failed to generate configuration JSON")
		api.SendPanic(w, r)
		return
	}

	// Send the response:
	_, err = w.Write([]byte(jsonConfig))
	if err != nil {
		log := logger.NewOCMLogger(r.Context())
		log.Extra("endpoint", r.URL.Path).Extra("method", r.Method).Extra("error", err.Error()).
			Error("Failed to send configuration response body")
		return
	}
}
