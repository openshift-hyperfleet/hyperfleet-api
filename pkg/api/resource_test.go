package api

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
)

func setupTestRegistry() {
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:   "Channel",
		Plural: "channels",
	})
	registry.Register(registry.EntityDescriptor{
		Kind:       "Version",
		Plural:     "versions",
		ParentKind: "Channel",
	})
}

func strPtr(s string) *string {
	return &s
}

func TestResourceList_Index(t *testing.T) {
	RegisterTestingT(t)

	emptyList := ResourceList{}
	emptyIndex := emptyList.Index()
	Expect(len(emptyIndex)).To(Equal(0))

	r1 := &Resource{}
	r1.ID = "res-1"
	r1.Name = "test-resource-1"

	r2 := &Resource{}
	r2.ID = "res-2"
	r2.Name = "test-resource-2"

	multiList := ResourceList{r1, r2}
	multiIndex := multiList.Index()
	Expect(len(multiIndex)).To(Equal(2))
	Expect(multiIndex["res-1"]).To(Equal(r1))
	Expect(multiIndex["res-2"]).To(Equal(r2))

	r1Dup := &Resource{}
	r1Dup.ID = "res-1"
	r1Dup.Name = "duplicate"

	dupList := ResourceList{r1, r1Dup}
	dupIndex := dupList.Index()
	Expect(len(dupIndex)).To(Equal(1))
	Expect(dupIndex["res-1"].Name).To(Equal("duplicate"))
}

func TestResource_BeforeCreate_IDGeneration(t *testing.T) {
	RegisterTestingT(t)
	setupTestRegistry()

	r := &Resource{Name: "test", Kind: "Channel"}

	err := r.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(r.ID).ToNot(BeEmpty())
}

func TestResource_BeforeCreate_IDPreservation(t *testing.T) {
	RegisterTestingT(t)
	setupTestRegistry()

	r := &Resource{Name: "test", Kind: "Channel"}
	r.ID = "pre-set-id"

	err := r.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(r.ID).To(Equal("pre-set-id"))
}

func TestResource_BeforeCreate_GenerationDefault(t *testing.T) {
	RegisterTestingT(t)
	setupTestRegistry()

	r := &Resource{Name: "test", Kind: "Channel"}

	err := r.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(r.Generation).To(Equal(int32(1)))
}

func TestResource_BeforeCreate_GenerationPreserved(t *testing.T) {
	RegisterTestingT(t)
	setupTestRegistry()

	r := &Resource{Name: "test", Kind: "Channel", Generation: 5}

	err := r.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(r.Generation).To(Equal(int32(5)))
}

func TestResource_BeforeCreate_Timestamps(t *testing.T) {
	RegisterTestingT(t)
	setupTestRegistry()

	before := time.Now()
	r := &Resource{Name: "test", Kind: "Channel"}

	err := r.BeforeCreate(nil)
	Expect(err).To(BeNil())

	Expect(r.CreatedTime).ToNot(BeZero())
	Expect(r.UpdatedTime).ToNot(BeZero())
	Expect(r.CreatedTime.After(before) || r.CreatedTime.Equal(before)).To(BeTrue())
	Expect(r.UpdatedTime.After(before) || r.UpdatedTime.Equal(before)).To(BeTrue())
}

func TestResource_BeforeCreate_CreatedTimePreserved(t *testing.T) {
	RegisterTestingT(t)
	setupTestRegistry()

	fixedTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	r := &Resource{Name: "test", Kind: "Channel"}
	r.CreatedTime = fixedTime

	err := r.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(r.CreatedTime).To(Equal(fixedTime))
}

func TestResource_BeforeCreate_HrefTopLevel(t *testing.T) {
	RegisterTestingT(t)
	setupTestRegistry()

	r := &Resource{Name: "stable", Kind: "Channel"}
	err := r.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(r.Href).To(Equal("/api/hyperfleet/v1/channels/" + r.ID))
}

func TestResource_BeforeCreate_HrefChild(t *testing.T) {
	RegisterTestingT(t)
	setupTestRegistry()

	r := &Resource{
		Name:      "4-17-12",
		Kind:      "Version",
		OwnerID:   strPtr("ch-1"),
		OwnerKind: strPtr("Channel"),
	}
	err := r.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(r.Href).To(Equal("/api/hyperfleet/v1/channels/ch-1/versions/" + r.ID))
	Expect(*r.OwnerHref).To(Equal("/api/hyperfleet/v1/channels/ch-1"))
}

func TestResource_BeforeCreate_OwnerKindMissing(t *testing.T) {
	RegisterTestingT(t)
	setupTestRegistry()

	r := &Resource{
		Name:    "4-17-12",
		Kind:    "Version",
		OwnerID: strPtr("ch-1"),
	}
	err := r.BeforeCreate(nil)
	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("owner_kind is required"))
}

func TestResource_BeforeCreate_OwnerKindEmpty(t *testing.T) {
	RegisterTestingT(t)
	setupTestRegistry()

	r := &Resource{
		Name:      "4-17-12",
		Kind:      "Version",
		OwnerID:   strPtr("ch-1"),
		OwnerKind: strPtr(""),
	}
	err := r.BeforeCreate(nil)
	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("owner_kind is required"))
}

func TestResource_BeforeCreate_HrefChildWithPresetOwnerHref(t *testing.T) {
	RegisterTestingT(t)
	setupTestRegistry()

	r := &Resource{
		Name:      "some-label",
		Kind:      "Version",
		OwnerID:   strPtr("v-1"),
		OwnerKind: strPtr("Version"),
		OwnerHref: strPtr("/api/hyperfleet/v1/channels/ch-1/versions/v-1"),
	}
	err := r.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(r.Href).To(Equal("/api/hyperfleet/v1/channels/ch-1/versions/v-1/versions/" + r.ID))
	Expect(*r.OwnerHref).To(Equal("/api/hyperfleet/v1/channels/ch-1/versions/v-1"))
}

func TestResource_BeforeCreate_HrefPreserved(t *testing.T) {
	RegisterTestingT(t)
	setupTestRegistry()

	r := &Resource{Name: "test", Kind: "Channel", Href: "/custom/href"}
	err := r.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(r.Href).To(Equal("/custom/href"))
}

func TestResource_BeforeUpdate_UpdatesTimestamp(t *testing.T) {
	RegisterTestingT(t)

	r := &Resource{Name: "test", Kind: "Channel"}
	r.UpdatedTime = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	before := time.Now()
	err := r.BeforeUpdate(nil)
	Expect(err).To(BeNil())
	Expect(r.UpdatedTime.After(before) || r.UpdatedTime.Equal(before)).To(BeTrue())
}

func TestResource_MarkDeleted(t *testing.T) {
	RegisterTestingT(t)

	r := &Resource{Name: "test", Kind: "Channel"}
	now := time.Now()

	r.MarkDeleted("admin", now)

	Expect(r.DeletedTime).ToNot(BeNil())
	Expect(*r.DeletedTime).To(Equal(now))
	Expect(r.DeletedBy).ToNot(BeNil())
	Expect(*r.DeletedBy).To(Equal("admin"))
}

func TestResource_IncrementGeneration(t *testing.T) {
	RegisterTestingT(t)

	r := &Resource{Name: "test", Kind: "Channel", Generation: 1}
	r.IncrementGeneration()
	Expect(r.Generation).To(Equal(int32(2)))

	r.IncrementGeneration()
	Expect(r.Generation).To(Equal(int32(3)))
}

func TestResource_TableName(t *testing.T) {
	RegisterTestingT(t)

	r := Resource{}
	Expect(r.TableName()).To(Equal("resources"))
}
