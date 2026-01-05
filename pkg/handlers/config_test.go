package handlers

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
)

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// TestConfigEndpoint_ReturnsExpectedValues tests that the /config endpoint returns correct values
func TestConfigEndpoint_ReturnsExpectedValues(t *testing.T) {
	// Create a config with known values
	cfg := config.NewApplicationConfig()
	cfg.App.Name = "test-app"
	cfg.App.Version = "5.0.0"
	cfg.Server.Port = 9000
	cfg.Database.Host = "db.example.com"
	cfg.Database.Password = randSeq(10)
	cfg.OCM.ClientSecret = randSeq(10)
	cfg.Server.Auth.JWT.CertURL = "https://example.com/certs" // Should be redacted (contains 'cert')

	// Create handler
	handler := NewConfigHandler(cfg)

	// Create test request
	req := httptest.NewRequest("GET", "/api/hyperfleet/config", nil)
	w := httptest.NewRecorder()

	// Call handler
	handler.Get(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 OK")
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"), "Should return JSON")

	// Parse response body
	var response config.ApplicationConfig
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Response should be valid JSON")

	// Verify non-sensitive values are present
	assert.Equal(t, "test-app", response.App.Name, "App name should be present")
	assert.Equal(t, "5.0.0", response.App.Version, "App version should be present")
	assert.Equal(t, 9000, response.Server.Port, "Server port should be present")
	assert.Equal(t, "db.example.com", response.Database.Host, "Database host should be present")

	// Verify sensitive values are redacted
	assert.Equal(t, "***", response.Database.Password, "Database password should be redacted")
	assert.Equal(t, "***", response.OCM.ClientSecret, "OCM client secret should be redacted")
	assert.Equal(t, "***", response.Server.Auth.JWT.CertURL, "JWT cert URL should be redacted")
}

// TestConfigEndpoint_SensitiveFieldsRedacted tests various sensitive field patterns
func TestConfigEndpoint_SensitiveFieldsRedacted(t *testing.T) {
	cfg := config.NewApplicationConfig()
	cfg.Database.Password = randSeq(10)
	cfg.OCM.ClientSecret = randSeq(10)
	cfg.OCM.SelfToken = "token-789"
	cfg.Server.Auth.JWT.CertFile = "/path/to/cert.pem"
	cfg.Server.Auth.JWT.CertURL = "https://certs.example.com"
	cfg.Server.HTTPS.CertFile = "/path/to/server-cert.pem"
	cfg.Server.HTTPS.KeyFile = "/path/to/server-key.pem"
	cfg.Database.RootCertFile = "/path/to/root-cert.pem"

	// Create handler
	handler := NewConfigHandler(cfg)

	// Create test request
	req := httptest.NewRequest("GET", "/api/hyperfleet/config", nil)
	w := httptest.NewRecorder()

	// Call handler
	handler.Get(w, req)

	// Parse response
	var response config.ApplicationConfig
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// All sensitive fields should be redacted
	assert.Equal(t, "***", response.Database.Password, "Password should be redacted")
	assert.Equal(t, "***", response.OCM.ClientSecret, "ClientSecret should be redacted")
	assert.Equal(t, "***", response.OCM.SelfToken, "SelfToken should be redacted")
	assert.Equal(t, "***", response.Server.Auth.JWT.CertFile, "CertFile should be redacted")
	assert.Equal(t, "***", response.Server.Auth.JWT.CertURL, "CertURL should be redacted")
	assert.Equal(t, "***", response.Server.HTTPS.CertFile, "HTTPS CertFile should be redacted")
	assert.Equal(t, "***", response.Server.HTTPS.KeyFile, "KeyFile should be redacted")
	assert.Equal(t, "***", response.Database.RootCertFile, "RootCertFile should be redacted")
}

// TestConfigEndpoint_EmptySensitiveFieldsNotRedacted tests that empty sensitive fields remain empty
func TestConfigEndpoint_EmptySensitiveFieldsNotRedacted(t *testing.T) {
	cfg := config.NewApplicationConfig()
	// Leave sensitive fields empty

	// Create handler
	handler := NewConfigHandler(cfg)

	// Create test request
	req := httptest.NewRequest("GET", "/api/hyperfleet/config", nil)
	w := httptest.NewRecorder()

	// Call handler
	handler.Get(w, req)

	// Parse response
	var response config.ApplicationConfig
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Empty fields should remain empty, not "***"
	assert.Empty(t, response.Database.Password, "Empty password should remain empty")
	assert.Empty(t, response.OCM.ClientSecret, "Empty client secret should remain empty")
}

// TestConfigEndpoint_IntegrationWithLoadConfig tests the full integration
func TestConfigEndpoint_IntegrationWithLoadConfig(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	configYAML := `
app:
  name: integration-test
  version: 6.0.0
server:
  host: file-host
  port: 7777
metrics:
  host: localhost
  port: 8080
health_check:
  host: localhost
  port: 8083
database:
  dialect: postgres
  password: secret123
  username: testuser
ocm:
  base_url: https://api.integration.openshift.com
  client_secret: ocmsecret
`
	err := os.WriteFile(configFile, []byte(configYAML), 0o644)
	require.NoError(t, err)

	// Override with environment variable
	os.Setenv("HYPERFLEET_SERVER_PORT", "8888")
	defer os.Unsetenv("HYPERFLEET_SERVER_PORT")

	// Create flag set
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := config.NewApplicationConfig()

	// Create viper and configure flags
	v := config.NewCommandConfig()
	cfg.ConfigureFlags(v, flags)

	err = flags.Parse([]string{"--config=" + configFile})
	require.NoError(t, err)

	// Load config with full precedence
	loadedCfg, err := config.LoadConfig(v, flags)
	require.NoError(t, err)

	// Create handler with loaded config
	handler := NewConfigHandler(loadedCfg)

	// Create test request
	req := httptest.NewRequest("GET", "/api/hyperfleet/config", nil)
	w := httptest.NewRecorder()

	// Call handler
	handler.Get(w, req)

	// Parse response
	var response config.ApplicationConfig
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Verify precedence in endpoint response
	assert.Equal(t, "file-host", response.Server.Host, "Should get hostname from file")
	assert.Equal(t, 8888, response.Server.Port, "Should get port from env var (overrides file)")
	assert.Equal(t, "testuser", response.Database.Username, "Should get username from file")

	// Verify redaction in endpoint response
	assert.Equal(t, "***", response.Database.Password, "Password should be redacted in endpoint")
	assert.Equal(t, "***", response.OCM.ClientSecret, "Client secret should be redacted in endpoint")
}
