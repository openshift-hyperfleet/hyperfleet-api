package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/mendsley/gojwk"
	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
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
		Issuers: []config.JWTIssuerConfig{{
			IssuerURL:  "https://test-issuer.example.com",
			JWKCertURL: jwksServer.URL,
		}},
	})
	Expect(err).NotTo(HaveOccurred())
	mw := handler.Middleware(nextHandler)

	t.Run("valid token passes through", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, privateKey, jwt.MapClaims{
			"iss": "https://test-issuer.example.com",
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(),
		})
		rr := serve(mw, "/protected", "Bearer "+token)
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
			Issuers: []config.JWTIssuerConfig{{
				IssuerURL:  "https://test-issuer.example.com",
				JWKCertURL: jwksServer.URL,
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		token := signToken(t, privateKey, jwt.MapClaims{
			"iss":      "https://test-issuer.example.com",
			"exp":      time.Now().Add(time.Hour).Unix(),
			"iat":      time.Now().Unix(),
			"username": "testuser",
		})
		rr := serve(h.Middleware(claimsHandler), "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusOK))
	})

	t.Run("expired token returns 401", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, privateKey, jwt.MapClaims{
			"iss": "https://test-issuer.example.com",
			"exp": time.Now().Add(-time.Hour).Unix(),
			"iat": time.Now().Add(-2 * time.Hour).Unix(),
		})
		rr := serve(mw, "/protected", "Bearer "+token)
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
		rr := serve(mw, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})

	t.Run("wrong issuer returns 401", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, privateKey, jwt.MapClaims{
			"iss": "https://wrong-issuer.example.com",
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(),
		})
		rr := serve(mw, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})

	t.Run("missing Authorization header returns 401", func(t *testing.T) {
		RegisterTestingT(t)
		rr := serve(mw, "/protected", "")
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})

	t.Run("lowercase bearer scheme accepted per RFC 7235", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, privateKey, jwt.MapClaims{
			"iss": "https://test-issuer.example.com",
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(),
		})
		rr := serve(mw, "/protected", "bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusOK))
	})

	t.Run("malformed Authorization header returns 401", func(t *testing.T) {
		RegisterTestingT(t)
		rr := serve(mw, "/protected", "Basic dXNlcjpwYXNz")
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})

	t.Run("garbage token returns 401", func(t *testing.T) {
		RegisterTestingT(t)
		rr := serve(mw, "/protected", "Bearer not.a.jwt")
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
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
		rr := serve(mw, "/protected", "Bearer "+tokenString)
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
		Issuers: []config.JWTIssuerConfig{{
			IssuerURL:  "https://test-issuer.example.com",
			JWKCertURL: badServer.URL,
		}},
	})
	Expect(err).NotTo(HaveOccurred())

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	token := signToken(t, privateKey, jwt.MapClaims{
		"iss": "https://test-issuer.example.com",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	rr := serve(handler.Middleware(next), "/protected", "Bearer "+token)
	Expect(rr.Code).To(Equal(http.StatusUnauthorized))
}

func TestJWTHandler_RequiresIssuersConfig(t *testing.T) {
	RegisterTestingT(t)

	_, err := NewJWTHandler(t.Context(), JWTHandlerConfig{})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("at least one issuer"))
}

func TestJWTHandler_WithAudience(t *testing.T) {
	RegisterTestingT(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	jwksServer := newJWKSServer(t, &privateKey.PublicKey)
	defer jwksServer.Close()

	handler, err := NewJWTHandler(t.Context(), JWTHandlerConfig{
		Issuers: []config.JWTIssuerConfig{{
			IssuerURL:  "https://test-issuer.example.com",
			JWKCertURL: jwksServer.URL,
			Audience:   "my-api",
		}},
	})
	Expect(err).NotTo(HaveOccurred())

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := handler.Middleware(next)

	t.Run("correct audience passes", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, privateKey, jwt.MapClaims{
			"iss": "https://test-issuer.example.com",
			"aud": "my-api",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		rr := serve(mw, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusOK))
	})

	t.Run("wrong audience returns 401", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, privateKey, jwt.MapClaims{
			"iss": "https://test-issuer.example.com",
			"aud": "wrong-api",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		rr := serve(mw, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})
}

func TestJWTHandler_WithoutAudience_AcceptsAny(t *testing.T) {
	RegisterTestingT(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	jwksServer := newJWKSServer(t, &privateKey.PublicKey)
	defer jwksServer.Close()

	handler, err := NewJWTHandler(t.Context(), JWTHandlerConfig{
		Issuers: []config.JWTIssuerConfig{{
			IssuerURL:  "https://test-issuer.example.com",
			JWKCertURL: jwksServer.URL,
		}},
	})
	Expect(err).NotTo(HaveOccurred())

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	token := signToken(t, privateKey, jwt.MapClaims{
		"iss": "https://test-issuer.example.com",
		"aud": "any-audience",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	rr := serve(handler.Middleware(next), "/protected", "Bearer "+token)
	Expect(rr.Code).To(Equal(http.StatusOK))
}

func TestJWTHandler_FileOnlyKeyfunc(t *testing.T) {
	RegisterTestingT(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	jwksFile := writeJWKSFile(t, &privateKey.PublicKey)

	handler, err := NewJWTHandler(t.Context(), JWTHandlerConfig{
		Issuers: []config.JWTIssuerConfig{{
			IssuerURL:   "https://test-issuer.example.com",
			JWKCertFile: jwksFile,
		}},
	})
	Expect(err).NotTo(HaveOccurred())

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})
	mw := handler.Middleware(next)

	t.Run("valid token accepted via file keys", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, privateKey, jwt.MapClaims{
			"iss": "https://test-issuer.example.com",
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(),
		})
		rr := serve(mw, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(rr.Body.String()).To(Equal("ok"))
	})

	t.Run("wrong key rejected via file keys", func(t *testing.T) {
		RegisterTestingT(t)
		otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
		Expect(err).NotTo(HaveOccurred())
		token := signToken(t, otherKey, jwt.MapClaims{
			"iss": "https://test-issuer.example.com",
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(),
		})
		rr := serve(mw, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})
}

func TestJWTHandler_CombinedKeyfunc(t *testing.T) {
	RegisterTestingT(t)

	fileKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	jwksFile := writeJWKSFile(t, &fileKey.PublicKey)
	jwksServer := newJWKSServer(t, &fileKey.PublicKey)
	defer jwksServer.Close()

	handler, err := NewJWTHandler(t.Context(), JWTHandlerConfig{
		Issuers: []config.JWTIssuerConfig{{
			IssuerURL:   "https://test-issuer.example.com",
			JWKCertFile: jwksFile,
			JWKCertURL:  jwksServer.URL,
		}},
	})
	Expect(err).NotTo(HaveOccurred())

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := handler.Middleware(next)

	t.Run("constructor succeeds with both file and URL", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(handler).NotTo(BeNil())
	})

	t.Run("file key accepted in combined mode", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, fileKey, jwt.MapClaims{
			"iss": "https://test-issuer.example.com",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		rr := serve(mw, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusOK))
	})

	t.Run("unknown key rejected in combined mode", func(t *testing.T) {
		RegisterTestingT(t)
		otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
		Expect(err).NotTo(HaveOccurred())
		token := signToken(t, otherKey, jwt.MapClaims{
			"iss": "https://test-issuer.example.com",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		rr := serve(mw, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})
}

func TestJWTHandler_TLSWithCAFile(t *testing.T) {
	RegisterTestingT(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	tlsServer := newTLSJWKSServer(t, &privateKey.PublicKey)
	defer tlsServer.Close()

	caFile := writeTLSCAFile(t, tlsServer)

	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	handler, err := NewJWTHandler(t.Context(), JWTHandlerConfig{
		Issuers: []config.JWTIssuerConfig{{
			IssuerURL:     "https://test-issuer.example.com",
			JWKCertURL:    tlsServer.URL,
			JWKCertCAFile: caFile,
		}},
	})
	Expect(err).NotTo(HaveOccurred())
	defer handler.Close()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := handler.Middleware(next)

	cases := []struct {
		signingKey *rsa.PrivateKey
		name       string
		wantStatus int
	}{
		{privateKey, "valid token accepted", http.StatusOK},
		{otherKey, "wrong key rejected", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			token := signToken(t, tc.signingKey, jwt.MapClaims{
				"iss": "https://test-issuer.example.com",
				"exp": time.Now().Add(time.Hour).Unix(),
				"iat": time.Now().Unix(),
			})
			rr := serve(mw, "/protected", "Bearer "+token)
			Expect(rr.Code).To(Equal(tc.wantStatus))
		})
	}
}

func TestJWTHandler_CombinedKeyfuncWithCA(t *testing.T) {
	RegisterTestingT(t)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())
	tlsServer := newTLSJWKSServer(t, &privateKey.PublicKey)
	defer tlsServer.Close()
	caFile := writeTLSCAFile(t, tlsServer)
	jwksFile := writeJWKSFile(t, &privateKey.PublicKey)
	handler, err := NewJWTHandler(t.Context(), JWTHandlerConfig{
		Issuers: []config.JWTIssuerConfig{{
			IssuerURL:     "https://test-issuer.example.com",
			JWKCertURL:    tlsServer.URL,
			JWKCertFile:   jwksFile,
			JWKCertCAFile: caFile,
		}},
	})
	Expect(err).NotTo(HaveOccurred())
	defer handler.Close()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	token := signToken(t, privateKey, jwt.MapClaims{
		"iss": "https://test-issuer.example.com",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})
	rr := serve(handler.Middleware(next), "/protected", "Bearer "+token)
	Expect(rr.Code).To(Equal(http.StatusOK))
}

func TestJWTHandler_TLSWithoutCAFile_Fails(t *testing.T) {
	RegisterTestingT(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	tlsServer := newTLSJWKSServer(t, &privateKey.PublicKey)
	defer tlsServer.Close()

	handler, err := NewJWTHandler(t.Context(), JWTHandlerConfig{
		Issuers: []config.JWTIssuerConfig{{
			IssuerURL:  "https://test-issuer.example.com",
			JWKCertURL: tlsServer.URL,
		}},
	})
	Expect(err).NotTo(HaveOccurred())
	defer handler.Close()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	token := signToken(t, privateKey, jwt.MapClaims{
		"iss": "https://test-issuer.example.com",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	rr := serve(handler.Middleware(next), "/protected", "Bearer "+token)
	Expect(rr.Code).To(Equal(http.StatusUnauthorized))
}

func TestJWTHandler_CAFileErrors(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name          string
		caFile        func(t *testing.T) string
		wantErrSubstr string
	}{
		{
			"missing CA file",
			func(t *testing.T) string { return "/nonexistent/ca.crt" },
			"failed to read CA file",
		},
		{
			"invalid CA file",
			func(t *testing.T) string {
				badCAFile := filepath.Join(t.TempDir(), "bad-ca.crt")
				Expect(os.WriteFile(badCAFile, []byte("not a certificate"), 0600)).To(Succeed())
				return badCAFile
			},
			"failed to parse CA certificate",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			_, err := NewJWTHandler(t.Context(), JWTHandlerConfig{
				Issuers: []config.JWTIssuerConfig{{
					IssuerURL:     "https://test-issuer.example.com",
					JWKCertURL:    "https://keys.example.com",
					JWKCertCAFile: tc.caFile(t),
				}},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(tc.wantErrSubstr))
		})
	}
}

func TestJWTHandler_Close(t *testing.T) {
	RegisterTestingT(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	jwksServer := newJWKSServer(t, &privateKey.PublicKey)
	defer jwksServer.Close()

	handler, err := NewJWTHandler(t.Context(), JWTHandlerConfig{
		Issuers: []config.JWTIssuerConfig{{
			IssuerURL:  "https://test-issuer.example.com",
			JWKCertURL: jwksServer.URL,
		}},
	})
	Expect(err).NotTo(HaveOccurred())

	handler.Close()
	handler.Close() // idempotent, should not panic
}

func TestJWTHandler_ResponseBody(t *testing.T) {
	RegisterTestingT(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	jwksServer := newJWKSServer(t, &privateKey.PublicKey)
	defer jwksServer.Close()

	handler, err := NewJWTHandler(t.Context(), JWTHandlerConfig{
		Issuers: []config.JWTIssuerConfig{{
			IssuerURL:  "https://test-issuer.example.com",
			JWKCertURL: jwksServer.URL,
		}},
	})
	Expect(err).NotTo(HaveOccurred())

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := handler.Middleware(next)

	t.Run("missing header returns problem+json with no-credentials code", func(t *testing.T) {
		RegisterTestingT(t)
		rr := serve(mw, "/protected", "")
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
		Expect(rr.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))

		var body map[string]any
		Expect(json.NewDecoder(rr.Body).Decode(&body)).To(Succeed())
		Expect(body["code"]).To(Equal("HYPERFLEET-AUT-001"))
		Expect(body["status"]).To(BeNumerically("==", 401))
	})

	t.Run("expired token returns problem+json with expired code", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, privateKey, jwt.MapClaims{
			"iss": "https://test-issuer.example.com",
			"exp": time.Now().Add(-time.Hour).Unix(),
			"iat": time.Now().Add(-2 * time.Hour).Unix(),
		})
		rr := serve(mw, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
		Expect(rr.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))

		var body map[string]any
		Expect(json.NewDecoder(rr.Body).Decode(&body)).To(Succeed())
		Expect(body["code"]).To(Equal("HYPERFLEET-AUT-003"))
		Expect(body["status"]).To(BeNumerically("==", 401))
	})

	t.Run("invalid token returns problem+json with invalid-credentials code", func(t *testing.T) {
		RegisterTestingT(t)
		rr := serve(mw, "/protected", "Bearer not.a.jwt")
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
		Expect(rr.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))

		var body map[string]any
		Expect(json.NewDecoder(rr.Body).Decode(&body)).To(Succeed())
		Expect(body["code"]).To(Equal("HYPERFLEET-AUT-002"))
		Expect(body["status"]).To(BeNumerically("==", 401))
	})

	t.Run("non-Bearer scheme returns problem+json with invalid-credentials code", func(t *testing.T) {
		RegisterTestingT(t)
		rr := serve(mw, "/protected", "Basic dXNlcjpwYXNz")
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
		Expect(rr.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))

		var body map[string]any
		Expect(json.NewDecoder(rr.Body).Decode(&body)).To(Succeed())
		Expect(body["code"]).To(Equal("HYPERFLEET-AUT-002"))
		Expect(body["status"]).To(BeNumerically("==", 401))
	})
}

func TestJWTHandler_MultiIssuer(t *testing.T) {
	RegisterTestingT(t)

	key1, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())
	key2, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	jwksServer1 := newJWKSServer(t, &key1.PublicKey)
	defer jwksServer1.Close()
	jwksServer2 := newJWKSServer(t, &key2.PublicKey)
	defer jwksServer2.Close()

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issuerCfg, ok := GetJWTIssuerConfigFromContext(r.Context())
		if ok {
			w.Header().Set("X-Matched-Issuer", issuerCfg.IssuerURL)
		}
		w.WriteHeader(http.StatusOK)
	})

	handler, err := NewJWTHandler(t.Context(), JWTHandlerConfig{
		Issuers: []config.JWTIssuerConfig{
			{IssuerURL: "https://issuer-1.example.com", JWKCertURL: jwksServer1.URL},
			{IssuerURL: "https://issuer-2.example.com", JWKCertURL: jwksServer2.URL},
		},
	})
	Expect(err).NotTo(HaveOccurred())
	mw := handler.Middleware(nextHandler)

	validIssuerCases := []struct {
		name      string
		key       *rsa.PrivateKey
		issuerURL string
	}{
		{"token from issuer 1 matches issuer 1 config", key1, "https://issuer-1.example.com"},
		{"token from issuer 2 matches issuer 2 config", key2, "https://issuer-2.example.com"},
	}
	for _, tc := range validIssuerCases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			token := signToken(t, tc.key, jwt.MapClaims{
				"iss": tc.issuerURL,
				"exp": time.Now().Add(time.Hour).Unix(),
			})
			rr := serve(mw, "/protected", "Bearer "+token)
			Expect(rr.Code).To(Equal(http.StatusOK))
			Expect(rr.Header().Get("X-Matched-Issuer")).To(Equal(tc.issuerURL))
		})
	}

	t.Run("token from unknown issuer rejected", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, key1, jwt.MapClaims{
			"iss": "https://unknown-issuer.example.com",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		rr := serve(mw, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})

	t.Run("token signed with wrong key rejected", func(t *testing.T) {
		RegisterTestingT(t)
		wrongKey, err := rsa.GenerateKey(rand.Reader, 2048)
		Expect(err).NotTo(HaveOccurred())
		token := signToken(t, wrongKey, jwt.MapClaims{
			"iss": "https://issuer-1.example.com",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		rr := serve(mw, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})
}

func TestJWTHandler_CustomHeader(t *testing.T) {
	RegisterTestingT(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	jwksServer := newJWKSServer(t, &privateKey.PublicKey)
	defer jwksServer.Close()

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issuerCfg, ok := GetJWTIssuerConfigFromContext(r.Context())
		if ok {
			w.Header().Set("X-Matched-Issuer", issuerCfg.IssuerURL)
		}
		w.WriteHeader(http.StatusOK)
	})

	handler, err := NewJWTHandler(t.Context(), JWTHandlerConfig{
		Issuers: []config.JWTIssuerConfig{{
			IssuerURL:  "https://test-issuer.example.com",
			JWKCertURL: jwksServer.URL,
			Header:     "X-Custom-Auth",
		}},
	})
	Expect(err).NotTo(HaveOccurred())
	mw := handler.Middleware(nextHandler)

	t.Run("valid token on custom header passes", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, privateKey, jwt.MapClaims{
			"iss": "https://test-issuer.example.com",
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(),
		})
		rr := serveWithHeader(mw, "/protected", "X-Custom-Auth", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(rr.Header().Get("X-Matched-Issuer")).To(Equal("https://test-issuer.example.com"))
	})

	t.Run("token on default Authorization header ignored when issuer uses custom header", func(t *testing.T) {
		RegisterTestingT(t)
		token := signToken(t, privateKey, jwt.MapClaims{
			"iss": "https://test-issuer.example.com",
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(),
		})
		rr := serve(mw, "/protected", "Bearer "+token)
		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
	})
}

func TestJWTHandler_UnknownKIDRefresh(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name   string
		withCA bool
	}{
		{"with CA", true},
		{"without CA", false},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/new kid accepted without restart", func(t *testing.T) {
			RegisterTestingT(t)
			keyA := genRSAKey(t)
			keyB := genRSAKey(t)
			srv, h := newRotatingHandler(t, tc.withCA, map[string]*rsa.PublicKey{"kid-a": &keyA.PublicKey})
			mw := h.Middleware(okHandler())

			Expect(serveToken(mw, signTokenWithKID(t, keyA, "kid-a", issuerClaims())).Code).To(Equal(http.StatusOK))

			srv.setKeys(map[string]*rsa.PublicKey{"kid-a": &keyA.PublicKey, "kid-b": &keyB.PublicKey})
			Expect(serveToken(mw, signTokenWithKID(t, keyB, "kid-b", issuerClaims())).Code).To(Equal(http.StatusOK))
		})

		t.Run(tc.name+"/old key rejected after refresh", func(t *testing.T) {
			RegisterTestingT(t)
			keyA := genRSAKey(t)
			keyB := genRSAKey(t)
			srv, h := newRotatingHandler(t, tc.withCA, map[string]*rsa.PublicKey{"kid-a": &keyA.PublicKey})
			mw := h.Middleware(okHandler())

			Expect(serveToken(mw, signTokenWithKID(t, keyA, "kid-a", issuerClaims())).Code).To(Equal(http.StatusOK))

			srv.setKeys(map[string]*rsa.PublicKey{"kid-a": &keyA.PublicKey, "kid-b": &keyB.PublicKey})
			Expect(serveToken(mw, signTokenWithKID(t, keyB, "kid-b", issuerClaims())).Code).To(Equal(http.StatusOK))

			srv.setKeys(map[string]*rsa.PublicKey{"kid-b": &keyB.PublicKey})
			before := srv.requests.Load()
			unknownTok := signTokenWithKID(t, keyA, "kid-unknown", issuerClaims())
			Expect(serveToken(mw, unknownTok).Code).To(Equal(http.StatusUnauthorized))
			Expect(srv.requests.Load()).To(BeNumerically(">", before), "unknown kid must trigger a JWKS refresh")

			Expect(serveToken(mw, signTokenWithKID(t, keyA, "kid-a", issuerClaims())).Code).To(Equal(http.StatusUnauthorized))
			Expect(serveToken(mw, signTokenWithKID(t, keyB, "kid-b", issuerClaims())).Code).To(Equal(http.StatusOK))
		})

		t.Run(tc.name+"/unknown kid flood is rate limited", func(t *testing.T) {
			RegisterTestingT(t)
			keyA := genRSAKey(t)
			srv, h := newRotatingHandler(t, tc.withCA, map[string]*rsa.PublicKey{"kid-a": &keyA.PublicKey})
			mw := h.Middleware(okHandler())

			const requests = 50
			baseline := srv.requests.Load()
			start := time.Now()
			for i := range requests {
				kid := fmt.Sprintf("flood-%d-%d", start.UnixNano(), i)
				reqStart := time.Now()
				code := serveToken(mw, signTokenWithKID(t, keyA, kid, issuerClaims())).Code
				Expect(code).To(Equal(http.StatusUnauthorized))
				Expect(time.Since(reqStart)).To(BeNumerically("<", unknownKIDWaitMax+500*time.Millisecond))
			}

			refreshes := srv.requests.Load() - baseline
			maxRefreshes := int64(unknownKIDRefreshBurst) + int64(time.Since(start)/unknownKIDRefreshEvery) + 1
			Expect(refreshes).To(BeNumerically(">=", 1))
			Expect(refreshes).To(BeNumerically("<=", maxRefreshes))
		})

		t.Run(tc.name+"/concurrent first use of a new kid", func(t *testing.T) {
			RegisterTestingT(t)
			keyA := genRSAKey(t)
			keyB := genRSAKey(t)
			srv, h := newRotatingHandler(t, tc.withCA, map[string]*rsa.PublicKey{"kid-a": &keyA.PublicKey})
			mw := h.Middleware(okHandler())

			srv.setKeys(map[string]*rsa.PublicKey{"kid-a": &keyA.PublicKey, "kid-b": &keyB.PublicKey})
			tok := signTokenWithKID(t, keyB, "kid-b", issuerClaims())

			n := unknownKIDRefreshBurst
			codes := make([]int, n)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := range n {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					codes[i] = serveToken(mw, tok).Code
				}()
			}
			close(start)
			wg.Wait()
			for _, code := range codes {
				Expect(code).To(Equal(http.StatusOK))
			}
		})
	}
}

func TestJWTHandler_CombinedKeyfuncUnknownKIDRefresh(t *testing.T) {
	cases := []struct {
		name   string
		withCA bool
	}{
		{"with CA", true},
		{"without CA", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)

			fileKey := genRSAKey(t)
			urlKey := genRSAKey(t)
			rotatedKey := genRSAKey(t)
			srv := newMutableJWKSServer(t, tc.withCA, map[string]*rsa.PublicKey{"kid-url": &urlKey.PublicKey})
			issuerCfg := config.JWTIssuerConfig{
				IssuerURL:   "https://test-issuer.example.com",
				JWKCertURL:  srv.URL,
				JWKCertFile: writeJWKSFile(t, &fileKey.PublicKey),
			}
			if tc.withCA {
				issuerCfg.JWKCertCAFile = writeTLSCAFile(t, srv.Server)
			}

			h, err := NewJWTHandler(t.Context(), JWTHandlerConfig{Issuers: []config.JWTIssuerConfig{issuerCfg}})
			Expect(err).NotTo(HaveOccurred())
			t.Cleanup(func() {
				_ = h.Close()
				srv.Close()
			})
			mw := h.Middleware(okHandler())

			// The URL is fetched when the handler is created, not only once a token needs it.
			Expect(srv.requests.Load()).To(BeNumerically(">=", 1), "the JWKS URL must be fetched at startup")
			Expect(serveToken(mw, signTokenWithKID(t, urlKey, "kid-url", issuerClaims())).Code).To(Equal(http.StatusOK))

			// A key published later is picked up through the unknown-kid refresh.
			srv.setKeys(map[string]*rsa.PublicKey{"kid-rotated": &rotatedKey.PublicKey})
			Expect(serveToken(mw, signTokenWithKID(t, rotatedKey, "kid-rotated", issuerClaims())).Code).To(Equal(http.StatusOK))

			// The file key stays valid whatever the URL publishes.
			Expect(serveToken(mw, signToken(t, fileKey, issuerClaims())).Code).To(Equal(http.StatusOK))
		})
	}
}

// An issuer that is unreachable at startup must not stop the API from starting in combined mode, as in URL-only
// mode, and the file keys must keep working.
func TestJWTHandler_CombinedKeyfuncUnreachableURLAtStartup(t *testing.T) {
	RegisterTestingT(t)

	fileKey := genRSAKey(t)
	down := httptest.NewServer(http.NotFoundHandler())
	deadURL := down.URL
	down.Close()

	h, err := NewJWTHandler(t.Context(), JWTHandlerConfig{Issuers: []config.JWTIssuerConfig{{
		IssuerURL:   "https://test-issuer.example.com",
		JWKCertURL:  deadURL,
		JWKCertFile: writeJWKSFile(t, &fileKey.PublicKey),
	}}})
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = h.Close() })

	Expect(serveToken(h.Middleware(okHandler()), signToken(t, fileKey, issuerClaims())).Code).To(Equal(http.StatusOK))
}

// Issuers can share one header and are tried in order. A token must only ever make its own issuer look up
// keys or refresh a JWKS, never an issuer that was merely tried first.
func TestJWTHandler_IssuerScopedKeyLookup(t *testing.T) {
	RegisterTestingT(t)

	const issuerA = "https://issuer-a.example.com"
	const issuerB = "https://issuer-b.example.com"

	claimsFor := func(iss string, exp time.Time) jwt.MapClaims {
		return jwt.MapClaims{"iss": iss, "exp": exp.Unix(), "iat": time.Now().Unix()}
	}
	problemCode := func(rr *httptest.ResponseRecorder) string {
		var body map[string]any
		Expect(json.NewDecoder(rr.Body).Decode(&body)).To(Succeed())
		code, _ := body["code"].(string)
		return code
	}

	// Issuer A is listed first, so every token for issuer B is tried against A's key lookup before B's.
	setup := func(t *testing.T) (srvA, srvB *mutableJWKSServer, keyB *rsa.PrivateKey, mw http.Handler) {
		t.Helper()
		keyA := genRSAKey(t)
		keyB = genRSAKey(t)
		srvA = newMutableJWKSServer(t, true, map[string]*rsa.PublicKey{"kid-a": &keyA.PublicKey})
		srvB = newMutableJWKSServer(t, true, map[string]*rsa.PublicKey{"kid-b": &keyB.PublicKey})
		h, err := NewJWTHandler(t.Context(), JWTHandlerConfig{Issuers: []config.JWTIssuerConfig{
			{IssuerURL: issuerA, JWKCertURL: srvA.URL, JWKCertCAFile: writeTLSCAFile(t, srvA.Server)},
			{IssuerURL: issuerB, JWKCertURL: srvB.URL, JWKCertCAFile: writeTLSCAFile(t, srvB.Server)},
		}})
		Expect(err).NotTo(HaveOccurred())
		t.Cleanup(func() {
			_ = h.Close()
			srvA.Close()
			srvB.Close()
		})
		return srvA, srvB, keyB, h.Middleware(okHandler())
	}

	t.Run("valid tokens for a later issuer cause no JWKS fetch on an earlier issuer", func(t *testing.T) {
		RegisterTestingT(t)
		srvA, srvB, keyB, mw := setup(t)
		baseA, baseB := srvA.requests.Load(), srvB.requests.Load()

		for range 10 {
			tok := signTokenWithKID(t, keyB, "kid-b", claimsFor(issuerB, time.Now().Add(time.Hour)))
			Expect(serveToken(mw, tok).Code).To(Equal(http.StatusOK))
		}

		Expect(srvA.requests.Load()).To(Equal(baseA), "issuer A must not fetch its JWKS for issuer B's tokens")
		Expect(srvB.requests.Load()).To(Equal(baseB))
	})

	t.Run("a token naming an unknown issuer is rejected without any JWKS fetch", func(t *testing.T) {
		RegisterTestingT(t)
		srvA, srvB, keyB, mw := setup(t)
		baseA, baseB := srvA.requests.Load(), srvB.requests.Load()

		tok := signTokenWithKID(t, keyB, "kid-unknown", claimsFor("https://unknown.example.com", time.Now().Add(time.Hour)))
		rr := serveToken(mw, tok)

		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
		Expect(problemCode(rr)).To(Equal("HYPERFLEET-AUT-002"))
		Expect(srvA.requests.Load()).To(Equal(baseA))
		Expect(srvB.requests.Load()).To(Equal(baseB))
	})

	t.Run("a token without an issuer claim is rejected without any JWKS fetch", func(t *testing.T) {
		RegisterTestingT(t)
		srvA, srvB, keyB, mw := setup(t)
		baseA, baseB := srvA.requests.Load(), srvB.requests.Load()

		tok := signTokenWithKID(t, keyB, "kid-unknown", jwt.MapClaims{"exp": time.Now().Add(time.Hour).Unix()})
		rr := serveToken(mw, tok)

		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
		Expect(problemCode(rr)).To(Equal("HYPERFLEET-AUT-002"))
		Expect(srvA.requests.Load()).To(Equal(baseA))
		Expect(srvB.requests.Load()).To(Equal(baseB))
	})

	t.Run("an expired token for a later issuer still reports expired", func(t *testing.T) {
		RegisterTestingT(t)
		_, _, keyB, mw := setup(t)

		tok := signTokenWithKID(t, keyB, "kid-b", claimsFor(issuerB, time.Now().Add(-time.Hour)))
		rr := serveToken(mw, tok)

		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
		Expect(problemCode(rr)).To(Equal("HYPERFLEET-AUT-003"))
	})

	t.Run("an unconfigured issuer is invalid even when the token is expired", func(t *testing.T) {
		RegisterTestingT(t)
		_, _, keyB, mw := setup(t)

		tok := signTokenWithKID(t, keyB, "kid-b", claimsFor("https://unconfigured.example.com", time.Now().Add(-time.Hour)))
		rr := serveToken(mw, tok)

		Expect(rr.Code).To(Equal(http.StatusUnauthorized))
		Expect(problemCode(rr)).To(Equal("HYPERFLEET-AUT-002"))
	})

	t.Run("a new kid still refreshes only the issuer named in the token", func(t *testing.T) {
		RegisterTestingT(t)
		srvA, srvB, keyB, mw := setup(t)
		keyB2 := genRSAKey(t)
		srvB.setKeys(map[string]*rsa.PublicKey{"kid-b": &keyB.PublicKey, "kid-b2": &keyB2.PublicKey})
		baseA, baseB := srvA.requests.Load(), srvB.requests.Load()

		tok := signTokenWithKID(t, keyB2, "kid-b2", claimsFor(issuerB, time.Now().Add(time.Hour)))

		Expect(serveToken(mw, tok).Code).To(Equal(http.StatusOK))
		Expect(srvB.requests.Load()).To(BeNumerically(">", baseB), "issuer B must refresh for its own new kid")
		Expect(srvA.requests.Load()).To(Equal(baseA))
	})
}

// A burst of first requests carrying the same kid must share one JWKS fetch, also when the IdP is slow. On its own
// the JWKS library gives every request its own fetch and rejects the ones that find the refresh budget empty, even
// though the key is about to arrive.
func TestJWTHandler_ConcurrentRequestsShareOneRefresh(t *testing.T) {
	const requests = 10
	cases := []struct {
		name   string
		withCA bool
	}{
		{"with CA", true},
		{"without CA", false},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/new kid published", func(t *testing.T) {
			RegisterTestingT(t)
			keyA := genRSAKey(t)
			keyB := genRSAKey(t)
			srv, h := newRotatingHandler(t, tc.withCA, map[string]*rsa.PublicKey{"kid-a": &keyA.PublicKey})
			mw := h.Middleware(okHandler())
			srv.setKeys(map[string]*rsa.PublicKey{"kid-a": &keyA.PublicKey, "kid-b": &keyB.PublicKey})
			srv.setDelay(time.Second)
			baseline := srv.requests.Load()

			codes := serveConcurrently(mw, signTokenWithKID(t, keyB, "kid-b", issuerClaims()), requests)

			for _, code := range codes {
				Expect(code).To(Equal(http.StatusOK))
			}
			Expect(srv.requests.Load() - baseline).To(Equal(int64(1)))
		})

		t.Run(tc.name+"/unknown kid never published", func(t *testing.T) {
			RegisterTestingT(t)
			keyA := genRSAKey(t)
			srv, h := newRotatingHandler(t, tc.withCA, map[string]*rsa.PublicKey{"kid-a": &keyA.PublicKey})
			mw := h.Middleware(okHandler())
			srv.setDelay(500 * time.Millisecond)
			baseline := srv.requests.Load()

			codes := serveConcurrently(mw, signTokenWithKID(t, keyA, "kid-unknown", issuerClaims()), requests)

			for _, code := range codes {
				Expect(code).To(Equal(http.StatusUnauthorized))
			}
			Expect(srv.requests.Load() - baseline).To(Equal(int64(1)))
		})
	}
}

// Lookups are shared per kid only: a request with a known kid must neither wait for, nor take the key of, a slow
// refresh that another kid started.
func TestJWTHandler_SlowRefreshDoesNotBlockOtherKIDs(t *testing.T) {
	cases := []struct {
		name   string
		withCA bool
	}{
		{"with CA", true},
		{"without CA", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			keyA := genRSAKey(t)
			keyB := genRSAKey(t)
			srv, h := newRotatingHandler(t, tc.withCA, map[string]*rsa.PublicKey{"kid-a": &keyA.PublicKey})
			mw := h.Middleware(okHandler())
			srv.setKeys(map[string]*rsa.PublicKey{"kid-a": &keyA.PublicKey, "kid-b": &keyB.PublicKey})
			srv.setDelay(time.Second)
			baseline := srv.requests.Load()

			newKid := make(chan int, 1)
			go func() { newKid <- serveToken(mw, signTokenWithKID(t, keyB, "kid-b", issuerClaims())).Code }()
			Eventually(func() int64 { return srv.requests.Load() - baseline }).Should(BeNumerically(">=", 1),
				"the refresh for the new kid must be in flight")

			start := time.Now()
			code := serveToken(mw, signTokenWithKID(t, keyA, "kid-a", issuerClaims())).Code
			elapsed := time.Since(start)

			Expect(code).To(Equal(http.StatusOK))
			Expect(elapsed).To(BeNumerically("<", 500*time.Millisecond), "a known kid must not wait for another kid's refresh")
			Expect(<-newKid).To(Equal(http.StatusOK))
		})
	}
}

// --- helpers ---

func signToken(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	return signTokenWithKID(t, key, testKID, claims)
}

func signTokenWithKID(t *testing.T, key *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return s
}

func serve(handler http.Handler, path, authHeader string) *httptest.ResponseRecorder {
	return serveWithHeader(handler, path, "Authorization", authHeader)
}

func serveToken(handler http.Handler, token string) *httptest.ResponseRecorder {
	return serve(handler, "/protected", "Bearer "+token)
}

// serveConcurrently sends the same bearer token n times at once and returns the status codes.
func serveConcurrently(handler http.Handler, token string, n int) []int {
	codes := make([]int, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			codes[i] = serveToken(handler, token).Code
		}()
	}
	close(start)
	wg.Wait()
	return codes
}

func serveWithHeader(handler http.Handler, path, headerName, headerValue string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if headerValue != "" {
		req.Header.Set(headerName, headerValue)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func writeJWKSFile(t *testing.T, pubKey *rsa.PublicKey) string {
	t.Helper()
	jwk, err := gojwk.PublicKey(pubKey)
	if err != nil {
		t.Fatalf("failed to create JWK: %v", err)
	}
	jwk.Kid = testKID
	jwk.Alg = "RS256"
	jwkBytes, err := gojwk.Marshal(jwk)
	if err != nil {
		t.Fatalf("failed to marshal JWK: %v", err)
	}
	data := fmt.Sprintf(`{"keys":[%s]}`, string(jwkBytes))
	path := filepath.Join(t.TempDir(), "jwks.json")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatalf("failed to write JWKS file: %v", err)
	}
	return path
}

func newJWKSServer(t *testing.T, pubKey *rsa.PublicKey) *httptest.Server {
	t.Helper()
	return httptest.NewServer(jwksHandler(t, pubKey))
}

func newTLSJWKSServer(t *testing.T, pubKey *rsa.PublicKey) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(jwksHandler(t, pubKey))
}

func writeTLSCAFile(t *testing.T, tlsServer *httptest.Server) string {
	t.Helper()
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: tlsServer.TLS.Certificates[0].Certificate[0],
	})
	caPath := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(caPath, certPEM, 0600); err != nil {
		t.Fatalf("failed to write CA file: %v", err)
	}
	return caPath
}

func genRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	return key
}

func issuerClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"iss": "https://test-issuer.example.com",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// newRotatingHandler builds a JWT handler whose JWKS URL is served by a mutable server. withCA selects the
// TLS + custom CA path; otherwise the plain URL-only path is used.
func newRotatingHandler(t *testing.T, withCA bool, keys map[string]*rsa.PublicKey) (*mutableJWKSServer, *JWTHandler) {
	t.Helper()
	srv := newMutableJWKSServer(t, withCA, keys)
	issuerCfg := config.JWTIssuerConfig{
		IssuerURL:  "https://test-issuer.example.com",
		JWKCertURL: srv.URL,
	}
	if withCA {
		issuerCfg.JWKCertCAFile = writeTLSCAFile(t, srv.Server)
	}
	h, err := NewJWTHandler(t.Context(), JWTHandlerConfig{Issuers: []config.JWTIssuerConfig{issuerCfg}})
	if err != nil {
		t.Fatalf("failed to create JWT handler: %v", err)
	}
	t.Cleanup(func() {
		_ = h.Close()
		srv.Close()
	})
	return srv, h
}

// mutableJWKSServer serves the current kid -> public key set and counts every request it receives.
type mutableJWKSServer struct {
	*httptest.Server
	keys     map[string]*rsa.PublicKey
	mu       sync.Mutex
	requests atomic.Int64
	delay    atomic.Int64 // nanoseconds every response is held back
}

func newMutableJWKSServer(t *testing.T, useTLS bool, keys map[string]*rsa.PublicKey) *mutableJWKSServer {
	t.Helper()
	s := &mutableJWKSServer{keys: keys}
	if useTLS {
		s.Server = httptest.NewTLSServer(s)
	} else {
		s.Server = httptest.NewServer(s)
	}
	return s
}

func (s *mutableJWKSServer) setDelay(d time.Duration) {
	s.delay.Store(int64(d))
}

func (s *mutableJWKSServer) setKeys(keys map[string]*rsa.PublicKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys = keys
}

func (s *mutableJWKSServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.requests.Add(1)
	if d := time.Duration(s.delay.Load()); d > 0 {
		select {
		case <-time.After(d):
		case <-r.Context().Done():
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]json.RawMessage, 0, len(s.keys))
	for kid, pub := range s.keys {
		jwk, err := gojwk.PublicKey(pub)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jwk.Kid = kid
		jwk.Alg = defaultSigningAlgorithm
		jwkBytes, err := gojwk.Marshal(jwk)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		keys = append(keys, jwkBytes)
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string][]json.RawMessage{"keys": keys}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func jwksHandler(t *testing.T, pubKey *rsa.PublicKey) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	})
}
