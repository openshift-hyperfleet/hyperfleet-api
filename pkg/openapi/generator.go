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

// Package openapi provides dynamic OpenAPI specification generation from CRD definitions.

package openapi

import (
	"sort"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/crd"
)

// GenerateSpec builds an OpenAPI 3.0 spec from the CRD registry.
// It dynamically creates paths and schemas based on loaded CRD definitions.
func GenerateSpec(registry *crd.Registry) *openapi3.T {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info: &openapi3.Info{
			Title:   "HyperFleet API",
			Version: "1.0.0",
			Contact: &openapi3.Contact{
				Name: "HyperFleet Team",
			},
			License: &openapi3.License{
				Name: "Apache 2.0",
				URL:  "https://www.apache.org/licenses/LICENSE-2.0",
			},
			Description: "HyperFleet API provides simple CRUD operations for managing cluster resources and their status history.\n\n**Architecture**: Simple CRUD only, no business logic, no event creation.\nSentinel operator handles all orchestration logic.\nAdapters handle the specifics of managing spec",
		},
		Paths: openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas:         make(openapi3.Schemas),
			Parameters:      make(openapi3.ParametersMap),
			SecuritySchemes: make(openapi3.SecuritySchemes),
		},
		Servers: openapi3.Servers{
			&openapi3.Server{
				URL:         "https://hyperfleet.redhat.com",
				Description: "Production",
			},
		},
	}

	// Add common schemas (Error, pagination, conditions, etc.)
	addCommonSchemas(doc)

	// Add common parameters (page, pageSize, orderBy, order, search)
	addCommonParameters(doc)

	// Add security schemes
	addSecuritySchemes(doc)

	// Get all resource definitions and sort them for deterministic output
	defs := registry.All()
	sort.Slice(defs, func(i, j int) bool {
		// Root resources first, then by kind name
		if defs[i].IsRoot() != defs[j].IsRoot() {
			return defs[i].IsRoot()
		}
		return defs[i].Kind < defs[j].Kind
	})

	// Generate paths and schemas for each CRD
	// First pass: generate schemas (needed for path references)
	for _, def := range defs {
		addResourceSchemas(doc, def)
	}

	// Second pass: generate paths (may reference schemas)
	for _, def := range defs {
		addResourcePaths(doc, def, registry)
	}

	return doc
}
