package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/mendsley/gojwk"
	. "github.com/onsi/gomega"
)

const testKID = "test-key-1"

func TestJWTHandler(t *testing.T) {
	RegisterTestingT(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	jwksServer := newJWKSServer(t, &privateKey.PublicKey)
	defer jwksServer.Close()

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	handler, err := NewJWTHandler(t.Context(), JWTHandlerConfig{
		KeysURL:     jwksServer.URL,
		IssuerURL:   "https://test-issuer.example.com",
		PublicPaths: []string{"^/healthz$", "^/openapi$"},
		Next:        nextHandler,
	})
	Expect(err).NotTo(HaveOccurred())

	t.Run("valid token passes through", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, privateKey, jwt.MapClaims{
			"iss": "https://test-issuer.example.com",
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(),
		})
		rr := serve(handler, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(rr.Body.String()).To(Equal("ok"))
	})

	t.Run("valid token sets claims in context", func(t *testing.T) {
		RegisterTestingT(t)
		claimsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok := GetJWTTokenFromContext(r.Context())
			Expect(tok).NotTo(BeNil())
			claims, ok := tok.Claims.(jwt.MapClaims)
			Expect(ok).To(BeTrue())
			Expect(claims["username"]).To(Equal("testuser"))
			w.WriteHeader(http.StatusOK)
		})
		h, err := NewJWTHandler(t.Context(), JWTHandlerConfig{
			KeysURL:   jwksServer.URL,
			IssuerURL: "https://test-issuer.example.com",
			Next:      claimsHandler,
		})
		Expect(err).NotTo(HaveOccurred())

		token := signToken(t, privateKey, jwt.MapClaims{
			"iss":      "https://test-issuer.example.com",
			"exp":      time.Now().Add(time.Hour).Unix(),
			"iat":      time.Now().Unix(),
			"username": "testuser",
		})
		rr := serve(h, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusOK))
	})

	t.Run("expired token returns 401", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, privateKey, jwt.MapClaims{
			"iss": "https://test-issuer.example.com",
			"exp": time.Now().Add(-time.Hour).Unix(),
			"iat": time.Now().Add(-2 * time.Hour).Unix(),
		})
		rr := serve(handler, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})

	t.Run("invalid signature returns 401", func(t *testing.T) {
		RegisterTestingT(t)
		otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
		Expect(err).NotTo(HaveOccurred())
		token := signToken(t, otherKey, jwt.MapClaims{
			"iss": "https://test-issuer.example.com",
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(),
		})
		rr := serve(handler, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})

	t.Run("wrong issuer returns 401", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, privateKey, jwt.MapClaims{
			"iss": "https://wrong-issuer.example.com",
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(),
		})
		rr := serve(handler, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})

	t.Run("missing Authorization header returns 401", func(t *testing.T) {
		RegisterTestingT(t)
		rr := serve(handler, "/protected", "")
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})

	t.Run("malformed Authorization header returns 401", func(t *testing.T) {
		RegisterTestingT(t)
		rr := serve(handler, "/protected", "Basic dXNlcjpwYXNz")
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})

	t.Run("garbage token returns 401", func(t *testing.T) {
		RegisterTestingT(t)
		rr := serve(handler, "/protected", "Bearer not.a.jwt")
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})

	t.Run("public endpoint without token passes through", func(t *testing.T) {
		RegisterTestingT(t)
		rr := serve(handler, "/healthz", "")
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(rr.Body.String()).To(Equal("ok"))
	})

	t.Run("public endpoint with invalid token still passes through", func(t *testing.T) {
		RegisterTestingT(t)
		rr := serve(handler, "/healthz", "Bearer garbage")
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(rr.Body.String()).To(Equal("ok"))
	})

	t.Run("HS256 signed token rejected", func(t *testing.T) {
		RegisterTestingT(t)
		claims := jwt.MapClaims{
			"iss": "https://test-issuer.example.com",
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(),
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenString, err := tok.SignedString([]byte("secret-key-for-hmac"))
		Expect(err).NotTo(HaveOccurred())
		rr := serve(handler, "/protected", "Bearer "+tokenString)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})
}

func TestJWTHandler_FailClosed_NoValidKeys(t *testing.T) {
	RegisterTestingT(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer badServer.Close()

	handler, err := NewJWTHandler(t.Context(), JWTHandlerConfig{
		KeysURL:   badServer.URL,
		IssuerURL: "https://test-issuer.example.com",
		Next: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	})
	Expect(err).NotTo(HaveOccurred())

	token := signToken(t, privateKey, jwt.MapClaims{
		"iss": "https://test-issuer.example.com",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	rr := serve(handler, "/protected", "Bearer "+token)
	Expect(rr.Code).To(Equal(http.StatusUnauthorized))
}

func TestJWTHandler_RequiresKeysConfig(t *testing.T) {
	RegisterTestingT(t)

	_, err := NewJWTHandler(t.Context(), JWTHandlerConfig{
		IssuerURL: "https://test-issuer.example.com",
		Next:      http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
	})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("KeysFile or KeysURL"))
}

func TestJWTHandler_WithAudience(t *testing.T) {
	RegisterTestingT(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	jwksServer := newJWKSServer(t, &privateKey.PublicKey)
	defer jwksServer.Close()

	handler, err := NewJWTHandler(t.Context(), JWTHandlerConfig{
		KeysURL:   jwksServer.URL,
		IssuerURL: "https://test-issuer.example.com",
		Audience:  "my-api",
		Next: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	})
	Expect(err).NotTo(HaveOccurred())

	t.Run("correct audience passes", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, privateKey, jwt.MapClaims{
			"iss": "https://test-issuer.example.com",
			"aud": "my-api",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		rr := serve(handler, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusOK))
	})

	t.Run("wrong audience returns 401", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, privateKey, jwt.MapClaims{
			"iss": "https://test-issuer.example.com",
			"aud": "wrong-api",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		rr := serve(handler, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})
}

// --- helpers ---

func signToken(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = testKID
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return s
}

func serve(handler http.Handler, path, authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func newJWKSServer(t *testing.T, pubKey *rsa.PublicKey) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jwk, err := gojwk.PublicKey(pubKey)
		if err != nil {
			t.Errorf("failed to create JWK: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		jwk.Kid = testKID
		jwk.Alg = "RS256"
		jwkBytes, err := gojwk.Marshal(jwk)
		if err != nil {
			t.Errorf("failed to marshal JWK: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"keys":[%s]}`, string(jwkBytes))
	}))
}
