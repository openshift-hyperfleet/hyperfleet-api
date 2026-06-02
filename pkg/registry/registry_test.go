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

func TestValidate_InvalidOnParentDelete_Panics(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Register(EntityDescriptor{Kind: "Channel", Plural: "channels"})
	Register(EntityDescriptor{
		Kind:           "Version",
		Plural:         "versions",
		ParentKind:     "Channel",
		OnParentDelete: "invalid",
	})

	Expect(func() {
		Validate()
	}).To(PanicWith(ContainSubstring("invalid on_parent_delete")))
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
