package test

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"gorm.io/gorm"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/container"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/servecmd"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/server"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/auth"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/closer"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/test/factories"
	"github.com/openshift-hyperfleet/hyperfleet-api/test/mocks"
)

const (
	jwkKID = "uhctestkey"
	jwkAlg = "RS256"
)

var (
	helper *Helper
	once   sync.Once
)

const defaultTestIdentityHeader = "X-HyperFleet-Identity"

func integrationTestConfigPath() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("test setup: runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(thisFile), "testdata", "integration-config.yaml")
}

func defaultTestEntities() []registry.EntityDescriptor {
	return []registry.EntityDescriptor{
		{
			Kind:              "Cluster",
			Plural:            "clusters",
			SpecSchemaName:    "ClusterSpec",
			NameMinLen:        3,
			NameMaxLen:        53,
			RequireSpecSchema: true,
			RequiredAdapters:  []string{"validation", "dns", "pullsecret", "hypershift"},
		},
		{
			Kind:              "NodePool",
			Plural:            "nodepools",
			ParentKind:        "Cluster",
			OnParentDelete:    registry.OnParentDeleteCascade,
			SpecSchemaName:    "NodePoolSpec",
			NameMinLen:        3,
			NameMaxLen:        15,
			RequireSpecSchema: true,
			RequiredAdapters:  []string{"validation", "hypershift"},
		},
		{
			Kind:           "Channel",
			Plural:         "channels",
			SpecSchemaName: "ChannelSpec",
		},
		{
			Kind:           "Version",
			Plural:         "versions",
			ParentKind:     "Channel",
			OnParentDelete: registry.OnParentDeleteRestrict,
			SpecSchemaName: "VersionSpec",
		},
		{
			Kind:           "WifConfig",
			Plural:         "wifconfigs",
			SpecSchemaName: "WifConfigSpec",
		},
	}
}

type Helper struct {
	Factories     factories.Factories
	DBFactory     db.SessionFactory
	Container     *container.Container
	APIServer     *server.APIServer
	AppConfig     *config.ApplicationConfig
	JWTPrivateKey *rsa.PrivateKey
	JWTCA         *rsa.PublicKey
	jwtHandler    *auth.JWTHandler
	closer        *closer.Closer
	tables        []string
}

func NewHelper() *Helper {
	once.Do(func() {
		initTestLogger()
		ctx := context.Background()

		jwtKey, jwtCA, err := parseJWTKeys()
		if err != nil {
			panic(fmt.Sprintf("test setup: unable to load JWT keys: %v", err))
		}

		// Use an explicit --config flag, not HYPERFLEET_CONFIG, so we never hijack a developer's own env var.
		cmd := &cobra.Command{}
		cmd.Flags().String("config", "", "config file path")
		cmd.Flags().Set("config", integrationTestConfigPath()) //nolint:errcheck // string flag, Set never errors

		loader := config.NewConfigLoader()
		cfg, err := loader.Load(ctx, cmd)
		if err != nil {
			panic(fmt.Sprintf("test setup: load config: %v", err))
		}

		if logLevel := os.Getenv("LOGLEVEL"); logLevel != "" {
			logger.With(ctx, logger.FieldLogLevel, logLevel).Info("Using custom loglevel")
			cfg.Logging.Level = logLevel
		}

		if len(cfg.Entities) == 0 {
			cfg.Entities = defaultTestEntities()
		}
		registry.LoadDescriptors(cfg.Entities)
		registry.Validate()

		c := closer.New()
		ctr := container.NewContainer(cfg, c)

		if err = db.Migrate(ctr.SessionFactory().New(ctx)); err != nil {
			abortSetup(ctx, c, err, "migration failed")
		}

		jwkURL, jwkTeardown := mocks.NewJWKCertServerMock(jwtCA, jwkKID, jwkAlg)
		c.Add(jwkTeardown)
		if len(cfg.Server.JWT.Configs) == 0 {
			abortSetup(ctx, c, nil, "integration-config.yaml must define at least one JWT issuer")
		}
		cfg.Server.JWT.Configs[0].JWKCertURL = jwkURL

		helper = &Helper{
			Factories:     factories.New(ctr.ResourceService()),
			AppConfig:     cfg,
			DBFactory:     ctr.SessionFactory(),
			Container:     ctr,
			JWTPrivateKey: jwtKey,
			JWTCA:         jwtCA,
			closer:        c,
		}

		tables, err := helper.getAllTables(ctr.SessionFactory().New(ctx))
		if err != nil {
			abortSetup(ctx, c, err, "discover tables for truncation")
		}
		helper.tables = tables

		helper.startAPIServer()
	})
	return helper
}

func (helper *Helper) Teardown() {
	if err := helper.closer.Close(); err != nil {
		logger.WithError(context.Background(), err).Error("teardown errors")
	}
}

func (helper *Helper) requireJWTIssuers() {
	if helper.AppConfig.Server.JWT.Enabled && len(helper.AppConfig.Server.JWT.Configs) == 0 {
		abortSetup(context.Background(), helper.closer, nil, "JWT enabled but no issuer configs defined")
	}
}

// abortSetup logs msg, cleans up already-created resources via c, then panics - for unrecoverable NewHelper failures.
func abortSetup(ctx context.Context, c *closer.Closer, err error, msg string) {
	if err != nil {
		logger.WithError(ctx, err).Error(msg)
	} else {
		logger.Error(ctx, msg)
	}
	_ = c.Close()
	panic(fmt.Sprintf("test setup: %s", msg))
}

func (helper *Helper) startAPIServer() {
	ctx := context.Background()
	helper.requireJWTIssuers()
	if len(helper.AppConfig.Server.JWT.Configs) > 0 {
		if helper.AppConfig.Server.JWT.Configs[0].IdentityHeader == "" {
			helper.AppConfig.Server.JWT.Configs[0].IdentityHeader = defaultTestIdentityHeader
		}
	}
	cfg := helper.AppConfig

	jwtHandler := helper.Container.JWTHandler()
	helper.jwtHandler = jwtHandler

	apiServer, err := servecmd.BuildAPIServer(
		cfg,
		helper.Container.ResourceService(),
		helper.Container.AdapterStatusService(),
		helper.Container.SchemaValidator(),
		jwtHandler,
		helper.DBFactory,
	)
	if err != nil {
		abortSetup(ctx, helper.closer, err, "Unable to build Test API server")
	}
	helper.APIServer = apiServer
	// No graceful drain here: nothing depends on it, the test binary exits right after Teardown.
	helper.closer.Add(helper.APIServer.Close)

	listener, err := helper.APIServer.Listen()
	if err != nil {
		abortSetup(ctx, helper.closer, err, "Unable to start Test API server")
	}
	go func() {
		logger.Debug(ctx, "Test API server started")
		if err := helper.APIServer.Serve(listener); err != nil {
			logger.WithError(ctx, err).Error("Test API server terminated with errors")
		}
		logger.Debug(ctx, "Test API server stopped")
	}()
}

// NewID creates a new unique ID used internally
func (helper *Helper) NewID() string {
	id, err := uuid.NewV7()
	if err != nil {
		panic(fmt.Sprintf("test helper: failed to generate UUID v7: %v", err))
	}
	return id.String()
}

func (helper *Helper) baseURL() string {
	scheme := "http"
	if helper.AppConfig.Server.TLS.Enabled {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, helper.AppConfig.Server.BindAddress())
}

func (helper *Helper) RestURL(path string) string {
	return helper.baseURL() + "/api/hyperfleet/v1" + path
}

func (helper *Helper) NewAPIClient() *openapi.ClientWithResponses {
	client, err := openapi.NewClientWithResponses(helper.baseURL())
	if err != nil {
		panic(fmt.Sprintf("test setup: failed to create API client: %v", err))
	}
	return client
}

type TestAccount struct {
	Username  string
	FirstName string
	LastName  string
	Email     string
}

func (helper *Helper) NewRandAccount() *TestAccount {
	return helper.NewAccount(helper.NewID(), gofakeit.Name(), gofakeit.Email())
}

func (helper *Helper) NewAccount(username, name, email string) *TestAccount {
	var firstName, lastName string
	names := strings.SplitN(name, " ", 2)
	if len(names) < 2 {
		firstName = name
	} else {
		firstName = names[0]
		lastName = names[1]
	}

	return &TestAccount{
		Username:  username,
		FirstName: firstName,
		LastName:  lastName,
		Email:     email,
	}
}

type contextKeyAccessToken struct{}

// ContextAccessToken is the context key for access tokens (used by tests)
var ContextAccessToken = contextKeyAccessToken{}

func (helper *Helper) NewAuthenticatedContext(account *TestAccount) context.Context {
	tokenString := helper.CreateJWTString(account)
	return context.WithValue(context.Background(), ContextAccessToken, tokenString)
}

// GetAccessTokenFromContext extracts the access token from the context
func GetAccessTokenFromContext(ctx context.Context) string {
	if token, ok := ctx.Value(ContextAccessToken).(string); ok {
		return token
	}
	return ""
}

// WithAuthToken returns a RequestEditorFn that adds the Authorization header from context
func WithAuthToken(ctx context.Context) openapi.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		token := GetAccessTokenFromContext(ctx)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		return nil
	}
}

// WithIdentityHeader returns a RequestEditorFn that sets the caller identity header.
func WithIdentityHeader(headerName, headerValue string) openapi.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		if headerName != "" && headerValue != "" {
			req.Header.Set(headerName, headerValue)
		}
		return nil
	}
}

// IdentityHeaderName returns the configured identity header name from the first JWT issuer config.
func (helper *Helper) IdentityHeaderName() string {
	if helper == nil || helper.AppConfig == nil {
		return ""
	}
	configs := helper.AppConfig.Server.JWT.Configs
	if len(configs) > 0 {
		return configs[0].IdentityHeader
	}
	return ""
}

func (helper *Helper) ResetDB() error {
	if len(helper.tables) == 0 {
		return nil
	}
	g2 := helper.DBFactory.New(context.Background())
	if err := g2.Exec(fmt.Sprintf("TRUNCATE TABLE %s CASCADE", strings.Join(helper.tables, ", "))).Error; err != nil {
		return fmt.Errorf("truncate business tables: %w", err)
	}
	return nil
}

func (helper *Helper) MigrateDB() error {
	return db.Migrate(helper.DBFactory.New(context.Background()))
}

func (helper *Helper) CleanDB() error {
	g2 := helper.DBFactory.New(context.Background())

	tables, err := helper.getAllTables(g2)
	if err != nil {
		return fmt.Errorf("error discovering tables: %w", err)
	}

	orderedTables, err := helper.orderTablesByDependencies(g2, tables)
	if err != nil {
		return fmt.Errorf("error ordering tables by dependencies: %w", err)
	}

	for _, table := range orderedTables {
		if err := g2.Migrator().DropTable(table); err != nil {
			return fmt.Errorf("error dropping table %s: %w", table, err)
		}
	}

	if err := g2.Exec("TRUNCATE TABLE migrations").Error; err != nil {
		return fmt.Errorf("error truncating migrations table: %w", err)
	}

	return nil
}

var systemTables = []string{"migrations"}

func isSystemTable(tableName string) bool {
	return slices.Contains(systemTables, tableName)
}

func (helper *Helper) getAllTables(g2 *gorm.DB) ([]string, error) {
	var tables []string
	query := `
		SELECT tablename
		FROM pg_tables
		WHERE schemaname = 'public'
		AND tablename NOT IN (?)
		ORDER BY tablename
	`
	err := g2.Raw(query, systemTables).Scan(&tables).Error
	if err != nil {
		return nil, err
	}
	return tables, nil
}

type fkEdge struct {
	TableName      string
	ReferencedName string
}

func (helper *Helper) orderTablesByDependencies(g2 *gorm.DB, tables []string) ([]string, error) {
	var edges []fkEdge
	query := `
		SELECT DISTINCT tc.table_name, ccu.table_name AS referenced_name
		FROM information_schema.table_constraints AS tc
		JOIN information_schema.key_column_usage AS kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage AS ccu
			ON ccu.constraint_name = tc.constraint_name
			AND ccu.table_schema = tc.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
			AND tc.table_schema = 'public'
	`
	if err := g2.Raw(query).Scan(&edges).Error; err != nil {
		return nil, fmt.Errorf("query foreign key edges: %w", err)
	}

	dependencies := make(map[string][]string, len(tables))
	for _, table := range tables {
		dependencies[table] = nil
	}
	for _, e := range edges {
		if e.TableName == e.ReferencedName {
			continue
		}
		if _, inScope := dependencies[e.ReferencedName]; !inScope {
			continue
		}
		if !isSystemTable(e.ReferencedName) {
			dependencies[e.TableName] = append(dependencies[e.TableName], e.ReferencedName)
		}
	}

	ordered := []string{}
	visited := make(map[string]bool)
	visiting := make(map[string]bool)

	var visit func(string) error
	visit = func(table string) error {
		if visited[table] {
			return nil
		}
		if visiting[table] {
			return fmt.Errorf("circular foreign key dependency detected involving table '%s'", table)
		}

		visiting[table] = true
		for _, dep := range dependencies[table] {
			if err := visit(dep); err != nil {
				return err
			}
		}
		visiting[table] = false
		visited[table] = true
		ordered = append(ordered, table)
		return nil
	}

	for _, table := range tables {
		if err := visit(table); err != nil {
			return nil, err
		}
	}

	slices.Reverse(ordered)

	return ordered, nil
}

func (helper *Helper) RebuildSchema() error {
	if err := helper.CleanDB(); err != nil {
		return err
	}
	if err := helper.MigrateDB(); err != nil {
		return err
	}
	tables, err := helper.getAllTables(helper.DBFactory.New(context.Background()))
	if err != nil {
		return fmt.Errorf("discover tables for truncation: %w", err)
	}
	helper.tables = tables
	return nil
}

func (helper *Helper) CreateJWTString(account *TestAccount) string {
	helper.requireJWTIssuers()
	var issuerURL, audience string
	if len(helper.AppConfig.Server.JWT.Configs) > 0 {
		issuerURL = helper.AppConfig.Server.JWT.Configs[0].IssuerURL
		audience = helper.AppConfig.Server.JWT.Configs[0].Audience
	}
	claims := jwt.MapClaims{
		"iss":        issuerURL,
		"username":   strings.ToLower(account.Username),
		"first_name": account.FirstName,
		"last_name":  account.LastName,
		"typ":        "Bearer",
		"iat":        time.Now().Unix(),
		"exp":        time.Now().Add(1 * time.Hour).Unix(),
	}
	if audience != "" {
		claims["aud"] = audience
	}
	if account.Email != "" {
		claims["email"] = account.Email
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = jwkKID

	signedToken, err := token.SignedString(helper.JWTPrivateKey)
	if err != nil {
		panic(fmt.Sprintf("test setup: unable to sign test JWT: %s", err))
	}
	return signedToken
}

func parseJWTKeys() (*rsa.PrivateKey, *rsa.PublicKey, error) {
	privateBytes, err := privatebytes()
	if err != nil {
		return nil, nil, fmt.Errorf("unable to decode JWT private key: %w", err)
	}
	pubBytes, err := publicbytes()
	if err != nil {
		return nil, nil, fmt.Errorf("unable to decode JWT CA: %w", err)
	}

	//nolint:staticcheck
	privateKey, err := jwt.ParseRSAPrivateKeyFromPEMWithPassword(privateBytes, "passwd")
	if err != nil {
		return nil, nil, fmt.Errorf("unable to parse JWT private key: %w", err)
	}
	pubKey, err := jwt.ParseRSAPublicKeyFromPEM(pubBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to parse JWT ca: %w", err)
	}

	return privateKey, pubKey, nil
}

func privatebytes() ([]byte, error) {
	s := `LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpQcm9jLVR5cGU6IDQsRU5DUllQVEVECkRF
Sy1JbmZvOiBERVMtRURFMy1DQkMsMkU2NTExOEU2QzdCNTIwNwoKN2NZVVRXNFpCZG1WWjRJTEIw
OGhjVGRtNWliMEUwemN5K0k3cEhwTlFmSkh0STdCSjRvbXlzNVMxOXVmSlBCSgpJellqZU83b1RW
cUkzN0Y2RVVtalpxRzRXVkUyVVFiUURrb3NaYlpOODJPNElwdTFsRkFQRWJ3anFlUE1LdWZ6CnNu
U1FIS2ZuYnl5RFBFVk5sSmJzMTlOWEM4djZnK3BRYXk1ckgvSTZOMmlCeGdzVG11ZW1aNTRFaE5R
TVp5RU4KUi9DaWhlQXJXRUg5SDgvNGhkMmdjOVRiMnMwTXdHSElMTDRrYmJObTV0cDN4dzRpazdP
WVdOcmozbStuRzZYYgp2S1hoMnhFYW5BWkF5TVhUcURKVEhkbjcvQ0VxdXNRUEpqWkdWK01mMWtq
S3U3cDRxY1hGbklYUDVJTG5UVzdiCmxIb1dDNGV3ZUR6S09NUnpYbWJBQkVWU1V2eDJTbVBsNFRj
b0M1TDFTQ0FIRW1aYUtiYVk3UzVsNTN1NmdsMGYKVUx1UWJ0N0hyM1RIem5sTkZLa0dUMS95Vk50
MlFPbTFlbVpkNTVMYU5lOEU3WHNOU2xobDBncllRK1VlOEpiYQp4ODVPYXBsdFZqeE05d1ZDd2Jn
RnlpMDRpaGRLSG85ZSt1WUtlVEdLdjBoVTVPN0hFSDFldjZ0L3MydS9VRzZoClRxRXNZclZwMENN
SHB0NXVBRjZuWnlLNkdaL0NIVHhoL3J6MWhBRE1vZmVtNTkrZTZ0VnRqblBHQTNFam5KVDgKQk1P
dy9EMlFJRHhqeGoyR1V6eitZSnA1MEVOaFdyTDlvU0RrRzJuenY0TlZMNzdRSXkrVC8yL2Y0UGdv
a1VETwpRSmpJZnhQV0U0MGNIR0hwblF0WnZFUG94UDBIM1QwWWhtRVZ3dUp4WDN1YVdPWS84RmEx
YzdMbjBTd1dkZlY1CmdZdkpWOG82YzNzdW1jcTFPM2FnUERsSEM1TzRJeEc3QVpROENIUkR5QVNv
Z3pma1k2UDU3OVpPR1lhTzRhbDcKV0ExWUlwc0hzMy8xZjRTQnlNdVdlME5Wa0Zmdlhja2pwcUdy
QlFwVG1xUXprNmJhYTBWUTBjd1UzWGxrd0hhYwpXQi9mUTRqeWx3RnpaRGNwNUpBbzUzbjZhVTcy
emdOdkRsR1ROS3dkWFhaSTVVM0pQb2NIMEFpWmdGRldZSkxkCjYzUEpMRG5qeUUzaTZYTVZseGlm
WEtrWFZ2MFJZU3orQnlTN096OWFDZ25RaE5VOHljditVeHRma1BRaWg1ekUKLzBZMkVFRmtuYWpt
RkpwTlhjenpGOE9FemFzd21SMEFPamNDaWtsWktSZjYxcmY1ZmFKeEpoaHFLRUVCSnVMNgpvb2RE
VlJrM09HVTF5UVNCYXpUOG5LM1YrZTZGTW8zdFdrcmEyQlhGQ0QrcEt4VHkwMTRDcDU5UzF3NkYx
Rmp0CldYN2VNV1NMV2ZRNTZqMmtMTUJIcTVnYjJhcnFscUgzZnNZT1REM1ROakNZRjNTZ3gzMDlr
VlB1T0s1dnc2MVAKcG5ML0xOM2lHWTQyV1IrOWxmQXlOTjJxajl6dndLd3NjeVlzNStEUFFvUG1j
UGNWR2Mzdi91NjZiTGNPR2JFVQpPbEdhLzZnZEQ0R0NwNUU0ZlAvN0dibkVZL1BXMmFicXVGaEdC
K3BWZGwzLzQrMVUvOGtJdGxmV05ab0c0RmhFCmdqTWQ3Z2xtcmRGaU5KRkZwZjVrczFsVlhHcUo0
bVp4cXRFWnJ4VUV3Y2laam00VjI3YStFMkt5VjlObmtzWjYKeEY0dEdQS0lQc3ZOVFY1bzhacWpp
YWN4Z2JZbXIyeXdxRFhLQ2dwVS9SV1NoMXNMYXBxU1FxYkgvdzBNcXVVagpWaFZYMFJNWUgvZm9L
dGphZ1pmL0tPMS9tbkNJVGw4NnRyZUlkYWNoR2dSNHdyL3FxTWpycFBVYVBMQ1JZM0pRCjAwWFVQ
MU11NllQRTBTbk1ZQVZ4WmhlcUtIbHkzYTFwZzRYcDdZV2xNNjcxb1VPUnMzK1ZFTmZuYkl4Z3Ir
MkQKVGlKVDlQeHdwZks1M09oN1JCU1dISlpSdUFkTFVYRThERytibDBOL1FrSk02cEZVeFRJMUFR
PT0KLS0tLS1FTkQgUlNBIFBSSVZBVEUgS0VZLS0tLS0K`

	return base64.StdEncoding.DecodeString(s)
}

func publicbytes() ([]byte, error) {
	s := `LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUMvekNDQWVlZ0F3SUJBZ0lCQVRBTkJna3Fo
a2lHOXcwQkFRVUZBREFhTVFzd0NRWURWUVFHRXdKVlV6RUwKTUFrR0ExVUVDZ3dDV2pRd0hoY05N
VE13T0RJNE1UZ3lPRE0wV2hjTk1qTXdPREk0TVRneU9ETTBXakFhTVFzdwpDUVlEVlFRR0V3SlZV
ekVMTUFrR0ExVUVDZ3dDV2pRd2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtB
b0lCQVFEZmRPcW90SGQ1NVNZTzBkTHoyb1hlbmd3L3RaK3EzWm1PUGVWbU11T01JWU8vQ3Yxd2sy
VTAKT0s0cHVnNE9CU0pQaGwwOVpzNkl3QjhOd1BPVTdFRFRnTU9jUVVZQi82UU5DSTFKN1ptMm9M
dHVjaHp6NHBJYgorbzRaQWhWcHJMaFJ5dnFpOE9US1E3a2ZHZnM1VHV3bW4xTS8wZlFrZnpNeEFE
cGpPS05nZjB1eTZsTjZ1dGpkClRyUEtLRlVRTmRjNi9UeThFZVRuUUV3VWxzVDJMQVhDZkVLeFRu
NVJsUmxqRHp0UzdTZmdzOFZMMEZQeTFRaTgKQitkRmNnUllLRnJjcHNWYVoxbEJtWEtzWERSdTVR
Ui9SZzNmOURScTRHUjFzTkg4UkxZOXVBcE1sMlNOeitzUgo0elJQRzg1Ui9zZTVRMDZHdTBCVVEz
VVBtNjdFVFZaTEFnTUJBQUdqVURCT01CMEdBMVVkRGdRV0JCUUhaUFRFCnlRVnUvMEkvM1FXaGxU
eVc3V29UelRBZkJnTlZIU01FR0RBV2dCUUhaUFRFeVFWdS8wSS8zUVdobFR5VzdXb1QKelRBTUJn
TlZIUk1FQlRBREFRSC9NQTBHQ1NxR1NJYjNEUUVCQlFVQUE0SUJBUURIeHFKOXk4YWxUSDdhZ1ZN
VwpaZmljL1JicmR2SHd5cStJT3JnRFRvcXlvMHcrSVo2QkNuOXZqdjVpdWhxdTRGb3JPV0RBRnBR
S1pXMERMQkpFClF5LzcvMCs5cGsyRFBoSzFYemRPb3ZsU3JrUnQrR2NFcEduVVhuekFDWERCYk8w
K1dyaytoY2pFa1FSUksxYlcKMnJrbkFSSUVKRzlHUytwU2hQOUJxLzBCbU5zTWVwZE5jQmEwejNh
NUIwZnpGeUNRb1VsWDZSVHF4UncxaDFRdAo1RjAwcGZzcDdTalhWSXZZY2V3SGFOQVNidG8xbjVo
clN6MVZZOWhMYmExMWl2TDFONFdvV2JtekFMNkJXYWJzCkMyRC9NZW5TVDIvWDZoVEt5R1hwZzNF
ZzJoM2lMdlV0d2NObnkwaFJLc3RjNzNKbDl4UjNxWGZYS0pIMFRoVGwKcTBncQotLS0tLUVORCBD
RVJUSUZJQ0FURS0tLS0tCg==`
	return base64.StdEncoding.DecodeString(s)
}

func initTestLogger() {
	cfg := &logger.LogConfig{
		Level:     slog.LevelInfo,
		Format:    logger.FormatText,
		Output:    os.Stdout,
		Component: "hyperfleet-api-test",
		Version:   "test",
		Hostname:  "test-host",
	}
	logger.InitGlobalLogger(cfg)
}
