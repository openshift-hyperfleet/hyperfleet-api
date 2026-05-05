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

type JWTHandlerConfig struct {
	Next        http.Handler
	KeysFile    string
	KeysURL     string
	IssuerURL   string
	Audience    string
	PublicPaths []string
}

func NewJWTHandler(ctx context.Context, cfg JWTHandlerConfig) (http.Handler, error) {
	kf, err := buildKeyfunc(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build JWKS keyfunc: %w", err)
	}

	publicPatterns := make([]*regexp.Regexp, 0, len(cfg.PublicPaths))
	for _, p := range cfg.PublicPaths {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("invalid public path pattern %q: %w", p, err)
		}
		publicPatterns = append(publicPatterns, re)
	}

	parserOpts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{defaultSigningAlgorithm}),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(defaultLeeway),
	}
	if cfg.IssuerURL != "" {
		parserOpts = append(parserOpts, jwt.WithIssuer(cfg.IssuerURL))
	} else {
		logger.Warn(ctx, "JWT issuer validation disabled: no issuer_url configured")
	}
	if cfg.Audience != "" {
		parserOpts = append(parserOpts, jwt.WithAudience(cfg.Audience))
	}

	return &jwtHandler{
		keyfunc:        kf,
		parser:         jwt.NewParser(parserOpts...),
		publicPatterns: publicPatterns,
		next:           cfg.Next,
	}, nil
}

type jwtHandler struct {
	keyfunc        keyfunc.Keyfunc
	next           http.Handler
	parser         *jwt.Parser
	publicPatterns []*regexp.Regexp
}

func (h *jwtHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	tokenString, found := strings.CutPrefix(authHeader, "Bearer ")
	if !found {
		handleError(r.Context(), w, r, hferrors.CodeAuthInvalidCredentials, "Authorization header must use Bearer scheme")
		return
	}

	token, err := h.parser.Parse(tokenString, h.keyfunc.Keyfunc)
	if err != nil {
		logger.WithError(r.Context(), err).Warn("JWT validation failed")
		if errors.Is(err, jwt.ErrTokenExpired) {
			handleError(r.Context(), w, r, hferrors.CodeAuthExpiredToken, "JWT token has expired")
		} else {
			handleError(r.Context(), w, r, hferrors.CodeAuthInvalidCredentials, "invalid or expired JWT token")
		}
		return
	}

	ctx := SetJWTTokenContext(r.Context(), token)
	h.next.ServeHTTP(w, r.WithContext(ctx))
}

func buildKeyfunc(ctx context.Context, cfg JWTHandlerConfig) (keyfunc.Keyfunc, error) {
	hasFile := cfg.KeysFile != ""
	hasURL := cfg.KeysURL != ""

	if !hasFile && !hasURL {
		return nil, fmt.Errorf("at least one of KeysFile or KeysURL must be provided")
	}

	if hasFile && !hasURL {
		data, err := os.ReadFile(cfg.KeysFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read JWKS file %q: %w", cfg.KeysFile, err)
		}
		return keyfunc.NewJWKSetJSON(json.RawMessage(data))
	}

	if !hasFile && hasURL {
		return keyfunc.NewDefaultCtx(ctx, []string{cfg.KeysURL})
	}

	data, err := os.ReadFile(cfg.KeysFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read JWKS file %q: %w", cfg.KeysFile, err)
	}
	fileKF, err := keyfunc.NewJWKSetJSON(json.RawMessage(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWKS file: %w", err)
	}

	httpStorage, err := jwkset.NewHTTPClient(jwkset.HTTPClientOptions{
		Given: fileKF.Storage(),
		HTTPURLs: map[string]jwkset.Storage{
			cfg.KeysURL: jwkset.NewMemoryStorage(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP JWKS client: %w", err)
	}

	return keyfunc.New(keyfunc.Options{
		Ctx:     ctx,
		Storage: httpStorage,
	})
}
