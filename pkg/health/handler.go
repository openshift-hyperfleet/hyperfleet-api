package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
)

// Handler provides HTTP handlers for health checks
type Handler struct {
	sessionFactory db.SessionFactory
	dbPingTimeout  time.Duration
}

// NewHandler creates a new health handler
func NewHandler(sessionFactory db.SessionFactory, dbPingTimeout time.Duration) *Handler {
	return &Handler{
		sessionFactory: sessionFactory,
		dbPingTimeout:  dbPingTimeout,
	}
}

// LivenessHandler handles the /healthz endpoint
// Returns 200 OK if the application is alive
func (h *Handler) LivenessHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ReadinessHandler handles the /readyz endpoint
// Returns 200 OK if the application is ready to receive traffic
// Returns 503 Service Unavailable if the application is shutting down or not ready
func (h *Handler) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	state := GetReadinessState()

	if state.IsShuttingDown() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "shutting_down",
			"reason": "Application is shutting down",
		})
		return
	}

	if !state.IsReady() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "not_ready",
			"reason": "Application is not ready",
		})
		return
	}

	// Check database connectivity if session factory is available
	if h.sessionFactory != nil {
		sqlDB := h.sessionFactory.DirectDB()
		if sqlDB == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "not_ready",
				"reason": "Database connection not available",
			})
			return
		}

		pingCtx, cancel := context.WithTimeout(r.Context(), h.dbPingTimeout)
		defer cancel()

		if err := sqlDB.PingContext(pingCtx); err != nil {
			reason := err.Error()
			if pingCtx.Err() == context.DeadlineExceeded {
				reason = "database ping timeout"
			}
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "not_ready",
				"reason": reason,
			})
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
