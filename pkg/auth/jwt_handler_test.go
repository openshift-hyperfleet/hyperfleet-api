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
	return serveWithHeader(handler, path, "Authorization", authHeader)
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
