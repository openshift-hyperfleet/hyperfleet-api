package handlers

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

//go:embed poc-dashboard.html
var pocDashboard embed.FS

type PocDashboardHandler struct {
	content []byte
}

func NewPocDashboardHandler() (*PocDashboardHandler, error) {
	content, err := fs.ReadFile(pocDashboard, "poc-dashboard.html")
	if err != nil {
		return nil, errors.GeneralError(
			"can't load PoC dashboard HTML from embedded file: %v",
			err,
		)
	}
	slog.InfoContext(context.Background(), "Loaded PoC dashboard HTML from embedded file")

	return &PocDashboardHandler{content: content}, nil
}

func (h *PocDashboardHandler) GetPocDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(h.content); err != nil {
		// Response already committed, can't report error
		slog.ErrorContext(r.Context(),
			"Failed to write PoC dashboard response", logger.HTTPPath(r.URL.Path),
			logger.HTTPMethod(r.Method),
			logger.HTTPStatusCode(http.StatusOK), "error", err)
		return
	}
}
