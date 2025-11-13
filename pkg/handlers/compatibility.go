package handlers

import (
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet/pkg/errors"
)

type compatibilityHandler struct{}

func NewCompatibilityHandler() *compatibilityHandler {
	return &compatibilityHandler{}
}

// Get returns API compatibility information
func (h compatibilityHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			response := map[string]interface{}{
				"api_version": "v1",
				"compatible":  true,
				"features": []string{
					"clusters",
					"nodepools",
					"adapter_status",
					"status_aggregation",
				},
			}
			return response, nil
		},
	}

	handleGet(w, r, cfg)
}
