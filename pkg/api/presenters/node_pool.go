package presenters

import (
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
)

// ConvertNodePool converts openapi.NodePool to api.NodePool (GORM model)
func ConvertNodePool(nodePool openapi.NodePool) *api.NodePool {
	// Use NodePoolFromOpenAPICreate helper
	createdBy := nodePool.CreatedBy
	if createdBy == "" {
		createdBy = "system"
	}

	ownerID := ""
	if nodePool.OwnerReferences.Id != nil {
		ownerID = *nodePool.OwnerReferences.Id
	}

	req := &openapi.NodePoolCreateRequest{
		Kind:   nodePool.Kind,
		Name:   nodePool.Name,
		Spec:   nodePool.Spec,
		Labels: nodePool.Labels,
	}

	return api.NodePoolFromOpenAPICreate(req, ownerID, createdBy)
}

// PresentNodePool converts api.NodePool (GORM model) to openapi.NodePool
func PresentNodePool(nodePool *api.NodePool) openapi.NodePool {
	// Use the ToOpenAPI method we implemented
	result := nodePool.ToOpenAPI()
	if result == nil {
		// Return empty nodePool if conversion fails
		return openapi.NodePool{}
	}
	return *result
}
