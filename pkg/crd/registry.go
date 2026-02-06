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
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"gopkg.in/yaml.v3"
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

// LoadFromDirectory loads all YAML CRD files from the specified directory.
// Files must have .yaml or .yml extension.
func (r *Registry) LoadFromDirectory(dirPath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if directory exists
	info, err := os.Stat(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("CRD directory does not exist: %s", dirPath)
		}
		return fmt.Errorf("failed to stat CRD directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("CRD path is not a directory: %s", dirPath)
	}

	// Find all YAML files
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("failed to read CRD directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		filePath := filepath.Join(dirPath, entry.Name())
		if err := r.loadFile(filePath); err != nil {
			return fmt.Errorf("failed to load CRD from %s: %w", filePath, err)
		}
	}

	return nil
}

// loadFile loads a single CRD YAML file.
func (r *Registry) loadFile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	var def api.ResourceDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Validate required fields
	if def.Kind == "" {
		return fmt.Errorf("missing required field 'kind'")
	}
	if def.Plural == "" {
		return fmt.Errorf("missing required field 'plural'")
	}
	if def.Scope == "" {
		return fmt.Errorf("missing required field 'scope'")
	}

	// Validate scope value
	if def.Scope != api.ResourceScopeRoot && def.Scope != api.ResourceScopeOwned {
		return fmt.Errorf("invalid scope '%s': must be 'Root' or 'Owned'", def.Scope)
	}

	// Validate owned resources have owner configuration
	if def.Scope == api.ResourceScopeOwned && def.Owner == nil {
		return fmt.Errorf("owned resource '%s' must have 'owner' configuration", def.Kind)
	}

	// Set singular default if not provided
	if def.Singular == "" {
		def.Singular = def.Kind
	}

	// Check for duplicates
	if _, exists := r.byKind[def.Kind]; exists {
		return fmt.Errorf("duplicate kind '%s'", def.Kind)
	}
	if _, exists := r.byPlural[def.Plural]; exists {
		return fmt.Errorf("duplicate plural '%s'", def.Plural)
	}

	// Register the CRD
	r.byKind[def.Kind] = &def
	r.byPlural[def.Plural] = &def
	if def.Enabled {
		r.all = append(r.all, &def)
	}

	return nil
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

// Default returns the global default registry.
func Default() *Registry {
	return defaultRegistry
}

// LoadFromDirectory loads CRDs into the default registry.
func LoadFromDirectory(dirPath string) error {
	return defaultRegistry.LoadFromDirectory(dirPath)
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
