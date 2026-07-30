package presenters

import (
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

type ResourceWithStatusesResponse struct {
	Resource        openapi.Resource       `json:"resource"`
	AdapterStatuses []openapi.AdapterStatus `json:"adapter_statuses"`
}

type ResourceWithStatusesList struct {
	Items []ResourceWithStatusesResponse `json:"items"`
	Page  int32                          `json:"page"`
	Size  int32                          `json:"size"`
	Total int64                          `json:"total"`
}

func PresentResourceWithStatusesList(
	items []services.ResourceWithStatuses, paging *api.PagingMeta,
) (*ResourceWithStatusesList, *errors.ServiceError) {
	presented := make([]ResourceWithStatusesResponse, 0, len(items))
	for _, item := range items {
		resource := PresentResource(item.Resource)

		statuses := make([]openapi.AdapterStatus, 0, len(item.AdapterStatuses))
		for _, as := range item.AdapterStatuses {
			s, err := PresentAdapterStatus(as)
			if err != nil {
				return nil, errors.GeneralError("failed to present adapter status: %s", err)
			}
			statuses = append(statuses, s)
		}

		presented = append(presented, ResourceWithStatusesResponse{
			Resource:        resource,
			AdapterStatuses: statuses,
		})
	}

	return &ResourceWithStatusesList{
		Items: presented,
		Page:  int32(paging.Page),
		Size:  int32(len(presented)),
		Total: paging.Total,
	}, nil
}
