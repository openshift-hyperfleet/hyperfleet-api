package presenters

import (
	"encoding/json"
	"fmt"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/util"
)

// ConvertResource converts an openapi.ResourceCreateRequest to an api.Resource GORM model.
// CreatedBy/UpdatedBy are set by the service layer from auth context.
func ConvertResource(req *openapi.ResourceCreateRequest) (*api.Resource, error) {
	specJSON, err := json.Marshal(req.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal spec: %w", err)
	}

	labels, labelErr := convertLabelsToModel(req.Labels)
	if labelErr != nil {
		return nil, fmt.Errorf("invalid labels: %w", labelErr)
	}

	return &api.Resource{
		Kind:   req.Kind,
		Name:   req.Name,
		Spec:   specJSON,
		Labels: labels,
	}, nil
}

// ConvertResourceWithOwner converts a request to a child resource with owner references set.
func ConvertResourceWithOwner(
	req *openapi.ResourceCreateRequest,
	ownerID, ownerKind, ownerHref string,
) (*api.Resource, error) {
	resource, err := ConvertResource(req)
	if err != nil {
		return nil, err
	}
	resource.SetOwner(ownerID, ownerKind, ownerHref)
	return resource, nil
}

// PresentResource converts an api.Resource GORM model to an openapi.Resource.
func PresentResource(r *api.Resource) openapi.Resource {
	var spec map[string]interface{}
	if len(r.Spec) > 0 {
		if err := json.Unmarshal(r.Spec, &spec); err != nil {
			spec = nil
		}
	}

	labels := presentLabels(r.Labels)

	resp := openapi.Resource{
		Id:          r.ID,
		Kind:        r.Kind,
		Name:        r.Name,
		Href:        util.PtrString(r.Href),
		Spec:        spec,
		Labels:      labels,
		Generation:  r.Generation,
		CreatedTime: r.CreatedTime,
		UpdatedTime: r.UpdatedTime,
		CreatedBy:   r.CreatedBy,
		UpdatedBy:   r.UpdatedBy,
		DeletedTime: r.DeletedTime,
		Status: openapi.ResourceStatus{
			Conditions: presentResourceConditions(r.Conditions),
		},
	}

	if r.DeletedBy != nil {
		resp.DeletedBy = r.DeletedBy
	}

	if r.OwnerID != nil && *r.OwnerID != "" {
		resp.OwnerReferences = &struct {
			openapi.ObjectReference `yaml:",inline"`
		}{
			ObjectReference: openapi.ObjectReference{
				Id:   r.OwnerID,
				Kind: util.NilToEmptyString(r.OwnerKind),
				Href: r.OwnerHref,
			},
		}
	}

	if len(r.References) > 0 {
		refs := presentResourceReferences(r.References)
		resp.References = &refs
	}

	return resp
}

// PresentResourceList converts a slice of resources and paging metadata to an openapi.ResourceList.
func PresentResourceList(resources api.ResourceList, paging *api.PagingMeta) openapi.ResourceList {
	items := make([]openapi.Resource, 0, len(resources))
	for i := range resources {
		items = append(items, PresentResource(resources[i]))
	}
	return openapi.ResourceList{
		Items: items,
		Page:  int32(paging.Page), //nolint:gosec
		Size:  int32(paging.Size), //nolint:gosec
		Total: paging.Total,
	}
}

func presentResourceReferences(refs []api.ResourceReference) api.ReferenceMap {
	result := make(api.ReferenceMap)
	for _, ref := range refs {
		id := ref.TargetID
		objRef := openapi.ObjectReference{
			Id:   &id,
			Kind: ref.TargetKind,
		}
		if targetDesc, ok := registry.Get(ref.TargetKind); ok {
			href := fmt.Sprintf("/api/hyperfleet/v1/%s/%s", targetDesc.Plural, ref.TargetID)
			objRef.Href = &href
		}
		result[ref.RefType] = append(result[ref.RefType], objRef)
	}
	return result
}

func presentResourceConditions(conditions []api.ResourceCondition) []openapi.ResourceCondition {
	if len(conditions) == 0 {
		return []openapi.ResourceCondition{}
	}
	result := make([]openapi.ResourceCondition, len(conditions))
	for i, c := range conditions {
		result[i] = openapi.ResourceCondition{
			Type:               c.Type,
			Status:             openapi.ResourceConditionStatus(c.Status),
			Reason:             c.Reason,
			Message:            c.Message,
			ObservedGeneration: c.ObservedGeneration,
			CreatedTime:        c.CreatedTime,
			LastUpdatedTime:    c.LastUpdatedTime,
			LastTransitionTime: c.LastTransitionTime,
		}
	}
	return result
}

func convertLabelsToModel(labels *map[string]string) ([]api.ResourceLabel, error) {
	if labels == nil || len(*labels) == 0 {
		return nil, nil
	}
	result := make([]api.ResourceLabel, 0, len(*labels))
	for k, v := range *labels {
		if err := api.ValidateLabel(k, v); err != nil {
			return nil, err
		}
		result = append(result, api.ResourceLabel{Key: k, Value: v})
	}
	return result, nil
}

func presentLabels(labels []api.ResourceLabel) *map[string]string {
	if len(labels) == 0 {
		return nil
	}
	m := make(map[string]string, len(labels))
	for _, l := range labels {
		m[l.Key] = l.Value
	}
	return &m
}
