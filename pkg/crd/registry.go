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

// Package crd provides a registry for loading and managing Custom Resource Definitions.

package crd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

const (
	// HyperfleetGroup is the API group for HyperFleet CRDs
	HyperfleetGroup = "hyperfleet.io"

	// Annotation keys for HyperFleet-specific configuration
	AnnotationScope            = "hyperfleet.io/scope"
	AnnotationOwnerKind        = "hyperfleet.io/owner-kind"
	AnnotationOwnerPathParam   = "hyperfleet.io/owner-path-param"
	AnnotationRequiredAdapters = "hyperfleet.io/required-adapters"
	AnnotationEnabled          = "hyperfleet.io/enabled"
)

// Registry holds all loaded CRD definitions and provides lookup methods.
type Registry struct {
	mu       sync.RWMutex
	byKind   map[string]*api.ResourceDefinition
	byPlural map[string]*api.ResourceDefinition
	all      []*api.ResourceDefinition
}

// NewRegistry creates an empty CRD registry.
func NewRegistry() *Registry {
	return &Registry{
		byKind:   make(map[string]*api.ResourceDefinition),
		byPlural: make(map[string]*api.ResourceDefinition),
		all:      make([]*api.ResourceDefinition, 0),
	}
}

// LoadFromKubernetes loads CRDs from the Kubernetes API server.
// It discovers all CRDs in the hyperfleet.io group and parses their annotations.
func (r *Registry) LoadFromKubernetes(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Create Kubernetes client
	config, err := getKubeConfig()
	if err != nil {
		return fmt.Errorf("failed to get kubernetes config: %w", err)
	}

	clientset, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create apiextensions client: %w", err)
	}

	// List all CRDs
	crdList, err := clientset.ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list CRDs: %w", err)
	}

	// Filter and process HyperFleet CRDs
	for i := range crdList.Items {
		crd := &crdList.Items[i]
		if crd.Spec.Group != HyperfleetGroup {
			continue
		}

		def, err := r.parseCRD(crd)
		if err != nil {
			return fmt.Errorf("failed to parse CRD %s: %w", crd.Name, err)
		}

		// Register the CRD
		r.byKind[def.Kind] = def
		r.byPlural[def.Plural] = def
		if def.Enabled {
			r.all = append(r.all, def)
		}
	}

	return nil
}

// LoadFromDirectory loads CRDs from YAML files in the specified directory.
// Files must have .yaml or .yml extension and contain valid CRD definitions.
func (r *Registry) LoadFromDirectory(dir string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Find all YAML files
	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if !strings.HasSuffix(file.Name(), ".yaml") && !strings.HasSuffix(file.Name(), ".yml") {
			continue
		}

		path := filepath.Join(dir, file.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", path, err)
		}

		// Parse YAML into CRD
		var crd apiextensionsv1.CustomResourceDefinition
		if err := yaml.Unmarshal(data, &crd); err != nil {
			return fmt.Errorf("failed to parse CRD from %s: %w", path, err)
		}

		// Skip if not a HyperFleet CRD
		if crd.Spec.Group != HyperfleetGroup {
			continue
		}

		def, err := r.parseCRD(&crd)
		if err != nil {
			return fmt.Errorf("failed to parse CRD %s: %w", crd.Name, err)
		}

		// Register the CRD
		r.byKind[def.Kind] = def
		r.byPlural[def.Plural] = def
		if def.Enabled {
			r.all = append(r.all, def)
		}
	}

	return nil
}

// parseCRD converts a Kubernetes CRD to a ResourceDefinition using annotations.
func (r *Registry) parseCRD(crd *apiextensionsv1.CustomResourceDefinition) (*api.ResourceDefinition, error) {
	annotations := crd.Annotations
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// Parse scope
	scopeStr := annotations[AnnotationScope]
	if scopeStr == "" {
		scopeStr = "Root" // Default to Root
	}
	var scope api.ResourceScope
	switch scopeStr {
	case "Root":
		scope = api.ResourceScopeRoot
	case "Owned":
		scope = api.ResourceScopeOwned
	default:
		return nil, fmt.Errorf("invalid scope '%s': must be 'Root' or 'Owned'", scopeStr)
	}

	// Parse owner configuration for owned resources
	var owner *api.OwnerRef
	if scope == api.ResourceScopeOwned {
		ownerKind := annotations[AnnotationOwnerKind]
		ownerPathParam := annotations[AnnotationOwnerPathParam]
		if ownerKind == "" {
			return nil, fmt.Errorf("owned resource must have %s annotation", AnnotationOwnerKind)
		}
		if ownerPathParam == "" {
			ownerPathParam = strings.ToLower(ownerKind) + "_id"
		}
		owner = &api.OwnerRef{
			Kind:      ownerKind,
			PathParam: ownerPathParam,
		}
	}

	// Parse required adapters
	var requiredAdapters []string
	adaptersStr := annotations[AnnotationRequiredAdapters]
	if adaptersStr != "" {
		for _, adapter := range strings.Split(adaptersStr, ",") {
			adapter = strings.TrimSpace(adapter)
			if adapter != "" {
				requiredAdapters = append(requiredAdapters, adapter)
			}
		}
	}

	// Parse enabled flag
	enabledStr := annotations[AnnotationEnabled]
	enabled := enabledStr == "" || enabledStr == "true" // Default to enabled

	// Extract OpenAPI schema from CRD
	schema := extractOpenAPISchema(crd)

	def := &api.ResourceDefinition{
		APIVersion: HyperfleetGroup + "/v1",
		Kind:       crd.Spec.Names.Kind,
		Plural:     crd.Spec.Names.Plural,
		Singular:   crd.Spec.Names.Singular,
		Scope:      scope,
		Owner:      owner,
		StatusConfig: api.StatusConfig{
			RequiredAdapters: requiredAdapters,
		},
		Enabled: enabled,
		Schema:  schema,
	}

	return def, nil
}

// extractOpenAPISchema extracts the spec and status schemas from a CRD's openAPIV3Schema.
// It looks for the first served version and extracts the properties.spec and properties.status fields.
func extractOpenAPISchema(crd *apiextensionsv1.CustomResourceDefinition) *api.ResourceSchema {
	// Find the storage version or first served version
	var version *apiextensionsv1.CustomResourceDefinitionVersion
	for i := range crd.Spec.Versions {
		v := &crd.Spec.Versions[i]
		if v.Storage {
			version = v
			break
		}
		if version == nil && v.Served {
			version = v
		}
	}

	if version == nil || version.Schema == nil || version.Schema.OpenAPIV3Schema == nil {
		return nil
	}

	schema := &api.ResourceSchema{}
	openAPISchema := version.Schema.OpenAPIV3Schema

	// Extract properties from the schema
	if openAPISchema.Properties != nil {
		// Extract spec schema
		if specSchema, ok := openAPISchema.Properties["spec"]; ok {
			schema.Spec = jsonSchemaToMap(&specSchema)
		}

		// Extract status schema
		if statusSchema, ok := openAPISchema.Properties["status"]; ok {
			schema.Status = jsonSchemaToMap(&statusSchema)
		}
	}

	return schema
}

// jsonSchemaToMap converts a JSONSchemaProps to a map[string]interface{} for OpenAPI generation.
func jsonSchemaToMap(schema *apiextensionsv1.JSONSchemaProps) map[string]interface{} {
	if schema == nil {
		return nil
	}

	result := make(map[string]interface{})

	if schema.Type != "" {
		result["type"] = schema.Type
	}
	if schema.Description != "" {
		result["description"] = schema.Description
	}
	if schema.Format != "" {
		result["format"] = schema.Format
	}
	if len(schema.Enum) > 0 {
		enumValues := make([]interface{}, len(schema.Enum))
		for i, e := range schema.Enum {
			enumValues[i] = string(e.Raw)
		}
		result["enum"] = enumValues
	}
	if schema.Minimum != nil {
		result["minimum"] = *schema.Minimum
	}
	if schema.Maximum != nil {
		result["maximum"] = *schema.Maximum
	}
	if schema.MinLength != nil {
		result["minLength"] = *schema.MinLength
	}
	if schema.MaxLength != nil {
		result["maxLength"] = *schema.MaxLength
	}
	if schema.Pattern != "" {
		result["pattern"] = schema.Pattern
	}
	if schema.MinItems != nil {
		result["minItems"] = *schema.MinItems
	}
	if schema.MaxItems != nil {
		result["maxItems"] = *schema.MaxItems
	}
	if len(schema.Required) > 0 {
		result["required"] = schema.Required
	}

	// Handle properties (for object types)
	if len(schema.Properties) > 0 {
		props := make(map[string]interface{})
		for name, prop := range schema.Properties {
			props[name] = jsonSchemaToMap(&prop)
		}
		result["properties"] = props
	}

	// Handle items (for array types)
	if schema.Items != nil && schema.Items.Schema != nil {
		result["items"] = jsonSchemaToMap(schema.Items.Schema)
	}

	// Handle additionalProperties
	if schema.AdditionalProperties != nil {
		if schema.AdditionalProperties.Allows {
			result["additionalProperties"] = true
		} else if schema.AdditionalProperties.Schema != nil {
			result["additionalProperties"] = jsonSchemaToMap(schema.AdditionalProperties.Schema)
		}
	}

	// Handle x-kubernetes-preserve-unknown-fields (translates to additionalProperties: true)
	if schema.XPreserveUnknownFields != nil && *schema.XPreserveUnknownFields {
		result["additionalProperties"] = true
	}

	return result
}

// getKubeConfig returns a Kubernetes client config.
// It tries in-cluster config first, then falls back to kubeconfig file.
func getKubeConfig() (*rest.Config, error) {
	// Try in-cluster config first
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}

	// Fall back to kubeconfig
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	return kubeConfig.ClientConfig()
}

// Register adds a CRD definition programmatically.
func (r *Registry) Register(def *api.ResourceDefinition) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if def.Kind == "" || def.Plural == "" {
		return fmt.Errorf("kind and plural are required")
	}

	if _, exists := r.byKind[def.Kind]; exists {
		return fmt.Errorf("duplicate kind '%s'", def.Kind)
	}
	if _, exists := r.byPlural[def.Plural]; exists {
		return fmt.Errorf("duplicate plural '%s'", def.Plural)
	}

	r.byKind[def.Kind] = def
	r.byPlural[def.Plural] = def
	if def.Enabled {
		r.all = append(r.all, def)
	}

	return nil
}

// GetByKind returns the CRD definition for the given kind.
func (r *Registry) GetByKind(kind string) (*api.ResourceDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.byKind[kind]
	return def, ok
}

// GetByPlural returns the CRD definition for the given plural name.
func (r *Registry) GetByPlural(plural string) (*api.ResourceDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.byPlural[plural]
	return def, ok
}

// All returns all enabled CRD definitions.
func (r *Registry) All() []*api.ResourceDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*api.ResourceDefinition, len(r.all))
	copy(result, r.all)
	return result
}

// Count returns the number of enabled CRD definitions.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.all)
}

// Global default registry
var defaultRegistry = NewRegistry()

// DefaultRegistry returns the global default registry.
func DefaultRegistry() *Registry {
	return defaultRegistry
}

// LoadFromKubernetes loads CRDs into the default registry from Kubernetes API.
func LoadFromKubernetes(ctx context.Context) error {
	return defaultRegistry.LoadFromKubernetes(ctx)
}

// LoadFromDirectory loads CRDs into the default registry from local YAML files.
func LoadFromDirectory(dir string) error {
	return defaultRegistry.LoadFromDirectory(dir)
}

// GetByKind looks up a CRD by kind in the default registry.
func GetByKind(kind string) (*api.ResourceDefinition, bool) {
	return defaultRegistry.GetByKind(kind)
}

// GetByPlural looks up a CRD by plural name in the default registry.
func GetByPlural(plural string) (*api.ResourceDefinition, bool) {
	return defaultRegistry.GetByPlural(plural)
}

// All returns all enabled CRDs from the default registry.
func All() []*api.ResourceDefinition {
	return defaultRegistry.All()
}
