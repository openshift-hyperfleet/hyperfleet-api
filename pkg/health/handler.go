package health

import (
	"encoding/json"
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
)

// Handler provides HTTP handlers for health checks
type Handler struct {
	sessionFactory db.SessionFactory
}

// NewHandler creates a new health handler
func NewHandler(sessionFactory db.SessionFactory) *Handler {
	return &Handler{
		sessionFactory: sessionFactory,
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

		if err := sqlDB.PingContext(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "not_ready",
				"reason": "Database ping failed",
			})
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
