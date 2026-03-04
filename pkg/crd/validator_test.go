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
	"testing"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// newValidCRD returns a CRD that passes all governance checks
func newValidCRD() *apiextensionsv1.CustomResourceDefinition {
	preserveFalse := false
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "widgets.hyperfleet.io",
			Labels: map[string]string{
				"app.kubernetes.io/part-of": "hyperfleet",
			},
			Annotations: map[string]string{
				AnnotationScope:            "Root",
				AnnotationRequiredAdapters: "validation",
			},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: HyperfleetGroup,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind:     "Widget",
				Plural:   "widgets",
				Singular: "widget",
			},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"spec": {
									Type: "object",
									XPreserveUnknownFields: &preserveFalse,
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"name": {Type: "string"},
									},
								},
								"status": {
									Type: "object",
								},
							},
						},
					},
					Subresources: &apiextensionsv1.CustomResourceSubresources{
						Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
					},
				},
			},
		},
	}
}

func TestValidateCRDGovernance_ValidCRD(t *testing.T) {
	RegisterTestingT(t)
	violations := ValidateCRDGovernance(newValidCRD())
	Expect(violations).To(BeEmpty())
}

func TestValidateCRDGovernance_WrongGroup(t *testing.T) {
	RegisterTestingT(t)
	crd := newValidCRD()
	crd.Spec.Group = "other.io"
	violations := ValidateCRDGovernance(crd)
	Expect(violations).To(ContainElement(ContainSubstring("spec.group must be")))
}

func TestValidateCRDGovernance_MissingLabel(t *testing.T) {
	RegisterTestingT(t)
	crd := newValidCRD()
	crd.Labels = nil
	violations := ValidateCRDGovernance(crd)
	Expect(violations).To(ContainElement(ContainSubstring("app.kubernetes.io/part-of=hyperfleet")))
}

func TestValidateCRDGovernance_InvalidScope(t *testing.T) {
	RegisterTestingT(t)
	crd := newValidCRD()
	crd.Annotations[AnnotationScope] = "Invalid"
	violations := ValidateCRDGovernance(crd)
	Expect(violations).To(ContainElement(ContainSubstring("must be \"Root\" or \"Owned\"")))
}

func TestValidateCRDGovernance_OwnedMissingOwnerKind(t *testing.T) {
	RegisterTestingT(t)
	crd := newValidCRD()
	crd.Annotations[AnnotationScope] = "Owned"
	// No owner-kind set
	violations := ValidateCRDGovernance(crd)
	Expect(violations).To(ContainElement(ContainSubstring("owner-kind")))
}

func TestValidateCRDGovernance_OwnedWithOwnerKind(t *testing.T) {
	RegisterTestingT(t)
	crd := newValidCRD()
	crd.Annotations[AnnotationScope] = "Owned"
	crd.Annotations[AnnotationOwnerKind] = "Cluster"
	violations := ValidateCRDGovernance(crd)
	// Should have no scope/owner violations
	for _, v := range violations {
		Expect(v).ToNot(ContainSubstring("owner-kind"))
	}
}

func TestValidateCRDGovernance_MissingAdapters(t *testing.T) {
	RegisterTestingT(t)
	crd := newValidCRD()
	crd.Annotations[AnnotationRequiredAdapters] = ""
	violations := ValidateCRDGovernance(crd)
	Expect(violations).To(ContainElement(ContainSubstring("required-adapters")))
}

func TestValidateCRDGovernance_OpaqueSpec(t *testing.T) {
	RegisterTestingT(t)
	crd := newValidCRD()
	preserveTrue := true
	version := &crd.Spec.Versions[0]
	version.Schema.OpenAPIV3Schema.Properties["spec"] = apiextensionsv1.JSONSchemaProps{
		Type:                   "object",
		XPreserveUnknownFields: &preserveTrue,
		Properties: map[string]apiextensionsv1.JSONSchemaProps{
			"name": {Type: "string"},
		},
	}
	violations := ValidateCRDGovernance(crd)
	Expect(violations).To(ContainElement(ContainSubstring("x-kubernetes-preserve-unknown-fields")))
}

func TestValidateCRDGovernance_SpecNoProperties(t *testing.T) {
	RegisterTestingT(t)
	crd := newValidCRD()
	version := &crd.Spec.Versions[0]
	version.Schema.OpenAPIV3Schema.Properties["spec"] = apiextensionsv1.JSONSchemaProps{
		Type: "object",
		// No properties defined
	}
	violations := ValidateCRDGovernance(crd)
	Expect(violations).To(ContainElement(ContainSubstring("at least one property")))
}

func TestValidateCRDGovernance_KindNotPascalCase(t *testing.T) {
	RegisterTestingT(t)
	crd := newValidCRD()
	crd.Spec.Names.Kind = "widget" // lowercase
	violations := ValidateCRDGovernance(crd)
	Expect(violations).To(ContainElement(ContainSubstring("PascalCase")))
}

func TestValidateCRDGovernance_PluralNotLowercase(t *testing.T) {
	RegisterTestingT(t)
	crd := newValidCRD()
	crd.Spec.Names.Plural = "Widgets"
	violations := ValidateCRDGovernance(crd)
	Expect(violations).To(ContainElement(ContainSubstring("plural")))
}

func TestValidateCRDGovernance_WrongMetadataName(t *testing.T) {
	RegisterTestingT(t)
	crd := newValidCRD()
	crd.Name = "wrong-name"
	violations := ValidateCRDGovernance(crd)
	Expect(violations).To(ContainElement(ContainSubstring("metadata.name must be")))
}

func TestValidateCRDGovernance_MissingStatusSubresource(t *testing.T) {
	RegisterTestingT(t)
	crd := newValidCRD()
	crd.Spec.Versions[0].Subresources = nil
	violations := ValidateCRDGovernance(crd)
	Expect(violations).To(ContainElement(ContainSubstring("subresources.status")))
}

func TestValidateCRDGovernance_MultipleStorageVersions(t *testing.T) {
	RegisterTestingT(t)
	crd := newValidCRD()
	// Add a second storage version
	crd.Spec.Versions = append(crd.Spec.Versions, apiextensionsv1.CustomResourceDefinitionVersion{
		Name:    "v2",
		Served:  true,
		Storage: true,
	})
	violations := ValidateCRDGovernance(crd)
	Expect(violations).To(ContainElement(ContainSubstring("exactly one version with storage: true")))
}

func TestValidateCRDGovernance_ExistingCRDsPassValidation(t *testing.T) {
	RegisterTestingT(t)

	// Load the actual CRD files and verify they pass governance
	registry := NewRegistry()
	err := registry.LoadFromDirectory("../../charts/crds")
	Expect(err).To(BeNil())
	Expect(registry.Count()).To(BeNumerically(">=", 3))
}

func TestIsPascalCase(t *testing.T) {
	RegisterTestingT(t)

	Expect(isPascalCase("Cluster")).To(BeTrue())
	Expect(isPascalCase("NodePool")).To(BeTrue())
	Expect(isPascalCase("IDP")).To(BeTrue())
	Expect(isPascalCase("cluster")).To(BeFalse())
	Expect(isPascalCase("node_pool")).To(BeFalse())
	Expect(isPascalCase("node-pool")).To(BeFalse())
	Expect(isPascalCase("")).To(BeFalse())
}
