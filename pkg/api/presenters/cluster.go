package presenters

import (
	"encoding/json"
	"fmt"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/util"
)

// ConvertCluster converts openapi.ClusterCreateRequest to api.Cluster (GORM model)
func ConvertCluster(req *openapi.ClusterCreateRequest, createdBy string) (*api.Cluster, error) {
	// Marshal Spec
	specJSON, err := json.Marshal(req.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cluster spec: %w", err)
	}

	// Marshal Labels
	labels := make(map[string]string)
	if req.Labels != nil {
		labels = *req.Labels
	}
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cluster labels: %w", err)
	}

	// Get Kind value, use default if not provided
	kind := "Cluster"
	if req.Kind != nil {
		kind = *req.Kind
	}

	return &api.Cluster{
		Kind:             kind,
		Name:             req.Name,
		Spec:             specJSON,
		Labels:           labelsJSON,
		Generation:       1,
		CreatedBy:        createdBy,
		UpdatedBy:        createdBy,
	}, nil
}

// Helper to convert string to openapi_types.Email
func toEmail(s string) openapi_types.Email {
	return openapi_types.Email(s)
}

// PresentCluster converts api.Cluster (GORM model) to openapi.Cluster
func PresentCluster(cluster *api.Cluster) (openapi.Cluster, error) {
	// Unmarshal Spec
	var spec map[string]interface{}
	if len(cluster.Spec) > 0 {
		if err := json.Unmarshal(cluster.Spec, &spec); err != nil {
			return openapi.Cluster{}, fmt.Errorf("failed to unmarshal cluster spec: %w", err)
		}
	}

	// Unmarshal Labels
	var labels map[string]string
	if len(cluster.Labels) > 0 {
		if err := json.Unmarshal(cluster.Labels, &labels); err != nil {
			return openapi.Cluster{}, fmt.Errorf("failed to unmarshal cluster labels: %w", err)
		}
	}

	// Unmarshal StatusConditions
	var statusConditions []api.ResourceCondition
	if len(cluster.StatusConditions) > 0 {
		if err := json.Unmarshal(cluster.StatusConditions, &statusConditions); err != nil {
			return openapi.Cluster{}, fmt.Errorf("failed to unmarshal cluster status conditions: %w", err)
		}
	}

	// Generate Href if not set (fallback)
	href := cluster.Href
	if href == "" {
		href = "/api/hyperfleet/v1/clusters/" + cluster.ID
	}

	// Convert domain ResourceConditions to openapi format
	openapiConditions := make([]openapi.ResourceCondition, len(statusConditions))
	for i, cond := range statusConditions {
		openapiConditions[i] = openapi.ResourceCondition{
			CreatedTime:        cond.CreatedTime,
			LastTransitionTime: cond.LastTransitionTime,
			LastUpdatedTime:    cond.LastUpdatedTime,
			Message:            cond.Message,
			ObservedGeneration: cond.ObservedGeneration,
			Reason:             cond.Reason,
			Status:             openapi.ResourceConditionStatus(cond.Status),
			Type:               cond.Type,
		}
	}

	result := openapi.Cluster{
		CreatedBy:   toEmail(cluster.CreatedBy),
		CreatedTime: cluster.CreatedTime,
		Generation:  cluster.Generation,
		Href:        &href,
		Id:          &cluster.ID,
		Kind:        util.PtrString(cluster.Kind),
		Labels:      &labels,
		Name:        cluster.Name,
		Spec:        spec,
		Status: openapi.ClusterStatus{
			Conditions: openapiConditions,
		},
		UpdatedBy:   toEmail(cluster.UpdatedBy),
		UpdatedTime: cluster.UpdatedTime,
	}

	return result, nil
}
