package auth

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	hfl "github.com/openshift-hyperfleet/hyperfleet-logger"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

func TestHandleError_OnlyServerFailuresIncludeStack(t *testing.T) {
	for _, tt := range []struct {
		name   string
		code   string
		status int
		stack  bool
	}{
		{name: "missing credentials", code: errors.CodeAuthNoCredentials, status: http.StatusUnauthorized},
		{name: "internal failure", code: errors.CodeInternalGeneral, status: http.StatusInternalServerError, stack: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(logger.NewLogger("test", logger.HandlerConfig{
				Level: slog.LevelInfo, Format: hfl.FormatJSON, Output: &output,
			}))
			t.Cleanup(func() { slog.SetDefault(previous) })
			req := httptest.NewRequest(http.MethodGet, "/test", nil).WithContext(t.Context())
			res := httptest.NewRecorder()
			handleError(req.Context(), res, req, tt.code, "test error")
			if res.Code != tt.status {
				t.Errorf("HTTP status = %d, want %d", res.Code, tt.status)
			}
			var record map[string]any
			if err := json.Unmarshal(output.Bytes(), &record); err != nil {
				t.Fatal(err)
			}
			if _, ok := record["stack_trace"]; ok != tt.stack {
				t.Errorf("stack present = %v, want %v", ok, tt.stack)
			}
		})
	}
}
