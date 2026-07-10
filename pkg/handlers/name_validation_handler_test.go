package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	"gorm.io/datatypes"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

func newNameValidationHandler(
	t *testing.T,
	ctrl *gomock.Controller,
	descriptor registry.EntityDescriptor,
) (*ResourceHandler, *services.MockResourceService) {
	t.Helper()
	mockResourceSvc := services.NewMockResourceService(ctrl)
	handler := NewResourceHandler(descriptor, mockResourceSvc)
	return handler, mockResourceSvc
}

func TestResourceHandler_Create_NameValidation(t *testing.T) {
	tests := []struct {
		name       string
		inputName  string
		nameMinLen int
		nameMaxLen int
		wantStatus int
	}{
		{"too short", "ab", 3, 53, http.StatusBadRequest},
		{"too long", "toolongname", 3, 5, http.StatusBadRequest},
		{"at min length", "abc", 3, 53, http.StatusCreated},
		{"at max length", "abcde", 3, 5, http.StatusCreated},
		{"no validation when zero lengths", "x", 0, 0, http.StatusCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			descriptor := registry.EntityDescriptor{
				Kind:       "TestEntity",
				Plural:     "testentities",
				NameMinLen: tt.nameMinLen,
				NameMaxLen: tt.nameMaxLen,
			}
			handler, mockSvc := newNameValidationHandler(t, ctrl, descriptor)

			if tt.wantStatus == http.StatusCreated {
				now := time.Now()
				mockSvc.EXPECT().Create(
					gomock.Any(), "TestEntity", gomock.AssignableToTypeOf(&api.Resource{}), gomock.Any(),
				).Return(&api.Resource{
					Meta:       api.Meta{ID: "test-id", CreatedTime: now, UpdatedTime: now},
					Kind:       "TestEntity",
					Name:       tt.inputName,
					Href:       "/api/hyperfleet/v1/testentities/test-id",
					Spec:       datatypes.JSON(`{"key":"value"}`),
					Generation: 1,
					CreatedBy:  "system@hyperfleet.local",
					UpdatedBy:  "system@hyperfleet.local",
				}, nil)
			}

			body := fmt.Sprintf(`{"kind":"TestEntity","name":%q,"spec":{"key":"value"}}`, tt.inputName)
			req := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/testentities", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			handler.Create(rr, req)
			Expect(rr.Code).To(Equal(tt.wantStatus))

			if tt.wantStatus == http.StatusCreated {
				var resp openapi.Resource
				err := json.Unmarshal(rr.Body.Bytes(), &resp)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Name).To(Equal(tt.inputName))
			}
		})
	}
}
