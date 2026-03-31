package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/onsi/gomega"
	postgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
)

type dbSessionFactory struct {
	gormDB *gorm.DB
	sqlDB  *sql.DB
}

func (f *dbSessionFactory) Init(*config.DatabaseConfig)                             {}
func (f *dbSessionFactory) New(_ context.Context) *gorm.DB                          { return f.gormDB }
func (f *dbSessionFactory) CheckConnection() error                                  { return nil }
func (f *dbSessionFactory) Close() error                                            { return nil }
func (f *dbSessionFactory) ResetDB()                                                {}
func (f *dbSessionFactory) NewListener(_ context.Context, _ string, _ func(string)) {}
func (f *dbSessionFactory) GetAdvisoryLockTimeout() int                             { return 300 }
func (f *dbSessionFactory) DirectDB() *sql.DB                                       { return f.sqlDB }

func newDBSessionFactory(t *testing.T, setupMock func(sqlmock.Sqlmock)) *dbSessionFactory {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	setupMock(mock)

	gormDB, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{
		Logger: gormlogger.Discard,
	})
	if err != nil {
		t.Fatalf("failed to open GORM: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sqlmock expectations: %v", err)
		}
	})
	return &dbSessionFactory{gormDB: gormDB, sqlDB: db}
}

func TestIsWriteMethod_StandardHTTPMethods(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		expected bool
	}{
		// Write methods
		{"POST is write", http.MethodPost, true},
		{"PUT is write", http.MethodPut, true},
		{"PATCH is write", http.MethodPatch, true},
		{"DELETE is write", http.MethodDelete, true},

		// Read methods
		{"GET is read-only", http.MethodGet, false},
		{"HEAD is read-only", http.MethodHead, false},
		{"OPTIONS is read-only", http.MethodOptions, false},

		// Edge cases
		{"CONNECT is read-only (conservative)", http.MethodConnect, false},
		{"TRACE is read-only (conservative)", http.MethodTrace, false},
		{"Empty string is read-only", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isWriteMethod(tt.method)
			if result != tt.expected {
				t.Errorf("isWriteMethod(%q) = %v, want %v", tt.method, result, tt.expected)
			}
		})
	}
}

func TestIsWriteMethod_NonStandardMethods(t *testing.T) {
	// WebDAV and other non-standard methods
	// These are treated as read-only (false) which is safe because:
	// 1. hyperfleet-api router doesn't accept these methods (405 at routing layer)
	// 2. If they somehow reach here, no transaction is conservative but acceptable
	nonStandardMethods := []string{
		"PROPFIND",
		"PROPPATCH",
		"MKCOL",
		"COPY",
		"MOVE",
		"LOCK",
		"UNLOCK",
		"CUSTOM",
	}

	for _, method := range nonStandardMethods {
		t.Run(method, func(t *testing.T) {
			result := isWriteMethod(method)
			if result != false {
				t.Errorf("isWriteMethod(%q) = %v, want false (non-standard methods default to read-only)", method, result)
			}
		})
	}
}

func TestTransactionMiddleware_DBUnavailable(t *testing.T) {
	tests := []struct {
		setupMock      func(sqlmock.Sqlmock)
		name           string
		expectedType   string
		expectedCode   string
		expectedDetail string
		expectedStatus int
	}{
		{
			name: "DB unreachable (connection refused) - returns 503",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin().WillReturnError(
					&net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED},
				)
			},
			expectedStatus: http.StatusServiceUnavailable,
			expectedType:   "https://api.hyperfleet.io/errors/service-unavailable",
			expectedCode:   "HYPERFLEET-SVC-001",
			expectedDetail: "Database connection unavailable",
		},
		{
			name: "DB unreachable (connection dropped) - returns 503",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin().WillReturnError(io.EOF)
			},
			expectedStatus: http.StatusServiceUnavailable,
			expectedType:   "https://api.hyperfleet.io/errors/service-unavailable",
			expectedCode:   "HYPERFLEET-SVC-001",
			expectedDetail: "Database connection unavailable",
		},
		{
			name: "internal DB error (permission denied) - returns 500",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin().WillReturnError(errors.New("permission denied for table clusters"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedType:   "https://api.hyperfleet.io/errors/internal-error",
			expectedCode:   "HYPERFLEET-INT-001",
			expectedDetail: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			factory := newDBSessionFactory(t, tt.setupMock)
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			handler := TransactionMiddleware(next, factory, 0)
			req := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/clusters", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(tt.expectedStatus))
			Expect(rec.Header().Get("Content-Type")).To(Equal("application/problem+json"))

			var body struct {
				Code   *string `json:"code"`
				Detail string  `json:"detail"`
				Type   string  `json:"type"`
				Status int     `json:"status"`
			}
			Expect(json.NewDecoder(rec.Body).Decode(&body)).To(Succeed())
			Expect(body.Type).To(Equal(tt.expectedType))
			Expect(body.Status).To(Equal(tt.expectedStatus))
			Expect(body.Code).NotTo(BeNil())
			Expect(*body.Code).To(Equal(tt.expectedCode))
			if tt.expectedDetail != "" {
				Expect(body.Detail).To(Equal(tt.expectedDetail))
			}
		})
	}
}

func TestIsDBConnectionError(t *testing.T) {
	tests := []struct {
		err      error
		name     string
		expected bool
	}{
		{
			name:     "nil",
			err:      nil,
			expected: false,
		},
		{
			name:     "TCP connection refused",
			err:      &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED},
			expected: true,
		},
		{
			name:     "TCP connection reset",
			err:      &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET},
			expected: true,
		},
		{
			name:     "EOF",
			err:      io.EOF,
			expected: true,
		},
		{
			name:     "unexpected EOF",
			err:      io.ErrUnexpectedEOF,
			expected: true,
		},
		{
			name:     "wrapped TCP connection refused",
			err:      fmt.Errorf("driver: %w", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}),
			expected: true,
		},
		{
			name:     "database constraint violation",
			err:      errors.New("violates unique constraint"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(IsDBConnectionError(tt.err)).To(Equal(tt.expected))
		})
	}
}
