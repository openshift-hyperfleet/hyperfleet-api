package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

// ResourceHandler handles HTTP requests for generic CRD-based resources.
// It is CRD-aware and adapts behavior based on the resource definition.
type ResourceHandler struct {
	resource         services.ResourceService
	kind             string
	plural           string
	isOwned          bool
	ownerKind        string
	ownerPathParam   string
	requiredAdapters []string
}

// ResourceHandlerConfig contains configuration for creating a ResourceHandler.
type ResourceHandlerConfig struct {
	Kind             string
	Plural           string
	IsOwned          bool
	OwnerKind        string
	OwnerPathParam   string
	RequiredAdapters []string
}

// NewResourceHandler creates a new ResourceHandler instance.
func NewResourceHandler(
	resourceService services.ResourceService,
	cfg ResourceHandlerConfig,
) *ResourceHandler {
	return &ResourceHandler{
		resource:         resourceService,
		kind:             cfg.Kind,
		plural:           cfg.Plural,
		isOwned:          cfg.IsOwned,
		ownerKind:        cfg.OwnerKind,
		ownerPathParam:   cfg.OwnerPathParam,
		requiredAdapters: cfg.RequiredAdapters,
	}
}

// Create handles POST requests to create a new resource.
func (h *ResourceHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req api.ResourceCreateRequest
	cfg := &handlerConfig{
		&req,
		[]validate{
			validateName(&req, "Name", "name", 3, 63),
		},
		func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()

			// Convert request to domain model
			resource, err := h.convertCreateRequest(&req, r)
			if err != nil {
				return nil, err
			}

			// Create the resource
			resource, svcErr := h.resource.Create(ctx, resource, h.requiredAdapters)
			if svcErr != nil {
				return nil, svcErr
			}

			// Return the created resource
			return h.presentResource(resource), nil
		},
		handleError,
	}

	handle(w, r, cfg, http.StatusCreated)
}

// Get handles GET requests to retrieve a single resource.
func (h *ResourceHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			id := mux.Vars(r)["id"]

			var resource *api.Resource
			var svcErr *errors.ServiceError

			if h.isOwned {
				ownerID := mux.Vars(r)[h.ownerPathParam]
				resource, svcErr = h.resource.GetByOwner(ctx, h.kind, ownerID, id)
			} else {
				resource, svcErr = h.resource.Get(ctx, h.kind, id)
			}

			if svcErr != nil {
				return nil, svcErr
			}

			return h.presentResource(resource), nil
		},
	}

	handleGet(w, r, cfg)
}

// List handles GET requests to list resources.
func (h *ResourceHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			listArgs := services.NewListArguments(r.URL.Query())

			var resources api.ResourceList
			var total int64
			var svcErr *errors.ServiceError

			// Calculate offset from page and size
			offset := (listArgs.Page - 1) * int(listArgs.Size)
			limit := int(listArgs.Size)

			if h.isOwned {
				ownerID := mux.Vars(r)[h.ownerPathParam]
				resources, total, svcErr = h.resource.ListByOwner(ctx, h.kind, ownerID, offset, limit)
			} else {
				resources, total, svcErr = h.resource.ListByKind(ctx, h.kind, offset, limit)
			}

			if svcErr != nil {
				return nil, svcErr
			}

			// Build response list
			items := make([]map[string]interface{}, 0, len(resources))
			for _, resource := range resources {
				items = append(items, h.presentResource(resource))
			}

			return map[string]interface{}{
				"kind":  h.kind + "List",
				"page":  listArgs.Page,
				"size":  len(items),
				"total": total,
				"items": items,
			}, nil
		},
	}

	handleList(w, r, cfg)
}

// Patch handles PATCH requests to update a resource.
func (h *ResourceHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var patch api.ResourcePatchRequest

	cfg := &handlerConfig{
		&patch,
		[]validate{},
		func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			id := mux.Vars(r)["id"]

			// Get existing resource
			var found *api.Resource
			var svcErr *errors.ServiceError

			if h.isOwned {
				ownerID := mux.Vars(r)[h.ownerPathParam]
				found, svcErr = h.resource.GetByOwner(ctx, h.kind, ownerID, id)
			} else {
				found, svcErr = h.resource.Get(ctx, h.kind, id)
			}

			if svcErr != nil {
				return nil, svcErr
			}

			// Apply patch
			if patch.Spec != nil {
				specJSON, err := json.Marshal(*patch.Spec)
				if err != nil {
					return nil, errors.GeneralError("Failed to marshal spec: %v", err)
				}
				found.Spec = specJSON
			}

			if patch.Labels != nil {
				labelsJSON, err := json.Marshal(*patch.Labels)
				if err != nil {
					return nil, errors.GeneralError("Failed to marshal labels: %v", err)
				}
				found.Labels = labelsJSON
			}

			// Update user info
			found.UpdatedBy = "system@hyperfleet.local" // TODO: Get from auth context

			// Replace the resource
			resource, svcErr := h.resource.Replace(ctx, found)
			if svcErr != nil {
				return nil, svcErr
			}

			return h.presentResource(resource), nil
		},
		handleError,
	}

	handle(w, r, cfg, http.StatusOK)
}

// Delete handles DELETE requests to remove a resource.
func (h *ResourceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			id := mux.Vars(r)["id"]

			// Verify resource exists
			var svcErr *errors.ServiceError
			if h.isOwned {
				ownerID := mux.Vars(r)[h.ownerPathParam]
				_, svcErr = h.resource.GetByOwner(ctx, h.kind, ownerID, id)
			} else {
				_, svcErr = h.resource.Get(ctx, h.kind, id)
			}
			if svcErr != nil {
				return nil, svcErr
			}

			// Delete the resource
			svcErr = h.resource.Delete(ctx, h.kind, id)
			if svcErr != nil {
				return nil, svcErr
			}

			return nil, nil
		},
	}

	handleDelete(w, r, cfg, http.StatusNoContent)
}

// convertCreateRequest converts the API request to a domain Resource model.
func (h *ResourceHandler) convertCreateRequest(req *api.ResourceCreateRequest, r *http.Request) (*api.Resource, *errors.ServiceError) {
	// Marshal Spec
	specJSON, err := json.Marshal(req.Spec)
	if err != nil {
		return nil, errors.GeneralError("Failed to marshal spec: %v", err)
	}

	// Marshal Labels
	labels := make(map[string]string)
	if req.Labels != nil {
		labels = *req.Labels
	}
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return nil, errors.GeneralError("Failed to marshal labels: %v", err)
	}

	resource := &api.Resource{
		Kind:       h.kind,
		Name:       req.Name,
		Spec:       specJSON,
		Labels:     labelsJSON,
		Generation: 1,
		CreatedBy:  "system@hyperfleet.local", // TODO: Get from auth context
		UpdatedBy:  "system@hyperfleet.local",
	}

	// Set owner references for owned resources
	if h.isOwned {
		ownerID := mux.Vars(r)[h.ownerPathParam]
		ownerHref := fmt.Sprintf("/api/hyperfleet/v1/%s/%s", h.plural, ownerID) // Simplified, adjust as needed
		resource.OwnerID = &ownerID
		resource.OwnerKind = &h.ownerKind
		resource.OwnerHref = &ownerHref
	}

	// Set Href
	if h.isOwned {
		ownerID := mux.Vars(r)[h.ownerPathParam]
		resource.Href = fmt.Sprintf("/api/hyperfleet/v1/%s/%s/%s", getOwnerPlural(h.ownerKind), ownerID, h.plural)
	} else {
		resource.Href = fmt.Sprintf("/api/hyperfleet/v1/%s", h.plural)
	}

	return resource, nil
}

// presentResource converts a domain Resource to an API response map.
func (h *ResourceHandler) presentResource(resource *api.Resource) map[string]interface{} {
	result := map[string]interface{}{
		"id":           resource.ID,
		"kind":         resource.Kind,
		"name":         resource.Name,
		"href":         resource.Href,
		"generation":   resource.Generation,
		"created_time": resource.CreatedTime,
		"updated_time": resource.UpdatedTime,
		"created_by":   resource.CreatedBy,
		"updated_by":   resource.UpdatedBy,
	}

	// Unmarshal and add spec
	if len(resource.Spec) > 0 {
		var spec map[string]interface{}
		if err := json.Unmarshal(resource.Spec, &spec); err == nil {
			result["spec"] = spec
		}
	}

	// Unmarshal and add labels
	if len(resource.Labels) > 0 {
		var labels map[string]string
		if err := json.Unmarshal(resource.Labels, &labels); err == nil {
			result["labels"] = labels
		}
	}

	// Unmarshal and add status conditions
	if len(resource.StatusConditions) > 0 {
		var conditions []api.ResourceCondition
		if err := json.Unmarshal(resource.StatusConditions, &conditions); err == nil {
			result["status"] = map[string]interface{}{
				"conditions": conditions,
			}
		}
	}

	// Add owner reference for owned resources
	if resource.OwnerID != nil && *resource.OwnerID != "" {
		result["owner"] = map[string]interface{}{
			"id":   *resource.OwnerID,
			"kind": resource.OwnerKind,
			"href": resource.OwnerHref,
		}
	}

	return result
}

// getOwnerPlural returns the plural form of an owner kind.
// This is a simple mapping; in production, you'd look this up from the CRD registry.
func getOwnerPlural(kind string) string {
	plurals := map[string]string{
		"Cluster":  "clusters",
		"NodePool": "nodepools",
	}
	if plural, ok := plurals[kind]; ok {
		return plural
	}
	// Default: lowercase + "s"
	return kind + "s"
}
