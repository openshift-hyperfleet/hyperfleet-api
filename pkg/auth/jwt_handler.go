package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/time/rate"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	hferrors "github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

const (
	defaultSigningAlgorithm  = "RS256"
	defaultLeeway            = 30 * time.Second
	defaultHTTPClientTimeout = 30 * time.Second

	// jwksRefreshInterval is how often each JWKS is re-fetched
	jwksRefreshInterval = 5 * time.Minute
	// unknownKIDRefreshEvery and unknownKIDRefreshBurst rate-limit JWKS refreshes triggered by an unknown kid.
	unknownKIDRefreshEvery = 30 * time.Second
	unknownKIDRefreshBurst = 3
	// unknownKIDWaitMax is the time limit of an unknown-kid refresh. It covers both the wait for a refresh slot and
	// the refresh request itself.
	unknownKIDWaitMax = 2 * time.Second
)

type JWTHandlerConfig struct {
	Issuers []config.JWTIssuerConfig
}

// issuerValidator holds the pre-built key lookup and parser for a single JWT issuer.
type issuerValidator struct {
	keyFn     jwt.Keyfunc
	parser    *jwt.Parser
	header    string
	issuerCfg config.JWTIssuerConfig
}

// issuerKeyFunc refuses to look up a key for a token that names a different issuer. Issuers can share one
// header and are tried in order, so without this a valid token for one issuer would make every issuer tried
// before it fetch or refresh its JWKS. The claims are already decoded when the key lookup runs; the token's
// issuer is compared exactly, as jwt.WithIssuer does. The error names neither issuer: it is logged, and an issuer
// URL can embed credentials.
func issuerKeyFunc(issuerURL string, next jwt.Keyfunc) jwt.Keyfunc {
	return func(token *jwt.Token) (any, error) {
		iss, err := token.Claims.GetIssuer()
		if err != nil || iss != issuerURL {
			return nil, fmt.Errorf("%w: token issuer is not the one this validator accepts", jwt.ErrTokenInvalidIssuer)
		}
		return next(token)
	}
}

func NewJWTHandler(ctx context.Context, cfg JWTHandlerConfig) (*JWTHandler, error) {
	if len(cfg.Issuers) == 0 {
		return nil, fmt.Errorf("at least one issuer config is required")
	}

	// Enabled must be true so Validate() runs its checks; this handler is only created when JWT is already enabled.
	jwtCfg := config.JWTConfig{Enabled: true, Configs: cfg.Issuers}
	jwtCfg.ApplyDefaults()
	if err := jwtCfg.Validate(); err != nil {
		return nil, fmt.Errorf("issuer config validation failed: %w", err)
	}
	cfg.Issuers = jwtCfg.Configs

	ctx, cancel := context.WithCancel(ctx)

	validators := make([]issuerValidator, 0, len(cfg.Issuers))
	for i, issuer := range cfg.Issuers {
		kf, err := buildKeyfunc(ctx, issuer)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to build JWKS keyfunc for issuer %d (%s): %w", i, issuer.IssuerURL, err)
		}

		parserOpts := []jwt.ParserOption{
			jwt.WithValidMethods([]string{defaultSigningAlgorithm}),
			jwt.WithExpirationRequired(),
			jwt.WithLeeway(defaultLeeway),
			jwt.WithIssuer(issuer.IssuerURL),
		}
		if issuer.Audience != "" {
			parserOpts = append(parserOpts, jwt.WithAudience(issuer.Audience))
		}

		validators = append(validators, issuerValidator{
			keyFn:     issuerKeyFunc(issuer.IssuerURL, kf.Keyfunc),
			parser:    jwt.NewParser(parserOpts...),
			header:    issuer.Header,
			issuerCfg: issuer,
		})
	}

	return &JWTHandler{
		validators: validators,
		cancel:     cancel,
	}, nil
}

// JWTHandler validates JWT tokens on incoming requests. Call Close() during
// shutdown to stop the background JWKS refresh goroutine.
type JWTHandler struct {
	cancel     context.CancelFunc
	validators []issuerValidator
}

func (h *JWTHandler) Close() error {
	if h.cancel != nil {
		h.cancel()
	}
	return nil
}

// matchValidator tries each configured issuer validator against the request headers.
// It returns the parsed token and matching issuer config on success. On failure it
// returns a nil token, whether a non-Bearer scheme was seen, and the most relevant
// error (expired tokens take precedence over other parse errors). A nil error with
// a nil token means no configured issuer header was present at all.
func (h *JWTHandler) matchValidator(r *http.Request) (*jwt.Token, config.JWTIssuerConfig, bool, error) {
	var lastErr error
	sawNonBearer := false
	for _, v := range h.validators {
		headerVal := r.Header.Get(v.header)
		if headerVal == "" {
			continue
		}

		parts := strings.Fields(headerVal)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			sawNonBearer = true
			continue
		}

		token, err := v.parser.Parse(parts[1], v.keyFn)
		if err != nil {
			if !errors.Is(lastErr, jwt.ErrTokenExpired) {
				lastErr = err
			}
			continue
		}

		return token, v.issuerCfg, false, nil
	}
	return nil, config.JWTIssuerConfig{}, sawNonBearer, lastErr
}

// Middleware returns a standard HTTP middleware that validates JWT tokens.
// Requests with a valid token proceed to next with claims in context;
// requests without valid credentials receive a 401 response.
func (h *JWTHandler) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, issuerCfg, sawNonBearer, lastErr := h.matchValidator(r)
		if token != nil {
			ctx := SetJWTTokenContext(r.Context(), token)
			ctx = SetJWTIssuerConfigContext(ctx, issuerCfg)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// No validator matched - return the most appropriate error
		if lastErr != nil {
			slog.WarnContext(r.Context(), "JWT validation failed", "error", lastErr)
			if errors.Is(lastErr, jwt.ErrTokenExpired) {
				handleError(r.Context(), w, r, hferrors.CodeAuthExpiredToken, "JWT token has expired")
			} else {
				handleError(r.Context(), w, r, hferrors.CodeAuthInvalidCredentials, "invalid JWT token")
			}
			return
		}

		if sawNonBearer {
			handleError(r.Context(), w, r,
				hferrors.CodeAuthInvalidCredentials, "authorization header does not use Bearer scheme")
			return
		}

		handleError(r.Context(), w, r, hferrors.CodeAuthNoCredentials, "missing authorization header")
	})
}

func buildKeyfunc(ctx context.Context, issuer config.JWTIssuerConfig) (keyfunc.Keyfunc, error) {
	hasFile := issuer.JWKCertFile != ""
	hasURL := issuer.JWKCertURL != ""

	if !hasFile && !hasURL {
		return nil, fmt.Errorf("at least one of jwk_cert_file or jwk_cert_url must be provided")
	}
	if hasFile && !hasURL {
		return buildFileOnlyKeyfunc(issuer)
	}
	if !hasFile {
		return buildURLOnlyKeyfunc(ctx, issuer)
	}
	return buildCombinedKeyfunc(ctx, issuer)
}

func buildFileOnlyKeyfunc(issuer config.JWTIssuerConfig) (keyfunc.Keyfunc, error) {
	data, err := os.ReadFile(issuer.JWKCertFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read JWKS file %q: %w", issuer.JWKCertFile, err)
	}
	kf, err := keyfunc.NewJWKSetJSON(json.RawMessage(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWKS file %q: %w", issuer.JWKCertFile, err)
	}
	return kf, nil
}

// newUnknownKIDLimiter returns a fresh limiter so that issuers never share a refresh budget.
func newUnknownKIDLimiter() *rate.Limiter {
	return rate.NewLimiter(rate.Every(unknownKIDRefreshEvery), unknownKIDRefreshBurst)
}

func buildURLOnlyKeyfunc(ctx context.Context, issuer config.JWTIssuerConfig) (keyfunc.Keyfunc, error) {
	if issuer.JWKCertCAFile == "" {
		kf, err := keyfunc.NewDefaultOverrideCtx(ctx, []string{issuer.JWKCertURL}, keyfunc.Override{
			RefreshInterval:   jwksRefreshInterval,
			RefreshUnknownKID: newUnknownKIDLimiter(),
			RateLimitWaitMax:  unknownKIDWaitMax,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create JWKS client from URL %q: %w", issuer.JWKCertURL, err)
		}
		return kf, nil
	}

	urlStorage, err := newStorageWithCA(ctx, issuer.JWKCertURL, issuer.JWKCertCAFile)
	if err != nil {
		return nil, err
	}
	httpStorage, err := jwkset.NewHTTPClient(jwkset.HTTPClientOptions{
		HTTPURLs: map[string]jwkset.Storage{
			issuer.JWKCertURL: urlStorage,
		},
		RateLimitWaitMax:  unknownKIDWaitMax,
		RefreshUnknownKID: newUnknownKIDLimiter(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP JWKS client: %w", err)
	}
	kf, err := keyfunc.New(keyfunc.Options{
		Ctx:     ctx,
		Storage: httpStorage,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create JWKS keyfunc from URL %q with CA: %w", issuer.JWKCertURL, err)
	}
	return kf, nil
}

// buildCombinedKeyfunc serves keys from both the JWKS file and the JWKS URL. The file keys are checked first and are
// never replaced by a refresh; the URL keys follow the same refresh rules as a URL-only issuer.
func buildCombinedKeyfunc(ctx context.Context, issuer config.JWTIssuerConfig) (keyfunc.Keyfunc, error) {
	data, err := os.ReadFile(issuer.JWKCertFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read JWKS file %q: %w", issuer.JWKCertFile, err)
	}
	fileKF, err := keyfunc.NewJWKSetJSON(json.RawMessage(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWKS file: %w", err)
	}

	// The URL needs an HTTP-backed store with or without a CA file. A plain in-memory store would never fetch it.
	var urlStorage jwkset.Storage
	if issuer.JWKCertCAFile != "" {
		urlStorage, err = newStorageWithCA(ctx, issuer.JWKCertURL, issuer.JWKCertCAFile)
		if err != nil {
			return nil, err
		}
	} else {
		urlStorage, err = newHTTPStorage(ctx, issuer.JWKCertURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create JWKS storage from URL %q: %w", issuer.JWKCertURL, err)
		}
	}

	httpStorage, err := jwkset.NewHTTPClient(jwkset.HTTPClientOptions{
		Given: fileKF.Storage(),
		HTTPURLs: map[string]jwkset.Storage{
			issuer.JWKCertURL: urlStorage,
		},
		RateLimitWaitMax:  unknownKIDWaitMax,
		RefreshUnknownKID: newUnknownKIDLimiter(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP JWKS client: %w", err)
	}

	kf, err := keyfunc.New(keyfunc.Options{
		Ctx:     ctx,
		Storage: httpStorage,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create combined JWKS keyfunc: %w", err)
	}
	return kf, nil
}

// newStorageWithCA builds the HTTP-backed key store for a JWKS URL whose certificate is issued by the CA in caFile.
func newStorageWithCA(ctx context.Context, jwkURL, caFile string) (jwkset.Storage, error) {
	httpClient, err := buildHTTPClientWithCA(caFile)
	if err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "JWKS client configured with custom CA", "url", jwkURL, "ca_file", caFile)
	storage, err := newHTTPStorage(ctx, jwkURL, httpClient, "ca_file", caFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create JWKS storage from URL %q with CA %q: %w", jwkURL, caFile, err)
	}
	return storage, nil
}

// newHTTPStorage builds the key store for one JWKS URL. It fetches the JWKS at creation, tolerating a failed first
// fetch so an unreachable issuer does not stop the API from starting, then refreshes it in the background. A nil
// client uses the library default. logAttrs are added to the log line written when a refresh fails.
func newHTTPStorage(ctx context.Context, jwkURL string, client *http.Client, logAttrs ...any) (jwkset.Storage, error) {
	return jwkset.NewStorageFromHTTP(jwkURL, jwkset.HTTPClientStorageOptions{
		Client:                    client,
		Ctx:                       ctx,
		NoErrorReturnFirstHTTPReq: true,
		RefreshErrorHandler: func(ctx context.Context, err error) {
			attrs := append([]any{"url", jwkURL, "error", err}, logAttrs...)
			slog.ErrorContext(ctx, "failed to refresh JWKS from URL", attrs...)
		},
		RefreshInterval: jwksRefreshInterval,
	})
}

func buildHTTPClientWithCA(caFile string) (*http.Client, error) {
	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA file %q: %w", caFile, err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate from %q", caFile)
	}
	return &http.Client{
		Timeout: defaultHTTPClientTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    pool,
				MinVersion: tls.VersionTLS12,
			},
		},
	}, nil
}
