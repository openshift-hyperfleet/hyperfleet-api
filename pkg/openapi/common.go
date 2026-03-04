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
	"github.com/getkin/kin-openapi/openapi3"
)

// addCommonSchemas adds reusable schemas used across all resources.
func addCommonSchemas(doc *openapi3.T) {
	// Error schema (RFC 9457)
	doc.Components.Schemas["Error"] = &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:     &openapi3.Types{"object"},
			Required: []string{"type", "title", "status"},
			Properties: openapi3.Schemas{
				"type": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Format:      "uri",
						Description: "URI reference identifying the problem type",
						Example:     "https://api.hyperfleet.io/errors/validation-error",
					},
				},
				"title": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Description: "Short human-readable summary of the problem",
						Example:     "Validation Failed",
					},
				},
				"status": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"integer"},
						Description: "HTTP status code",
						Example:     400,
					},
				},
				"detail": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Description: "Human-readable explanation specific to this occurrence",
						Example:     "The cluster name field is required",
					},
				},
				"instance": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Format:      "uri",
						Description: "URI reference for this specific occurrence",
						Example:     "/api/hyperfleet/v1/clusters",
					},
				},
				"code": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Description: "Machine-readable error code in HYPERFLEET-CAT-NUM format",
						Example:     "HYPERFLEET-VAL-001",
					},
				},
				"timestamp": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Format:      "date-time",
						Description: "RFC3339 timestamp of when the error occurred",
						Example:     "2024-01-15T10:30:00Z",
					},
				},
				"trace_id": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Description: "Distributed trace ID for correlation",
						Example:     "abc123def456",
					},
				},
				"errors": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"array"},
						Items: &openapi3.SchemaRef{
							Ref: "#/components/schemas/ValidationError",
						},
						Description: "Field-level validation errors (for validation failures)",
					},
				},
			},
			Description: "RFC 9457 Problem Details error format with HyperFleet extensions",
		},
	}

	// ValidationError schema
	doc.Components.Schemas["ValidationError"] = &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:     &openapi3.Types{"object"},
			Required: []string{"field", "message"},
			Properties: openapi3.Schemas{
				"field": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Description: "JSON path to the field that failed validation",
						Example:     "spec.name",
					},
				},
				"value": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Description: "The invalid value that was provided (if safe to include)",
					},
				},
				"constraint": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
						Enum: []interface{}{
							"required", "min", "max", "min_length", "max_length",
							"pattern", "enum", "format", "unique",
						},
						Description: "The validation constraint that was violated",
						Example:     "required",
					},
				},
				"message": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Description: "Human-readable error message for this field",
						Example:     "Cluster name is required",
					},
				},
			},
			Description: "Field-level validation error detail",
		},
	}

	// ResourceCondition schema
	doc.Components.Schemas["ResourceCondition"] = &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:     &openapi3.Types{"object"},
			Required: []string{"type", "last_transition_time", "status", "observed_generation", "created_time", "last_updated_time"},
			Properties: openapi3.Schemas{
				"type": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Description: "Condition type",
					},
				},
				"reason": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Description: "Machine-readable reason code",
					},
				},
				"message": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Description: "Human-readable message",
					},
				},
				"last_transition_time": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Format:      "date-time",
						Description: "When this condition last transitioned status (API-managed)",
					},
				},
				"status": &openapi3.SchemaRef{
					Ref: "#/components/schemas/ResourceConditionStatus",
				},
				"observed_generation": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"integer"},
						Format:      "int32",
						Description: "Generation of the spec that this condition reflects",
					},
				},
				"created_time": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Format:      "date-time",
						Description: "When this condition was first created (API-managed)",
					},
				},
				"last_updated_time": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Format:      "date-time",
						Description: "When the corresponding adapter last reported (API-managed)",
					},
				},
			},
			Description: "Condition in resource status",
		},
	}

	// ResourceConditionStatus enum
	doc.Components.Schemas["ResourceConditionStatus"] = &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:        &openapi3.Types{"string"},
			Enum:        []interface{}{"True", "False"},
			Description: "Status value for resource conditions",
		},
	}

	// ObjectReference schema
	doc.Components.Schemas["ObjectReference"] = &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"id": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Description: "Resource identifier",
					},
				},
				"kind": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Description: "Resource kind",
					},
				},
				"href": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Description: "Resource URI",
					},
				},
			},
		},
	}

	// AdapterStatus schema
	doc.Components.Schemas["AdapterStatus"] = &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:     &openapi3.Types{"object"},
			Required: []string{"adapter", "observed_generation", "conditions", "created_time", "last_report_time"},
			Properties: openapi3.Schemas{
				"adapter": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Description: "Adapter name (e.g., \"validator\", \"dns\", \"provisioner\")",
					},
				},
				"observed_generation": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"integer"},
						Format:      "int32",
						Description: "Which generation of the resource this status reflects",
					},
				},
				"metadata": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"object"},
						Description: "Job execution metadata",
						Properties: openapi3.Schemas{
							"job_name":       &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
							"job_namespace":  &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
							"attempt":        &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}, Format: "int32"}},
							"started_time":   &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}, Format: "date-time"}},
							"completed_time": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}, Format: "date-time"}},
							"duration":       &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
						},
					},
				},
				"data": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:                 &openapi3.Types{"object"},
						AdditionalProperties: openapi3.AdditionalProperties{Has: boolPtr(true)},
						Description:          "Adapter-specific data (structure varies by adapter type)",
					},
				},
				"conditions": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"array"},
						Items: &openapi3.SchemaRef{
							Ref: "#/components/schemas/AdapterCondition",
						},
						Description: "Kubernetes-style conditions tracking adapter state",
					},
				},
				"created_time": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Format:      "date-time",
						Description: "When this adapter status was first created (API-managed)",
					},
				},
				"last_report_time": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Format:      "date-time",
						Description: "When this adapter last reported its status (API-managed)",
					},
				},
			},
			Description: "AdapterStatus represents the complete status report from an adapter",
		},
	}

	// AdapterCondition schema
	doc.Components.Schemas["AdapterCondition"] = &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:     &openapi3.Types{"object"},
			Required: []string{"type", "last_transition_time", "status"},
			Properties: openapi3.Schemas{
				"type": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Description: "Condition type",
					},
				},
				"reason": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Description: "Machine-readable reason code",
					},
				},
				"message": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Description: "Human-readable message",
					},
				},
				"last_transition_time": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Format:      "date-time",
						Description: "When this condition last transitioned status (API-managed)",
					},
				},
				"status": &openapi3.SchemaRef{
					Ref: "#/components/schemas/AdapterConditionStatus",
				},
			},
			Description: "Condition in AdapterStatus",
		},
	}

	// AdapterConditionStatus enum
	doc.Components.Schemas["AdapterConditionStatus"] = &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:        &openapi3.Types{"string"},
			Enum:        []interface{}{"True", "False", "Unknown"},
			Description: "Status value for adapter conditions",
		},
	}

	// AdapterStatusCreateRequest schema
	doc.Components.Schemas["AdapterStatusCreateRequest"] = &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:     &openapi3.Types{"object"},
			Required: []string{"adapter", "observed_generation", "observed_time", "conditions"},
			Properties: openapi3.Schemas{
				"adapter": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Description: "Adapter name (e.g., \"validator\", \"dns\", \"provisioner\")",
					},
				},
				"observed_generation": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"integer"},
						Format:      "int32",
						Description: "Which generation of the resource this status reflects",
					},
				},
				"observed_time": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						Format:      "date-time",
						Description: "When the adapter observed this resource state",
					},
				},
				"metadata": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"object"},
						Description: "Job execution metadata",
						Properties: openapi3.Schemas{
							"job_name":       &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
							"job_namespace":  &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
							"attempt":        &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}, Format: "int32"}},
							"started_time":   &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}, Format: "date-time"}},
							"completed_time": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}, Format: "date-time"}},
							"duration":       &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
						},
					},
				},
				"data": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:                 &openapi3.Types{"object"},
						AdditionalProperties: openapi3.AdditionalProperties{Has: boolPtr(true)},
						Description:          "Adapter-specific data (structure varies by adapter type)",
					},
				},
				"conditions": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"array"},
						Items: &openapi3.SchemaRef{
							Ref: "#/components/schemas/ConditionRequest",
						},
					},
				},
			},
			Description: "Request payload for creating/updating adapter status",
		},
	}

	// ConditionRequest schema
	doc.Components.Schemas["ConditionRequest"] = &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:     &openapi3.Types{"object"},
			Required: []string{"type", "status"},
			Properties: openapi3.Schemas{
				"type": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
				"status": &openapi3.SchemaRef{
					Ref: "#/components/schemas/AdapterConditionStatus",
				},
				"reason": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
				"message": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
			Description: "Condition data for create/update requests (from adapters)",
		},
	}

	// AdapterStatusList schema
	doc.Components.Schemas["AdapterStatusList"] = &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:     &openapi3.Types{"object"},
			Required: []string{"kind", "page", "size", "total", "items"},
			Properties: openapi3.Schemas{
				"kind": &openapi3.SchemaRef{
					Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
				},
				"page": &openapi3.SchemaRef{
					Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}, Format: "int32"},
				},
				"size": &openapi3.SchemaRef{
					Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}, Format: "int32"},
				},
				"total": &openapi3.SchemaRef{
					Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}, Format: "int32"},
				},
				"items": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"array"},
						Items: &openapi3.SchemaRef{
							Ref: "#/components/schemas/AdapterStatus",
						},
					},
				},
			},
			Description: "List of adapter statuses with pagination metadata",
		},
	}

	// OrderDirection enum
	doc.Components.Schemas["OrderDirection"] = &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type: &openapi3.Types{"string"},
			Enum: []interface{}{"asc", "desc"},
		},
	}
}

// addCommonParameters adds reusable query parameters for pagination and search.
func addCommonParameters(doc *openapi3.T) {
	// Page parameter
	doc.Components.Parameters["page"] = &openapi3.ParameterRef{
		Value: &openapi3.Parameter{
			Name:     "page",
			In:       "query",
			Required: false,
			Schema: &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type:    &openapi3.Types{"integer"},
					Format:  "int32",
					Default: 1,
				},
			},
		},
	}

	// PageSize parameter
	doc.Components.Parameters["pageSize"] = &openapi3.ParameterRef{
		Value: &openapi3.Parameter{
			Name:     "pageSize",
			In:       "query",
			Required: false,
			Schema: &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type:    &openapi3.Types{"integer"},
					Format:  "int32",
					Default: 20,
				},
			},
		},
	}

	// OrderBy parameter
	doc.Components.Parameters["orderBy"] = &openapi3.ParameterRef{
		Value: &openapi3.Parameter{
			Name:     "orderBy",
			In:       "query",
			Required: false,
			Schema: &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type:    &openapi3.Types{"string"},
					Default: "created_time",
				},
			},
		},
	}

	// Order parameter
	doc.Components.Parameters["order"] = &openapi3.ParameterRef{
		Value: &openapi3.Parameter{
			Name:     "order",
			In:       "query",
			Required: false,
			Schema: &openapi3.SchemaRef{
				Ref: "#/components/schemas/OrderDirection",
			},
		},
	}

	// Search parameter
	doc.Components.Parameters["search"] = &openapi3.ParameterRef{
		Value: &openapi3.Parameter{
			Name:        "search",
			In:          "query",
			Required:    false,
			Description: "Filter results using TSL (Tree Search Language) query syntax. Examples: `status.conditions.Ready='True'`, `name in ('c1','c2')`, `labels.region='us-east'`",
			Schema: &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"string"},
				},
			},
		},
	}
}

// addSecuritySchemes adds the security schemes to the OpenAPI spec.
func addSecuritySchemes(doc *openapi3.T) {
	doc.Components.SecuritySchemes = openapi3.SecuritySchemes{
		"BearerAuth": &openapi3.SecuritySchemeRef{
			Value: &openapi3.SecurityScheme{
				Type:   "http",
				Scheme: "bearer",
			},
		},
	}
}

// boolPtr returns a pointer to a boolean value.
func boolPtr(b bool) *bool {
	return &b
}
