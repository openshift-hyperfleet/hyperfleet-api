package presenters

import (
	"github.com/openshift-hyperfleet/hyperfleet/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/api/openapi"
)

// ConvertCluster converts openapi.Cluster to api.Cluster (GORM model)
func ConvertCluster(cluster openapi.Cluster) *api.Cluster {
	// Use ClusterFromOpenAPICreate helper
	createdBy := cluster.CreatedBy
	if createdBy == "" {
		createdBy = "system"
	}

	req := &openapi.ClusterCreateRequest{
		Kind:   cluster.Kind,
		Name:   cluster.Name,
		Spec:   cluster.Spec,
		Labels: cluster.Labels,
	}

	return api.ClusterFromOpenAPICreate(req, createdBy)
}

// PresentCluster converts api.Cluster (GORM model) to openapi.Cluster
func PresentCluster(cluster *api.Cluster) openapi.Cluster {
	// Use the ToOpenAPI method we implemented
	result := cluster.ToOpenAPI()
	if result == nil {
		// Return empty cluster if conversion fails
		return openapi.Cluster{}
	}
	return *result
}
