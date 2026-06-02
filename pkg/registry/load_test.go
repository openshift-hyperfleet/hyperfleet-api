package registry

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

func TestLoadDescriptors_Empty(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	LoadDescriptors(nil)
	Expect(All()).To(BeEmpty())

	LoadDescriptors([]EntityDescriptor{})
	Expect(All()).To(BeEmpty())
}

func TestLoadDescriptors_RegistersEntities(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	LoadDescriptors([]EntityDescriptor{
		{Kind: "Channel", Plural: "channels", SpecSchemaName: "ChannelSpec"},
		{
			Kind:           "Version",
			Plural:         "versions",
			ParentKind:     "Channel",
			SpecSchemaName: "VersionSpec",
		},
	})

	Expect(All()).To(HaveLen(2))
	version := MustGet("Version")
	Expect(version.OnParentDelete).To(Equal(OnParentDeleteRestrict))
}

func TestLoadDescriptors_DefaultOnParentDelete(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	LoadDescriptors([]EntityDescriptor{
		{Kind: "Channel", Plural: "channels"},
		{Kind: "Version", Plural: "versions", ParentKind: "Channel"},
	})

	Expect(MustGet("Version").OnParentDelete).To(Equal(OnParentDeleteRestrict))
}

func TestSearchDisallowedFieldsForKind(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Expect(SearchDisallowedFieldsForKind("Channel")).To(BeNil())

	Register(EntityDescriptor{
		Kind:                   "Channel",
		Plural:                 "channels",
		SearchDisallowedFields: []string{"spec"},
	})

	fields := SearchDisallowedFieldsForKind("Channel")
	Expect(fields).To(HaveKeyWithValue("spec", "spec"))
}

func TestValidateSchemas_MissingSchemaPanics(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Register(EntityDescriptor{
		Kind:           "Channel",
		Plural:         "channels",
		SpecSchemaName: "ChannelSpec",
	})

	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "schema.yaml")
	err := os.WriteFile(schemaPath, []byte(`
openapi: 3.0.0
info:
  title: Test
  version: 1.0.0
paths: {}
components:
  schemas: {}
`), 0600)
	Expect(err).NotTo(HaveOccurred())

	Expect(func() {
		ValidateSchemas(schemaPath)
	}).To(PanicWith(ContainSubstring(`spec_schema_name "ChannelSpec" not found`)))
}

func TestValidateSchemas_Success(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Register(EntityDescriptor{
		Kind:           "Channel",
		Plural:         "channels",
		SpecSchemaName: "ChannelSpec",
	})

	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "schema.yaml")
	err := os.WriteFile(schemaPath, []byte(`
openapi: 3.0.0
info:
  title: Test
  version: 1.0.0
paths: {}
components:
  schemas:
    ChannelSpec:
      type: object
`), 0600)
	Expect(err).NotTo(HaveOccurred())

	Expect(func() {
		ValidateSchemas(schemaPath)
	}).NotTo(Panic())
}

func TestValidateSchemas_NoDescriptorsNeedingSchema(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Expect(func() {
		ValidateSchemas("")
	}).NotTo(Panic())
}

func TestSchemaValidationKey(t *testing.T) {
	RegisterTestingT(t)
	Expect(SchemaValidationKey("Channel")).To(Equal("channel"))
}
