package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	hferrors "github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

const (
	defaultSigningAlgorithm = "RS256"
	defaultLeeway           = 30 * time.Second
)

type JWTHandlerConfig struct {
	Next        http.Handler
	Issuers     []config.JWTIssuerConfig
	PublicPaths []string
}

// issuerValidator holds the pre-built keyfunc and parser for a single JWT issuer.
type issuerValidator struct {
	keyfunc   keyfunc.Keyfunc
	parser    *jwt.Parser
	header    string
	issuerCfg config.JWTIssuerConfig
}

func NewJWTHandler(ctx context.Context, cfg JWTHandlerConfig) (*JWTHandler, error) {
	if len(cfg.Issuers) == 0 {
		return nil, fmt.Errorf("at least one issuer config is required")
	}

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
			keyfunc:   kf,
			parser:    jwt.NewParser(parserOpts...),
			header:    issuer.Header,
			issuerCfg: issuer,
		})
	}

	publicPatterns := make([]*regexp.Regexp, 0, len(cfg.PublicPaths))
	for _, p := range cfg.PublicPaths {
		re, err := regexp.Compile(p)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("invalid public path pattern %q: %w", p, err)
		}
		publicPatterns = append(publicPatterns, re)
	}

	return &JWTHandler{
		validators:     validators,
		publicPatterns: publicPatterns,
		next:           cfg.Next,
		cancel:         cancel,
	}, nil
}

// JWTHandler validates JWT tokens on incoming requests. Call Close() during
// shutdown to stop the background JWKS refresh goroutine.
type JWTHandler struct {
	validators     []issuerValidator
	next           http.Handler
	cancel         context.CancelFunc
	publicPatterns []*regexp.Regexp
}

func (h *JWTHandler) Close() {
	if h.cancel != nil {
		h.cancel()
	}
}

func (h *JWTHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for _, re := range h.publicPatterns {
		if re.MatchString(r.URL.Path) {
			h.next.ServeHTTP(w, r)
			return
		}
	}

	// Try each issuer's validator: check its header, extract Bearer token, validate
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

		token, err := v.parser.Parse(parts[1], v.keyfunc.Keyfunc)
		if err != nil {
			if !errors.Is(lastErr, jwt.ErrTokenExpired) {
				lastErr = err
			}
			continue
		}

		ctx := SetJWTTokenContext(r.Context(), token)
		ctx = SetJWTIssuerConfigContext(ctx, v.issuerCfg)
		h.next.ServeHTTP(w, r.WithContext(ctx))
		return
	}

	// No validator matched — return the most appropriate error
	if lastErr != nil {
		logger.WithError(r.Context(), lastErr).Warn("JWT validation failed")
		if errors.Is(lastErr, jwt.ErrTokenExpired) {
			handleError(r.Context(), w, r, hferrors.CodeAuthExpiredToken, "JWT token has expired")
		} else {
			handleError(r.Context(), w, r, hferrors.CodeAuthInvalidCredentials, "invalid JWT token")
		}
		return
	}

	if sawNonBearer {
		handleError(r.Context(), w, r, hferrors.CodeAuthInvalidCredentials, "authorization header does not use Bearer scheme")
		return
	}

	handleError(r.Context(), w, r, hferrors.CodeAuthNoCredentials, "missing authorization header")
}

func buildKeyfunc(ctx context.Context, issuer config.JWTIssuerConfig) (keyfunc.Keyfunc, error) {
	hasFile := issuer.JWKCertFile != ""
	hasURL := issuer.JWKCertURL != ""

	if !hasFile && !hasURL {
		return nil, fmt.Errorf("at least one of jwk_cert_file or jwk_cert_url must be provided")
	}

	if hasFile && !hasURL {
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

	if !hasFile && hasURL {
		kf, err := keyfunc.NewDefaultCtx(ctx, []string{issuer.JWKCertURL})
		if err != nil {
			return nil, fmt.Errorf("failed to create JWKS client from URL %q: %w", issuer.JWKCertURL, err)
		}
		return kf, nil
	}

	data, err := os.ReadFile(issuer.JWKCertFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read JWKS file %q: %w", issuer.JWKCertFile, err)
	}
	fileKF, err := keyfunc.NewJWKSetJSON(json.RawMessage(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWKS file: %w", err)
	}

	httpStorage, err := jwkset.NewHTTPClient(jwkset.HTTPClientOptions{
		Given: fileKF.Storage(),
		HTTPURLs: map[string]jwkset.Storage{
			issuer.JWKCertURL: jwkset.NewMemoryStorage(),
		},
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
