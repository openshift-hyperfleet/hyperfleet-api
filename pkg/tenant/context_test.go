package tenant

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"
)

func TestTenancyJSON(t *testing.T) {
	RegisterTestingT(t)

	// No tenant in context: empty map.
	Expect(string(TenancyJSON(context.Background()))).To(Equal("{}"))

	// System identity: empty map.
	sysCtx := WithTenant(context.Background(), &ResolvedTenant{System: true})
	Expect(string(TenancyJSON(sysCtx))).To(Equal("{}"))

	// Scoped tenant: dimensions serialized.
	ctx := WithTenant(context.Background(), &ResolvedTenant{
		Dimensions: map[string]string{"org": "acme", "project": "proj-1"},
	})
	var got map[string]string
	Expect(json.Unmarshal(TenancyJSON(ctx), &got)).To(Succeed())
	Expect(got).To(Equal(map[string]string{"org": "acme", "project": "proj-1"}))
}

func TestContainmentJSON(t *testing.T) {
	RegisterTestingT(t)

	_, scoped := ContainmentJSON(context.Background())
	Expect(scoped).To(BeFalse())

	_, scoped = ContainmentJSON(WithTenant(context.Background(), &ResolvedTenant{System: true}))
	Expect(scoped).To(BeFalse())

	_, scoped = ContainmentJSON(WithTenant(context.Background(), &ResolvedTenant{
		Dimensions: map[string]string{},
	}))
	Expect(scoped).To(BeFalse())

	containment, scoped := ContainmentJSON(WithTenant(context.Background(), &ResolvedTenant{
		Dimensions: map[string]string{"org": "acme"},
	}))
	Expect(scoped).To(BeTrue())
	var got map[string]string
	Expect(json.Unmarshal([]byte(containment), &got)).To(Succeed())
	Expect(got).To(Equal(map[string]string{"org": "acme"}))
}

func TestScoped(t *testing.T) {
	RegisterTestingT(t)

	Expect(Scoped(context.Background())).To(BeFalse())
	Expect(Scoped(WithTenant(context.Background(), &ResolvedTenant{System: true}))).To(BeFalse())
	Expect(Scoped(WithTenant(context.Background(), &ResolvedTenant{
		Dimensions: map[string]string{"org": "acme"},
	}))).To(BeTrue())
}
