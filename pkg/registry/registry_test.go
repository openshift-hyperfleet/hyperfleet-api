package registry

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestRegister_Success(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	d := EntityDescriptor{
		Kind:   "Channel",
		Plural: "channels",
	}

	Register(d)

	got, ok := Get("Channel")
	Expect(ok).To(BeTrue())
	Expect(got.Kind).To(Equal("Channel"))
	Expect(got.Plural).To(Equal("channels"))
}

func TestRegister_DuplicateKind_Panics(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Register(EntityDescriptor{Kind: "Channel", Plural: "channels"})

	Expect(func() {
		Register(EntityDescriptor{Kind: "Channel", Plural: "ch"})
	}).To(PanicWith(ContainSubstring("already registered")))
}

func TestRegister_EmptyKind_Panics(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Expect(func() {
		Register(EntityDescriptor{Kind: "", Plural: "things"})
	}).To(PanicWith(ContainSubstring("entity kind cannot be empty")))
}

func TestGet_NotFound(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	_, ok := Get("NonExistent")
	Expect(ok).To(BeFalse())
}

func TestMustGet_Success(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Register(EntityDescriptor{Kind: "Channel", Plural: "channels"})

	d := MustGet("Channel")
	Expect(d.Kind).To(Equal("Channel"))
}

func TestMustGet_NotFound_Panics(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Expect(func() {
		MustGet("NonExistent")
	}).To(PanicWith(ContainSubstring("not registered")))
}

func TestAll_ReturnsSnapshot(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Register(EntityDescriptor{Kind: "Channel", Plural: "channels"})
	Register(EntityDescriptor{Kind: "Version", Plural: "versions", ParentKind: "Channel"})

	all := All()
	Expect(all).To(HaveLen(2))

	types := make(map[string]bool)
	for _, d := range all {
		types[d.Kind] = true
	}
	Expect(types).To(HaveKey("Channel"))
	Expect(types).To(HaveKey("Version"))
}

func TestWithSpecSchema(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Expect(WithSpecSchema()).To(BeEmpty())

	Register(EntityDescriptor{Kind: "Channel", Plural: "channels"})
	Register(EntityDescriptor{
		Kind:           "Version",
		Plural:         "versions",
		ParentKind:     "Channel",
		SpecSchemaName: "VersionSpec",
	})
	Register(EntityDescriptor{Kind: "Cluster", Plural: "clusters", SpecSchemaName: "ClusterSpec"})
	Register(EntityDescriptor{Kind: "WifConfig", Plural: "wifconfigs", SpecSchemaName: "WifConfigSpec"})

	result := WithSpecSchema()
	Expect(result).To(HaveLen(3))

	var kinds []string
	for _, d := range result {
		kinds = append(kinds, d.Kind)
		Expect(d.SpecSchemaName).NotTo(BeEmpty())
	}
	Expect(kinds).To(ConsistOf("Version", "Cluster", "WifConfig"))
}

func TestChildrenOf(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Register(EntityDescriptor{Kind: "Channel", Plural: "channels"})
	Register(EntityDescriptor{Kind: "Version", Plural: "versions", ParentKind: "Channel"})
	Register(EntityDescriptor{Kind: "WifConfig", Plural: "wifconfigs"})

	children := ChildrenOf("Channel")
	Expect(children).To(HaveLen(1))
	Expect(children[0].Kind).To(Equal("Version"))

	children = ChildrenOf("WifConfig")
	Expect(children).To(BeEmpty())
}

func TestValidate_MissingParent_Panics(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Register(EntityDescriptor{Kind: "Version", Plural: "versions", ParentKind: "Ghost"})

	Expect(func() {
		Validate()
	}).To(PanicWith(ContainSubstring("unregistered parent kind")))
}

func TestValidate_DuplicatePlural_Panics(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Register(EntityDescriptor{Kind: "Channel", Plural: "things"})
	Register(EntityDescriptor{Kind: "Version", Plural: "things"})

	Expect(func() {
		Validate()
	}).To(PanicWith(ContainSubstring("duplicate plural")))
}

func TestRegister_EmptyPlural_Panics(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Expect(func() {
		Register(EntityDescriptor{Kind: "Channel", Plural: ""})
	}).To(PanicWith(ContainSubstring("has empty plural")))
}

func TestValidate_Success(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Register(EntityDescriptor{Kind: "Channel", Plural: "channels"})
	Register(EntityDescriptor{Kind: "Version", Plural: "versions", ParentKind: "Channel"})

	Expect(func() {
		Validate()
	}).ToNot(Panic())
}

func TestValidate_EmptyRegistry(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Expect(func() {
		Validate()
	}).ToNot(Panic())
}

func TestValidateSpecSchemas_PanicsOnMissingSchema(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Register(EntityDescriptor{
		Kind:              "Channel",
		Plural:            "channels",
		SpecSchemaName:    "ChannelSpec",
		RequireSpecSchema: true,
	})

	Expect(func() {
		ValidateSpecSchemas(func(name string) bool { return false })
	}).To(PanicWith(ContainSubstring("ChannelSpec")))
}

func TestValidateSpecSchemas_PassesWhenAllSchemasResolve(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Register(EntityDescriptor{
		Kind:              "Channel",
		Plural:            "channels",
		SpecSchemaName:    "ChannelSpec",
		RequireSpecSchema: true,
	})
	Register(EntityDescriptor{
		Kind:              "Version",
		Plural:            "versions",
		ParentKind:        "Channel",
		SpecSchemaName:    "VersionSpec",
		RequireSpecSchema: true,
	})

	Expect(func() {
		ValidateSpecSchemas(func(name string) bool { return true })
	}).ToNot(Panic())
}

func TestValidateSpecSchemas_SkipsDescriptorsWithoutSpecSchema(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Register(EntityDescriptor{
		Kind:   "Channel",
		Plural: "channels",
	})

	called := false
	ValidateSpecSchemas(func(name string) bool {
		called = true
		return false
	})
	Expect(called).To(BeFalse())
}

func TestValidateSpecSchemas_EmptyRegistry(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Expect(func() {
		ValidateSpecSchemas(func(name string) bool {
			t.Fatal("callback should not be called on empty registry")
			return false
		})
	}).ToNot(Panic())
}

func TestValidateSpecSchemas_MixedDescriptors(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Register(EntityDescriptor{
		Kind:   "Channel",
		Plural: "channels",
	})
	Register(EntityDescriptor{
		Kind:           "Version",
		Plural:         "versions",
		ParentKind:     "Channel",
		SpecSchemaName: "VersionSpec",
	})
	Register(EntityDescriptor{
		Kind:              "Cluster",
		Plural:            "clusters",
		SpecSchemaName:    "ClusterSpec",
		RequireSpecSchema: true,
	})

	resolved := map[string]bool{"VersionSpec": true, "ClusterSpec": true}
	Expect(func() {
		ValidateSpecSchemas(func(name string) bool { return resolved[name] })
	}).ToNot(Panic())

	resolved["ClusterSpec"] = false
	Expect(func() {
		ValidateSpecSchemas(func(name string) bool { return resolved[name] })
	}).To(PanicWith(ContainSubstring("ClusterSpec")))
}

func TestValidateSpecSchemas_SkipsNonRequiredMissingSchema(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Register(EntityDescriptor{
		Kind:           "Channel",
		Plural:         "channels",
		SpecSchemaName: "ChannelSpec",
	})

	Expect(func() {
		ValidateSpecSchemas(func(name string) bool { return false })
	}).ToNot(Panic())
}

func TestDescriptorFields(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Register(EntityDescriptor{
		Kind:                   "Version",
		Plural:                 "versions",
		ParentKind:             "Channel",
		OnParentDelete:         OnParentDeleteRestrict,
		SpecSchemaName:         "VersionSpec",
		SearchDisallowedFields: []string{"spec"},
	})

	d := MustGet("Version")
	Expect(d.ParentKind).To(Equal("Channel"))
	Expect(d.OnParentDelete).To(Equal(OnParentDeleteRestrict))
	Expect(d.SpecSchemaName).To(Equal("VersionSpec"))
	Expect(d.SearchDisallowedFields).To(ConsistOf("spec"))
}
