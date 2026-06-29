package presenters

import (
	"encoding/json"
	"fmt"

	"gorm.io/datatypes"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/util"
)

// ConvertResource converts an openapi.ResourceCreateRequest to an api.Resource GORM model.
// CreatedBy/UpdatedBy are set by the service layer from auth context.
func ConvertResource(req *openapi.ResourceCreateRequest) (*api.Resource, error) {
	specJSON, err := json.Marshal(req.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal spec: %w", err)
	}

	labelsJSON, err := marshalLabels(req.Labels)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal labels: %w", err)
	}

	return &api.Resource{
		Kind:   req.Kind,
		Name:   req.Name,
		Spec:   specJSON,
		Labels: labelsJSON,
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

	labels := unmarshalLabels(r.Labels)

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
			Conditions: []openapi.ResourceCondition{},
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

func marshalLabels(labels *map[string]string) (datatypes.JSON, error) {
	if labels == nil {
		return datatypes.JSON("{}"), nil
	}
	b, err := json.Marshal(*labels)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(b), nil
}

func unmarshalLabels(raw datatypes.JSON) *map[string]string {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	if len(m) == 0 {
		return nil
	}
	return &m
}
