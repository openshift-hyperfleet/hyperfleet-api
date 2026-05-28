package auth

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/gomega"
)

func TestGetIdentityFromContext(t *testing.T) {
	tests := []struct {
		name          string
		claims        jwt.MapClaims
		identityField string
		want          string
		wantErr       bool
		errSubstring  string
	}{
		{
			name: "reads configured claim directly",
			claims: jwt.MapClaims{
				"email": "user@example.com",
				"sub":   "subject-id",
			},
			identityField: "sub",
			want:          "subject-id",
		},
		{
			name:          "defaults to email when field is empty",
			claims:        jwt.MapClaims{"email": "user@example.com"},
			identityField: "",
			want:          "user@example.com",
		},
		{
			name:          "falls back to preferred_username via payload normalization",
			claims:        jwt.MapClaims{"preferred_username": "jdoe"},
			identityField: "preferred_username",
			want:          "jdoe",
		},
		{
			name:          "returns error when claim is missing",
			claims:        jwt.MapClaims{"email": "user@example.com"},
			identityField: "missing_claim",
			wantErr:       true,
			errSubstring:  "missing_claim",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			ctx := contextWithClaims(tc.claims)

			identity, err := GetIdentityFromContext(ctx, tc.identityField)
			if tc.wantErr {
				Expect(err).To(HaveOccurred())
				if tc.errSubstring != "" {
					Expect(err.Error()).To(ContainSubstring(tc.errSubstring))
				}
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(identity).To(Equal(tc.want))
		})
	}
}

func contextWithClaims(claims jwt.MapClaims) context.Context {
	token := &jwt.Token{Claims: claims}
	return SetJWTTokenContext(context.Background(), token)
}
