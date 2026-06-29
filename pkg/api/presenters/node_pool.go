package presenters

import (
	"encoding/json"
	"fmt"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
)

const nodePoolKind = "NodePool"

// ConvertNodePool converts openapi.NodePoolCreateRequest to api.NodePool (GORM model)
func ConvertNodePool(req *openapi.NodePoolCreateRequest, ownerID string) (*api.NodePool, error) {
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

	kind := req.Kind
	if kind == "" {
		kind = nodePoolKind
	}

	return &api.NodePool{
		Kind:      kind,
		Name:      req.Name,
		Spec:      specJSON,
		Labels:    labelsJSON,
		OwnerID:   ownerID,
		OwnerKind: "Cluster",
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

	result := openapi.NodePool{
		CreatedBy:   nodePool.CreatedBy,
		CreatedTime: nodePool.CreatedTime,
		DeletedBy:   nodePool.DeletedBy,
		DeletedTime: nodePool.DeletedTime,
		Generation:  nodePool.Generation,
		Href:        &href,
		Id:          &nodePool.ID,
		Kind:        nodePool.Kind,
		Labels:      &labels,
		Name:        nodePool.Name,
		OwnerReferences: openapi.ObjectReference{
			Id:   &nodePool.OwnerID,
			Kind: nodePool.OwnerKind,
			Href: &ownerHref,
		},
		Spec: spec,
		Status: openapi.NodePoolStatus{
			Conditions: openapiConditions,
		},
		UpdatedBy:   nodePool.UpdatedBy,
		UpdatedTime: nodePool.UpdatedTime,
	}

	return result, nil
}
