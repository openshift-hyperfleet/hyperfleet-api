package presenters

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
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

	// Marshal empty StatusConditions
	statusConditionsJSON, err := json.Marshal([]api.ResourceCondition{})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cluster status conditions: %w", err)
	}

	return &api.Cluster{
		Kind:                     req.Kind,
		Name:                     req.Name,
		Spec:                     specJSON,
		Labels:                   labelsJSON,
		Generation:               1,
		StatusPhase:              "NotReady",
		StatusObservedGeneration: 0,
		StatusConditions:         statusConditionsJSON,
		CreatedBy:                createdBy,
		UpdatedBy:                createdBy,
	}, nil
}

// PresentCluster converts api.Cluster (GORM model) to openapi.Cluster
func PresentCluster(cluster *api.Cluster) openapi.Cluster {
	// Unmarshal Spec
	var spec map[string]interface{}
	if len(cluster.Spec) > 0 {
		_ = json.Unmarshal(cluster.Spec, &spec)
	}

	// Unmarshal Labels
	var labels map[string]string
	if len(cluster.Labels) > 0 {
		_ = json.Unmarshal(cluster.Labels, &labels)
	}

	// Unmarshal StatusConditions
	var statusConditions []api.ResourceCondition
	if len(cluster.StatusConditions) > 0 {
		_ = json.Unmarshal(cluster.StatusConditions, &statusConditions)
	}

	// Generate Href if not set (fallback)
	href := cluster.Href
	if href == "" {
		href = "/api/hyperfleet/v1/clusters/" + cluster.ID
	}

	result := openapi.Cluster{
		Id:          &cluster.ID,
		Kind:        cluster.Kind,
		Href:        &href,
		Name:        cluster.Name,
		Spec:        spec,
		Labels:      &labels,
		Generation:  cluster.Generation,
		CreatedTime: cluster.CreatedTime,
		UpdatedTime: cluster.UpdatedTime,
		CreatedBy:   cluster.CreatedBy,
		UpdatedBy:   cluster.UpdatedBy,
	}

	// Build ClusterStatus - set required fields with defaults if nil
	lastTransitionTime := time.Time{}
	if cluster.StatusLastTransitionTime != nil {
		lastTransitionTime = *cluster.StatusLastTransitionTime
	}

	lastUpdatedTime := time.Time{}
	if cluster.StatusLastUpdatedTime != nil {
		lastUpdatedTime = *cluster.StatusLastUpdatedTime
	}

	// Set phase, use NOT_READY as default if not set
	phase := api.PhaseNotReady
	if cluster.StatusPhase != "" {
		phase = api.ResourcePhase(cluster.StatusPhase)
	}

	// Convert domain ResourceConditions to openapi format
	openapiConditions := make([]openapi.ResourceCondition, len(statusConditions))
	for i, cond := range statusConditions {
		openapiConditions[i] = openapi.ResourceCondition{
			ObservedGeneration: cond.ObservedGeneration,
			CreatedTime:        cond.CreatedTime,
			LastUpdatedTime:    cond.LastUpdatedTime,
			Type:               cond.Type,
			Status:             openapi.ConditionStatus(string(cond.Status)),
			Reason:             cond.Reason,
			Message:            cond.Message,
			LastTransitionTime: cond.LastTransitionTime,
		}
	}

	result.Status = openapi.ClusterStatus{
		Phase:              openapi.ResourcePhase(string(phase)),
		ObservedGeneration: cluster.StatusObservedGeneration,
		Conditions:         openapiConditions,
		LastTransitionTime: lastTransitionTime,
		LastUpdatedTime:    lastUpdatedTime,
	}

	return result
}
