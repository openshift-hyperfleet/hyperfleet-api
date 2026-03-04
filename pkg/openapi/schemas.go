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
	"github.com/getkin/kin-openapi/openapi3"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

// addResourceSchemas generates OpenAPI schemas for a resource definition.
// It creates: {Kind}, {Kind}Spec, {Kind}Status, {Kind}List, {Kind}CreateRequest
func addResourceSchemas(doc *openapi3.T, def *api.ResourceDefinition) {
	// Generate spec schema
	doc.Components.Schemas[def.Kind+"Spec"] = buildSpecSchema(def)

	// Generate status schema
	doc.Components.Schemas[def.Kind+"Status"] = buildStatusSchema(def)

	// Generate main resource schema
	doc.Components.Schemas[def.Kind] = buildResourceSchema(def)

	// Generate list schema
	doc.Components.Schemas[def.Kind+"List"] = buildListSchema(def)

	// Generate create request schema
	doc.Components.Schemas[def.Kind+"CreateRequest"] = buildCreateRequestSchema(def)
}

// buildSpecSchema creates the OpenAPI schema for a resource's spec field.
func buildSpecSchema(def *api.ResourceDefinition) *openapi3.SchemaRef {
	schema := &openapi3.Schema{
		Type:        &openapi3.Types{"object"},
		Description: def.Kind + " specification. Accepts any properties as the spec is provider-agnostic.",
	}

	// If we have schema information from the CRD, use it
	if def.Schema != nil && def.Schema.Spec != nil {
		applySchemaProperties(schema, def.Schema.Spec)
	} else {
		// Default to allowing additional properties for flexibility
		schema.AdditionalProperties = openapi3.AdditionalProperties{Has: boolPtr(true)}
	}

	return &openapi3.SchemaRef{Value: schema}
}

// buildStatusSchema creates the OpenAPI schema for a resource's status field.
func buildStatusSchema(def *api.ResourceDefinition) *openapi3.SchemaRef {
	schema := &openapi3.Schema{
		Type:     &openapi3.Types{"object"},
		Required: []string{"conditions"},
		Properties: openapi3.Schemas{
			"conditions": &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"array"},
					Items: &openapi3.SchemaRef{
						Ref: "#/components/schemas/ResourceCondition",
					},
					MinItems:    2,
					Description: "List of status conditions for the " + def.Singular + ".\n\n**Mandatory conditions**: \n- `type: \"Ready\"`: Whether all adapters report successfully at the current generation.\n- `type: \"Available\"`: Aggregated adapter result for a common observed_generation.\n\nThese conditions are present immediately upon resource creation.",
				},
			},
		},
		Description: def.Kind + " status computed from all status conditions.\n\nThis object is computed by the service and CANNOT be modified directly.",
	}

	return &openapi3.SchemaRef{Value: schema}
}

// buildResourceSchema creates the main OpenAPI schema for a resource.
func buildResourceSchema(def *api.ResourceDefinition) *openapi3.SchemaRef {
	required := []string{"name", "spec", "created_time", "updated_time", "created_by", "updated_by", "generation", "status"}

	properties := openapi3.Schemas{
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
		"labels": &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
				AdditionalProperties: openapi3.AdditionalProperties{
					Schema: &openapi3.SchemaRef{
						Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
					},
				},
				Description: "Labels for the API resource as pairs of name:value strings",
			},
		},
		"name": &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type:        &openapi3.Types{"string"},
				MinLength:   3,
				MaxLength:   uint64Ptr(63),
				Pattern:     "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$",
				Description: def.Kind + " name (unique)",
			},
		},
		"spec": &openapi3.SchemaRef{
			Ref: "#/components/schemas/" + def.Kind + "Spec",
		},
		"created_time": &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type:   &openapi3.Types{"string"},
				Format: "date-time",
			},
		},
		"updated_time": &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type:   &openapi3.Types{"string"},
				Format: "date-time",
			},
		},
		"created_by": &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type:   &openapi3.Types{"string"},
				Format: "email",
			},
		},
		"updated_by": &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type:   &openapi3.Types{"string"},
				Format: "email",
			},
		},
		"generation": &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type:        &openapi3.Types{"integer"},
				Format:      "int32",
				Min:         float64Ptr(1),
				Description: "Generation field is updated on customer updates, reflecting the version of the \"intent\" of the customer",
			},
		},
		"status": &openapi3.SchemaRef{
			Ref: "#/components/schemas/" + def.Kind + "Status",
		},
	}

	// Add owner_references for owned resources
	if def.IsOwned() {
		required = append(required, "owner_references")
		properties["owner_references"] = &openapi3.SchemaRef{
			Ref: "#/components/schemas/ObjectReference",
		}
	}

	return &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:       &openapi3.Types{"object"},
			Required:   required,
			Properties: properties,
		},
	}
}

// buildListSchema creates the OpenAPI schema for a list of resources.
func buildListSchema(def *api.ResourceDefinition) *openapi3.SchemaRef {
	return &openapi3.SchemaRef{
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
							Ref: "#/components/schemas/" + def.Kind,
						},
					},
				},
			},
		},
	}
}

// buildCreateRequestSchema creates the OpenAPI schema for a create request.
func buildCreateRequestSchema(def *api.ResourceDefinition) *openapi3.SchemaRef {
	return &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:     &openapi3.Types{"object"},
			Required: []string{"name", "spec"},
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
				"labels": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						AdditionalProperties: openapi3.AdditionalProperties{
							Schema: &openapi3.SchemaRef{
								Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
							},
						},
						Description: "Labels for the API resource as pairs of name:value strings",
					},
				},
				"name": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"string"},
						MinLength:   3,
						MaxLength:   uint64Ptr(63),
						Pattern:     "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$",
						Description: def.Kind + " name (unique)",
					},
				},
				"spec": &openapi3.SchemaRef{
					Ref: "#/components/schemas/" + def.Kind + "Spec",
				},
			},
		},
	}
}

// applySchemaProperties applies properties from a CRD schema map to an OpenAPI schema.
func applySchemaProperties(schema *openapi3.Schema, crdSchema map[string]interface{}) {
	if props, ok := crdSchema["properties"].(map[string]interface{}); ok {
		schema.Properties = make(openapi3.Schemas)
		for name, propSchema := range props {
			if propMap, ok := propSchema.(map[string]interface{}); ok {
				schema.Properties[name] = convertCRDSchemaToOpenAPI(propMap)
			}
		}
	}

	schema.Required = toStringSlice(crdSchema["required"])

	if desc, ok := crdSchema["description"].(string); ok {
		schema.Description = desc
	}

	applyAdditionalProperties(schema, crdSchema)
}

// convertCRDSchemaToOpenAPI converts a CRD schema map to an OpenAPI SchemaRef.
func convertCRDSchemaToOpenAPI(crdSchema map[string]interface{}) *openapi3.SchemaRef {
	schema := &openapi3.Schema{}

	if t, ok := crdSchema["type"].(string); ok {
		schema.Type = &openapi3.Types{t}
	}

	if desc, ok := crdSchema["description"].(string); ok {
		schema.Description = desc
	}

	if format, ok := crdSchema["format"].(string); ok {
		schema.Format = format
	}

	if enum, ok := crdSchema["enum"].([]interface{}); ok {
		schema.Enum = enum
	}

	if min, ok := crdSchema["minimum"].(float64); ok {
		schema.Min = &min
	}

	if max, ok := crdSchema["maximum"].(float64); ok {
		schema.Max = &max
	}

	if v, ok := toUint64(crdSchema["minLength"]); ok {
		schema.MinLength = v
	}

	if v, ok := toUint64(crdSchema["maxLength"]); ok {
		schema.MaxLength = &v
	}

	if pattern, ok := crdSchema["pattern"].(string); ok {
		schema.Pattern = pattern
	}

	if props, ok := crdSchema["properties"].(map[string]interface{}); ok {
		schema.Properties = make(openapi3.Schemas)
		for name, propSchema := range props {
			if propMap, ok := propSchema.(map[string]interface{}); ok {
				schema.Properties[name] = convertCRDSchemaToOpenAPI(propMap)
			}
		}
	}

	if items, ok := crdSchema["items"].(map[string]interface{}); ok {
		schema.Items = convertCRDSchemaToOpenAPI(items)
	}

	schema.Required = toStringSlice(crdSchema["required"])

	applyAdditionalProperties(schema, crdSchema)

	return &openapi3.SchemaRef{Value: schema}
}

// toStringSlice converts an interface{} (typically []interface{} from JSON/YAML unmarshal) to []string.
// Returns nil if the value is nil or not convertible.
func toStringSlice(v interface{}) []string {
	switch val := v.(type) {
	case []string:
		return val
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// toUint64 converts an interface{} (typically float64 or int from JSON/YAML unmarshal) to uint64.
func toUint64(v interface{}) (uint64, bool) {
	switch val := v.(type) {
	case float64:
		return uint64(val), true
	case int:
		return uint64(val), true
	case int64:
		return uint64(val), true
	}
	return 0, false
}

// applyAdditionalProperties handles the additionalProperties field which can be a bool or an object.
func applyAdditionalProperties(schema *openapi3.Schema, crdSchema map[string]interface{}) {
	ap := crdSchema["additionalProperties"]
	if ap == nil {
		return
	}
	switch val := ap.(type) {
	case bool:
		schema.AdditionalProperties = openapi3.AdditionalProperties{Has: boolPtr(val)}
	case map[string]interface{}:
		schema.AdditionalProperties = openapi3.AdditionalProperties{
			Schema: convertCRDSchemaToOpenAPI(val),
		}
	}
}

// uint64Ptr returns a pointer to a uint64 value.
func uint64Ptr(v uint64) *uint64 {
	return &v
}

// float64Ptr returns a pointer to a float64 value.
func float64Ptr(v float64) *float64 {
	return &v
}
