package tenant

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
)

func TestFromContext_NoTenant(t *testing.T) {
	RegisterTestingT(t)

	got := FromContext(context.Background())
	Expect(got).To(BeNil())
}

func TestWithTenant_RoundTrip(t *testing.T) {
	RegisterTestingT(t)

	want := &ResolvedTenant{Dimensions: map[string]string{"org": "acme"}}
	ctx := WithTenant(context.Background(), want)

	got := FromContext(ctx)
	Expect(got).To(Equal(want))
}

func TestTenancyJSON(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{
			name: "no tenant in context",
			ctx:  context.Background(),
			want: "{}",
		},
		{
			name: "system identity",
			ctx:  WithTenant(context.Background(), &ResolvedTenant{System: true, Dimensions: map[string]string{"org": "acme"}}),
			want: "{}",
		},
		{
			name: "empty dimensions",
			ctx:  WithTenant(context.Background(), &ResolvedTenant{Dimensions: map[string]string{}}),
			want: "{}",
		},
		{
			name: "nil dimensions",
			ctx:  WithTenant(context.Background(), &ResolvedTenant{}),
			want: "{}",
		},
		{
			name: "tenant with single dimension",
			ctx:  WithTenant(context.Background(), &ResolvedTenant{Dimensions: map[string]string{"org": "acme"}}),
			want: `{"org":"acme"}`,
		},
		{
			name: "tenant with multiple dimensions",
			ctx: WithTenant(context.Background(), &ResolvedTenant{
				Dimensions: map[string]string{"org": "acme", "project": "project-1"},
			}),
			want: `{"org":"acme","project":"project-1"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			got := TenancyJSON(tt.ctx)
			Expect(string(got)).To(MatchJSON(tt.want))
		})
	}
}
