package test

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bxcodec/faker/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gorm.io/gorm"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/server"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/test/factories"
	"github.com/openshift-hyperfleet/hyperfleet-api/test/mocks"
)

const (
	apiPort    = ":8777"
	jwtKeyFile = "test/support/jwt_private_key.pem"
	jwtCAFile  = "test/support/jwt_ca.pem"
	jwkKID     = "uhctestkey"
	jwkAlg     = "RS256"
)

var (
	helper *Helper
	once   sync.Once
)

// jwkURL stores the JWK mock server URL for testing
var jwkURL string

// TimeFunc defines a way to get a new Time instance common to the entire test suite.
// Aria's environment has Virtual Time that may not be actual time. We compensate
// by synchronizing on a common time func attached to the test harness.
type TimeFunc func() time.Time

type Helper struct {
	Factories     factories.Factories
	Ctx           context.Context
	DBFactory     db.SessionFactory
	APIServer     server.Server
	MetricsServer server.Server
	HealthServer  server.Server
	AppConfig     *config.ApplicationConfig
	TimeFunc      TimeFunc
	JWTPrivateKey *rsa.PrivateKey
	JWTCA         *rsa.PublicKey
	T             *testing.T
	teardowns     []func() error
}

func NewHelper(t *testing.T) *Helper {
	once.Do(func() {
		// Initialize logger first
		initTestLogger()
		ctx := context.Background()

		jwtKey, jwtCA, err := parseJWTKeys()
		if err != nil {
			fmt.Println("Unable to read JWT keys - this may affect tests that make authenticated server requests")
		}

		// Load configuration using ConfigLoader (same path as production)
		emptyCmd := &cobra.Command{}
		loader := config.NewConfigLoader()
		cfg, err := loader.Load(ctx, emptyCmd)
		if err != nil {
			logger.WithError(ctx, err).Error("Failed to load test configuration")
			os.Exit(1)
		}

		env := environments.Environment()
		env.Config = cfg

		err = env.SetEnvironmentDefaults(pflag.CommandLine)
		if err != nil {
			logger.WithError(ctx, err).Error("Unable to set environment defaults")
			os.Exit(1)
		}
		if logLevel := os.Getenv("LOGLEVEL"); logLevel != "" {
			logger.With(ctx, logger.FieldLogLevel, logLevel).Info("Using custom loglevel")
			pflag.CommandLine.Set("-v", logLevel) //nolint:errcheck // best-effort log level override for tests
		}
		pflag.Parse()

		err = env.Initialize()
		if err != nil {
			logger.WithError(ctx, err).Error("Unable to initialize testing environment")
			os.Exit(1)
		}

		helper = &Helper{
			AppConfig:     environments.Environment().Config,
			DBFactory:     environments.Environment().Database.SessionFactory,
			JWTPrivateKey: jwtKey,
			JWTCA:         jwtCA,
		}

		// Start JWK certificate mock server for testing
		jwkMockTeardown := helper.StartJWKCertServerMock()
		// Teardown order: terminate the testcontainer FIRST so the
		// container is removed before anything else. If server shutdown hangs
		// and the force-exit goroutine kills the process, the container
		// would remain alive and keep the Prow pod stuck (HYPERFLEET-625).
		// CleanDB is omitted because the container is destroyed anyway.
		helper.teardowns = []func() error{
			helper.teardownEnv,
			helper.stopAPIServer,
			helper.stopMetricsServer,
			helper.stopHealthServer,
			jwkMockTeardown,
		}
		helper.startAPIServer()
		helper.startMetricsServer()
		helper.startHealthServer()
	})
	helper.T = t
	return helper
}

func (helper *Helper) Env() *environments.Env {
	return environments.Environment()
}

func (helper *Helper) teardownEnv() error {
	helper.Env().Teardown()
	return nil
}

func (helper *Helper) Teardown() {
	for _, f := range helper.teardowns {
		err := f()
		if err != nil {
			helper.T.Errorf("error running teardown func: %s", err)
		}
	}
}

func (helper *Helper) startAPIServer() {
	ctx := context.Background()
	// Configure JWK certificate URL for API server
	helper.Env().Config.Server.JWK.CertURL = jwkURL
	// Disable tracing for integration tests (no OTLP collector required)
	helper.APIServer = server.NewAPIServer(false)
	listener, err := helper.APIServer.Listen()
	if err != nil {
		logger.WithError(ctx, err).Error("Unable to start Test API server")
		os.Exit(1)
	}
	go func() {
		logger.Debug(ctx, "Test API server started")
		helper.APIServer.Serve(listener)
		logger.Debug(ctx, "Test API server stopped")
	}()
}

func (helper *Helper) stopAPIServer() error {
	if err := helper.APIServer.Stop(); err != nil {
		return fmt.Errorf("unable to stop api server: %s", err.Error())
	}
	return nil
}

func (helper *Helper) startMetricsServer() {
	ctx := context.Background()
	helper.MetricsServer = server.NewMetricsServer()
	go func() {
		logger.Debug(ctx, "Test Metrics server started")
		helper.MetricsServer.Start()
		logger.Debug(ctx, "Test Metrics server stopped")
	}()
}

func (helper *Helper) stopMetricsServer() error {
	if err := helper.MetricsServer.Stop(); err != nil {
		return fmt.Errorf("unable to stop metrics server: %s", err.Error())
	}
	return nil
}

func (helper *Helper) stopHealthServer() error {
	if err := helper.HealthServer.Stop(); err != nil {
		return fmt.Errorf("unable to stop health server: %s", err.Error())
	}
	return nil
}

func (helper *Helper) startHealthServer() {
	ctx := context.Background()
	helper.HealthServer = server.NewHealthServer()
	go func() {
		logger.Debug(ctx, "Test health check server started")
		helper.HealthServer.Start()
		logger.Debug(ctx, "Test health check server stopped")
	}()
}

func (helper *Helper) RestartServer() {
	ctx := context.Background()
	if err := helper.stopAPIServer(); err != nil {
		logger.WithError(ctx, err).Warn("unable to stop api server on restart")
	}
	helper.startAPIServer()
	logger.Debug(ctx, "Test API server restarted")
}

func (helper *Helper) RestartMetricsServer() {
	ctx := context.Background()
	if err := helper.stopMetricsServer(); err != nil {
		logger.WithError(ctx, err).Warn("unable to stop metrics server on restart")
	}
	helper.startMetricsServer()
	logger.Debug(ctx, "Test metrics server restarted")
}

func (helper *Helper) Reset() {
	ctx := context.Background()
	logger.Info(ctx, "Resetting testing environment")
	env := environments.Environment()
	// Reset the configuration
	env.Config = config.NewApplicationConfig()

	// Re-read command-line configuration into a NEW flagset
	// This new flag set ensures we don't hit conflicts defining the same flag twice
	// Also on reset, we don't care to be re-defining 'v' and other glog flags
	flagset := pflag.NewFlagSet(helper.NewID(), pflag.ContinueOnError)
	if err := env.SetEnvironmentDefaults(flagset); err != nil {
		logger.WithError(ctx, err).Error("Unable to set environment defaults on Reset")
		os.Exit(1)
	}
	pflag.Parse()

	err := env.Initialize()
	if err != nil {
		logger.WithError(ctx, err).Error("Unable to reset testing environment")
		os.Exit(1)
	}
	helper.AppConfig = env.Config
	helper.RestartServer()
}

// NewID creates a new unique ID used internally
func (helper *Helper) NewID() string {
	id, err := uuid.NewV7()
	if err != nil {
		panic(fmt.Sprintf("test helper: failed to generate UUID v7: %v", err))
	}
	return id.String()
}

// NewUUID creates a new unique UUID, which has different formatting than ksuid
// UUID is used by telemeter and we validate the format.
func (helper *Helper) NewUUID() string {
	return uuid.New().String()
}

func (helper *Helper) RestURL(path string) string {
	protocol := "http" //nolint:goconst // Protocol strings used across URL builders
	if helper.AppConfig.Server.TLS.Enabled {
		protocol = "https" //nolint:goconst // Protocol strings used across URL builders
	}
	return fmt.Sprintf("%s://%s/api/hyperfleet/v1%s", protocol, helper.AppConfig.Server.BindAddress(), path)
}

func (helper *Helper) MetricsURL(path string) string {
	protocol := "http" //nolint:goconst // Protocol strings used across URL builders
	if helper.AppConfig.Metrics.TLS.Enabled {
		protocol = "https" //nolint:goconst // Protocol strings used across URL builders
	}
	return fmt.Sprintf("%s://%s%s", protocol, helper.AppConfig.Metrics.BindAddress(), path)
}

func (helper *Helper) HealthURL(path string) string {
	protocol := "http" //nolint:goconst // Protocol strings used across URL builders
	if helper.AppConfig.Health.TLS.Enabled {
		protocol = "https" //nolint:goconst // Protocol strings used across URL builders
	}
	return fmt.Sprintf("%s://%s%s", protocol, helper.AppConfig.Health.BindAddress(), path)
}

func (helper *Helper) NewAPIClient() *openapi.ClientWithResponses {
	// Build the server URL
	protocol := "http" //nolint:goconst // Protocol strings used across URL builders
	if helper.AppConfig.Server.TLS.Enabled {
		protocol = "https" //nolint:goconst // Protocol strings used across URL builders
	}
	serverURL := fmt.Sprintf("%s://%s", protocol, helper.AppConfig.Server.BindAddress())
	client, err := openapi.NewClientWithResponses(serverURL)
	if err != nil {
		helper.T.Fatalf("Failed to create API client: %v", err)
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
	return helper.NewAccount(helper.NewID(), faker.Name(), faker.Email())
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

// contextKeyAccessToken is a context key for storing the access token
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

func (helper *Helper) StartJWKCertServerMock() (teardown func() error) {
	jwkURL, teardown = mocks.NewJWKCertServerMock(helper.T, helper.JWTCA, jwkKID, jwkAlg)
	helper.Env().Config.Server.JWK.CertURL = jwkURL
	return teardown
}

func (helper *Helper) DeleteAll(table interface{}) {
	g2 := helper.DBFactory.New(context.Background())
	err := g2.Model(table).Delete(table).Error
	if err != nil {
		helper.T.Errorf("error deleting from table %v: %v", table, err)
	}
}

func (helper *Helper) Delete(obj interface{}) {
	g2 := helper.DBFactory.New(context.Background())
	err := g2.Delete(obj).Error
	if err != nil {
		helper.T.Errorf("error deleting object %v: %v", obj, err)
	}
}

func (helper *Helper) SkipIfShort() {
	if testing.Short() {
		helper.T.Skip("Skipping execution of test in short mode")
	}
}

func (helper *Helper) Count(table string) int64 {
	g2 := helper.DBFactory.New(context.Background())
	var count int64
	err := g2.Table(table).Count(&count).Error
	if err != nil {
		helper.T.Errorf("error getting count for table %s: %v", table, err)
	}
	return count
}

func (helper *Helper) MigrateDB() error {
	return db.Migrate(helper.DBFactory.New(context.Background()))
}

func (helper *Helper) MigrateDBTo(migrationID string) {
	db.MigrateTo(helper.DBFactory, migrationID)
}

func (helper *Helper) ClearAllTables() {
	// Reserved for future use
}

func (helper *Helper) CleanDB() error {
	g2 := helper.DBFactory.New(context.Background())

	tables, err := helper.getAllTables(g2)
	if err != nil {
		helper.T.Errorf("error discovering tables: %v", err)
		return err
	}

	orderedTables, err := helper.orderTablesByDependencies(g2, tables)
	if err != nil {
		helper.T.Errorf("error ordering tables by dependencies: %v", err)
		return err
	}

	for _, table := range orderedTables {
		if g2.Migrator().HasTable(table) {
			if err := g2.Migrator().DropTable(table); err != nil {
				helper.T.Errorf("error dropping table %s: %v", table, err)
				return err
			}
		}
	}

	// Truncate migrations table so MigrateDB() will re-run all migrations
	// This ensures tables are recreated after CleanDB() drops them
	if err := g2.Exec("TRUNCATE TABLE migrations").Error; err != nil {
		helper.T.Errorf("error truncating migrations table: %v", err)
		return err
	}

	return nil
}

// System tables should not be dropped
var systemTables = []string{"migrations"}

func isSystemTable(tableName string) bool {
	for _, sysTable := range systemTables {
		if tableName == sysTable {
			return true
		}
	}
	return false
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

// Child tables (with foreign keys) come before parent tables to ensure safe deletion
func (helper *Helper) orderTablesByDependencies(g2 *gorm.DB, tables []string) ([]string, error) {
	dependencies := make(map[string][]string)

	for _, table := range tables {
		deps, err := helper.getTableDependencies(g2, table)
		if err != nil {
			return nil, err
		}

		filteredDeps := []string{}
		for _, dep := range deps {
			if !isSystemTable(dep) {
				filteredDeps = append(filteredDeps, dep)
			}
		}
		dependencies[table] = filteredDeps
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
			err := fmt.Errorf("circular foreign key dependency detected involving table '%s'", table)
			helper.T.Errorf("%v", err)
			return err
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

	for i, j := 0, len(ordered)-1; i < j; i, j = i+1, j-1 {
		ordered[i], ordered[j] = ordered[j], ordered[i]
	}

	return ordered, nil
}

func (helper *Helper) getTableDependencies(g2 *gorm.DB, tableName string) ([]string, error) {
	var dependencies []string
	query := `
		SELECT DISTINCT ccu.table_name
		FROM information_schema.table_constraints AS tc
		JOIN information_schema.key_column_usage AS kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage AS ccu
			ON ccu.constraint_name = tc.constraint_name
			AND ccu.table_schema = tc.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
			AND tc.table_schema = 'public'
			AND tc.table_name = ?
	`
	err := g2.Raw(query, tableName).Scan(&dependencies).Error
	if err != nil {
		return nil, err
	}
	return dependencies, nil
}

func (helper *Helper) ResetDB() error {
	if err := helper.CleanDB(); err != nil {
		return err
	}

	if err := helper.MigrateDB(); err != nil {
		return err
	}

	return nil
}

func (helper *Helper) CreateJWTString(account *TestAccount) string {
	claims := jwt.MapClaims{
		"iss":        helper.Env().Config.Server.JWT.IssuerURL,
		"username":   strings.ToLower(account.Username),
		"first_name": account.FirstName,
		"last_name":  account.LastName,
		"typ":        "Bearer",
		"iat":        time.Now().Unix(),
		"exp":        time.Now().Add(1 * time.Hour).Unix(),
	}
	if aud := helper.Env().Config.Server.JWT.Audience; aud != "" {
		claims["aud"] = aud
	}
	if account.Email != "" {
		claims["email"] = account.Email
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = jwkKID

	signedToken, err := token.SignedString(helper.JWTPrivateKey)
	if err != nil {
		helper.T.Errorf("Unable to sign test jwt: %s", err)
		return ""
	}
	return signedToken
}

func (helper *Helper) CreateJWTToken(account *TestAccount) *jwt.Token {
	tokenString := helper.CreateJWTString(account)

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return helper.JWTCA, nil
	})
	if err != nil {
		helper.T.Errorf("Unable to parse signed jwt: %s", err)
		return nil
	}
	return token
}

// OpenapiError Convert an error response body to an openapi error struct
func (helper *Helper) OpenapiError(body []byte) openapi.Error {
	var exErr openapi.Error
	jsonErr := json.Unmarshal(body, &exErr)
	if jsonErr != nil {
		helper.T.Errorf("Unable to convert error response to openapi error: %s", jsonErr)
	}
	return exErr
}

func parseJWTKeys() (*rsa.PrivateKey, *rsa.PublicKey, error) {
	// projectRootDir := getProjectRootDir()
	// privateBytes, err := os.ReadFile(filepath.Join(projectRootDir, jwtKeyFile))
	privateBytes, err := privatebytes()
	if err != nil {
		err = fmt.Errorf("unable to read JWT key file %s: %s", jwtKeyFile, err)
		return nil, nil, err
	}
	// pubBytes, err := ioutil.ReadFile(filepath.Join(projectRootDir, jwtCAFile))
	pubBytes, err := publicbytes()
	if err != nil {
		err = fmt.Errorf("unable to read JWT ca file %s: %s", jwtKeyFile, err)
		return nil, nil, err
	}

	// Parse keys
	// ParseRSAPrivateKeyFromPEMWithPassword is deprecated in the stdlib but there's
	// no suitable alternative for our test fixture keys; explicitly silence the
	// staticcheck warning here.
	//nolint:staticcheck
	privateKey, err := jwt.ParseRSAPrivateKeyFromPEMWithPassword(privateBytes, "passwd")
	if err != nil {
		err = fmt.Errorf("unable to parse JWT private key: %s", err)
		return nil, nil, err
	}
	pubKey, err := jwt.ParseRSAPublicKeyFromPEM(pubBytes)
	if err != nil {
		err = fmt.Errorf("unable to parse JWT ca: %s", err)
		return nil, nil, err
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

// initTestLogger initializes a default logger for tests
func initTestLogger() {
	cfg := &logger.LogConfig{
		Level:     slog.LevelInfo,
		Format:    logger.FormatText, // Use text format for test readability
		Output:    os.Stdout,
		Component: "hyperfleet-api-test",
		Version:   "test",
		Hostname:  "test-host",
	}
	logger.InitGlobalLogger(cfg)
}
