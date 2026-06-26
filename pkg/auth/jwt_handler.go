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

	hferrors "github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

const (
	defaultSigningAlgorithm = "RS256"
	defaultLeeway           = 30 * time.Second
)

// JWTHandlerConfig defines the JWT handler's overall configuration.
// A request is accepted if its token validates against any entry in Issuers.
type JWTHandlerConfig struct {
	Next        http.Handler
	PublicPaths []string
	Issuers     []JWTIssuerHandlerConfig
}

// JWTIssuerHandlerConfig is the per-issuer configuration for the JWT handler.
type JWTIssuerHandlerConfig struct {
	IssuerURL            string
	Audience             string
	KeysFile             string
	KeysURL              string
	IdentityClaim        string
	IdentityClaimPattern string
	Header               string
}

// compiledIssuer holds the pre-built state for a single issuer.
type compiledIssuer struct {
	keyfunc         keyfunc.Keyfunc
	compiledPattern *regexp.Regexp
	parser          *jwt.Parser
	identityCfg     CallerIdentityConfig
}

// JWTHandler validates JWT tokens against one or more issuers. Call Close() during
// shutdown to stop background JWKS refresh goroutines.
type JWTHandler struct {
	issuers        []compiledIssuer
	cancels        []context.CancelFunc
	next           http.Handler
	publicPatterns []*regexp.Regexp
}

func NewJWTHandler(ctx context.Context, cfg JWTHandlerConfig) (*JWTHandler, error) {
	if len(cfg.Issuers) == 0 {
		return nil, fmt.Errorf("at least one issuer must be configured")
	}

	publicPatterns := make([]*regexp.Regexp, 0, len(cfg.PublicPaths))
	for _, p := range cfg.PublicPaths {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("invalid public path pattern %q: %w", p, err)
		}
		publicPatterns = append(publicPatterns, re)
	}

	issuers := make([]compiledIssuer, 0, len(cfg.Issuers))
	cancels := make([]context.CancelFunc, 0, len(cfg.Issuers))
	for i, ic := range cfg.Issuers {
		issuerCtx, cancel := context.WithCancel(ctx)
		ci, err := buildCompiledIssuer(issuerCtx, ic)
		if err != nil {
			cancel()
			for _, c := range cancels {
				c()
			}
			return nil, fmt.Errorf("issuer[%d]: %w", i, err)
		}
		issuers = append(issuers, ci)
		cancels = append(cancels, cancel)
	}

	return &JWTHandler{
		issuers:        issuers,
		cancels:        cancels,
		publicPatterns: publicPatterns,
		next:           cfg.Next,
	}, nil
}

func buildCompiledIssuer(ctx context.Context, ic JWTIssuerHandlerConfig) (compiledIssuer, error) {
	kf, err := buildKeyfunc(ctx, ic)
	if err != nil {
		return compiledIssuer{}, fmt.Errorf("failed to build keyfunc: %w", err)
	}

	parserOpts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{defaultSigningAlgorithm}),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(defaultLeeway),
	}
	if ic.IssuerURL != "" {
		parserOpts = append(parserOpts, jwt.WithIssuer(ic.IssuerURL))
	} else {
		logger.Warn(ctx, "JWT issuer validation disabled: no issuer_url configured")
	}
	if ic.Audience != "" {
		parserOpts = append(parserOpts, jwt.WithAudience(ic.Audience))
	}

	var compiledPattern *regexp.Regexp
	if ic.IdentityClaimPattern != "" {
		compiledPattern, err = regexp.Compile(ic.IdentityClaimPattern)
		if err != nil {
			return compiledIssuer{}, fmt.Errorf("identity_claim_pattern %q is not a valid regex: %w",
				ic.IdentityClaimPattern, err)
		}
	}

	return compiledIssuer{
		parser:          jwt.NewParser(parserOpts...),
		keyfunc:         kf,
		compiledPattern: compiledPattern,
		identityCfg: CallerIdentityConfig{
			JWTIdentityClaim:     ic.IdentityClaim,
			IdentityClaimPattern: ic.IdentityClaimPattern,
			HeaderName:           ic.Header,
		},
	}, nil
}

func (h *JWTHandler) Close() {
	for _, cancel := range h.cancels {
		if cancel != nil {
			cancel()
		}
	}
}

func (h *JWTHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for _, re := range h.publicPatterns {
		if re.MatchString(r.URL.Path) {
			h.next.ServeHTTP(w, r)
			return
		}
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		handleError(r.Context(), w, r, hferrors.CodeAuthNoCredentials, "missing Authorization header")
		return
	}

	parts := strings.Fields(authHeader)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		handleError(r.Context(), w, r, hferrors.CodeAuthInvalidCredentials, "Authorization header must use Bearer scheme")
		return
	}
	tokenString := parts[1]

	var lastErr error
	for _, issuer := range h.issuers {
		token, err := issuer.parser.Parse(tokenString, issuer.keyfunc.Keyfunc)
		if err != nil {
			lastErr = err
			continue
		}
		ctx := SetJWTTokenContext(r.Context(), token)
		ctx = SetMatchedIdentityConfig(ctx, issuer.identityCfg, issuer.compiledPattern)
		h.next.ServeHTTP(w, r.WithContext(ctx))
		return
	}

	logger.WithError(r.Context(), lastErr).Warn("JWT validation failed against all configured issuers")
	if errors.Is(lastErr, jwt.ErrTokenExpired) {
		handleError(r.Context(), w, r, hferrors.CodeAuthExpiredToken, "JWT token has expired")
	} else {
		handleError(r.Context(), w, r, hferrors.CodeAuthInvalidCredentials, "invalid or expired JWT token")
	}
}

func buildKeyfunc(ctx context.Context, ic JWTIssuerHandlerConfig) (keyfunc.Keyfunc, error) {
	hasFile := ic.KeysFile != ""
	hasURL := ic.KeysURL != ""

	if !hasFile && !hasURL {
		return nil, fmt.Errorf("at least one of KeysFile or KeysURL must be provided")
	}

	if hasFile && !hasURL {
		data, err := os.ReadFile(ic.KeysFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read JWKS file %q: %w", ic.KeysFile, err)
		}
		kf, err := keyfunc.NewJWKSetJSON(json.RawMessage(data))
		if err != nil {
			return nil, fmt.Errorf("failed to parse JWKS file %q: %w", ic.KeysFile, err)
		}
		return kf, nil
	}

	if !hasFile && hasURL {
		kf, err := keyfunc.NewDefaultCtx(ctx, []string{ic.KeysURL})
		if err != nil {
			return nil, fmt.Errorf("failed to create JWKS client from URL %q: %w", ic.KeysURL, err)
		}
		return kf, nil
	}

	data, err := os.ReadFile(ic.KeysFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read JWKS file %q: %w", ic.KeysFile, err)
	}
	fileKF, err := keyfunc.NewJWKSetJSON(json.RawMessage(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWKS file: %w", err)
	}

	httpStorage, err := jwkset.NewHTTPClient(jwkset.HTTPClientOptions{
		Given: fileKF.Storage(),
		HTTPURLs: map[string]jwkset.Storage{
			ic.KeysURL: jwkset.NewMemoryStorage(),
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
