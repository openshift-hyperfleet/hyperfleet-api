package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm/logger"
)

func TestEscapeDSNValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple alphabetic value doesn't need quotes",
			input:    "simple",
			expected: "simple",
		},
		{
			name:     "value with spaces is quoted",
			input:    "pass word",
			expected: "'pass word'",
		},
		{
			name:     "single quotes are escaped and quoted",
			input:    "it's",
			expected: "'it\\'s'",
		},
		{
			name:     "backslashes are escaped and quoted",
			input:    "back\\slash",
			expected: "'back\\\\slash'",
		},
		{
			name:     "value with tab is quoted",
			input:    "has\ttab",
			expected: "'has\ttab'",
		},
		{
			name:     "value with newline is quoted",
			input:    "has\nnewline",
			expected: "'has\nnewline'",
		},
		{
			name:     "value with carriage return is quoted",
			input:    "has\rcarriage",
			expected: "'has\rcarriage'",
		},
		{
			name:     "empty string stays empty",
			input:    "",
			expected: "",
		},
		{
			name:     "value with both backslash and quote is escaped and quoted",
			input:    "path\\to\\'file",
			expected: "'path\\\\to\\\\\\'file'",
		},
		{
			name:     "value with multiple spaces is quoted",
			input:    "multiple  spaces  here",
			expected: "'multiple  spaces  here'",
		},
		{
			name:     "numeric value doesn't need quotes",
			input:    "12345",
			expected: "12345",
		},
		{
			name:     "hostname with dots doesn't need quotes",
			input:    "db.example.com",
			expected: "db.example.com",
		},
		{
			name:     "path with spaces is quoted",
			input:    "/path/to/my file.pem",
			expected: "'/path/to/my file.pem'",
		},
		{
			name:     "password with special characters is quoted",
			input:    "p@ss!word#123",
			expected: "'p@ss!word#123'",
		},
		{
			name:     "password with equals sign is quoted",
			input:    "pass=word",
			expected: "'pass=word'",
		},
		{
			name:     "password with multiple special chars",
			input:    "p@ss'w=rd\\123",
			expected: "'p@ss\\'w=rd\\\\123'",
		},
		{
			name:     "hyphen is allowed without quotes",
			input:    "my-database",
			expected: "my-database",
		},
		{
			name:     "underscore requires quotes",
			input:    "my_database",
			expected: "'my_database'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeDSNValue(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConnectionString verifies DSN string assembly and integration between components.
// Character escaping is thoroughly tested in TestEscapeDSNValue - this test focuses on:
// - Correct DSN format and parameter ordering
// - SSL configuration integration
// - Integration validation with one representative special character test case
func TestConnectionString(t *testing.T) {
	tests := []struct {
		name     string
		config   *DatabaseConfig
		ssl      bool
		expected string
	}{
		{
			name: "basic connection without SSL",
			config: &DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				Name:     "testdb",
				Username: "testuser",
				Password: "testpass",
				SSL: SSLConfig{
					Mode: "disable",
				},
			},
			ssl:      false,
			expected: "host=localhost port=5432 dbname=testdb user=testuser password=testpass sslmode=disable",
		},
		{
			name: "connection with SSL enabled",
			config: &DatabaseConfig{
				Host:     "db.example.com",
				Port:     5432,
				Name:     "proddb",
				Username: "admin",
				Password: "secret",
				SSL: SSLConfig{
					Mode: "require",
				},
			},
			ssl:      true,
			expected: "host=db.example.com port=5432 dbname=proddb user=admin password=secret sslmode=require",
		},
		{
			name: "connection with SSL cert file",
			config: &DatabaseConfig{
				Host:     "secure.db.com",
				Port:     5432,
				Name:     "securedb",
				Username: "secureuser",
				Password: "securepass",
				SSL: SSLConfig{
					Mode:         "verify-full",
					RootCertFile: "/etc/ssl/certs/ca.pem",
				},
			},
			ssl: true,
			expected: "host=secure.db.com port=5432 dbname=securedb user=secureuser password=securepass " +
				"sslmode=verify-full sslrootcert='/etc/ssl/certs/ca.pem'",
		},
		{
			name: "SSL mode disable even when ssl=true",
			config: &DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				Name:     "testdb",
				Username: "user",
				Password: "pass",
				SSL: SSLConfig{
					Mode: "disable",
				},
			},
			ssl:      true,
			expected: "host=localhost port=5432 dbname=testdb user=user password=pass sslmode=disable",
		},
		{
			name: "SSL cert file with spaces requires quoting",
			config: &DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				Name:     "testdb",
				Username: "user",
				Password: "pass",
				SSL: SSLConfig{
					Mode:         "verify-full",
					RootCertFile: "/path/to/my cert.pem",
				},
			},
			ssl: true,
			expected: "host=localhost port=5432 dbname=testdb user=user password=pass " +
				"sslmode=verify-full sslrootcert='/path/to/my cert.pem'",
		},
		{
			name: "hostname with hyphens and dots (simple values, no quotes)",
			config: &DatabaseConfig{
				Host:     "db-host.example.com",
				Port:     5432,
				Name:     "my-database",
				Username: "my-user",
				Password: "my-pass",
			},
			ssl:      false,
			expected: "host=db-host.example.com port=5432 dbname=my-database user=my-user password=my-pass sslmode=disable",
		},
		{
			// Integration test: verifies that escapeDSNValue integrates correctly with ConnectionString
			// for all special characters mentioned in code review (', \, =, spaces, #, @)
			// Detailed character escaping is tested in TestEscapeDSNValue
			name: "integration test with special characters in username and password",
			config: &DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				Name:     "testdb",
				Username: "admin@example.com",
				Password: "p@ss w'rd=test\\path#123",
			},
			ssl: false,
			expected: "host=localhost port=5432 dbname=testdb user='admin@example.com' " +
				"password='p@ss w\\'rd=test\\\\path#123' sslmode=disable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.ConnectionString(tt.ssl)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLogSafeConnectionString(t *testing.T) {
	tests := []struct {
		name     string
		config   *DatabaseConfig
		ssl      bool
		expected string
	}{
		{
			name: "credentials are redacted",
			config: &DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				Name:     "testdb",
				Username: "testuser",
				Password: "secretpassword",
				SSL: SSLConfig{
					Mode: "disable",
				},
			},
			ssl: false,
			expected: "host=localhost port=5432 dbname=testdb user='" + RedactedValue +
				"' password='" + RedactedValue + "' sslmode=disable",
		},
		{
			name: "credentials with special characters are redacted",
			config: &DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				Name:     "testdb",
				Username: "user with space",
				Password: "pass'word",
			},
			ssl: false,
			expected: "host=localhost port=5432 dbname=testdb user='" + RedactedValue +
				"' password='" + RedactedValue + "' sslmode=disable",
		},
		{
			name: "empty credentials are redacted",
			config: &DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				Name:     "testdb",
				Username: "",
				Password: "",
			},
			ssl: false,
			expected: "host=localhost port=5432 dbname=testdb user='" + RedactedValue +
				"' password='" + RedactedValue + "' sslmode=disable",
		},
		{
			name: "with SSL and cert file",
			config: &DatabaseConfig{
				Host:     "secure.db.com",
				Port:     5432,
				Name:     "securedb",
				Username: "admin",
				Password: "topsecret",
				SSL: SSLConfig{
					Mode:         "verify-full",
					RootCertFile: "/etc/ssl/certs/ca.pem",
				},
			},
			ssl: true,
			expected: "host=secure.db.com port=5432 dbname=securedb user='" + RedactedValue +
				"' password='" + RedactedValue + "' sslmode=verify-full sslrootcert='/etc/ssl/certs/ca.pem'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.LogSafeConnectionString(tt.ssl)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConnectionStringWithName(t *testing.T) {
	config := &DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		Name:     "originaldb",
		Username: "user",
		Password: "pass",
		SSL: SSLConfig{
			Mode: "disable",
		},
	}

	result := config.ConnectionStringWithName("customdb", false)
	expected := "host=localhost port=5432 dbname=customdb user=user password=pass sslmode=disable"

	assert.Equal(t, expected, result)
	// Verify original config is unchanged
	assert.Equal(t, "originaldb", config.Name)
}

func TestLogSafeConnectionStringWithName(t *testing.T) {
	config := &DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		Name:     "originaldb",
		Username: "user",
		Password: "secret",
		SSL: SSLConfig{
			Mode: "disable",
		},
	}

	result := config.LogSafeConnectionStringWithName("testdb", false)
	expected := "host=localhost port=5432 dbname=testdb user='" + RedactedValue +
		"' password='" + RedactedValue + "' sslmode=disable"

	assert.Equal(t, expected, result)
	// Verify original config is unchanged
	assert.Equal(t, "originaldb", config.Name)
}

func TestDatabaseConfigMarshalJSON(t *testing.T) {
	config := &DatabaseConfig{
		Dialect:  "postgres",
		Host:     "localhost",
		Port:     5432,
		Name:     "testdb",
		Username: "testuser",
		Password: "secretpassword",
		Debug:    true,
		SSL: SSLConfig{
			Mode:         "verify-full",
			RootCertFile: "/etc/ssl/ca.pem",
		},
		Pool: PoolConfig{
			MaxConnections: 50,
		},
	}

	jsonBytes, err := config.MarshalJSON()
	assert.NoError(t, err)

	jsonStr := string(jsonBytes)

	// Verify username and password are redacted
	assert.Contains(t, jsonStr, `"username":"`+RedactedValue+`"`)
	assert.Contains(t, jsonStr, `"password":"`+RedactedValue+`"`)

	// Verify non-sensitive fields are present
	assert.Contains(t, jsonStr, `"dialect":"postgres"`)
	assert.Contains(t, jsonStr, `"host":"localhost"`)
	assert.Contains(t, jsonStr, `"port":5432`)
	assert.Contains(t, jsonStr, `"name":"testdb"`)
	assert.Contains(t, jsonStr, `"debug":true`)

	// Verify password is not in plain text
	assert.NotContains(t, jsonStr, "secretpassword")
}

func TestDatabaseConfigMarshalJSON_EmptyCredentials(t *testing.T) {
	config := &DatabaseConfig{
		Dialect:  "postgres",
		Host:     "localhost",
		Port:     5432,
		Name:     "testdb",
		Username: "",
		Password: "",
	}

	jsonBytes, err := config.MarshalJSON()
	assert.NoError(t, err)

	jsonStr := string(jsonBytes)

	// Verify empty credentials show as empty strings, not redacted
	assert.Contains(t, jsonStr, `"username":""`)
	assert.Contains(t, jsonStr, `"password":""`)
}

func TestSetLogLevel(t *testing.T) {
	tests := []struct {
		name           string
		debug          bool
		globalLogLevel string
		expected       logger.LogLevel
	}{
		{
			name:           "DB_DEBUG=true takes precedence over LOG_LEVEL=info",
			debug:          true,
			globalLogLevel: "info",
			expected:       logger.Info,
		},
		{
			name:           "DB_DEBUG=true takes precedence over LOG_LEVEL=error",
			debug:          true,
			globalLogLevel: "error",
			expected:       logger.Info,
		},
		{
			name:           "LOG_LEVEL=debug enables SQL query logging",
			debug:          false,
			globalLogLevel: "debug",
			expected:       logger.Info,
		},
		{
			name:           "LOG_LEVEL=error suppresses SQL logs",
			debug:          false,
			globalLogLevel: "error",
			expected:       logger.Silent,
		},
		{
			name:           "LOG_LEVEL=info defaults to Warn",
			debug:          false,
			globalLogLevel: "info",
			expected:       logger.Warn,
		},
		{
			name:           "LOG_LEVEL=warn defaults to Warn",
			debug:          false,
			globalLogLevel: "warn",
			expected:       logger.Warn,
		},
		{
			name:           "empty LOG_LEVEL defaults to Warn",
			debug:          false,
			globalLogLevel: "",
			expected:       logger.Warn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &DatabaseConfig{
				Debug: tt.debug,
			}
			result := config.SetLogLevel(tt.globalLogLevel)
			assert.Equal(t, tt.expected, result)
		})
	}
}
