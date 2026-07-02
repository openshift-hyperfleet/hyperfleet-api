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

func TestLoadDescriptors_RegistersAll(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	descriptors := []EntityDescriptor{
		{Kind: "Channel", Plural: "channels", SpecSchemaName: "ChannelSpec"},
		{Kind: "Version", Plural: "versions", ParentKind: "Channel", SpecSchemaName: "VersionSpec"},
	}

	LoadDescriptors(descriptors)

	ch, ok := Get("Channel")
	Expect(ok).To(BeTrue())
	Expect(ch.Plural).To(Equal("channels"))
	Expect(ch.SpecSchemaName).To(Equal("ChannelSpec"))

	ver, ok := Get("Version")
	Expect(ok).To(BeTrue())
	Expect(ver.Plural).To(Equal("versions"))
	Expect(ver.ParentKind).To(Equal("Channel"))
}

func TestLoadDescriptors_EmptySlice(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	LoadDescriptors(nil)
	Expect(All()).To(BeEmpty())

	LoadDescriptors([]EntityDescriptor{})
	Expect(All()).To(BeEmpty())
}

func TestLoadDescriptors_DuplicateKind_Panics(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Register(EntityDescriptor{Kind: "Channel", Plural: "channels"})

	Expect(func() {
		LoadDescriptors([]EntityDescriptor{
			{Kind: "Channel", Plural: "ch"},
		})
	}).To(PanicWith(ContainSubstring("already registered")))
}

func TestLoadDescriptors_AllFields(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	LoadDescriptors([]EntityDescriptor{
		{
			Kind:                   "Cluster",
			Plural:                 "clusters",
			SpecSchemaName:         "ClusterSpec",
			SearchDisallowedFields: []string{"spec"},
			RequiredAdapters:       []string{"provisioner"},
		},
		{
			Kind:                   "NodePool",
			Plural:                 "nodepools",
			ParentKind:             "Cluster",
			OnParentDelete:         OnParentDeleteCascade,
			SpecSchemaName:         "NodePoolSpec",
			SearchDisallowedFields: []string{"spec"},
			RequiredAdapters:       []string{"provisioner", "lifecycle"},
		},
	})

	cluster := MustGet("Cluster")
	Expect(cluster.SpecSchemaName).To(Equal("ClusterSpec"))
	Expect(cluster.SearchDisallowedFields).To(ConsistOf("spec"))
	Expect(cluster.RequiredAdapters).To(ConsistOf("provisioner"))
	Expect(cluster.ParentKind).To(BeEmpty())

	np := MustGet("NodePool")
	Expect(np.ParentKind).To(Equal("Cluster"))
	Expect(np.OnParentDelete).To(Equal(OnParentDeleteCascade))
	Expect(np.SpecSchemaName).To(Equal("NodePoolSpec"))
	Expect(np.RequiredAdapters).To(ConsistOf("provisioner", "lifecycle"))
}

func TestValidate_ReferenceTargetKindUnregistered_Panics(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Register(EntityDescriptor{
		Kind:   "Cluster",
		Plural: "clusters",
		References: []ReferenceDescriptor{
			{RefType: "wif_config", TargetKind: "WifConfig"},
		},
	})

	Expect(func() {
		Validate()
	}).To(PanicWith(ContainSubstring("targets unregistered kind")))
}

func TestValidate_DuplicateRefType_Panics(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Register(EntityDescriptor{Kind: "WifConfig", Plural: "wifconfigs"})
	Register(EntityDescriptor{
		Kind:   "Cluster",
		Plural: "clusters",
		References: []ReferenceDescriptor{
			{RefType: "wif_config", TargetKind: "WifConfig"},
			{RefType: "wif_config", TargetKind: "WifConfig"},
		},
	})

	Expect(func() {
		Validate()
	}).To(PanicWith(ContainSubstring("duplicate ref_type")))
}

func TestValidate_ReferenceMaxLessThanMin_Panics(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Register(EntityDescriptor{Kind: "WifConfig", Plural: "wifconfigs"})
	Register(EntityDescriptor{
		Kind:   "Cluster",
		Plural: "clusters",
		References: []ReferenceDescriptor{
			{RefType: "wif_config", TargetKind: "WifConfig", Min: 2, Max: 1},
		},
	})

	Expect(func() {
		Validate()
	}).To(PanicWith(ContainSubstring("max (1) < min (2)")))
}

func TestValidate_ValidReferences_Success(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Register(EntityDescriptor{Kind: "WifConfig", Plural: "wifconfigs"})
	Register(EntityDescriptor{Kind: "Network", Plural: "networks"})
	Register(EntityDescriptor{
		Kind:   "Cluster",
		Plural: "clusters",
		References: []ReferenceDescriptor{
			{RefType: "wif_config", TargetKind: "WifConfig", Min: 1, Max: 1},
			{RefType: "network", TargetKind: "Network", Min: 0, Max: 0},
		},
	})

	Expect(func() {
		Validate()
	}).ToNot(Panic())
}
