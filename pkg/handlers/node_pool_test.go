package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

func TestNodePoolHandler_List(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
	mockGenericSvc := services.NewMockGenericService(ctrl)
	mockGenericSvc.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ interface{}, _ *services.ListArguments, out interface{}) (*api.PagingMeta, *errors.ServiceError) {
			nodePools := out.(*[]api.NodePool)
			*nodePools = []api.NodePool{
				{
					Meta:             api.Meta{ID: "np-1", CreatedTime: now, UpdatedTime: now},
					Name:             "test-np",
					Spec:             []byte(`{}`),
					Labels:           []byte(`{}`),
					StatusConditions: []byte(`[]`),
					CreatedBy:        testSystemUser,
					UpdatedBy:        testSystemUser,
				},
			}
			return &api.PagingMeta{Page: 1, Size: 1, Total: 1}, nil
		})

	handler := NewNodePoolHandler(mockNodePoolSvc, mockGenericSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/hyperfleet/v1/nodepools", nil)
	rr := httptest.NewRecorder()

	handler.List(rr, req)
	Expect(rr.Code).To(Equal(http.StatusOK))

	var raw map[string]interface{}
	Expect(json.Unmarshal(rr.Body.Bytes(), &raw)).To(Succeed())
	Expect(raw).NotTo(HaveKey("kind"))
}
