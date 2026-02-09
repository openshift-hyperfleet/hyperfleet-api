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

package openapi

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/jinzhu/inflection"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/crd"
)

const basePath = "/api/hyperfleet/v1"

// addResourcePaths generates CRUD paths for a resource based on its scope.
func addResourcePaths(doc *openapi3.T, def *api.ResourceDefinition, registry *crd.Registry) {
	if def.IsOwned() {
		addOwnedResourcePaths(doc, def, registry)
	} else {
		addRootResourcePaths(doc, def)
	}
}

// addRootResourcePaths generates paths for root-level resources.
// - GET    /api/hyperfleet/v1/{plural}           - List
// - POST   /api/hyperfleet/v1/{plural}           - Create
// - GET    /api/hyperfleet/v1/{plural}/{id}      - Get
// - PATCH  /api/hyperfleet/v1/{plural}/{id}      - Patch
// - DELETE /api/hyperfleet/v1/{plural}/{id}      - Delete
func addRootResourcePaths(doc *openapi3.T, def *api.ResourceDefinition) {
	collectionPath := fmt.Sprintf("%s/%s", basePath, def.Plural)
	itemPath := fmt.Sprintf("%s/{%s_id}", collectionPath, def.Singular)
	statusesPath := fmt.Sprintf("%s/statuses", itemPath)

	// Collection path: GET (list), POST (create)
	doc.Paths.Set(collectionPath, &openapi3.PathItem{
		Get:  buildListOperation(def, nil),
		Post: buildCreateOperation(def, nil),
	})

	// Item path: GET (get), PATCH (patch), DELETE (delete)
	doc.Paths.Set(itemPath, &openapi3.PathItem{
		Get:    buildGetOperation(def, nil),
		Patch:  buildPatchOperation(def, nil),
		Delete: buildDeleteOperation(def, nil),
	})

	// Statuses path: GET (list statuses), POST (create/update status)
	doc.Paths.Set(statusesPath, &openapi3.PathItem{
		Get:  buildListStatusesOperation(def, nil),
		Post: buildCreateStatusOperation(def, nil),
	})
}

// addOwnedResourcePaths generates paths for owned resources.
// - GET    /api/hyperfleet/v1/{owner_plural}/{owner_id}/{plural}           - List
// - POST   /api/hyperfleet/v1/{owner_plural}/{owner_id}/{plural}           - Create
// - GET    /api/hyperfleet/v1/{owner_plural}/{owner_id}/{plural}/{id}      - Get
// - PATCH  /api/hyperfleet/v1/{owner_plural}/{owner_id}/{plural}/{id}      - Patch
// - DELETE /api/hyperfleet/v1/{owner_plural}/{owner_id}/{plural}/{id}      - Delete
func addOwnedResourcePaths(doc *openapi3.T, def *api.ResourceDefinition, registry *crd.Registry) {
	ownerDef := getOwnerDefinitionFromRegistry(def, registry)
	if ownerDef == nil {
		return // Cannot generate paths without owner definition
	}

	ownerPathParam := def.GetOwnerPathParam()
	collectionPath := fmt.Sprintf("%s/%s/{%s}/%s", basePath, ownerDef.Plural, ownerPathParam, def.Plural)
	itemPath := fmt.Sprintf("%s/{%s_id}", collectionPath, def.Singular)
	statusesPath := fmt.Sprintf("%s/statuses", itemPath)

	// Collection path: GET (list), POST (create)
	doc.Paths.Set(collectionPath, &openapi3.PathItem{
		Get:  buildListOperation(def, ownerDef),
		Post: buildCreateOperation(def, ownerDef),
	})

	// Item path: GET (get), PATCH (patch), DELETE (delete)
	doc.Paths.Set(itemPath, &openapi3.PathItem{
		Get:    buildGetOperation(def, ownerDef),
		Patch:  buildPatchOperation(def, ownerDef),
		Delete: buildDeleteOperation(def, ownerDef),
	})

	// Statuses path: GET (list statuses), POST (create/update status)
	doc.Paths.Set(statusesPath, &openapi3.PathItem{
		Get:  buildListStatusesOperation(def, ownerDef),
		Post: buildCreateStatusOperation(def, ownerDef),
	})

	// Also add a global list endpoint for owned resources (without owner filter)
	globalListPath := fmt.Sprintf("%s/%s", basePath, def.Plural)
	doc.Paths.Set(globalListPath, &openapi3.PathItem{
		Get: buildGlobalListOperation(def),
	})
}

// getOwnerDefinitionFromRegistry returns the ResourceDefinition for the owner of an owned resource.
func getOwnerDefinitionFromRegistry(def *api.ResourceDefinition, registry *crd.Registry) *api.ResourceDefinition {
	if def.Owner == nil {
		return nil
	}
	ownerDef, ok := registry.GetByKind(def.Owner.Kind)
	if !ok {
		return nil
	}
	return ownerDef
}

// buildListOperation creates a GET operation for listing resources.
func buildListOperation(def *api.ResourceDefinition, ownerDef *api.ResourceDefinition) *openapi3.Operation {
	operationID := fmt.Sprintf("get%s", inflection.Plural(def.Kind))
	summary := fmt.Sprintf("List %s", def.Plural)

	params := []*openapi3.ParameterRef{
		{Ref: "#/components/parameters/search"},
		{Ref: "#/components/parameters/page"},
		{Ref: "#/components/parameters/pageSize"},
		{Ref: "#/components/parameters/orderBy"},
		{Ref: "#/components/parameters/order"},
	}

	// Add owner path parameter for owned resources
	if ownerDef != nil {
		operationID = fmt.Sprintf("get%sBy%sId", inflection.Plural(def.Kind), ownerDef.Kind)
		summary = fmt.Sprintf("List all %s for %s", def.Plural, ownerDef.Singular)
		params = append([]*openapi3.ParameterRef{
			buildPathParameter(def.GetOwnerPathParam(), ownerDef.Kind+" ID"),
		}, params...)
	}

	return &openapi3.Operation{
		OperationID: operationID,
		Summary:     summary,
		Parameters:  params,
		Responses:   buildListResponses(def),
		Security:    &openapi3.SecurityRequirements{{"BearerAuth": {}}},
	}
}

// buildGlobalListOperation creates a GET operation for listing all resources globally.
func buildGlobalListOperation(def *api.ResourceDefinition) *openapi3.Operation {
	return &openapi3.Operation{
		OperationID: fmt.Sprintf("get%s", inflection.Plural(def.Kind)),
		Summary:     fmt.Sprintf("List all %s", def.Plural),
		Description: fmt.Sprintf("Returns the list of all %s", def.Plural),
		Parameters: []*openapi3.ParameterRef{
			{Ref: "#/components/parameters/search"},
			{Ref: "#/components/parameters/page"},
			{Ref: "#/components/parameters/pageSize"},
			{Ref: "#/components/parameters/orderBy"},
			{Ref: "#/components/parameters/order"},
		},
		Responses: buildListResponses(def),
		Security:  &openapi3.SecurityRequirements{{"BearerAuth": {}}},
	}
}

// buildCreateOperation creates a POST operation for creating a resource.
func buildCreateOperation(def *api.ResourceDefinition, ownerDef *api.ResourceDefinition) *openapi3.Operation {
	operationID := fmt.Sprintf("post%s", def.Kind)
	summary := fmt.Sprintf("Create %s", def.Singular)

	var params []*openapi3.ParameterRef

	// Add owner path parameter for owned resources
	if ownerDef != nil {
		operationID = fmt.Sprintf("create%s", def.Kind)
		summary = fmt.Sprintf("Create %s for %s", def.Singular, ownerDef.Singular)
		params = append(params, buildPathParameter(def.GetOwnerPathParam(), ownerDef.Kind+" ID"))
	}

	return &openapi3.Operation{
		OperationID: operationID,
		Summary:     summary,
		Description: fmt.Sprintf("Create a new %s resource.\n\n**Note**: The `status` object in the response is read-only and computed by the service. It is NOT part of the request body.", def.Singular),
		Parameters:  params,
		RequestBody: &openapi3.RequestBodyRef{
			Value: &openapi3.RequestBody{
				Required: true,
				Content: openapi3.Content{
					"application/json": &openapi3.MediaType{
						Schema: &openapi3.SchemaRef{
							Ref: "#/components/schemas/" + def.Kind + "CreateRequest",
						},
					},
				},
			},
		},
		Responses: buildCreateResponses(def),
		Security:  &openapi3.SecurityRequirements{{"BearerAuth": {}}},
	}
}

// buildGetOperation creates a GET operation for getting a single resource.
func buildGetOperation(def *api.ResourceDefinition, ownerDef *api.ResourceDefinition) *openapi3.Operation {
	operationID := fmt.Sprintf("get%sById", def.Kind)
	summary := fmt.Sprintf("Get %s by ID", def.Singular)

	params := []*openapi3.ParameterRef{
		{Ref: "#/components/parameters/search"},
		buildPathParameter(def.Singular+"_id", def.Kind+" ID"),
	}

	// Add owner path parameter for owned resources
	if ownerDef != nil {
		params = append([]*openapi3.ParameterRef{
			buildPathParameter(def.GetOwnerPathParam(), ownerDef.Kind+" ID"),
		}, params...)
	}

	return &openapi3.Operation{
		OperationID: operationID,
		Summary:     summary,
		Parameters:  params,
		Responses:   buildGetResponses(def),
		Security:    &openapi3.SecurityRequirements{{"BearerAuth": {}}},
	}
}

// buildPatchOperation creates a PATCH operation for updating a resource.
func buildPatchOperation(def *api.ResourceDefinition, ownerDef *api.ResourceDefinition) *openapi3.Operation {
	operationID := fmt.Sprintf("patch%s", def.Kind)
	summary := fmt.Sprintf("Update %s", def.Singular)

	params := []*openapi3.ParameterRef{
		buildPathParameter(def.Singular+"_id", def.Kind+" ID"),
	}

	// Add owner path parameter for owned resources
	if ownerDef != nil {
		params = append([]*openapi3.ParameterRef{
			buildPathParameter(def.GetOwnerPathParam(), ownerDef.Kind+" ID"),
		}, params...)
	}

	return &openapi3.Operation{
		OperationID: operationID,
		Summary:     summary,
		Parameters:  params,
		RequestBody: &openapi3.RequestBodyRef{
			Value: &openapi3.RequestBody{
				Required: true,
				Content: openapi3.Content{
					"application/json": &openapi3.MediaType{
						Schema: &openapi3.SchemaRef{
							Ref: "#/components/schemas/" + def.Kind + "CreateRequest",
						},
					},
				},
			},
		},
		Responses: buildGetResponses(def),
		Security:  &openapi3.SecurityRequirements{{"BearerAuth": {}}},
	}
}

// buildDeleteOperation creates a DELETE operation for deleting a resource.
func buildDeleteOperation(def *api.ResourceDefinition, ownerDef *api.ResourceDefinition) *openapi3.Operation {
	operationID := fmt.Sprintf("delete%s", def.Kind)
	summary := fmt.Sprintf("Delete %s", def.Singular)

	params := []*openapi3.ParameterRef{
		buildPathParameter(def.Singular+"_id", def.Kind+" ID"),
	}

	// Add owner path parameter for owned resources
	if ownerDef != nil {
		params = append([]*openapi3.ParameterRef{
			buildPathParameter(def.GetOwnerPathParam(), ownerDef.Kind+" ID"),
		}, params...)
	}

	return &openapi3.Operation{
		OperationID: operationID,
		Summary:     summary,
		Parameters:  params,
		Responses:   buildDeleteResponses(),
		Security:    &openapi3.SecurityRequirements{{"BearerAuth": {}}},
	}
}

// buildListStatusesOperation creates a GET operation for listing adapter statuses.
func buildListStatusesOperation(def *api.ResourceDefinition, ownerDef *api.ResourceDefinition) *openapi3.Operation {
	operationID := fmt.Sprintf("get%sStatuses", def.Kind)
	summary := fmt.Sprintf("List all adapter statuses for %s", def.Singular)

	params := []*openapi3.ParameterRef{
		buildPathParameter(def.Singular+"_id", def.Kind+" ID"),
		{Ref: "#/components/parameters/search"},
		{Ref: "#/components/parameters/page"},
		{Ref: "#/components/parameters/pageSize"},
		{Ref: "#/components/parameters/orderBy"},
		{Ref: "#/components/parameters/order"},
	}

	// Add owner path parameter for owned resources
	if ownerDef != nil {
		params = append([]*openapi3.ParameterRef{
			buildPathParameter(def.GetOwnerPathParam(), ownerDef.Kind+" ID"),
		}, params...)
	}

	return &openapi3.Operation{
		OperationID: operationID,
		Summary:     summary,
		Description: fmt.Sprintf("Returns adapter status reports for this %s", def.Singular),
		Parameters:  params,
		Responses:   buildStatusListResponses(),
		Security:    &openapi3.SecurityRequirements{{"BearerAuth": {}}},
	}
}

// buildCreateStatusOperation creates a POST operation for creating/updating adapter status.
func buildCreateStatusOperation(def *api.ResourceDefinition, ownerDef *api.ResourceDefinition) *openapi3.Operation {
	operationID := fmt.Sprintf("post%sStatuses", def.Kind)
	summary := "Create or update adapter status"

	params := []*openapi3.ParameterRef{
		buildPathParameter(def.Singular+"_id", def.Kind+" ID"),
	}

	// Add owner path parameter for owned resources
	if ownerDef != nil {
		params = append([]*openapi3.ParameterRef{
			buildPathParameter(def.GetOwnerPathParam(), ownerDef.Kind+" ID"),
		}, params...)
	}

	return &openapi3.Operation{
		OperationID: operationID,
		Summary:     summary,
		Description: fmt.Sprintf("Adapter creates or updates its status report for this %s.\nIf adapter already has a status, it will be updated (upsert by adapter name).\n\nResponse includes the full adapter status with all conditions.\nAdapter should call this endpoint every time it evaluates the %s.", def.Singular, def.Singular),
		Parameters:  params,
		RequestBody: &openapi3.RequestBodyRef{
			Value: &openapi3.RequestBody{
				Required: true,
				Content: openapi3.Content{
					"application/json": &openapi3.MediaType{
						Schema: &openapi3.SchemaRef{
							Ref: "#/components/schemas/AdapterStatusCreateRequest",
						},
					},
				},
			},
		},
		Responses: buildStatusCreateResponses(),
	}
}

// buildPathParameter creates a path parameter reference.
func buildPathParameter(name, description string) *openapi3.ParameterRef {
	return &openapi3.ParameterRef{
		Value: &openapi3.Parameter{
			Name:        name,
			In:          "path",
			Required:    true,
			Description: description,
			Schema: &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"string"},
				},
			},
		},
	}
}

// buildListResponses creates standard responses for list operations.
func buildListResponses(def *api.ResourceDefinition) *openapi3.Responses {
	responses := openapi3.NewResponses()
	responses.Set("200", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: stringPtr("The request has succeeded."),
			Content: openapi3.Content{
				"application/json": &openapi3.MediaType{
					Schema: &openapi3.SchemaRef{
						Ref: "#/components/schemas/" + def.Kind + "List",
					},
				},
			},
		},
	})
	responses.Set("400", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: stringPtr("The server could not understand the request due to invalid syntax."),
		},
	})
	responses.Set("default", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: stringPtr("An unexpected error response."),
			Content: openapi3.Content{
				"application/problem+json": &openapi3.MediaType{
					Schema: &openapi3.SchemaRef{
						Ref: "#/components/schemas/Error",
					},
				},
			},
		},
	})
	return responses
}

// buildGetResponses creates standard responses for get operations.
func buildGetResponses(def *api.ResourceDefinition) *openapi3.Responses {
	responses := openapi3.NewResponses()
	responses.Set("200", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: stringPtr("The request has succeeded."),
			Content: openapi3.Content{
				"application/json": &openapi3.MediaType{
					Schema: &openapi3.SchemaRef{
						Ref: "#/components/schemas/" + def.Kind,
					},
				},
			},
		},
	})
	responses.Set("400", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: stringPtr("The server could not understand the request due to invalid syntax."),
		},
	})
	responses.Set("default", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: stringPtr("An unexpected error response."),
			Content: openapi3.Content{
				"application/problem+json": &openapi3.MediaType{
					Schema: &openapi3.SchemaRef{
						Ref: "#/components/schemas/Error",
					},
				},
			},
		},
	})
	return responses
}

// buildCreateResponses creates standard responses for create operations.
func buildCreateResponses(def *api.ResourceDefinition) *openapi3.Responses {
	responses := openapi3.NewResponses()
	responses.Set("201", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: stringPtr("The request has succeeded and a new resource has been created as a result."),
			Content: openapi3.Content{
				"application/json": &openapi3.MediaType{
					Schema: &openapi3.SchemaRef{
						Ref: "#/components/schemas/" + def.Kind,
					},
				},
			},
		},
	})
	responses.Set("400", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: stringPtr("The server could not understand the request due to invalid syntax."),
		},
	})
	responses.Set("default", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: stringPtr("An unexpected error response."),
			Content: openapi3.Content{
				"application/problem+json": &openapi3.MediaType{
					Schema: &openapi3.SchemaRef{
						Ref: "#/components/schemas/Error",
					},
				},
			},
		},
	})
	return responses
}

// buildDeleteResponses creates standard responses for delete operations.
func buildDeleteResponses() *openapi3.Responses {
	responses := openapi3.NewResponses()
	responses.Set("204", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: stringPtr("The resource has been successfully deleted."),
		},
	})
	responses.Set("400", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: stringPtr("The server could not understand the request due to invalid syntax."),
		},
	})
	responses.Set("404", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: stringPtr("The server cannot find the requested resource."),
		},
	})
	responses.Set("default", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: stringPtr("An unexpected error response."),
			Content: openapi3.Content{
				"application/problem+json": &openapi3.MediaType{
					Schema: &openapi3.SchemaRef{
						Ref: "#/components/schemas/Error",
					},
				},
			},
		},
	})
	return responses
}

// buildStatusListResponses creates standard responses for status list operations.
func buildStatusListResponses() *openapi3.Responses {
	responses := openapi3.NewResponses()
	responses.Set("200", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: stringPtr("The request has succeeded."),
			Content: openapi3.Content{
				"application/json": &openapi3.MediaType{
					Schema: &openapi3.SchemaRef{
						Ref: "#/components/schemas/AdapterStatusList",
					},
				},
			},
		},
	})
	responses.Set("400", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: stringPtr("The server could not understand the request due to invalid syntax."),
		},
	})
	responses.Set("404", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: stringPtr("The server cannot find the requested resource."),
		},
	})
	responses.Set("default", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: stringPtr("An unexpected error response."),
			Content: openapi3.Content{
				"application/problem+json": &openapi3.MediaType{
					Schema: &openapi3.SchemaRef{
						Ref: "#/components/schemas/Error",
					},
				},
			},
		},
	})
	return responses
}

// buildStatusCreateResponses creates standard responses for status create operations.
func buildStatusCreateResponses() *openapi3.Responses {
	responses := openapi3.NewResponses()
	responses.Set("201", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: stringPtr("The request has succeeded and a new resource has been created as a result."),
			Content: openapi3.Content{
				"application/json": &openapi3.MediaType{
					Schema: &openapi3.SchemaRef{
						Ref: "#/components/schemas/AdapterStatus",
					},
				},
			},
		},
	})
	responses.Set("400", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: stringPtr("The server could not understand the request due to invalid syntax."),
		},
	})
	responses.Set("404", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: stringPtr("The server cannot find the requested resource."),
		},
	})
	responses.Set("409", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: stringPtr("The request conflicts with the current state of the server."),
		},
	})
	return responses
}

// stringPtr returns a pointer to a string value.
func stringPtr(s string) *string {
	return &s
}
