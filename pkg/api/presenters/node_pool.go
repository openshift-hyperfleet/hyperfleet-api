package presenters

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
)

// ConvertNodePool converts openapi.NodePoolCreateRequest to api.NodePool (GORM model)
func ConvertNodePool(req *openapi.NodePoolCreateRequest, ownerID, createdBy string) (*api.NodePool, error) {
	// Marshal Spec
	specJSON, err := json.Marshal(req.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal nodepool spec: %w", err)
	}

	// Marshal Labels
	labels := make(map[string]string)
	if req.Labels != nil {
		labels = *req.Labels
	}
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal nodepool labels: %w", err)
	}

	// Marshal empty StatusConditions
	statusConditionsJSON, err := json.Marshal([]api.ResourceCondition{})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal nodepool status conditions: %w", err)
	}

	kind := "NodePool"
	if req.Kind != nil {
		kind = *req.Kind
	}

	return &api.NodePool{
		Kind:                     kind,
		Name:                     req.Name,
		Spec:                     specJSON,
		Labels:                   labelsJSON,
		OwnerID:                  ownerID,
		OwnerKind:                "Cluster",
		StatusPhase:              "NotReady",
		StatusObservedGeneration: 0,
		StatusConditions:         statusConditionsJSON,
		CreatedBy:                createdBy,
		UpdatedBy:                createdBy,
	}, nil
}

// PresentNodePool converts api.NodePool (GORM model) to openapi.NodePool
func PresentNodePool(nodePool *api.NodePool) (openapi.NodePool, error) {
	// Unmarshal Spec
	var spec map[string]interface{}
	if len(nodePool.Spec) > 0 {
		if err := json.Unmarshal(nodePool.Spec, &spec); err != nil {
			return openapi.NodePool{}, fmt.Errorf("failed to unmarshal nodepool spec: %w", err)
		}
	}

	// Unmarshal Labels
	var labels map[string]string
	if len(nodePool.Labels) > 0 {
		if err := json.Unmarshal(nodePool.Labels, &labels); err != nil {
			return openapi.NodePool{}, fmt.Errorf("failed to unmarshal nodepool labels: %w", err)
		}
	}

	// Unmarshal StatusConditions
	var statusConditions []api.ResourceCondition
	if len(nodePool.StatusConditions) > 0 {
		if err := json.Unmarshal(nodePool.StatusConditions, &statusConditions); err != nil {
			return openapi.NodePool{}, fmt.Errorf("failed to unmarshal nodepool status conditions: %w", err)
		}
	}

	// Generate Href if not set (fallback)
	href := nodePool.Href
	if href == "" {
		href = fmt.Sprintf("/api/hyperfleet/v1/clusters/%s/nodepools/%s", nodePool.OwnerID, nodePool.ID)
	}

	// Generate OwnerHref if not set (fallback)
	ownerHref := nodePool.OwnerHref
	if ownerHref == "" {
		ownerHref = "/api/hyperfleet/v1/clusters/" + nodePool.OwnerID
	}

	kind := nodePool.Kind
	result := openapi.NodePool{
		Id:         &nodePool.ID,
		Kind:       &kind,
		Href:       &href,
		Name:       nodePool.Name,
		Spec:       spec,
		Labels:     &labels,
		Generation: nodePool.Generation,
		OwnerReferences: openapi.ObjectReference{
			Id:   &nodePool.OwnerID,
			Kind: &nodePool.OwnerKind,
			Href: &ownerHref,
		},
		CreatedTime: nodePool.CreatedTime,
		UpdatedTime: nodePool.UpdatedTime,
		CreatedBy:   nodePool.CreatedBy,
		UpdatedBy:   nodePool.UpdatedBy,
	}

	// Build NodePoolStatus - set required fields with defaults if nil
	lastTransitionTime := time.Time{}
	if nodePool.StatusLastTransitionTime != nil {
		lastTransitionTime = *nodePool.StatusLastTransitionTime
	}

	lastUpdatedTime := time.Time{}
	if nodePool.StatusLastUpdatedTime != nil {
		lastUpdatedTime = *nodePool.StatusLastUpdatedTime
	}

	// Set phase, use NOT_READY as default if not set
	phase := api.PhaseNotReady
	if nodePool.StatusPhase != "" {
		phase = api.ResourcePhase(nodePool.StatusPhase)
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

	result.Status = openapi.NodePoolStatus{
		Phase:              openapi.ResourcePhase(string(phase)),
		ObservedGeneration: nodePool.StatusObservedGeneration,
		Conditions:         openapiConditions,
		LastTransitionTime: lastTransitionTime,
		LastUpdatedTime:    lastUpdatedTime,
	}

	return result, nil
}
