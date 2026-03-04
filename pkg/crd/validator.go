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

package crd

import (
	"fmt"
	"strings"
	"unicode"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// ValidateCRDGovernance checks that a CRD follows HyperFleet conventions.
// Returns a list of violation descriptions. An empty list means the CRD is compliant.
func ValidateCRDGovernance(crd *apiextensionsv1.CustomResourceDefinition) []string {
	var violations []string

	// Rule: Group must equal hyperfleet.io
	if crd.Spec.Group != HyperfleetGroup {
		violations = append(violations, fmt.Sprintf("spec.group must be %q, got %q", HyperfleetGroup, crd.Spec.Group))
	}

	// Rule: Must have app.kubernetes.io/part-of: hyperfleet label
	labels := crd.Labels
	if labels == nil || labels["app.kubernetes.io/part-of"] != "hyperfleet" {
		violations = append(violations, "must have label app.kubernetes.io/part-of=hyperfleet")
	}

	annotations := crd.Annotations
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// Rule: Scope annotation must be "Root" or "Owned"
	scopeStr := annotations[AnnotationScope]
	if scopeStr != "Root" && scopeStr != "Owned" {
		violations = append(violations, fmt.Sprintf("annotation %s must be \"Root\" or \"Owned\", got %q", AnnotationScope, scopeStr))
	}

	// Rule: If scope is "Owned", owner-kind must be set
	if scopeStr == "Owned" && annotations[AnnotationOwnerKind] == "" {
		violations = append(violations, fmt.Sprintf("owned resource must have annotation %s", AnnotationOwnerKind))
	}

	// Rule: Required adapters must be set and non-empty
	adaptersStr := annotations[AnnotationRequiredAdapters]
	if strings.TrimSpace(adaptersStr) == "" {
		violations = append(violations, fmt.Sprintf("annotation %s must be set and non-empty", AnnotationRequiredAdapters))
	}

	// Rule: Must have exactly one version with storage: true
	storageVersions := 0
	var storageVersion *apiextensionsv1.CustomResourceDefinitionVersion
	for i := range crd.Spec.Versions {
		if crd.Spec.Versions[i].Storage {
			storageVersions++
			storageVersion = &crd.Spec.Versions[i]
		}
	}
	if storageVersions != 1 {
		violations = append(violations, fmt.Sprintf("must have exactly one version with storage: true, found %d", storageVersions))
	}

	// Rules that require a valid storage version with schema
	if storageVersion != nil {
		if storageVersion.Schema == nil || storageVersion.Schema.OpenAPIV3Schema == nil {
			violations = append(violations, "storage version must have openAPIV3Schema")
		} else {
			schema := storageVersion.Schema.OpenAPIV3Schema

			// Rule: Must have spec property of type object
			specSchema, hasSpec := schema.Properties["spec"]
			if !hasSpec {
				violations = append(violations, "openAPIV3Schema must have a spec property")
			} else {
				if specSchema.Type != "object" {
					violations = append(violations, "spec must be of type object")
				}

				// Rule: spec must NOT have x-kubernetes-preserve-unknown-fields: true
				if specSchema.XPreserveUnknownFields != nil && *specSchema.XPreserveUnknownFields {
					violations = append(violations, "spec must not have x-kubernetes-preserve-unknown-fields: true (define explicit properties instead)")
				}

				// Rule: spec must define at least one property
				if len(specSchema.Properties) == 0 {
					violations = append(violations, "spec must define at least one property")
				}
			}
		}

		// Rule: subresources.status must be defined
		if storageVersion.Subresources == nil || storageVersion.Subresources.Status == nil {
			violations = append(violations, "storage version must define subresources.status")
		}
	}

	// Rule: kind must be PascalCase
	kind := crd.Spec.Names.Kind
	if kind == "" || !isPascalCase(kind) {
		violations = append(violations, fmt.Sprintf("kind %q must be PascalCase", kind))
	}

	// Rule: plural and singular must be lowercase
	plural := crd.Spec.Names.Plural
	if plural != strings.ToLower(plural) {
		violations = append(violations, fmt.Sprintf("plural %q must be lowercase", plural))
	}
	singular := crd.Spec.Names.Singular
	if singular != strings.ToLower(singular) {
		violations = append(violations, fmt.Sprintf("singular %q must be lowercase", singular))
	}

	// Rule: metadata.name must equal {plural}.hyperfleet.io
	expectedName := plural + "." + HyperfleetGroup
	if crd.Name != expectedName {
		violations = append(violations, fmt.Sprintf("metadata.name must be %q, got %q", expectedName, crd.Name))
	}

	return violations
}

// isPascalCase returns true if s starts with an uppercase letter and contains no underscores or hyphens.
func isPascalCase(s string) bool {
	if len(s) == 0 {
		return false
	}
	if !unicode.IsUpper(rune(s[0])) {
		return false
	}
	for _, r := range s {
		if r == '_' || r == '-' || r == ' ' {
			return false
		}
	}
	return true
}
